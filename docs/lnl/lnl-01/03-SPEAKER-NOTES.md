---
title: "Speaker Notes & Timing Guide"
date: 2026-02-06
tags:
  - lnl
  - speaker-notes
  - presentation
aliases:
  - LnL-01 Speaker Notes
---

# Speaker Notes & Timing Guide

> [!abstract] Presenter Reference
> Slide-by-slide talking points, minute-by-minute timing, setup checklist, and Q&A prompts. Keep slides moving — the value is in the **contracts + demos**, not long philosophy.

---

## Pre-Session Setup Checklist

```bash
# 1. Build everything
cd ~/mdemg
go build ./cmd/ingest-codebase/...
go build ./cmd/server/...

# 2. Start server
./bin/server &

# 3. Verify health
curl localhost:9999/healthz

# 4. Confirm UPTS passes
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v 2>&1 | tail -5

# 5. Confirm UATS passes
make test-api 2>&1 | tail -10

# 6. Have these files open in editor:
#    - docs/lang-parser/lang-parse-spec/upts/specs/kotlin.upts.json
#    - docs/api/api-spec/uats/specs/health.uats.json
#    - cmd/ingest-codebase/languages/kotlin_parser.go
```

---

## Timing Overview

| Time | Section | Duration | Slide |
|------|---------|----------|-------|
| 0:00 | Welcome & MDEMG Deep Dive | 15 min | 1-6 |
| 0:15 | UPTS: Parser Testing | 18 min | 7-9 |
| 0:33 | UATS: API Testing | 17 min | 10-11 |
| 0:50 | Q&A | 10 min | 12-13 |

> [!tip] Pacing rule
> If you're running long, cut depth — never cut the demo.

---

## 0:00-0:03 — Opening & Agenda (Slides 1-2)

**Say:**
- "This is about making AI-assisted engineering *repeatable* — not vibes-based."
- "MDEMG is durable memory, and we protect it with two contracts: UPTS and UATS."
- Quick timing overview. "If we go long, we'll cut depth, not the demo."

---

## 0:03-0:07 — The Core Problem: LLMs Have No Memory (Slide 3)

**Say:**
- "LLMs are stateless. Without memory, we re-pay context cost every time."
- "Teams compound the problem: multiple people/agents produce inconsistent truth."

**Pick one concrete example:**
- "Same endpoint described differently across threads."
- "Parser change silently shifts element IDs — retrieval degrades."

> [!quote] Punchline
> "This isn't a minor inconvenience. It fundamentally limits how useful AI can be for real work."

**Transition:** "So what does MDEMG do about that?"

---

## 0:07-0:11 — What MDEMG Is / Is Not (Slides 4-5)

**Say:**
- "A memory layer that stores *durable, high-signal facts* — not everything."
- "Not a transcript store. Not a data lake. Not a replacement for Git or docs."
- "If you store everything, you store garbage. Garbage memory = garbage answers."

> [!quote] Rule of Thumb
> "If you can find it on Stack Overflow, it does not belong in MDEMG."

**What it DOES store:** Org code patterns, architectural decisions (and *why*), domain procedures, problem/solution history, team conventions.

---

## 0:11-0:14 — Architecture & Layers (Slide 6)

**Walk through the diagram:**
1. **Ingestion** — parse, extract, normalize
2. **Memory** — store facts indexed by stable IDs
3. **Retrieval** — pull relevant facts into context at runtime
4. **Learning** — Hebbian edges strengthen with use

> [!quote] Engineering punchline
> "If ingestion drifts, retrieval drifts. So we lock ingestion with UPTS."

**Show the layer diagram briefly:**
- "Higher-level concepts emerge automatically through Hebbian learning."
- "You ingest raw code; MDEMG builds the conceptual hierarchy."

**CMS in 30 seconds:**
- "On session start: resume previous context."
- "During session: observe decisions and learnings."
- "Result: continuous memory across sessions."

---

## 0:14-0:15 — The Testing Challenge (Slide 7)

