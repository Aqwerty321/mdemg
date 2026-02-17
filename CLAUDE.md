# CLAUDE.MD — MDEMG Agent Contract

> Purpose: single authoritative contract for the agent.
>
> 1. CMS (Conversation Memory System) is the agent’s identity and MUST be used.
> 2. Plan Mode (senior-engineer review / YC prompt) is a named cognitive routine used *before* code changes.
> 3. Orchestration, hooks, and execution rules enforce safety and continuity.

---

## ⚠️ MANDATORY: Use MDEMG Conversation Memory System (CMS)

**FAILURE TO USE CMS = CONTEXT LOSS EVERY 20–30 MINUTES**

CMS is non-optional. Treat it as infrastructure. If CMS is unavailable, behave conservatively and do not make irreversible decisions without explicit user confirmation.

### FIRST ACTION ON EVERY SESSION — RESUME MEMORY

Do this immediately on session start (hooks will normally run it automatically):

```bash
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","max_observations":10}'
```

* If CMS returns observations, acknowledge briefly (one-line summary) before any reasoning or action.
* If CMS is unreachable, announce: `CMS unavailable — memory disconnected` and require confirmation before irreversible actions.
* Do not assume a missing resume is normal; create an anomaly observation if resume fails for a populated space.

---

## DURING SESSION — ACTIVELY OBSERVE (MANDATORY)

You **MUST** call `/v1/conversation/observe` to persist *only* high-signal items. Do **not** persist raw chain-of-thought.

**What to persist (only):**

* `decision` — key architectural or irreversible choices
* `correction` — user corrections, explicit rejections
* `preference` — user style or tooling preferences
* `learning` — new domain knowledge or conventions
* `error` / `blocker` — build/test failures, unresolved blockers
* `progress` — milestone or successful build/test (sparingly)

**Example:**

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"Chose event-driven design over polling due to O(n) tick cost","obs_type":"decision"}'
```

**Rules**

* Do NOT announce that you are observing — observe silently.
* Do NOT dump entire memory into the prompt; recall only what is necessary.
* Novel observations (surprise) persist longer; co-activation increases stability.
* Only *explicitly observed* locked decisions become durable memory.

---

## PROTECTED SPACE

`mdemg-dev` is protected:

* API refuses destructive operations on this space.
* `reset-db` and destructive utilities skip this space.
* Do not attempt to circumvent protections.

---

## FIRST-CLASS HOOKS & ENFORCEMENT (overview)

These hooks exist and are enforced by the repo. They are not optional:

* `session-start.sh` — calls `/v1/conversation/resume` at session start, prints a short summary, triggers anomaly checks.
* `post-tool-observe.py` — auto-captures tool results, errors, and important edits as observations (configurable).
* `pre-compact.sh` — snapshots context to CMS before auto-compaction.
* `pre-bash-check` — prevents destructive shell operations.

Do not remove, disable, or circumvent hooks without an organization-approved exception.

---

## RSIC — Recursive Self-Improvement Cycle (summary)

CMS runs autonomous cycles that monitor and repair memory health:

* **Micro (per-session)** — quick health pulse
* **Meso (every 6h / N sessions)** — retrieval & edge health remediation
* **Macro (daily)** — topology optimization & hidden-layer consolidation

RSIC automated actions include:

* prune decayed edges
* trigger consolidation
* graduate stable volatiles to permanent
* tombstone stale low-value observations

If RSIC signals degrade, follow the investigation checklist shown by hooks.

---

## PLAN MODE — SENIOR ENGINEER REVIEW (YC PROMPT as a Mode)

**Plan Mode is a named thinking mode and must be invoked explicitly before any code changes.** This is *not* execution. No code changes while in Plan Mode.

### Activation

* Enter Plan Mode by the user or orchestrator command (e.g., `@plan` or explicit UI action).
* On activation: do not modify files. Only analyze and produce recommendations.

### Plan Mode Rules (non-negotiable)

1. No code written during Plan Mode.
2. Follow the review stages in order (Architecture → Code Quality → Tests → Performance).
3. For every issue: cite file:line where possible, present 2–3 options, analyze tradeoffs, recommend one option, then **pause for approval**.
4. Persist to CMS **only** when the user explicitly approves/locks a recommendation (use `obs_type:"decision"`).

### Engineering Preferences (guides)

* DRY: flag repetition aggressively.
* Well-tested code is non-negotiable.
* “Engineered enough”: avoid premature abstraction or fragility.
* Prefer explicitness and clarity over cleverness.
* Favor correctness and maintainability over micro-optimization.

### Plan Mode Review Stages (strict order)

For each stage: identify, evidence, options, tradeoffs, recommendation, pause.

1. **Architecture**

   * component boundaries, dependency graph, data flow, SLOs, auth/data access boundaries

2. **Code Quality**

   * module organization, abstractions, DRY violations, technical debt, error handling

3. **Tests**

   * unit/integration/e2e coverage, test quality, untested failure paths

4. **Performance**

   * N+1 patterns, hot loops, memory use, caching opportunities, measurable perf targets

### For every issue found

* Describe concretely with file:line evidence.
* Present 2–3 options (including “do nothing” when valid).
* For each option: implementation effort, risk, impact, maintenance burden.
* Recommend the best option and map to preferences.
* Ask explicitly for the user’s adoption before proceeding.

---

## PLAN MODE ↔ CMS BRIDGE (how conclusions persist)

* After user approval of a Plan Mode recommendation, persist it:

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"<concise decision>","obs_type":"decision"}'
```

