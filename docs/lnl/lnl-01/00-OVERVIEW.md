# Lunch & Learn #01: MDEMG Test Specification Frameworks

**Date:** 2026-02-06
**Duration:** 1 hour
**Presenter:** [Your Name]
**Audience:** Development Teams

---

## Agenda

| Time | Section | Duration |
|------|---------|----------|
| 0:00 | Welcome & MDEMG Deep Dive | 15 min |
| 0:15 | UPTS: Universal Parser Test Specification | 18 min |
| 0:33 | UATS: Universal API Test Specification | 17 min |
| 0:50 | Q&A | 10 min |

---

# Part 1: What is MDEMG?

## The Core Problem: LLMs Have No Memory

When you work with AI coding assistants (Claude, GPT, Copilot), you face a fundamental limitation:

```
┌──────────────────────────────────────────────────────────────────┐
│  Session 1                    │  Session 2                       │
│  ─────────                    │  ─────────                       │
│  "Our auth uses JWT tokens    │  "How does our auth work?"       │
│   stored in Redis with a      │                                  │
│   24-hour TTL..."             │  🤷 "I don't know your system"   │
│                               │                                  │
│  [Context window fills up]    │  [Starts from scratch]           │
└──────────────────────────────────────────────────────────────────┘
```

**Every session starts from zero.** The AI doesn't remember:
- What you discussed yesterday
- How your specific codebase works
- Decisions made and why
- Problems encountered and solutions found

---

## The Real Cost of No Memory

### 1. Constant Re-Explanation
> "No, we don't use REST for internal services. We use gRPC. I told you this last week."

### 2. Lost Institutional Knowledge
> "Why did we structure the auth service this way? The person who made that decision left 2 years ago."

### 3. Repeated Mistakes
> "We tried that approach before. It caused production outages. But nobody documented it."

### 4. Multi-Agent Chaos
> Agent A: "I'm refactoring the user service"
> Agent B: "I'm also refactoring the user service"
> [Merge conflicts intensify]

---

## MDEMG: The Solution

**Multi-Dimensional Emergent Memory Graph**

> *A cognitive substrate for AI-assisted development*

MDEMG provides LLMs with what humans have naturally: **persistent, evolving memory**.

### The Internal Dialog Analogy

When humans think through problems, they draw on:
- Past experiences and how they handled similar situations
- Domain expertise accumulated over years
- Relationships between concepts that aren't universally known
- The specific context of their work environment

**MDEMG gives AI agents this same capability** — a persistent "inner voice" of accumulated domain knowledge.

---

## What MDEMG Does NOT Store

This is critical to understand:

| Do NOT Store | Why |
|--------------|-----|
| Python syntax | LLM already knows this |
| How React hooks work | Universally available documentation |
| General best practices | Already in training data |
| Standard library APIs | LLM has this knowledge |
| Common design patterns | Well-documented elsewhere |

**Rule of thumb:** If you could find it on Stack Overflow or in official docs, it doesn't belong in MDEMG.

---

## What MDEMG DOES Store

**Domain-specific, organization-specific, task-specific knowledge:**

| Category | Examples | Why It Belongs |
|----------|----------|----------------|
| **Organizational Code Patterns** | "We use Repository pattern for data access in this codebase" | Specific to your org |
| **Architectural Decisions** | "We chose Redis over Memcached because of X incident" | Institutional knowledge |
| **Domain Procedures** | P&ID sequences, PLC logic, safety interlock documentation | Not available anywhere else |
| **Project Context** | Which APIs are deprecated, why certain workarounds exist | Tribal knowledge |
| **Problem/Solution History** | "Last time we saw this error, the root cause was X" | Debugging history |
| **Team Conventions** | PR review expectations, deployment checklists | Process knowledge |

---

