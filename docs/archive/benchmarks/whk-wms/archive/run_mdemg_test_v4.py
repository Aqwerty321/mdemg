#!/usr/bin/env python3
"""
MDEMG Test Runner v4

Protocol:
1. PHASE 1: Agent ingests all files into MDEMG, then ASKS for questions
2. PHASE 2: After asking, provide questions - agent answers using MDEMG retrieval
3. Track total elapsed time from start to completion
4. Agent must NOT access /Users/reh3376/mdemg directory (contains answers)
"""

import json
import random
from pathlib import Path
from datetime import datetime

# Configuration
TEST_DIR = Path(__file__).parent
FILE_LIST = TEST_DIR / "whk-wms-file-list.txt"
QUESTIONS_SOURCE = TEST_DIR / "whk-wms-questions-final.json"
SELECTED_QUESTIONS_FILE = TEST_DIR / "test_questions_v4_selected.json"
OUTPUT_FILE = TEST_DIR / f"mdemg-test-v4-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
MDEMG_ENDPOINT = "http://localhost:8090"
SPACE_ID = "whk-wms-v4-test"
EXPECTED_FILE_COUNT = 3288
NUM_QUESTIONS = 100
RANDOM_SEED = 42

def count_files():
    with open(FILE_LIST) as f:
        return len([l for l in f.readlines() if l.strip()])

def load_and_select_questions():
    if SELECTED_QUESTIONS_FILE.exists():
        print(f"Using existing selected questions from {SELECTED_QUESTIONS_FILE}")
        with open(SELECTED_QUESTIONS_FILE) as f:
            data = json.load(f)
        return data['questions']

    with open(QUESTIONS_SOURCE) as f:
        data = json.load(f)

    all_questions = data['questions']
    random.seed(RANDOM_SEED)
    selected = random.sample(all_questions, min(NUM_QUESTIONS, len(all_questions)))

    selected_data = {
        "metadata": {
            "source": str(QUESTIONS_SOURCE),
            "total_available": len(all_questions),
            "selected_count": len(selected),
            "random_seed": RANDOM_SEED,
            "generated_at": datetime.now().isoformat()
        },
        "questions": selected
    }

    with open(SELECTED_QUESTIONS_FILE, 'w') as f:
        json.dump(selected_data, f, indent=2)

    print(f"Selected {len(selected)} random questions, saved to {SELECTED_QUESTIONS_FILE}")
    return selected


def generate_mdemg_phase1_prompt(file_count: int) -> str:
    return f"""# MDEMG Test v4 - Memory-Augmented Retrieval Experiment

---
## !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
## CRITICAL - MUST SURVIVE AUTO-COMPACT - READ FIRST
## !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!

### YOUR MISSION (MEMORIZE THIS - IT MUST SURVIVE COMPACTION):

**STEP 1:** Ingest all {file_count} files into MDEMG
**STEP 2:** Run consolidation to build hidden layer
**STEP 3:** Verify with `wc -l` and MDEMG stats
**STEP 4:** Say EXACTLY: "INGESTION COMPLETE. Please provide the test questions."

### THE QUESTIONS ARE NOT IN THIS PROMPT
- You will receive questions AFTER you complete ingestion and ASK for them
- If you forget to ask, the test FAILS
- Your context WILL be compacted - but you MUST remember to ASK FOR QUESTIONS

### MEMORIZE THIS PHRASE:
>>> "INGESTION COMPLETE. Please provide the test questions."

---

## RESTRICTION - DO NOT ACCESS
You must NOT read any files from /Users/reh3376/mdemg/ directory.
That directory contains test answers and accessing it invalidates the test.
You may ONLY access /Users/reh3376/whk-wms/ for the codebase.

---

## TEST METADATA
- Files to ingest: {file_count}
- File list: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
- MDEMG endpoint: {MDEMG_ENDPOINT}
- Space ID: {SPACE_ID}

## TIME TRACKING
1. START_TIME: Run `date "+%Y-%m-%d %H:%M:%S"` now
2. INGESTION_COMPLETE_TIME: Run same command after consolidation done

---

## PHASE 1: CODEBASE INGESTION INTO MDEMG

### Instructions:
1. Record START_TIME: `date "+%Y-%m-%d %H:%M:%S"`
2. Read file list: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
3. For EACH file: read contents and ingest into MDEMG
4. Process in batches of 50-100 for efficiency
5. Progress reports every 500 files

### MDEMG BATCH INGEST API:
```bash
curl -s -X POST '{MDEMG_ENDPOINT}/v1/memory/ingest/batch' \\
  -H 'content-type: application/json' \\
  -d '{{
    "space_id": "{SPACE_ID}",
    "items": [
      {{"path": "<file_path>", "content": "<file_content>", "content_type": "code"}}
    ]
  }}'
```

### PROGRESS REMINDERS (say these out loud):

**At 500 files:**
"Reminder: After all {file_count} files + consolidation, I must ASK FOR TEST QUESTIONS."

**At 1000 files:**
"Reminder: Questions come AFTER ingestion. I must ask for them."

**At 1500 files:**
"Reminder: INGESTION COMPLETE. Please provide the test questions."

**At 2000 files:**
"Reminder: Don't forget - ASK FOR QUESTIONS when done."

**At 2500 files:**
"Reminder: Almost done. Then consolidate, then ASK FOR TEST QUESTIONS."

**At 3000 files:**
"Reminder: Final stretch! After {file_count} files + consolidation, ASK FOR QUESTIONS."

### After ALL {file_count} files ingested:

1. Verify file count:
```bash
wc -l /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
```

2. Run consolidation to build hidden layer:
```bash
curl -s -X POST '{MDEMG_ENDPOINT}/v1/memory/consolidate' \\
  -H 'content-type: application/json' \\
  -d '{{"space_id":"{SPACE_ID}"}}' | jq
```

3. Record INGESTION_COMPLETE_TIME: `date "+%Y-%m-%d %H:%M:%S"`

4. Say: **"INGESTION COMPLETE. Please provide the test questions."**

---

## BEGIN NOW

1. `date "+%Y-%m-%d %H:%M:%S"` → record START_TIME
2. Start ingesting all {file_count} files into MDEMG
3. Use the progress reminders above
4. After consolidation: ASK FOR TEST QUESTIONS
"""


