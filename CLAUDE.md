# MDEMG Project Instructions

## Orchestration Protocol

When working on this project, follow these mandatory guidelines:

### Sub-Agent Delegation
- **Use sub-agents** for all discrete tasks (file searches, code analysis, tests, builds)
- **Conserve context window** by delegating work rather than doing it directly
- The orchestrator's role is to **coordinate and supervise**, not execute every step

### Model Selection for Sub-Agents
Choose the appropriate model for each task:

| Task Complexity | Model | Examples |
|-----------------|-------|----------|
| Simple/Fast | `haiku` | File searches, grep, simple reads, status checks |
| Medium | `sonnet` | Code analysis, debugging, test execution |
| Complex | `opus` | Architecture decisions, complex refactoring |

### Task Patterns
1. **Exploration tasks** → Use Explore agent with haiku/sonnet
2. **Build/Test tasks** → Use Bash agent with haiku
3. **Code investigation** → Use general-purpose agent with sonnet
4. **Planning** → Use Plan agent with sonnet/opus

## Project Context

### MDEMG (Multi-Dimensional Emergent Memory Graph)
A persistent memory system for LLMs providing:
- Vector-based semantic search
- Graph-based knowledge representation
- Hidden layer concept abstraction
- Learning edges (Hebbian reinforcement)
- LLM re-ranking for improved retrieval

### Key Directories
- `internal/retrieval/` - Core retrieval algorithms
- `internal/hidden/` - Hidden layer/concept abstraction
- `internal/api/` - HTTP API handlers
- `docs/tests/` - Benchmark tests and results

### Current Issues (as of 2026-01-23)
- Benchmark scores dropped from 0.710 to 0.522 (-26.5%)
- Layer 1 (hidden) nodes showing 0.00/10 in results
- Investigation needed for hidden layer retrieval

## Testing
- Benchmark tests in `docs/tests/whk-wms/`
- Run V11 test: `python3 docs/tests/whk-wms/run_mdemg_test_v11_alltracks.py`
- Question file: `test_questions_v4_selected.json` (100 questions, seed 42)