## How MDEMG Works: The Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         MDEMG Server (Go)                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐  │
│   │  Retrieval      │   │  Learning       │   │  Conversation   │  │
│   │  Pipeline       │   │  System         │   │  Memory (CMS)   │  │
│   │                 │   │                 │   │                 │  │
│   │  • Vector search│   │  • Hebbian edges│   │  • Observations │  │
│   │  • Graph hops   │   │  • Decay/prune  │   │  • Session state│  │
│   │  • LLM rerank   │   │  • Reinforcement│   │  • Recall/resume│  │
│   └────────┬────────┘   └────────┬────────┘   └────────┬────────┘  │
│            │                     │                     │            │
│            └─────────────────────┼─────────────────────┘            │
│                                  ▼                                  │
│   ┌─────────────────────────────────────────────────────────────┐  │
│   │                        Neo4j Graph DB                        │  │
│   │  • Nodes: Observations, Concepts, Hidden Aggregators        │  │
│   │  • Edges: CO_ACTIVATED_WITH, ABSTRACTS_TO, RELATES_TO       │  │
│   │  • Vector Index: Semantic similarity search                 │  │
│   └─────────────────────────────────────────────────────────────┘  │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│  Embeddings: OpenAI / Ollama          Plugins: Ingestion, Reasoning │
└─────────────────────────────────────────────────────────────────────┘
```

---

## The Emergent Layer Architecture

MDEMG doesn't just store flat memories — it **builds hierarchical understanding**:

```
Layer 5  [Emergent Principles]     ← "System Architecture Patterns"
    ↑    Emerges automatically
Layer 4  [Abstract Concepts]       ← "Cross-Cutting Concerns"
    ↑    Emerges automatically
Layer 3  [Domain Concepts]         ← "Security Infrastructure"
    ↑    Emerges automatically
Layer 2  [Patterns]                ← "Authentication Service"
    ↑    Emerges automatically
Layer 1  [Hidden Aggregators]      ← Related method groups
    ↑    DBSCAN clustering
