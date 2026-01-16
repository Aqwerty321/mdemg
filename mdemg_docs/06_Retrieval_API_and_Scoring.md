# Retrieval API + Scoring + Explainability

## 1) API contract framework
`POST /v1/memory/retrieve`
Body:
- `space_id`
- `query_text` OR `query_node_id` OR `query_path`
- `top_k` (default 20)
- `candidate_k` (default 200)
- `hop_depth` (default 2)
- `layer_range` e.g. [0,3])
- `policy_context` (roles, sensitivity limits)

Response:
- `results[]`: { node_id, path, name, summary, score }
- `explanations[]`: subgraph edges/nodes that justify each result
- `debug` : activation stats, thresholds hit

## 2) Candidate generation (vector + optional keyword)
### Vector recall
1) embed query_text
2) vector index query on `:MemoryNode.embedding`

```cypher
WITH $queryEmbedding AS q
CALL db.index.vector.queryNodes('memNodeEmbedding', $candidateK, q)
YIELD node, score
WHERE node.space_id = $spaceId
RETURN node.node_id AS node_id, score
ORDER BY score DESC;
```

## 3) Graph expansion
For each candidate, fetch neighborhood edges within allowed types and within hop_depth.

Highly Recommended:
- restrict to edge types that represent useful association (avoid CONTAINS exploding unless filtered)
- apply degree caps (e.g., ignore nodes with out-degree > N unless they are structural anchors)

## 4) Activation pass
Compute activation over fetched subgraph in-memory (see `04_Activation_and_Learning.md`).

## 5) Final ranking
Example scoring:
- `score = α*vector_sim + β*activation + γ*recency + δ*confidence - κ*redundancy - φ*hub_penalty`

Where:
- `hub_penalty` can be proportional to log(degree) to avoid generic nodes dominating
- `redundancy` penalizes near-duplicates (same abstraction parent, same path prefix, etc.)

## 6) Explainability (must-have)
Return:
- top contributing paths: (seed → ... → result) with edge weights and types
- evidence counts and timestamps for edges used

A simple explainability payload per node:
- `vector_sim`
- `activation`
- `top_incoming_contributors[]`: list of (src_node_id, relType, w_eff, src_activation)

## 7) Learning updates after retrieval
For final top-K nodes:
- update/create `CO_ACTIVATED_WITH` edges among nodes above activation threshold
- increment evidence counts, update timestamps
- optional: strengthen `ABSTRACTS_TO` edges if consistent

Keep learning updates small and bounded per request.
