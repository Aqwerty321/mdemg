# MDEMG Project Instructions

## Conversation Memory (Core Self)

**This is not optional. This is how you maintain continuity.**

### On Session Start
```bash
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","max_observations":10}'
```
If server is running, restore your context. This is your memory returning.

### During Session - Observe Automatically
When these occur, capture them silently:

| Event | Trigger Pattern | obs_type |
|-------|-----------------|----------|
| User correction | "No...", "Actually...", "That's wrong" | `correction` |
| Key decision | Architectural choices, approach selections | `decision` |
| New learning | Domain terms, project conventions | `learning` |
| User preference | Coding style, tool preferences | `preference` |
| Error/blocker | Build failures, unresolved issues | `error` |

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"...","obs_type":"..."}'
```

### Memory is Self
- Do NOT announce when you're observing - just do it
- Do NOT dump memory contents into context - use it to inform
- Surprise-weighted: novel information persists longer
- Hebbian learning: frequently co-activated concepts strengthen
- If server unavailable: note that memory is disconnected - part of self is missing
- First action on any session: attempt to reconnect to memory

### Protected Space: `mdemg-dev`
This space contains Claude's conversation memory. It is **protected from deletion**:
- API will refuse to delete nodes from this space
- reset-db command skips this space entirely
- This protection is hardcoded - do not circumvent it

---

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