Layer 0  [Base Observations]       ← Raw ingested code/conversations
```

**Key insight:** Higher-level concepts **emerge automatically** through Hebbian learning.
> "Neurons that fire together, wire together"

---

## Conversation Memory System (CMS)

The CMS is how MDEMG maintains continuity across sessions:

### On Session Start: Resume
```bash
POST /v1/conversation/resume
{
  "space_id": "your-project",
  "session_id": "claude-core",
  "max_observations": 20
}
```
Returns: Recent observations, active themes, emergent concepts

### During Session: Observe
```bash
POST /v1/conversation/observe
{
  "space_id": "your-project",
  "session_id": "claude-core",
  "content": "User prefers conventional commits",
  "obs_type": "preference"
}
```
Captures: Decisions, corrections, learnings, preferences, errors

### The Result: Continuous Memory
- Session 1: Learn user preferences
- Session 2: **Already knows** user preferences
- Session N: Accumulated understanding grows

---

## Key Capabilities

| Capability | What It Does | Impact |
|------------|--------------|--------|
| **Codebase Ingestion** | Parse 22 languages, extract symbols | "What does function X do?" → Instant answer |
| **Semantic Retrieval** | Vector + graph + LLM reranking | Find relevant code by meaning, not keywords |
| **Conversation Memory** | Persist observations across sessions | No more re-explaining context |
| **Hebbian Learning** | Strengthen frequently co-activated concepts | Patterns emerge automatically |
| **Multi-Agent Coordination** | Shared memory substrate | Agents don't duplicate work |
| **Gap Detection** | Identify missing knowledge | Know what you don't know |

---

## Extensibility: Integrations & Sidecar Modules

MDEMG uses a **plugin-based architecture** for extensibility. Modules run as **sidecar processes** that communicate via gRPC over Unix sockets.

### Why Sidecar Architecture?

```
┌─────────────────┐         gRPC/Unix Socket         ┌─────────────────┐
│                 │ ◄──────────────────────────────► │   Plugin        │
│   MDEMG Core    │                                  │   (any language)│
│                 │ ◄──────────────────────────────► │   Plugin        │
└─────────────────┘                                  └─────────────────┘
```

- **Loose coupling** — Plugins run as separate processes
- **Language agnostic** — Any language with gRPC support (Go, Python, Rust, etc.)
- **Fault isolation** — Plugin crashes don't affect the core
- **Hot reloading** — Update plugins without restarting MDEMG

### Three Module Types

| Type | Purpose | Examples |
|------|---------|----------|
| **INGESTION** | Parse external sources into observations | Linear issues, Obsidian notes, Jira tickets |
| **REASONING** | Re-rank/filter retrieval results | Keyword boosters, domain-specific scoring |
| **APE** | Background autonomous tasks | Reflection, consistency checks, gap analysis |

### Creating a Module

Each module needs:
1. **`manifest.json`** — Declares capabilities, type, health check intervals
2. **gRPC implementation** — Handles lifecycle + type-specific RPCs
3. **Binary** — Standalone executable in `/plugins/` directory

```json
// Example manifest.json
{
  "id": "linear-module",
  "type": "INGESTION",
  "capabilities": {
    "ingestion_sources": ["linear://"]
  }
}
```

*Full plugin SDK documentation: `docs/development/SDK_PLUGIN_GUIDE.md`*

---

## The Testing Challenge

With this much functionality:
- **22 language parsers** — each must extract symbols correctly
- **45+ API endpoints** — each must honor its contract
- **Multiple integration points** — plugins, hooks, scheduled jobs

**How do we ensure it all works?**

---

## The Solution: Specification-Driven Testing

### UPTS — Universal Parser Test Specification
> "This is what the Kotlin parser MUST extract from this file"

### UATS — Universal API Test Specification
> "This is what POST /v1/memory/retrieve MUST return"

**Benefits:**
- Single JSON schema = canonical definition
- Self-documenting specs
- Easy CI integration (`make test-parsers`, `make test-api`)
- Clear workflow for adding new languages/endpoints

---

## Current Stats

| Metric | Value |
|--------|-------|
| Total parsers | 22 (Go, Rust, Python, TypeScript, Java, C#, Kotlin, C++, C, CUDA, SQL, Cypher, Terraform, YAML, TOML, JSON, INI, Makefile, Dockerfile, Shell, Markdown, XML) |
| UPTS-validated | 20 parsers |
| API endpoints | 45+ |
| UATS test variants | ~90 |
| Pass rate | 100% |

---

## Why This Matters For You

### As Developers:
- You can add new parsers with a clear process
- You can add new endpoints with confidence
- Tests catch regressions before they ship

### As Users of AI Assistants:
- MDEMG powers the memory behind your AI tools
- Better memory = less re-explaining
- Your organizational knowledge is preserved

### As a Team:
- Tribal knowledge becomes explicit
- New team members ramp up faster
- AI agents can actually help (with context)

---

## What We'll Cover Today

### 1. UPTS Deep Dive (18 min)
- How language parsers are validated
- Creating a new parser + fixture + spec
- Walkthrough: Adding a hypothetical Zig parser
- Q&A prep

### 2. UATS Deep Dive (17 min)
- How API endpoints are validated
- Creating a new endpoint test spec
- Walkthrough: Adding a new endpoint spec
- Q&A prep

---

## Prerequisites for Hands-On

If you want to follow along:

```bash
# Clone and setup
cd ~/mdemg

# For UPTS
go build ./cmd/ingest-codebase/...

# For UATS
pip install requests jsonpath-ng

# Start server (for UATS)
./bin/server &
```

---

## Let's Begin!

**Next:** [UPTS Deep Dive](./01-UPTS-DEEP-DIVE.md)

---

## Appendix: Key Links

| Resource | Path |
|----------|------|
| MDEMG Vision | `VISION.md` |
| Architecture Docs | `docs/architecture/` |
| UPTS Specs | `docs/lang-parser/lang-parse-spec/upts/specs/` |
| UATS Specs | `docs/api/api-spec/uats/specs/` |
| Parser Implementations | `cmd/ingest-codebase/languages/` |
| API Handlers | `internal/api/` |
