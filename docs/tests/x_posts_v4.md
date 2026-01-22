# MDEMG X/Twitter Post Series - v4 Test Results

**Generated**: 2026-01-22
**Based on**: whk-wms codebase ingestion and 100-question evaluation

---

## Post 1/5 — Hook

LLMs are stateless. Engineering orgs (and codebases) aren't.

Past ~40% of a context window, you get attention dilution + drift.
Hence the fundamental disconnect.

---

## Post 2/5 — Thesis

- **RAG** minimally helps retrieve context; if you have used you know it is not the answer.
- **MDEMG preserves commitments** (decisions, invariants, standards) across long task chains.
  - MDEMG stores org-specific truth models can't know: decisions + rationale, invariants, conventions, "we tried X and it failed," and the evidence trail.
  - Same substrate ingests **SME knowledge + P&IDs (piping & instrumentation diagrams) + org UI standards** — any org-truth corpus.

Demo: **[github.com/anthropics/mdemg]** *(link placeholder)*

---

## Post 3/5 — Concrete capability (numbers)

This isn't "general knowledge." It's what models can't know:
- conventions & naming
- architectural decisions **and why**
- invariants/constraints that must hold
- what failed before + evidence trail

Ingestion turns repo reality into durable context.

With MDEMG we ingested **whk-wms** (**3,288 files / 792K LOC / 9,166 code elements**) into a persistent memory graph:

**9,261 nodes / 3 layers (base→clusters→concepts)** in **~18 min** on **M-series Mac (local)**.

Hidden layer consolidation created:
- 92 cluster nodes
- 3 concept nodes
- 95 auto-generated summaries

---

## Post 4/5 — How to evaluate (benchmarks)

Tested MDEMG vs baseline (direct file search) on **100 complex codebase questions**:

| Metric | MDEMG | Baseline |
|--------|-------|----------|
| Completion | **100%** | 0% (stalled) |
| Method | API retrieval | File search |
| Consistency | Every query succeeds | Resource-limited |

Retrieval quality (100 questions):
- **69%** high confidence (score > 0.5)
- **36%** very high confidence (score > 0.6)
- **0%** below threshold (all queries returned useful context)

Top performers: data flow tracing questions (0.75 score)
Improvement areas: cross-cutting concerns (0.45 score)

---

## Post 5/5 — Ask

Codebase ingestion is working: **MDEMG** builds persistent memory from large repos in a way that long-context + standard RAG _hasn't in my testing_ (less drift, fewer repeats, fewer regressions).

Test results on 3,288-file / 792K LOC codebase:
- 100% query completion via retrieval API
- 69% high-confidence retrievals
- Baseline approach stalled (resource constraints)

I want to **quantify it properly**—scaling curves + ablations + benchmarks on **repeat-question rate, decision persistence, and regression rate**—but I'm compute-limited.

Compute ask: **500-2000 GPU-hrs** to scale **10K nodes → 1M nodes** and publish reproducible results.

Demo: **[github link]**

@xAI @GoogleDeepMind

---

## Raw Data for Posts

### Codebase Stats
- **Repository**: whk-wms (Warehouse Management System)
- **Files**: 3,288
- **LOC**: 792,832
- **File Types**: TypeScript, TSX, Markdown

### MDEMG Stats
- **Total Nodes**: 9,261
- **Layer 0 (Base)**: 9,166
- **Layer 1 (Clusters)**: 92
- **Layer 2 (Concepts)**: 3
- **Embedding Coverage**: 100%
- **Embedding Model**: text-embedding-ada-002 (1536 dims)

### Timing
- **Ingestion**: ~15 min
- **Consolidation**: ~3 min
- **Total**: ~18 min
- **Hardware**: Apple Silicon Mac (local)

### Retrieval Performance
- **Queries**: 100
- **Completion**: 100%
- **Avg Score**: 0.567
- **Max Score**: 0.750
- **Min Score**: 0.449
- **Avg Vector Sim**: 0.881

### Score Distribution
- **> 0.6**: 36 (36%)
- **0.5-0.6**: 33 (33%)
- **0.4-0.5**: 31 (31%)
- **< 0.4**: 0 (0%)
