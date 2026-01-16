# Activation + Learning Engine

This doc specifies the “physics” that produces emergent association patterns.

## 1) Activation state
Activation values are transient per query.

Define for each node i:
- `a_i ∈ [0,1]` activation
- `b_i` baseline (e.g., recency/importance prior)

Seed activation for candidate nodes:
- from vector similarity score
- from explicit query node(s)

Example seed:
- `a_i(0) = clamp(score_i, 0, 1)`

## 2) Effective edge weight
Each relationship carries:
- base `weight`
- dimension scalars: `dim_semantic`, `dim_temporal`, `dim_causal`, `dim_coactivation`, ...

Define:
- `w_eff(i→j) = weight * (α*dim_semantic + β*dim_temporal + γ*dim_coactivation + ...) * recency_factor`
- `recency_factor = exp(-ρ * Δt)` using `updated_at` or `last_activated_at`

Treat `CONTRADICTS` edges as inhibitory:
- `w_inhib(i→j) = abs(weight) * dim_contradiction`

## 3) Spreading activation (discrete steps)
For T steps:
- `a_j(t+1) = clamp( (1-λ)*a_j(t) + Σ_i a_i(t)*w_eff(i→j) - Σ_k a_k(t)*w_inhib(k→j), 0, 1 )`

Knobs:
- `T` 2–5 is usually enough for retrieval contexts
- `λ` step decay prevents runaway amplification

## 4) Learning: Hebbian update (co-activation)
After retrieval, strengthen co-activated links among top-K nodes.

For nodes i,j in returned context:
- `Δw_ij = η * a_i * a_j - μ * w_ij`
- `w_ij ← clamp(w_ij + Δw_ij, w_min, w_max)`

Notes:
- apply to edge type `CO_ACTIVATED_WITH` (create if missing)
- increment `evidence_count`
- update timestamps
- keep the graph symmetric for CO_ACTIVATED_WITH: store both directions or enforce undirected semantics

## 5) Decay (edge weight over wall-clock time)
Periodically:
- `w_ij ← w_ij * exp(-decay_rate * Δt)`

Prune if:
- `w_ij < prune_threshold` AND `evidence_count` low AND not pinned

## 6) Where to compute activation: use compute in service runtime.
- Step 1: fetch neighborhood edges for candidate nodes (<hop-range> hops)
- Step 2: run activation math in-memory (fast)
- Step 3: write only learning deltas back to Neo4j

Why: writing per-query activation into Neo4j creates write amplification and contention, and it’s not a durable “memory.”

## 7) Neo4j query patterns to support activation computation
Fetch a candidate neighborhood (bounded):
```cypher
MATCH (seed:MemoryNode {space_id:$spaceId})
WHERE seed.node_id IN $seedNodeIds
MATCH (seed)-[r]->(nbr:MemoryNode {space_id:$spaceId})
WHERE type(r) IN $allowedRels
RETURN seed.node_id AS src, nbr.node_id AS dst,
       type(r) AS relType,
       r.weight AS w,
       r.dim_semantic AS dim_semantic,
       r.dim_temporal AS dim_temporal,
       r.dim_coactivation AS dim_coactivation,
       r.updated_at AS updated_at
LIMIT $maxEdges;
```

## 8) Emergence failure modes - how to not die!
- **Hub explosion**: one node connects to everything.
  - fix: degree caps, regularization, prune weak edges, penalize high-degree nodes in ranking
- **Clique spam**: CO_ACTIVATED_WITH grows dense.
  - fix: apply updates only to top-K and require minimum activation threshold, <act-thres>
- **Forgetting everything**: decay too aggressive.
  - fix: lower decay, add pinning, add baseline importance
