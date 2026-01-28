// V0009: Identity & Visibility Layer for CMS
// Adds support for multi-tenant collaboration with private/team/global visibility

// Index for filtering by user_id (owner lookups)
CREATE INDEX memorynode_user_id_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.user_id);

// Index for filtering by visibility level
CREATE INDEX memorynode_visibility_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.visibility);

// Index for volatile nodes (Context Cooler graduation queries)
CREATE INDEX memorynode_volatile_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.volatile);

// Composite index for common query pattern: volatile nodes by user
CREATE INDEX memorynode_user_volatile_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.user_id, n.volatile);

// Composite index for visibility filtering in retrieval
CREATE INDEX memorynode_visibility_layer_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.visibility, n.layer);
