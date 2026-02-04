# MDEMG - Multi-Dimensional Emergent Memory Graph

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://golang.org/)
[![Neo4j](https://img.shields.io/badge/Neo4j-5.x-008CC1.svg)](https://neo4j.com/)
[![CI](https://github.com/reh3376/mdemg/actions/workflows/ci.yml/badge.svg)](https://github.com/reh3376/mdemg/actions/workflows/ci.yml)

A persistent memory system for AI coding agents built on Neo4j with native vector indexes. Implements semantic retrieval with hidden layer concept abstraction and Hebbian learning.

> **Key insight**: The critical metric isn't average retrieval score—it's **state survival under context compaction**. Baseline agents forget architectural decisions after ~3 compactions. MDEMG maintains decision persistence indefinitely.

---

## Reproduce the Benchmark

Everything needed to independently verify our results is included.

### Prerequisites

- Docker (Neo4j)
- Go 1.24+
- Python 3.10+ (grading + utilities)

**Embeddings (choose one):**
- OpenAI API key, or
- Ollama (local embeddings)

**Agent under test (choose one):**
- Claude Code (recommended baseline runner), or
- Any tool-using LLM agent that can call the MDEMG API

### Reproduction Steps

```bash
# 1. Clone and setup
git clone https://github.com/reh3376/mdemg.git && cd mdemg
cp .env.example .env  # Add your embedding provider credentials

# 2. Start services
docker compose up -d
go build -o bin/mdemg ./cmd/server && ./bin/mdemg &
# Server writes .mdemg.port with actual port (dynamic allocation if preferred port is busy)

# 3. Ingest test codebase (or use your own)
go build -o bin/ingest-codebase ./cmd/ingest-codebase
./bin/ingest-codebase --space-id=benchmark --path=/path/to/target-repo

# 4. Run consolidation (reads port from .mdemg.port automatically)
PORT=$(cat .mdemg.port 2>/dev/null || echo 9999)
curl -X POST http://localhost:$PORT/v1/memory/consolidate \
  -H "Content-Type: application/json" -d '{"space_id": "benchmark"}'

# 5. Run benchmark (see docs/benchmarks/whk-wms/)
# Questions: test_questions_120_agent.json
# Grader: docs/benchmarks/grader_v4.py
```

**Report output**: `grades_*.json` contains per-question scores with evidence breakdown.

### Verify Integrity

```bash
# Question bank hash (SHA-256)
shasum -a 256 docs/benchmarks/whk-wms/test_questions_120_agent.json
# Expected: 24aa17a215e4e58b8b44c7faef9f14228edb0e6d3f8f657d867b1bfa850f7e9e
```

---

## Benchmark Receipts

Full reproducibility details for skeptics:

| Item | Value |
|------|-------|
| **MDEMG Commit** | `779d753` |
| **Question Set** | `test_questions_120_agent.json` (120 questions) |
| **Question Hash** | `sha256:24aa17a2...` |
| **Answer Key** | `test_questions_120.json` |
| **Grader** | `grade_answers.py` |
| **Scoring Weights** | Evidence: 0.70 / Concept: 0.15 / Semantic: 0.15 |
| **Target Codebase** | whk-wms (507K LOC TypeScript) |
| **Include Patterns** | `**/*.ts`, `**/*.tsx`, `**/*.json` |
| **Exclude Patterns** | `node_modules/`, `dist/`, `.git/`, `docs-website/` |
| **Agent Model** | Claude Haiku (via Claude Code) |
| **Runs** | 2 per condition (run 3 excluded for consistency) |
| **Embedding Provider** | OpenAI text-embedding-3-small |

> **Baseline definition:** Same agent runner and tool permissions, **no MDEMG retrieval**, relying on long-context + auto-compaction only (memory off).

### Key Results (2026-01-30, whk-wms 507K LOC)

| Metric | Baseline | MDEMG + Edge Attention | Delta |
|--------|----------|------------------------|-------|
| **Mean Score** | 0.854 | **0.898** | **+5.2%** |
| Standard Deviation | 0.088 | 0.059 | -51% variance |
| High Score Rate (≥0.7) | 97.9% | **100%** | +2.1pp |
| Strong Evidence Rate | 97.9% | **100%** | +2.1pp |

**Category Performance (Edge Attention):**
| Category | Mean Score |
|----------|------------|
| Disambiguation | 0.958 |
| Service Relationships | 0.916 |
| Architecture Structure | 0.889 |
| Data Flow Integration | 0.882 |

### Key Metric: State Survival

The Q&A battery measures single-turn retrieval accuracy. The critical differentiator is **state survival under compaction**:

| Metric | Baseline | MDEMG | Source |
|--------|----------|-------|--------|
| Decision Persistence @5 compactions | 0% | 95% | Compaction torture test |

When context windows fill and auto-compaction kicks in, baseline agents lose architectural decisions. MDEMG persists them in the graph.

**The baseline forgets under pressure. MDEMG remembers.**

---

## Overview

MDEMG provides long-term memory for AI agents, enabling them to:

- **Store observations**: Persist code patterns, decisions, and architectural knowledge
- **Semantic recall**: Retrieve relevant memories via vector similarity search
- **Concept abstraction**: Automatically form higher-level concepts from related memories (hidden layers)
- **Associative learning**: Build connections between memories through Hebbian reinforcement
- **LLM re-ranking**: Apply GPT-powered relevance scoring for improved retrieval quality

## Key Features

- **Multi-layer graph architecture**: Base observations (L0) → Hidden concepts (L1) → Abstract concepts (L2+)
- **Hybrid search**: Combines vector similarity with graph traversal
- **Conversation Memory System (CMS)**: Persistent memory across agent sessions with surprise-weighted learning
- **Symbol extraction (UPTS)**: Unified Parser Test Schema supporting 30+ languages with file:line evidence
- **Plugin system**: Extensible via ingestion, reasoning, and APE (Autonomous Pattern Extraction) modules
- **Evidence-based retrieval**: Returns symbol-level citations (file:line references) with results
- **Capability gap detection**: Identifies missing knowledge areas for targeted improvement
- **Codebase ingestion API**: Background job processing for large codebase ingestion with consolidation
- **Git commit hooks**: Automatic incremental ingestion on every commit via post-commit hook
- **Freshness tracking**: TapRoot-level staleness detection with configurable thresholds
- **Scheduled sync**: Periodic background sync to keep memory graphs up-to-date

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      AI Coding Agent                        │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   IDE/CLI    │◄──►│  MCP Server  │◄──►│ MDEMG Client │  │
│  │  (Cursor)    │    │   (tools)    │    │    (HTTP)    │  │
│  └──────────────┘    └──────────────┘    └──────┬───────┘  │
└─────────────────────────────────────────────────┼──────────┘
                                                  │
                    ┌─────────────────────────────▼─────────┐
                    │     MDEMG Service (dynamic port)      │
                    │  ┌─────────┐  ┌───────────────────┐  │
                    │  │Embedding│  │    Neo4j Graph    │  │
                    │  │ Provider│  │ (Vector + Graph)  │  │
                    │  └─────────┘  └───────────────────┘  │
                    └───────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.24+
- Docker (for Neo4j)
- Embedding provider: Ollama (local) or OpenAI API key

### Setup

```bash
# Clone the repo
git clone https://github.com/reh3376/mdemg.git
cd mdemg

# Copy environment config
cp .env.example .env
# Edit .env with your settings (embedding provider, Neo4j credentials)

# Start Neo4j
docker compose up -d

# Build the server
go build -o bin/mdemg ./cmd/server

# Run the server
./bin/mdemg
```

### Ingest a Codebase

```bash
# Build the ingestion tool
go build -o bin/ingest-codebase ./cmd/ingest-codebase

# Ingest a codebase
./bin/ingest-codebase --space-id=my-project --path=/path/to/repo

# Incremental ingest (only changed files since last commit)
./bin/ingest-codebase --space-id=my-project --path=/path/to/repo --incremental

# Quiet mode (suppress non-error output, useful for hooks/CI)
./bin/ingest-codebase --space-id=my-project --path=/path/to/repo --quiet

# Log to file instead of stderr
./bin/ingest-codebase --space-id=my-project --path=/path/to/repo --log-file /tmp/ingest.log

# Run consolidation to create concept layers
curl -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id": "my-project"}'
```

### Git Commit Hook (Automatic Ingestion)

Install the post-commit hook to automatically ingest changes on every commit:

```bash
# Install the hook
./scripts/install-hook.sh /path/to/your/repo

# The hook runs quietly by default. Configure via environment:
# MDEMG_SPACE_ID - space to ingest into (default: repo directory name)
# MDEMG_ENDPOINT - server URL (default: http://localhost:9999)
# MDEMG_VERBOSE  - set to "true" for verbose output
# MDEMG_LOG_FILE - redirect logs to a file
```

## API Endpoints

### Core Memory Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/memory/retrieve` | POST | Semantic search with optional LLM re-ranking |
| `/v1/memory/consult` | POST | SME-style Q&A with evidence citations |
| `/v1/memory/ingest` | POST | Store a single observation |
| `/v1/memory/ingest/batch` | POST | Store multiple observations |
| `/v1/memory/consolidate` | POST | Trigger hidden layer creation |
| `/v1/memory/stats` | GET | Per-space memory statistics |
| `/v1/memory/ingest-codebase` | POST | Background codebase ingestion job |
| `/v1/memory/symbols` | GET | Query extracted code symbols |
| `/v1/memory/ingest/files` | POST | Ingest files with background job processing |
| `/v1/memory/spaces/{id}/freshness` | GET | Space freshness and staleness status |

### Conversation Memory System (CMS)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/conversation/resume` | POST | Restore session context with themes and concepts |
| `/v1/conversation/observe` | POST | Record observations (decisions, corrections, learnings) |
| `/v1/conversation/correct` | POST | Record user corrections for learning |
| `/v1/conversation/recall` | POST | Query conversation history |
| `/v1/conversation/consolidate` | POST | Create themes from observations |

### Learning Control

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/learning/stats` | GET | Hebbian learning edge statistics |
| `/v1/learning/freeze` | POST | Freeze learning for stable scoring |
| `/v1/learning/unfreeze` | POST | Resume learning edge creation |
| `/v1/learning/prune` | POST | Remove decayed edges |

## Symbol Extraction (UPTS)

MDEMG extracts code symbols during ingestion using the Unified Parser Test Schema (UPTS):

**Supported Languages (30+):**
- TypeScript, JavaScript, Python, Go, Rust, Java, C/C++, C#
- Ruby, PHP, Swift, Kotlin, Scala, Dart
- SQL, GraphQL, Protobuf, YAML, JSON, TOML, Dockerfile
- And more...

**Extracted Symbol Types:**
- Functions, methods, classes, interfaces, types
- Constants, variables, enums
- Imports, exports, module declarations

Symbols include file:line references for evidence-based retrieval.

## Conversation Memory System (CMS)

CMS provides persistent memory for AI agents across sessions:

```bash
# Resume session context (call at session start)
curl -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id": "my-agent", "session_id": "session-1", "max_observations": 10}'

# Record an observation (decision, learning, correction)
curl -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-agent",
    "session_id": "session-1",
    "content": "User prefers TypeScript for new files",
    "obs_type": "preference"
  }'
```

**Observation Types:** `decision`, `correction`, `learning`, `preference`, `error`, `progress`

Observations are surprise-weighted (novel information persists longer) and form themes via consolidation.

## MCP Integration (Cursor IDE)

MDEMG provides an MCP server for IDE integration. Add to `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "mdemg": {
      "command": "/path/to/mdemg/bin/mdemg-mcp",
      "args": [],
      "env": {
        "MDEMG_ENDPOINT": "http://localhost:9999"
      }
    }
  }
}
```

## Project Structure

```
mdemg/
├── cmd/                  # CLI tools (server, ingest, MCP, etc.)
├── internal/             # Core logic
│   ├── api/              # HTTP handlers
│   ├── retrieval/        # Search algorithms
│   ├── hidden/           # Concept abstraction
│   ├── learning/         # Hebbian edges
│   ├── conversation/     # Conversation Memory System (CMS)
│   ├── symbols/          # Code symbol extraction
│   └── plugins/          # Plugin system
├── docs/                 # Documentation
│   └── benchmarks/       # Benchmark questions, graders, results
├── migrations/           # Neo4j schema (Cypher)
├── tests/                # Integration tests
└── docker-compose.yml    # Neo4j container
```

## Documentation

- [Architecture](docs/architecture/01_Architecture.md) - System design and components
- [Graph Schema](docs/architecture/02_Graph_Schema.md) - Neo4j labels and relationships
- [Retrieval & Scoring](docs/architecture/06_Retrieval_API_and_Scoring.md) - Scoring algorithm details
- [Benchmarking Guide](docs/benchmarks/BENCHMARK_V4_README.md) - Running and validating benchmarks
- [CI/CD Integration](docs/development/CI_CD_INTEGRATION.md) - Git hooks, GitHub Actions, and scheduled sync
- [API Reference](docs/development/API_REFERENCE.md) - Full API endpoint documentation

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## Security

See [SECURITY.md](SECURITY.md) for the vulnerability reporting policy.

## License

MIT License - see [LICENSE](LICENSE) for details.
