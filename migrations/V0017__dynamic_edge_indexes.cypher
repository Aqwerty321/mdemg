// Phase 75B: Indexes for dynamic edge types used by hidden layer inference
// These edges are created during consolidation by InferEdgeType()

CREATE INDEX analogous_to_space_idx IF NOT EXISTS
FOR ()-[r:ANALOGOUS_TO]-() ON (r.space_id);

CREATE INDEX contrasts_with_space_idx IF NOT EXISTS
FOR ()-[r:CONTRASTS_WITH]-() ON (r.space_id);

CREATE INDEX composes_with_space_idx IF NOT EXISTS
FOR ()-[r:COMPOSES_WITH]-() ON (r.space_id);

CREATE INDEX tensions_with_space_idx IF NOT EXISTS
FOR ()-[r:TENSIONS_WITH]-() ON (r.space_id);

CREATE INDEX influences_space_idx IF NOT EXISTS
FOR ()-[r:INFLUENCES]-() ON (r.space_id);

CREATE INDEX specializes_space_idx IF NOT EXISTS
FOR ()-[r:SPECIALIZES]-() ON (r.space_id);

CREATE INDEX generalizes_to_space_idx IF NOT EXISTS
FOR ()-[r:GENERALIZES_TO]-() ON (r.space_id);

CREATE INDEX emerges_from_space_idx IF NOT EXISTS
FOR ()-[r:EMERGES_FROM]-() ON (r.space_id);

CREATE INDEX bridges_space_idx IF NOT EXISTS
FOR ()-[r:BRIDGES]-() ON (r.space_id);

CREATE INDEX unifies_space_idx IF NOT EXISTS
FOR ()-[r:UNIFIES]-() ON (r.space_id);

// Index THEME_OF (created by V0016 migration)
CREATE INDEX theme_of_space_idx IF NOT EXISTS
FOR ()-[r:THEME_OF]-() ON (r.space_id);

// Index DEFINES_SYMBOL (already exists but needs space_id index)
CREATE INDEX defines_symbol_space_idx IF NOT EXISTS
FOR ()-[r:DEFINES_SYMBOL]-() ON (r.space_id);
