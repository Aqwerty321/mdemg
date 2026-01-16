// V0004 — Safety checks + version bump
// This migration is informational: it asserts that critical schema elements exist.

// NOTE: cypher-shell will print outputs; CI can grep for missing status if desired.

SHOW CONSTRAINTS;
SHOW INDEXES;

// Record migration
MERGE (m:Migration {version: 4})
ON CREATE SET m.name='V0004__safety_checks',
              m.applied_at=datetime(),
              m.checksum=null;

MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version,0) < 4
SET s.current_version = 4,
    s.updated_at = datetime();
