# MDEMG Build Starter Pack

This folder is the **build-ready** starting point for the Multi-Dimensional Emergent Memory Graph (MDEMG) on **Neo4j + native vector indexes**.

Core strategy (keep this invariant):
- **Vector index = recall** (fast candidate generation)
- **Graph = reasoning** (typed edges with evidence)
- **Runtime = activation physics** (spreading activation computed in-memory)
- **DB writes = only learning deltas** (bounded, regularized)

## What’s inside
- `docs/` — build docs you can hand to engineers and actually implement
- `migrations/` — idempotent Cypher migrations + schema versioning
- `service/` — Go service skeleton: ingest + retrieve + bounded expansion + learning writeback

## Quick start (local)
1) Apply migrations (examples):
```bash
export NEO4J_URI=bolt://localhost:7687
export NEO4J_USER=neo4j
export NEO4J_PASS='yourpass'

# run in order
for f in migrations/V*.cypher; do
  echo "Applying $f";
  cypher-shell -a "$NEO4J_URI" -u "$NEO4J_USER" -p "$NEO4J_PASS" -f "$f";
done
```

2) Run the Go service:
```bash
cd service
export REQUIRED_SCHEMA_VERSION=4
export VECTOR_INDEX_NAME=memNodeEmbedding

go run ./cmd/server
```

3) Retrieve:
```bash
curl -s localhost:8080/v1/memory/retrieve \
  -H 'content-type: application/json' \
  -d '{"space_id":"demo","query_embedding":[0.0,0.1,0.2],"candidate_k":50,"top_k":10,"hop_depth":2}' | jq
```

