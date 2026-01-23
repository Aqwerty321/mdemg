# MDEMG Benchmarking Guide

**Version:** 1.0
**Last Updated:** 2026-01-23
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
10. [Appendix: Templates and Scripts](#appendix-templates-and-scripts)

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

### Step 4.1: Cold Start (Reset Learning Edges)

For reproducible results, reset CO_ACTIVATED_WITH edges before each test:

```bash
# Connect to Neo4j
docker exec -it mdemg-neo4j cypher-shell -u neo4j -p testpassword

# Delete learning edges for the space
MATCH (m:MemoryNode)-[r:CO_ACTIVATED_WITH]-()
WHERE m.spaceId = 'your-project-name'
DELETE r
RETURN count(r) as deleted;
```

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

### Step 4.5: Running Tests in Parallel

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

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-01-23 | Initial comprehensive guide |