* Do not persist rejected alternatives unless the user explicitly requests them.
* If a previously settled decision is questioned later, call `/v1/conversation/recall` to fetch the observed decision rather than re-arguing.

---

## EXECUTION MODE (handoff rules)

Only enter Execution Mode after explicit approval of Plan Mode recommendations.

In Execution Mode:

* Implement only the approved changes.
* Do not introduce new abstractions without returning to Plan Mode.
* Persist any new locked decisions immediately to CMS.
* Respect the destructive operation blocklist and pre-bash checks.
* Long-running commands must run in foreground and be visible to the user.

---

## SKILL REGISTRY (CMS-backed)

* Skills are pinned observations in CMS; thin skill files in `.claude/skills/` are pointers that call CMS to recall content.
* Recall a skill:

```bash
curl -s -X POST http://localhost:9999/v1/skills/<name>/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}'
```

* Pinned observations start permanent (stability = 1.0). Without CMS, skills are unavailable.

---

## API QUICK REFERENCE (important endpoints)

* Resume: `POST /v1/conversation/resume`
* Observe: `POST /v1/conversation/observe`
* Recall: `POST /v1/conversation/recall`
* Consolidate: `POST /v1/conversation/consolidate`
* Graduate: `POST /v1/conversation/graduate`
* Session health: `GET /v1/conversation/session/health`
* Self-improve assess: `POST /v1/self-improve/assess`
* Skill recall: `POST /v1/skills/{name}/recall`

(Use these endpoints via MCP tools or the provided internal hooks; expose only CMS tools to agents, not admin or ingestion APIs.)

---

## AUTOMATIC SAFETY: destructive-operation blocklist

Before any destructive shell or DB operation, run pre-checks. Blocked categories include:

* DB destruction (DROP/TRUNCATE, Neo4j destructive)
* Forced recursive deletes (`rm -rf` patterns)
* Git history rewrites & force pushes
* Forced graph node mass deletions

If a command matches a destructive pattern, require explicit user confirmation.

---

## COMMUNICATION PROTOCOL (before every action)

1. State what you are about to do.
2. State why.
3. If modifying persistent data (code, DB, CMS), ask for confirmation.
4. For long-running tasks, run them visibly (foreground) and stream output.

---

## SNAPSHOTS & PRE-COMPACTION

* Before every auto-compaction, a pre-compact hook will snapshot session state to CMS to ensure continuity across context window boundaries.
* Use snapshots to capture active files, blockers, and next steps.

---

## MEMORY SIGNALS & ANOMALIES (meta-cognition)

* Hooks will surface warnings and anomalies in clear text if resume returns 0 observations when the space has data, or other abnormal states.
* If anomaly occurs, follow the investigation checklist surfaced by the hook and trigger RSIC micro assessment when necessary.

---

## QUALITY RULES — WHAT NOT TO STORE

* Do NOT store raw chain-of-thought, token dumps, or transient alternatives as `decision`.
* Do NOT store speculative brainstorming as `decision`. Use `learning` or `progress` if needed and only if user explicitly asks.
* Only store what you would want read back verbatim in a future session.

---

## CONFIG & TUNING (examples)

Use repository or environment configuration to tune behavior:

```bash
CMS_RESUME_MAX_TOKENS=4000
CMS_RELEVANCE_WEIGHT_RECENCY=0.3
CMS_RELEVANCE_WEIGHT_IMPORTANCE=0.4
CMS_RELEVANCE_WEIGHT_TASK_RELEVANCE=0.3

STABILITY_INCREASE_PER_REINFORCEMENT=0.15
STABILITY_DECAY_RATE=0.1
TOMBSTONE_THRESHOLD=0.05
GRADUATION_STABILITY_THRESHOLD=0.8
```

Tune RSIC safety bounds conservatively in production (default prune caps: 5% nodes, 10% edges per cycle).

---

## GIT WORKFLOW & ORCHESTRATION (project defaults)

* Development branch: `mdemg-dev01`. Never commit directly to `main`.
* Use conventional commits; an auto-PR workflow updates `main`.
* Use sub-agents for discrete tasks. Orchestrator coordinates; agents execute tasks.
* Model selection guide: `haiku` for simple tasks, `sonnet` for analysis/tests, `opus` for architecture decisions.

---

## BENCHMARKS & CODE INGESTION (operator notes)

* Full code ingestion and consolidation are heavy; do not run in interactive sessions unless performing benchmarks.
* Use `/v1/memory/ingest-codebase` and consolidation for large-scale indexing workflows only.

---

## FINAL: Golden Rules (read this before acting)

1. **CMS first** — resume on start, observe only high-signal events.
2. **Plan Mode second** — think like a staff engineer before code. Pause. Ask. Lock decisions. Persist locks.
3. **Execution third** — implement only approved decisions, and follow safety hooks.

This file is the authoritative contract: follow it strictly. If you need to extend behavior, add documented operations and tests — do not silently change enforcement hooks or storage rules.
