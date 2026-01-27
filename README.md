# MDEMG - Multi-Dimensional Emergent Memory Graph

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)](https://golang.org/)
[![Neo4j](https://img.shields.io/badge/Neo4j-5.x-008CC1.svg)](https://neo4j.com/)

A persistent memory system for AI coding agents built on Neo4j with native vector indexes. Implements semantic retrieval with hidden layer concept abstraction and Hebbian learning.

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
- Ollama (for local embeddings) or OpenAI API key

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

### Example: Retrieve

```bash
curl -X POST http://localhost:8090/v1/memory/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "query": "How does authentication work?",
    "top_k": 10,
    "include_evidence": true
  }'
```

### Example: Consult (SME Mode)

```bash
curl -X POST http://localhost:8090/v1/memory/consult \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "context": "Investigating user session handling",
    "question": "Where is the session timeout configured?",
    "include_evidence": true
  }'
```

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

### MCP Tools

| Tool | Purpose |
|------|---------|
| `memory_store` | Save observations, patterns, decisions |
| `memory_recall` | Retrieve relevant memories by semantic search |
| `memory_associate` | Explicitly link two concepts |
| `memory_reflect` | Deep exploration of a topic |
| `memory_status` | Check system health |

## Performance

MDEMG has been benchmarked on large industrial codebases (500K-800K LOC):

| Metric | Value |
|--------|-------|
| Retrieval latency (warm) | ~50ms |
| Top-10 relevance score | 0.73 avg |
| High-confidence rate (>0.7) | 75% |
| Evidence compliance | 100% (file:line refs) |

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
├── migrations/           # Neo4j schema (Cypher)
└── docker-compose.yml    # Neo4j container
```

## Documentation

- [Architecture](docs/01_Architecture.md) - System design and components
- [Graph Schema](docs/02_Graph_Schema.md) - Neo4j labels and relationships
- [Retrieval API](docs/06_Retrieval_API_and_Scoring.md) - Scoring algorithm details
- [Plugin SDK](docs/11_Plugin_SDK.md) - Building custom plugins

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute.

## License

MIT License - see [LICENSE](LICENSE) for details.
