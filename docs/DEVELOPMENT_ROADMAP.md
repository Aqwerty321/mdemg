# MDEMG Development Roadmap

**Created**: 2026-01-22
**Based on**: v4 Test Results (whk-wms codebase, 100-question evaluation)
**Goal**: Improve retrieval quality from 0.567 avg score to 0.70+ avg score

---

## Executive Summary

The v4 test validated MDEMG's core hypothesis (100% completion vs 0% baseline), but revealed specific retrieval weaknesses:
- Cross-cutting concerns: 0.45-0.46 scores
- Abstract architecture questions: lower confidence
- Configuration/constants: below average
- Temporal patterns: ~0.46 scores
- Learning edges: 0 created (feature not active)

This roadmap addresses these gaps through 5 improvement tracks.

---

## Priority Matrix

| Track | Impact | Effort | Priority |
|-------|--------|--------|----------|
| Edge Strengthening (Learning) | HIGH | MEDIUM | P0 |
| Cross-Cutting Concern Nodes | HIGH | MEDIUM | P1 |
| Architectural Comparison Nodes | MEDIUM | MEDIUM | P2 |
| Configuration Boosting | MEDIUM | LOW | P2 |
| Temporal Pattern Detection | LOW | MEDIUM | P3 |

---

## Track 1: Edge Strengthening (Learning Edges) ✅ COMPLETE
**Priority**: P0 | **Status**: COMPLETE (v10)

### Problem (Resolved)
`co_activated_edges` remained at 0 during the entire test. The learning mechanism existed but wasn't creating edges.

### Root Cause (Identified)
Only top 2 candidates were seeded with activation values in `activation.go`. Combined with sparse graph connectivity, most returned nodes had zero activation and failed the 0.20 learning threshold.

### Fix Applied
Modified `activation.go` to seed ALL candidates with their VectorSim values:
```go
// Before: only top 2 seeded
// After: all candidates seeded
for _, c := range cands {
    act[c.NodeID] = c.VectorSim
}
```

### Results
| Metric | Before Fix | After Fix | Improvement |
|--------|------------|-----------|-------------|
| Pairs per query | ~0.8 | **43.1** | **54x faster** |
| Edges (cold start) | 0 | **8,622** | ✅ |
| Avg Score | 0.619 | **0.710** | **+14.6%** |

### Documentation
- See `docs/LEARNING_EDGES_ANALYSIS.md` for full diagnosis
- Commit: `3fc2136` - fix(learning): seed all candidates for Hebbian learning activation

---

## Track 2: Cross-Cutting Concern Nodes
**Priority**: P1 | **Estimated Effort**: 3-4 days

### Problem
Questions about ACL, RBAC, authentication, and error-handling scored 0.45-0.46 (below average 0.567).

### Root Cause
Cross-cutting concerns span multiple files/modules but aren't linked in the graph. Individual file embeddings don't capture the "concern" abstraction.

### Implementation Tasks

#### 2.1 Concern Detection During Ingestion
```
Files: mdemg_build/service/cmd/ingest-codebase/
```
- [ ] Detect common cross-cutting patterns:
  - `@Guard`, `@Interceptor`, `@Filter` decorators (NestJS)
  - Files matching `*auth*`, `*acl*`, `*permission*`, `*error*`
  - Import patterns (modules importing auth/acl services)
- [ ] Tag memories with detected concerns

#### 2.2 Concern Node Creation During Consolidation
```
Files: mdemg_build/service/internal/consolidation/
```
- [ ] Create dedicated concern nodes at Layer 1:
  - `authentication`
  - `authorization` / `acl`
  - `error-handling`
  - `logging`
  - `caching`
  - `validation`
- [ ] Link all memories tagged with a concern to its node
- [ ] Generate concern summaries via LLM

#### 2.3 New Edge Types
```
Files: mdemg_build/migrations/
```
- [ ] Add `IMPLEMENTS_CONCERN` edge type
- [ ] Add `SHARES_CONCERN` edge between modules with same concerns

### Success Metrics
- [ ] ACL/RBAC questions score > 0.55
- [ ] Cross-cutting concern queries return concern node + linked memories

---

## Track 3: Architectural Comparison Nodes
**Priority**: P2 | **Estimated Effort**: 3-4 days

### Problem
Questions like "What is the purpose of having both DeltaSyncModule and SyncModule?" scored lower because understanding requires comparing two entities.

### Root Cause
Individual file embeddings don't capture "this module is an alternative to / complement of that module" relationships.

### Implementation Tasks

#### 3.1 Similar Module Detection
```
Files: mdemg_build/service/internal/consolidation/
```
- [ ] During consolidation, identify modules with:
  - Similar names (e.g., `*Sync*`, `*Service*`, `*Module*`)
  - Similar imports/dependencies
  - Similar file structure
- [ ] Use embedding similarity to find related modules

#### 3.2 Comparison Node Generation
- [ ] Create comparison nodes linking similar modules:
  ```
  (DeltaSyncModule)-[:COMPARED_IN]->(comparison_node)<-[:COMPARED_IN]-(SyncModule)
  ```
