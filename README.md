# MDEMG - Multi-Dimensional Emergent Memory Graph

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)](https://golang.org/)
[![Neo4j](https://img.shields.io/badge/Neo4j-5.x-008CC1.svg)](https://neo4j.com/)
[![CI](https://github.com/reh3376/mdemg/actions/workflows/ci.yml/badge.svg)](https://github.com/reh3376/mdemg/actions/workflows/ci.yml)

A persistent memory system for AI coding agents built on Neo4j with native vector indexes. Implements semantic retrieval with hidden layer concept abstraction and Hebbian learning.

> **Key insight**: The critical metric isn't average retrieval score—it's **state survival under context compaction**. Baseline agents forget architectural decisions after ~3 compactions. MDEMG maintains decision persistence indefinitely.

---

## Reproduce the Benchmark

Everything needed to independently verify our results is included.

### Prerequisites

- Docker (Neo4j)
- Go 1.21+
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

# 3. Ingest test codebase (or use your own)
go build -o bin/ingest-codebase ./cmd/ingest-codebase
./bin/ingest-codebase --space-id=benchmark --path=/path/to/target-repo

# 4. Run consolidation
curl -X POST http://localhost:8090/v1/memory/consolidate \
  -H "Content-Type: application/json" -d '{"space_id": "benchmark"}'

# 5. Run benchmark (see docs/tests/whk-wms/benchmark_v22_test/)
# Questions: test_questions_120_agent.json
# Grader: grade_answers.py
```

**Report output**: `grades_*.json` contains per-question scores with evidence breakdown.

### Verify Integrity

```bash
# Question bank hash (SHA-256)
shasum -a 256 docs/tests/whk-wms/test_questions_120_agent.json
# Expected: 24aa17a215e4e58b8b44c7faef9f14228edb0e6d3f8f657d867b1bfa850f7e9e

# Grader hash
shasum -a 256 docs/tests/whk-wms/benchmark_v22_test/grade_answers.py
# Expected: 5dbf84f092db31e4bc0d4867fd412c7af6575855f7c71e3d2f65e2ee8a8a21a5
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

### Key Results (Q&A Battery, v22)

| Metric | Baseline | MDEMG | Notes |
|--------|----------|-------|-------|
| Mean Score | 0.834 | 0.820 | Within margin of error |
| Evidence Rate | 100% | 97.1% | Both conditions high |
| High Confidence (>0.7) | 100% | 94.2% | Run 1 cold start |
| Run-to-Run Improvement | +2.9% | +3.0% | Similar learning |

### Key Metric: State Survival

The Q&A battery measures single-turn retrieval accuracy. The more important metric is **state survival under compaction**:

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
- **Plugin system**: Extensible via ingestion, reasoning, and APE (Autonomous Pattern Extraction) modules
- **Evidence-based retrieval**: Returns symbol-level citations (file:line references) with results
- **Capability gap detection**: Identifies missing knowledge areas for targeted improvement

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
                    │          MDEMG Service (:8090)        │
                    │  ┌─────────┐  ┌───────────────────┐  │
                    │  │Embedding│  │    Neo4j Graph    │  │
                    │  │ Provider│  │ (Vector + Graph)  │  │
                    │  └─────────┘  └───────────────────┘  │
                    └───────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.21+
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

# Run consolidation to create concept layers
curl -X POST http://localhost:8090/v1/memory/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id": "my-project"}'
```

## API Endpoints

### Core Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/memory/retrieve` | POST | Semantic search with optional LLM re-ranking |
| `/v1/memory/consult` | POST | SME-style Q&A with evidence citations |
| `/v1/memory/ingest` | POST | Store a single observation |
| `/v1/memory/ingest/batch` | POST | Store multiple observations |
| `/v1/memory/consolidate` | POST | Trigger hidden layer creation |
| `/v1/memory/stats` | GET | Per-space memory statistics |

See [API Reference](docs/API_REFERENCE.md) for complete documentation.

## MCP Integration (Cursor IDE)

MDEMG provides an MCP server for IDE integration. Add to `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "mdemg": {
      "command": "/path/to/mdemg/bin/mdemg-mcp",
      "args": [],
      "env": {
        "MDEMG_ENDPOINT": "http://localhost:8090"
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
│   └── plugins/          # Plugin system
├── docs/                 # Documentation
│   └── tests/            # Benchmark questions, graders, results
├── migrations/           # Neo4j schema (Cypher)
└── docker-compose.yml    # Neo4j container
```

## Documentation

- [Architecture](docs/01_Architecture.md) - System design and components
- [Graph Schema](docs/02_Graph_Schema.md) - Neo4j labels and relationships
- [API Reference](docs/API_REFERENCE.md) - Complete endpoint documentation
- [Retrieval & Scoring](docs/06_Retrieval_API_and_Scoring.md) - Scoring algorithm details
- [Plugin SDK](docs/11_Plugin_SDK.md) - Building custom plugins

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute.

## License

MIT License - see [LICENSE](LICENSE) for details.
