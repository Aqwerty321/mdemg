# Changelog

All notable changes to MDEMG will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Diagnostics Framework**: Structured `Diagnostic` struct with severity, code, message, parser, and context fields; `DiagnosticSummary` for aggregate reporting; `TruncateContentWithInfo()` and `NewDiagnostic()` helpers; wired into `walkCodebase` with summary logging
- **4 New Language Parsers**: C# (.cs), Kotlin (.kt, .kts), Terraform/HCL (.tf, .tfvars), Makefile (.mk, Makefile) — all with UPTS specs, test fixtures, and diagnostics support
- **UPTS Evidence Validation**: Structural consistency checks in the Go-native test harness — validates LineEnd consistency, CodeElement ranges, symbol containment, and LineEnd matching against specs; enabled for Go and Rust parsers
- **20 UPTS-Validated Parsers**: All 20 language parsers pass via `go test` across Go, Python, TypeScript, Rust, Java, C, C++, CUDA, SQL, Cypher, YAML, TOML, JSON, INI, Dockerfile, Shell, C#, Kotlin, Terraform, and Makefile

### Fixed
- **Ingestion whitelist**: `getEnabledLanguages()` now includes all 22 registered parsers (was missing yaml, toml, ini, dockerfile, shell, cuda, cypher)
- **Makefile parser `:=` assignment**: Fixed disambiguation logic that incorrectly rejected `:=` variable assignments as target definitions

### Previously Added
- **UPTS Go-Native Test Harness**: `upts_test.go` and `upts_types.go` — validates all language parsers directly via `go test` without external dependencies
- **Phase 9.5: Conflict Resolution & Consistency**: Data integrity during concurrent updates, orphan detection, and edge consistency
  - Version tracking: `version` counter incremented on every MERGE update, archive, and unarchive operation
  - `last_ingested_at` timestamp on every ingest update, distinct from `updated_at`
  - Conflict logging: DEBUG log when a node is updated (update_count > 1) with version and update_count
  - `POST /v1/memory/cleanup/orphans` — Orphan detection endpoint with `list`, `archive`, and `delete` actions; supports `dry_run` mode and `limit` parameter
  - Protected space enforcement: `delete` action blocked on protected spaces (e.g., `mdemg-dev`)
  - `edges_stale` flag: set on nodes when embedding changes during re-ingest
  - `RefreshStaleEdges()` method: refreshes ASSOCIATED_WITH edge weights for stale nodes, propagates staleness to parent hidden nodes
  - Edge refresh wired into consolidation pipeline as Step 6
- **Phase 9.4: Plugin-Specific Triggers**: Event-driven integration layer for external event sources
  - `TriggerEventWithContext()` on APE scheduler — passes `space_id`, `ingest_type`, and other context to APE modules
  - `POST /v1/webhooks/linear` — Linear webhook endpoint with HMAC-SHA256 signature verification, 10s debouncing, and automatic observation ingestion via plugin Parse
  - `cmd/watch` — Standalone file watcher binary using fsnotify; monitors directories for changes and triggers file ingestion via API
  - APE event wiring: `source_changed` and `ingest_complete` events fired after all ingest completion paths (batch, file, codebase)
  - Config: `LINEAR_WEBHOOK_SECRET`, `LINEAR_WEBHOOK_SPACE_ID` environment variables
- **Phase 9.1: Git Commit Hooks**: `--quiet` and `--log-file` CLI flags for `ingest-codebase`; git hook passes `--quiet` by default
- **Phase 9.2: Time-Based Scheduled Sync**: TapRoot freshness tracking (`last_ingest_at`, `last_ingest_type`, `ingest_count`), `GET /v1/memory/spaces/{space_id}/freshness` endpoint, periodic scheduled sync via `SYNC_INTERVAL_MINUTES`, stale space detection, MCP `memory_space_freshness` tool
- **Phase 9.3: User-Triggered Re-Ingestion**: Wired `runIngestJob()` to CLI binary with streaming progress via `--progress-json`
- **File-level re-ingest endpoint**: `POST /v1/memory/ingest/files` for targeted file re-ingestion (sync ≤50 files, background >50)
- **MCP tool `memory_ingest_files`**: Re-ingest specific files from IDE
- **CLI `--progress-json` flag**: Structured JSON progress events on stdout for `ingest-codebase`

