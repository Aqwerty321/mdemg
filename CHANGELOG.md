# Changelog

All notable changes to MDEMG will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
