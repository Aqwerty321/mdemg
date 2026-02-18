#!/usr/bin/env bash
# cold-start.sh — One-command bootstrap for MDEMG + MCP on a fresh machine.
#
# Usage:
#   ./scripts/cold-start.sh                     # full bootstrap
#   ./scripts/cold-start.sh --skip-ollama       # skip Ollama pull (already have model)
#   ./scripts/cold-start.sh --project /path/to  # copy .vscode/mcp.json into another project
#
# Prerequisites: docker, go (1.22+), ollama (installed, not necessarily running)
#
# What this script does:
#   1. Starts Neo4j via docker compose
#   2. Applies all Cypher migrations
#   3. Ensures Ollama is running and the embedding model is pulled
#   4. Auto-detects embedding dimensions and fixes vector indexes if mismatched
#   5. Builds bin/mdemg and bin/mdemg-mcp
#   6. Starts the MDEMG server
#   7. Optionally copies .vscode/mcp.json into a target project
#   8. Runs a smoke test (observe → resume round-trip)

set -euo pipefail

# ─── Configuration (override via env or .env) ───────────────────────────────
REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO"

# Source .env if present (don't fail if missing)
if [[ -f .env ]]; then
  set -a; source .env; set +a
fi

NEO4J_CONTAINER="${NEO4J_CONTAINER:-mdemg-neo4j}"
NEO4J_USER="${NEO4J_USER:-neo4j}"
NEO4J_PASS="${NEO4J_PASS:-testpassword}"
OLLAMA_ENDPOINT="${OLLAMA_ENDPOINT:-http://localhost:11434}"
OLLAMA_MODEL="${OLLAMA_MODEL:-qwen3-embedding:8b-fp16}"
EMBEDDING_PROVIDER="${EMBEDDING_PROVIDER:-ollama}"
LISTEN_ADDR="${LISTEN_ADDR:-:9999}"
MDEMG_PORT="${LISTEN_ADDR#:}"
MDEMG_URL="http://localhost:${MDEMG_PORT}"
SPACE="${SPACE:-mdemg-dev}"
SESSION="${SESSION:-claude-core}"

# ─── Parse flags ─────────────────────────────────────────────────────────────
SKIP_OLLAMA=false
TARGET_PROJECT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-ollama) SKIP_OLLAMA=true; shift ;;
    --project)     TARGET_PROJECT="$2"; shift 2 ;;
    -h|--help)
      head -12 "$0" | tail -10
      exit 0
      ;;
    *) echo "Unknown flag: $1"; exit 1 ;;
  esac
done

# ─── Helpers ─────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

