# Speaker Notes & Timing Guide

## Session Overview

**Total Time:** 60 minutes
**Format:** Presentation + Live Demo + Q&A
**Materials Needed:**
- Terminal with mdemg repo open
- Server running (`./bin/server &`)
- Browser with Neo4j if available

---

## Detailed Timing

### 0:00-0:15 — Welcome & MDEMG Deep Dive (15 min)

#### Opening (1 min)

> "Welcome to our first lunch and learn. Today we're covering MDEMG and the testing frameworks that ensure it works correctly: UPTS for parsers and UATS for APIs."

#### The Core Problem: LLMs Have No Memory (3 min)

**Show the Session 1 / Session 2 diagram**

> "When you work with AI coding assistants, every session starts from zero. Yesterday you explained your auth system in detail. Today? The AI has no idea."

**Real costs to highlight:**
1. Constant re-explanation — "I told you this last week"
2. Lost institutional knowledge — "Why did we build it this way? The person who knew left"
3. Repeated mistakes — "We tried that before, it broke production"
4. Multi-agent chaos — Two agents refactoring the same code

**Key point:**
> "This isn't a minor inconvenience. This fundamentally limits how useful AI can be for real work."

#### MDEMG: The Solution (4 min)

**Introduce the name:**
> "Multi-Dimensional Emergent Memory Graph — a cognitive substrate for AI-assisted development."

**The Internal Dialog Analogy:**
> "Humans have an inner voice of accumulated experience. MDEMG gives AI agents the same thing — a persistent memory of domain knowledge."

**Critical distinction — What MDEMG does NOT store:**
- Python syntax (LLM knows this)
- React hooks (universally documented)
- General best practices (in training data)

**Rule of thumb:**
> "If you can find it on Stack Overflow, it doesn't belong in MDEMG."

**What MDEMG DOES store:**
- Your organization's code patterns
- Architectural decisions and WHY they were made
- Domain-specific procedures (P&IDs, PLC logic)
- Project context (deprecated APIs, workarounds)
- Problem/solution history

#### How It Works (2 min)

**Show the architecture diagram**

**Three main components:**
1. **Retrieval Pipeline** — Vector search + graph hops + LLM reranking
2. **Learning System** — Hebbian edges ("neurons that fire together, wire together")
3. **Conversation Memory (CMS)** — Session resume, observation capture

**Show the layer diagram:**
> "MDEMG doesn't just store flat memories. It builds hierarchical understanding. Higher-level concepts EMERGE automatically."

**Quick CMS demo concept:**
> "On session start: resume previous context. During session: observe decisions and learnings. Result: continuous memory across sessions."

#### Extensibility: Sidecar Modules (1 min)

**Brief mention — don't deep dive:**
> "MDEMG is extensible via sidecar modules. Plugins run as separate processes communicating over gRPC."

**Three types to mention:**
- INGESTION — Pull from external sources (Linear, Jira, Obsidian)
- REASONING — Custom re-ranking in the retrieval pipeline
- APE — Background autonomous tasks (reflection, consistency checks)

**Key benefits:**
> "Language agnostic, fault isolated, hot reloadable. Full SDK docs in the repo."

#### The Testing Challenge (2 min)

**Transition:**
> "So we have 22 language parsers and 45+ API endpoints. How do we ensure this all works correctly?"

> "Before we had spec-driven testing, each parser had different test formats. Adding a new language meant inventing a new test structure."

> "The solution: UPTS for parsers, UATS for APIs. Let's dive into UPTS first."

---

### 0:15-0:33 — UPTS Deep Dive (18 min)

#### Section 1: What is UPTS? (4 min)

**Speaker Notes**
- Show the directory structure
- Walk through a real spec file (kotlin.upts.json)
- Highlight: fixture, expected symbols, config

**Key Talking Points:**
> "UPTS stands for Universal Parser Test Specification. The key word is 'Universal' - same format for Go, Python, Rust, Kotlin, C++, all 20 languages."

> "Three components: Parser (Go code), Fixture (source file), Spec (JSON). The spec is the contract."

#### Section 2: Live Demo - Running UPTS (4 min)

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

#### Section 3: Walkthrough - Adding a New Parser (8 min)

**Walk through the Zig example from the doc:**

1. **Parser file** (2 min)
   > "First you implement the LanguageParser interface. Key methods: Name, Extensions, ParseFile."
   - Show the regex patterns
   - Explain symbol extraction

2. **Fixture file** (2 min)
   > "The fixture is a representative source file. Include all the patterns you want to test."
   - Point out P1-P7 patterns (constants, functions, classes, interfaces, enums, methods, type aliases)

3. **Spec file** (3 min)
   > "The spec lists every symbol you expect the parser to find."
   - Show line numbers, types, exported flags
   - Explain line_tolerance

4. **Run and iterate** (1 min)
   > "Run the test. If it fails, either fix the parser or fix the spec."

#### Section 4: Q&A Transition (2 min)

> "Any questions on UPTS before we move to UATS?"

**Anticipated questions to prepare for:**
- "Why JSON and not YAML?" → Better tooling, no whitespace issues
- "What if parser finds more than expected?" → Fine if allow_extra_symbols: true
- "How do you handle multi-line symbols?" → Use line_end in spec

---

### 0:33-0:50 — UATS Deep Dive (17 min)

#### Section 1: What is UATS? (4 min)

**Speaker Notes**

> "UATS is for APIs what UPTS is for parsers. Universal API Test Specification."

- Show directory structure
- Walk through health.uats.json (simplest example)
- Then show retrieve.uats.json (more complex)

**Key Talking Points:**
> "Each spec defines: the request (method, path, body) and the expected response (status, headers, body assertions)."

> "We use JSONPath for assertions. Similar to XPath but for JSON."

