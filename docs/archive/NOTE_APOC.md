# NOTE: APOC dependency

The learning writeback query in `internal/retrieval/retrieval.go` uses `apoc.math.clamp` for convenient clamping.

If you do **not** have APOC installed, replace the clamp with manual Cypher `CASE` min/max logic.
This is a deliberate “flag” so you decide whether APOC is acceptable in your environment.
