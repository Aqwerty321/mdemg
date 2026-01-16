// V0001 — Schema metadata + migration log scaffolding
// Idempotent: safe to re-run.

// 1) Uniqueness for schema meta key and migration versions
CREATE CONSTRAINT schema_meta_key_unique IF NOT EXISTS
FOR (s:SchemaMeta) REQUIRE s.key IS UNIQUE;

CREATE CONSTRAINT migration_version_unique IF NOT EXISTS
FOR (m:Migration) REQUIRE m.version IS UNIQUE;

// 2) Ensure SchemaMeta exists
MERGE (s:SchemaMeta {key:'schema'})
ON CREATE SET s.current_version = 0,
              s.created_at = datetime(),
              s.updated_at = datetime();

// 3) Record migration if not already recorded
MERGE (m:Migration {version: 1})
ON CREATE SET m.name='V0001__schema_meta',
              m.applied_at=datetime(),
              m.checksum=null;

// 4) Advance schema version if needed
MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version,0) < 1
SET s.current_version = 1,
    s.updated_at = datetime();
