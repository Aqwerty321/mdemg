# Contributing to MDEMG

Thank you for your interest in contributing to MDEMG (Multi-Dimensional Emergent Memory Graph). This document provides guidelines for contributing to the project.

## Getting Started

### Prerequisites

- Go 1.24 or later
- Neo4j 5.x with vector index support
- An embedding provider (OpenAI API or Ollama)
- Python 3.10+ (for benchmark and parser test runners)

### Development Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/reh3376/mdemg.git
   cd mdemg
   ```

2. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

3. Configure your `.env` file with:
   - Neo4j connection details
   - Embedding provider credentials

4. Start Neo4j (if using Docker):
   ```bash
   docker compose up -d
   ```

5. Build the server:
   ```bash
   go build -o bin/mdemg ./cmd/server
   ```

6. Run the server:
   ```bash
   ./bin/mdemg
   ```

## Code Style

- Follow standard Go conventions and idioms
- Run `go fmt` before committing
- Run `golangci-lint run` to catch lint issues (this runs in CI)
- Use meaningful variable and function names
- Add comments for exported functions and complex logic
- Keep functions focused and reasonably sized

## Testing

- Write tests for new functionality
- Run existing tests before submitting:
  ```bash
  # Unit tests
  go test ./internal/...

  # Integration tests (requires running Neo4j + MDEMG server)
  go test -tags=integration ./tests/integration/...

  # Parser validation — Go-native UPTS harness (no external deps)
  go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v

  # Parser validation — Python UPTS runner (requires bin/extract-symbols)
  make test-parsers

  # Validate a single language parser
  make test-parser-go
  make test-parser-python
  make test-parser-typescript

  # Go-native UPTS for a single language
  go test ./cmd/ingest-codebase/languages/ -run TestUPTS/rust -v

  # API validation (UATS - requires running server)
  make test-api

  # Smoke tests (health + readiness only)
  make test-smoke

  # All tests (UPTS + UATS)
  make test-all
  ```
- Include both unit tests and integration tests where appropriate

### Test Frameworks

**UPTS (Universal Parser Test Specification)** validates 20 language parsers against JSON spec files. There are two runners:

1. **Go-native test harness** (`cmd/ingest-codebase/languages/upts_test.go`): Loads UPTS specs, parses fixtures through the actual Go parser, and validates output against expected symbols. No external dependencies — runs via standard `go test`. This is the primary validation method.

2. **Python runner** (`docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py`): Validates via the `bin/extract-symbols` CLI binary. Useful for cross-validation and CI.

Specs live in `docs/lang-parser/lang-parse-spec/upts/specs/`. Fixtures live in `docs/lang-parser/lang-parse-spec/upts/fixtures/`.

### Adding or Updating a Parser Test

1. Create or edit `docs/lang-parser/lang-parse-spec/upts/specs/<language>.upts.json`
2. Create or edit the fixture file in `docs/lang-parser/lang-parse-spec/upts/fixtures/`
3. Run the Go-native harness: `go test ./cmd/ingest-codebase/languages/ -run TestUPTS/<language> -v`
4. Optionally cross-validate with the Python runner: `make test-parser-<language>`

### Parser Development Workflow

When adding a new language parser or modifying an existing one:

1. **Create the parser** in `cmd/ingest-codebase/languages/<language>_parser.go` (see that directory's README for the interface)
2. **Create a fixture** — a representative source file covering all symbol types the parser should extract
3. **Create a UPTS spec** — JSON file declaring expected symbols with name, type, line number, and export status
4. **Validate** — run `go test ./cmd/ingest-codebase/languages/ -run TestUPTS/<language> -v` until all assertions pass
5. **Key spec fields:**
   - `line_tolerance`: how far actual line can be from expected (default ±2)
   - `optional`: set `true` on symbols that may or may not be emitted (e.g., duplicate declarations)
   - `pattern`: document which extraction pattern this symbol tests (e.g., `P2_FUNCTION`)
   - See existing specs for examples of the full schema

**UATS (Universal API Test Specification)** validates API endpoints against JSON spec files. Specs live in `docs/api/api-spec/uats/specs/`. The `--base-url` flag in the Makefile dynamically reads the server's port from `.mdemg.port` (see Dynamic Port Allocation below). To add or update an API test:
1. Create or edit `docs/api/api-spec/uats/specs/<endpoint>.uats.json`
2. Run `make test-api-<endpoint>` to validate
3. Install UATS dependencies: `make uats-setup`

### Dynamic Port Allocation

The MDEMG server uses dynamic port allocation. When started, it writes the actual bound port to `.mdemg.port`. All test commands (`make test-api`, `make test-smoke`) read this file automatically. If the file doesn't exist, port `9999` is used as the fallback default.

## Submitting Changes

### Pull Request Process

1. Fork the repository and create a feature branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. Make your changes following the code style guidelines

3. Write or update tests as needed

4. Commit your changes with clear, descriptive messages:
   ```bash
   git commit -m "feat: add new retrieval optimization"
   ```

   Use conventional commit prefixes:
   - `feat:` - New features
   - `fix:` - Bug fixes
   - `docs:` - Documentation changes
   - `test:` - Test additions or changes
   - `refactor:` - Code refactoring
   - `perf:` - Performance improvements

5. Push to your fork and open a Pull Request

6. Ensure CI checks pass and address any review feedback

### What to Include in a PR

- Clear description of the changes
- Motivation/context for the changes
- Any breaking changes noted
- Test plan or evidence of testing

## Project Structure

```
mdemg/
├── cmd/                  # CLI entry points (server, ingest-codebase, mcp-server, etc.)
├── internal/             # Internal packages
│   ├── api/              # HTTP API handlers
│   ├── retrieval/        # Core retrieval algorithms
│   ├── hidden/           # Hidden layer and concept abstraction
│   ├── learning/         # Hebbian learning edges
│   ├── embeddings/       # Embedding providers (OpenAI, Ollama)
│   ├── conversation/     # Conversation Memory System (CMS)
│   ├── symbols/          # Code symbol extraction
│   └── plugins/          # Plugin system
├── migrations/           # Neo4j schema migrations (Cypher)
├── tests/                # Integration tests
├── scripts/              # Utility scripts
├── docs/                 # Documentation and benchmarks
└── plugins/              # Plugin modules
```

## API Endpoints

Full API specs are in `docs/api/api-spec/uats/specs/` (one `.uats.json` per endpoint). Below is the complete endpoint list registered in `internal/api/server.go`:

### Health & Readiness

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/healthz` | Liveness check |
| GET | `/readyz` | Readiness check (includes embedding provider status) |

