// V0006: Add fulltext index for hybrid retrieval (BM25 + Vector)
// This enables keyword-based search alongside vector similarity search.
//
// The fulltext index covers:
// - name: Module/file names for identifier matching
// - path: File paths for directory/location queries
// - summary: Generated summaries for conceptual matching
// - description: Detailed descriptions if available
//
// Combined with vector search via Reciprocal Rank Fusion (RRF),
// this improves retrieval for queries mentioning specific identifiers,
// paths, or domain terminology.

// Create fulltext index on MemoryNode searchable fields
CREATE FULLTEXT INDEX memNodeFullText IF NOT EXISTS
FOR (n:MemoryNode)
ON EACH [n.name, n.path, n.summary, n.description];

// Update schema version
MATCH (s:SchemaMeta {key:'schema'})
SET s.current_version = 6, s.updated_at = datetime();
