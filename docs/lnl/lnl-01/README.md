---
title: "LnL-01: MDEMG Overview & Test Specification Frameworks"
date: 2026-02-06
tags:
  - lnl
  - mdemg
  - upts
  - uats
  - testing
  - training
aliases:
  - Lunch and Learn 01
  - LnL-01
---

# LnL-01: MDEMG Overview & Test Specification Frameworks

> [!info] Session Details
> **Date:** 2026-02-06 | **Duration:** 1 hour | **Format:** Presentation + Live Demo + Q&A

---

## Materials

| Doc | What it covers | Est. Time |
|-----|---------------|-----------|
| [[00-OVERVIEW]] | Why MDEMG exists, what it stores, architecture, CMS, the testing challenge | 15 min |
| [[01-UPTS-DEEP-DIVE]] | Parser testing: spec anatomy, running tests, adding a new parser walkthrough | 18 min |
| [[02-UATS-DEEP-DIVE]] | API testing: spec anatomy, running tests, adding a new endpoint walkthrough | 17 min |
| [[03-SPEAKER-NOTES]] | Slide-by-slide talking points, timing, setup checklist, Q&A prompts | — |

---

## What Your Team Will Walk Away With

> [!tip] Key Outcomes
> 1. **Understand what MDEMG solves** — persistent memory for AI agents so they stop asking the same questions
> 2. **Know how to add a language parser** — parser + fixture + spec, run `go test`, iterate
> 3. **Know how to add an API test** — spec + variants, run `make test-api`, iterate
> 4. **Trust the CI gates** — 100% pass rate is the merge requirement

---

## Quick Reference

### MDEMG Fundamentals

- **The Problem:** LLMs forget everything between sessions — your team re-explains context constantly
- **The Solution:** Emergent memory graph with Hebbian learning (concepts that co-occur link together automatically)
- **What to Store:** Org-specific code patterns, architectural decisions, domain procedures, project context
- **What NOT to Store:** Anything on Stack Overflow or in official docs

### UPTS (Parser Testing)

- 25 UPTS-validated parsers across all major language families
- Three components: **Parser** (Go code) + **Fixture** (source file) + **Spec** (JSON contract)
- Workflow: Create parser → Create fixture → Create spec → Iterate until green

### UATS (API Testing)

- 45+ specs covering every MDEMG endpoint
- JSONPath assertions for response validation
- Variants for error cases baked into each spec

---

## Commands to Demo

```bash
# UPTS — Run all parser tests
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v

# UPTS — Single language
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/kotlin -v

# UATS — Run all API tests
make test-api

# UATS — Single endpoint
make test-api-health

# CMS — Resume a session (show memory continuity)
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"your-project","session_id":"demo","max_observations":10}' | jq
```

---

## Repo Quick Links

### MDEMG Core

| Resource | Path |
|----------|------|
| Vision Document | `VISION.md` |
| Architecture | `docs/architecture/01_Architecture.md` |
| Conversation Memory | `docs/architecture/conversation_memory_phase1.md` |
| Learning Edges | `docs/architecture/LEARNING_EDGES.md` |

### UPTS Resources

| Resource | Path |
|----------|------|
| Specs (25 languages) | `docs/lang-parser/lang-parse-spec/upts/specs/` |
| Fixtures | `docs/lang-parser/lang-parse-spec/upts/fixtures/` |
| Documentation | `docs/lang-parser/lang-parse-spec/upts/README.md` |
| Parser implementations | `cmd/ingest-codebase/languages/` |

### UATS Resources

| Resource | Path |
|----------|------|
| Specs (45+ endpoints) | `docs/api/api-spec/uats/specs/` |
| Runner | `docs/api/api-spec/uats/runners/uats_runner.py` |
| Documentation | `docs/api/api-spec/uats/README.md` |

---

## Prerequisites

```bash
# Build parser binary
go build ./cmd/ingest-codebase/...

# Install UATS dependencies
pip install requests jsonpath-ng

# Start server
./bin/server &

# Verify
curl localhost:9999/healthz
```
