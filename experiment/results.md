# MDEMG Context Retention Experiment Results

**Date:** 2026-01-21
**Codebase:** aci-claude-go
**Test Questions:** 20 (see test_questions.md)

## Experiment Setup

### Baseline (Without MDEMG)
- Method: Systematically read source files into context window
- Files read: ~40 key source files (agent/, config/, tui/, etc.)
- Context: Full file contents available

### MDEMG Experiment
- Method: Ingest codebase into MDEMG, then query for context
- Elements ingested: 405 / 1,774 (23% coverage)
- Consolidation: 1 hidden node created, 2 updated
- Embedding provider: OpenAI text-embedding-ada-002

## Results Summary

| Question | Category | Baseline | MDEMG | Delta |
|----------|----------|----------|-------|-------|
| Q1 | Architecture | 1.0 | 0.5 | -0.5 |
| Q2 | Architecture | 1.0 | 0.5 | -0.5 |
| Q3 | Architecture | 1.0 | 0.5 | -0.5 |
| Q4 | Architecture | 1.0 | 0.0 | -1.0 |
| Q5 | Architecture | 1.0 | 0.0 | -1.0 |
| Q6 | Implementation | 1.0 | 1.0 | 0.0 |
| Q7 | Implementation | 1.0 | 0.0 | -1.0 |
| Q8 | Implementation | 1.0 | 0.5 | -0.5 |
| Q9 | Implementation | 1.0 | 0.0 | -1.0 |
| Q10 | Implementation | 1.0 | 0.0 | -1.0 |
| Q11 | Specific Code | 1.0 | 1.0 | 0.0 |
| Q12 | Specific Code | 1.0 | 0.0 | -1.0 |
| Q13 | Specific Code | 1.0 | 0.0 | -1.0 |
| Q14 | Specific Code | 1.0 | 0.5 | -0.5 |
| Q15 | Specific Code | 1.0 | 1.0 | 0.0 |
| Q16 | Deep Knowledge | 1.0 | 0.0 | -1.0 |
| Q17 | Deep Knowledge | 1.0 | 0.0 | -1.0 |
| Q18 | Deep Knowledge | 1.0 | 0.0 | -1.0 |
| Q19 | Deep Knowledge | 1.0 | 0.0 | -1.0 |
| Q20 | Deep Knowledge | 1.0 | 0.0 | -1.0 |
| **Average** | | **1.0** | **0.275** | **-0.725** |

## Score by Category

| Category | Questions | Baseline Avg | MDEMG Avg |
|----------|-----------|--------------|-----------|
| Architecture | Q1-Q5 | 1.0 | 0.3 |
| Implementation | Q6-Q10 | 1.0 | 0.3 |
| Specific Code | Q11-Q15 | 1.0 | 0.5 |
| Deep Knowledge | Q16-Q20 | 1.0 | 0.0 |

## Analysis

### Why MDEMG Underperformed

1. **Partial Ingestion (23%)**: Only 405 of 1,774 code elements were ingested before timeout. Key packages like `internal/agent/`, `internal/tui/views/`, and documentation files were not fully captured.

2. **Shallow Content**: The ingest-codebase tool captures:
   - Function/type names
   - Package names
   - File paths
   - Parameter/return counts

   But NOT:
   - Actual code implementation
   - Documentation text (CLAUDE.md, README.md)
   - Default values
   - Specific patterns

3. **Query Limitations**: MDEMG retrieves nodes by semantic similarity, but specific details (default values, exact paths) require exact content matches.

### Where MDEMG Succeeded

- **Q6 (Agent Interface)**: Correctly inferred Type() and Run() methods from Coder agent retrievals
- **Q11 (CLI Entry Point)**: Correctly identified cmd/aci/main.go
- **Q15 (Config Package)**: Correctly identified internal/config/config.go

### Key Observations

1. **Path-based questions** worked well (Q11, Q15) because file paths are preserved
2. **Method signature questions** partially worked (Q6) through inference
3. **Deep knowledge questions** (Q16-Q20) failed entirely - specific defaults, patterns, and conventions are not captured in summaries

## Recommendations for Improved MDEMG Performance

### Short-term Fixes

1. **Complete ingestion**: Allow full 1,774 element ingestion (estimated 45 minutes with current settings)
2. **Increase batch timeout**: Modify ingest-codebase to use longer HTTP timeouts
3. **Parallel embedding**: Use local Ollama to avoid API rate limits

### Structural Improvements

1. **Ingest documentation**: Add markdown file ingestion (README.md, CLAUDE.md, docs/*.md)
2. **Rich content extraction**: Include actual code content, not just signatures
3. **Configuration capture**: Extract environment variable defaults and config values
4. **Relationship extraction**: Create explicit edges for imports, calls, implements

### Hidden Layer Enhancements

1. **More consolidation passes**: Run 5-10 consolidation cycles to strengthen patterns
2. **Larger clusters**: Adjust DBSCAN parameters for better clustering
3. **Concept naming**: Generate meaningful names for hidden nodes via LLM reflection

## Conclusion

This experiment demonstrates that MDEMG's effectiveness depends heavily on:

1. **Ingestion completeness**: 23% coverage is insufficient
2. **Content richness**: Summaries must capture specific details
3. **Query strategy**: Multiple queries may be needed per question

The baseline (direct file reading) outperformed MDEMG significantly because it preserves full content. However, MDEMG is designed for scenarios where context compression has already occurred and full content is unavailable. A fair comparison would require:

1. Full codebase ingestion
2. Complete context compression (not simulated)
3. MDEMG as the only available context source

**Files:**
- Test questions: `/Users/reh3376/mdemg/experiment/test_questions.md`
- Results: `/Users/reh3376/mdemg/experiment/results.md`
- MDEMG space: `aci-claude-experiment` (405 nodes)