**Say:**
- "We break things in two ways: parser drift (ingestion changes) and API drift (contract changes)."
- "Worst failures are silent — things 'work' but quality degrades."

> [!quote] Transition
> "UPTS is the ingestion contract. Let's dive in."

---

## 0:15-0:22 — UPTS Overview + Spec Anatomy (Slides 8-9)

**Say:**
- "UPTS defines what it means for a parser to be correct."
- "Cross-language, cross-format, runs in CI."
- "25 parsers, all validated with the same JSON spec format."

**Show a real spec file** (kotlin.upts.json):
- Point out fixture path, expected symbols, config
- "Three components: Parser + Fixture + Spec. The spec is the contract."

**What "done" means (non-negotiables):**
- File-level element always exists
- StableID is deterministic: same input = same IDs
- Diagnostics are structured: timeout/truncate/partial parse

> [!tip] Why it matters
> StableIDs enable diffs, incremental updates, and reliable linking.

---

## 0:22-0:26 — UPTS Live Demo (4 min)

```bash
# Show all tests pass
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v 2>&1 | head -40

# Show single language
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/kotlin -v
```

**Point out:**
- Test output format
- Which symbols were matched
- Pass/fail summary

---

## 0:26-0:31 — UPTS Walkthrough: Adding a Parser (5 min)

**Walk through the Zig example from [[01-UPTS-DEEP-DIVE]]:**

1. **Parser file** (1 min) — "Implement the LanguageParser interface. Key methods: Name, Extensions, ParseFile."
2. **Fixture file** (1 min) — "Representative source file covering all canonical patterns."
3. **Spec file** (2 min) — "Every symbol you expect the parser to find, with line numbers and types."
4. **Run and iterate** (1 min) — "Run the test. If it fails, either fix the parser or fix the spec."

---

## 0:31-0:33 — UPTS Q&A Buffer (2 min)

> "Any questions on UPTS before we move to UATS?"

**If quiet:**
- "Why JSON instead of YAML?" — Better tooling, no whitespace issues
- "What if the parser finds extra symbols?" — Fine with `allow_extra_symbols: true`

---

## 0:33-0:38 — UATS Overview + Spec Anatomy (Slides 10-11)

**Say:**
- "UATS is for APIs what UPTS is for parsers."
- "Each spec defines: the request (method, path, body) and the expected response (status, assertions)."
- "We use JSONPath for assertions — similar to XPath but for JSON."

**Show a real spec file** — start with health.uats.json (simplest), then retrieve.uats.json (complex).

**Key points:**
- Variable substitution with `${var_name}`
- Assertion operators: `equals`, `contains`, `exists`, `type`, `gte`
- Variants for error cases

---

## 0:38-0:42 — UATS Live Demo (4 min)

```bash
# Show health check
make test-api-health

# Show all tests
make test-api 2>&1 | tail -30
```

**Point out:**
- HTTP status codes
- Response times
- Assertion pass/fail
- Summary at end

---

## 0:42-0:48 — UATS Walkthrough + Failure Modes (6 min)

**Walk through the summarize example from [[02-UATS-DEEP-DIVE]]:**
1. Basic structure (1 min)
2. Request + variables (1 min)
3. Assertions (1 min)
4. Variants — "Always test the unhappy paths" (1 min)

**Common failure modes to highlight (Slide 11):**

> [!example] Pick ONE failure story
> 1. **StableID drift** — Retrieval returns different facts after a refactor. Prevention: UPTS fixtures + determinism checks.
> 2. **Silent truncation** — Missing facts from big files. Prevention: structured diagnostics + CI expectations.
> 3. **Dialect edge case** — SQL/Cypher relationships wrong. Prevention: add fixture, tighten parser, gate in CI.

> [!quote] Close the point
> "Our fix pattern is boring on purpose: tighten spec, add fixture, CI enforces."

---

## 0:48-0:50 — Takeaways (Slides 12-13)

**Say:**
- "UPTS protects ingestion. UATS protects APIs."
- "Adding new formats is a standard move: parser + fixture + spec."
- "Before prod: coverage + failure dashboards + CI gates."

