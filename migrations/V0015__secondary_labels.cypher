// Phase 75B: Apply secondary labels to existing MemoryNodes for faster filtering
// Secondary labels coexist with the primary :MemoryNode label

// Apply secondary labels based on role_type
CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'hidden' AND NOT n:HiddenPattern
  SET n:HiddenPattern
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'concept' AND NOT n:Concept
  SET n:Concept
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'concern' AND NOT n:Concern
  SET n:Concern
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'config' AND NOT n:ConfigPattern
  SET n:ConfigPattern
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'comparison' AND NOT n:Comparison
  SET n:Comparison
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'temporal' AND NOT n:TemporalPattern
  SET n:TemporalPattern
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'constraint' AND NOT n:Constraint
  SET n:Constraint
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'conversation_observation' AND NOT n:ConversationObs
  SET n:ConversationObs
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'conversation_theme' AND NOT n:ConversationTheme
  SET n:ConversationTheme
} IN TRANSACTIONS OF 1000 ROWS;

CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'emergent_concept' AND NOT n:EmergentConcept
  SET n:EmergentConcept
} IN TRANSACTIONS OF 1000 ROWS;

// Create btree indexes on secondary labels for space_id lookups
CREATE INDEX hidden_pattern_space_idx IF NOT EXISTS FOR (n:HiddenPattern) ON (n.space_id);
CREATE INDEX concept_space_idx IF NOT EXISTS FOR (n:Concept) ON (n.space_id);
CREATE INDEX concern_space_idx IF NOT EXISTS FOR (n:Concern) ON (n.space_id);
CREATE INDEX config_pattern_space_idx IF NOT EXISTS FOR (n:ConfigPattern) ON (n.space_id);
CREATE INDEX comparison_space_idx IF NOT EXISTS FOR (n:Comparison) ON (n.space_id);
CREATE INDEX temporal_pattern_space_idx IF NOT EXISTS FOR (n:TemporalPattern) ON (n.space_id);
CREATE INDEX constraint_space_idx IF NOT EXISTS FOR (n:Constraint) ON (n.space_id);
CREATE INDEX conv_obs_space_idx IF NOT EXISTS FOR (n:ConversationObs) ON (n.space_id);
CREATE INDEX conv_theme_space_idx IF NOT EXISTS FOR (n:ConversationTheme) ON (n.space_id);
CREATE INDEX emergent_concept_space_idx IF NOT EXISTS FOR (n:EmergentConcept) ON (n.space_id);
