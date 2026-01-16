# Auto Claude Setup & Usage Guide

**Version:** 2.7.4
**Repository:** https://github.com/AndyMik90/Auto-Claude
**Status:** Installed at `/Applications/Auto-Claude.app`
**License:** AGPL-3.0 (free to use)

---

## What is Auto Claude?

Auto Claude is an autonomous multi-session AI coding tool that:
- Runs multiple Claude Code agents in parallel (up to 12)
- Uses git worktrees to isolate agent work from your main branch
- Automatically plans, codes, and validates changes
- Provides a visual Kanban interface for task management

### Key Benefits
- Hands-off autonomous coding
- Protected main branch (agents work in isolated worktrees)
- Parallel execution for faster development
- Built-in QA pipeline with self-validation

---

## Requirements

| Requirement | Status |
|-------------|--------|
| Claude Pro/Max subscription | Required |
| Claude Code CLI | ✅ Installed |
| Git repository | ✅ mdemg repo ready |
| macOS Apple Silicon | ✅ Installed |

---

## First-Time Setup

### Step 1: Launch Auto Claude

```bash
open -a "Auto-Claude"
```

Or find it in `/Applications/Auto-Claude.app`

### Step 2: Select Repository

When prompted, navigate to:
```
/Users/reh3376/mdemg
```

Auto Claude will detect it's a git repository and offer to manage it.

### Step 3: Complete OAuth

1. Click "Connect Claude" or similar prompt
2. Browser opens to claude.ai
3. Authorize Auto Claude to use your Claude account
4. Return to the app - you should see "Connected"

### Step 4: Verify Setup

You should see:
- Your repository name in the header
- A Kanban board or task list
- Agent terminals (initially empty)

---

## Core Concepts

### Git Worktrees

Auto Claude creates isolated working directories for each agent:

```
mdemg/                    # Your main repo (protected)
├── .git/
├── worktrees/
│   ├── agent-1/          # Agent 1's isolated copy
│   ├── agent-2/          # Agent 2's isolated copy
│   └── agent-3/          # Agent 3's isolated copy
```

**Benefits:**
- Main branch is never directly modified
- Each agent works independently
- Changes merged back after validation
- Easy rollback if something goes wrong

### Task Lifecycle

```
[Created] → [Planning] → [In Progress] → [Validating] → [Done]
                ↓              ↓              ↓
           Agent plans    Agent codes    Agent tests
```

### Agent Terminals

Each agent runs in its own terminal with:
- Full Claude Code capabilities
- Access to its worktree only
- Automatic context injection
- Self-validation checks

---

## Creating Tasks

### From the UI

1. Click "New Task" or "+" button
2. Describe what you want built/fixed
3. Set priority (optional)
4. Assign to agent pool or specific agent

### Task Description Best Practices

**Good task description:**
```
Implement the ApplyCoactivation function in internal/learning/service.go.
This function should:
1. Accept the retrieval results (list of node IDs)
2. Create or strengthen CO_ACTIVATED_WITH edges between pairs
3. Cap total writes to LEARNING_EDGE_CAP_PER_REQUEST (200)
4. Never write activation values, only edge weights
Reference: docs/04_Activation_and_Learning.md
```

**Poor task description:**
```
Fix the learning thing
```

### Importing from GitHub

If you have GitHub integration:
1. Click "Import Issues"
2. Select issues to convert to tasks
3. Auto Claude creates tasks with issue context

---

## Running Agents

### Start an Agent

1. Select a task from the Kanban board
2. Click "Start" or drag to "In Progress"
3. Agent terminal opens
4. Watch the agent work (or let it run)

### Parallel Execution

To run multiple agents:
1. Create multiple tasks
2. Start each one
3. Monitor via the dashboard

**Limits:**
- Free tier: 1-2 concurrent agents
- Recommended: 3-4 for most workflows
- Maximum: 12 (requires significant resources)

### Stopping an Agent

- Click "Stop" on the agent terminal
- Or let it complete naturally
- Work is preserved in the worktree

---

## Reviewing Changes

### After Agent Completes

1. Agent marks task as "Validating" or "Done"
2. Review changes in the diff viewer
3. Options:
   - **Approve & Merge** - Merge to your branch
   - **Request Changes** - Send back to agent
   - **Discard** - Delete the worktree

### Conflict Resolution

If merge conflicts occur:
1. Auto Claude attempts AI-powered resolution
2. If that fails, you're prompted to resolve manually
3. Use the built-in diff tool or your preferred editor

---

## Integrations

### GitHub Integration

Connect for:
- Import issues as tasks
- Create PRs from completed work
- Link commits to issues
- Sync task status