#### Section 2: Live Demo - Running UATS (4 min)

```bash
# Show health check
make test-api-health

# Show all tests
make test-api 2>&1 | tail -30
```

**Point out:**
- HTTP status codes
- Response time
- Assertion pass/fail
- Summary at end

#### Section 3: Walkthrough - Adding a New Spec (7 min)

**Walk through the summarize example from the doc:**

1. **Basic structure** (1 min)
   > "Start with api, metadata, config sections."
   - These are boilerplate but important for documentation

2. **Request definition** (2 min)
   > "Method, path, headers, body. Use variables for test-specific values."
   - Show variable substitution with ${var_name}

3. **Expected response** (2 min)
   > "Status code plus body assertions."
   - Show different assertion types: equals, type, exists, gte

4. **Variants** (2 min)
   > "Each variant tests a different scenario. Error cases are critical."
   - Show the missing_space_id variant
   - Emphasize: "Always test the unhappy paths"

#### Section 4: UPTS vs UATS Comparison (2 min)

Show the comparison table:
- UPTS: Parsers, source files, symbol JSON
- UATS: APIs, HTTP requests, HTTP responses
- Same principle: **spec is the contract**

---

### 0:50-1:00 — Q&A (10 min)

**Opening Q&A:**
> "Now let's open it up for questions. Anything about UPTS, UATS, or MDEMG in general."

**If no questions, prompt with:**
1. "Has anyone worked with similar spec-driven testing frameworks?"
2. "Any thoughts on how we could use this approach in other projects?"
3. "Questions about how MDEMG would help with your specific workflows?"
4. "Interest in seeing the Conversation Memory System in action?"

**Prepared answers for common questions:**

**Q: How long does it take to add a new language parser?**
> "Once you understand the pattern, 2-4 hours for a basic parser. More for complex languages like C++ with templates. We added 4 new parsers (C#, Kotlin, Terraform, Makefile) in one session."

**Q: What about languages with unusual syntax?**
> "The regex-based approach works for most cases. For really complex syntax, you can use tree-sitter or other parsing libraries, then normalize the output."

**Q: Can we use UATS for our own APIs?**
> "Absolutely. The runner is standalone Python. Copy the schema and runner, adapt to your endpoints."

**Q: What's the maintenance burden?**
> "Specs need updating when APIs change. But that's the point - the spec is the contract. If you change the API, you should update the spec. It's documentation that verifies itself."

**Q: How does MDEMG compare to RAG?**
> "RAG retrieves static chunks. MDEMG builds an evolving graph where concepts emerge and relationships strengthen over time. It learns from use, not just from ingestion."

**Q: What's the performance overhead of MDEMG?**
> "Retrieval adds ~100-300ms depending on query complexity. For most use cases, that's invisible compared to LLM response time."

**Q: Can we use MDEMG with Claude Code / Cursor / other tools?**
> "Yes, MDEMG has a standard HTTP API. Any tool that can make HTTP requests can integrate. We have documentation for Claude Code integration specifically."

---

## Backup Material

### If Running Short on Time

Skip:
- Detailed walkthrough of Zig parser (reference the doc)
- Multiple UATS variants (show just one)
- Comparison table (they can read it)
- Deep dive on layer architecture

### If Running Long on Time

Add:
- Show Neo4j browser with actual graph data
- Demonstrate a real codebase ingestion
- Walk through a failing test and how to debug
- Live demo of CMS observe/resume cycle
- Show the actual emergence of a hidden layer node

---

## Technical Setup Checklist

Before the session:

```bash
# 1. Build everything
cd ~/mdemg
go build ./cmd/ingest-codebase/...
go build ./cmd/server/...

# 2. Start server
./bin/server &

# 3. Verify health
curl localhost:9999/healthz

# 4. Run UPTS tests (ensure they pass)
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v 2>&1 | tail -5

# 5. Run UATS tests (ensure they pass)
make test-api 2>&1 | tail -10

# 6. Have these files open in editor:
#    - docs/lang-parser/lang-parse-spec/upts/specs/kotlin.upts.json
#    - docs/api/api-spec/uats/specs/health.uats.json
#    - cmd/ingest-codebase/languages/kotlin_parser.go
#    - VISION.md (for reference)

# 7. Have terminal ready to demo:
#    - UPTS test run
#    - UATS test run
#    - Optional: CMS observe/resume
```

---

## Key Takeaways to Emphasize

1. **MDEMG solves the "no memory" problem** — AI agents maintain context across sessions
2. **Spec-driven testing = self-documenting contracts** — Tests are documentation
3. **Same pattern for parsers and APIs** — Learn once, apply everywhere
4. **Clear workflow: Create → Test → Iterate → Document** — Anyone can add new languages/endpoints
5. **100% pass rate as CI gate** — Regressions caught before merge

---

## Follow-Up Resources

Share after the session:

- `docs/lnl/lnl-01/` - All presentation materials
- `VISION.md` - MDEMG philosophy and architecture
- `docs/lang-parser/lang-parse-spec/upts/README.md` - Full UPTS documentation
- `docs/api/api-spec/uats/README.md` - Full UATS documentation
- `cmd/ingest-codebase/languages/README.md` - Parser implementation guide
- `docs/architecture/` - Deep dive architecture docs

---

## Post-Session Action

Observe this session to CMS:

```bash
curl -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "session_id": "claude-core",
    "content": "Conducted LnL-01: MDEMG overview (purpose, problems solved, architecture) + UPTS & UATS test specification frameworks. Covered parser validation, API testing, workflows for adding new specs. Attendees: [list]. Questions: [summarize]. Materials in docs/lnl/lnl-01/.",
    "obs_type": "progress",
    "tags": ["lnl", "documentation", "training", "mdemg-overview"]
  }'
```
