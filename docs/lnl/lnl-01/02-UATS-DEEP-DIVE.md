---
title: "UATS: Universal API Test Specification"
date: 2026-02-06
tags:
  - lnl
  - uats
  - testing
  - api
  - spec-driven
aliases:
  - UATS Deep Dive
  - LnL-01 Part 3
---

# Part 3: UATS — Universal API Test Specification

> [!abstract] 17-minute deep dive
> How we validate 45+ API endpoints with declarative JSON specs. Anatomy of a spec, running tests, and a complete walkthrough of adding a new endpoint test.

---

## What is UATS?

UATS is a **specification-driven API testing framework** for validating all MDEMG HTTP endpoints. Each spec defines the request contract, expected response, and assertions.

> [!tip] One sentence
> UATS is for APIs what [[01-UPTS-DEEP-DIVE|UPTS]] is for parsers — a universal contract format.

### Before vs After

| Before UATS | After UATS |
|-------------|------------|
| Ad-hoc curl commands in bash scripts | One JSON spec per endpoint |
| No consistent format for expected responses | Declarative request/response contracts |
| Hard to maintain as endpoints changed | Self-documenting specs |
| CI integration was painful | `make test-api` |
| No single source of truth for API contracts | Spec IS the contract |

---

## Directory Structure

```
docs/api/api-spec/uats/
├── schema/
│   └── uats.schema.json           # JSON Schema definition
├── specs/                          # One spec per endpoint (45 total)
│   ├── health.uats.json
│   ├── retrieve.uats.json
│   ├── ingest.uats.json
│   ├── conversation_observe.uats.json
│   └── ... (45 total)
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

  "config": {
    "timeout_ms": 15000
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
  "headers": {"Content-Type": "application/json"},
  "body": {
    "space_id": "${test_space}",
    "query": "What is the main function?",
    "limit": 5
  }
}
```

### 2. Variable Substitution

Variables are defined once and used with `${var_name}`:

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

### 3. Body Assertions (JSONPath)

> [!info] Assertion Operators
> | Operator | Meaning |
> |----------|---------|
> | `equals` | Exact match |
> | `contains` | String contains |
> | `exists` | Field exists |
> | `type` | Type check (`array`, `object`, `string`, `number`) |
> | `gte`, `lte`, `gt`, `lt` | Numeric comparisons |
> | `matches` | Regex match |

```json
"body_assertions": [
  {"path": "$.status", "equals": "ok"},
  {"path": "$.results", "type": "array"},
  {"path": "$.results.length()", "gte": 1},
  {"path": "$.data.memory_count", "exists": true}
]
```

### 4. Test Variants

One spec can have multiple test cases — especially important for error paths:

```json
"variants": [
  {
    "name": "missing_query",
    "description": "Error when query is missing",
    "request_override": {
      "body": {"space_id": "${test_space}"}
    },
    "expected": {
      "status": 400,
      "body_assertions": [
        {"path": "$.error", "contains": "query"}
      ]
    }
  },
  {
    "name": "empty_space",
    "description": "Error when space_id is empty",
    "request_override": {
      "body": {"space_id": "", "query": "test"}
    },
    "expected": {"status": 400}
  }
]
```

> [!warning] Always test the unhappy paths
> Every spec should have variants for missing required fields, invalid values, and authorization errors.

---

## Endpoint Categories

### Health (2 specs)

| Spec | Endpoint | Purpose |
|------|----------|---------|
| health | `GET /healthz` | Liveness probe |
| readiness | `GET /readyz` | Readiness probe |

### Core Memory (14 specs)

| Spec | Endpoint | Purpose |
|------|----------|---------|
| retrieve | `POST /v1/memory/retrieve` | Semantic search |
| ingest | `POST /v1/memory/ingest` | Add memory node |
| ingest_batch | `POST /v1/memory/ingest/batch` | Batch ingest |
| stats | `GET /v1/memory/stats` | Memory statistics |
| consolidate | `POST /v1/memory/consolidate` | Run consolidation |
| symbols | `GET /v1/memory/symbols` | Symbol search |

### Conversation CMS (7 specs)

| Spec | Endpoint | Purpose |
|------|----------|---------|
| conversation_observe | `POST /v1/conversation/observe` | Capture observation |
| conversation_resume | `POST /v1/conversation/resume` | Resume session |
| conversation_recall | `POST /v1/conversation/recall` | Recall memories |
| conversation_correct | `POST /v1/conversation/correct` | Record correction |

### Learning (5 specs)

| Spec | Endpoint | Purpose |
|------|----------|---------|
| learning_stats | `GET /v1/learning/stats` | Learning statistics |
| learning_freeze | `POST /v1/learning/freeze` | Freeze learning |
| learning_prune | `POST /v1/learning/prune` | Prune learning edges |

---

## Running UATS Tests

### All tests

```bash
make test-api
```

### Single endpoint

```bash
make test-api-health
```

### By category

```bash
make test-api-conversation  # All conversation_*.uats.json
make test-api-learning      # All learning_*.uats.json
make test-api-memory        # All memory-related specs
```

### Directly via runner

```bash
python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
    --spec-dir docs/api/api-spec/uats/specs/ \
    --base-url http://localhost:9999 \
    --report /tmp/api-report.json
```

---

## Walkthrough: Adding a New Endpoint Spec

> [!example] Worked Example: `POST /v1/memory/summarize`
> A hypothetical new endpoint that returns a summary of memories in a space.

### Step 1: Understand the Endpoint

