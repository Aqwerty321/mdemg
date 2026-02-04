# MDEMG API Reference

This document provides a complete reference for all MDEMG HTTP API endpoints.

**Base URL**: `http://localhost:9999` (default)

## Table of Contents

- [Health Checks](#health-checks)
- [Memory Operations](#memory-operations)
- [Retrieval & Search](#retrieval--search)
- [Consolidation](#consolidation)
- [Learning System](#learning-system)
- [Conversation Memory](#conversation-memory)
- [Capability Gaps](#capability-gaps)
- [Linear Integration](#linear-integration)
- [Webhooks](#webhooks)
- [Plugins & Modules](#plugins--modules)
- [System & Monitoring](#system--monitoring)

---

## Health Checks

### GET /healthz

Basic liveness check.

**Response**:
```json
{"status": "ok"}
```

### GET /readyz

Readiness check (verifies Neo4j schema version).

**Response**:
```json
{
  "status": "ready",
  "embedding_provider": "openai",
  "embedding_dimensions": 1536
}
```

---

## Memory Operations

### POST /v1/memory/ingest

Store a single observation.

**Request Body**:
```json
{
  "space_id": "my-project",
  "name": "UserService.authenticate",
  "path": "src/services/user.ts",
  "content": "Authentication logic using JWT tokens...",
  "tags": ["auth", "security"],
  "timestamp": "2024-01-15T10:30:00Z",
  "source": "code-analysis"
}
```

**Response**:
```json
{
  "node_id": "mem-abc123",
  "status": "created",
  "embedding_dims": 1536
}
```

### POST /v1/memory/ingest/batch

Store multiple observations in a single request.

**Request Body**:
```json
{
  "space_id": "my-project",
  "observations": [
    {
      "name": "ConfigLoader",
      "path": "src/config/loader.ts",
      "content": "Loads environment configuration..."
    },
    {
      "name": "DatabaseConnection",
      "path": "src/db/connection.ts",
      "content": "PostgreSQL connection pool..."
    }
  ]
}
```

**Response**:
```json
{
  "success_count": 2,
  "error_count": 0,
  "results": [
    {"node_id": "mem-abc123", "status": "success"},
    {"node_id": "mem-def456", "status": "success"}
  ]
}
```

### POST /v1/memory/ingest/trigger

Trigger a background codebase re-ingestion job. Returns immediately with a job ID.

**Request Body**:
```json
{
  "space_id": "my-project",
  "path": "/path/to/codebase",
  "batch_size": 100,
  "workers": 4,
  "extract_symbols": true,
  "consolidate": true,
  "incremental": false,
  "exclude_dirs": ["vendor", "node_modules"]
}
```

**Response** (202 Accepted):
```json
{
  "job_id": "ingest-abc12345",
  "space_id": "my-project",
  "status": "pending",
  "message": "Ingestion job created. Use GET /v1/memory/ingest/status/ingest-abc12345 to check progress.",
  "created_at": "2026-02-04T10:00:00Z"
}
```

### GET /v1/memory/ingest/status/{job_id}

Check the status and progress of an ingestion job.

**Response**:
```json
{
  "job_id": "ingest-abc12345",
  "space_id": "my-project",
  "status": "running",
  "progress": {
    "total": 4522,
    "current": 1200,
    "percentage": 26.5,
    "phase": "ingestion",
    "rate": "15.2 elements/sec"
  },
  "started_at": "2026-02-04T10:00:01Z",
  "created_at": "2026-02-04T10:00:00Z"
}
```

### POST /v1/memory/ingest/cancel/{job_id}

Cancel a running or pending ingestion job.

**Response**:
```json
{
  "job_id": "ingest-abc12345",
  "status": "cancelled",
  "message": "Job cancellation requested"
}
```

### GET /v1/memory/ingest/jobs

List all ingestion jobs with their current status.

**Response**:
```json
{
  "jobs": [
    {
      "job_id": "ingest-abc12345",
      "status": "completed",
      "space_id": "my-project",
      "progress": {"total": 4522, "current": 4522, "percentage": 100},
      "created_at": "2026-02-04T10:00:00Z",
      "completed_at": "2026-02-04T10:05:06Z"
    }
  ],
  "count": 1
}
```

### POST /v1/memory/ingest/files

Re-ingest specific files into memory. Synchronous for ≤50 files; returns a background job ID for >50.

**Request Body**:
```json
{
  "space_id": "my-project",
  "files": ["/path/to/file1.go", "/path/to/file2.ts"],
  "extract_symbols": true,
  "consolidate": false
}
```

**Response** (synchronous):
```json
{
  "space_id": "my-project",
  "total_files": 2,
  "success_count": 2,
  "error_count": 0,
  "results": [
    {"file": "/path/to/file1.go", "status": "success", "node_id": "mem-abc123"},
    {"file": "/path/to/file2.ts", "status": "success", "node_id": "mem-def456"}
  ]
}
```

**Response** (>50 files, 202 Accepted):
```json
{
  "space_id": "my-project",
  "total_files": 75,
  "job_id": "ingest-files-abc12345"
}
```

### POST /v1/memory/nodes/{node_id}/archive

Soft-delete a memory node (sets `is_archived=true`).

**Request Body** (optional):
```json
{
  "reason": "Outdated implementation"
}
```

**Response**:
```json
{
  "node_id": "mem-abc123",
  "name": "OldService",
  "archived_at": "2024-01-15T10:30:00Z",
  "reason": "Outdated implementation"
}
```

### POST /v1/memory/nodes/{node_id}/unarchive

Restore an archived memory node.

**Response**:
```json
{
  "node_id": "mem-abc123",
  "name": "OldService",
  "unarchived_at": "2024-01-15T11:00:00Z"
}
```

### DELETE /v1/memory/nodes/{node_id}?confirm=true

Permanently delete a memory node. Requires `confirm=true` query parameter.

**Response**:
```json
{
  "node_id": "mem-abc123",
  "deleted_nodes": 1,
  "deleted_edges": 5
}
```

### POST /v1/memory/archive/bulk

Archive multiple nodes in a single request.

**Request Body**:
```json
{
  "space_id": "my-project",
  "node_ids": ["mem-abc123", "mem-def456"],
  "reason": "Deprecated module"
}
```

**Response**:
```json
{
  "space_id": "my-project",
  "total_items": 2,
  "success_count": 2,
  "error_count": 0,
  "results": [...]
}
```

---

## Retrieval & Search

### POST /v1/memory/retrieve

Semantic search with optional LLM re-ranking.

**Request Body**:
```json
{
  "space_id": "my-project",
  "query": "How does authentication work?",
  "top_k": 10,
  "include_evidence": true,
  "include_hidden": true,
  "min_score": 0.5,
  "rerank": true
}
```

**Parameters**:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `space_id` | string | Yes | Memory space identifier |
| `query` | string | Yes | Search query text |
| `top_k` | int | No | Max results (default: 10) |
| `include_evidence` | bool | No | Include file:line references |
| `include_hidden` | bool | No | Include hidden layer nodes |
| `min_score` | float | No | Minimum similarity score |
| `rerank` | bool | No | Apply LLM re-ranking |

**Response**:
```json
{
  "results": [
    {
      "node_id": "mem-abc123",
      "name": "AuthService.login",
      "score": 0.85,
      "layer": 0,
      "evidence": [
        {
          "symbol_name": "login",
          "symbol_type": "function",
          "file_path": "src/auth/service.ts",
          "line_number": 42
        }
      ]
    }
  ],
  "debug": {
    "query_time_ms": 45,
    "embedding_provider": "openai"
  }
}
```

### POST /v1/memory/consult

SME-style consultation with evidence-based answers.

**Request Body**:
```json
{
  "space_id": "my-project",
  "context": "Investigating authentication flow",
  "question": "What is the session timeout value?",
  "max_suggestions": 5,
  "include_evidence": true
}
```

**Response**:
```json
{
  "answer": "The session timeout is configured to 3600 seconds (1 hour) in src/config/auth.ts:15",
  "confidence": "HIGH",
  "suggestions": [
    {
      "node_id": "mem-abc123",
      "relevance": 0.92,
      "evidence": [...]
    }
  ]
}
```

### POST /v1/memory/suggest

Context-triggered suggestions (proactive memory surfacing).

**Request Body**:
```json
{
  "space_id": "my-project",
  "context": "User is editing authentication code",
  "max_suggestions": 3
}
```

**Response**:
```json
{
  "suggestions": [
    {
      "node_id": "mem-abc123",
      "name": "Security best practices",
      "relevance": 0.88,
      "reason": "Related to authentication patterns"
    }
  ]
}
```

### POST /v1/memory/reflect

Deep exploration of a topic.

**Request Body**:
```json
{
  "space_id": "my-project",
  "topic": "database connection patterns",
  "depth": 3
}
```

---

## Consolidation

### POST /v1/memory/consolidate

Trigger hidden layer creation (concept abstraction).

**Request Body**:
```json
{
  "space_id": "my-project",
  "skip_clustering": false,
  "skip_forward": false,
  "skip_backward": false
}
```

**Response**:
```json
{
  "data": {
    "space_id": "my-project",
    "enabled": true,
    "hidden_nodes_created": 45,
    "concept_nodes_created": 12,
    "concern_nodes_created": 3,
    "comparison_nodes_created": 8,
    "hidden_nodes_updated": 150,
    "edges_strengthened": 230,
    "summaries_generated": 57,
    "duration_ms": 12500
  }
}
```

---

## Learning System

### GET /v1/learning/stats?space_id={space_id}

Get Hebbian learning edge statistics.

**Response**:
```json
{
  "space_id": "my-project",
  "total_edges": 1250,
  "avg_weight": 0.42,
  "max_weight": 0.95,
  "decay_per_day": 0.01,
  "prune_threshold": 0.1,
  "max_edges_per_node": 50
}
```

### POST /v1/learning/prune?space_id={space_id}

Prune decayed and excess learning edges.

**Response**:
```json
{
  "space_id": "my-project",
  "decayed_deleted": 45,
  "excess_deleted": 12,
  "total_deleted": 57
}
```

---

## Conversation Memory

Endpoints for the Conversation Memory System (CMS) - capturing, recalling, and managing conversational knowledge.

### POST /v1/conversation/observe

Capture a significant observation with auto-surprise scoring.

**Request Body**:
```json
{
  "space_id": "my-project",
  "session_id": "session-abc123",
  "content": "User prefers TypeScript over JavaScript",
  "obs_type": "preference",
  "tags": ["coding-style"],
  "metadata": {"context": "discussion about language choice"},
  "user_id": "alice",
  "visibility": "team",
  "refers_to": ["sym-validateInput-xyz"]
}
```

**Fields**:
- `obs_type`: `decision`, `learning`, `preference`, `error`, `task`, `correction`
- `visibility`: `private` (owner only), `team` (space members), `global` (everyone, default)
- `refers_to`: Array of node IDs to create REFERS_TO edges

**Response**:
```json
{
  "obs_id": "obs-abc123",
  "node_id": "mem-xyz789",
  "surprise_score": 0.65,
  "surprise_factors": {
    "term_novelty": 0.7,
    "correction_score": 0.0,
    "embedding_novelty": 0.6
  },
  "summary": "User preference for TypeScript"
}
```

### POST /v1/conversation/correct

Capture an explicit user correction (high surprise, persistent).

**Request Body**:
```json
{
  "space_id": "my-project",
  "session_id": "session-abc123",
  "incorrect": "The ORM is Hibernate",
  "correct": "The ORM is BlueSeerData, a custom framework",
  "context": "User corrected my assumption about the database layer",
  "user_id": "alice",
  "visibility": "global"
}
```

**Response**: Same as `/observe` with higher surprise score (baseline 0.5).

### POST /v1/conversation/resume

Restore context after context compaction.

**Request Body**:
```json
{
  "space_id": "my-project",
  "session_id": "session-abc123",
  "include_tasks": true,
  "include_decisions": true,
  "include_learnings": true,
  "max_observations": 20,
  "requesting_user_id": "alice"
}
```

**Response**:
```json
{
  "space_id": "my-project",
  "session_id": "session-abc123",
  "observations": [
    {
      "node_id": "mem-obs1",
      "obs_type": "decision",
      "content": "Using plugin architecture",
      "summary": "Architecture decision",
      "surprise_score": 0.5,
      "created_at": "2026-01-27T10:00:00Z"
    }
  ],
  "themes": [...],
  "emergent_concepts": [...],
  "jiminy": {
    "rationale": "Restoring 15 observations from session...",
    "confidence": 0.72,
    "score_breakdown": {
      "observation_coverage": 0.85,
      "theme_relevance": 0.65,
      "recency_boost": 0.70
    },
    "highlights": ["Decision: Using plugin architecture"]
  },
  "summary": "Resuming with 15 observations..."
}
```

### POST /v1/conversation/recall

Retrieve relevant conversation knowledge via semantic query.

**Request Body**:
```json
{
  "space_id": "my-project",
  "query": "What do I know about user preferences?",
  "top_k": 10,
  "include_themes": true,
  "include_concepts": true,
  "requesting_user_id": "alice"
}
```

**Response**:
```json
{
  "space_id": "my-project",
  "query": "What do I know about user preferences?",
  "results": [
    {
      "type": "emergent_concept",
      "node_id": "concept-123",
      "content": "User prefers modular architecture",
      "score": 0.85,
      "layer": 2
    }
  ]
}
```

### POST /v1/conversation/consolidate

Trigger consolidation to form themes and emergent concepts.

**Request Body**:
```json
{
  "space_id": "my-project"
}
```

**Response**:
```json
{
  "space_id": "my-project",
  "themes_created": 3,
  "concepts_created": 1,
  "duration_ms": 1250
}
```

### GET /v1/conversation/volatile/stats

Get statistics about volatile (ungraduated) observations.

**Query Parameters**:
- `space_id` (required)

**Response**:
```json
{
  "space_id": "my-project",
  "volatile_count": 15,
  "permanent_count": 42,
  "avg_volatile_stability": 0.35,
  "min_volatile_stability": 0.10,
  "max_volatile_stability": 0.72
}
```

### POST /v1/conversation/graduate

Manually trigger graduation processing for the Context Cooler.

**Request Body**:
```json
{
  "space_id": "my-project"
}
```

**Response**:
```json
{
  "space_id": "my-project",
  "timestamp": "2026-01-27T12:00:00Z",
  "graduated": 3,
  "tombstoned": 1,
  "remaining_volatile": 11,
  "decay_applied": 5
}
```

---

## Capability Gaps

Endpoints for capability gap detection and interview prompts.

### GET /v1/system/capability-gaps

List all capability gaps.

**Query Parameters**:
- `status`: `open`, `addressed`, `dismissed`
- `type`: `data_source`, `reasoning`, `query_pattern`
- `space_id`: Filter by space

**Response**:
```json
{
  "data": {
    "gaps": [
      {
        "id": "gap-abc123",
        "type": "data_source",
        "description": "Content references Slack but no integration exists",
        "evidence": ["slack"],
        "suggested_plugin": {
          "type": "INGESTION",
          "name": "slack-ingestion",
          "description": "Ingest Slack messages and channels"
        },
        "priority": 0.85,
        "status": "open",
        "occurrence_count": 15
      }
    ],
    "summary": {
      "total": 5,
      "by_type": {"data_source": 3, "reasoning": 2},
      "high_priority": 2
    }
  }
}
```

### GET /v1/system/gap-interviews

Get pending interview prompts.

**Query Parameters**:
- `space_id` (optional)

**Response**:
```json
{
  "prompts": [
    {
      "id": "interview-abc123",
      "gap_id": "gap-xyz789",
      "gap_type": "data_source",
      "question": "How should MDEMG integrate with Slack?",
      "context": "This gap has been detected 15 times.",
      "suggestions": ["Create slack-ingestion plugin", "Configure webhook"],
      "priority": 0.85,
      "status": "pending"
    }
  ],
  "stats": {
    "total": 10,
    "pending": 5,
    "answered": 4,
    "skipped": 1
  }
}
```

### POST /v1/system/gap-interviews/run

Manually trigger the weekly gap interview process.

**Request Body** (optional):
```json
{
  "max_prompts": 10,
  "min_priority": 0.3,
  "min_occurrences": 3
}
```

**Response**:
```json
{
  "total_gaps_analyzed": 8,
  "prompts_generated": 5,
  "high_priority_count": 2,
  "gaps_by_type": {"data_source": 3, "reasoning": 2},
  "processed_at": "2026-01-27T12:00:00Z",
  "next_scheduled_at": "2026-02-03T12:00:00Z"
}
```

### POST /v1/system/gap-interviews/{id}/answer

Mark an interview prompt as answered.

**Request Body**:
```json
{
  "observation_node_id": "obs-abc123"
}
```

### POST /v1/system/gap-interviews/{id}/skip

Skip an interview prompt.

**Request Body**:
```json
{
  "reason": "Not relevant to this project"
}
```

---

## Linear Integration

CRUD operations for Linear issues, projects, and comments. Requires the `linear-module` plugin to be running.

### POST /v1/linear/issues

Create a new Linear issue.

**Request Body**:
```json
{
  "title": "Fix login bug",
  "team_id": "team-uuid",
  "description": "Users cannot log in on mobile",
  "priority": 2,
  "assignee_id": "user-uuid",
  "project_id": "project-uuid",
  "label_ids": ["label-uuid-1", "label-uuid-2"]
}
```

**Required fields**: `title`, `team_id`

**Response**:
```json
{
  "id": "issue-uuid",
  "identifier": "ENG-213",
  "title": "Fix login bug",
  "state": "Triage",
  "team_key": "ENG",
  "priority": "2",
  "created_at": "2026-02-03T10:00:00Z"
}
```

### GET /v1/linear/issues

List Linear issues with optional filters.

**Query Parameters**:
| Param | Type | Description |
|-------|------|-------------|
| `team` | string | Filter by team key (e.g., "ENG") |
| `state` | string | Filter by state name (e.g., "In Progress") |
| `assignee` | string | Filter by assignee name |
| `label` | string | Filter by label name |
| `limit` | int | Max results (default: 50) |
| `cursor` | string | Pagination cursor |

**Response**:
```json
{
  "issues": [
    {
      "id": "issue-uuid",
      "identifier": "ENG-213",
      "title": "Fix login bug",
      "state": "In Progress",
      "priority": "2"
    }
  ],
  "next_cursor": "cursor-string",
  "has_more": true
}
```

### GET /v1/linear/issues/{id}

Read a single issue by ID.

**Response**: Same shape as individual issue in list response, with all fields populated.

### PUT /v1/linear/issues/{id}

Update an existing issue. Only provided fields are modified.

**Request Body**:
```json
{
  "title": "Updated title",
  "priority": 1,
  "state_id": "state-uuid",
  "assignee_id": "user-uuid"
}
```

**Response**: Updated issue fields.

### DELETE /v1/linear/issues/{id}

Archive (soft-delete) an issue.

**Response**:
```json
{
  "success": true
}
```

### POST /v1/linear/projects

Create a new Linear project.

**Request Body**:
```json
{
  "name": "Q1 Sprint",
  "description": "First quarter deliverables"
}
```

**Required fields**: `name`

### GET /v1/linear/projects

List Linear projects.

**Query Parameters**: `limit`, `cursor`

### GET /v1/linear/projects/{id}

Read a single project by ID.

### PUT /v1/linear/projects/{id}

Update an existing project.

### POST /v1/linear/comments

Add a comment to an issue.

**Request Body**:
```json
{
  "issue_id": "issue-uuid",
  "body": "This needs attention."
}
```

**Required fields**: `issue_id`, `body`

**Response**:
```json
{
  "id": "comment-uuid",
  "body": "This needs attention.",
  "issue_id": "issue-uuid",
  "created_at": "2026-02-03T10:05:00Z"
}
```

---

## Webhooks

### POST /v1/webhooks/linear

Receives Linear webhook events and ingests them as observations via the `linear-module` plugin.

**Authentication:** HMAC-SHA256 signature verification via the `Linear-Signature` header.

**Environment variables:**
- `LINEAR_WEBHOOK_SECRET` — HMAC-SHA256 signing secret (required)
- `LINEAR_WEBHOOK_SPACE_ID` — Target space ID for ingested observations (required)

**Supported events:**
- `Issue` create/update
- `Project` update

Other event types are acknowledged with 200 but ignored.

**Debouncing:** Rapid events for the same entity are coalesced with a 10-second window.

**Request headers:**
- `Linear-Signature` — HMAC-SHA256 hex digest of the request body

**Response:**
```json
{
  "status": "accepted",
  "type": "Issue",
  "action": "create",
  "debounce": "Issue:ISS-123"
}
```

**Error responses:**
- `401 Unauthorized` — Missing or invalid signature
- `405 Method Not Allowed` — Non-POST request
- `500 Internal Server Error` — Webhook secret not configured

---

## Plugins & Modules

### GET /v1/plugins

List all plugins.

**Response**:
```json
{
  "data": {
    "plugins": [
      {
        "id": "github-issues",
        "name": "GitHub Issues Ingestion",
        "type": "INGESTION",
        "version": "1.0.0",
        "status": "running"
      }
    ]
  }
}
```

### GET /v1/plugins/{id}

Get plugin details.

### POST /v1/plugins/create

Create a new plugin scaffold.

**Request Body**:
```json
{
  "name": "my-plugin",
  "type": "INGESTION",
  "version": "1.0.0",
  "description": "Custom data ingestion",
  "capabilities": ["custom-source"]
}
```

### POST /v1/plugins/{id}/validate

Validate a plugin's manifest, proto compliance, and health.

### GET /v1/modules

List loaded plugin modules.

### POST /v1/modules/{id}/sync

Trigger a sync operation on an ingestion module.

---

## System & Monitoring

### GET /v1/metrics?space_id={space_id}

Get graph metrics (nodes, edges, hub analysis).

**Response**:
```json
{
  "total_nodes": 5000,
  "total_edges": 15000,
  "nodes_by_layer": {"0": 4500, "1": 450, "2": 50},
  "edges_by_type": {
    "ABSTRACTS_TO": 5000,
    "CO_ACTIVATED_WITH": 10000
  },
  "hub_nodes": [
    {"node_id": "mem-abc", "name": "CoreModule", "degree": 250}
  ],
  "orphan_nodes": 10,
  "avg_edge_weight": 0.45
}
```

### GET /v1/memory/stats?space_id={space_id}

Get per-space memory statistics.

**Response**:
```json
{
  "space_id": "my-project",
  "memory_count": 5000,
  "observation_count": 5500,
  "memories_by_layer": {"0": 4500, "1": 450, "2": 50},
  "embedding_coverage": 0.99,
  "avg_embedding_dimensions": 1536,
  "learning_activity": {
    "co_activated_edges": 10000,
    "avg_weight": 0.42,
    "max_weight": 0.95
  },
  "connectivity": {
    "avg_degree": 3.5,
    "max_degree": 250,
    "orphan_count": 10
  },
  "health_score": 0.85
}
```

### GET /v1/memory/cache/stats

Get query result cache statistics.

### GET /v1/memory/query/metrics

Get Neo4j query execution statistics.

### GET /v1/ape/status

Get APE (Autonomous Pattern Extraction) scheduler status.

### POST /v1/ape/trigger

Manually trigger an APE event.

**Request Body**:
```json
{
  "event": "consolidation_complete"
}
```

### GET /v1/system/capability-gaps?status={status}&type={type}&space_id={space_id}

List detected capability gaps.

**Response**:
```json
{
  "data": {
    "gaps": [
      {
        "id": "gap-abc123",
        "type": "data_source",
        "description": "Missing Jira integration",
        "priority": "high",
        "status": "open"
      }
    ],
    "summary": {
      "total": 5,
      "by_type": {"data_source": 2, "reasoning": 3},
      "high_priority": 2
    }
  }
}
```

### POST /v1/system/capability-gaps/{id}/dismiss

Dismiss a capability gap.

### POST /v1/system/capability-gaps/{id}/address

Mark a capability gap as addressed.

### POST /v1/feedback

Submit feedback for capability gap detection.

**Request Body**:
```json
{
  "space_id": "my-project",
  "query_text": "How does caching work?",
  "rating": "negative",
  "comment": "Results were not relevant"
}
```

---

## Space Freshness (Phase 9.2)

### `GET /v1/memory/spaces/{space_id}/freshness`

Returns freshness and staleness information for a space's TapRoot node.

**Path Parameters**:
- `space_id` - The space to check freshness for

**Response** (`200 OK`):
```json
{
  "space_id": "my-project",
  "last_ingest_at": "2026-02-03T15:30:00Z",
  "last_ingest_type": "codebase-ingest",
  "ingest_count": 12,
  "is_stale": false,
  "stale_hours": 8,
  "threshold_hours": 24
}
```

**Fields**:
- `last_ingest_at` - ISO8601 timestamp of last ingest (omitted if never ingested)
- `last_ingest_type` - Type of last ingest (`codebase-ingest`, `file-ingest`)
- `ingest_count` - Total number of ingestions for this space
- `is_stale` - Whether the space is considered stale based on `SYNC_STALE_THRESHOLD_HOURS`
- `stale_hours` - Hours since last ingest
- `threshold_hours` - Configured staleness threshold in hours

---

## Error Responses

All endpoints return errors in a consistent format:

```json
{
  "error": "error message"
}
```

**HTTP Status Codes**:
- `200` - Success
- `201` - Created
- `207` - Multi-Status (partial success in batch operations)
- `400` - Bad Request (validation error)
- `404` - Not Found
- `405` - Method Not Allowed
- `409` - Conflict (e.g., cannot delete node with children)
- `500` - Internal Server Error
- `503` - Service Unavailable

---

## Authentication

MDEMG does not currently require authentication. For production deployments, consider placing it behind a reverse proxy with authentication.
