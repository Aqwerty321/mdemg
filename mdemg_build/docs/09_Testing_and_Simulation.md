# Testing & Simulation

## Test Suite Overview

| Type | Location | Command |
|------|----------|---------|
| Unit Tests | `internal/*/` | `go test ./internal/...` |
| Integration Tests | `tests/integration/` | `go test -tags=integration ./tests/integration/...` |
| Handler Tests | `internal/api/*_test.go` | `go test ./internal/api/...` |

---

## 1) Unit Tests

Located in each package alongside source files.

### Key Test Files

| Package | Test File | Coverage |
|---------|-----------|----------|
| `internal/learning` | `service_test.go` | Hebbian learning, edge cap, weight bounds |
| `internal/anomaly` | `service_test.go` | Duplicate detection, stale detection, timeout |
| `internal/api` | `handlers_test.go` | Handler validation, response formatting |
| `internal/retrieval` | `reflection_test.go` | Reflection stages, insight generation |
| `internal/embeddings` | `cache_test.go` | LRU cache, eviction, thread safety |

### Running Unit Tests
```bash
cd mdemg_build/service
go test ./internal/... -v
```

---

## 2) Integration Tests

Full end-to-end tests against a live Neo4j database.

### Location
```
tests/integration/
├── helpers_test.go          # Test utilities, Neo4j setup
├── ingest_test.go           # Ingest pipeline tests
├── retrieval_test.go        # Retrieval and scoring tests
├── scoring_golden_test.go   # Golden tests for scoring validation
├── reflection_test.go       # Reflection endpoint tests
└── anomaly_test.go          # Anomaly detection tests
```

### Prerequisites
- Neo4j running on `bolt://localhost:7687`
- Test database credentials (neo4j/testpassword)
- Build tag: `integration`

### Running Integration Tests
```bash
cd mdemg_build/service

# Run all integration tests
go test -tags=integration -v ./tests/integration/...

# Run specific test
go test -tags=integration -v ./tests/integration/... -run TestScoringGolden

# With timeout
go test -tags=integration -v -timeout 5m ./tests/integration/...
```

---

## 3) Golden Tests (Scoring Validation)

Validate that the scoring algorithm produces expected results for known graph structures.

### Location
`tests/integration/scoring_golden_test.go`

### Test Cases

| Test | Description |
|------|-------------|
| `TestScoringGolden` | 5-node graph with controlled embeddings, validates ranking order |
| `TestScoringGoldenDeterminism` | Verifies identical results across multiple runs |
| `TestScoringComponentBreakdown` | Tests isolated scoring component values |

### Golden Graph Structure
```
     A (v=0.90)     B (v=0.80)
         \           /  |
      0.60\     0.30/   |0.25 (CONTRADICTS)
           \     /      |
            v   v       v
           C (v=0.40)
           /      \
       0.50/    0.20\
         v          v
    D (v=0.20) ---> E (v=0.10)
              0.40
```

### Expected Ranking
A > B > C > D > E (by final score)

### Tolerances
- Vector similarity: ±0.02
- Activation: ±0.05
- Final score: ±0.05

---

## 4) Synthetic Graph Generation

For testing emergent behaviors, generate controlled graph structures.

### Test Patterns

**Clusters:**
- Dense internal edges (weight > 0.5)
- Sparse external edges (weight < 0.2)
- Assert: Query near cluster retrieves cluster members

**Bridges:**
- Few edges connecting clusters
- Assert: Activation traverses bridges only with strong seeding

**Hubs:**
- High degree nodes (20+ edges)
- Assert: Hub penalty prevents hub domination

**Contradictory Pairs:**
- CONTRADICTS edges between nodes
- Assert: Contradicting nodes don't both appear in top-K

---

## 5) Handler Tests

HTTP handler unit tests with mocked dependencies.

### Location
`internal/api/handlers_test.go`

### Coverage
- Request validation
- Response formatting
- Error handling
- Content-type headers

### Example
```go
func TestHandleRetrieve_MissingSpaceID(t *testing.T) {
    req := httptest.NewRequest("POST", "/v1/memory/retrieve",
        strings.NewReader(`{"query_text":"test"}`))
    rec := httptest.NewRecorder()

    server.handleRetrieve(rec, req)

    assert.Equal(t, http.StatusBadRequest, rec.Code)
}
```

---

## 6) Regression Tests: Memory Quality

Maintain frozen test sets to detect algorithm drift.

### Metrics to Track
- **Recall@K**: Are expected nodes in top-K?
- **Novelty Rate**: Unique nodes per query session
- **Redundancy**: Path-prefix overlap in results
- **Activation Spread**: Average nodes with activation > 0.2

### Implementation
```go
// tests/integration/regression_test.go
func TestMemoryQualityRegression(t *testing.T) {
    // Load frozen query/expected pairs
    // Execute queries
    // Compare against baseline metrics
    // Fail if drift > threshold
}
```

---

## 7) Load Tests

Performance benchmarks for capacity planning.

### Metrics

| Metric | Target |
|--------|--------|
| Ingest throughput | > 100 obs/sec (with embedding) |
| Retrieval latency (1K nodes) | < 50ms p99 |
| Retrieval latency (10K nodes) | < 200ms p99 |
| Decay CLI (10K edges) | < 30s |
| Consolidation CLI (1K nodes) | < 60s |

### Running Benchmarks
```bash
go test -bench=. ./internal/retrieval/...
go test -bench=. ./internal/learning/...
```

---

## 8) Test Helpers

### `helpers_test.go` Utilities

| Function | Purpose |
|----------|---------|
| `setupTestNeo4j()` | Create test session, clean database |
| `createTestNode()` | Insert test MemoryNode |
| `createTestEdge()` | Insert test edge with weights |
| `CreateControlledEmbedding()` | Generate embedding with specific similarity |
| `CreateQueryEmbedding()` | Standard query vector [1, 0, 0, ...] |
| `cleanupSpace()` | Delete all nodes in test space |

### Test Isolation
Each test uses a unique `space_id` (e.g., `test-ingest-{uuid}`) to prevent interference.

---

## 9) CI/CD Integration

### GitHub Actions Workflow
```yaml
- name: Run Unit Tests
  run: go test ./internal/... -v

- name: Start Neo4j
  run: docker compose up -d

- name: Wait for Neo4j
  run: sleep 30

- name: Run Integration Tests
  run: go test -tags=integration ./tests/integration/... -v
```

### Required Checks
- All unit tests pass
- All integration tests pass
- No race conditions (`go test -race`)
- Lint clean (`golangci-lint run`)
