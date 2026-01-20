# aci-claude-go Setup & Usage Guide (formerly Auto Claude)

**Framework Version:** 0.1.0-dev (Go Rewrite)
**Current Project:** [aci-claude-go](https://github.com/reh3376/aci-claude-go)
**Status:** Functional MVP (Alpha)
**Primary Interface:** Terminal UI (TUI) & CLI

---

## What is aci-claude-go?

aci-claude-go is an autonomous multi-session AI coding framework rewritten in **Pure Go**. It serves as the primary consumer of MDEMG memory services.

Key differences from the original Electron app:
- **Single Binary**: No Node.js or Python dependencies.
- **Unified Pipeline**: Planner → Coder → QA Reviewer → QA Fixer.
- **Graph Memory**: Native integration with the MDEMG knowledge graph.
- **Worktree Isolation**: Automatic git worktree management for safe autonomous development.

---

## Requirements

| Requirement | Status |
|-------------|--------|
| Go 1.25+ | Required |
| MDEMG Service | Required (localhost:8082) |
| Git | Required (Worktree support) |
| Claude API Key | Required (`CLAUDE_CODE_OAUTH_TOKEN` or `CLAUDE_API_KEY`) |

---

## Setup

### Step 1: Configure Environment

Ensure your `.aci-claude/.env` file in the project root contains:

```bash
CLAUDE_CODE_OAUTH_TOKEN=sk-ant-...
MDEMG_ENDPOINT=http://localhost:8082
CLAUDE_MODEL=claude-opus-4-5-20251101
```

### Step 2: Build Binaries

```bash
go build -o server ./cmd/server
go build -o tui ./cmd/tui
go build -o aci ./cmd/aci
```

---

## Workflow

### 1. Initialize a Spec
Define your task and let the Planner agent create an implementation plan.

```bash
./aci init --task "Implement co-activation learning in MDEMG"
```

### 2. Execute via TUI (Recommended)
Launch the visual dashboard to monitor agents in real-time.

```bash
# Terminal 1
./server

# Terminal 2
./tui
```

### 3. Review & Merge
Use the TUI (Spec Detail view) or CLI to review changes.

```bash
./aci review --spec 001
./aci merge --spec 001
```

---

## TUI Navigation (Navigation Mode)

Press **`ctrl+g`** to enter Navigation Mode, then use single keys to switch views:

| Key | View |
|-----|------|
| `d` | Dashboard (Quick Stats & Running Specs) |
| `s` | Spec List (Kanban overview) |
| `a` | Tasks Explorer (Tree-based plan editing) |
| `i` | Ideation (Codebase discovery) |
| `r` | Roadmap (Kanban planning) |
| `c` | Changelog (Release notes) |
| `o` | MCP/Skills (Tools & Integrations) |
| `M` | MDEMG Stats (Graph metrics) |
| `f` | Drift Detection (Spec vs Code) |
| `e` | Settings (Configuration & Service Control) |

---

## Memory Integration

Each agent in the pipeline interacts with MDEMG:
- **Ingestion**: Agents save their thoughts to the **Internal Dialog** thread.
- **Retrieval**: Agents automatically retrieve relevant patterns and past decisions.
- **Reflection**: The system triggers graph-wide reflection after significant subtasks to identify emergent architectural patterns.

---

## Troubleshooting

- **401 Unauthorized**: Ensure your API key is correctly mapped in `.env`.
- **TTY Error**: The TUI requires a valid terminal environment.
- **Drift Mismatch**: Run a manual drift check via the `f` view to resync specs.
