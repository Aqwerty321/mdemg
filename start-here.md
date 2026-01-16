# MDEMG Long-Term Memory - Build Starter (for a coding agent)

> **Note:** This is the original build starter document. For current development status and quick start, see [HANDOFF.md](./HANDOFF.md).
>
> **Recommended startup:** Use `./start-mdemg.sh` which handles all configuration automatically.

## Axioms (do not violate)
- Vector index = recall (candidate set)
- Graph = reasoning (multi-hop context)
- Runtime = activation physics (spreading activation computed in-memory)
- DB writes = learning deltas only (no per-request activation writes)

## Prereqs
- Docker Desktop (or Docker Engine) running
- Go 1.22+ installed
- Git installed

## 1) Start Neo4j (local dev)
Create `docker-compose.yml` at repo root:

```yaml
services:
  neo4j:
    image: neo4j:5
    container_name: mdemg-neo4j
    ports:
      - "7474:7474"   # Browser
      - "7687:7687"   # Bolt
    environment:
      NEO4J_AUTH: "neo4j/testpassword"
      NEO4J_dbms_memory_pagecache_size: "2G"
      NEO4J_dbms_memory_heap_initial__size: "1G"
      NEO4J_dbms_memory_heap_max__size: "1G"
    volumes:
      - neo4j_data:/data
      - neo4j_logs:/logs
volumes:
  neo4j_data:
  neo4j_logs:
```

Run:
```bash
docker compose up -d
```
Open Neo4j Browser: `http://localhost:7474` (user `neo4j`, password `testpassword`).

## 2) Apply Cypher migrations (schema + vector indexes)
Assumption: migrations live in `migrations/` and are named `V0001__*.cypher`, `V0002__*.cypher`, ...

Install/locate `cypher-shell`:
- easiest: use the one inside the container

Apply all migrations in order:
```bash
for f in migrations/V*.cypher; do
  echo "Applying $f"
  docker exec -i mdemg-neo4j cypher-shell -u neo4j -p testpassword < "$f"
done
```

Sanity checks (run in Browser):
- `SHOW CONSTRAINTS;`
- `SHOW INDEXES;` (confirm vector index exists)
- `MATCH (m:SchemaMeta {key:'schema'}) RETURN m;` (confirm schema version tracking node)

## 3) Configure the Go service
Create `mdemg_build/service/.env` with the following configuration:

```bash
# Neo4j Connection
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASS=testpassword
REQUIRED_SCHEMA_VERSION=4

# Service
LISTEN_ADDR=:8082

# Embedding Provider (choose one)
EMBEDDING_PROVIDER=openai              # or "ollama"

# OpenAI Configuration (if EMBEDDING_PROVIDER=openai)
OPENAI_API_KEY=sk-...                  # Your OpenAI API key
OPENAI_MODEL=text-embedding-ada-002    # 1536 dimensions
OPENAI_ENDPOINT=https://api.openai.com/v1

# Ollama Configuration (if EMBEDDING_PROVIDER=ollama)
# OLLAMA_ENDPOINT=http://localhost:11434
# OLLAMA_MODEL=nomic-embed-text        # 768 dimensions

# Embedding Cache
EMBEDDING_CACHE_ENABLED=true
EMBEDDING_CACHE_SIZE=1000
```

**Important Notes:**
- `REQUIRED_SCHEMA_VERSION` must match the last migration version applied.
- Vector index dimensions must match embedding provider (1536 for OpenAI, 768 for Ollama).
- If switching providers, drop and recreate the vector index with correct dimensions.

## 4) Run the service
From the service module root (where `go.mod` is):

```bash
go mod tidy
GO111MODULE=on go run ./cmd/server
```

Expected endpoints:
- `GET /healthz` : process up
- `GET /readyz` : DB reachable + schema version matches + embedding provider status
- `POST /v1/memory/ingest` : ingest single observation (creates/updates nodes, embeddings)
- `POST /v1/memory/ingest/batch` : bulk ingest up to 100 observations
- `POST /v1/memory/retrieve` : retrieve candidates (vector recall + expansion + scoring)
- `POST /v1/memory/reflect` : deep context exploration (3-stage traversal)
- `GET /v1/metrics` : graph health statistics

## 5) Retrieval pipeline implementation checklist
Implement in this order to get an end-to-end loop:

1. **Vector recall query**
   - Use Neo4j vector index to return top-K `MemoryNode` candidates by cosine similarity.
   - Keep this query fast and shallow.

2. **Bounded expansion query**
   - Expand from recall seeds with hard bounds:
     - max depth (e.g., 2)
     - max neighbors per hop (e.g., 25)
     - allowed relationship types only
   - Return a compact subgraph for scoring (nodes + edges + weights).

3. **Runtime activation physics**
   - Compute activation by spreading along returned edges in-memory.
   - Apply decay per hop and edge weighting.

4. **Final scoring and ranking**
   - Combine: vector similarity + activation + recency + hub penalty.
   - Produce deterministic scores for a worked example (golden test).

5. **Learning delta writeback (only)**
   - Write only bounded deltas, e.g. `CO_ACTIVATED_WITH` between top results.
   - Cap writes per request (e.g., top 10 results => max 45 pair edges).
   - Absolutely do not write per-request activation values.

## 6) Embeddings (keep it swappable)
- The service should accept a `query_embedding` for `/v1/retrieve` and an `embedding` for `/v1/ingest`.
- If `query_text` is provided, embedding generation can be plugged in later via an interface.
- Avoid baking model/provider assumptions into core retrieval logic.

## 7) Minimal smoke test (manual)
1) Ingest one node with a known embedding:
- POST `/v1/ingest` with `id`, `content`, `embedding` (1536-dim if that is your configured dim)

2) Retrieve with the same/similar embedding:
- POST `/v1/retrieve` with `query_embedding`
- Confirm:
  - vector recall returns it
  - bounded expansion does not explode
  - score is stable across runs
  - learning deltas writeback is bounded and visible in the graph

## 8) Common failure modes (debug fast)
- **/readyz fails**: schema version mismatch or missing vector index; re-run migrations.
- **Vector query error**: dims mismatch between stored embeddings and index config.
- **Traversal explosion**: missing bounds; enforce depth and per-hop cap.
- **DB write load too high**: you accidentally write activation traces; remove them.

## Definition of done for "first vertical slice"
- `docker compose up` starts Neo4j
- migrations apply cleanly
- service starts and passes `/readyz`
- you can ingest a memory, retrieve it, and observe bounded learning deltas written

