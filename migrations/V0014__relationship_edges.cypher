// Phase 75A: Indexes for parser-derived relationship edges between SymbolNodes
// These edges are created during codebase ingestion by the query engine

CREATE INDEX imports_source_idx IF NOT EXISTS
FOR ()-[r:IMPORTS]-() ON (r.space_id);

CREATE INDEX calls_source_idx IF NOT EXISTS
FOR ()-[r:CALLS]-() ON (r.space_id);

CREATE INDEX extends_source_idx IF NOT EXISTS
FOR ()-[r:EXTENDS]-() ON (r.space_id);

CREATE INDEX implements_source_idx IF NOT EXISTS
FOR ()-[r:IMPLEMENTS]-() ON (r.space_id);
