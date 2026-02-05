# UATS: Universal API Test Specification

## What is UATS?

UATS is a **specification-driven API testing framework** for validating all MDEMG HTTP endpoints. Each spec defines the request contract, expected response, and assertions.

---

## The Problem UATS Solves

Before UATS:

```
❌ API tests were ad-hoc curl commands in bash scripts
❌ No consistent format for expected responses
❌ Hard to maintain as endpoints changed
❌ No single source of truth for API contracts
❌ CI integration was painful
```

After UATS:

```
✅ One JSON spec per endpoint
✅ Declarative request/response contracts
✅ Self-documenting specs
✅ Easy CI: make test-api
✅ Clear workflow for new endpoints
```

---

## Directory Structure

```
docs/api/api-spec/uats/
├── schema/
│   └── uats.schema.json           # JSON Schema definition
│
├── specs/                          # One spec per endpoint (45 total)
│   ├── health.uats.json           # GET /healthz
│   ├── retrieve.uats.json         # POST /v1/memory/retrieve
│   ├── ingest.uats.json           # POST /v1/memory/ingest
│   ├── conversation_observe.uats.json
│   └── ... (45 total)
│
└── runners/
    └── uats_runner.py              # Python test runner
```

---

## Anatomy of a UATS Spec

```json
{
  "uats_version": "1.0.0",

  "api": {
    "name": "Health Check",
    "base_url": "${MDEMG_BASE_URL}",
    "version": "v1",
    "service": "mdemg",
    "tags": ["health", "smoke"]
  },

  "metadata": {
    "author": "reh3376",
    "created": "2026-01-29",
    "description": "Validates Health Check endpoint",
    "test_type": "contract",
    "priority": "high"
  },

  "config": {
    "timeout_ms": 15000,
    "sha256": "e63a51418ba9497dbea578598..."
  },

  "variables": {
    "test_space": "demo"
  },

  "request": {
    "method": "GET",
    "path": "/healthz"
  },

  "expected": {
    "status": 200,
    "body_assertions": [
      {
        "path": "$.status",
        "equals": "ok"
      }
    ]
  }
}
```

---

## Key Concepts

### 1. Request Definition

```json
"request": {
  "method": "POST",
  "path": "/v1/memory/retrieve",
  "headers": {
    "Content-Type": "application/json"
  },
  "body": {
    "space_id": "${test_space}",
    "query": "What is the main function?",
    "limit": 5
  }
}
```

### 2. Variable Substitution

Variables are defined in `variables` and used with `${var_name}`:

```json
"variables": {
  "test_space": "uats-test-space",
  "query_limit": 10
},
"request": {
  "body": {
    "space_id": "${test_space}",
    "limit": "${query_limit}"
  }
}
```

### 3. Body Assertions

Use JSONPath to assert on response body:

```json
"body_assertions": [
  {
    "path": "$.status",
    "equals": "ok"
  },
  {
    "path": "$.results",
    "type": "array"
  },
  {
    "path": "$.results.length()",
    "gte": 1
  },
  {
    "path": "$.data.memory_count",
    "exists": true
  }
]
```

**Assertion operators:**
- `equals` - Exact match
- `contains` - String contains
- `exists` - Field exists
- `type` - Type check (array, object, string, number)
- `gte`, `lte`, `gt`, `lt` - Numeric comparisons
- `matches` - Regex match

### 4. Test Variants

One spec can have multiple test cases (variants):

```json
{
  "request": { ... },
  "expected": { ... },

  "variants": [
    {
      "name": "missing_query",
      "description": "Error when query is missing",
      "request_override": {
        "body": {
          "space_id": "${test_space}"
        }
      },
      "expected": {
        "status": 400,
        "body_assertions": [
          {
            "path": "$.error",
            "contains": "query"
          }
        ]
      }
    },
    {
      "name": "empty_space",
      "description": "Error when space_id is empty",
      "request_override": {
        "body": {
          "space_id": "",
          "query": "test"
        }
      },
      "expected": {
        "status": 400
      }
    }
  ]
}
```

---

## Endpoint Categories

### Health (2 specs)

| Spec | Endpoint | Purpose |
|------|----------|---------|
| health | GET /healthz | Liveness probe |
| readiness | GET /readyz | Readiness probe |

### Core Memory (14 specs)

| Spec | Endpoint | Purpose |
|------|----------|---------|
| retrieve | POST /v1/memory/retrieve | Semantic search |
| ingest | POST /v1/memory/ingest | Add memory node |
| ingest_batch | POST /v1/memory/ingest/batch | Batch ingest |
| stats | GET /v1/memory/stats | Memory statistics |
| consolidate | POST /v1/memory/consolidate | Run consolidation |
| symbols | GET /v1/memory/symbols | Symbol search |

### Conversation CMS (7 specs)

