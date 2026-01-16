# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **New here?** Read `HANDOFF.md` for current development status, TODO list, and context from the previous session.

## Project Overview

MDEMG (Multi-Dimensional Emergent Memory Graph) is a long-term memory system built on Neo4j with native vector indexes. It implements a retrieval-augmented memory graph with spreading activation and Hebbian learning.

### Core Invariants (Do Not Violate)
- **Vector index = recall** (fast candidate generation)
- **Graph = reasoning** (typed edges with evidence)
- **Runtime = activation physics** (spreading activation computed in-memory)
- **DB writes = learning deltas only** (no per-request activation writes)

## Commands

### Start Neo4j (local dev)
```bash
docker compose up -d
# Browser: http://localhost:7474 (neo4j/testpassword)
```

### Apply Cypher Migrations
```bash
for f in mdemg_build/migrations/V*.cypher; do
  echo "Applying $f"
  docker exec -i mdemg-neo4j cypher-shell -u neo4j -p testpassword < "$f"
done
```

### Run the Go Service
```bash
cd mdemg_build/service
export NEO4J_URI=bolt://localhost:7687
export NEO4J_USER=neo4j
export NEO4J_PASS=testpassword
export REQUIRED_SCHEMA_VERSION=4
export VECTOR_INDEX_NAME=memNodeEmbedding
go mod tidy
go run ./cmd/server
```

### Test Retrieval
```bash
curl -s localhost:8080/v1/memory/retrieve \
  -H 'content-type: application/json' \
  -d '{"space_id":"demo","query_embedding":[0.0,0.1,0.2],"candidate_k":50,"top_k":10,"hop_depth":2}' | jq
```

## Architecture

### Directory Structure
- `mdemg_build/` — Build-ready implementation
  - `docs/` — Technical documentation (00-14 numbered docs)
  - `migrations/` — Idempotent Cypher migrations (V0001-V0004)
  - `service/` — Go service implementation
- `mdemg_docs/` — Reference documentation
- `mdemg_neo4j_docs/` — Neo4j-specific docs

### Go Service Structure (`mdemg_build/service/`)
```
cmd/server/main.go      — Entry point, schema version check
internal/
  api/                  — HTTP handlers (healthz, readyz, retrieve, ingest)
  config/               — Environment-based configuration
  db/                   — Neo4j driver + schema validation
  models/               — Request/response types
  retrieval/            — Core retrieval pipeline
    service.go          — Vector recall + bounded expansion
    activation.go       — Spreading activation computation
    scoring.go          — Final ranking logic
  learning/             — CO_ACTIVATED_WITH edge writeback
```

### Retrieval Pipeline (service.go:Retrieve)
1. **Vector recall** — Query `memNodeEmbedding` index for top-K candidates
2. **Bounded expansion** — Iterative 1-hop fetch with caps (max depth=3, per-node limit)
3. **Spreading activation** — In-memory computation with decay
4. **Scoring + ranking** — Combine vector similarity, activation, recency, hub penalty

### Graph Schema (Core Labels)
- `:TapRoot` — Singleton per space_id
- `:MemoryNode` — Main memory nodes with embeddings (1536-dim default)
- `:Observation` — Append-only events linked to MemoryNodes
- `:SchemaMeta` — Schema version tracking

### Key Relationship Types
- `ASSOCIATED_WITH`, `CO_ACTIVATED_WITH` — Associative
- `CAUSES`, `ENABLES` — Causal
- `TEMPORALLY_ADJACENT` — Temporal
- `ABSTRACTS_TO`, `INSTANTIATES` — Layer hierarchy
- `HAS_OBSERVATION` — Node to observation link

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEO4J_URI` | (required) | Bolt connection URI |
| `NEO4J_USER` | (required) | Neo4j username |
| `NEO4J_PASS` | (required) | Neo4j password |
| `REQUIRED_SCHEMA_VERSION` | (required) | Must match latest migration |
| `VECTOR_INDEX_NAME` | `memNodeEmbedding` | Vector index name |
| `DEFAULT_CANDIDATE_K` | 200 | Vector recall candidates |
| `DEFAULT_TOP_K` | 20 | Final results returned |
| `DEFAULT_HOP_DEPTH` | 2 | Graph expansion depth |
| `MAX_NEIGHBORS_PER_NODE` | 50 | Per-hop degree cap |
| `LEARNING_EDGE_CAP_PER_REQUEST` | 200 | Max edges written per request |

## Key Design Notes

- **Embeddings are external**: The service expects `query_embedding` (retrieve) and `embedding` (ingest) to be provided; it does not generate embeddings.
- **APOC optional**: `internal/retrieval/retrieval.go` uses `apoc.math.clamp`; replace with manual Cypher if APOC unavailable.
- **Vector index dimensions**: Default is 1536 (OpenAI ada-002); modify `V0003__vector_indexes.cypher` for other models.
- **Activation is transient**: Never persist per-query activation values; only write learning deltas (CO_ACTIVATED_WITH edges).
