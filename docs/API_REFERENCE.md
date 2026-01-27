# MDEMG API Reference

This document provides a complete reference for all MDEMG HTTP API endpoints.

**Base URL**: `http://localhost:8090` (default)

## Table of Contents

- [Health Checks](#health-checks)
- [Memory Operations](#memory-operations)
- [Retrieval & Search](#retrieval--search)
- [Consolidation](#consolidation)
- [Learning System](#learning-system)
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