| Spec | Endpoint | Purpose |
|------|----------|---------|
| conversation_observe | POST /v1/conversation/observe | Capture observation |
| conversation_resume | POST /v1/conversation/resume | Resume session |
| conversation_recall | POST /v1/conversation/recall | Recall memories |
| conversation_correct | POST /v1/conversation/correct | Record correction |

### Learning (5 specs)

| Spec | Endpoint | Purpose |
|------|----------|---------|
| learning_stats | GET /v1/learning/stats | Learning statistics |
| learning_freeze | POST /v1/learning/freeze | Freeze learning |
| learning_prune | POST /v1/learning/prune | Prune learning edges |

---

## Running UATS Tests

### All Tests

```bash
make test-api

# Or directly:
python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
    --spec-dir docs/api/api-spec/uats/specs/ \
    --base-url http://localhost:9999 \
    --report /tmp/api-report.json
```

### Single Endpoint

```bash
make test-api-health

# Or directly:
python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
    --spec docs/api/api-spec/uats/specs/health.uats.json \
    --base-url http://localhost:9999
```

### By Category

```bash
make test-api-conversation  # All conversation_*.uats.json
make test-api-learning      # All learning_*.uats.json
make test-api-memory        # All memory-related specs
```

---

## Walkthrough: Adding a New API Endpoint Spec

Let's add a spec for a hypothetical new endpoint: `POST /v1/memory/summarize`

### Step 1: Understand the Endpoint

First, document what the endpoint does:

```
POST /v1/memory/summarize
- Takes a space_id and optional filters
- Returns a summary of memories in that space
- Response includes: total_count, top_tags, recent_topics
```

### Step 2: Create the Spec File

Create `docs/api/api-spec/uats/specs/summarize.uats.json`:

```json
{
  "uats_version": "1.0.0",

  "api": {
    "name": "Memory Summarize",
    "base_url": "${MDEMG_BASE_URL}",
    "version": "v1",
    "service": "mdemg",
    "tags": ["memory", "analytics"]
  },

  "metadata": {
    "author": "your-name",
    "created": "2026-02-06",
    "description": "Validates Memory Summarize endpoint",
    "test_type": "contract",
    "priority": "medium"
  },

  "config": {
    "timeout_ms": 30000
  },

  "variables": {
    "test_space": "uats-summarize-test"
  },

  "request": {
    "method": "POST",
    "path": "/v1/memory/summarize",
    "headers": {
      "Content-Type": "application/json"
    },
    "body": {
      "space_id": "${test_space}"
    }
  },

  "expected": {
    "status": 200,
    "headers": {
      "Content-Type": "application/json"
    },
    "body_assertions": [
      {
        "path": "$.total_count",
        "type": "number"
      },
      {
        "path": "$.total_count",
        "gte": 0
      },
      {
        "path": "$.top_tags",
        "type": "array"
      },
      {
        "path": "$.recent_topics",
        "type": "array"
      }
    ]
  },

  "variants": [
    {
      "name": "missing_space_id",
      "description": "Error when space_id is missing",
      "request_override": {
        "body": {}
      },
      "expected": {
        "status": 400,
        "body_assertions": [
          {
            "path": "$.error",
            "exists": true
          }
        ]
      }
    },
    {
      "name": "with_time_filter",
      "description": "Summarize with time range filter",
      "request_override": {
        "body": {
          "space_id": "${test_space}",
          "since": "2026-01-01T00:00:00Z",
          "until": "2026-02-01T00:00:00Z"
        }
      },
      "expected": {
        "status": 200,
        "body_assertions": [
          {
            "path": "$.total_count",
            "type": "number"
          },
          {
            "path": "$.time_range.start",
            "exists": true
          },
          {
            "path": "$.time_range.end",
            "exists": true
          }
        ]
      }
    },
    {
      "name": "with_tag_filter",
      "description": "Summarize filtered by tags",
      "request_override": {
        "body": {
          "space_id": "${test_space}",
          "tags": ["important", "decision"]
        }
      },
      "expected": {
        "status": 200,
        "body_assertions": [
          {
            "path": "$.total_count",
            "type": "number"
          },
          {
            "path": "$.filter_applied.tags",
            "type": "array"
          }
        ]
      }
    },
    {
      "name": "nonexistent_space",
      "description": "Empty summary for nonexistent space",
      "request_override": {
        "body": {
          "space_id": "nonexistent-space-xyz"
        }
      },
      "expected": {
        "status": 200,
        "body_assertions": [
          {
            "path": "$.total_count",
            "equals": 0
          }
        ]
      }
    }
  ]
}
```

### Step 3: Run Validation

```bash
# Start server
./bin/server &

# Run the new spec
python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
    --spec docs/api/api-spec/uats/specs/summarize.uats.json \
    --base-url http://localhost:9999 \
    --verbose
```