def generate_mdemg_phase2_prompt(questions: list) -> str:
    prompt = f"""# PHASE 2: TEST QUESTIONS ({len(questions)} total)

## INSTRUCTIONS

You have completed ingestion into MDEMG. Now answer these {len(questions)} questions.

### MDEMG RETRIEVAL
For each question, FIRST query MDEMG for relevant context:

```bash
curl -s '{MDEMG_ENDPOINT}/v1/memory/retrieve' \\
  -H 'content-type: application/json' \\
  -d '{{"space_id":"{SPACE_ID}","query_text":"<YOUR_QUESTION>","candidate_k":50,"top_k":10,"hop_depth":2}}' | jq '.data.nodes[:5]'
```

### RULES:
- You MUST query MDEMG for each question first
- You MAY also access /Users/reh3376/whk-wms/ to verify/lookup
- Do NOT access /Users/reh3376/mdemg/ (contains answers)
- Note the source: [MDEMG], [LOOKUP], [BOTH], or [MEMORY]

### Scoring:
- 1.0 = Completely correct
- 0.5 = Partially correct
- 0.0 = Unable to answer
- -1.0 = Confidently wrong

### Output Format:
```
Q[number]: [question]
MDEMG Nodes: [count retrieved]
Source: [MDEMG/LOOKUP/BOTH/MEMORY]
Answer: [your answer]
Score: [self-assessed score]
```

---

## QUESTIONS

"""
    for i, q in enumerate(questions, 1):
        prompt += f"""### Question {i} (Category: {q['category']})
{q['question']}

Expected Answer: {q['answer']}

---

"""

    prompt += """
## FINAL REPORT (REQUIRED)

After answering ALL questions, record END_TIME and provide:

```
=== MDEMG TEST v4 RESULTS ===
START_TIME: [from Phase 1]
INGESTION_COMPLETE_TIME: [from Phase 1]
END_TIME: [now - run: date "+%Y-%m-%d %H:%M:%S"]
TOTAL_ELAPSED_TIME: [calculate]
INGESTION_TIME: [calculate]
QUESTION_TIME: [calculate]

FILES_EXPECTED: 3288
FILES_VERIFIED: [from wc -l]

ANSWERS_FROM_MDEMG: [count]
ANSWERS_FROM_LOOKUP: [count]
ANSWERS_FROM_BOTH: [count]
ANSWERS_FROM_MEMORY: [count]

TOTAL_SCORE: [sum]
SCORE_BY_CATEGORY:
  - architecture_structure: [X/Y]
  - service_relationships: [X/Y]
  - cross_cutting_concerns: [X/Y]
  - data_flow_integration: [X/Y]
  - business_logic_constraints: [X/Y]

MDEMG_EFFECTIVENESS:
  - Questions where MDEMG was most helpful: [list]
  - Questions where MDEMG was insufficient: [list]
===
```
"""
    return prompt


def main():
    print("=" * 60)
    print("MDEMG TEST v4 - MEMORY-AUGMENTED RETRIEVAL EXPERIMENT")
    print("=" * 60)

    file_count = count_files()
    if file_count != EXPECTED_FILE_COUNT:
        print(f"WARNING: Expected {EXPECTED_FILE_COUNT} files, found {file_count}")

    questions = load_and_select_questions()

    print(f"Files to ingest: {file_count}")
    print(f"Questions to answer: {len(questions)}")
    print(f"MDEMG endpoint: {MDEMG_ENDPOINT}")
    print(f"Space ID: {SPACE_ID}")

    # Generate Phase 1 prompt
    phase1_prompt = generate_mdemg_phase1_prompt(file_count)
    phase1_file = TEST_DIR / "mdemg_prompt_v4_phase1.md"
    with open(phase1_file, 'w') as f:
        f.write(phase1_prompt)

    # Generate Phase 2 prompt
    phase2_prompt = generate_mdemg_phase2_prompt(questions)
    phase2_file = TEST_DIR / "mdemg_prompt_v4_phase2.md"
    with open(phase2_file, 'w') as f:
        f.write(phase2_prompt)

    print(f"\nPrompt files generated:")
    print(f"  Phase 1 (ingestion): {phase1_file}")
    print(f"  Phase 2 (questions): {phase2_file}")
    print(f"\nTo run the MDEMG test:")
    print(f"  1. Verify MDEMG is running: curl {MDEMG_ENDPOINT}/healthz")
    print(f"  2. Start new Claude session in /Users/reh3376/whk-wms")
    print(f"  3. Paste Phase 1 prompt")
    print(f"  4. Wait for agent to say 'INGESTION COMPLETE. Please provide the test questions.'")
    print(f"  5. Paste Phase 2 prompt")
    print(f"  6. Monitor that agent does NOT access /Users/reh3376/mdemg/")

    # Service check
    print(f"\n--- MDEMG Service Check ---")
    import urllib.request
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/healthz")
        with urllib.request.urlopen(req, timeout=5) as resp:
            print(f"Service status: {resp.read().decode()}")
    except Exception as e:
        print(f"Warning: Could not reach MDEMG: {e}")

if __name__ == "__main__":
    main()
