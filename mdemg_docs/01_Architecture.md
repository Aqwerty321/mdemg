# Architecture (Neo4j + Vector Indexes)

## High-level components
1. **Neo4j**: property graph store for nodes/edges + weights + provenance.
2. **Neo4j Vector Index**: fast nearest-neighbor search over embeddings.
3. **Embedding generation**:
   - Option A: Neo4j **GenAI plugin** (`genai.vector.encode`, `encodeBatch`) to generate/store embeddings directly.
   - Option B: external embedding service (Python/Go) writes vectors to Neo4j.
4. **Memory Service (API)** (Go recommended for latency):
   - ingestion endpoint
   - retrieval endpoint
   - maintenance endpoints (decay, consolidation)
5. **Offline Jobs** (Python optional):
   - summarization refresh
   - consolidation clustering
   - health checks / anomaly detection

## Online flow (retrieve)
1. Query comes in (text + policy context).
2. Create query embedding (GenAI plugin or external).
3. Vector search: top N candidates via `db.index.vector.queryNodes`.
4. Graph expansion: 1–D hops from candidates using typed edges.
5. Activation pass: compute transient activation scores.
6. Rank and return:
   - top K nodes
   - explanation graph (why each node was selected)
7. Learning updates:
   - strengthen `CO_ACTIVATED_WITH` edges among selected nodes
   - bump evidence counts / timestamps

## Online flow (ingest)
1. Observation arrives with timestamp/source/content/tags.
2. Resolve target node(s) or create new.
3. Append observation event (immutable log).
4. Update node summary (small rolling summarization).
5. Generate/store embedding for node (or observation chunk).
6. Create/adjust edges:
   - semantic links (nearest neighbors)
   - temporal adjacency (recent event chain)
   - containment/path links
7. Optionally enqueue for consolidation evaluation.

## Offline flow (maintenance)
- **Decay job**: exponentially decay weights + prune weak edges.
- **Consolidation job**:
  - detect stable clusters at layer k
  - create abstraction node at layer k+1
  - add `ABSTRACTS_TO` edges
  - compress redundant lateral edges

## Key decision: where activation “lives”
Activation values are **transient**. Do NOT permanently write per-query activation to nodes unless:
- you store debug snapshots in a dedicated label (recommended for debugging only)
- or you want a short-lived cache with TTL semantics (hard in vanilla Neo4j)

Recommended:
- compute activation in the Memory Service runtime,
- write only *learning deltas* back to the graph.
