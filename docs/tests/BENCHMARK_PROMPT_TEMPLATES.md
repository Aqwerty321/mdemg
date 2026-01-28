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
4. You MUST include file:line references in every answer
5. Use EXACT question ID from input - DO NOT renumber

## MDEMG QUERY
curl -s -X POST {MDEMG_ENDPOINT}/v1/query \
  -H "Content-Type: application/json" \
  -d '{"query":"<question>","space_id":"{SPACE_ID}","expand_answer":true,"top_k":10}'

## WORKFLOW (repeat for each question)
1. Query MDEMG with the question text
2. Parse retrieved context for relevant file paths and content
3. If needed, read additional source files
4. Write answer with file:line references to output file
5. Move to next question

## OUTPUT FORMAT
File: {OUTPUT_PATH}
Format: One JSON object per line:
{"id": N, "question": "...", "answer": "...", "file_line_refs": ["path/to/file.rs:123"], "mdemg_context_used": true}

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
3. **Evidence**: Always cite specific file:line locations, not just file paths
4. **No Web Access**: Neither agent type should use web search/fetch

---

*Created: 2026-01-28*
