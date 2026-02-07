// V0012: Observation Templates and CMS Advanced II Schema
// Phase 60: CMS Advanced Functionality II

// =============================================================================
// Observation Templates
// Templates are stored in sub-space: {space_id}:templates
// =============================================================================

// Create ObservationTemplate node constraint
CREATE CONSTRAINT obs_template_unique IF NOT EXISTS
FOR (t:ObservationTemplate)
REQUIRE (t.space_id, t.template_id) IS UNIQUE;

// Index for template lookup
CREATE INDEX obs_template_space_idx IF NOT EXISTS
FOR (t:ObservationTemplate) ON (t.space_id);

// =============================================================================
// Extended Observation Properties
// These are added to existing Observation nodes
// =============================================================================

// Index for template-based observations
CREATE INDEX obs_template_id_idx IF NOT EXISTS
FOR (o:Observation) ON (o.template_id);

// Index for org review status
CREATE INDEX obs_org_review_idx IF NOT EXISTS
FOR (o:Observation) ON (o.org_review_status);

// Index for importance score (for relevance-based resume)
CREATE INDEX obs_importance_idx IF NOT EXISTS
FOR (o:Observation) ON (o.importance_score);

// Index for tier (for tiered resume)
CREATE INDEX obs_tier_idx IF NOT EXISTS
FOR (o:Observation) ON (o.tier);

// =============================================================================
// Task Snapshots
// =============================================================================

// Create TaskSnapshot node constraint
CREATE CONSTRAINT task_snapshot_unique IF NOT EXISTS
FOR (s:TaskSnapshot)
REQUIRE s.snapshot_id IS UNIQUE;

// Index for snapshot lookup by session
CREATE INDEX task_snapshot_session_idx IF NOT EXISTS
FOR (s:TaskSnapshot) ON (s.space_id, s.session_id);

// Index for snapshot lookup by trigger type
CREATE INDEX task_snapshot_trigger_idx IF NOT EXISTS
FOR (s:TaskSnapshot) ON (s.trigger);

// =============================================================================
// Observation Relationships
// =============================================================================

// Note: Relationship types are created on first use in Neo4j
// These are the new relationship types for Phase 60:
// - SUPERSEDES: Observation supersedes another (newer version)
// - RELATES_TO: Observations are related
// - CONTRADICTS: Observations contradict each other
// - FOLLOWS_FROM: Observation follows from another
// - SUMMARIZES: Summary observation consolidates others

// =============================================================================
// Default Templates (inserted via application code, not Cypher)
// Templates will be created in {space_id}:templates sub-space
// See internal/conversation/templates.go for default template definitions
// =============================================================================

// Update schema version
MATCH (m:SchemaMeta)
SET m.version = 12, m.updated_at = datetime()
RETURN m.version AS new_version;