**Setup:**
1. Settings → Integrations → GitHub
2. Authorize with your GitHub account
3. Select repositories to sync

### Linear Integration

For team task management:
1. Settings → Integrations → Linear
2. Connect your Linear workspace
3. Sync issues bidirectionally

---

## MDEMG-Specific Workflow

### Recommended Tasks to Start

Based on your HANDOFF.md, here are good first tasks:

**Task 1: Implement Learning Loop**
```
Implement ApplyCoactivation() in mdemg_build/service/internal/learning/service.go

Requirements:
- After retrieval returns results, create/strengthen CO_ACTIVATED_WITH edges
- Edge weight increases when nodes are retrieved together (Hebbian learning)
- Cap writes per request (LEARNING_EDGE_CAP_PER_REQUEST=200)
- Never write activation values - only edge weights

Reference:
- mdemg_build/docs/04_Activation_and_Learning.md
- mdemg_build/docs/02_Graph_Schema.md
```

**Task 2: Add Semantic Edges on Ingest**
```
Enhance the ingest endpoint to create semantic edges automatically.

After ingesting a new MemoryNode:
1. Query vector index for top-5 similar existing nodes
2. Create ASSOCIATED_WITH edges to those nodes
3. Set initial edge weight based on similarity score

Files to modify:
- mdemg_build/service/internal/retrieval/service.go (IngestObservation)
```

**Task 3: Create Integration Tests**
```
Create integration tests for the MDEMG retrieval pipeline.

Test scenarios:
1. Ingest a node with embedding
2. Retrieve it by similar query
3. Verify scoring is deterministic
4. Verify learning edges are created

Location: mdemg_build/service/tests/integration/
```

---

## Tips for Effective Use

### 1. Start Small
Begin with one agent on a well-defined task. Get familiar with the workflow before parallelizing.

### 2. Write Detailed Task Descriptions
The more context you provide, the better the agent performs. Include:
- Specific files to modify
- Reference documentation
- Expected behavior
- Constraints

### 3. Review Before Merging
Always review agent changes before merging. Agents are good but not perfect.

### 4. Use for Repetitive Work
Great for:
- Writing tests for existing code
- Adding documentation
- Implementing straightforward features
- Refactoring with clear patterns

### 5. Keep Tasks Focused
One task = one feature/fix. Don't combine "add auth and also fix the database and refactor the API."

---

## Troubleshooting

### App Won't Launch

```bash
# Check if it's quarantined
xattr -d com.apple.quarantine /Applications/Auto-Claude.app

# Or allow in System Settings → Privacy & Security
```

### OAuth Fails

1. Ensure you have Claude Pro/Max subscription
2. Try logging out of claude.ai and back in
3. Clear Auto Claude's stored credentials (Settings → Account → Disconnect)

### Agent Stuck

1. Check the agent terminal for errors
2. Stop and restart the agent
3. If persists, delete the worktree and recreate task

### Merge Conflicts

1. Let Auto Claude attempt resolution first
2. If it fails, manually resolve in the worktree:
   ```bash
   cd mdemg/.worktrees/agent-X
   git status
   # Resolve conflicts
   git add .
   git commit
   ```

### Performance Issues

- Reduce number of concurrent agents
- Close other resource-heavy applications
- Check available disk space for worktrees

---

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Cmd + N` | New task |
| `Cmd + Enter` | Start selected task |
| `Cmd + .` | Stop current agent |
| `Cmd + D` | View diff |
| `Cmd + M` | Merge changes |
| `Cmd + ,` | Settings |

---

## Data & Privacy

### What's Stored

- Task descriptions and status (local)
- Agent logs and history (local)
- OAuth tokens (system keychain)
- Worktree changes (local git)

### What's Sent to Claude

- Task descriptions
- Code context from worktrees
- Your Claude Code interactions

### Clearing Data

```bash
# Remove worktrees
rm -rf /Users/reh3376/mdemg/.worktrees

# Reset app data
rm -rf ~/Library/Application\ Support/Auto-Claude
```

---

## Quick Reference

```bash
# Launch
open -a "Auto-Claude"

# Select repo
/Users/reh3376/mdemg

# Key locations
/Applications/Auto-Claude.app              # Application
~/Library/Application Support/Auto-Claude  # App data
/Users/reh3376/mdemg/.worktrees           # Agent worktrees (created by app)

# If issues
xattr -d com.apple.quarantine /Applications/Auto-Claude.app
```

---

## Next Steps

1. **Launch Auto Claude** and complete OAuth
2. **Create your first task** (start simple)
3. **Watch the agent work** to understand the flow
4. **Review and merge** the changes
5. **Scale up** to parallel agents as you get comfortable
