# MDEMG - Multi-Dimensional Emergent Memory Graph

A long-term memory system for AI coding agents built on Neo4j with native vector indexes. Implements retrieval-augmented memory with spreading activation and Hebbian learning.

## Overview

MDEMG provides persistent memory for AI agents running in IDEs like Cursor. It enables agents to:
- Store observations, patterns, and decisions
- Maintain an ongoing 'internal' dialog with the main coding agent
- Intercept 'thoughts' from the coding agenet(s) to review against context specific knowledge maintained in mdemg
- Recall relevant memories via semantic search
- Build associative connections between concepts
- Develop emergent behaviors over time

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    aci-claude-go (CLI/TUI)                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ Claude Agent │◄──►│ Memory Adapter│◄──►│ MDEMG Client │  │
│  │ (Go Coder)   │    │ (internal)   │    │  (pkg/mdemg) │  │
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

## Integration with aci-claude-go

MDEMG is the primary long-term memory layer for the **aci-claude-go** autonomous framework. It facilitates:
- **Internal Dialog Persistence**: Chronological thought threading across agents and sessions.
- **Cross-Session Learning**: Automatic semantic retrieval of past decisions and patterns.
- **Autonomous Self-Reflection**: System-triggered analysis of learned patterns to identify architectural drift.
- **Subject Matter Expertise**: Acting as a cognitive substrate for the Planner, Coder, and QA agents.

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
for f in migrations/V*.cypher; do
  docker exec -i mdemg-neo4j cypher-shell -u neo4j -p testpassword < "$f"
done

# Install Ollama embedding model
ollama pull nomic-embed-text

# Start the service
./start-mdemg.sh
```

## MCP Integration (aci-claude-go / Cursor)

MDEMG provides an MCP server for seamless integration.

**For aci-claude-go:**
The framework utilizes `pkg/mdemg` directly via the `internal/agent/Memory` interface.

**For Cursor IDE:**
Add to `~/.cursor/mcp.json`:
```json
{
  "mcpServers": {
    "mdemg": {
      "command": "/path/to/mdemg/cmd/mcp-server/mdemg-mcp",
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

## Benchmarks & Performance

MDEMG has been rigorously tested against large-scale industrial codebases, including **whk-wms** (792K LOC) and **plc-gbt** (Industrial Control Systems). The transition from the v4 baseline to the v11 production iteration resulted in a **29.3% improvement** in average retrieval quality and a **7.5x increase** in high-confidence architectural reasoning.

**Key v11 Results (whk-wms):**
*   **Average Retrieval Score**: 0.733 (vs 0.567 baseline)
*   **High Confidence Rate (Score > 0.7)**: 75%
*   **Retrieval Latency**: < 50ms
*   **Token Efficiency**: 99.7% reduction in token cost per query.

**Cross-Codebase Validation (plc-gbt):**
*   **Average Retrieval Score**: 0.719
*   **High Confidence Rate**: 58%
*   **ICS-Specific Performance**: Verified 0.71+ scores for control loop and data model architecture.

For a detailed breakdown of the technical evolution and performance metrics across all iterations, see the [Up-to-Date Benchmark Summary](docs/tests/UP_TO_DATE_BENCHMARK_SUMMARY.md).

## Documentation

- [Vision & Architecture](VISION.md) - Core philosophy and design
- [Detailed Benchmarks](docs/tests/UP_TO_DATE_BENCHMARK_SUMMARY.md) - Evolution from v4 to v11
- [Repo-to-Public Roadmap](docs/repo-to-public-roadmap.md) - Plan for open-source transition
- [Architecture Details](docs/01_Architecture.md) - System design
- [Graph Schema](docs/02_Graph_Schema.md) - Labels and relationships
- [Retrieval & Scoring](docs/06_Retrieval_API_and_Scoring.md) - Scoring algorithm details

## Project Structure

```
mdemg/
├── cmd/                  # Binaries (Server, MCP, Ingest, etc.)
├── internal/             # Private core logic (Retrieval, Learning, APE)
├── pkg/                  # Importable Go client logic
├── migrations/           # Neo4j schema migrations (Cypher)
├── docs/                 # Technical documentation & Benchmarks
├── docker-compose.yml    # Neo4j container
└── start-mdemg.sh        # One-command startup
```

## Emergent Behaviors

Over time, the system is designed to develop:
1. **Concept clustering** - Similar patterns grouping together
2. **Cross-project transfer** - Knowledge flowing between projects
3. **Workflow recognition** - Recognizing recurring patterns
4. **Abstraction emergence** - General principles from specific examples

## License

MIT
