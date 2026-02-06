# MDEMG Agent Handoff Document

**Date:** 2026-02-06
**Branch:** `mdemg-dev01`
**Repository:** `/Users/reh3376/mdemg`
**Purpose:** Complete context for continuing development of the MDEMG framework

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Architecture Summary](#2-architecture-summary)
3. [Environment Setup](#3-environment-setup)
4. [Phase Numbering Convention](#4-phase-numbering-convention)
5. [Phase Registry](#5-phase-registry)
6. [Completed Phases (31-33)](#6-completed-phases-31-33)
7. [In-Progress Phases (34+)](#7-in-progress-phases-34)
8. [Planned Phases (35-40)](#8-planned-phases-35-40)
9. [Core Infrastructure Phases (41-52)](#9-core-infrastructure-phases-41-52)
10. [Governance & Testing Frameworks](#10-governance--testing-frameworks)
11. [File Inventory by Domain](#11-file-inventory-by-domain)
12. [Development Principles](#12-development-principles)
13. [Known Issues & Technical Debt](#13-known-issues--technical-debt)
14. [Quick Reference Commands](#14-quick-reference-commands)

---

## 1. Project Overview

**MDEMG** (Multi-Dimensional Emergent Memory Graph) is a long-term memory system for AI agents, built on Neo4j with native vector indexes. It implements a retrieval-augmented memory graph with spreading activation and Hebbian learning.

### Core Purpose

MDEMG provides AI agents with the **ANN equivalent of human internal dialog** — persistent cognitive context that survives across sessions. It stores:

- **Task History** — Decisions made, problems solved, work performed
- **SME Domain Knowledge** — Organization-specific procedures, institutional memory, tribal knowledge

It does **NOT** store general knowledge that LLMs already possess.

### Read First (in order)

| Document | Path | Purpose |
|----------|------|---------|
| Vision | `VISION.md` | Core purpose, architecture philosophy, emergent layer design |
| Architecture | `CLAUDE.md` | Commands, directory structure, environment variables, retrieval pipeline |
| Development Roadmap | `docs/development/DEVELOPMENT_ROADMAP.md` | Feature tracks, benchmarks, retrieval improvements (v4→v11) |
| API Reference | `docs/development/API_REFERENCE.md` | All HTTP endpoints (1,268 lines) |
| Collaboration Plan | `docs/specs/development-space-collaboration.md` | Master plan for DevSpace phases (the Space Transfer pipeline) |

### Technical Invariants (Do NOT Violate)

- **Vector index = recall** (fast candidate generation)
- **Graph = reasoning** (typed edges with evidence)
- **Runtime = activation physics** (spreading activation computed in-memory, NEVER persisted)
- **DB writes = learning deltas only** (bounded, no per-request activation writes)

---

## 2. Architecture Summary

### Technology Stack

| Component | Technology | Notes |
|-----------|-----------|-------|
| Graph DB | Neo4j 5.x | Docker: `docker compose up -d` |
| Backend | Go (latest stable) | Service at `cmd/server/main.go` |
| gRPC | Protocol Buffers | `api/proto/*.proto` |
| Embeddings | OpenAI `text-embedding-3-small` (1536d) / Ollama (768d) | Configurable |
| Plugins | Binary sidecar via gRPC Unix sockets | `plugins/*/` |

### Directory Structure

```
api/
  proto/                    # Proto definitions
    mdemg-module.proto      # Plugin/module protocol
    space-transfer.proto    # Space transfer service
    devspace.proto          # DevSpace hub + messaging
  modulepb/                 # Generated Go (mdemg-module)
  transferpb/               # Generated Go (space-transfer)
  devspacepb/               # Generated Go (devspace)
cmd/
  server/                   # Main MDEMG server
  mcp-server/               # MCP tool server for IDEs
  ingest-codebase/          # Codebase ingestion CLI
  consolidate/              # Consolidation CLI
  decay/                    # Edge weight decay CLI
  space-transfer/           # Space transfer CLI (export/import/serve/pull)
  reset-db/                 # DB cleanup tool
internal/
  api/                      # HTTP handlers + middleware
  anomaly/                  # Anomaly detection on ingest
  ape/                      # Active Participant Engine scheduler
  config/                   # Environment-based configuration
  consulting/               # Agent consulting service
  conversation/             # CMS (observe, recall, resume, correct)
  db/                       # Neo4j driver + schema validation
  devspace/                 # DevSpace hub (catalog, broker, server)
  domain/                   # Domain types
  embeddings/               # Embedding clients (OpenAI/Ollama)
  gaps/                     # Capability gap detection
  hidden/                   # Hidden layer abstraction/consolidation
  jobs/                     # Background job tracking
  learning/                 # Hebbian learning (CO_ACTIVATED_WITH)
  models/                   # Request/response types
  observations/             # Observation service
  plugins/                  # Plugin manager + scaffold
  retrieval/                # Core retrieval pipeline (vector + activation + scoring + cache)
  summarize/                # LLM summary service
  symbols/                  # Symbol extraction (tree-sitter)
  transfer/                 # Space Transfer (exporter, importer, format, validate, grpc_server)
  validation/               # Request validation
plugins/
  linear-module/            # Linear integration plugin
  reflection-module/        # APE reflection plugin
  keyword-booster/          # Sample reasoning plugin
migrations/                 # Neo4j Cypher migrations (V0001-V0011)
tests/
  integration/              # Integration tests (Neo4j required)
  udts/                     # UDTS contract tests (gRPC)
docs/
  specs/                    # Feature specifications (per-phase)
  architecture/             # Architecture docs (00-14 numbered)
  development/              # Dev guides, roadmap, API reference
  api/api-spec/             # UATS + UDTS specs, schemas, runners
  lang-parser/              # UPTS parser specs (25 languages)
  research/                 # Research papers (GAT, edge attention, etc.)
  benchmarks/               # Benchmark results and scripts
```

### Graph Schema (Core Labels)

| Label | Purpose |
|-------|---------|
| `:TapRoot` | Singleton per `space_id` |
| `:MemoryNode` | Main memory nodes with embeddings (1536-dim default) |
| `:Observation` | Append-only events linked to MemoryNodes |
| `:SymbolNode` | Extracted code symbols (constants, functions, classes) |
| `:SchemaMeta` | Schema version tracking |
| `:CapabilityGap` | Identified retrieval gaps |
| `:InterviewPrompt` | Gap interview prompts |

### Key Relationship Types

| Type | Category | Description |
|------|----------|-------------|
| `ASSOCIATED_WITH` | Associative | Semantic relationship |
| `CO_ACTIVATED_WITH` | Learned | Hebbian-strengthened co-activation |
| `CAUSES`, `ENABLES` | Causal | Causal chains |
| `TEMPORALLY_ADJACENT` | Temporal | Time proximity |
| `ABSTRACTS_TO`, `INSTANTIATES` | Hierarchy | Layer abstraction |
| `HAS_OBSERVATION` | Structural | Node → observation link |
| `DEFINED_IN` | Symbol | Symbol → file link |
| `IMPLEMENTS_CONCERN` | Cross-cutting | Node → concern node |
| `COMPARED_IN` | Comparison | Module → comparison node |
| `IMPLEMENTS_CONFIG` | Config | File → config summary |
| `GENERALIZES` | Hierarchy | Hidden layer generalization |

### Retrieval Pipeline (`internal/retrieval/service.go`)

1. **Vector recall** — Query `memNodeEmbedding` vector index for top-K candidates
2. **Symbol search** — Pattern-match query for symbol names (exact, prefix, fuzzy)
3. **Bounded expansion** — Iterative 1-hop fetch with caps (max depth=3, per-node limit)
4. **Spreading activation** — In-memory computation with decay
5. **Scoring + ranking** — Combine vector similarity (α=0.55), activation (β=0.30), recency (γ=0.10), confidence (δ=0.05), hub penalty (φ=0.08), redundancy (κ=0.12)
6. **Caching** — TTL-LRU cache (98.9% latency improvement on repeated queries)

---

## 3. Environment Setup

### Start Neo4j

```bash
docker compose up -d
# Browser: http://localhost:7474 (neo4j/testpassword)
```

### Apply Migrations

```bash
for f in migrations/V*.cypher; do
  echo "Applying $f"
  docker exec -i mdemg-neo4j cypher-shell -u neo4j -p testpassword < "$f"
done
```

### Run the Go Service

```bash
cd /Users/reh3376/mdemg
export NEO4J_URI=bolt://localhost:7687
export NEO4J_USER=neo4j
export NEO4J_PASS=testpassword
export REQUIRED_SCHEMA_VERSION=4
export VECTOR_INDEX_NAME=memNodeEmbedding
go run ./cmd/server
```

### Run Tests

```bash
# Unit tests
go test ./internal/... -v

# Integration tests (Neo4j must be running)
go test -tags=integration ./tests/integration/... -v

# UDTS contract tests (server must be running on port 50051/50052)
UDTS_TARGET=localhost:50052 go test ./tests/udts/... -v

# Full build check
go build ./... && go vet ./...
```

### Run Space Transfer

```bash
# Export a space to file
go run ./cmd/space-transfer export -space-id demo -output demo.mdemg

# Serve gRPC for remote pulls (+ DevSpace hub)
go run ./cmd/space-transfer serve -port 50052 -enable-devspace -devspace-data-dir ./devspace-data

# Pull from remote
go run ./cmd/space-transfer pull -remote localhost:50052 -space-id demo -output demo.mdemg
```

### Environment Variables

Full list in `CLAUDE.md` — key ones:

| Variable | Default | Description |
|----------|---------|-------------|
| `NEO4J_URI` | required | Bolt connection |
| `NEO4J_USER` / `NEO4J_PASS` | required | Auth |
| `REQUIRED_SCHEMA_VERSION` | required | Must match latest migration |
| `VECTOR_INDEX_NAME` | `memNodeEmbedding` | Vector index name |
| `SCORING_ALPHA` | 0.55 | Vector similarity weight |
| `SCORING_BETA` | 0.30 | Activation weight |
| `QUERY_CACHE_ENABLED` | true | Result caching toggle |
| `QUERY_CACHE_TTL_SECONDS` | 300 | Cache TTL |

---

## 4. Phase Numbering Convention

Phases are organized into **numbered series** to group related work:

| Series | Range | Domain |
|--------|-------|--------|
| **30s** | 31-40 | **Space Transfer & DevSpace Collaboration** — The multi-agent collaboration pipeline |
| **40s** | 41-43 | **Core Engine** — Original infrastructure phases (cleanup, self-ingest, CMS) |
| **50s** | 44-52 | **Advanced Features** — Modular intelligence, symbols, incremental updates, caching, LLM SDK, public readiness |

### Mapping from Old to New

| Old Phase # | New Phase # | Name | Status |
|-------------|-------------|------|--------|
| Phase 1 (Space Transfer) | **Phase 31** | Space Transfer | ✅ Complete |
| Phase 2 (DevSpace Hub) | **Phase 32** | DevSpace Hub + Out-of-Band Distribution | ✅ Complete |
| Phase 3 (Inter-Agent Comms) | **Phase 33** | Inter-Agent Communications | ✅ Complete |
| Phase 4 (Incremental Sync) | **Phase 34** | Incremental Sync (Delta Export) | ✅ Complete |
| Phase 5 (CRDT + Lineage) | **Phase 35** | CRDT for Learned Edges + Space Lineage | 📋 Planned |
| Phase 7 (Observation Forwarding) | **Phase 36** | Selective Observation Forwarding (CMS) | 📋 Planned |
| Phase 8 (Agent Health) | **Phase 37** | Agent Health / Heartbeat / Presence | 📋 Planned |
| — (UNTS) | **Phase 38** | Hash Verification (UNTS / Nash Verification) | 📋 Spec Complete |
| Phase 1 (Cleanup) | **Phase 41** | Space Cleanup | ✅ Complete |
| Phase 2 (Self-Ingest) | **Phase 42** | Self-Ingest MDEMG Codebase | ✅ Complete |
| Phase 3A (CMS Enforcement) | **Phase 43A** | CMS Agent Enforcement | ✅ Complete |
| Phase 3B (CMS Quality) | **Phase 43B** | CMS Quality & Retrieval Improvements | ✅ Complete |
| Phase 3C (Multi-Agent CMS) | **Phase 43C** | Multi-Agent CMS Support | ✅ Complete |
| Phase 4 (Linear CRUD) | **Phase 44** | Linear Integration — Full CRUD + Workflows | 📋 Approved (not started) |
| Phase 6 (Modular Intelligence) | **Phase 45** | Modular Intelligence & Active Participation | 🔄 Partial |
| Phase 8 (Symbols) | **Phase 46** | Symbol-Level Indexing | ✅ Complete (8.5-8.6 archived) |
| Phase 9 (Incremental Updates) | **Phase 47** | Incremental Update & Re-Ingestion | 🔄 Partial |
| Phase 10 (Query Optimization) | **Phase 48** | Query Optimization & Caching | ✅ Complete (10.1-10.2) |
| Phase 11 (LLM SDK) | **Phase 49** | LLM Plugin SDK & Self-Improvement | 🔄 Partial |
| Phase 7 (Public Readiness) | **Phase 50** | Public Readiness & Open Source Hardening | 📋 Planned |

---

## 5. Phase Registry

### Status Legend

| Icon | Meaning |
|------|---------|
| ✅ | Complete — implemented, tested, verified |
| 🔄 | In Progress — partially implemented |
| 📋 | Planned — spec exists, no implementation |
| 📦 | Archived — deferred or superseded |

### Quick Status Table

| Phase | Name | Status | Spec File |
|-------|------|--------|-----------|
| 31 | Space Transfer | ✅ | `docs/specs/space-transfer.md` |
| 32 | DevSpace Hub | ✅ | `docs/specs/phase-devspace-hub.md` |
| 33 | Inter-Agent Comms | ✅ | `docs/specs/phase3-inter-agent-comms.md` |
| 34 | Incremental Sync | ✅ | `docs/specs/phase4-incremental-sync.md` |
| 35 | CRDT + Lineage | 📋 | `docs/specs/development-space-collaboration.md` §Phase 5 |
| 36 | Observation Forwarding | 📋 | `docs/specs/development-space-collaboration.md` §Phase 7 |
| 37 | Agent Health / Presence | 📋 | `docs/specs/development-space-collaboration.md` §Phase 8 |
| 38 | UNTS Hash Verification | 📋 | `docs/specs/unts-hash-verification.md` |
| 41 | Space Cleanup | ✅ | `docs/specs/phase1-space-cleanup.md` |
| 42 | Self-Ingest | ✅ | `docs/specs/phase2-self-ingest.md` |
| 43A | CMS Enforcement | ✅ | `docs/specs/phase3a-cms-enforcement.md` |
| 43B | CMS Quality | ✅ | `docs/specs/phase3b-cms-quality.md` |
| 43C | Multi-Agent CMS | ✅ | `docs/specs/phase3c-multi-agent.md` |
| 44 | Linear CRUD | 📋 | `docs/specs/phase4-linear-crud.md` |
| 45 | Modular Intelligence | 🔄 | `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 6 |
| 46 | Symbol Indexing | ✅ | `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 8 |
| 47 | Incremental Updates | 🔄 | `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 9 |
| 48 | Query Optimization | ✅ | `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 10 |
| 49 | LLM Plugin SDK | 🔄 | `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 11 |
| 50 | Public Readiness | 📋 | `docs/development/repo-to-public-roadmap.md` |

---

## 6. Completed Phases (31-33)

### Phase 31: Space Transfer ✅

**Spec:** `docs/specs/space-transfer.md`
**Master Plan:** `docs/specs/development-space-collaboration.md` §Phase 1

**What it does:** Enables sharing mature MDEMG space_id graphs between developer environments via gRPC streaming or file export/import.

**Key files:**

| File | Purpose |
|------|---------|
| `api/proto/space-transfer.proto` | gRPC service definition (Export, Import, ListSpaces, SpaceInfo) |
| `api/transferpb/*.pb.go` | Generated Go code |
| `internal/transfer/exporter.go` | Neo4j → chunks (with ProgressFunc, delta support) |
| `internal/transfer/importer.go` | Chunks → Neo4j (skip/overwrite/error conflict modes) |
| `internal/transfer/format.go` | File I/O (`.mdemg` JSON format) |
| `internal/transfer/validate.go` | Schema version validation |
| `internal/transfer/grpc_server.go` | gRPC SpaceTransfer server |
| `internal/transfer/format_test.go` | Unit tests (round-trip, embeddings, ExportFromRequest, Phase 34 delta) |
| `cmd/space-transfer/main.go` | CLI (export, import, list, info, serve, pull, profiles, git check) |
| `tests/integration/transfer_test.go` | Integration tests |
| `tests/udts/contract_test.go` | UDTS contract tests (ListSpaces, SpaceInfo, ExportDelta) |
| `docs/api/api-spec/udts/specs/space_transfer_*.udts.json` | UDTS specs |

**Capabilities:**
- File export/import with `.mdemg` format
- gRPC streaming (serve/pull)
- Export profiles: `full`, `codebase`, `cms`, `learned`, `metadata`
- Conflict modes: `skip`, `overwrite`, `error`
- Progress reporting, pre-export git check
- Schema version validation

---

### Phase 32: DevSpace Hub + Out-of-Band Distribution ✅

**Spec:** `docs/specs/phase-devspace-hub.md`
**Master Plan:** `docs/specs/development-space-collaboration.md` §Phase 2

**What it does:** Named collaboration groups ("DevSpaces") with registered agents. Agents publish exports to the hub; other members list and pull exports.

**Key files:**

| File | Purpose |
|------|---------|
| `api/proto/devspace.proto` | DevSpace service (RegisterAgent, ListExports, PullExport, Connect) |
| `api/devspacepb/*.pb.go` | Generated Go code |
| `internal/devspace/catalog.go` | In-memory catalog (agents, exports) |
| `internal/devspace/server.go` | gRPC DevSpace server |
| `internal/devspace/broker.go` | Message broker for inter-agent messaging (Phase 33) |
| `cmd/space-transfer/main.go` | `-enable-devspace` flag, `-devspace-data-dir` |
| `docs/api/api-spec/udts/specs/devspace_*.udts.json` | UDTS specs (register_agent, list_exports, pull_export) |

**RPCs:** `RegisterAgent`, `DeregisterAgent`, `ListExports`, `PublishExport`, `PullExport`

---

### Phase 33: Inter-Agent Communications ✅

**Spec:** `docs/specs/phase3-inter-agent-comms.md`
**Master Plan:** `docs/specs/development-space-collaboration.md` §Phase 3

**What it does:** Bidirectional gRPC streaming for agent-to-agent messaging within a DevSpace. Agents connect to the hub and exchange `AgentMessage` payloads (context, bugs, notifications).

**Key files:**

| File | Purpose |
|------|---------|
| `api/proto/devspace.proto` | `Connect(stream AgentMessage) returns (stream AgentMessage)` |
| `internal/devspace/broker.go` | In-memory message broker; routes by `dev_space_id` + optional `topic` |
| `internal/devspace/server.go` | `Connect` handler |
| `docs/api/api-spec/udts/specs/devspace_connect.udts.json` | UDTS spec |
| `tests/udts/contract_test.go` | `TestDevSpaceConnect` |

---

## 7. Recently Completed Phases

### Phase 34: Incremental Sync (Delta Export) ✅

**Completed:** 2026-02-06
**Spec:** `docs/specs/phase4-incremental-sync.md`
**Master Plan:** `docs/specs/development-space-collaboration.md` §Phase 4

**What it does:** Export/import only changes since a given timestamp or cursor, reducing payload for frequent syncs.

**All tasks complete:**
- [x] Proto: `ExportRequest` extended with `since_timestamp` (field 9) and `since_cursor` (field 10)
- [x] Proto: `TransferSummary` extended with `next_cursor` (field 8)
- [x] Exporter: All `fetch*Batch` functions filter by `updated_at`/`created_at`/`timestamp` when `since` is set
- [x] Exporter: `countEntities` filters by since for accurate delta counts
- [x] Exporter: Summary chunk sets `next_cursor = completedAt` for delta exports
- [x] CLI: `-since-timestamp` and `-since-cursor` flags; prints "Next cursor for delta" to stderr
- [x] Unit test: `TestExportFromRequest_Phase4Delta` (passes)
- [x] Integration test: `TestTransferDeltaExport` (passes)
- [x] UDTS spec: `space_transfer_export_delta.udts.json` (added)
- [x] UDTS test: `TestSpaceTransferExportDelta` (added)
- [x] Import idempotency verified: Uses MERGE for nodes/edges (no duplicates)
- [x] Run UDTS test against live server: 7/7 tests pass
- [x] User verification of delta export/import end-to-end

**Key files:**

| File | Purpose |
|------|---------|
| `internal/transfer/exporter.go` | Delta filtering in `countEntities`, `fetchNodeBatch`, `fetchEdgeBatch`, `fetchObservationBatch`, `fetchSymbolBatch`; `NextCursor` in summary |
| `internal/transfer/importer.go` | Idempotent MERGE for nodes (node_id) and edges (relationship keys) |
| `internal/transfer/format_test.go` | `TestExportFromRequest_Phase4Delta` |
| `tests/integration/transfer_test.go` | `TestTransferDeltaExport` |
| `tests/udts/contract_test.go` | `TestSpaceTransferExportDelta` |
| `docs/api/api-spec/udts/specs/space_transfer_export_delta.udts.json` | UDTS spec |
| `cmd/space-transfer/main.go` | `-since-timestamp`, `-since-cursor` flags |

---

### UOBS: Embedding Health Monitor ✅

**Added:** 2026-02-06

Extended the UOBS (Universal Observability Specification) framework to include embedding model health monitoring with active probe validation.

**Components:**

| Component | Path | Description |
|-----------|------|-------------|
| Schema | `docs/tests/uobs/schema/uobs.schema.json` | Added "dependency" test type |
| Spec | `docs/tests/uobs/specs/embedding_health.uobs.json` | Embedding health validation spec |
| Handler | `internal/api/handlers.go` | `handleEmbeddingHealth()` function |
| Runner | `docs/tests/uobs/runners/uobs_runner.py` | Added `run_dependency_test()` |

**API Endpoint: `GET /v1/embedding/health`**

Returns embedding provider health status with active probe validation.

```json
{
  "status": "healthy",
  "provider": "openai",
  "model": "text-embedding-ada-002",
  "dimensions": 1536,
  "latency_ms": 923,
  "cache_enabled": true,
  "success_rate_24h": 100,
  "error_count_24h": 0,
  "circuit_breaker": "closed",
  "configured_env_var": true
}
```

**Health Checks (8 total):**
- `embedding_connectivity` — Endpoint reachable
- `embedding_status` — Status is healthy/degraded
- `embedding_active_probe` — Actually generates embedding
- `embedding_latency_threshold` — Latency <= 2000ms
- `embedding_success_rate` — Success rate >= 99%
- `embedding_error_rate` — Error rate <= 1%
- `embedding_configuration` — Env vars and dimensions valid
- `embedding_circuit_breaker` — Circuit breaker closed

---

## 8. Planned Phases (35-40)

### Phase 35: CRDT for Learned Edges + Space Lineage 📋

**Master Plan:** `docs/specs/development-space-collaboration.md` §Phase 5

**Goal:** CO_ACTIVATED_WITH edges merge with CRDT semantics (e.g. max weight, sum evidence_count) so concurrent updates from multiple agents don't lose data. Space lineage tracks origin, merges, and who shared what.

**Deliverables:**
- Define merge rules for CO_ACTIVATED_WITH (max weight, sum evidence_count)
- Implement in importer when conflict mode is "merge" or new "crdt" mode
- Proto fields for lineage (e.g. `SpaceMetadata.lineage`)
- Exporter records origin; importer appends to lineage on merge
- Tests for CRDT merge behavior and lineage round-trip

**Dependencies:** Phase 31 (Space Transfer). Phase 34 helpful for delta + CRDT together.

---

### Phase 36: Selective Observation Forwarding (CMS) 📋

**Master Plan:** `docs/specs/development-space-collaboration.md` §Phase 7

**Goal:** Agents mark observations as "team-visible" or forward selected observations into a shared DevSpace feed.

**Deliverables:**
- Proto: `ForwardObservation` or extend CMS observe with `visibility: team` and DevSpace target
- Implementation: store/route observations to DevSpace feed; recall filters by visibility
- UDTS specs and tests

**Dependencies:** Phase 32 (DevSpace) and existing CMS (Phase 43A-C).

---

### Phase 37: Agent Health / Heartbeat / Presence 📋

**Master Plan:** `docs/specs/development-space-collaboration.md` §Phase 8

**Goal:** Agents in a DevSpace have online/away status via heartbeat. Optional offline queue for disconnected agents.

**Deliverables:**
- Proto: `Heartbeat`, `GetPresence` (or `ListAgents` with status), optional `SetQueueSize`
- Hub stores `last_heartbeat` per agent; presence endpoint
- Optional bounded queue per agent
- UDTS specs and tests

**Dependencies:** Phase 32 (registration).

---

### Phase 38: UNTS Hash Verification (Nash Verification Module) 📋

**Spec:** `docs/specs/unts-hash-verification.md`
**Governance:** `docs/specs/FRAMEWORK_GOVERNANCE.md`

**Goal:** Central registry + API for hash verification of all framework-protected files (manifest, UDTS proto_sha256, future UATS/UBTS/USTS/UOTS/UAMS/UPTS hashes). Current + historical (last 3) hashes per file. Revert capability. gRPC/REST for monitoring and manipulation.

**Deliverables:**
- Registry: `docs/specs/unts-registry.json` (or DB)
- Scanners: Ingest from `manifest.sha256` and UDTS spec `proto_sha256` fields
- Core logic: VerifyNow, UpdateHash, RevertToPreviousHash
- gRPC service: `mdemg.unts.v1.HashVerification` (or `NashVerification`)
  - RPCs: ListVerifiedFiles, GetFileStatus, GetHashHistory, RevertToPreviousHash, UpdateHash, VerifyNow, RegisterTrackedFile
- UDTS specs for UNTS RPCs
- Implementation: `internal/unts/` (or `internal/hashverify/`), `api/proto/unts.proto`

**Data model:** See spec for `VerifiedFileRecord` (path, framework, current_hash, status, updated_at, history[3]).

**Dependencies:** None. Governance-level capability.

---

## 9. Core Infrastructure Phases (41-52)

### Phase 41: Space Cleanup ✅

**Spec:** `docs/specs/phase1-space-cleanup.md`

Cleared 570,436 non-protected nodes from Neo4j, preserving only `mdemg-dev` (2,789 nodes). Used `go run ./cmd/reset-db --all --yes`.

---

### Phase 42: Self-Ingest MDEMG Codebase ✅

**Spec:** `docs/specs/phase2-self-ingest.md`

Ingested MDEMG codebase into `mdemg-codebase` space (1,561 elements, 0 errors, 100% embedding coverage). Added optional `space_id` parameter to all MCP tools in `cmd/mcp-server/main.go`.

---

### Phase 43A: CMS Agent Enforcement ✅

**Spec:** `docs/specs/phase3a-cms-enforcement.md`

**What it does:** Tracks per-session CMS usage, exposes session health scores, warns when agents skip resume.

**Key files:**
- `internal/conversation/session_tracker.go` — SessionState, SessionTracker (sync.Map, TTL cleanup)
- `internal/conversation/session_tracker_test.go` — 6 test functions
- `internal/api/middleware.go` — SessionResumeWarningMiddleware

**Endpoint:** `GET /v1/conversation/session/health?session_id=X`
**Warning header:** `X-MDEMG-Warning: session-not-resumed`

---

### Phase 43B: CMS Quality & Retrieval Improvements ✅

**Spec:** `docs/specs/phase3b-cms-quality.md`

**What it does:** Multi-factor observation quality scoring, relevance-weighted resume ranking, near-duplicate detection (cosine similarity > 0.95).

**Key files:**
- `internal/conversation/quality.go` — Specificity, actionability, context-richness scoring
- `internal/conversation/dedup.go` — Cosine similarity dedup
- `internal/conversation/bench_test.go` — 8 benchmark tests
- `internal/conversation/service.go` — Relevance-weighted resume query

**Resume ranking formula:**
```
relevanceScore = 0.40 * recencyScore + 0.25 * surpriseScore + 0.20 * typePriority + 0.15 * coactivationScore
```

---

### Phase 43C: Multi-Agent CMS Support ✅

**Spec:** `docs/specs/phase3c-multi-agent.md`

**What it does:** Persistent `agent_id` on all CMS operations (survives across sessions). Agent isolation (private obs), team visibility, cross-session resume.

**Key files:**
- `migrations/V0011__agent_identity.cypher` — Neo4j indexes for agent_id
- `internal/conversation/multi_agent_test.go` — 11 test functions
- `internal/conversation/types.go` — AgentID on Observation
- `internal/conversation/service.go` — Agent filtering, cross-session resume

---

### Phase 44: Linear Integration — Full CRUD + Workflows 📋

**Spec:** `docs/specs/phase4-linear-crud.md`

**What it does:** Full CRUD operations for Linear (create/read/update/delete issues, projects, comments). Config-driven workflow engine. Generic CRUDModule protobuf service.

**Status:** Approved, not started. The existing Linear ingestion plugin works; this phase adds write operations.

**Deliverables:**
- CRUDModule proto service (generic entity_type dispatch)
- Linear plugin implements CRUD for issues/projects/comments
- REST endpoints: `/v1/linear/issues`, `/v1/linear/projects`, `/v1/linear/comments`
- MCP tools for create, read, update, list, comment, search
- YAML workflow engine (triggers, conditions, actions)
- Plugin manager `additional_services` support

**Dependencies:** Existing Linear ingestion module, Phase 43 CMS.

---

### Phase 45: Modular Intelligence & Active Participation 🔄

**Roadmap:** `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 6

**What it does:** Plugin architecture, Jiminy explainable retrieval, APE scheduler.

| Deliverable | Status | Key Files |
|-------------|--------|-----------|
| 45.1 Jiminy (Explainable Retrieval) | ✅ | `internal/retrieval/service.go` |
| 45.2 Binary Sidecar Host (Plugin Manager) | ✅ | `internal/plugins/manager.go`, `docs/development/SDK_PLUGIN_GUIDE.md` |
| 45.3 Code Parser Module Migration | 📋 | Extract parsers to RPC module |
| 45.4 Non-Code Integrations (Linear, Obsidian) | 🔄 | Linear ingestion done; CRUD = Phase 44; Obsidian pending |
| 45.5 APE (Active Participant Engine) | 🔄 | `internal/ape/scheduler.go`, `plugins/reflection-module/` — Context Cooler and Constraint Module pending |

---

### Phase 46: Symbol-Level Indexing ✅

**Roadmap:** `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 8

**What it does:** Tree-sitter symbol extraction (TS/JS/Go/Python), SymbolNode storage, symbol-aware retrieval.

| Deliverable | Status | Key Files |
|-------------|--------|-----------|
| 46.1 Parser Infrastructure | ✅ | `internal/symbols/parser.go`, `internal/symbols/types.go`, `internal/symbols/parser_test.go` |
| 46.2 Storage Schema | ✅ | `migrations/V0007__symbol_nodes.cypher`, `internal/symbols/store.go` |
| 46.3 Ingestion Integration | ✅ | `cmd/ingest-codebase/main.go` (`--extract-symbols`), `internal/api/handlers.go` |
| 46.4 Symbol-Aware Retrieval | ✅ | `internal/retrieval/service.go` (hybrid scoring with ε=0.25 symbol match) |
| 46.5 Symbol Search Endpoint | 📦 Archived | Deferred; use retrieve with `include_symbols: true` |
| 46.6 Testing & Validation | 📦 Archived | Core parser tests (12) done; VS Code benchmark deferred |

---

### Phase 47: Incremental Update & Re-Ingestion 🔄

**Roadmap:** `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 9

| Deliverable | Status | Key Files |
|-------------|--------|-----------|
| 47.1 Git Commit Hooks | ✅ | `cmd/ingest-codebase/main.go` (`--incremental`, `--since`, `--archive-deleted`) |
| 47.2 Time-Based Scheduled Sync | 🔄 | Freshness tracking done (TapRoot properties, `GET /v1/memory/spaces/{space_id}/freshness`); APE INGEST action pending |
| 47.3 User-Triggered Updates | ✅ | `POST /v1/memory/ingest/trigger`, `/status/{job_id}`, `/cancel/{job_id}`, `/jobs`; file-level re-ingest at `POST /v1/memory/ingest/files` |
| 47.4 Plugin-Specific Triggers | 📋 | Linear webhook, file watcher (fsnotify), event-driven module updates |
| 47.5 Conflict Resolution | ✅ | Optimistic locking with retry, edge consistency cascade |

**47.5 Optimistic Lock Retry + Edge Consistency (Completed 2026-02-06):**

| Component | Location | Description |
|-----------|----------|-------------|
| Retry package | `internal/optimistic/lock.go` | Exponential backoff with jitter, `WithRetry()`, error types |
| Versioned updates | `internal/retrieval/versioned_update.go` | `UpdateNodeWithVersion()`, `UpdateEdgeWithVersion()` |
| Edge consistency | `internal/retrieval/edge_consistency.go` | `PropagateEdgeStaleness()`, `RefreshStaleCoactivationEdges()` |
| Retry helpers | `internal/retrieval/ingest_retry.go` | `IngestWithRetry()`, `PropagateEdgeStalenessAfterIngest()` |
| Learning retry | `internal/learning/edge_retry.go` | `UpdateEdgeWithRetry()` for CO_ACTIVATED_WITH edges |
| API handlers | `internal/api/handlers_edge_consistency.go` | Stale edge stats and refresh endpoints |

**New API Endpoints:**
- `GET /v1/memory/edges/stale/stats?space_id=xxx` — Stale edge statistics
- `POST /v1/memory/edges/stale/refresh` — Trigger stale edge refresh

**Configuration (`.env.example`):**
```bash
OPTIMISTIC_RETRY_ENABLED=true           # default: true
OPTIMISTIC_RETRY_MAX_ATTEMPTS=5         # default: 5
OPTIMISTIC_RETRY_BASE_DELAY_MS=10       # default: 10
OPTIMISTIC_RETRY_MAX_DELAY_MS=1000      # default: 1000
OPTIMISTIC_RETRY_MULTIPLIER=2.0         # default: 2.0
EDGE_STALENESS_CASCADE_ENABLED=true     # default: true
EDGE_STALENESS_REFRESH_BATCH_SIZE=100   # default: 100
EDGE_STALENESS_RECLUSTER_THRESHOLD=0.3  # default: 0.3
```

---

### Phase 48: Query Optimization & Caching ✅

**Roadmap:** `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 10

| Deliverable | Status | Key Files |
|-------------|--------|-----------|
| 48.1 Query Profiling + Indexes | ✅ | `internal/retrieval/profiling.go`, `/v1/memory/query/metrics` |
| 48.2 Result Caching | ✅ | `internal/retrieval/cache.go`, `internal/retrieval/cache_test.go`, `/v1/memory/cache/stats` |
| 48.3 Data Transmission (gzip, pagination) | 📋 | |
| 48.4 Connection Pooling | 📋 | |
| 48.5 Benchmarking & Monitoring | ✅ | See below |

**48.5 Observability Stack (Completed 2026-02-06):**

| Component | Location | Description |
|-----------|----------|-------------|
| Prometheus config | `deploy/docker/prometheus.yml` | Scrape jobs for MDEMG, service health, TCP probes |
| Grafana provisioning | `deploy/docker/grafana/` | Auto-import datasources and dashboards |
| Blackbox exporter | `deploy/docker/blackbox/` | HTTP/TCP health monitoring |
| Alert rules | `deploy/docker/prometheus/alerts/latency_slo.yaml` | 7 SLO alerts |
| Dev compose | `deploy/docker/docker-compose.observability.yml` | Local testing stack |
| Dashboard | `deploy/docker/grafana/dashboards/mdemg-overview.json` | 10-panel overview |

**Dashboard Panels:** Request Rate, P95 Latency, Error Rate, Circuit Breakers, Request Latency Distribution, Requests by Status, Cache Hit Ratios, Retrieval Latency, Rate Limit Rejections, Embedding Latency.

**Metrics Fixes:**
- Fixed histogram bucket initialization (`server.go` - use `DefaultConfig()`)
- Fixed histogram Observe() double-counting (`prometheus.go`)
- Added retrieval latency instrumentation (`retrieval/service.go`)
- Added embedding latency instrumentation (`openai.go`, `ollama.go`)

**Results:** 92.5% uncached improvement (387ms→29ms); 98.9% cached improvement (387ms→4ms).

---

### Phase 49: LLM Plugin SDK & Self-Improvement 🔄

**Roadmap:** `docs/development/DEVELOPMENT_ROADMAP.md` §Phase 11

| Deliverable | Status | Key Files |
|-------------|--------|-----------|
| 49.1 Plugin SDK Documentation | ✅ | `docs/development/SDK_PLUGIN_GUIDE.md` (1,582 lines) |
| 49.2 LLM Semantic Summary Service | ✅ | `internal/summarize/service.go`, `internal/summarize/service_test.go` |
| 49.3 Claude Plugin Creation Skill | ✅ | `.claude/skills/create-plugin.md` |
| 49.4 Plugin Scaffolding Generator | 📋 | CLI `mdemg plugin new <name> --type=<TYPE>` |
| 49.5 Plugin Validation & Testing | 📋 | Automated manifest validation, gRPC contract testing |
| 49.6 Plugin Creation API | 📋 | `POST /v1/plugins/create` |
| 49.7 Capability Gap Detection | 📋 | Query pattern analysis, plugin suggestions |

---

### Phase 50: Public Readiness & Open Source Hardening 📋

**Spec:** `docs/development/repo-to-public-roadmap.md`

| Area | Status | Tasks |
|------|--------|-------|
| Governance & Collaboration | 📋 | PR/Issue templates, CONTRIBUTING.md, CODE_OF_CONDUCT.md |
| Security Hardening | 📋 | Secret scrubbing, path normalization, error sanitization |
| Repository Restructuring | 📋 | Standard Go layout, docs consolidation |
| CI/CD Guards | 📋 | GitHub Actions, integration CI with Neo4j |
| Public Onboarding | 📋 | README overhaul, SemVer releases, MIT License |

---

## 10. Governance & Testing Frameworks

### Framework Inventory

**Spec:** `docs/specs/FRAMEWORK_GOVERNANCE.md`

| Framework | Name | Purpose | Location |
|-----------|------|---------|----------|
| **UNTS** | Hash Verification | Hash verification for all protected files | `docs/specs/unts-hash-verification.md` (spec only) |
| **UDTS** | DevSpace Test Spec | gRPC contract/integration tests | `docs/api/api-spec/udts/` |
| **UATS** | API Test Spec | HTTP API acceptance tests | `docs/api/api-spec/uats/` |
| **UPTS** | Parser Test Spec | Language parser specs (25 languages) | `docs/lang-parser/lang-parse-spec/upts/` |
| **UBTS** | Benchmark Test Spec | Performance/load testing | Not yet created |
| **USTS** | Security Test Spec | Security hardening/vuln checks | Not yet created |
| **UOTS** | Observability Test Spec | Metrics, tracing, logging | Not yet created |
| **UAMS** | Auth Management Spec | Authentication/authorization | Not yet created |

### UDTS (Active)

| File | Tests |
|------|-------|
| `docs/api/api-spec/udts/schema/udts.schema.json` | JSON schema for UDTS specs |
| `docs/api/api-spec/udts/specs/space_transfer_list_spaces.udts.json` | ListSpaces contract |
| `docs/api/api-spec/udts/specs/space_transfer_space_info.udts.json` | SpaceInfo contract |
| `docs/api/api-spec/udts/specs/space_transfer_export_delta.udts.json` | Export delta (Phase 34) |
| `docs/api/api-spec/udts/specs/devspace_register_agent.udts.json` | RegisterAgent |
| `docs/api/api-spec/udts/specs/devspace_list_exports.udts.json` | ListExports |
| `docs/api/api-spec/udts/specs/devspace_pull_export.udts.json` | PullExport |
| `docs/api/api-spec/udts/specs/devspace_connect.udts.json` | Connect (bidi stream) |
| `tests/udts/contract_test.go` | Go test runner (all UDTS tests) |

### UATS (Active)

Located at `docs/api/api-spec/uats/specs/` — 40+ specs covering all HTTP endpoints. Runner: `docs/api/api-spec/uats/runners/uats_runner.py`.

### UPTS (Active)

Located at `docs/lang-parser/lang-parse-spec/upts/` — 25 language parser specs with fixtures and Python runner.

### Manifest

`docs/specs/manifest.sha256` — SHA256 hashes for all spec docs. Verified by `scripts/verify-manifest.sh`.

### Test Coverage Baseline

`docs/specs/test-coverage-baseline.md` — Coverage percentages per `internal/` package. New code gate: 80% minimum.

---

## 11. File Inventory by Domain

### Proto Files

| File | Service | Generated Output |
|------|---------|-----------------|
| `api/proto/mdemg-module.proto` | Plugin lifecycle, CRUDModule, SymbolInfo | `api/modulepb/` |
| `api/proto/space-transfer.proto` | Export, Import, ListSpaces, SpaceInfo | `api/transferpb/` |
| `api/proto/devspace.proto` | RegisterAgent, ListExports, PullExport, Connect | `api/devspacepb/` |

**Current space-transfer.proto SHA256:** `50c838e8cf291ac9c6b89341255c64aadaeb7cae3916c9f93a342bec75d9b85e`

### Migrations

| File | Content |
|------|---------|
| `migrations/V0001__base_schema.cypher` | Base MemoryNode, TapRoot, Observation, SchemaMeta |
| `migrations/V0002__edge_types.cypher` | Relationship types and constraints |
| `migrations/V0003__vector_indexes.cypher` | Vector index `memNodeEmbedding` (1536d) |
| `migrations/V0004__learning_edges.cypher` | CO_ACTIVATED_WITH edge support |
| `migrations/V0005__hidden_layer_support.cypher` | Hidden layer nodes and GENERALIZES edges |
| `migrations/V0006__improvement_tracks.cypher` | ConcernNode, ComparisonNode, ConfigurationNode |
| `migrations/V0007__symbol_nodes.cypher` | SymbolNode label, indexes, constraints, vector index |
| `migrations/V0008-V0010` | (Various incremental improvements) |
| `migrations/V0011__agent_identity.cypher` | Agent_id indexes for multi-agent CMS |

### Integration Tests

| File | Tests |
|------|-------|
| `tests/integration/transfer_test.go` | Export/import round-trip, delta export, profiles |
| `tests/integration/retrieval_test.go` | Ingest+retrieve, graph expansion, scoring determinism |
| `tests/integration/scoring_golden_test.go` | Golden file scoring (pre-existing failure) |
| `tests/integration/hidden_test.go` | Hidden layer consolidation |
| `tests/integration/ingest_test.go` | Ingest creates node, generates embedding, idempotent |
| `tests/integration/reflection_test.go` | Reflect endpoint flow |
| `tests/integration/stats_test.go` | Stats endpoint, embedding coverage |

### Documentation Map

| Path | Contents |
|------|----------|
| `docs/architecture/` | 24 files: Architecture (01), Graph Schema (02), Embeddings (03), Activation (04), Ingestion (05), Retrieval (06), Consolidation (07), Config (08), Testing (09), Ops (10), Migrations (11), Scoring Examples (12), Go Framework (13), Runbook (14), plus Hidden Layer, Hybrid Rerank, Interceptor, Learning Edges, Modular Intelligence, Recursive Consolidation, Temporal Decay specs |
| `docs/development/` | API Reference, CI/CD, Dev Roadmap, Linear Guide, Module Dev Guide, Public Roadmap, Research Roadmap, SDK Plugin Guide |
| `docs/specs/` | Phase specs (31-50 mapping), Framework Governance, UNTS spec, manifest, template |
| `docs/research/` | Edge Type Attention, GAT, Hybrid Edge Strategy, Enhancement Research, Query-Aware Expansion, Temporal Decay Results |
| `docs/benchmarks/` | Benchmark results, scripts, analysis (43 files) |
| `docs/lang-parser/` | UPTS specs for 25 languages, fixtures, parser roadmap, C++ analysis |

---

## 12. Development Principles

### Methodical and Modular (from Master Plan)

1. **One phase at a time.** No phase starts until the previous phase is complete (spec, impl, UDTS coverage, manifest hash).
2. **New code in new packages.** Prefer `internal/devspace/`, `api/proto/devspace.proto`, `cmd/devspace-hub/`; avoid touching core files unless required.
3. **Spec before impl.** Write/update the phase spec, then implement, then add UDTS/UATS specs and tests.

### Phase Completion Checklist

Before marking any phase **complete**:

- [ ] Phase spec updated and accurate
- [ ] All new/changed RPCs have at least one UDTS spec
- [ ] UDTS runner/tests pass for that phase's specs
- [ ] Proto (and spec doc if new) added to `docs/specs/manifest.sha256`
- [ ] `go build ./...` and `go test ./...` pass
- [ ] User interactive testing verifies functionality (NEVER mark complete without user verification)

### Spec Template

Use `docs/specs/TEMPLATE.md` for new phase specs. Required sections: Overview, Requirements (FR/NFR), API Contract, Data Model, Test Plan, Acceptance Criteria, Dependencies, Files Changed.

### Git Workflow

- **Branch:** `mdemg-dev01` (current)
- **Commit style:** Conventional Commits (`feat:`, `fix:`, `docs:`)
- **Main branch:** `main`

---

## 13. Known Issues & Technical Debt

| Issue | Severity | Location | Notes |
|-------|----------|----------|-------|
| ~~`TestScoringGolden`~~ | ✅ Fixed | `tests/integration/scoring_golden_test.go` | Updated target similarities to be above retrieval threshold |
| ~~UOBS Prometheus metrics~~ | ✅ Fixed | `docs/tests/uobs/specs/prometheus_metrics.uobs.json` | All 10/10 metrics now passing |
| UATS specs not all verified | Low | `docs/api/api-spec/uats/specs/` | 40+ specs exist but runner needs server |
| Obsidian module not started | Low | Phase 44/45 | Listed in roadmap but no implementation |
| Context Cooler (APE) not started | Medium | Phase 45.5 | Volatile observation graduation to long-term memory |
| `internal/ape/` low coverage | Medium | `docs/specs/test-coverage-baseline.md` | 0.0% coverage |
| `internal/consulting/` low coverage | Low | Same | 0.0% coverage |
| CRDT merge semantics undefined | Medium | Phase 35 | Need to finalize max-weight vs sum-evidence approach |

---

## 14. Quick Reference Commands

```bash
# === Build & Verify ===
go build ./...
go vet ./...
go test ./internal/... -v
go test -tags=integration ./tests/integration/... -v

# === Space Transfer ===
go run ./cmd/space-transfer export -space-id demo -output demo.mdemg
go run ./cmd/space-transfer import -input demo.mdemg -conflict skip
go run ./cmd/space-transfer serve -port 50052 -enable-devspace
go run ./cmd/space-transfer pull -remote localhost:50052 -space-id demo -output demo.mdemg

# === Delta Export (Phase 34) ===
go run ./cmd/space-transfer export -space-id demo -since-timestamp "2026-01-01T00:00:00Z" -output delta.mdemg

# === UDTS Tests ===
UDTS_TARGET=localhost:50052 go test ./tests/udts/... -v

# === UOBS Tests (Observability) ===
python3 docs/tests/uobs/runners/uobs_runner.py --spec "docs/tests/uobs/specs/*.uobs.json"
python3 docs/tests/uobs/runners/uobs_runner.py --spec docs/tests/uobs/specs/embedding_health.uobs.json

# === Health Endpoints ===
curl http://localhost:9999/healthz                    # Liveness probe
curl http://localhost:9999/readyz                     # Readiness probe
curl http://localhost:9999/v1/embedding/health        # Embedding model health

# === Ingestion ===
go run ./cmd/ingest-codebase --space-id mdemg-codebase --path /Users/reh3376/mdemg --extract-symbols
go run ./cmd/ingest-codebase --incremental --space-id mdemg-codebase --since HEAD~5

# === Server ===
NEO4J_URI=bolt://localhost:7687 NEO4J_USER=neo4j NEO4J_PASS=testpassword \
  REQUIRED_SCHEMA_VERSION=4 go run ./cmd/server

# === Neo4j ===
docker compose up -d
docker exec -i mdemg-neo4j cypher-shell -u neo4j -p testpassword

# === Manifest Verification ===
# scripts/verify-manifest.sh

# === Proto Regeneration ===
protoc --go_out=. --go-grpc_out=. api/proto/space-transfer.proto
protoc --go_out=. --go-grpc_out=. api/proto/devspace.proto
protoc --go_out=. --go-grpc_out=. api/proto/mdemg-module.proto
```

---

*Last updated: 2026-02-06 — Phase 47.5 (Optimistic Lock Retry + Edge Consistency) completed.*
