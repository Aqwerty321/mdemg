// V0002 — Core constraints and BTREE indexes

// TapRoot is singleton per space
CREATE CONSTRAINT taproot_space_unique IF NOT EXISTS
FOR (t:TapRoot) REQUIRE t.space_id IS UNIQUE;

// MemoryNode uniqueness
CREATE CONSTRAINT memorynode_path_unique IF NOT EXISTS
FOR (n:MemoryNode) REQUIRE (n.space_id, n.path) IS UNIQUE;

CREATE CONSTRAINT memorynode_id_unique IF NOT EXISTS
FOR (n:MemoryNode) REQUIRE (n.space_id, n.node_id) IS UNIQUE;

// Observation uniqueness
CREATE CONSTRAINT observation_id_unique IF NOT EXISTS
FOR (o:Observation) REQUIRE (o.space_id, o.obs_id) IS UNIQUE;

// Helpful query indexes (BTREE)
CREATE INDEX memorynode_name_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.name);

CREATE INDEX memorynode_layer_role_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.layer, n.role_type);

CREATE INDEX observation_time_idx IF NOT EXISTS
FOR (o:Observation) ON (o.space_id, o.timestamp);

// Record migration
MERGE (m:Migration {version: 2})
ON CREATE SET m.name='V0002__constraints_and_btree_indexes',
              m.applied_at=datetime(),
              m.checksum=null;

MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version,0) < 2
SET s.current_version = 2,
    s.updated_at = datetime();
