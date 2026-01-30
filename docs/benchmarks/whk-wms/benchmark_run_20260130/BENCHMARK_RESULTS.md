# whk-wms Benchmark Results (ID-Corrected)

**Benchmark Date:** 2026-01-30
**Codebase:** whk-wms (507K LOC TypeScript)
**Questions:** 120 (Test Question Schema v1.0)
**Runs:** 2 baseline + 2 MDEMG
**Grading:** grader_v4.py (evidence-weighted scoring)

---

## Executive Summary

| Metric | Baseline (avg) | MDEMG (valid) | Delta |
|--------|----------------|---------------|-------|
| **Mean Score** | 0.550 | 0.513 | -0.037 (-6.7%) |
| **Evidence Rate** | 60.0% | 55.0% | -5.0pp |
| **High Score Rate** | 16.7% | 10.8% | -5.9pp |

**Key Finding:** With corrected ID preservation (100% match for all runs), baseline slightly outperforms MDEMG on this codebase.

---

## ID Preservation Success

This benchmark used corrected prompts with explicit ID preservation instructions.

| Run | IDs Matched | Status |
|-----|-------------|--------|
| Baseline Run 1 | 120/120 (100%) | Valid |
| Baseline Run 2 | 120/120 (100%) | Valid |
| MDEMG Run 1 | 120/120 (100%) | Invalid (formatting issue) |
| MDEMG Run 2 | 120/120 (100%) | Valid |

**Note:** MDEMG Run 1 had correct IDs but included annotations in file_line_refs (e.g., `"file.ts:20-29 (description)"`) which the grader couldn't parse, resulting in 61.7% "None" evidence tier.

---

## Detailed Run Results

### Per-Run Scores

| Run | Mode | Mean | Std | CV% | High Score Rate | Evidence Rate |
|-----|------|------|-----|-----|-----------------|---------------|
| 1 | Baseline | **0.583** | 0.161 | 27.6% | 19.2% | 69.2% |
| 2 | Baseline | 0.517 | 0.191 | 36.9% | 14.2% | 50.8% |
| 1 | MDEMG | 0.172 | 0.141 | 82.0% | 0.0% | 0.8% (excluded) |
| 2 | MDEMG | 0.513 | 0.189 | 36.8% | 10.8% | 55.0% |

### Evidence Tier Distribution (Valid Runs)

| Run | Strong | Moderate | Weak | None |
|-----|--------|----------|------|------|
| Baseline 1 | 0.0% | 69.2% | 30.8% | 0.0% |
| Baseline 2 | 0.0% | 50.8% | 45.0% | 4.2% |
| MDEMG 2 | 0.0% | 55.0% | 41.7% | 3.3% |

---

## Analysis

### Baseline Performance
- Baseline Run 1 (0.583) significantly outperformed Run 2 (0.517)
- Higher evidence rate in Run 1 (69.2% vs 50.8%) drove the score difference
- Both runs had 100% ID preservation with corrected prompts

### MDEMG Performance
- MDEMG Run 2 (0.513) performed within 6.7% of baseline average
- Evidence rate (55.0%) was between the two baseline runs
- MDEMG Run 1 was invalid due to formatting issue, not ID issue

### Prompt Correction Impact
- Previous benchmark (2026-01-29) had 50-67% ID match for MDEMG runs
- This benchmark achieved 100% ID match for all runs
- Stronger ID preservation instructions resolved the issue

---

## Conclusions

### Findings

1. **ID Preservation Fixed:** Explicit instructions in prompts achieved 100% ID match
2. **Baseline Advantage:** On whk-wms, baseline outperforms MDEMG by ~6.7%
3. **Evidence Quality:** No strong evidence matches in any run (all moderate or weak)
4. **Formatting Matters:** Agent answer formatting affects grading (annotations broke parser)

### Recommendations

1. **Prompt Templates Updated:** `BENCHMARK_PROMPT_TEMPLATES.md` now includes stronger ID preservation instructions
2. **Answer Format:** Agents should use clean `file.ts:123` format, not `file.ts:123 (description)`
3. **Grader Enhancement:** Consider making grader more tolerant of annotation suffixes

---

## Files Generated

```
docs/benchmarks/whk-wms/benchmark_run_20260130/
├── answers_baseline_run1.jsonl (120 answers, 100% ID match)
├── answers_baseline_run2.jsonl (120 answers, 100% ID match)
├── answers_mdemg_run1.jsonl (120 answers, 100% ID match, bad formatting)
├── answers_mdemg_run2.jsonl (120 answers, 100% ID match)
├── grades_baseline_run1.json
├── grades_baseline_run2.json
├── grades_mdemg_run1.json (excluded - formatting issue)
├── grades_mdemg_run2.json
└── BENCHMARK_RESULTS.md (this file)
```

---

## Appendix: Grading Methodology

**Grader:** grader_v4.py (v4.1 Spec)

| Component | Weight | Description |
|-----------|--------|-------------|
| Evidence Score | 70% | File:line citations matching expected |
| Semantic Score | 15% | N-gram + recall-weighted similarity |
| Concept Score | 15% | Technical concept overlap |
| Citation Bonus | +10% | Citing correct file (capped at 1.0) |

**Evidence Tiers:**
- Strong (1.0): file:line AND file matches AND line within +/- 10
- Moderate (0.7): file:line AND file matches BUT line outside tolerance
- Weak (0.4): file:line BUT file doesn't match expected
- Minimal (0.2): File name without line number
- None (0.0): No file references

---

## Comparison with Previous Benchmark (2026-01-29)

| Metric | Previous (partial ID) | Current (100% ID) |
|--------|----------------------|-------------------|
| Baseline Mean | 0.509 | 0.550 |
| MDEMG Mean | 0.525 (81/120) | 0.513 (120/120) |
| ID Match Rate | 50-100% | 100% |
| Valid Comparison | Limited | Full |

The current benchmark provides a more reliable comparison due to complete ID preservation.