### Fixed
- **MCP `memory_ingest_trigger` field mismatch**: `source_path` → `path`, `mode` → `incremental`, `exclude_pattern` → `exclude_dirs`

### Deprecated
- **`/v1/memory/ingest-codebase` endpoint**: Superseded by `/v1/memory/ingest/trigger` with superior job tracking; responses include `Deprecation` header
- **Linear CRUD Operations**: Full Create/Read/Update/Delete for issues, projects, and comments via Linear GraphQL API
- **CRUDModule protobuf service**: Generic gRPC service with entity_type dispatch and map fields, reusable by future plugins
- **Linear REST API endpoints**: `/v1/linear/issues`, `/v1/linear/projects`, `/v1/linear/comments` with full HTTP method dispatch
- **Linear MCP tools**: 6 tools for IDE integration — `linear_create_issue`, `linear_list_issues`, `linear_read_issue`, `linear_update_issue`, `linear_add_comment`, `linear_search`
- **Workflow engine**: Config-driven YAML automation with triggers (on-create/update/delete), conditions (eq/neq/contains/changed_to/exists), and actions (add-comment, auto-assign, auto-label, auto-transition, set-field)
- **Plugin additional_services**: Backward-compatible mechanism for modules to declare extra capabilities (e.g., INGESTION + CRUD)
- Edge-Type Attention for query-aware activation spreading
- Query-type detection (symbol_lookup, data_flow, architecture, generic)
- RetrievalHints for fine-grained retrieval control
- Layer-specific temporal decay (L0: 0.05/day, L1: 0.02/day, L2: 0.01/day)
- Hybrid edge strategy with query-aware graph expansion
- Universal Parser Test Schema (UPTS) v1.1 with 16 language parsers passing
- Universal API Test Schema (UATS) v1.0.1 with 41 endpoint specs
- Conversation Memory System (CMS) with hooks and protocols
- MCP server for IDE integration
- Codebase ingestion CLI and API endpoint (`/v1/memory/ingest-codebase`)
- Hidden layer concept abstraction and consolidation
- Hebbian learning loop with co-activation edge creation
- Edge weight decay and pruning CLI commands
- Plugin system with scaffold and validation tools
- CI pipeline with build, test, lint, and Trivy security scanning
- SECURITY.md with vulnerability reporting policy
- CONTRIBUTING.md with development guidelines

### Fixed
- **Parser symbol extraction**: Fixed C, C++, CUDA, SQL, Cypher parsers for correct function name extraction (was extracting parameter names)
- **CUDA multi-line kernel signatures**: Kernel pattern now handles `__global__` functions with parameters spanning multiple lines
- **SQL DEFAULT value parsing**: Parenthesis balancing prevents truncation of function calls like `gen_random_uuid()`
- **Cypher symbol types**: Labels, relationships, constraints, and indexes now emit correct UPTS types
- **C++ `static const` extraction**: Parser now recognizes `static const` and `static constexpr` constants
- **UPTS spec corrections**: Fixed 45 spec authoring errors across C (16), C++ (21), and CUDA (16) specs where auto-generated entries had parameter names instead of function names
- VectorSim floor to prevent spurious learning edges
- Migration files excluded from learning edge creation
- L0-only learning scope to reduce noise
- File extension filter handling for `#symbol` suffix queries
- Duplicate node prevention via idempotent ingestion

### Changed
- Standardized symbol field names to UPTS across codebase
- Reorganized documentation structure

## [0.1.0] - 2026-01-15

### Added
- Initial project scaffolding
- Neo4j graph database integration with vector indexes
- Semantic retrieval with embedding-based search (OpenAI, Ollama)
- Graph-based knowledge representation with memory nodes
- Core API server with health, ingest, retrieve, and consolidate endpoints
- Database migration framework (10 idempotent Cypher migrations)
- Docker Compose configuration for Neo4j
- Environment configuration via `.env` with example template
