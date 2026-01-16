# Ingestion Pipeline (Observation → Nodes/Edges)

## 1) Observation payload
Minimum fields:
- `space_id`
- `timestamp`
- `source`
- `content` (string or JSON)
Optional:
- `tags[]`
- `explicit_node_paths[]` or `explicit_node_ids[]`
- `layer_hint`, `role_hint`
- `sensitivity`, `confidence`

## 2) Node resolution strategy (v1)
In priority order:
1. explicit node reference (path or id)
2. exact path match
3. vector recall against node embeddings (top N)
4. create new node (default L0 leaf)

## 3) Create node (if new)
```cypher
CREATE (n:MemoryNode {
  space_id: $spaceId,
  node_id: $nodeId,
  name: $name,
  path: $path,
  layer: $layer,
  role_type: $roleType,
  version: 1,
  status: 'active',
  created_at: datetime(),
  updated_at: datetime(),
  update_count: 0,
  description: $description,
  summary: $initialSummary,
  confidence: coalesce($confidence, 0.6),
  sensitivity: coalesce($sensitivity, 'internal'),
  tags: coalesce($tags, [])
});
```

## 4) Append observation (immutable)
```cypher
MATCH (n:MemoryNode {space_id:$spaceId, node_id:$nodeId})
CREATE (o:Observation {
  space_id:$spaceId,
  obs_id:$obsId,
  timestamp: datetime($timestamp),
  source:$source,
  content:$content,
  math_block:$mathBlock,
  created_at: datetime()
})
MERGE (n)-[:HAS_OBSERVATION {space_id:$spaceId, created_at:datetime()}]->(o)
SET n.updated_at = datetime(),
    n.update_count = n.update_count + 1;
```

## 5) Update summary (rolling)
You can do this in the service layer (recommended) then write summary back:
```cypher
MATCH (n:MemoryNode {space_id:$spaceId, node_id:$nodeId})
SET n.summary = $newSummary,
    n.updated_at = datetime(),
    n.version = n.version + 1;
```

## 6) Create/update embedding
See `03_Vector_Embeddings_and_Indexes.md`.

## 7) Edge updates (v1 heuristic)
### 7.1 Temporal adjacency (event chain)
Link current node to most recently touched node(s):
```cypher
MATCH (a:MemoryNode {space_id:$spaceId, node_id:$prevNodeId})
MATCH (b:MemoryNode {space_id:$spaceId, node_id:$nodeId})
MERGE (a)-[r:TEMPORALLY_ADJACENT {space_id:$spaceId}]->(b)
ON CREATE SET r.edge_id=$edgeId, r.created_at=datetime(), r.updated_at=datetime(),
              r.weight=0.2, r.dim_temporal=1.0, r.decay_rate=0.01, r.evidence_count=1, r.status='active'
ON MATCH SET r.updated_at=datetime(), r.evidence_count=r.evidence_count+1,
             r.weight = min(1.0, r.weight + 0.02);
```

### 7.2 Semantic Association via Vector Nearest Neighbors

On every ingest, the service automatically creates ASSOCIATED_WITH edges to similar existing nodes.

**Process:**
1. Generate embedding for new/updated node content
2. Query vector index for top-N most similar nodes
3. Filter by minimum similarity threshold
4. Create/update ASSOCIATED_WITH edges with similarity-based weights

**Configuration:**
```bash
SEMANTIC_EDGE_TOP_N=5                # Max edges created per ingest (default 5)
SEMANTIC_EDGE_MIN_SIMILARITY=0.7     # Minimum cosine similarity (default 0.7)
```

**Implementation (service.go):**
```cypher
CALL db.index.vector.queryNodes('memNodeEmbedding', $topN, $nodeEmbedding)
YIELD node AS neighbor, score
WHERE neighbor.space_id = $spaceId
  AND neighbor.node_id <> $nodeId
  AND score >= $minSimilarity
RETURN neighbor.node_id AS neighbor_id, score
ORDER BY score DESC
LIMIT $topN
```

**Edge Creation:**
```cypher
MERGE (a)-[r:ASSOCIATED_WITH {space_id:$spaceId}]->(b)
ON CREATE SET
  r.edge_id = $edgeId,
  r.created_at = datetime(),
  r.updated_at = datetime(),
  r.weight = $similarity,
  r.dim_semantic = 1.0,
  r.dim_temporal = 0.0,
  r.dim_coactivation = 0.0,
  r.evidence_count = 1,
  r.status = 'active'
ON MATCH SET
  r.updated_at = datetime(),
  r.weight = CASE WHEN $similarity > r.weight THEN $similarity ELSE r.weight END,
  r.evidence_count = r.evidence_count + 1
```

**Response includes created edges:**
```json
{
  "node_id": "abc-123",
  "obs_id": "obs-456",
  "created_edges": [
    {"target": "def-789", "type": "ASSOCIATED_WITH", "weight": 0.85},
    {"target": "ghi-012", "type": "ASSOCIATED_WITH", "weight": 0.78}
  ]
}
```

### 7.3 Anomaly Detection During Ingest

Non-blocking anomaly detection runs on each ingest (100ms timeout).

**Detected Anomalies:**
- **Duplicate**: Vector similarity > 0.95 to existing node
- **Stale Update**: Node not modified in 30+ days being updated

**Configuration:**
```bash
ANOMALY_DETECTION_ENABLED=true
ANOMALY_DUPLICATE_THRESHOLD=0.95
ANOMALY_STALE_DAYS=30
ANOMALY_MAX_CHECK_MS=100
```

**Response includes anomalies:**
```json
{
  "node_id": "abc-123",
  "anomalies": [
    {"type": "duplicate", "similar_node_id": "xyz-999", "similarity": 0.97}
  ]
}
```

### 7.4 Containment via path conventions
If your `path` encodes hierarchy, derive CONTAINS/PART_OF edges accordingly.

## 8) Evidence discipline
Whenever you adjust an edge based on an observation, increment:
- `evidence_count`
and optionally store:
- `last_evidence_obs_id`

Keep the graph explainable.
