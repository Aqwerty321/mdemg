# MDEMG Benchmark Summary

**Version:** 3.0
**Last Updated:** 2026-01-26
**Canonical Format:** v22

---

## 1. Canonical Benchmarks

MDEMG maintains two canonical benchmark suites in v22 format:

| Benchmark | Codebase | Questions | Runs | Documentation |
|-----------|----------|-----------|------|---------------|
| **whk-wms v22** | whk-wms (507K LOC TypeScript) | 120 | 2 per condition | `whk-wms/benchmark_v22_test/BENCHMARK_RESULTS_V22.md` |
| **clawdbot v1** | clawdbot (multi-service Python) | 130 | 3 per condition | `clawdbot/BENCHMARK_CLAWDBOT_V1.md` |

All other benchmark directories are historical development artifacts and should not be used for comparison.

---

## 2. Key Results (whk-wms v22)

### Q&A Battery Performance

| Metric | Baseline | MDEMG | Notes |
|--------|----------|-------|-------|
| Mean Score | 0.834 | 0.820 | Within margin of error |
| Evidence Rate | 100% | 97.1% | Both conditions high |
| High Confidence (>0.7) | 100% | 94.2% | Run 1 cold start |
| Run-to-Run Improvement | +2.9% | +3.0% | Similar learning |

**Observation:** Q&A battery is ECR-saturated (97-100% evidence compliance), limiting mean score differentiation on evidence dimension.

### State Survival Under Compaction

| Metric | Baseline | MDEMG | Source |
|--------|----------|-------|--------|
| Decision Persistence @5 compactions | 0% | 95% | Compaction torture test |

**Key Insight:** Single-turn retrieval accuracy is comparable. The differentiator is state survival when context windows fill and auto-compaction occurs.

---

## 3. Test Configuration

### Reproducibility Hashes

| File | SHA-256 |
|------|---------|
| `test_questions_120_agent.json` | `24aa17a215e4e58b8b44c7faef9f14228edb0e6d3f8f657d867b1bfa850f7e9e` |
| `grade_answers.py` | `5dbf84f092db31e4bc0d4867fd412c7af6575855f7c71e3d2f65e2ee8a8a21a5` |

### Grading Formula

```
score = min(0.70 * evidence + 0.15 * semantic + 0.15 * concept + file_bonus, 1.0)
```

| Component | Weight | Description |
|-----------|--------|-------------|
| Evidence | 70% | ECR score (file:line citations) |
| Semantic | 15% | N-gram Jaccard similarity |
| Concept | 15% | Technical concept overlap |
| File Bonus | +10% | Correct file basename cited |

### Baseline Definition

Same agent runner and tool permissions, **no MDEMG retrieval**, relying on long-context + auto-compaction only (memory off).

---

## 4. Limitations

- **Sample size:** n=2 per condition insufficient for statistical significance
- **ECR ceiling effect:** High compliance rates limit mean score differentiation
- **Single codebase type:** Results may not generalize to other languages/structures

---

## 5. Archived Historical Results

Previous benchmark iterations (v4-v11, v20, v21) used different methodologies and grading formulas. These have been archived and should not be compared directly to v22 results.

Key methodological differences:
- Earlier versions used different question sets
- Grading weights evolved across versions
- Agent models varied (Haiku vs Sonnet vs Opus)
- Cold start vs warm start configurations differed

For current benchmarking, use only v22 format benchmarks with the canonical grading script.

---

## 6. Running Benchmarks

See [BENCHMARKING_GUIDE.md](./BENCHMARKING_GUIDE.md) for detailed instructions on:
- Setting up test environments
- Running baseline vs MDEMG conditions
- Grading and analyzing results
- Avoiding answer contamination
