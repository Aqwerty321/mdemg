# MDEMG - Multi-Dimensional Emergent Memory Graph

A long-term memory system for AI coding agents built on Neo4j with native vector indexes. Implements retrieval-augmented memory with spreading activation and Hebbian learning.

## Overview

MDEMG provides persistent memory for AI agents running in IDEs like Cursor. It enables agents to:
- Store observations, patterns, and decisions
- Recall relevant memories via semantic search
- Build associative connections between concepts
- Develop emergent behaviors over time

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Cursor IDE                            │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ Claude Agent │◄──►│  MCP Client  │◄──►│ MDEMG MCP    │  │
│  │              │    │              │    │   Server     │  │
│  └──────────────┘    └──────────────┘    └──────┬───────┘  │
└─────────────────────────────────────────────────┼──────────┘
                                                  │
                    ┌─────────────────────────────▼─────────┐
                    │          MDEMG Service (:8082)        │
                    │  ┌─────────┐  ┌───────────────────┐  │
                    │  │Embedding│  │    Neo4j Graph    │  │
                    │  │ Provider│  │ (Vector + Graph)  │  │
                    │  └─────────┘  └───────────────────┘  │
                    └───────────────────────────────────────┘
```

## Core Design Principles

- **Vector index = recall** - Fast candidate generation via cosine similarity
- **Graph = reasoning** - Typed edges with evidence weights
- **Runtime = activation physics** - Spreading activation computed in-memory
- **DB writes = learning deltas only** - No per-request activation writes

## Quick Start

### Prerequisites
- Go 1.25+
- Docker (for Neo4j)
- Ollama (for local embeddings) or OpenAI API key

### Setup

```bash
# Clone the repo
git clone https://github.com/reh3376/mdemg.git
cd mdemg

# Start Neo4j
docker compose up -d

# Apply migrations
for f in mdemg_build/migrations/V*.cypher; do
  docker exec -i mdemg-neo4j cypher-shell -u neo4j -p testpassword < "$f"
done

# Install Ollama embedding model
ollama pull nomic-embed-text

# Start the service
./start-mdemg.sh
```

### MCP Integration (Cursor)

Add to `~/.cursor/mcp.json`:
```json
{
  "mcpServers": {
    "mdemg": {
      "command": "/path/to/mdemg/mdemg_build/mcp-server/mdemg-mcp",
      "args": [],
      "env": {
        "MDEMG_ENDPOINT": "http://localhost:8082"
      }
    }
  }
}
```

## MCP Tools

| Tool | Purpose |
|------|---------|
| `memory_store` | Save observations, patterns, decisions |
| `memory_recall` | Retrieve relevant memories by semantic search |
| `memory_associate` | Explicitly link two concepts |
| `memory_reflect` | Deep exploration of a topic |
| `memory_status` | Check system health |

## Documentation

- [HANDOFF.md](HANDOFF.md) - Development status and quickstart
- [CLAUDE.md](CLAUDE.md) - AI assistant context
- [Architecture](mdemg_build/docs/01_Architecture.md) - System design
- [Graph Schema](mdemg_build/docs/02_Graph_Schema.md) - Labels and relationships
- [Retrieval & Scoring](mdemg_build/docs/06_Retrieval_API_and_Scoring.md) - Scoring algorithm

## Project Structure

```
mdemg/
├── mdemg_build/
│   ├── service/          # Go HTTP service
│   ├── mcp-server/       # MCP server for agent integration
│   ├── migrations/       # Neo4j schema migrations
│   └── docs/             # Technical documentation
├── docker-compose.yml    # Neo4j container
├── start-mdemg.sh        # One-command startup
└── HANDOFF.md            # Development status
```

## Emergent Behaviors

Over time, the system is designed to develop:
1. **Concept clustering** - Similar patterns grouping together
2. **Cross-project transfer** - Knowledge flowing between projects
3. **Workflow recognition** - Recognizing recurring patterns
4. **Abstraction emergence** - General principles from specific examples

## License

MIT