### Memory Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/memory/retrieve` | Semantic retrieval with graph expansion |
| POST | `/v1/memory/ingest` | Ingest a single observation |
| POST | `/v1/memory/ingest/batch` | Batch ingest observations |
| POST | `/v1/memory/reflect` | Deep reflection on a topic |
| GET | `/v1/memory/stats` | Memory graph statistics |
| POST | `/v1/memory/consolidate` | Trigger hidden layer consolidation |
| POST | `/v1/memory/archive/bulk` | Bulk archive nodes |
| * | `/v1/memory/nodes/{id}` | Node CRUD operations |
| GET | `/v1/memory/symbols` | Search code symbols |
| GET | `/v1/memory/distribution` | Score distribution stats |
| GET | `/v1/memory/cache/stats` | Embedding/query cache stats |
| GET | `/v1/memory/query/metrics` | Query performance metrics |
| POST | `/v1/memory/consult` | SME-style Q&A |
| POST | `/v1/memory/suggest` | Suggest related concepts |

### Codebase Ingestion (Background Jobs)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/memory/ingest/trigger` | Start background ingestion job |
| GET | `/v1/memory/ingest/status/{id}` | Check job progress |
| POST | `/v1/memory/ingest/cancel/{id}` | Cancel running job |
| GET | `/v1/memory/ingest/jobs` | List all ingestion jobs |
| POST | `/v1/memory/ingest/files` | Ingest files with background job |
| * | `/v1/memory/ingest-codebase` | Codebase ingestion route (deprecated) |

### Freshness & Sync

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/memory/spaces/{id}/freshness` | Space freshness and staleness status |

### Learning & Hebbian Edges

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/learning/prune` | Prune low-weight edges |
| GET | `/v1/learning/stats` | Learning edge statistics |
| POST | `/v1/learning/freeze` | Freeze learning for stable scoring |
| POST | `/v1/learning/unfreeze` | Unfreeze learning |
| GET | `/v1/learning/freeze/status` | Check freeze status |

### Conversation Memory System (CMS)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/conversation/observe` | Store observation |
| POST | `/v1/conversation/correct` | Store correction |
| POST | `/v1/conversation/resume` | Resume session (restore context) |
| POST | `/v1/conversation/recall` | Recall conversation memory |
| POST | `/v1/conversation/consolidate` | Consolidate conversation themes |
| GET | `/v1/conversation/volatile/stats` | Volatile memory stats |
| POST | `/v1/conversation/graduate` | Graduate volatile to permanent |

### Cleanup

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/memory/cleanup/orphans` | Detect/archive/delete orphaned nodes after re-ingestion |

### Webhooks

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/webhooks/linear` | Linear webhook receiver (HMAC-SHA256 verified) |

### System & Plugins

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/metrics` | Prometheus-style metrics |
| GET | `/v1/modules` | List modules |
| * | `/v1/modules/{id}` | Module sync |
| * | `/v1/plugins` | Plugin operations |
| GET | `/v1/ape/status` | APE (Autonomous Prompt Evolution) status |
| POST | `/v1/ape/trigger` | Trigger APE |
| POST | `/v1/feedback` | Submit feedback |
| GET | `/v1/system/capability-gaps` | List capability gaps |
| GET | `/v1/system/gap-interviews` | Gap interview sessions |
| GET | `/v1/system/pool-metrics` | Neo4j connection pool metrics |

## Reporting Issues

Use the [bug report](https://github.com/reh3376/mdemg/issues/new?template=bug_report.yml) or [feature request](https://github.com/reh3376/mdemg/issues/new?template=feature_request.yml) templates when opening issues.

## Code of Conduct

This project follows a Code of Conduct to ensure a welcoming and respectful environment for everyone. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before participating.

## License

By contributing to MDEMG, you agree that your contributions will be licensed under the MIT License.
