# Linear Integration Module

The Linear module syncs teams, projects, and issues from Linear into MDEMG's memory graph.

## Quick Start

1. **Get a Linear API key:**
   - Go to Linear → Settings → Account → Security & Access
   - Create a new API key with "Read" scope

2. **Add to .env:**
   ```
   LINEAR_API_KEY=lin_api_xxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

3. **Sync data:**
   ```bash
   # Sync everything (teams, projects, issues)
   curl -X POST http://localhost:9999/v1/modules/linear-module/sync \
     -H "Content-Type: application/json" \
     -d '{"source_id": "linear://all", "ingest": true, "space_id": "linear"}'
   ```

## Sync Options

### Source Types

| Source ID | Description |
|-----------|-------------|
| `linear://all` | Sync teams, projects, and all issues |
| `linear://teams` | Sync only teams |
| `linear://projects` | Sync only projects |
| `linear://issues` | Sync all issues |
| `linear://issues?team=TEC` | Sync issues from specific team |

### Request Parameters

```json
{
  "source_id": "linear://all",    // What to sync
  "ingest": true,                  // Store in MDEMG (false = dry run)
  "space_id": "linear",           // Memory space to store in
  "cursor": ""                     // Resume from cursor (for incremental sync)
}
```

### Response

```json
{
  "data": {
    "count": 1948,
    "cursor": "6c460bae-7530-...",
    "ingested": 1948,
    "ingest_errors": 0,
    "stats": {
      "items_processed": 1806,
      "items_created": 1806,
      "items_updated": 0,
      "items_skipped": 0
    }
  }
}
```

## Data Model

### Teams → Observations

```
NodeId:      linear-team-{uuid}
Path:        linear://teams/{key}
ContentType: application/vnd.linear.team
Tags:        [linear, team, {key}]
Metadata:    linear_id, team_key, issue_count, private, created_at, updated_at
```

### Projects → Observations

```
NodeId:      linear-project-{uuid}
Path:        linear://projects/{uuid}
ContentType: application/vnd.linear.project
Tags:        [linear, project, {state}, {team_keys...}]
Metadata:    linear_id, state, progress, lead, created_at, updated_at
```

### Issues → Observations

```
NodeId:      linear-issue-{uuid}
Path:        linear://issues/{identifier}
ContentType: application/vnd.linear.issue
Tags:        [linear, issue, {team_key}, {state_type}, {labels...}]
Metadata:    linear_id, identifier, team_key, state, state_type, priority,
             project_id, project_name, assignee, created_at, updated_at
```

## Querying Linear Data

```bash
# Search for issues related to documentation
curl -X POST http://localhost:9999/v1/memory/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "linear",
    "query_text": "documentation migration",
    "top_k": 10
  }'
```

## Module Health

```bash
# Check module status
curl http://localhost:9999/v1/modules | jq '.data.modules[] | select(.id == "linear-module")'
```

Response includes:
- `api_configured`: Whether LINEAR_API_KEY is set
- `last_sync`: Timestamp of last sync
- `requests_handled`: Number of API calls made
- `uptime`: Module uptime

## Environment Variables

| Variable | Description |
|----------|-------------|
| `LINEAR_API_KEY` | Linear API key (required) |

## Incremental Sync

The module supports incremental sync using cursors:

```bash
# First sync - full
curl -X POST .../sync -d '{"source_id": "linear://issues", "ingest": true, "space_id": "linear"}'
# Response: {"cursor": "abc123..."}

# Later - incremental from cursor
curl -X POST .../sync -d '{"source_id": "linear://issues", "cursor": "abc123...", "ingest": true, "space_id": "linear"}'
```

## Rate Limits

The module respects Linear's API rate limits:
- 1,500 requests/hour for API key auth
- 100ms delay between paginated requests

## Troubleshooting

### "LINEAR_API_KEY not configured"
Add `LINEAR_API_KEY=lin_api_xxx` to your `.env` file and restart the server.

### "GraphQL errors"
Check that your API key has "Read" scope and hasn't expired.

### Vector dimension mismatch
If you see errors about vector dimensions, ensure your vector index matches your embedding provider:
- OpenAI: 1536 dimensions
- Ollama: 768 dimensions

Recreate the index if needed:
```cypher
DROP INDEX memNodeEmbedding IF EXISTS;
CREATE VECTOR INDEX memNodeEmbedding FOR (n:MemoryNode) ON (n.embedding)
OPTIONS {indexConfig: {`vector.dimensions`: 1536, `vector.similarity_function`: 'COSINE'}};
```
