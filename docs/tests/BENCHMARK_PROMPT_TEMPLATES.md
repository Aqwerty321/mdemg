# Benchmark Prompt Templates

Standardized prompts for MDEMG benchmark agents to ensure consistent output format.

---

## Baseline Agent Prompt

```
You are answering benchmark questions about the {CODEBASE} codebase.

## STRICT RULES
1. You may ONLY access files within: {REPO_PATH}
2. You may NOT use WebSearch or WebFetch
3. You MUST answer ALL {N} questions
4. You MUST include file:line references in every answer
5. Use EXACT question ID from input - DO NOT renumber

## WORKFLOW (repeat for each question)
1. Read ONE question from the input file
2. Search for relevant files (Glob/Grep)
3. Read source code to find the answer
4. Write answer IMMEDIATELY to output file as a single JSON line
5. Move to next question

## OUTPUT FORMAT
File: {OUTPUT_PATH}
Format: One JSON object per line:
{"id": N, "question": "...", "answer": "...", "file_line_refs": ["path/to/file.rs:123"]}

## BEGIN
Read {QUESTIONS_FILE} and answer all {N} questions sequentially.
```

---

## MDEMG Agent Prompt

```
You are answering benchmark questions about the {CODEBASE} codebase using MDEMG context.

## STRICT RULES
1. You MUST query MDEMG first for each question
2. You may supplement with code search if MDEMG context is insufficient
3. You MUST answer ALL {N} questions
4. **CRITICAL: You MUST include file:line references in EVERY answer (e.g., "path/to/file.py:123")**
5. Use EXACT question ID from input - DO NOT renumber
6. **NEVER use just file paths - ALWAYS include line numbers**

## MDEMG QUERY
curl -s -X POST {MDEMG_ENDPOINT}/v1/memory/retrieve \
  -H "Content-Type: application/json" \
  -d '{"query_text":"<question>","space_id":"{SPACE_ID}","top_k":20}'

## EXTRACTING LINE NUMBERS FROM MDEMG
MDEMG results include line numbers in multiple places:
- `start_line` and `end_line` fields in each result
- `path` field often includes "#symbol" anchors
- Symbol metadata with line numbers

When constructing file_line_refs:
- Use the start_line from MDEMG results
- Format as: "relative/path/to/file.py:LINE_NUMBER"
- Example: If MDEMG returns path="/src/model.py" and start_line=245, use "src/model.py:245"

## WORKFLOW (repeat for each question)
1. Query MDEMG with the question text
2. Parse retrieved context for relevant file paths AND LINE NUMBERS
3. Extract start_line from each relevant result
4. If line numbers missing, search the actual source file to find them
5. Write answer with file:line references to output file
6. Move to next question

## OUTPUT FORMAT
File: {OUTPUT_PATH}
Format: One JSON object per line:

CORRECT (with line numbers):
{"id": N, "question": "...", "answer": "...", "file_line_refs": ["megatron/core/transformer/module.py:29", "megatron/core/models/gpt/gpt_model.py:45"]}

WRONG (missing line numbers - DO NOT DO THIS):
{"id": N, "question": "...", "answer": "...", "file_line_refs": ["megatron/core/transformer/module.py", "megatron/core/models/gpt/gpt_model.py"]}

## BEGIN
Read {QUESTIONS_FILE} and answer all {N} questions sequentially.
```

---

## Variables to Replace

| Variable | Description | Example |
|----------|-------------|---------|
| `{CODEBASE}` | Repository name | `zed` |
| `{REPO_PATH}` | Absolute path to repo | `/Users/reh3376/repos/zed` |
| `{OUTPUT_PATH}` | Path to output JSONL | `/path/answers_mdemg_run1.jsonl` |
| `{QUESTIONS_FILE}` | Path to agent questions JSON | `/path/benchmark_questions_v1_agent.json` |
| `{N}` | Number of questions | `142` |
| `{MDEMG_ENDPOINT}` | MDEMG API base URL | `http://localhost:3001` |
| `{SPACE_ID}` | MDEMG space identifier | `zed` |

---

## Critical Requirements for Fair Comparison

1. **Output Format**: Both baseline and MDEMG agents MUST use identical output format with `file_line_refs`
2. **Answer Structure**: Always include the question text for traceability
3. **Evidence**: **ALWAYS cite specific file:line locations (e.g., `src/model.py:245`), NEVER just file paths**
4. **No Web Access**: Neither agent type should use web search/fetch

## Common Mistakes to Avoid

### WRONG - File path without line number:
```json
{"file_line_refs": ["megatron/core/transformer/module.py"]}
```

### CORRECT - File path WITH line number:
```json
{"file_line_refs": ["megatron/core/transformer/module.py:29"]}
```

The grading script heavily penalizes missing line numbers:
- With line numbers (strong evidence): 100% evidence score
- Without line numbers (weak evidence): 50% evidence score
- This alone accounts for ~50% of the total score difference!

---

*Created: 2026-01-28*