ok()   { echo -e "${GREEN}[OK]${NC}   $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; exit 1; }
info() { echo -e "${YELLOW}[..]${NC}   $1"; }

cypher() {
  docker exec -i "$NEO4J_CONTAINER" cypher-shell -u "$NEO4J_USER" -p "$NEO4J_PASS" "$@"
}

# ─── Step 1: Prerequisites ──────────────────────────────────────────────────
info "Checking prerequisites..."
command -v docker >/dev/null 2>&1 || fail "docker not found"
command -v go     >/dev/null 2>&1 || fail "go not found (need 1.22+)"
if [[ "$SKIP_OLLAMA" == false ]]; then
  command -v ollama >/dev/null 2>&1 || fail "ollama not found (install: curl -fsSL https://ollama.com/install.sh | sh)"
fi
ok "Prerequisites: docker, go$(if [[ "$SKIP_OLLAMA" == false ]]; then echo ", ollama"; fi)"

# ─── Step 2: Start Neo4j ────────────────────────────────────────────────────
info "Starting Neo4j..."
docker compose up -d 2>/dev/null || docker-compose up -d 2>/dev/null

info "Waiting for Neo4j to become healthy..."
TRIES=0
until docker exec "$NEO4J_CONTAINER" cypher-shell -u "$NEO4J_USER" -p "$NEO4J_PASS" "RETURN 1" >/dev/null 2>&1; do
  TRIES=$((TRIES + 1))
  if [[ $TRIES -ge 60 ]]; then
    fail "Neo4j did not become healthy within 60s"
  fi
  sleep 1
done
ok "Neo4j healthy (container: $NEO4J_CONTAINER)"

# ─── Step 3: Apply migrations ───────────────────────────────────────────────
info "Checking schema version..."
SCHEMA_VER=$(echo "MATCH (s:SchemaMeta {key:'schema'}) RETURN s.current_version AS v" | cypher 2>/dev/null | tail -1 | tr -d '[:space:]' || echo "")

if [[ -z "$SCHEMA_VER" || "$SCHEMA_VER" == "v" ]]; then
  info "SchemaMeta missing — applying all migrations..."
  for f in migrations/V*.cypher; do
    echo -n "  $f... "
    cypher < "$f" >/dev/null 2>&1 && echo "done" || fail "Migration failed: $f"
  done

  SCHEMA_VER=$(echo "MATCH (s:SchemaMeta {key:'schema'}) RETURN s.current_version AS v" | cypher 2>/dev/null | tail -1 | tr -d '[:space:]')
  [[ -n "$SCHEMA_VER" && "$SCHEMA_VER" != "v" ]] || fail "SchemaMeta still missing after migrations"
  ok "Migrations applied (schema version: $SCHEMA_VER)"
else
  ok "Schema already at version $SCHEMA_VER — skipping migrations"
fi

# ─── Step 4: Ollama + embedding model ───────────────────────────────────────
if [[ "$EMBEDDING_PROVIDER" == "ollama" && "$SKIP_OLLAMA" == false ]]; then
  info "Checking Ollama..."

  if ! curl -sf "$OLLAMA_ENDPOINT/api/tags" >/dev/null 2>&1; then
    info "Starting Ollama..."
    ollama serve >/tmp/ollama-cold-start.log 2>&1 &
    sleep 3
    curl -sf "$OLLAMA_ENDPOINT/api/tags" >/dev/null 2>&1 || fail "Ollama failed to start (see /tmp/ollama-cold-start.log)"
  fi
  ok "Ollama running"

  if ! ollama list 2>/dev/null | grep -q "$OLLAMA_MODEL"; then
    info "Pulling $OLLAMA_MODEL (this may take a while on first run)..."
    ollama pull "$OLLAMA_MODEL" || fail "Failed to pull $OLLAMA_MODEL"
  fi
  ok "Model ready: $OLLAMA_MODEL"
fi

# ─── Step 5: Fix vector index dimensions ────────────────────────────────────
# Auto-detect actual embedding dimensions and compare with Neo4j indexes.

if [[ "$EMBEDDING_PROVIDER" == "ollama" ]]; then
  info "Detecting embedding dimensions for $OLLAMA_MODEL..."
  ACTUAL_DIMS=$(curl -s "$OLLAMA_ENDPOINT/api/embed" \
    -d "{\"model\":\"$OLLAMA_MODEL\",\"input\":\"dimension probe\"}" \
    | python3 -c "import json,sys; print(len(json.load(sys.stdin)['embeddings'][0]))" 2>/dev/null || echo "")

  if [[ -z "$ACTUAL_DIMS" ]]; then
    info "Could not detect dimensions — skipping index fix (server will auto-correct on first embed)"
  else
    # Get current index dimensions from Neo4j
    INDEX_DIMS=$(echo "SHOW INDEXES YIELD name, options WHERE name = 'memNodeEmbedding' RETURN options" \
      | cypher 2>/dev/null \
      | grep -oP 'vector\.dimensions.*?:\s*\K\d+' || echo "")

    if [[ -n "$INDEX_DIMS" && "$INDEX_DIMS" != "$ACTUAL_DIMS" ]]; then
      info "Dimension mismatch: indexes=$INDEX_DIMS, model=$ACTUAL_DIMS — rebuilding indexes..."

      # Drop old indexes
      echo "DROP INDEX memNodeEmbedding IF EXISTS;" | cypher >/dev/null 2>&1
      echo "DROP INDEX observationEmbedding IF EXISTS;" | cypher >/dev/null 2>&1
      echo "DROP INDEX symbolNodeEmbedding IF EXISTS;" | cypher >/dev/null 2>&1

      # Clear any embeddings with wrong dimensions
      echo "MATCH (n:MemoryNode) WHERE n.embedding IS NOT NULL REMOVE n.embedding;" | cypher >/dev/null 2>&1
      echo "MATCH (o:Observation) WHERE o.embedding IS NOT NULL REMOVE o.embedding;" | cypher >/dev/null 2>&1
      echo "MATCH (s:SymbolNode) WHERE s.embedding IS NOT NULL REMOVE s.embedding;" | cypher >/dev/null 2>&1

      # Recreate with correct dimensions
      echo "CREATE VECTOR INDEX memNodeEmbedding IF NOT EXISTS FOR (n:MemoryNode) ON n.embedding OPTIONS {indexConfig:{\"vector.dimensions\":$ACTUAL_DIMS,\"vector.similarity_function\":\"cosine\"}};" | cypher >/dev/null 2>&1
      echo "CREATE VECTOR INDEX observationEmbedding IF NOT EXISTS FOR (o:Observation) ON o.embedding OPTIONS {indexConfig:{\"vector.dimensions\":$ACTUAL_DIMS,\"vector.similarity_function\":\"cosine\"}};" | cypher >/dev/null 2>&1
      echo "CREATE VECTOR INDEX symbolNodeEmbedding IF NOT EXISTS FOR (s:SymbolNode) ON s.embedding OPTIONS {indexConfig:{\"vector.dimensions\":$ACTUAL_DIMS,\"vector.similarity_function\":\"cosine\"}};" | cypher >/dev/null 2>&1

      ok "Vector indexes rebuilt: $INDEX_DIMS → $ACTUAL_DIMS dimensions"
    elif [[ -n "$INDEX_DIMS" ]]; then
      ok "Vector index dimensions match model ($ACTUAL_DIMS) — no fix needed"
    else
      info "No vector indexes found (will be created by migrations or server)"
    fi
  fi
fi

# ─── Step 6: Build binaries ─────────────────────────────────────────────────
info "Building MDEMG binaries..."
go build -o bin/mdemg ./cmd/server         || fail "go build cmd/server failed"
go build -o bin/mdemg-mcp ./cmd/mcp-server || fail "go build cmd/mcp-server failed"
ok "Built bin/mdemg and bin/mdemg-mcp"

# ─── Step 7: Start MDEMG server ─────────────────────────────────────────────
info "Starting MDEMG server..."

# Kill any existing instance
pkill -f "bin/mdemg$" 2>/dev/null || true
sleep 1

"$REPO/bin/mdemg" > "$REPO/mdemg.stdout.log" 2>&1 &
MDEMG_PID=$!

# Wait for server to be ready
TRIES=0
until curl -sf "$MDEMG_URL/healthz" >/dev/null 2>&1; do
  TRIES=$((TRIES + 1))
  if [[ $TRIES -ge 30 ]]; then
    echo "--- Last 20 lines of mdemg.stdout.log ---"
    tail -20 "$REPO/mdemg.stdout.log"
    fail "MDEMG server did not start within 30s (PID: $MDEMG_PID)"
  fi
  # Check if process died
  if ! kill -0 "$MDEMG_PID" 2>/dev/null; then
    echo "--- Last 20 lines of mdemg.stdout.log ---"
    tail -20 "$REPO/mdemg.stdout.log"
    fail "MDEMG server exited immediately"
  fi
  sleep 1
done
ok "MDEMG server running on $MDEMG_URL (PID: $MDEMG_PID)"

# ─── Step 8: Copy MCP config to target project ──────────────────────────────
if [[ -n "$TARGET_PROJECT" ]]; then
  info "Setting up MCP config in $TARGET_PROJECT..."
  mkdir -p "$TARGET_PROJECT/.vscode"
  cat > "$TARGET_PROJECT/.vscode/mcp.json" << EOF
{
  "servers": {
    "mdemg": {
      "command": "$REPO/bin/mdemg-mcp",
      "args": [],
      "env": {
        "MDEMG_ENDPOINT": "$MDEMG_URL"
      }
    }
  }
}
EOF
  ok "Created $TARGET_PROJECT/.vscode/mcp.json"

  # Copy CLAUDE.md so agents follow the CMS contract in the target project
  if [[ -f "$REPO/CLAUDE.md" ]]; then
    cp "$REPO/CLAUDE.md" "$TARGET_PROJECT/CLAUDE.md"
    ok "Copied CLAUDE.md → $TARGET_PROJECT/CLAUDE.md"
  fi
fi

# Also ensure workspace mcp.json exists
if [[ ! -f "$REPO/.vscode/mcp.json" ]]; then
  mkdir -p "$REPO/.vscode"
  cat > "$REPO/.vscode/mcp.json" << EOF
{
  "servers": {
    "mdemg": {
      "command": "$REPO/bin/mdemg-mcp",
      "args": [],
      "env": {
        "MDEMG_ENDPOINT": "$MDEMG_URL"
      }
    }
  }
}
EOF
  ok "Created $REPO/.vscode/mcp.json"
fi

# ─── Step 9: Smoke test ─────────────────────────────────────────────────────
info "Running smoke test..."

# Observe
OBS_RESULT=$(curl -s -X POST "$MDEMG_URL/v1/conversation/observe" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"$SPACE\",\"session_id\":\"$SESSION\",\"content\":\"cold-start smoke test $(date -Iseconds)\",\"obs_type\":\"progress\"}" 2>&1)

OBS_ID=$(echo "$OBS_RESULT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('obs_id',''))" 2>/dev/null || echo "")

if [[ -z "$OBS_ID" ]]; then
  echo "  observe response: $OBS_RESULT"
  fail "Smoke test: observe did not return obs_id"
fi

# Resume
RESUME_RESULT=$(curl -s -X POST "$MDEMG_URL/v1/conversation/resume" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"$SPACE\",\"session_id\":\"$SESSION\",\"max_observations\":5}" 2>&1)

OBS_COUNT=$(echo "$RESUME_RESULT" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('observations',[])))" 2>/dev/null || echo "0")

if [[ "$OBS_COUNT" -gt 0 ]]; then
  ok "Smoke test passed: observed ($OBS_ID), resumed ($OBS_COUNT observations)"
else
  echo "  resume response: $RESUME_RESULT"
  fail "Smoke test: resume returned 0 observations"
fi

# ─── Done ────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  MDEMG cold start complete!${NC}"
echo -e "${GREEN}════════════════════════════════════════════════════════════════${NC}"
echo ""
echo "  Server:     $MDEMG_URL (PID: $MDEMG_PID)"
echo "  Healthz:    curl $MDEMG_URL/healthz"
echo "  MCP bridge: $REPO/bin/mdemg-mcp"
echo "  Log:        $REPO/mdemg.stdout.log"
echo ""
echo "  To stop:    pkill -f 'bin/mdemg$'"
echo "  To restart: $REPO/bin/mdemg &"
echo ""
if [[ -n "$TARGET_PROJECT" ]]; then
  echo "  MCP config: $TARGET_PROJECT/.vscode/mcp.json"
  echo "  Open that project in VS Code and reload the window."
fi
echo ""