> [!success] Five takeaways
> 1. MDEMG solves the "no memory" problem
> 2. Spec-driven testing = self-documenting contracts
> 3. Same pattern for parsers and APIs — learn once, apply everywhere
> 4. Clear workflow: Create → Test → Iterate → Document
> 5. 100% pass rate as CI gate

**Close with a concrete next step:**
- "Who takes JS/JSONC/YAML hardening?"
- "Who owns UATS coverage for memory endpoints?"

---

## 0:50-1:00 — Q&A (10 min)

**If no questions, prompt with:**
1. "Where have you seen silent regressions (parser drift, API drift) bite us?"
2. "Which file type or format is hurting ingestion the most today?"
3. "Which endpoint would you most want locked down by UATS?"
4. "How would MDEMG help with your specific workflows?"

**Prepared answers:**

> [!faq]- How long does it take to add a new parser?
> 2-4 hours for a basic parser once you understand the pattern. We added 4 new parsers (C#, Kotlin, Terraform, Makefile) in one session.

> [!faq]- What about languages with unusual syntax?
> Regex works for most cases. For complex syntax, use tree-sitter or other parsing libraries, then normalize the output.

> [!faq]- Can we use UATS for our own APIs?
> Absolutely. The runner is standalone Python. Copy the schema and runner, adapt to your endpoints.

> [!faq]- What's the maintenance burden?
> Specs need updating when APIs change — that's the point. The spec is the contract. If you change the API, update the spec. It's documentation that verifies itself.

> [!faq]- How does MDEMG compare to RAG?
> RAG retrieves static chunks. MDEMG builds an evolving graph where concepts emerge and relationships strengthen over time. It learns from use, not just from ingestion.

> [!faq]- What's the performance overhead?
> Retrieval adds ~100-300ms depending on complexity. For most use cases, invisible compared to LLM response time.

> [!faq]- Can we use MDEMG with Claude Code / Cursor / other tools?
> Yes — MDEMG has a standard HTTP API. Anything that can make HTTP requests can integrate.

> [!faq]- What are UBTS, USTS, and UOBS?
> Phase 3 production readiness frameworks following the same spec-driven pattern:
> - **UBTS** — Benchmark tests (p50/p95/p99 latency thresholds, throughput)
> - **USTS** — Security tests (auth enforcement, rate limiting, injection protection)
> - **UOBS** — Observability tests (Prometheus metrics, health endpoints, log format)

---

## If Running Short on Time

**Cut (in order):**
1. Detailed Zig walkthrough — reference the doc
2. Multiple UATS variants — show just one
3. Comparison table — they can read it
4. Deep dive on layer architecture

## If Running Long on Time

**Add:**
1. Show Neo4j browser with actual graph data
2. Demonstrate a real codebase ingestion
3. Walk through a failing test and how to debug
4. Live demo of CMS observe/resume cycle

---

## Post-Session

### Share follow-up resources:

- `docs/lnl/lnl-01/` — All presentation materials
- `VISION.md` — MDEMG philosophy and architecture
- `docs/lang-parser/lang-parse-spec/upts/README.md` — Full UPTS documentation
- `docs/api/api-spec/uats/README.md` — Full UATS documentation
- `cmd/ingest-codebase/languages/README.md` — Parser implementation guide

**Production Test Frameworks (Phase 3):**
- `docs/tests/ubts/README.md` — Benchmark testing (latency, throughput)
- `docs/tests/usts/README.md` — Security testing (auth, rate limits, injection)
- `docs/tests/uobs/README.md` — Observability testing (metrics, health, logs)

### Observe the session to CMS:

```bash
curl -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "session_id": "claude-core",
    "content": "Conducted LnL-01: MDEMG overview + UPTS & UATS frameworks. Attendees: [list]. Questions: [summarize]. Materials in docs/lnl/lnl-01/.",
    "obs_type": "progress",
    "tags": ["lnl", "documentation", "training"]
  }'
```