```
POST /v1/memory/summarize
  Takes: space_id + optional filters
  Returns: total_count, top_tags, recent_topics
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
  "config": {"timeout_ms": 30000},
  "variables": {"test_space": "uats-summarize-test"},

  "request": {
    "method": "POST",
    "path": "/v1/memory/summarize",
    "headers": {"Content-Type": "application/json"},
    "body": {"space_id": "${test_space}"}
  },

  "expected": {
    "status": 200,
    "body_assertions": [
      {"path": "$.total_count", "type": "number"},
      {"path": "$.total_count", "gte": 0},
      {"path": "$.top_tags", "type": "array"},
      {"path": "$.recent_topics", "type": "array"}
    ]
  },

  "variants": [
    {
      "name": "missing_space_id",
      "description": "Error when space_id is missing",
      "request_override": {"body": {}},
      "expected": {
        "status": 400,
        "body_assertions": [{"path": "$.error", "exists": true}]
      }
    },
    {
      "name": "with_time_filter",
      "description": "Summarize with time range",
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
          {"path": "$.total_count", "type": "number"},
          {"path": "$.time_range.start", "exists": true}
        ]
      }
    },
    {
      "name": "nonexistent_space",
      "description": "Empty summary for nonexistent space",
      "request_override": {
        "body": {"space_id": "nonexistent-space-xyz"}
      },
      "expected": {
        "status": 200,
        "body_assertions": [{"path": "$.total_count", "equals": 0}]
      }
    }
  ]
}
```

### Step 3: Run and Iterate

```bash
./bin/server &
python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
    --spec docs/api/api-spec/uats/specs/summarize.uats.json \
    --base-url http://localhost:9999 --verbose
```

> [!warning] Common failures and fixes
> - **Status code wrong** — Check endpoint implementation
> - **Body assertion fails** — Verify response structure matches spec
> - **Timeout** — Increase `timeout_ms` or optimize endpoint

### Step 4: Add SHA256 and Update Docs

```bash
# Lock the spec
python3 docs/api/api-spec/uats/runners/uats_runner.py add-hashes \
    --spec docs/api/api-spec/uats/specs/summarize.uats.json

# Update README table with the new endpoint
```

---

## Best Practices

> [!success] Five rules for good UATS specs
> 1. **Always test error cases** — missing fields, invalid values, auth errors
> 2. **Use meaningful variable names** — `test_space` not `var1`
> 3. **Keep specs self-contained** — don't rely on state from other specs
> 4. **Use unique test spaces** — `uats-{endpoint-name}-test` avoids conflicts
> 5. **Document complex assertions** — use `description` fields in variants

---

## UPTS vs UATS Comparison

| Aspect | UPTS (Parsers) | UATS (APIs) |
|--------|----------------|-------------|
| **Scope** | 25 languages | 45 endpoints |
| **Input** | Source code files | HTTP requests |
| **Output** | Symbol JSON | HTTP responses |
| **Validation** | Symbol matching | Status + headers + body |
| **Runner** | Go-native + Python | Python |
| **CI Command** | `go test -run TestUPTS` | `make test-api` |
| **Directory** | `docs/lang-parser/.../upts/` | `docs/api/api-spec/uats/` |

> [!tip] Same principle
> Both UPTS and UATS follow the same philosophy: **the spec is the contract**. Learn the pattern once, apply it everywhere.

---

## Q&A: Anticipated Questions

> [!faq]- How do I test endpoints that require authentication?
> Use the `--token` flag or add an `Authorization` header to the spec's request.

> [!faq]- What if my endpoint has side effects?
> Use unique identifiers, clean up in teardown, or use a dedicated test space that gets reset.

> [!faq]- Can I chain multiple requests?
> Not in a single spec. Create separate specs and run them in sequence via a wrapper script.

> [!faq]- What's the difference between `equals` and `contains`?
> `equals` = exact match. `contains` = substring match.

> [!faq]- How do I validate array contents?
> Use JSONPath array operations: `$.results[0].name`, `$.results[?(@.type=='function')]`, `$.results.length()`.

> [!faq]- What happens if the server is down?
> The runner reports `ERROR` (not `FAIL`). The report distinguishes: `PASS` (assertions passed), `FAIL` (server responded but assertions failed), `ERROR` (connection/timeout).

> [!faq]- How do I skip a spec temporarily?
> Add `"skip": true` and `"skip_reason": "..."` to metadata.

---

## Summary

> [!success] Key Takeaways
> 1. **UATS = single source of truth** for API contracts
> 2. **Spec structure:** API info + Request + Expected + Variants
> 3. **Workflow:** Understand endpoint → Create spec → Add variants → Iterate until green
> 4. **Run tests:** `make test-api`
> 5. **45+ specs, ~90 variants** — all green is the CI gate

---

## Related Frameworks (Phase 3)

UPTS and UATS are the foundation. Phase 3 adds three more spec-driven frameworks for production readiness:

| Framework | What It Tests | Location |
|-----------|---------------|----------|
| **UBTS** | Performance benchmarks (latency thresholds, throughput, concurrent load) | `docs/tests/ubts/` |
| **USTS** | Security (auth enforcement, rate limiting, injection protection) | `docs/tests/usts/` |
| **UOBS** | Observability (Prometheus metrics, health endpoints, log format) | `docs/tests/uobs/` |

> [!tip] Same Pattern
> All five frameworks (UPTS, UATS, UBTS, USTS, UOBS) follow the same principle: **the spec is the contract**. JSON schema, Python runner, CI integration.

---

**Next:** [[03-SPEAKER-NOTES|Speaker Notes & Timing →]]
