// V0005 — Hidden Layer Support
// Adds schema elements for hierarchical graph convolution with hidden layers

// =============================================================================
// NEW INDEXES FOR HIDDEN LAYER QUERIES
// =============================================================================

// Index for finding nodes by layer for message passing
CREATE INDEX memorynode_layer_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.layer);

// Index for finding nodes needing forward/backward pass updates
CREATE INDEX memorynode_last_forward_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.last_forward_pass);

CREATE INDEX memorynode_last_backward_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.last_backward_pass);

// Index for stability monitoring
CREATE INDEX memorynode_stability_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.stability_score);

// =============================================================================
// RELATIONSHIP TYPE DOCUMENTATION
// =============================================================================
//
// New relationship types introduced in V0005:
//
// GENERALIZES: (base:MemoryNode {layer:0})-[:GENERALIZES]->(hidden:MemoryNode {layer:1})
//   - Links base data nodes to their hidden layer generalization
//   - Properties: space_id, edge_id, weight (0-1), created_at, updated_at
//   - Weight indicates how well the base node fits the hidden pattern
//
// Note: AGGREGATES relationship (hidden -> concept) uses existing ABSTRACTS_TO
// for compatibility with current consolidation code. Future versions may
// introduce a distinct AGGREGATES type.
//
// =============================================================================

// =============================================================================
// PROPERTY DOCUMENTATION
// =============================================================================
//
// New properties on :MemoryNode for hidden layer support:
//
// message_pass_embedding: [float64]
//   - Result of last message passing update
//   - May differ from 'embedding' which is the original/stable embedding
//   - Used during retrieval for layer-aware scoring
//
// last_forward_pass: datetime
//   - Timestamp of last forward pass update
//   - Used to identify stale nodes needing refresh
//
// last_backward_pass: datetime
//   - Timestamp of last backward pass update
//   - Used to identify nodes needing feedback propagation
//
// aggregation_count: int
//   - Number of child nodes aggregated into this node
//   - For hidden nodes: count of base data nodes
//   - For concept nodes: count of hidden nodes
//
// stability_score: float64 (0-1)
//   - Measures how stable this node's embedding is over time
//   - Low scores indicate nodes that may need review/restructuring
//   - Computed as 1 - (embedding_drift / max_drift)
//
// =============================================================================

// =============================================================================
// RECORD MIGRATION
// =============================================================================

MERGE (m:Migration {version: 5})
ON CREATE SET m.name='V0005__hidden_layer_support',
              m.applied_at=datetime(),
              m.checksum=null;

MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version,0) < 5
SET s.current_version = 5,
    s.updated_at = datetime();
