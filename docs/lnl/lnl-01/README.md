# LnL-01: MDEMG Overview & Test Specification Frameworks

**Date:** 2026-02-06
**Duration:** 1 hour
**Topic:** MDEMG Purpose & Architecture + UPTS (Parser Testing) & UATS (API Testing)

---

## Session Overview

This lunch and learn covers:

1. **Why MDEMG exists** — The core problem of LLM memory loss and how MDEMG solves it
2. **What MDEMG stores** — Domain-specific knowledge vs. general knowledge
3. **How MDEMG works** — Architecture, emergent layers, conversation memory
4. **UPTS** — How we validate 22 language parsers with spec-driven testing
5. **UATS** — How we validate 45+ API endpoints with contract testing

---

## Materials

| File | Description | Est. Time |
|------|-------------|-----------|
| [00-OVERVIEW.md](./00-OVERVIEW.md) | MDEMG deep dive: problem, solution, architecture, CMS | 15 min |
| [01-UPTS-DEEP-DIVE.md](./01-UPTS-DEEP-DIVE.md) | Parser testing framework, workflow, Zig walkthrough, Q&A | 18 min |
| [02-UATS-DEEP-DIVE.md](./02-UATS-DEEP-DIVE.md) | API testing framework, workflow, summarize endpoint walkthrough, Q&A | 17 min |
| [03-SPEAKER-NOTES.md](./03-SPEAKER-NOTES.md) | Minute-by-minute timing, talking points, setup checklist | - |

---

## Key Concepts Covered

### MDEMG Fundamentals
- **The Problem:** LLMs have no persistent memory — every session starts from zero
- **The Cost:** Constant re-explanation, lost institutional knowledge, repeated mistakes
- **The Solution:** Emergent memory graph with Hebbian learning
- **What to Store:** Domain-specific, organization-specific, task-specific knowledge
- **What NOT to Store:** Anything you can find on Stack Overflow

### UPTS (Parsers)
- Universal Parser Test Specification
- 20 UPTS-validated parsers (Go, Rust, Python, TypeScript, Java, C#, Kotlin, C++, C, CUDA, SQL, Cypher, Terraform, YAML, TOML, JSON, INI, Makefile, Dockerfile, Shell)
- Three components: Parser + Fixture + Spec
- Workflow: Create parser → Create fixture → Create spec → Iterate

### UATS (APIs)
- Universal API Test Specification
- 45 specs covering all MDEMG endpoints
- JSONPath assertions for response validation
- Variants for testing error cases

---

## Quick Links

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
| Specs | `docs/lang-parser/lang-parse-spec/upts/specs/` |
| Fixtures | `docs/lang-parser/lang-parse-spec/upts/fixtures/` |
| Documentation | `docs/lang-parser/lang-parse-spec/upts/README.md` |
| Parsers | `cmd/ingest-codebase/languages/` |

### UATS Resources
| Resource | Path |
|----------|------|
| Specs | `docs/api/api-spec/uats/specs/` |
| Runner | `docs/api/api-spec/uats/runners/uats_runner.py` |
| Documentation | `docs/api/api-spec/uats/README.md` |

---

## Commands to Demo

```bash
# UPTS - Run all parser tests
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v

# UPTS - Single language
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/kotlin -v

# UATS - Run all API tests
make test-api

# UATS - Single endpoint
make test-api-health

# CMS - Resume session
curl -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"your-project","session_id":"demo","max_observations":10}'
```

---

## Session Summary

| Topic | Key Takeaway |
|-------|--------------|
| MDEMG Purpose | Persistent memory for AI agents — domain knowledge that survives sessions |
| What to Store | Organizational code patterns, architectural decisions, domain procedures, project context |
| UPTS | Parser testing via JSON specs — one format for 20 languages |
| UATS | API testing via request/response contracts — self-documenting tests |
| Workflow | Create → Test → Iterate → Document |

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
