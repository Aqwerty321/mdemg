# Graph Schema (Labels, Relationship Types, Properties)

This spec is optimized for Neo4j property graphs.

## Namespaces
All nodes include:
- `space_id` (string): tenant/agent namespace
- `path` (string): unique addressing within `space_id` (recommended unique constraint)
- `layer` (int): L0 (concrete) → Ln (abstract)
- `role_type` (string enum): `root|trunk|branch|shoot|leaf|stem|...`
- `status` (string enum): `active|deprecated|merged|tombstoned`
- timestamps + versioning

## Core labels
### `:TapRoot`
Singleton per space.
Properties:
- `space_id` (unique)
- `name`
- `created_at`

### `:MemoryNode`
Main concept/memory node.
Properties (minimum):
- `node_id` (ULID/UUID string)
- `space_id`
- `name`
- `path`
- `layer`
- `role_type`
- `version`
- `created_at`
- `updated_at`
- `update_count`
- `description`
- `summary`
- `confidence` (0..1)
- `sensitivity` (`public|internal|restricted`)
- `tags` (list of strings)
- `embedding` (vector list or VECTOR type depending on Neo4j version)

### `:Observation`
Append-only event.
Properties:
- `obs_id`
- `space_id`
- `timestamp`
- `source`
- `content` (string or JSON-serialized string)
- `embedding` (optional; if you embed chunks/events)
- `math_block` (optional JSON string)
- `created_at`

### debug labels
- `:ActivationSnapshot` (store per-query activation traces for debugging)

## Relationship types (minimum set)
All relationships include:
- `edge_id`
- `space_id`
- `created_at`, `updated_at`
- `version`
- `status`
- `weight` (float)
- `dimensions` (map-like; in Neo4j store as properties e.g. `dim_semantic`, `dim_temporal`, ...)
- `evidence_count` (int)
- `last_activated_at` (datetime)
- `decay_rate` (float)

### Structural
- `(:TapRoot)-[:HAS_LAYER]->(:Layer)` *(optional)*
- `(:MemoryNode)-[:CONTAINS]->(:MemoryNode)`
- `(:MemoryNode)-[:PART_OF]->(:MemoryNode)` (inverse of CONTAINS)
- `(:MemoryNode)-[:ABSTRACTS_TO]->(:MemoryNode)` must be layer k → k+1
- `(:MemoryNode)-[:INSTANTIATES]->(:MemoryNode)` inverse of ABSTRACTS_TO

### Associative / Dynamics
- `(:MemoryNode)-[:ASSOCIATED_WITH]->(:MemoryNode)`
- `(:MemoryNode)-[:CAUSES]->(:MemoryNode)`
- `(:MemoryNode)-[:ENABLES]->(:MemoryNode)`
- `(:MemoryNode)-[:TEMPORALLY_ADJACENT]->(:MemoryNode)`
- `(:MemoryNode)-[:CO_ACTIVATED_WITH]->(:MemoryNode)`
- `(:MemoryNode)-[:CONTRADICTS]->(:MemoryNode)` (treat as inhibitory)

### Observation links
- `(:MemoryNode)-[:HAS_OBSERVATION]->(:Observation)`
- `(:Observation)-[:REFERS_TO]->(:MemoryNode)` (optional, if one obs links multiple nodes)

## Constraints and indexes (Cypher examples)

### Uniqueness
```cypher
CREATE CONSTRAINT space_taproot_unique IF NOT EXISTS
FOR (t:TapRoot) REQUIRE t.space_id IS UNIQUE;

CREATE CONSTRAINT memorynode_path_unique IF NOT EXISTS
FOR (n:MemoryNode) REQUIRE (n.space_id, n.path) IS UNIQUE;

CREATE CONSTRAINT memorynode_id_unique IF NOT EXISTS
FOR (n:MemoryNode) REQUIRE (n.space_id, n.node_id) IS UNIQUE;

CREATE CONSTRAINT observation_id_unique IF NOT EXISTS
FOR (o:Observation) REQUIRE (o.space_id, o.obs_id) IS UNIQUE;
```

### Helpful search indexes
```cypher
CREATE INDEX memorynode_name IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.name);

CREATE INDEX memorynode_layer_role IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.layer, n.role_type);
```

## Property conventions
Neo4j relationships cannot store nested maps as flexibly as documents—prefer **flat dimension properties**:
- `dim_semantic`
- `dim_temporal`
- `dim_causal`
- `dim_coactivation`
- `dim_contains`
- `dim_contradiction` (or represent as separate edge type)

Compute effective traversal weight at query time:
- `w_eff = weight * (α*dim_semantic + β*dim_temporal + ...) * recency_factor`
