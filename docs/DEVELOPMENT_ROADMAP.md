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

## Track 1: Edge Strengthening (Learning Edges)
**Priority**: P0 | **Estimated Effort**: 2-3 days

### Problem
`co_activated_edges` remained at 0 during the entire test. The learning mechanism exists but isn't creating edges during retrieval.

### Root Cause Analysis
- [ ] Verify learning edge creation code path in `learning/` package
- [ ] Check if edge creation is gated by a feature flag or threshold
- [ ] Review `LEARNING_EDGE_CAP_PER_REQUEST` setting (default: 200)

### Implementation Tasks

#### 1.1 Debug Learning Edge Creation
```
Files: mdemg_build/service/internal/learning/
```
- [ ] Add logging to edge creation code path
- [ ] Verify co-retrieval detection logic
- [ ] Test with explicit co-retrieval scenarios

#### 1.2 Implement Co-Retrieval Tracking
- [ ] Track which memories are returned together in the same query
- [ ] Create `CO_ACTIVATED_WITH` edges when nodes are co-retrieved N times
- [ ] Configure threshold (suggest: 3 co-retrievals = edge creation)

#### 1.3 Edge Weight Decay
- [ ] Implement time-based weight decay for learning edges
- [ ] Prevent stale associations from dominating retrieval

### Success Metrics
- [ ] `co_activated_edges > 0` after 100 queries
- [ ] Improved retrieval consistency for repeated query patterns

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
| Metric | Current | Target |
|--------|---------|--------|
| Average retrieval score | 0.567 | 0.70+ |
| Cross-cutting questions | 0.45 | 0.60+ |
| Architecture questions | ~0.50 | 0.65+ |
| Configuration questions | ~0.45 | 0.60+ |
| Temporal questions | 0.46 | 0.55+ |
| Learning edges created | 0 | 50+ per 100 queries |

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
