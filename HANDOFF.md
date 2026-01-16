# MDEMG Development Handoff Document

**Date:** 2026-01-16
**Status:** Core Features Complete - 9 Tasks Merged (PRs #1-7 + Tasks 7-9)

> **Vision Document:** See [VISION.md](./VISION.md) for the complete architectural philosophy and design principles.

---

## 🚀 RESUME DEVELOPMENT - START HERE

### Before Coding
1. **Start the infrastructure:**
   ```bash
   cd /Users/reh3376/mdemg
   ./start-mdemg.sh
   ```
   This starts Neo4j (if not running), checks Ollama, and launches the MDEMG service on :8082.

2. **Verify everything is working:**
   ```bash
   curl -s http://localhost:8082/readyz | jq
   # Should show: {"status":"ready","embedding_provider":"ollama:nomic-embed-text",...}
   ```

3. **If Cursor was restarted**, the MCP tools (`memory_store`, `memory_recall`, etc.) should be available. Test with `memory_status`.

### What's Working
- ✅ Neo4j graph database with vector indexes (768-dim for Ollama)
- ✅ Go service with embedding generation (Ollama/OpenAI)
- ✅ Full retrieval pipeline: vector recall → graph expansion → spreading activation → scoring
- ✅ MCP server with 5 memory tools for agent integration
- ✅ Ingest and retrieve endpoints with auto-embedding
- ✅ **Hebbian Learning** - `ApplyCoactivation()` creates CO_ACTIVATED_WITH edges (PR #2)
- ✅ **Semantic Edges on Ingest** - Auto-creates ASSOCIATED_WITH edges to similar nodes (PR #6)
- ✅ **Edge Weight Decay CLI** - `cmd/decay` for memory maintenance (PR #1)
- ✅ **Consolidation CLI** - `cmd/consolidate` for cluster detection and abstraction promotion (PR #5)
- ✅ **Metrics Endpoint** - `GET /v1/metrics` for graph health monitoring (PR #4)
- ✅ **Integration Tests** - Comprehensive test suite for retrieval pipeline (PR #3)
- ✅ **Batch Ingest Endpoint** - `POST /v1/memory/ingest/batch` for bulk imports (Task 7)
- ✅ **Reflection Endpoint** - `POST /v1/memory/reflect` for deep context exploration (Task 8, PR #7)
- ✅ **Anomaly Detection** - Non-blocking duplicate/stale detection on ingest (Task 9)

### What's Next (Priority Order)
1. **Use the system!** - The more memories stored, the more emergent behaviors appear
2. **Graph pruning** - Remove weak/deprecated edges, merge redundant nodes
3. **Proactive surfacing** - Context suggestions, anomaly alerts

### Key Commands
```bash
# Start everything
./start-mdemg.sh

# Test retrieval
curl -s http://localhost:8082/v1/memory/retrieve \
  -H 'content-type: application/json' \
  -d '{"space_id":"ide-agent","query_text":"your query here","top_k":5}' | jq

# Check Neo4j graph
docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword \
  "MATCH (n:MemoryNode) RETURN n.name, n.path LIMIT 10"

# Stop service
pkill -f mdemg-server
```

---

## Project Overview

MDEMG (Multi-Dimensional Emergent Memory Graph) is a long-term memory system for AI agents built on Neo4j with native vector indexes. It implements retrieval-augmented memory with spreading activation and Hebbian learning.

### Core Design Principles (DO NOT VIOLATE)
- **Vector index = recall** (fast candidate generation via cosine similarity)
- **Graph = reasoning** (typed edges with evidence weights)
- **Runtime = activation physics** (spreading activation computed in-memory, NOT persisted)
- **DB writes = learning deltas only** (bounded, regularized - no per-request activation writes)

---

## Current State

### ✅ Completed
| Item | Details |
|------|---------|
| Go 1.25.6 | Installed via Homebrew at `/opt/homebrew/bin/go` |
| Neo4j 5 | Running in Docker container `mdemg-neo4j` |
| Schema migrations | V0001-V0004 applied (current version = 4) |
| Vector indexes | `memNodeEmbedding` (768-dim for Ollama, 1536-dim for OpenAI) |
| Go service | Full retrieval pipeline working with embeddings |
| Cursor workspace | `.cursor/settings.json` + `.vscode/` configured for Go |
| **Embedding generation** | `internal/embeddings/` package with OpenAI and Ollama providers |
| **End-to-end retrieval** | Tested with Ollama `nomic-embed-text` model |

### 🆕 New in This Session (2026-01-16) - 9 Tasks Merged

| PR/Task | Feature | Files Added/Modified |
|---------|---------|---------------------|
| #1 | **Edge Weight Decay CLI** | `cmd/decay/main.go` (617 lines), tests (443 lines) |
| #2 | **Hebbian Learning Loop** | `internal/learning/service.go`, unit + integration tests (2079 lines) |
| #3 | **Integration Tests** | `tests/integration/` - helpers, ingest, retrieval (1754 lines) |
| #4 | **Graph Health Metrics** | `GET /v1/metrics` endpoint, handler tests (570 lines) |
| #5 | **Consolidation CLI** | `cmd/consolidate/main.go` (760 lines), tests (829 lines) |
| #6 | **Semantic Edges on Ingest** | Auto-creates ASSOCIATED_WITH edges (182 lines) |
| Task 7 | **Batch Ingest Endpoint** | `POST /v1/memory/ingest/batch` - bulk imports up to 100 items |
| #7 | **Reflection Endpoint** | `POST /v1/memory/reflect` - `internal/retrieval/reflection.go`, tests |
| Task 9 | **Anomaly Detection** | `internal/anomaly/` - non-blocking duplicate/stale detection (783 lines) |

**Key Implementations:**

1. **Hebbian Learning** (`internal/learning/service.go`)
   - Formula: `Δw = η * a_i * a_j - μ * w_ij`
   - Configurable: η=0.1, μ=0.01, wmin=0.0, wmax=1.0
   - Edge cap per request (LEARNING_EDGE_CAP_PER_REQUEST=200)

2. **Edge Decay** (`cmd/decay/`)
   - Formula: `w_new = w_old * exp(-decay_rate * days)`
   - CLI: `go run ./cmd/decay --space-id <id> --dry-run`
   - Protection for pinned edges

3. **Semantic Edge Creation** (`internal/retrieval/service.go`)
   - Auto-creates ASSOCIATED_WITH edges on ingest
   - Configurable: SEMANTIC_EDGE_TOP_N=5, SEMANTIC_EDGE_MIN_SIMILARITY=0.7

4. **Consolidation** (`cmd/consolidate/`)
   - Detects clusters via CO_ACTIVATED_WITH edges
   - Creates abstraction nodes at layer+1
   - CLI: `go run ./cmd/consolidate --space-id <id> --dry-run`

5. **Metrics Endpoint** (`GET /v1/metrics`)
   - Node/edge counts, distribution by layer/type
   - Hub nodes, orphan count, avg edge weight, 24h activity

6. **Batch Ingest** (`POST /v1/memory/ingest/batch`)
   - Bulk ingest up to 100 observations per request
   - Partial success with HTTP 207 Multi-Status
   - Configurable: BATCH_INGEST_MAX_ITEMS=100

7. **Reflection Endpoint** (`POST /v1/memory/reflect`)
   - Stage 1: SEED - Vector search for topic
   - Stage 2: EXPAND - Lateral traversal via CO_ACTIVATED_WITH/ASSOCIATED_WITH
   - Stage 3: ABSTRACT - Upward traversal via ABSTRACTS_TO
   - Insight generation: clusters, patterns, knowledge gaps

8. **Anomaly Detection** (`internal/anomaly/`)
   - Non-blocking detection during ingest (100ms timeout)
   - Duplicate detection: vector similarity > 0.95
   - Stale update detection: nodes not modified in 30+ days
   - Configurable via ANOMALY_* environment variables

---

## Quick Start Commands

```bash
# ONE-COMMAND STARTUP (recommended)
./start-mdemg.sh

# OR manually:
cd /Users/reh3376/mdemg
docker compose up -d
cd mdemg_build/service
export NEO4J_URI=bolt://localhost:7687
export NEO4J_USER=neo4j
export NEO4J_PASS=testpassword
export REQUIRED_SCHEMA_VERSION=4
export LISTEN_ADDR=:8082
export EMBEDDING_PROVIDER=ollama
go run ./cmd/server

# Test retrieval with query_text
curl -s http://localhost:8082/v1/memory/retrieve \
  -H 'content-type: application/json' \
  -d '{"space_id":"demo","query_text":"How does vector search work?","top_k":5}' | jq

# Ingest with auto-embedding
curl -s http://localhost:8082/v1/memory/ingest \
  -H 'content-type: application/json' \
  -d '{
    "space_id": "demo",
    "path": "/concepts/example",
    "name": "Example Concept",
    "source": "manual",
    "timestamp": "2026-01-15T12:00:00Z",
    "content": "Description of the concept for embedding generation."
  }' | jq

# Batch ingest (up to 100 items per request)
curl -s http://localhost:8082/v1/memory/ingest/batch \
  -H 'content-type: application/json' \
  -d '{
    "space_id": "demo",
    "observations": [
      {"timestamp": "2026-01-16T12:00:00Z", "source": "batch", "name": "Item 1", "content": "First item"},
      {"timestamp": "2026-01-16T12:00:01Z", "source": "batch", "name": "Item 2", "content": "Second item"}
    ]
  }' | jq
```

**Neo4j Browser:** http://localhost:7474 (neo4j / testpassword)

---

## MCP Tools for Agents

The MCP server exposes 5 tools that agents can use:

| Tool | Purpose | When to Use |
|------|---------|-------------|
| `memory_store` | Save observations, patterns, decisions | After solving problems, learning patterns |
| `memory_recall` | Retrieve relevant memories | When starting work, making decisions |
| `memory_associate` | Link concepts explicitly | When discovering relationships |
| `memory_reflect` | Deep exploration of a topic | Before major decisions, reviewing context |
| `memory_status` | Check system health | Diagnostics |

### Expected Emergent Behaviors

As the system accumulates memories over time:

1. **Concept Clustering** - Similar code patterns will naturally group together in vector space
2. **Cross-Project Transfer** - Knowledge from one project will surface when relevant to another
3. **Workflow Recognition** - The system will learn recurring patterns in how you work
4. **Abstraction Emergence** - General principles will crystallize from specific examples via the `ABSTRACTS_TO` relationship

### MCP Configuration

The MCP server is configured in `~/.cursor/mcp.json`:
```json
{
  "mcpServers": {
    "mdemg": {
      "command": "/Users/reh3376/mdemg/mdemg_build/mcp-server/mdemg-mcp",
      "args": [],
      "env": {
        "MDEMG_ENDPOINT": "http://localhost:8082"
      }
    }
  }
}
```

**Note:** Restart Cursor after modifying mcp.json for changes to take effect.

---

## TODO List - Remaining Work

### ✅ Completed (2026-01-16)
- [x] **Learning Loop** - `ApplyCoactivation()` with Hebbian formula (PR #2)
- [x] **Edge Weight Decay CLI** - `cmd/decay` with exponential decay (PR #1)
- [x] **Semantic Edges on Ingest** - Auto-creates ASSOCIATED_WITH edges (PR #6)
- [x] **Integration Tests** - Comprehensive test suite (PR #3)
- [x] **Graph Health Metrics** - `GET /v1/metrics` endpoint (PR #4)
- [x] **Consolidation CLI** - Cluster detection and abstraction promotion (PR #5)
- [x] **Batch Ingest Endpoint** (Task 7) - `POST /v1/memory/ingest/batch`
  - Up to 100 observations per request (configurable via BATCH_INGEST_MAX_ITEMS)
  - Partial success support with HTTP 207 Multi-Status
  - Auto-generates embeddings for items without pre-computed embeddings
  - Per-item results with status, node_id, obs_id
- [x] **Reflection Endpoint** (Task 8) - `POST /v1/memory/reflect` (PR #7)
  - Multi-hop traversal with 3 stages: SEED (vector search) → EXPAND (lateral traversal) → ABSTRACT (upward hierarchy)
  - Insight generation: cluster detection, pattern identification, knowledge gaps
  - Comprehensive unit and integration tests
  - `internal/retrieval/reflection.go`, `internal/api/handlers.go`
- [x] **Anomaly Detection Service** (Task 9)
  - Non-blocking detection during ingest (100ms timeout)
  - Duplicate detection: vector similarity > 0.95
  - Stale update detection: nodes not modified in 30+ days
  - `internal/anomaly/types.go`, `internal/anomaly/service.go`, unit tests
  - Integration test: `TestIngestAnomalyDuplicateDetection`

### Priority 2: Testing & Validation
- [x] **Golden tests for scoring** - Complete at `tests/integration/scoring_golden_test.go`
  - `TestScoringGolden` - Validates 5-node graph with controlled edges, verifies ranking order
  - `TestScoringGoldenDeterminism` - Verifies ranking consistency across multiple runs
  - `TestScoringComponentBreakdown` - Tests isolated node scoring components
  - Fixed `TestScoringDeterminism` tolerance to account for Hebbian learning effects

### Priority 4: Future Enhancements
- [ ] **Graph pruning** - Remove weak/deprecated edges, merge redundant nodes
- [ ] **Proactive surfacing** - Context suggestions, anomaly alerts
- [ ] **Agent consulting service** - SME-like guidance API

---

## Key Files Reference

### Configuration
| File | Purpose |
|------|---------|
| `docker-compose.yml` | Neo4j container definition |
| `mdemg_build/service/.env` | Service environment variables |
| `mdemg_build/migrations/V*.cypher` | Database schema migrations |

### Go Service (`mdemg_build/service/`)
| File | Purpose |
|------|---------|
| `cmd/server/main.go` | Entry point, schema version check |
| `cmd/decay/main.go` | Edge weight decay CLI (NEW) |
| `cmd/consolidate/main.go` | Cluster detection + abstraction promotion CLI (NEW) |
| `internal/config/config.go` | Environment variable parsing |
| `internal/db/neo4j.go` | Driver creation, schema validation |
| `internal/api/server.go` | HTTP routes registration |
| `internal/api/handlers.go` | Request handlers (includes /v1/metrics) |
| `internal/api/handlers_test.go` | Handler unit tests (NEW) |
| `internal/embeddings/embeddings.go` | Embedder interface + factory |
| `internal/embeddings/openai.go` | OpenAI embedding provider |
| `internal/embeddings/ollama.go` | Ollama embedding provider |
| `internal/retrieval/service.go` | Vector recall + expansion + semantic edges |
| `internal/retrieval/activation.go` | Spreading activation |
| `internal/retrieval/scoring.go` | Final ranking |
| `internal/retrieval/reflection.go` | Reflection endpoint (SEED/EXPAND/ABSTRACT) |
| `internal/retrieval/reflection_test.go` | Reflection unit tests |
| `internal/learning/service.go` | Hebbian learning (ApplyCoactivation) |
| `internal/learning/service_test.go` | Learning unit tests (NEW) |
| `internal/anomaly/types.go` | Anomaly detection types and config (NEW) |
| `internal/anomaly/service.go` | Anomaly detection logic (NEW) |
| `internal/anomaly/service_test.go` | Anomaly unit tests (NEW) |
| `internal/models/models.go` | Request/response types |
| `tests/integration/` | Integration test suite (NEW) |

### MCP Server (`mdemg_build/mcp-server/`)
| File | Purpose |
|------|---------|
| `main.go` | MCP server with 5 memory tools |
| `mdemg-mcp` | Compiled binary (run via Cursor MCP) |

### Documentation
| File | Purpose |
|------|---------|
| `CLAUDE.md` | AI assistant context |
| `dev_conns.md` | Connection strings reference |
| `mdemg_build/docs/00_README.md` | Documentation index |
| `mdemg_build/docs/01_Architecture.md` | System architecture |
| `mdemg_build/docs/02_Graph_Schema.md` | Labels, relationships, properties |
| `mdemg_build/docs/06_Retrieval_API_and_Scoring.md` | Scoring algorithm |
| `START_HERE_build_agent.md` | Original build instructions |

---

## Environment Variables

```bash
# Required
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASS=testpassword
REQUIRED_SCHEMA_VERSION=4

# Retrieval tuning
VECTOR_INDEX_NAME=memNodeEmbedding
DEFAULT_CANDIDATE_K=200
DEFAULT_TOP_K=20
DEFAULT_HOP_DEPTH=2
MAX_NEIGHBORS_PER_NODE=50
MAX_TOTAL_EDGES_FETCHED=5000
LEARNING_EDGE_CAP_PER_REQUEST=200

# Service
LISTEN_ADDR=:8082

# Batch ingest
BATCH_INGEST_MAX_ITEMS=100         # Max observations per batch request (1-1000)

# Anomaly detection
ANOMALY_DETECTION_ENABLED=true     # Enable/disable anomaly detection
ANOMALY_DUPLICATE_THRESHOLD=0.95   # Vector similarity threshold for duplicates
ANOMALY_OUTLIER_STDDEVS=2.0        # Std devs for outlier detection
ANOMALY_STALE_DAYS=30              # Days after which update is stale
ANOMALY_MAX_CHECK_MS=100           # Timeout for anomaly checks

# Embedding Provider
EMBEDDING_PROVIDER=ollama          # "openai" or "ollama" or "" (disabled)

# OpenAI settings (if EMBEDDING_PROVIDER=openai)
OPENAI_API_KEY=sk-...  # Set in environment, never commit!
OPENAI_MODEL=text-embedding-ada-002
OPENAI_ENDPOINT=https://api.openai.com/v1

# Ollama settings (if EMBEDDING_PROVIDER=ollama)
OLLAMA_ENDPOINT=http://localhost:11434
OLLAMA_MODEL=nomic-embed-text
```

---

## Graph Schema Quick Reference

### Labels
- `:TapRoot` - Singleton per space_id
- `:MemoryNode` - Main memory nodes (has `embedding` property)
- `:Observation` - Append-only events
- `:SchemaMeta` - Schema version tracking

### Key Relationships
- `ASSOCIATED_WITH` - Semantic similarity
- `CO_ACTIVATED_WITH` - Learning signal (Hebbian)
- `CAUSES`, `ENABLES` - Causal links
- `TEMPORALLY_ADJACENT` - Time sequence
- `ABSTRACTS_TO` / `INSTANTIATES` - Layer hierarchy
- `HAS_OBSERVATION` - Node to observation link

### Vector Index
- Name: `memNodeEmbedding`
- Dimensions: 768 (Ollama nomic-embed-text) or 1536 (OpenAI ada-002)
- Similarity: cosine
- **Note:** Recreate index if switching embedding providers!

---

## Known Issues / Gotchas

1. **Port 8080/8081 in use** - Docker has other services on these ports. Use port 8082.

2. **Vector dimension mismatch** - If switching from OpenAI (1536-dim) to Ollama (768-dim) or vice versa:
   ```bash
   # Drop and recreate index with correct dimensions
   docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword \
     'DROP INDEX memNodeEmbedding IF EXISTS'
   docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword \
     'CREATE VECTOR INDEX memNodeEmbedding FOR (n:MemoryNode) ON (n.embedding) OPTIONS {indexConfig:{`vector.dimensions`: 768, `vector.similarity_function`: "cosine"}}'
   ```

3. **Ollama model required** - For local testing, ensure `nomic-embed-text` is installed:
   ```bash
   ollama pull nomic-embed-text
   ```

4. **Backup files** - `.bak` files in `internal/api/` and `internal/retrieval/` contain alternate implementations. May have useful code to reference but caused build conflicts.

5. **Go module path** - Module is `mdemg` (not `mdemg/service`). Imports should be `mdemg/internal/...`

---

## Next Development Tasks

### Task 7: Batch Ingest Endpoint

**Endpoint:** `POST /v1/memory/ingest/batch`

**Implementation:**
- Add `BatchIngestRequest/Response` to `internal/models/models.go`
- Add `handleBatchIngest` to `internal/api/handlers.go`
- Add `BatchIngestObservations` to `internal/retrieval/service.go`
- Max 100 observations per request, partial success supported

### Task 8: Reflection Endpoint

**Endpoint:** `POST /v1/memory/reflect`

**Implementation:**
- Create `internal/retrieval/reflection.go`
- Add `handleReflect` to `internal/api/handlers.go`
- Multi-hop traversal, abstraction surfacing, insight generation

### Task 9: Anomaly Detection Service ✅ COMPLETE

**Implementation:**
- `internal/anomaly/types.go` - Config, DetectionContext, Service types
- `internal/anomaly/service.go` - Duplicate and stale update detection
- `internal/anomaly/service_test.go` - Unit tests (10 tests)
- Integration test: `TestIngestAnomalyDuplicateDetection`

### Observing Emergent Behaviors

Monitor the graph as it learns:
```cypher
// See CO_ACTIVATED_WITH edges forming
MATCH ()-[r:CO_ACTIVATED_WITH]->()
RETURN count(r), avg(r.weight)

// Find strongly connected memory pairs
MATCH (a)-[r:CO_ACTIVATED_WITH]->(b)
WHERE r.weight > 0.5
RETURN a.name, b.name, r.weight
ORDER BY r.weight DESC LIMIT 20

// See clusters emerging
MATCH path = (a:MemoryNode)-[:CO_ACTIVATED_WITH*2..3]-(b:MemoryNode)
WHERE a <> b
RETURN a.name, b.name, length(path)

// Check semantic edges from ingest
MATCH ()-[r:ASSOCIATED_WITH]->()
RETURN count(r), avg(r.weight), avg(r.dim_semantic)
```

### MDEMG Vision Summary

MDEMG is a **cognitive substrate** for AI coding agents where higher-level concepts **emerge automatically** through Hebbian learning. Key design principles:

| Principle | Description |
|-----------|-------------|
| **Dynamic Layers** | Layers grow without limit (hardware-bound only); abstractions emerge as data accumulates |
| **Edge Stability** | Relationships persist while node organization is fluid |
| **Active Participation** | Not just passive storage; proactive surfacing, anomaly detection, agent consulting |
| **Combination Signals** | Promotion based on frequency + clustering + edge strength + temporal stability + cross-domain relevance |

**Expected Emergent Behaviors:**
1. **Concept clustering** - Similar patterns grouping together via `CO_ACTIVATED_WITH` edges
2. **Cross-project transfer** - Knowledge from one project surfaces when relevant to another
3. **Workflow recognition** - System learns recurring patterns in how you work
4. **Abstraction emergence** - Higher-layer nodes crystallize from specific examples

**Integration Modes:**
- Background service (always running)
- Event-driven hooks (git commits, file saves)
- Proactive surfacing (context suggestions, anomaly detection)
- Agent consulting service (SME-like guidance)

See [VISION.md](./VISION.md) for the complete architectural philosophy.