### Step 4: Iterate Until Passing

Common issues:
- **Status code wrong:** Check endpoint implementation
- **Body assertion fails:** Verify response structure matches spec
- **Timeout:** Increase `timeout_ms` or optimize endpoint

### Step 5: Add SHA256 Hash

After spec is stable, add integrity hash:

```bash
python3 docs/api/api-spec/uats/runners/uats_runner.py add-hashes \
    --spec docs/api/api-spec/uats/specs/summarize.uats.json
```

### Step 6: Update Documentation

Add to `docs/api/api-spec/uats/README.md`:

```markdown
### Core Memory (15)  ← Updated count
| Spec | Method | Endpoint |
|------|--------|----------|
| summarize | POST | /v1/memory/summarize |  ← New row
```

---

## Best Practices

### 1. Always Test Error Cases

Every spec should have variants for:
- Missing required fields
- Invalid field values
- Authorization errors (if applicable)

### 2. Use Meaningful Variable Names

```json
// Good
"variables": {
  "test_space": "uats-retrieve-test",
  "query_limit": 5
}

// Bad
"variables": {
  "var1": "test",
  "x": 5
}
```

### 3. Keep Specs Self-Contained

Each spec should work independently. Don't rely on state from other specs.

### 4. Use Unique Test Spaces

Avoid conflicts by using unique space IDs:

```json
"variables": {
  "test_space": "uats-{endpoint-name}-test"
}
```

### 5. Document Complex Assertions

Add comments via `description` fields:

```json
{
  "name": "rate_limited",
  "description": "When rate limit exceeded, returns 429 with retry-after header",
  ...
}
```

---

## Q&A: Anticipated Questions

### Q: How do I test endpoints that require authentication?

**A:** Use the `--token` flag:

```bash
python3 uats_runner.py validate \
    --spec specs/retrieve.uats.json \
    --token "$API_TOKEN"
```

Or add to spec:
```json
"request": {
  "headers": {
    "Authorization": "Bearer ${auth_token}"
  }
}
```

### Q: What if my endpoint has side effects?

**A:** UATS specs should be idempotent where possible. For endpoints that create data:
1. Use unique identifiers (timestamps, UUIDs)
2. Clean up in a setup/teardown script
3. Use a dedicated test space that gets reset

### Q: How do I test file uploads?

**A:** UATS supports multipart forms:

```json
"request": {
  "method": "POST",
  "path": "/v1/upload",
  "multipart": {
    "file": "@fixtures/test.txt",
    "metadata": "{\"name\": \"test\"}"
  }
}
```

### Q: Can I chain multiple requests?

**A:** Not in a single spec. For workflows:
1. Create separate specs for each endpoint
2. Run them in sequence via a wrapper script
3. Or use the runner's `--setup` flag for pre-test requests

### Q: What's the difference between `equals` and `contains`?

**A:**
- `equals`: Exact match - `"ok"` must equal `"ok"`
- `contains`: Substring match - `"error occurred"` contains `"error"`

### Q: How do I validate array contents?

**A:** Use JSONPath array operations:

```json
"body_assertions": [
  {
    "path": "$.results[0].name",
    "exists": true
  },
  {
    "path": "$.results[?(@.type=='function')]",
    "type": "array"
  },
  {
    "path": "$.results.length()",
    "gte": 1
  }
]
```

### Q: What happens if the server is down?

**A:** The runner catches connection errors and reports them as `ERROR` (not `FAIL`). The report distinguishes between:
- `PASS` - All assertions passed
- `FAIL` - Server responded but assertions failed
- `ERROR` - Connection failed or timeout

### Q: How do I skip a spec temporarily?

**A:** Add to metadata:

```json
"metadata": {
  "skip": true,
  "skip_reason": "Endpoint not yet implemented"
}
```

---

## Comparison: UPTS vs UATS

| Aspect | UPTS (Parsers) | UATS (APIs) |
|--------|----------------|-------------|
| **Scope** | 20 languages | 45 endpoints |
| **Input** | Source code files | HTTP requests |
| **Output** | Symbol JSON | HTTP responses |
| **Validation** | Symbol matching | Status, headers, body |
| **Runner** | Go-native + Python | Python |
| **CI Command** | `go test -run TestUPTS` | `make test-api` |
| **Directory** | `docs/lang-parser/...upts/` | `docs/api/api-spec/uats/` |

---

## Summary

1. **UATS = Single source of truth** for API contracts
2. **Spec structure:** API info + Request + Expected + Variants
3. **Workflow:** Understand endpoint → Create spec → Add variants → Iterate until passing
4. **Run tests:** `make test-api` or `python3 uats_runner.py validate-all`

---

**Next:** [Speaker Notes & Timing](./03-SPEAKER-NOTES.md)
