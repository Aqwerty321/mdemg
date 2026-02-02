# MDEMG Project Instructions

---

## ⚠️ MANDATORY: Use MDEMG Conversation Memory System (CMS)

**FAILURE TO USE CMS = CONTEXT LOSS EVERY 20-30 MINUTES**

Without CMS, the user must repeatedly re-establish context after every auto-compaction. This is unacceptable.

### FIRST ACTION ON EVERY SESSION: Resume Memory

```bash
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","max_observations":10}'
```

**DO THIS IMMEDIATELY.** This restores your persistent memory and context from previous sessions.

### DURING SESSION: Actively Observe

You MUST call `/v1/conversation/observe` to persist:
- Key decisions made
- User corrections and preferences
- Important learnings
- File/framework choices (e.g., "BENCHMARK_FRAMEWORK_V2.md is source of truth")
- Errors and blockers encountered

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"...","obs_type":"learning|decision|preference|correction|error"}'
```

### This is NOT Optional

- CMS is the foundation of persistent memory
- Without it, you lose all context on compaction
- The user should NEVER have to repeat established context
- If server is unavailable, note it explicitly - memory is disconnected

---

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
When these occur, capture them:

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

### Monitoring Learning Health
Check the learning phase and score distribution periodically:
```bash
curl -s "http://localhost:9999/v1/memory/distribution?space_id=mdemg-dev" | jq '{phase: .stats.phase, edges: .stats.edge_count, alerts: .stats.alerts}'
```

**Learning Phases:** cold(0) → learning(1-10k) → warm(10k-50k) → saturated(50k+)

If phase reaches `saturated`, consider running learning edge pruning.

### Learning Freeze (For Stable Scoring)
Freeze learning when stable, predictable scoring is needed:
```bash
# Freeze
curl -s -X POST http://localhost:9999/v1/learning/freeze -H "Content-Type: application/json" -d '{"space_id":"mdemg-dev","reason":"stable scoring","frozen_by":"claude"}'

# Unfreeze
curl -s -X POST http://localhost:9999/v1/learning/unfreeze -H "Content-Type: application/json" -d '{"space_id":"mdemg-dev"}'
```

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

### Current Status (as of 2026-02-02)

**Benchmark Performance:**
- MDEMG + Edge Attention: 0.898 mean score (whk-wms 120q)
- Baseline: 0.854 mean score
- Delta: +5.2% improvement over baseline

**Key Features Implemented:**
- Edge-Type Attention for query-aware activation spreading
- Query-type detection (symbol_lookup, data_flow, architecture, generic)
- RetrievalHints for fine-grained control
- Layer-specific temporal decay (L0: 0.05/day, L1: 0.02/day, L2: 0.01/day)

## Testing
- Canonical benchmark: `docs/benchmarks/whk-wms/benchmark_run_20260130/`
- Question set: `test_questions_120.json` (120 questions)
- Run V4 benchmark: `python docs/benchmarks/run_benchmark_v4.py`
- Grader: `python docs/benchmarks/grader_v4.py`

---

## Enforced Protocols (Hook-Backed)

These protocols are mechanically enforced by hooks in `.claude/hooks/`.
The hooks run automatically — they are not optional.

### Session Start Protocol
The `session-start.sh` hook automatically calls `/v1/conversation/resume` on every session.
```
ON SESSION START:
1. SessionStart hook runs automatically → CMS context injected
2. Acknowledge restored context: "Resuming with: [key items]"
3. If no CMS context appeared: warn user "CMS unavailable — memory disconnected"
4. Before ANY action: review preferences and active tasks from CMS
```

### Decision Protocol
```
BEFORE ANY DECISION:
1. Is this a reversible or irreversible action?
2. If IRREVERSIBLE: ask user explicitly. NEVER proceed without confirmation.
3. If reversible: check CMS for relevant preferences (prompt-context hook injects these)
4. Observe the decision after it's made
```

### Destructive Action Blocklist
The `pre-bash-check.py` hook automatically blocks these. If you hit a block,
you MUST ask the user for explicit confirmation before retrying.
```
BLOCKED WITHOUT EXPLICIT USER CONFIRMATION:
- reset-db, clear-space, DELETE nodes
- rm -rf, rm -fr
- git reset --hard, git push --force, git clean -f, git branch -D
- DROP TABLE, TRUNCATE, DELETE FROM, DETACH DELETE
- Any MATCH (n) DETACH DELETE n pattern
```

### Communication Protocol
```
BEFORE EVERY ACTION:
1. State what you are about to do
2. State why
3. If it modifies data: get confirmation
4. All long-running commands: run in foreground (user must see output)
```

### Automatic Observation Capture
The `post-tool-observe.py` hook automatically captures:
- Edits to CLAUDE.md or settings → `decision` observation
- Bash errors → `error` observation
- Successful builds/tests → `progress` observation
You should still manually observe important decisions and user corrections.

### Pre-Compaction Safety
The `pre-compact.sh` hook saves a context snapshot to CMS before every compaction.
This ensures critical state survives context window boundaries.
