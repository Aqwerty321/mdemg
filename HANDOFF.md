# MDEMG Development Handoff Document

**Date:** 2026-01-16
**Status:** MCP Integration Complete - Ready for Agent Use

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

### What's Next (Priority Order)
1. **Learning Loop** - Implement `ApplyCoactivation()` in `internal/learning/service.go`
2. **Semantic Edges on Ingest** - Auto-create `ASSOCIATED_WITH` edges to similar nodes
3. **Use the system!** - The more memories stored, the more emergent behaviors appear

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

### 🆕 New in This Session (2026-01-15)

1. **MCP Server Integration** (`mdemg_build/mcp-server/`)
   - Go-based MCP server for Cursor/Claude agent integration
   - Tools: `memory_store`, `memory_recall`, `memory_associate`, `memory_reflect`, `memory_status`
   - Configured in `~/.cursor/mcp.json`
   - Connects to MDEMG HTTP API on localhost:8082

2. **Embedding Package** (`internal/embeddings/`)
   - Interface-based design supporting multiple providers
   - OpenAI implementation (1536-dim, `text-embedding-ada-002`)
   - Ollama implementation (768-dim, `nomic-embed-text`)
   - Auto-initialization based on `EMBEDDING_PROVIDER` env var

3. **API Enhancements**
   - `/v1/memory/retrieve` now accepts `query_text` and auto-generates embeddings
   - `/v1/memory/ingest` auto-generates embeddings from content
   - `/readyz` reports embedding provider info

4. **IDE Setup**
   - `.vscode/extensions.json` - Recommended extensions
   - `.vscode/launch.json` - Debug configurations
   - `.vscode/tasks.json` - Common tasks
   - `mdemg_build/service/api-tests.http` - REST Client test file
   - `start-mdemg.sh` - One-command startup script

5. **Bug Fixes**
   - Fixed Cypher syntax error in `IngestObservation` (duplicate variable declaration)

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

### Priority 1: Learning Loop (Next)
- [ ] **Complete `internal/learning/service.go`**
  - Implement `ApplyCoactivation()` to write `CO_ACTIVATED_WITH` edges
  - Cap writes per request (default 200 edges max)
  - Never write activation values - only edge weights

- [ ] **Add decay job (offline)**
  - Exponentially decay edge weights over time
  - Prune edges below threshold
  - Can be a CLI command or separate cron job

### Priority 2: Enhanced Ingestion
- [ ] **Add semantic edge creation on ingest**
  - Find nearest neighbors by vector similarity
  - Create `ASSOCIATED_WITH` edges to top-N similar nodes

### Priority 3: Testing & Validation
- [ ] **Create golden test for scoring**
  - Ingest known nodes with known embeddings
  - Retrieve and verify scores match expected values
  - See `docs/12_Retrieval_Scoring_Worked_Examples.md`

- [ ] **Add integration tests**
  - Test against live Neo4j container
  - Verify schema version checks work
  - Test edge case: empty graph retrieval

### Priority 4: Consolidation & Pruning (Later)
- [ ] **Implement abstraction promotion**
  - Detect stable clusters at layer k
  - Create abstraction node at layer k+1
  - Add `ABSTRACTS_TO` edges

- [ ] **Implement graph pruning**
  - Remove weak/deprecated edges
  - Merge redundant nodes

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
| `internal/config/config.go` | Environment variable parsing |
| `internal/db/neo4j.go` | Driver creation, schema validation |
| `internal/api/server.go` | HTTP routes registration |
| `internal/api/handlers.go` | Request handlers |
| `internal/embeddings/embeddings.go` | Embedder interface + factory |
| `internal/embeddings/openai.go` | OpenAI embedding provider |
| `internal/embeddings/ollama.go` | Ollama embedding provider |
| `internal/retrieval/service.go` | Vector recall + expansion |
| `internal/retrieval/activation.go` | Spreading activation |
| `internal/retrieval/scoring.go` | Final ranking |
| `internal/learning/service.go` | CO_ACTIVATED_WITH writeback |
| `internal/models/models.go` | Request/response types |

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

# Embedding Provider (NEW)
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

## Next Development Task: Learning Loop

The retrieval pipeline is complete. The next step is implementing **Hebbian learning** so the graph strengthens connections between co-activated memories.

### Task: Implement `ApplyCoactivation()`

**File:** `mdemg_build/service/internal/learning/service.go`

**What it should do:**
1. After a retrieval returns results, create/strengthen `CO_ACTIVATED_WITH` edges between the returned nodes
2. Edge weight increases when nodes are retrieved together (Hebbian: "neurons that fire together wire together")
3. Cap writes per request (env: `LEARNING_EDGE_CAP_PER_REQUEST=200`)
4. Never write activation values - only edge weights

**Reference docs:**
- `mdemg_build/docs/04_Activation_and_Learning.md` - Learning algorithm
- `mdemg_build/docs/02_Graph_Schema.md` - Edge properties

**Current state:** The function is called in `handlers.go:handleRetrieve` but the implementation is a stub.

### Observing Emergent Behaviors

Once learning is implemented, monitor the graph:
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
