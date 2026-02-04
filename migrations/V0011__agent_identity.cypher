// V0011: Agent Identity for Multi-Agent CMS
// Adds agent_id support for persistent agent identity across sessions.
// Agents are long-lived identities; sessions are ephemeral.

// Index for filtering observations by agent_id
CREATE INDEX memorynode_agent_id_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.agent_id);

// Composite index for agent + visibility (common query pattern for multi-agent resume)
CREATE INDEX memorynode_agent_visibility_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.agent_id, n.visibility);

// Composite index for agent + session (cross-session lookups per agent)
CREATE INDEX memorynode_agent_session_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.agent_id, n.session_id);

// Record migration
MERGE (m:Migration {version: 11})
ON CREATE SET m.name='V0011__agent_identity',
              m.applied_at=datetime(),
              m.checksum=null;

// Update schema version
MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version, 0) < 11
SET s.current_version = 11,
    s.updated_at = datetime();
