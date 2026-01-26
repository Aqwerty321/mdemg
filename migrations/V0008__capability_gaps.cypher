// V0008: Capability Gap Detection Support
// Task #23: Self-improvement capability gap detection

// Create constraint for CapabilityGap node uniqueness
CREATE CONSTRAINT capability_gap_id IF NOT EXISTS
FOR (g:CapabilityGap) REQUIRE g.gap_id IS UNIQUE;

// Create index on status for filtering
CREATE INDEX capability_gap_status IF NOT EXISTS
FOR (g:CapabilityGap) ON (g.status);

// Create index on type for filtering
CREATE INDEX capability_gap_type IF NOT EXISTS
FOR (g:CapabilityGap) ON (g.type);

// Create index on priority for sorting
CREATE INDEX capability_gap_priority IF NOT EXISTS
FOR (g:CapabilityGap) ON (g.priority);

// Create composite index for common queries (status + priority)
CREATE INDEX capability_gap_status_priority IF NOT EXISTS
FOR (g:CapabilityGap) ON (g.status, g.priority);

// Update schema version
MATCH (s:SchemaMeta {key: 'version'})
SET s.value = 8;