- [ ] Generate comparison summaries via LLM:
  - "DeltaSyncModule handles X while SyncModule handles Y"
  - Key differences and use cases

#### 3.3 New Edge Types
- [ ] `ALTERNATIVE_TO` - modules that solve similar problems differently
- [ ] `COMPLEMENTS` - modules designed to work together
- [ ] `EXTENDS` - modules that build on others

### Success Metrics
- [ ] "Purpose of both X and Y" questions score > 0.60
- [ ] Comparison queries return comparison node with summary

---

## Track 4: Configuration Boosting
**Priority**: P2 | **Estimated Effort**: 1-2 days

### Problem
Questions about "runtime-config" and "system configurations" scored below average.

### Root Cause
Configuration files are often small, with terse content that doesn't embed as distinctively as implementation code.

### Implementation Tasks

#### 4.1 Configuration Detection
```
Files: mdemg_build/service/cmd/ingest-codebase/
```
- [ ] Detect configuration files:
  - `*.config.ts`, `*.config.js`
  - `config/`, `configuration/` directories
  - Files with `Config` suffix
  - `.env*` files (extract keys, not values)
- [ ] Tag as `config` memory type

#### 4.2 Configuration Boosting
- [ ] Apply score boost (1.2x) to config memories during retrieval
- [ ] Or: increase embedding weight for config content

#### 4.3 Configuration Node Creation
- [ ] Create dedicated `configuration` nodes during consolidation
- [ ] Link config files to the services they configure
- [ ] Generate config summaries: "This module configures X, Y, Z"

#### 4.4 Environment Variable Extraction
- [ ] Parse `.env.example` files
- [ ] Create memories for each env var with description
- [ ] Link env vars to code that uses them

### Success Metrics
- [ ] Configuration questions score > 0.55
- [ ] "What configurations are available" returns config node

---

## Track 5: Temporal Pattern Detection
**Priority**: P3 | **Estimated Effort**: 2-3 days

### Problem
Questions about `validFrom/validTo` temporal patterns scored ~0.46.

### Root Cause
Temporal modeling patterns are domain-specific and not recognized as a cross-cutting concern.

### Implementation Tasks

#### 5.1 Temporal Pattern Recognition
```
Files: mdemg_build/service/cmd/ingest-codebase/
```
- [ ] Detect temporal modeling patterns:
  - `validFrom`, `validTo` fields
  - `createdAt`, `updatedAt`, `deletedAt` (soft delete)
  - Date range queries, temporal joins
  - `effectiveDate`, `expirationDate` patterns
- [ ] Tag memories with `temporal-pattern`

#### 5.2 Temporal Pattern Edge
- [ ] Add `SHARES_TEMPORAL_PATTERN` edge type
- [ ] Link entities that use the same temporal validation logic

#### 5.3 Temporal Documentation Node
- [ ] Create summary node explaining temporal patterns in codebase
- [ ] Link all temporal entities to this node

### Success Metrics
- [ ] Temporal pattern questions score > 0.55
- [ ] "How does validFrom/validTo work" returns pattern documentation

---

## Implementation Phases

### Phase A: Foundation (Week 1)
- [ ] Track 1: Debug and enable learning edges
- [ ] Track 4.1-4.2: Configuration detection and boosting
- [ ] Run v5 test to measure improvement

### Phase B: Concern Nodes (Week 2)
- [ ] Track 2: Full cross-cutting concern implementation
- [ ] Track 3.1: Similar module detection
- [ ] Run v6 test to measure improvement

### Phase C: Advanced Relationships (Week 3)
- [ ] Track 3.2-3.3: Comparison nodes and edge types
- [ ] Track 5: Temporal pattern detection
- [ ] Run v7 test with full improvements

### Phase D: Validation (Week 4)
- [ ] Benchmark on second codebase (different domain)
- [ ] Scale test: 10K → 100K nodes
- [ ] Document final architecture

---

## Phase 6: Modular Intelligence & Active Participation

This phase transforms MDEMG into a plug-and-play cognitive engine. **Tracks 1-5 (Phases A-C) are foundational** and should proceed in parallel or prior to Phase 6.

### Deliverable 6.1: Jiminy (Explainable Retrieval)
- **Priority**: P0 | **Effort**: 2-3 days
- [ ] Update `/v1/memory/retrieve` to return `jiminy` block with rationale and confidence.
- [ ] Trace retrieval path (Vector → Spreading Activation → LLM Rerank) for explanation.
- [ ] Add `score_breakdown` showing contribution from each scoring component.
- [ ] **Integration**: Connect with the v9 LLM re-ranker to explain *why* specific results were promoted.

### Deliverable 6.2: Binary Plugin Host
- **Priority**: P1 | **Effort**: 4-5 days
- [ ] Finalize `mdemg-module.proto` with lifecycle RPCs (Handshake, HealthCheck, Shutdown).
- [ ] Implement **Plugin Manager** in Go:
  - Scan `/plugins` directory for module folders
  - Parse `manifest.json` for each module
  - Spawn binaries with Unix socket paths
  - Maintain health check loops
  - Restart crashed modules with backoff
- [ ] Create "echo" test module to validate RPC round-trip latency.
- [ ] Document module development guide with build instructions.

