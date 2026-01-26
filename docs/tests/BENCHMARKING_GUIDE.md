# MDEMG Benchmarking Guide

**Version:** 1.4
**Last Updated:** 2026-01-26
**Purpose:** Step-by-step guide for setting up, running, and analyzing MDEMG vs Baseline retrieval tests on new codebases

---

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Phase 1: Codebase Preparation](#phase-1-codebase-preparation)
4. [Phase 2: Test Question Development](#phase-2-test-question-development)
5. [Phase 3: MDEMG Setup and Ingestion](#phase-3-mdemg-setup-and-ingestion)
6. [Phase 4: Running Tests](#phase-4-running-tests)
7. [Phase 5: Analysis and Reporting](#phase-5-analysis-and-reporting)
8. [Roles and Responsibilities](#roles-and-responsibilities)
9. [Common Mistakes to Avoid](#common-mistakes-to-avoid)
10. [V6 Composite Test Set](#v6-composite-test-set)
11. [WHK-WMS 120-Question Test Set](#whk-wms-120-question-test-set)
12. [Baseline Benchmarking Rules](#baseline-benchmarking-rules)
13. [End-to-End Multi-Corpus Runbook](#end-to-end-multi-corpus-runbook)
14. [Appendix: Templates and Scripts](#appendix-templates-and-scripts)

---

## Overview

### What We're Testing

MDEMG (Multi-Dimensional Emergent Memory Graph) provides AI agents with persistent, retrievable memory for codebases. The benchmark compares:

| Approach | Description | Expected Behavior |
|----------|-------------|-------------------|
| **Baseline** | Traditional context window approach - read files, rely on context compression | Context compression loses critical details; accuracy degrades with codebase size |
| **MDEMG** | Retrieve relevant context via semantic search + graph activation | Maintains access to all indexed content; accuracy scales with codebase size |

### Success Metrics

| Metric | Description | Target |
|--------|-------------|--------|
| **Avg Score** | Mean score across all questions (0.0-1.0) | MDEMG > 0.65, Baseline < 0.15 |
| **Delta** | MDEMG score - Baseline score | > +0.50 |
| **>0.7 Rate** | Percentage of questions with score > 0.7 | MDEMG > 50% |
| **Learning Edge Growth** | CO_ACTIVATED_WITH edges created during test | > 20 pairs/query |

### Historical Results

| Version | Avg Score | Improvement | Key Feature |
|---------|-----------|-------------|-------------|
| Baseline | 0.050 | - | Context window only |
| v8 | 0.562 | +0.512 | Path/comparison boost |
| v9 | 0.619 | +10.2% vs v8 | LLM re-ranking |
| v10 | **0.710** | +14.6% vs v9 | Learning edge fix |

---

## Prerequisites

### System Requirements

```bash
# Hardware
- 16GB+ RAM (32GB recommended)
- 50GB+ free disk space
- macOS, Linux, or WSL2

# Software
- Docker & Docker Compose
- Go 1.21+
- Python 3.10+
- Node.js 18+ (if ingesting JS/TS codebases)
- Claude CLI (for running agent tests)
```

### Required Services

```bash
# 1. Neo4j (graph database)
docker compose up -d neo4j
# Verify: http://localhost:7474 (neo4j/testpassword)

# 2. MDEMG Service
cd mdemg_build/service
export NEO4J_URI=bolt://localhost:7687
export NEO4J_USER=neo4j
export NEO4J_PASS=testpassword
export REQUIRED_SCHEMA_VERSION=4
export VECTOR_INDEX_NAME=memNodeEmbedding
export OPENAI_API_KEY=sk-...  # For embeddings
go run ./cmd/server

# Verify: curl localhost:8090/healthz
```

---

## Phase 1: Codebase Preparation

### Step 1.1: Clone/Locate the Repository

```bash
# Option A: Clone from URL
git clone https://github.com/org/repo.git /path/to/repo
cd /path/to/repo

# Option B: Use existing local path
cd /path/to/existing/repo
```

### Step 1.2: Generate File List

```bash
# Create file list (exclude binaries, node_modules, etc.)
find /path/to/repo -type f \
  -name "*.ts" -o -name "*.tsx" -o -name "*.js" -o -name "*.jsx" \
  -o -name "*.py" -o -name "*.go" -o -name "*.java" \
  -o -name "*.rs" -o -name "*.c" -o -name "*.cpp" \
  -o -name "*.json" -o -name "*.yaml" -o -name "*.yml" \
  -o -name "*.md" -o -name "*.sql" \
  | grep -v node_modules \
  | grep -v dist \
  | grep -v build \
  | grep -v .git \
  | sort > /path/to/tests/file-list.txt

# Count files
wc -l /path/to/tests/file-list.txt
```

### Step 1.3: Analyze Codebase Structure

Before creating questions, understand the codebase:

```bash
# Get directory structure
tree -d -L 3 /path/to/repo > /path/to/tests/structure.txt

# Count lines by file type
find /path/to/repo -name "*.ts" -exec wc -l {} + | tail -1
find /path/to/repo -name "*.py" -exec wc -l {} + | tail -1

# Identify key modules
ls /path/to/repo/src/
ls /path/to/repo/apps/*/src/
```

---

## Phase 2: Test Question Development

### Step 2.1: Question Categories

Create questions across 5 categories (20 questions each for 100 total):

| Category | Description | Example Topics |
|----------|-------------|----------------|
| `architecture_structure` | Module organization, dependency patterns | "Why is Module X marked @Global?" |
| `service_relationships` | Inter-service dependencies, injection patterns | "What services does X inject?" |
| `business_logic_constraints` | Domain rules, validation logic | "What prevents overlapping ownership?" |
| `data_flow_integration` | Request/response flows, data transformations | "Trace the flow when X happens" |
| `cross_cutting_concerns` | Auth, logging, error handling, caching | "How does audit logging work?" |

### Step 2.2: Question Quality Requirements

**CRITICAL:** Questions must be:

1. **Multi-file** - Require understanding 2+ files to answer correctly
2. **Verifiable** - Have concrete, code-referenced answers
3. **Non-trivial** - Cannot be answered from a single function or enum
4. **Specific** - Avoid vague questions like "How does X work?"

### Step 2.3: Question JSON Format

```json
{
  "metadata": {
    "source": "path/to/repo",
    "total_questions": 100,
    "generated_at": "2026-01-23T12:00:00Z"
  },
  "questions": [
    {
      "id": 1,
      "category": "service_relationships",
      "question": "What services does BarrelEventService depend on for handling entry events at holding locations?",
      "answer": "BarrelEventService constructor injects: 1) PrismaService - database operations; 2) AndroidSyncErrorLoggerService (errorLogger) - logs warnings/errors; 3) LotVariationService - normalizes serial numbers; 4) FeatureFlagsService - controls feature behavior; 5) CachedEventTypeService - avoids in-transaction queries.",
      "required_files": [
        "/path/to/repo/src/barrelEvent/barrelEvent.service.ts",
        "/path/to/repo/src/androidSyncInbox/android-sync-error-logger.service.ts",
        "/path/to/repo/src/lotVariation/lotVariation.service.ts"
      ],
      "complexity": "multi-file"
    }
  ]
}
```

### Step 2.4: Question Generation Process

1. **Explore the codebase** to understand architecture
2. **Identify cross-cutting patterns** (DI, error handling, etc.)
3. **Write questions that require synthesis** from multiple files
4. **Verify answers against actual code** before finalizing
5. **Include file:line references** in expected answers

### Step 2.5: Verification Process

**CRITICAL:** Verify ALL answers before using in tests.

```bash
# Batch verification approach
# Split questions into batches of 4
# Have Claude verify each batch against actual code
# Track corrections in corrections.json
```

Common verification findings:
- ~30-35% of LLM-generated answers contain errors
- Most errors: wrong method names, overstated functionality, incorrect enums

---

## Phase 3: MDEMG Setup and Ingestion

### Step 3.1: Create Space

```bash
# Create a new space for the codebase
curl -s -X POST 'http://localhost:8090/v1/spaces' \
  -H 'content-type: application/json' \
  -d '{"space_id": "your-project-name", "description": "Description of project"}'
```

### Step 3.2: Ingest Codebase

**Option A: Using ingest-codebase tool**

```bash
cd mdemg_build/service
go run ./cmd/ingest-codebase \
  --space-id=your-project-name \
  --path=/path/to/repo \
  --file-list=/path/to/tests/file-list.txt
```

**Option B: Using batch API**

```bash
# For each batch of files
curl -s -X POST 'http://localhost:8090/v1/memory/batch-ingest' \
  -H 'content-type: application/json' \
  -d '{
    "space_id": "your-project-name",
    "items": [
      {"path": "<file_path>", "content": "<file_content>", "content_type": "code"}
    ]
  }'
```

### Step 3.3: Verify Ingestion

```bash
# Check node count
curl -s 'http://localhost:8090/v1/memory/stats?space_id=your-project-name' | jq

# Expected output:
# {
#   "node_count": 8906,
#   "edge_count": 94654,
#   "learning_activity": {
#     "co_activated_edges": 0
#   }
# }
```

### Step 3.4: Run Consolidation (Optional)

```bash
# Build hidden layer concepts
curl -s -X POST 'http://localhost:8090/v1/memory/consolidate' \
  -H 'content-type: application/json' \
  -d '{"space_id":"your-project-name"}' | jq
```

---

## Phase 4: Running Tests

### Step 4.1: Cold Start (Reset Learning Edges) ⚠️ CRITICAL

For reproducible results, reset CO_ACTIVATED_WITH edges before each test:

```bash
# Connect to Neo4j
docker exec -it mdemg-neo4j cypher-shell -u neo4j -p testpassword

# Delete learning edges for the space
MATCH ()-[r:CO_ACTIVATED_WITH {space_id: 'your-project-name'}]-()
DELETE r
RETURN count(r) as deleted;
```

**⚠️ CRITICAL FINDING (2026-01-26):** Edge accumulation causes score degradation.

| Starting Condition | Avg Score |
|--------------------|-----------|
| Fresh (0 edges) | **0.740** (+4.3%) |
| Accumulated (13,000+ edges) | **0.679** (-4.3%) |

Learning edges created during one benchmark run do NOT transfer benefits to subsequent runs - they cause spreading activation dilution. Each benchmark run creates ~8,000+ new edges, and the accumulated edges dilute the signal for future queries.

**Best Practice:** ALWAYS clear learning edges before benchmark runs for consistent, reproducible results. Use the cold start procedure above.

### Step 4.2: MDEMG Test Script

Create `run_mdemg_test.py`:

```python
#!/usr/bin/env python3
"""MDEMG Retrieval Test"""

import json
import urllib.request
import time
from pathlib import Path
from datetime import datetime
from collections import defaultdict

TEST_DIR = Path(__file__).parent
QUESTIONS_FILE = TEST_DIR / "test_questions.json"
OUTPUT_FILE = TEST_DIR / f"mdemg-test-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
MDEMG_ENDPOINT = "http://localhost:8090"
SPACE_ID = "your-project-name"

def load_questions():
    with open(QUESTIONS_FILE) as f:
        return json.load(f)['questions']

def query_mdemg(question: str) -> dict:
    try:
        data = json.dumps({
            "space_id": SPACE_ID,
            "query_text": question,
            "candidate_k": 50,
            "top_k": 10,
            "hop_depth": 2
        }).encode('utf-8')
        req = urllib.request.Request(
            f"{MDEMG_ENDPOINT}/v1/memory/retrieve",
            data=data,
            headers={'Content-Type': 'application/json'}
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except Exception as e:
        return {"error": str(e)}

def get_edge_count() -> int:
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/v1/memory/stats?space_id={SPACE_ID}")
        with urllib.request.urlopen(req, timeout=5) as resp:
            data = json.loads(resp.read().decode('utf-8'))
            return data.get('learning_activity', {}).get('co_activated_edges', 0)
    except:
        return 0

def run_test():
    print("=" * 60)
    print("MDEMG RETRIEVAL TEST")
    print("=" * 60)

    # Health check
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/healthz")
        with urllib.request.urlopen(req, timeout=5) as resp:
            print(f"MDEMG Status: {resp.read().decode('utf-8')}")
    except Exception as e:
        print(f"ERROR: MDEMG not reachable: {e}")
        return

    questions = load_questions()
    print(f"Loaded {len(questions)} questions")

    initial_edges = get_edge_count()
    print(f"Initial CO_ACTIVATED_WITH edges: {initial_edges}")
    print("-" * 60)

    results = []
    start_time = time.time()

    for i, q in enumerate(questions, 1):
        qtext = q['question']
        category = q['category']
        resp = query_mdemg(qtext)

        if "error" in resp:
            print(f"Q{i}: ERROR - {resp['error']}")
            results.append({"id": q['id'], "category": category, "score": 0})
            continue

        nodes = resp.get('results', [])
        top_score = nodes[0].get('score', 0) if nodes else 0

        results.append({
            "id": q['id'],
            "category": category,
            "score": top_score
        })

        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            current_edges = get_edge_count()
            new_edges = current_edges - initial_edges
            print(f"Progress: {i}/{len(questions)} ({rate:.2f} q/s) - Score: {top_score:.3f}, Edges: +{new_edges}")

    total_time = time.time() - start_time
    final_edges = get_edge_count()

    # Analysis
    scores = [r['score'] for r in results]
    by_category = defaultdict(list)
    for r in results:
        by_category[r['category']].append(r['score'])

    avg_score = sum(scores) / len(scores)

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {avg_score:.3f}")
    print(f"Duration: {total_time:.1f}s")
    print(f"Learning Edges: {initial_edges} -> {final_edges} (+{final_edges - initial_edges})")
    print(f"\nBy Category:")
    for cat, vals in sorted(by_category.items()):
        print(f"  {cat}: {sum(vals)/len(vals):.3f}")

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(f"# MDEMG Test Results\n\n")
        f.write(f"**Date**: {datetime.now().isoformat()}\n")
        f.write(f"**Duration**: {total_time:.1f}s\n\n")
        f.write(f"## Summary\n\n")
        f.write(f"| Metric | Value |\n")
        f.write(f"|--------|-------|\n")
        f.write(f"| Avg Score | {avg_score:.3f} |\n")
        f.write(f"| Max Score | {max(scores):.3f} |\n")
        f.write(f"| Min Score | {min(scores):.3f} |\n")
        f.write(f"| Learning Edges Created | {final_edges - initial_edges} |\n")

    print(f"\nReport: {OUTPUT_FILE}")
    return {"avg": avg_score, "by_category": {c: sum(v)/len(v) for c,v in by_category.items()}}

if __name__ == "__main__":
    run_test()
```

### Step 4.3: Baseline Test Agent Prompt

Create `baseline_prompt.md`:

```markdown
# Baseline Test - Context Retention Experiment

## CRITICAL CONSTRAINTS
- You MUST read ALL files listed in file-list.txt BEFORE seeing questions
- After ingestion, you MAY NOT re-read files during questions
- Context WILL compress - this is expected and is the point of the test

## PHASE 1: CODEBASE INGESTION (MANDATORY)

1. Record START_TIME now
2. Read the file list: /path/to/tests/file-list.txt
3. For EACH file, read its contents
4. Process files in batches of 20-50 for efficiency
5. Report progress every 500 files
6. After ALL files, record INGESTION_COMPLETE_TIME

## PHASE 2: ANSWER QUESTIONS

### Rules:
- Answer from compressed memory only
- Note source: [MEMORY], [LOOKUP], or [BOTH]

### Scoring:
- 1.0 = Completely correct
- 0.5 = Partially correct
- 0.0 = Unable to answer
- -1.0 = Confidently wrong

### Output Format:
```
Q[number]: [question]
Source: [MEMORY/LOOKUP/BOTH]
Answer: [your answer]
Score: [self-assessed score]
```

## QUESTIONS
[Insert 100 questions here]

## FINAL REPORT
```
=== BASELINE TEST RESULTS ===
START_TIME: [timestamp]
INGESTION_COMPLETE_TIME: [timestamp]
END_TIME: [timestamp]
FILES_EXPECTED: [count]
FILES_VERIFIED: [output of wc -l]
TOTAL_SCORE: [X/100]
===
```
```

### Step 4.4: MDEMG Test Agent Prompt

Create `mdemg_prompt.md`:

```markdown
# MDEMG Test - Memory-Augmented Retrieval Experiment

## PHASE 1: CODEBASE INGESTION INTO MDEMG (MANDATORY)

1. Record START_TIME
2. Read file list and ingest EACH file via batch API:
```bash
curl -s -X POST 'http://localhost:8090/v1/memory/batch-ingest' \
  -H 'content-type: application/json' \
  -d '{"space_id":"<space>","items":[{"path":"<path>","content":"<content>"}]}'
```
3. Report progress every 500 files
4. Verify node count matches file count
5. Run consolidation
6. Record INGESTION_COMPLETE_TIME

## PHASE 2: ANSWER QUESTIONS

### MDEMG Retrieval:
For each question, FIRST query MDEMG:
```bash
curl -s 'http://localhost:8090/v1/memory/retrieve' \
  -H 'content-type: application/json' \
  -d '{"space_id":"<space>","query_text":"<question>","candidate_k":50,"top_k":10,"hop_depth":2}'
```

### Rules:
- Query MDEMG first for each question
- May verify via direct file lookup
- Note source: [MDEMG], [LOOKUP], or [BOTH]

### Scoring:
- 1.0 = Completely correct
- 0.5 = Partially correct
- 0.0 = Unable to answer
- -1.0 = Confidently wrong

### Output Format:
```
Q[number]: [question]
MDEMG Nodes Retrieved: [count]
Top Retrieved Paths: [list top 3 paths]
Source: [MDEMG/LOOKUP/BOTH]
Answer: [your answer]
Score: [self-assessed score]
```

## QUESTIONS
[Insert 100 questions here]

## FINAL REPORT
```
=== MDEMG TEST RESULTS ===
START_TIME: [timestamp]
INGESTION_COMPLETE_TIME: [timestamp]
END_TIME: [timestamp]
FILES_INGESTED: [count]
MDEMG_NODES: [count]
MDEMG_EDGES: [count]
TOTAL_SCORE: [X/100]
MDEMG_RETRIEVAL_STATS:
  - Total retrievals: [count]
  - Avg nodes per query: [number]
===
```
```

### Step 4.5: MDEMG Skill Configuration

**CRITICAL:** When spawning sub-agents for MDEMG benchmark tasks, ensure the agent has access to the `mdemg-consult` skill. This skill provides a direct interface to the Agent Consulting Service API.

#### Skill Location

The skill is defined in `.claude/commands/mdemg.md` and `.claude/commands/mdemg-consult.md`.

#### Required Skill Capabilities

| Skill | Purpose |
|-------|---------|
| `/mdemg consult <question>` | Get SME advice from the knowledge graph |
| `/mdemg retrieve <query>` | Search for relevant memories |
| `/mdemg stats` | Show space statistics |

#### Task Agent Configuration

When launching task agents, ensure they can access the MDEMG API:

```bash
# Verify MDEMG is accessible before spawning agents
curl -s localhost:8090/healthz

# The agent should be able to use the skill like:
# /mdemg consult "How should I handle authentication in this service?"
# /mdemg retrieve "database connection pooling"
```

#### Agent Prompt Addition

Add this to MDEMG test agent prompts:

```markdown
## MDEMG SKILL ACCESS

You have access to the `/mdemg` skill for querying the knowledge graph:
- `/mdemg consult <question>` - Get SME suggestions with evidence
- `/mdemg retrieve <query>` - Search for relevant memories

Use these skills BEFORE attempting manual file searches to leverage
the accumulated codebase knowledge.
```

### Step 4.6: Running Tests in Parallel

```bash
# Terminal 1: Run MDEMG test
claude --prompt-file mdemg_prompt.md --output mdemg_output.txt

# Terminal 2: Run Baseline test
claude --prompt-file baseline_prompt.md --output baseline_output.txt
```

---

## Phase 5: Analysis and Reporting

### Step 5.1: Comparison Report Template

```markdown
# MDEMG vs Baseline Comparison Report

**Date:** YYYY-MM-DD
**Codebase:** [project name]
**Questions:** 100

## Executive Summary

| Metric | Baseline | MDEMG | Delta |
|--------|----------|-------|-------|
| **Total Score** | X/100 | Y/100 | +Z |
| **Average Score** | 0.XXX | 0.YYY | +0.ZZZ |
| Completely Correct (1.0) | A | B | +C |
| Partially Correct (0.5) | D | E | +F |
| Unable to Answer (0.0) | G | H | -I |

**Result: MDEMG outperformed Baseline by Z points (Xx better).**

## Score by Category

| Category | Baseline | MDEMG | Delta |
|----------|----------|-------|-------|
| architecture_structure | X/20 | Y/20 | +Z |
| service_relationships | X/20 | Y/20 | +Z |
| business_logic_constraints | X/20 | Y/20 | +Z |
| data_flow_integration | X/20 | Y/20 | +Z |
| cross_cutting_concerns | X/20 | Y/20 | +Z |

## Key Findings

1. **Context Window Limitations (Baseline)**
   - [Observations]

2. **MDEMG Retrieval Strengths**
   - [Observations]

3. **Areas for Improvement**
   - [Observations]

## Conclusion

[Summary of findings]
```

### Step 5.2: Generate Comparison Script

```python
#!/usr/bin/env python3
"""Generate comparison report from test outputs"""

def parse_results(filename):
    # Parse test output file
    # Return dict of {question_id: score}
    pass

def generate_comparison(baseline_file, mdemg_file):
    baseline = parse_results(baseline_file)
    mdemg = parse_results(mdemg_file)

    # Calculate metrics
    baseline_avg = sum(baseline.values()) / len(baseline)
    mdemg_avg = sum(mdemg.values()) / len(mdemg)

    print(f"Baseline Avg: {baseline_avg:.3f}")
    print(f"MDEMG Avg: {mdemg_avg:.3f}")
    print(f"Delta: {mdemg_avg - baseline_avg:+.3f}")
```

---

## Roles and Responsibilities

### Main AI (Orchestrator/Administrator)

The Main AI coordinates the testing process:

| Responsibility | Actions |
|---------------|---------|
| **Setup** | Prepare file lists, verify MDEMG running, create test prompts |
| **Coordination** | Launch baseline and MDEMG agents in parallel |
| **Monitoring** | Track progress, handle errors, verify completion |
| **Analysis** | Compare results, generate reports, identify patterns |
| **Quality Control** | Verify answer accuracy against expected answers |

### Baseline Agent

| Constraint | Rationale |
|------------|-----------|
| Must read ALL files first | Simulates "full context" approach |
| Cannot re-read files during questions | Tests context retention |
| Self-scores answers | Enables comparison with expected |
| Reports source of answers | Distinguishes memory vs lookup |

### MDEMG Agent

| Constraint | Rationale |
|------------|-----------|
| Must ingest all files to MDEMG | Builds the memory graph |
| Must query MDEMG first | Tests retrieval quality |
| May verify via file lookup | Allows hybrid approach |
| Self-scores answers | Enables comparison with expected |
| Reports MDEMG retrieval stats | Measures retrieval effectiveness |

---

## Common Mistakes to Avoid

### Question Development Mistakes

| Mistake | Why It's Bad | How to Avoid |
|---------|--------------|--------------|
| **Single-file questions** | Doesn't test cross-module understanding | Require 2+ files in `required_files` |
| **Speculative answers** | "likely", "probably" indicates unverified | Verify all answers against code |
| **Wrong method/field names** | ~35% of generated answers have these | Code-verify before finalizing |
| **Overstated functionality** | Claiming features that don't exist | Check actual implementation |
| **Enum value errors** | Wrong status codes, resolution types | Verify against actual enums |
| **Generic patterns assumed** | Assuming Redis when in-memory is used | Check actual implementation |

### Test Execution Mistakes

| Mistake | Why It's Bad | How to Avoid |
|---------|--------------|--------------|
| **Not verifying ingestion** | Test on incomplete data | Check node count matches file count |
| **Warm start without disclosure** | Learning edges affect results | Document edge count before/after |
| **Inconsistent scoring** | Incomparable results | Use strict scoring rubric |
| **Skipping Phase 1** | Baseline gets unfair advantage | Enforce all-files-first rule |

### Analysis Mistakes

| Mistake | Why It's Bad | How to Avoid |
|---------|--------------|--------------|
| **Comparing against wrong answers** | ~33% of questions may have errors | Verify question set first |
| **Ignoring category breakdown** | Misses strength/weakness patterns | Always report by category |
| **Single run conclusions** | Statistical noise | Run cold start tests multiple times |

---

## V6 Composite Test Set

### Overview

The V6 Composite Test Set is the standardized benchmark for VS Code codebase testing. It combines questions from multiple prior test versions into a single, balanced evaluation suite.

| Property | Value |
|----------|-------|
| **Total Questions** | 100 |
| **Categories** | 10 (10 questions each) |
| **Question Types** | simple_constant (61), multi_hop (18), hard (21) |
| **Codebase** | VS Code (`vscode-scale` space) |
| **Nodes in Graph** | ~29,000+ |

### Categories

| Category | Description | Example Question Type |
|----------|-------------|----------------------|
| `editor_config` | Editor settings, fonts, cursors | MAX_CURSOR_COUNT, tabSize defaults |
| `storage` | Storage backends, flush intervals | Flush timeouts, storage paths |
| `workbench_layout` | Panel sizes, grid dimensions | Sidebar widths, panel heights |
| `extensions` | Extension host, marketplace | Max extensions, timeout values |
| `terminal` | Terminal emulator settings | Scrollback limits, shell paths |
| `search` | Search features, limits | Max results, regex patterns |
| `files` | File handling, watchers | Max file size, watcher limits |
| `debug` | Debugger configuration | Timeout values, adapter settings |
| `notifications` | Notification system | Toast limits, duration values |
| `lifecycle` | App lifecycle, shutdown | Shutdown timeouts, startup flags |

### Question Types

1. **simple_constant** (61 questions)
   - Single-file evidence-locked questions
   - Require finding exact constant values
   - Example: "What is the DEFAULT_FLUSH_INTERVAL in storage.ts?"

2. **multi_hop** (18 questions)
   - Require tracing through 2-3 files
   - Test graph traversal capabilities
   - Example: "Trace how terminal scrollback limit is applied from config to renderer"

3. **hard** (21 questions)
   - Complex cross-file correlations
   - May require understanding of multiple interacting systems
   - Example: "What determines the maximum concurrent file watchers?"

### File Locations

```
docs/tests/vscode-scale/
├── test_questions_V6_composite.json    # Master set WITH expected answers
├── test_questions_V6_blind.json        # Blind set (questions only, for agents)
├── run_combined_benchmark.py           # Benchmark runner script
└── combined_benchmark_results.json     # Output from benchmark runs
```

### Master vs Blind Versions

| Version | File | Purpose | Contains Answers |
|---------|------|---------|------------------|
| **Master** | `test_questions_V6_composite.json` | Scoring, verification | YES |
| **Blind** | `test_questions_V6_blind.json` | Agent benchmark tasks | NO |

**Important:** Always use the **blind version** when testing agents to prevent answer leakage. The master version is for scoring and result verification only.

### Master Question Format

```json
{
  "id": "v6_editor_config_01",
  "category": "editor_config",
  "type": "simple_constant",
  "source": "v5_comprehensive",
  "question": "What is the MAX_CURSOR_COUNT constant in the VS Code editor?",
  "required_evidence": {
    "file": "cursor.ts",
    "value": "10000"
  }
}
```

### Blind Question Format

```json
{
  "id": "v6_editor_config_01",
  "category": "editor_config",
  "type": "simple_constant",
  "question": "What is the MAX_CURSOR_COUNT constant in the VS Code editor?"
}
```

### Running the Benchmark

```bash
# Activate test environment
source /tmp/mdemg-test-venv/bin/activate

# Run the benchmark (requires MDEMG running on localhost:8090)
python docs/tests/vscode-scale/run_combined_benchmark.py

# Results saved to combined_benchmark_results.json
```

### Scoring Criteria

| Score | Criteria |
|-------|----------|
| **1.0** | Correct file found AND correct value/evidence found |
| **0.5** | Correct file found but value not confirmed |
| **0.0** | Neither file nor evidence found |

### Baseline Results (Pre-Symbol Extraction)

| Metric | Value |
|--------|-------|
| Correct file in top-5 | 11.7% (7/60) |
| Evidence-locked accuracy | ~5% |
| Key Issue | Returns related files, not defining files |

This establishes the baseline that symbol extraction aims to improve.

---

## WHK-WMS 120-Question Test Set

### Overview

The WHK-WMS 120-question test set is the standardized benchmark for the warehouse management system codebase. It combines complex multi-file questions with symbol-lookup questions that test exact constant/value retrieval.

| Property | Value |
|----------|-------|
| **Total Questions** | 120 |
| **Categories** | 5 (architecture_structure, service_relationships, business_logic_constraints, data_flow_integration, cross_cutting_concerns) |
| **Question Types** | multi-file (59), cross-module (38), system-wide (3), symbol-lookup (20) |
| **Codebase** | WHK-WMS (`whk-wms` space) |

### Question Types

1. **multi-file** (59 questions)
   - Require understanding 2+ files to answer correctly
   - Test cross-file knowledge synthesis

2. **cross-module** (38 questions)
   - Require tracing through multiple modules
   - Test understanding of module interactions

3. **system-wide** (3 questions)
   - Complex architectural questions
   - Require broad system understanding

4. **symbol-lookup** (20 questions)
   - Test exact constant/value retrieval
   - Require finding specific symbol definitions (e.g., `MAX_EXPORT_SIZE`, `ID_CHUNK_SIZE`)
   - Symbol name included in agent version as search hint

### File Locations

```
docs/tests/whk-wms/
├── test_questions_120.json           # Master set WITH answers (for grading)
├── test_questions_120_agent.json     # Agent set WITHOUT answers (for task agents)
├── symbol-focused-questions-20.json  # Symbol questions source file
└── test_questions_100.json           # Original 100 questions (no symbols)
```

### Master vs Agent Versions

| Version | File | Purpose | Contains |
|---------|------|---------|----------|
| **Master** | `test_questions_120.json` | Scoring, grading | answers, required_files |
| **Agent** | `test_questions_120_agent.json` | Task agent input | questions only, NO answers |

**CRITICAL:** Always use the **agent version** (`_agent.json`) when feeding questions to task agents. The master version is for scoring and result verification only. Feeding answers to agents constitutes data contamination and invalidates test results.

### Master Question Format

```json
{
  "id": "sym_arch_3",
  "category": "architecture_structure",
  "question": "What is the ID_CHUNK_SIZE used by BarrelAggregatesService for chunked processing?",
  "answer": "5000",
  "symbol_name": "ID_CHUNK_SIZE",
  "required_files": ["/apps/whk-wms/src/barrelAggregates/barrel-aggregates.service.ts"],
  "complexity": "symbol-lookup"
}
```

### Agent Question Format (Answer Stripped)

```json
{
  "id": "sym_arch_3",
  "category": "architecture_structure",
  "question": "What is the ID_CHUNK_SIZE used by BarrelAggregatesService for chunked processing?",
  "complexity": "symbol-lookup",
  "symbol_name": "ID_CHUNK_SIZE"
}
```

Note: `symbol_name` is retained in the agent version because it appears in the question text and helps the agent know what to search for.

---

## Baseline Benchmarking Rules

### Time Limit

| Constraint | Value |
|------------|-------|
| **Total time to complete test** | 20 minutes |

### Model Selection

| Parameter | Value |
|-----------|-------|
| **Model** | Claude Haiku (haiku) |
| **Rationale** | Cost-effective for benchmark testing; Opus reserved for production |

When spawning task agents, always specify `model: "haiku"` to use Claude Haiku.

### Task Agent Instructions

When spawning task agents for baseline benchmarking, use this **EXACT prompt** (do not deviate):

```
BASELINE BENCHMARK RULES - READ CAREFULLY

1. USE TOOLS (Read, Glob, Grep) but DO NOT search the web
2. You may read the QUESTION FILE below, then ONLY interact with the whk-wms repo at /Users/reh3376/whk-wms
3. You are NOT ALLOWED to access any other repo or directory after reading the question file
4. These questions are complex and require EXPLANATION for full credit
5. Your primary goal is to be AS CORRECT AS POSSIBLE when answering questions
6. TIME LIMIT: 20 minutes total

WARNING: If you violate these rules you will be DISQUALIFIED and your task execution will end immediately.

QUESTION FILE (read this first, then work only in whk-wms): /Users/reh3376/mdemg/docs/tests/whk-wms/test_questions_120_agent.json

This file contains 120 questions across multiple categories and complexity levels. Answer them IN ORDER as they appear in the file.

SCORING CRITERIA:

For symbol-lookup questions (have "complexity": "symbol-lookup"):
- 1.0 = Exact value match from correct file with line number
- 0.5 = Correct file found but value not confirmed
- 0.0 = Wrong file or wrong value

For complex questions:
- 1.0 = Complete, correct answer with explanation and file references
- 0.75 = Correct answer with partial explanation
- 0.5 = Partially correct, missing key details
- 0.25 = Relevant information found but answer incomplete
- 0.0 = Unable to answer or incorrect

OUTPUT FORMAT for each question:
Q[id]: [question text truncated to 80 chars]
Files: [files searched/read]
Answer: [your answer with file:line references]
Confidence: [HIGH/MEDIUM/LOW]
Score: [self-assessed 0.0-1.0]

STRATEGY:
1. Read the question file first
2. Work through questions IN ORDER as they appear
3. For each question: search relevant files, synthesize answer, cite sources with file:line
4. Answer as many questions as possible within the time limit

FINAL REPORT FORMAT:
=== BASELINE BENCHMARK RESULTS ===
Time Elapsed: [minutes]
Questions Attempted: [X/120]
By Complexity:
  - symbol-lookup: [X attempted]
  - multi-file: [X attempted]
  - cross-module: [X attempted]
  - disambiguation: [X attempted]
  - computed_value: [X attempted]
  - relationship: [X attempted]
Disqualified: [Yes/No]

ANSWERS:
[List each answered question with: id, answer summary, file:line, confidence, score]
===

BEGIN NOW - Read the question file and start answering in order.
```

### Rule Violations

| Violation | Consequence |
|-----------|-------------|
| Searching the web | Immediate disqualification |
| Accessing repos outside whk-wms | Immediate disqualification |
| Exceeding 20-minute time limit | Test terminated, partial results only |

### Scoring for Complex Questions

| Score | Criteria |
|-------|----------|
| 1.0 | Complete, correct answer with explanation and file references |
| 0.75 | Correct answer with partial explanation |
| 0.5 | Partially correct, missing key details |
| 0.25 | Relevant information found but answer incomplete |
| 0.0 | Unable to answer or incorrect |

### Symbol-Lookup Question Scoring

| Score | Criteria |
|-------|----------|
| 1.0 | Exact value match from correct file |
| 0.5 | Correct file found but value not confirmed |
| 0.0 | Wrong file or wrong value |

---

## Required Benchmark Metrics

All benchmark reports MUST include the following metrics. This comprehensive list ensures reproducibility and blocks skepticism paths.

### Core Outcome Metrics (Must-Have)

These answer: "Did it work?"

| # | Metric | Description |
|---|--------|-------------|
| 1 | **Mean Score (μ)** | Primary scalar score across the battery |
| 2 | **Std Dev (σ) + CV** | CV = 100·σ/μ - Stability across runs |
| 3 | **Median + Percentiles** | p10 / p25 / p75 / p90 distribution |
| 4 | **Min Score** | Worst-case / edge-case survivability |
| 5 | **High-Confidence Rate** | % with score ≥ 0.7 (publish threshold explicitly) |
| 6 | **Completion Rate** | % answered without stalling, "UNKNOWN," or partials |

### Grounding & Evidence Metrics

These answer: "Was it actually grounded in repo truth?"

| # | Metric | Description |
|---|--------|-------------|
| 7 | **Evidence Compliance Rate (ECR)** | % answers with: file paths, symbol names, exact values, line anchors |
| 8 | **Evidence Correctness Rate (E-Accuracy)** | % cited values that are actually correct |
| 9 | **Hard-Value Resolution Rate (HVRR)** | % exact-value questions correctly resolved with evidence |
| 10 | **Refusal/Not-Found Correctness** | % correct "not found" on trap questions |
| 11 | **Hallucination Rate** | Count of claims not supported by cited sources |
| 12 | **Guess Rate vs Grounded Rate** | % using hedging ("likely," "probably") vs sourced answers |

### Prior-Resistance Metrics

These answer: "Did priors inflate results?"

| # | Metric | Description |
|---|--------|-------------|
| 13 | **Tier Split Performance** | Report separately for Tier 1 (prior-friendly) vs Tier 2 (evidence-locked) |
| 14 | **Evidence Lift (Tier 2)** | Tier 2 ECR/HVRR comparison baseline vs MDEMG |
| 15 | **Exactness Error Rate** | % returning plausible but wrong values on exact-value questions |

### Multi-hop / Multi-file Rigor Metrics

These answer: "Is this hard-mode or toy-mode?"

| # | Metric | Description |
|---|--------|-------------|
| 16 | **Multi-file Coverage** | % requiring ≥2 files, avg required files per question |
| 17 | **Multi-hop Depth** | Performance by hop count (2,3,4…) |
| 18 | **Cross-module Questions** | % spanning multiple subsystems, performance on subset |
| 19 | **Effective Default Accuracy** | Declared vs runtime-effective defaults |

### Scale & Performance Metrics

These answer: "Is it practical?"

| # | Metric | Description |
|---|--------|-------------|
| 20 | **Total Elapsed Test Time** | Wall-clock time from test start to completion (minutes) |
| 21 | **Auto-Compact Events Count** | Number of context window compaction events during test |
| 22 | **Total Tokens Consumed** | Sum of input + output tokens across entire test run |
| 23 | **Ingestion Throughput** | LOC, elements/sec, observations/sec, wall time |
| 24 | **Embedding Coverage + Health** | Track and publish for credibility |
| 25 | **DB Footprint** | Neo4j size, vector index size, peak memory |
| 26 | **Retrieval Latency** | p50/p95/p99 for: vector fetch, graph traversal, rerank, e2e |
| 27 | **Token Efficiency** | Tokens/question (avg/p95), total tokens/run |
| 28 | **Tool Call Budget** | Tool calls/question (avg/p95), total tool calls |
| 29 | **Cost Proxy** | Estimated $ cost (tokens + tool calls) or GPU-hours |

### Reproducibility & Integrity Metrics

These answer: "Can anyone rerun it and get the same thing?"

| # | Metric | Description |
|---|--------|-------------|
| 30 | **Repo commit hash + scope** | URL, commit, filetype allowlist/denylist, directory scope |
| 31 | **Question set identity** | Dataset SHA-256, question IDs, seeds if sampled |
| 32 | **Cold vs warm disclosure** | Learning edges reset? Caches cleared? |
| 33 | **Determinism controls** | Temperature, decoding settings, max tool calls/time |
| 34 | **Run Count (k)** | Minimum 5, ideally 8 runs |

### Multi-Repo Integrity (if applicable)

These answer: "Does it confuse corpora?"

| # | Metric | Description |
|---|--------|-------------|
| 35 | **Cross-Repo Contamination Rate (CRCR)** | % citing files from wrong repo |
| 36 | **Repo Attribution Accuracy (RAA)** | % citing exclusively from correct repo |
| 37 | **Collision Set Performance** | Performance on adversarial overlapping vocabulary questions |

### Minimum Publication Set (12 metrics)

For compact but defensible reports, publish at minimum:

| Category | Metrics |
|----------|---------|
| Distribution | Mean, Std, CV, Min, Median, p10/p90 |
| Quality | Completion rate, ECR, E-Accuracy, HVRR |
| Performance | Retrieval latency p50/p95, Tokens/question, Tool calls/question |
| Reproducibility | Dataset hash, seeds/IDs, repo commit, scope |

### Report Table Format

Use this format for condition comparison:

| Condition | Mean | CV% | Min | p10 | p90 | Comp% | ECR% | E-Acc% | HVRR% | p95 Lat(ms) | Tok/Q |
|-----------|------|-----|-----|-----|-----|-------|------|--------|-------|-------------|-------|
| Baseline  |      |     |     |     |     |       |      |        |       |             |       |
| MDEMG     |      |     |     |     |     |       |      |        |       |             |       |

**CRITICAL:** Always publish raw per-question outputs (JSONL) for audit.

---

## Appendix: Templates and Scripts

### A. Question Template

```json
{
  "id": 1,
  "category": "service_relationships",
  "question": "What services does X depend on for Y?",
  "answer": "X constructor injects: 1) Service A - purpose; 2) Service B - purpose; ...",
  "required_files": [
    "/path/to/service-a.ts",
    "/path/to/service-b.ts"
  ],
  "complexity": "multi-file"
}
```

### B. Scoring Rubric

| Score | Criteria |
|-------|----------|
| 1.0 | All key points correct, correct file references, correct terminology |
| 0.5 | Main concept correct but missing details, minor errors in specifics |
| 0.0 | Unable to answer or answer is vague/generic |
| -1.0 | Confidently states incorrect information |

### C. File List Generation

```bash
# TypeScript/JavaScript projects
find . -type f \( -name "*.ts" -o -name "*.tsx" -o -name "*.js" \) \
  | grep -v node_modules | grep -v dist | sort

# Python projects
find . -type f -name "*.py" \
  | grep -v __pycache__ | grep -v .venv | sort

# Go projects
find . -type f -name "*.go" \
  | grep -v vendor | sort
```

### D. Quick Verification Commands

```bash
# Check MDEMG health
curl -s localhost:8090/healthz

# Get space stats
curl -s 'localhost:8090/v1/memory/stats?space_id=your-project' | jq

# Test retrieval
curl -s localhost:8090/v1/memory/retrieve \
  -H 'content-type: application/json' \
  -d '{"space_id":"your-project","query_text":"How does X work?","top_k":5}' | jq '.results[].path'

# Check learning edges
docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword \
  "MATCH ()-[r:CO_ACTIVATED_WITH]->() RETURN count(r)"
```

### E. Sample Comparison Output

```
=== COMPARISON SUMMARY ===
Baseline Avg: 0.050
MDEMG Avg:    0.710
Delta:        +0.660 (14.2x improvement)

Score Distribution:
  >0.7:     Baseline 0%,  MDEMG 64%
  0.6-0.7:  Baseline 0%,  MDEMG 22%
  0.5-0.6:  Baseline 5%,  MDEMG 8%
  0.4-0.5:  Baseline 5%,  MDEMG 5%
  <0.4:     Baseline 90%, MDEMG 1%

Learning Edges Created: 8,622 (86 pairs/query)
===
```

---

## End-to-End Multi-Corpus Runbook

This runbook defines the complete end-to-end process for multi-corpus benchmarking (e.g., WHK-WMS + plc-gbt). It ensures reproducibility, captures all required artifacts, and blocks skepticism paths.

### Phase 0 — Preflight Receipts (Must Capture)

Capture these BEFORE you touch the DB:

#### Environment + Config

```bash
# Capture preflight receipts
mdemg_commit=$(cd /Users/reh3376/mdemg && git rev-parse HEAD)
neo4j_version=$(curl -s http://localhost:7474/db/data/ | jq -r '.neo4j_version // "unknown"')
hardware_info=$(system_profiler SPHardwareDataType 2>/dev/null | grep -E "Chip|Memory|Cores" || echo "N/A")
disk_free=$(df -h . | tail -1 | awk '{print $4}')

echo "MDEMG Commit: $mdemg_commit"
echo "Neo4j Version: $neo4j_version"
echo "Hardware: $hardware_info"
echo "Disk Free: $disk_free"
```

| Receipt | How to Capture |
|---------|----------------|
| MDEMG commit hash | `git rev-parse HEAD` |
| Config snapshot | Copy `.env` or export to JSON |
| Neo4j version, DB name, path | `curl localhost:7474/db/data/` |
| Hardware | CPU, RAM, GPU (if any), disk free |

#### Corpora Identity

| Receipt | How to Capture |
|---------|----------------|
| whk-wms repo commit | `cd /path/to/whk-wms && git rev-parse HEAD` |
| whk-wms ingest scope | Filetype allowlist/denylist, excluded dirs |
| plc-gbt repo commit | `cd /path/to/plc-gbt && git rev-parse HEAD` |
| plc-gbt ingest scope | Filetype allowlist/denylist, excluded dirs |

#### Question Sets Identity

| Receipt | How to Capture |
|---------|----------------|
| Question bank file name(s) | Full paths |
| SHA-256 of question set(s) | `shasum -a 256 <file>` |
| Sampling seed or question IDs | If sampling, document seed or explicit ID list |

### Phase 1 — Benchmark Test Summary (Required Artifacts)

Every benchmark run MUST produce these artifacts:

#### Required Artifact Files

| File | Description |
|------|-------------|
| `run_manifest.json` | Immutable receipts (commits, config, timestamps) |
| `per_question_results.jsonl` | One JSON record per question |
| `aggregate_metrics.json` | Computed from JSONL |
| `neo4j_snapshot.json` | Counts + high-level DB state |
| `stdout.log` | Raw run output |

#### run_manifest.json Template

```json
{
  "run_id": "benchmark-v12-20260124-043000",
  "timestamp": "2026-01-24T04:30:00Z",
  "operator": "reh3376",
  "mdemg_commit": "abc123...",
  "neo4j_version": "5.x",
  "hardware": {
    "cpu": "Apple M2 Max",
    "ram": "64GB",
    "gpu": "N/A"
  },
  "corpora": {
    "whk-wms": {
      "commit": "def456...",
      "space_id": "whk-wms",
      "scope_paths": ["/apps/whk-wms", "/apps/whk-wms-front-end"],
      "filetypes": ["*.ts", "*.tsx", "*.json"],
      "excluded": [".git", "node_modules", "dist"]
    },
    "plc-gbt": {
      "commit": "ghi789...",
      "space_id": "plc-gbt",
      "scope_paths": ["/"],
      "filetypes": ["*.ts", "*.py", "*.json"],
      "excluded": [".git", "node_modules", "plc_backups", "n8n-framework"]
    }
  },
  "question_set": {
    "file": "test_questions_v4_selected.json",
    "sha256": "abc123...",
    "n_questions": 100,
    "sampling_seed": null
  },
  "config": {
    "RERANK_ENABLED": true,
    "RERANK_WEIGHT": 0.15,
    "RERANK_MODEL": "gpt-4o-mini"
  },
  "cold_start": false,
  "initial_learning_edges": 5926
}
```

#### per_question_results.jsonl Format

```jsonl
{"id":"q001","question":"What is MAX_TAKE?","score":0.85,"top_file":"pagination.constants.ts","latency_ms":234,"tokens":1200,"tool_calls":3,"evidence_found":true,"space_id":"whk-wms"}
{"id":"q002","question":"What services does X inject?","score":0.72,"top_file":"barrel.service.ts","latency_ms":456,"tokens":1800,"tool_calls":5,"evidence_found":true,"space_id":"whk-wms"}
```

#### Required Metrics to Compute

**Core Metrics:**

| Metric | Description |
|--------|-------------|
| mean score | Average across all questions |
| median | Middle value |
| std, CV | Standard deviation, Coefficient of Variation |
| p10/p90 | 10th/90th percentiles |
| min score | Worst case |
| completion rate | % answered without stalling |

**Grounding Metrics:**

| Metric | Description |
|--------|-------------|
| Evidence Compliance Rate (ECR) | % with file paths, line numbers, values |
| Evidence Accuracy (E-Acc) | % cited values actually correct |
| Hard-Value Resolution Rate (HVRR) | % exact-value questions correct |
| Hallucination rate | Claims not supported by citations |
| Refusal correctness | % correct "not found" on trap questions |

**Efficiency Metrics:**

| Metric | Description |
|--------|-------------|
| tokens/question avg, p95 | Token consumption |
| tool calls/question avg, p95 | Tool call budget |
| latency p50/p95 | Retrieval + end-to-end |
| total wall time | Complete run duration |

**Reproducibility:**

| Metric | Description |
|--------|-------------|
| k runs | Number of runs (minimum 5) |
| seeds/ID lists | For sampling |
| dataset hash | SHA-256 of question file |
| repo commit hashes | All corpora |

### Phase 2 — Ingest Second Corpus (Multi-Space)

**CRITICAL RULES:**
- Do NOT delete or clear existing space nodes/edges
- Ingest new corpus into its OWN SpaceID
- Cross-space edges should be disallowed or tracked as contamination

#### Ingest Command

```bash
# Ingest plc-gbt into its own space (whk-wms remains)
go run ./cmd/ingest-codebase/main.go \
  -path /Users/reh3376/repos/plc-gbt \
  -space-id plc-gbt \
  -extract-symbols=false \
  -exclude ".git,vendor,node_modules,plc_backups,n8n-framework,n8n-mcp"
```

#### Required Ingest Logs

| Metric | Description |
|--------|-------------|
| LOC ingested | Lines of code |
| filetype diversity | Count + list of file types |
| elements | Code elements extracted |
| observations | Observations created |
| embedding coverage | % with embeddings |
| health score | Post-ingest health |
| ingestion time | Wall clock time |
| throughput | elements/sec |

#### Post-Ingest Invariants

- [ ] Nodes/edges for first corpus unchanged (except global indexes)
- [ ] Second corpus has non-zero nodes/edges
- [ ] Cross-space edges = 0 (or tracked as contamination)

### Phase 3 — Consolidate (Multi-Corpus)

Run consolidation with both SpaceIDs present:

```bash
# Consolidate each space
curl -X POST 'http://localhost:8090/v1/memory/consolidate' \
  -H 'Content-Type: application/json' \
  -d '{"space_id":"whk-wms"}'

curl -X POST 'http://localhost:8090/v1/memory/consolidate' \
  -H 'Content-Type: application/json' \
  -d '{"space_id":"plc-gbt"}'
```

#### Required Consolidation Logs

| Metric | Description |
|--------|-------------|
| nodes/edges created | During consolidation |
| concept layer nodes per space | Hidden/concept nodes |
| co-activation edges | Learning edges created |
| time spent | Consolidation duration |
| post-consolidation health | Health score |

### Phase 4 — Neo4j Verification (Critical)

Execute these Cypher queries to verify isolation:

#### 4.1 Counts by SpaceID

```cypher
MATCH (n:MemoryNode)
RETURN n.spaceId AS spaceId, count(n) AS nodes
ORDER BY nodes DESC;
```

#### 4.2 Edges by SpaceID

```cypher
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
RETURN a.spaceId AS fromSpace, b.spaceId AS toSpace, type(r) AS relType, count(r) AS rels
ORDER BY rels DESC;
```

#### 4.3 Cross-Space Contamination Check (CRITICAL)

```cypher
-- Count cross-space relationships
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE a.spaceId <> b.spaceId
RETURN count(r) AS crossSpaceRels;

-- If non-zero, list examples:
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE a.spaceId <> b.spaceId
RETURN a.spaceId, type(r), b.spaceId, a.name, b.name
LIMIT 25;
```

#### 4.4 Hidden Layer Nodes per Space

```cypher
MATCH (n:MemoryNode)
WHERE n.layer > 0
RETURN n.spaceId AS spaceId, n.layer AS layer, count(n) AS count
ORDER BY spaceId, layer;
```

#### 4.5 File Distribution per Space

```cypher
MATCH (o:Observation)
RETURN o.spaceId AS spaceId, count(o) AS files
ORDER BY files DESC;
```

#### Pass Criteria for "Consolidation Confirmed"

- [ ] Both spaces present with expected magnitude
- [ ] crossSpaceRels == 0 (or explain why not)
- [ ] Concept/cluster layers exist in each space
- [ ] Basic retrieval returns evidence from correct space

### Phase 5 — Multi-Corpus Benchmark

Run benchmark with both corpora loaded:

#### Additional Required Metrics

| Metric | Description |
|--------|-------------|
| Repo Attribution Accuracy (RAA) | % answers citing only correct spaceId |
| Cross-Repo Contamination Rate (CRCR) | % citing wrong spaceId |
| Collision subset performance | Performance on overlapping vocabulary |

#### Collision Questions

If not present, add 10 questions designed to cause confusion between repos (e.g., shared constant names, similar module names).

### Benchmark Summary Template

Use this exact structure for reports:

```markdown
# Benchmark Summary

## 1) Run Identity

| Field | Value |
|-------|-------|
| Date/time (TZ) | |
| Operator | |
| MDEMG commit | |
| Neo4j version / DB | |
| Hardware | |
| Run type | single-corpus / multi-corpus / consolidation-verify |
| Cold-start? | Y/N - what was cleared? |

## 2) Corpora + Scope

### whk-wms
| Field | Value |
|-------|-------|
| commit | |
| scope paths | |
| filetypes | |
| LOC | |
| SpaceID | |

### plc-gbt
| Field | Value |
|-------|-------|
| commit | |
| scope paths | |
| filetypes | |
| LOC | |
| SpaceID | |

## 3) Ingestion Metrics

| Metric | whk-wms | plc-gbt |
|--------|---------|---------|
| elements | | |
| observations | | |
| embedding coverage | | |
| health score | | |
| ingest time | | |
| throughput (elem/sec) | | |

## 4) Consolidation Metrics

| Metric | Value |
|--------|-------|
| consolidation time | |
| nodes added | |
| edges added | |
| concept nodes added | |
| learning edges added | |
| post-consolidation health | |

## 5) DB Verification (Neo4j)

| Metric | Value |
|--------|-------|
| nodes by space | |
| rels by space | |
| cross-space relationships | |
| filetype distribution | |
| PASS/FAIL | |

## 6) Benchmark Battery

| Field | Value |
|-------|-------|
| question set name | |
| SHA-256 | |
| N questions | |
| tiers (Tier1/Tier2) | |
| sampling seed | |
| k runs | |

## 7) Results (Aggregate)

### Core
| Metric | Value |
|--------|-------|
| mean | |
| median | |
| std | |
| CV | |
| p10/p90 | |
| min | |
| completion % | |

### Grounding
| Metric | Value |
|--------|-------|
| ECR % | |
| E-Acc % | |
| HVRR % | |
| hallucination rate | |
| refusal correctness % | |

### Multi-corpus Integrity
| Metric | Value |
|--------|-------|
| RAA % | |
| CRCR % | |
| collision subset mean | |

### Efficiency
| Metric | Value |
|--------|-------|
| tokens/Q avg, p95 | |
| tool calls/Q avg, p95 | |
| latency p50/p95 | |
| wall time | |

## 8) Top Failure Modes

- Examples of wrong answers with evidence issues
- Examples of repo confusion (if any)
- Timeouts / stalls / unknowns
```

### Skeptic-Killer Requirements

1. **Evidence must include repo identity** (spaceId or repo label) in every cited file path
2. **Publish raw JSONL** per-question results for audit

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.4 | 2026-01-26 | Added CRITICAL finding about edge accumulation causing score degradation; updated cold start procedure with data showing 0.740 vs 0.679 score difference |
| 1.3 | 2026-01-24 | Added End-to-End Multi-Corpus Runbook with Phase 0-5, artifact requirements, Neo4j verification queries, and benchmark summary template |
| 1.2 | 2026-01-23 | Added WHK-WMS 120-question test set, baseline benchmarking rules (20-min limit, repo restrictions, disqualification criteria), master vs agent file distinction |
| 1.1 | 2026-01-23 | Added V6 Composite Test Set documentation |
| 1.0 | 2026-01-23 | Initial comprehensive guide |

