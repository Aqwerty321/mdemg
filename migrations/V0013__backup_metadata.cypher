// BackupMeta tracking (Phase 70)
CREATE CONSTRAINT backup_meta_id IF NOT EXISTS
FOR (b:BackupMeta) REQUIRE b.backup_id IS UNIQUE;

CREATE INDEX backup_meta_started IF NOT EXISTS
FOR (b:BackupMeta) ON (b.started_at);