### Deliverable 6.3: Linear Integration Module (First Non-Code Module)
- **Priority**: P1 | **Effort**: 4-5 days
- [ ] Implement Linear API client (Go binary).
- [ ] Create `IngestionModule` that:
  - Fetches issues, tasks, projects from Linear workspace
  - Extracts engineering decisions, blockers, dependencies
  - Creates observation nodes with appropriate metadata
- [ ] Define Linear-specific edge types:
  - `BLOCKS` / `BLOCKED_BY` (task dependencies)
  - `ASSIGNED_TO` (ownership tracking)
  - `RELATES_TO` (cross-references)
- [ ] Incremental sync: track last sync cursor, fetch only new/updated items.

### Deliverable 6.4: Code Parser Module Migration
- **Priority**: P2 | **Effort**: 3-4 days
- [ ] Extract existing Go/TS parsers from `ingest-codebase` into standalone module.
- [ ] Refactor `ingest-codebase` to use RPC-based ingestion.
- [ ] Benchmark: RPC overhead vs direct call (target: <5ms added latency).

### Deliverable 6.5: The "Internal Dialog" Participant (APE)
- **Priority**: P2 | **Effort**: 5-7 days
- [ ] Implement background Consistency Checker (APEModule).
- [ ] Implement **General Reflection Module**: Periodically synthesizes "Internal Dialog" summaries from all observation sources.
- [ ] Implement **Constraint Module**: Detects non-code commitments (e.g., "Always use metric units in Whiskey House") and tags them as high-priority constraints.

---

## Testing Strategy

### Regression Test Suite
Create dedicated test questions for each improvement:
- 10 questions per track (50 total)
- Measure before/after scores
- Track improvement percentage

### Test Questions by Track

**Track 1 (Learning Edges)**:
- Repeat the same question 5 times, measure if retrieval improves

**Track 2 (Cross-Cutting)**:
- "How does authentication work across the application?"
- "What modules use the ACL interceptor?"
- "How are errors handled globally?"

**Track 3 (Comparison)**:
- "What's the difference between ServiceA and ServiceB?"
- "Why are there two sync modules?"
- "Compare the validation approaches in X and Y"

**Track 4 (Configuration)**:
- "What environment variables are required?"
- "How is the database configured?"
- "What runtime configurations exist?"

**Track 5 (Temporal)**:
- "How does temporal ownership work?"
- "What entities use validFrom/validTo?"
- "How are date ranges validated?"

---

## Migration Path

### Schema Changes (V0006)
```cypher
// New edge types
CREATE CONSTRAINT concern_edge IF NOT EXISTS
FOR ()-[r:IMPLEMENTS_CONCERN]-() REQUIRE r.concern IS NOT NULL;

CREATE CONSTRAINT comparison_edge IF NOT EXISTS
FOR ()-[r:COMPARED_IN]-() REQUIRE r.created_at IS NOT NULL;

// New node labels
// :ConcernNode - cross-cutting concern abstractions
// :ComparisonNode - architectural comparisons
// :ConfigurationNode - configuration summaries
```

### Backward Compatibility
- All changes are additive (new edge types, new node types)
- Existing queries continue to work
- New consolidation creates new nodes alongside existing structure

---

## Success Criteria

### Quantitative

| Metric | Baseline (v4) | v9 Rerank | v10 Learning | Target |
|--------|---------------|-----------|--------------|--------|
| Average retrieval score | 0.567 | 0.619 | **0.710 (+14.6%)** | 0.75+ |
| Cross-cutting questions | 0.45 | 0.52 | 0.59 | 0.70+ |
| Architecture questions | ~0.50 | 0.58 | 0.65 | 0.72+ |
| Configuration questions | ~0.45 | 0.54 | 0.61 | 0.68+ |
| Temporal questions | 0.46 | 0.49 | 0.55 | 0.62+ |
| Learning edges created | 0 | 0 | **8,622** | ✅ ACHIEVED |
| Score >0.7 rate | ~10% | ~25% | **64%** | 70%+ |

### Qualitative
- [ ] Retrieval returns concern/comparison nodes when appropriate
- [ ] Generated summaries are accurate and useful
- [ ] No regression in high-performing categories (data flow: 0.75)

---

## Appendix: File Locations

### Core Files to Modify
```
mdemg_build/
├── service/
│   ├── cmd/ingest-codebase/     # Track 2.1, 4.1, 5.1
│   │   └── main.go              # Add pattern detection
│   └── internal/
│       ├── consolidation/        # Track 2.2, 3.2, 4.3, 5.3
│       │   └── service.go        # Add concern/comparison node creation
│       ├── learning/             # Track 1
│       │   └── service.go        # Debug/enable edge creation
│       └── retrieval/            # Track 4.2
│           └── scoring.go        # Add config boost
└── migrations/
    └── V0006__improvement_tracks.cypher  # New schema
```

### Test Files
```
docs/tests/
├── track_1_learning_questions.json
├── track_2_concern_questions.json
├── track_3_comparison_questions.json
├── track_4_config_questions.json
└── track_5_temporal_questions.json
```
