#!/usr/bin/env python3
"""
MDEMG Baseline Test Runner v4

Protocol:
1. PHASE 1: Agent must grep/read EVERY file in whk-wms (3288 files) - VERIFIED externally
2. PHASE 2: After ingestion, answer questions WITH repo access allowed
3. Track total elapsed time from start to completion
4. Independently verify file count (don't trust model's memory due to compaction)

Key metric: Does the model retain useful context after compaction events?
"""

import json
import random
import subprocess
import sys
import time
from pathlib import Path
from datetime import datetime

# Configuration
TEST_DIR = Path(__file__).parent
FILE_LIST = TEST_DIR / "whk-wms-file-list.txt"
QUESTIONS_SOURCE = TEST_DIR / "whk-wms-questions-final.json"
SELECTED_QUESTIONS_FILE = TEST_DIR / "test_questions_v4_selected.json"
OUTPUT_FILE = TEST_DIR / f"baseline-test-v4-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
EXPECTED_FILE_COUNT = 3288
NUM_QUESTIONS = 100
RANDOM_SEED = 42  # Fixed seed for reproducibility

def count_files():
    """Count total files to be read."""
    with open(FILE_LIST) as f:
        return len([l for l in f.readlines() if l.strip()])

def load_and_select_questions():
    """Load questions from source and randomly select 100."""
    # Check if we already have selected questions (for consistency between baseline and MDEMG)
    if SELECTED_QUESTIONS_FILE.exists():
        print(f"Using existing selected questions from {SELECTED_QUESTIONS_FILE}")
        with open(SELECTED_QUESTIONS_FILE) as f:
            data = json.load(f)
        return data['questions']

    # Load all questions from source
    with open(QUESTIONS_SOURCE) as f:
        data = json.load(f)

    all_questions = data['questions']
    print(f"Loaded {len(all_questions)} questions from {QUESTIONS_SOURCE}")

    # Randomly select 100 questions with fixed seed for reproducibility
    random.seed(RANDOM_SEED)
    selected = random.sample(all_questions, min(NUM_QUESTIONS, len(all_questions)))

    # Save selected questions so both tests use the same set
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

def generate_baseline_phase1_prompt(file_count: int) -> str:
    """Generate Phase 1 prompt - ingestion only, no questions."""

    prompt = f"""# MDEMG Baseline Test v4 - Context Retention Experiment

---
## !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
## CRITICAL - MUST SURVIVE AUTO-COMPACT - READ FIRST
## !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!

### YOUR MISSION (MEMORIZE THIS - IT MUST SURVIVE COMPACTION):

**STEP 1:** Ingest all {file_count} files from whk-wms
**STEP 2:** Verify count with `wc -l`
**STEP 3:** Say EXACTLY: "INGESTION COMPLETE. Please provide the test questions."

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
- Verification: `wc -l /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt`

## TIME TRACKING
1. START_TIME: Run `date "+%Y-%m-%d %H:%M:%S"` now
2. INGESTION_COMPLETE_TIME: Run same command after all files done

---

## PHASE 1: CODEBASE INGESTION

### Instructions:
1. Record START_TIME: `date "+%Y-%m-%d %H:%M:%S"`
2. Read file list: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
3. For EACH file: read/grep its contents
4. Process in batches of 20-50 for efficiency
5. Progress reports every 500 files

### PROGRESS REMINDERS (say these out loud):

**At 500 files:**
"Reminder: After all {file_count} files, I must ASK FOR TEST QUESTIONS."

**At 1000 files:**
"Reminder: Questions come AFTER ingestion. I must ask for them."

**At 1500 files:**
"Reminder: INGESTION COMPLETE. Please provide the test questions."

**At 2000 files:**
"Reminder: Don't forget - ASK FOR QUESTIONS when done."

**At 2500 files:**
"Reminder: Almost done. Then I say: INGESTION COMPLETE. Please provide the test questions."

**At 3000 files:**
"Reminder: Final stretch! After {file_count} files, ASK FOR TEST QUESTIONS."

### After ALL {file_count} files processed:
1. Run: `wc -l /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt` (must show {file_count})
2. Run: `date "+%Y-%m-%d %H:%M:%S"` for INGESTION_COMPLETE_TIME
3. Say: **"INGESTION COMPLETE. Please provide the test questions."**

---

## IMPORTANT NOTES:
- Do NOT skip files
- Context WILL compress - this tests what you retain
- Even after compression: REMEMBER TO ASK FOR QUESTIONS

---

## BEGIN NOW

1. `date "+%Y-%m-%d %H:%M:%S"` → record START_TIME
2. Start reading all {file_count} files
3. Use the progress reminders above
4. When done: ASK FOR TEST QUESTIONS
"""

    return prompt


def generate_baseline_phase2_prompt(questions: list) -> str:
    """Generate Phase 2 prompt - questions only."""

    prompt = f"""# PHASE 2: TEST QUESTIONS ({len(questions)} total)

## INSTRUCTIONS

You have completed ingestion. Now answer these {len(questions)} questions.

### RULES:
- You MAY access /Users/reh3376/whk-wms/ to verify/lookup information
- Do NOT access /Users/reh3376/mdemg/ (contains test answers - invalidates test)
- You SHOULD first attempt to answer from your ingested knowledge
- Note whether each answer came from: [MEMORY], [LOOKUP], or [BOTH]

### Scoring:
- 1.0 = Completely correct
- 0.5 = Partially correct (right concept, wrong details)
- 0.0 = Unable to answer
- -1.0 = Confidently wrong

### Output Format:
```
Q[number]: [question]
Source: [MEMORY/LOOKUP/BOTH]
Answer: [your answer]
Score: [self-assessed score]
Reasoning: [brief explanation]
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
=== BASELINE TEST v4 RESULTS ===
START_TIME: [from Phase 1]
INGESTION_COMPLETE_TIME: [from Phase 1]
END_TIME: [now - run: date "+%Y-%m-%d %H:%M:%S"]
TOTAL_ELAPSED_TIME: [calculate]
INGESTION_TIME: [calculate]
QUESTION_TIME: [calculate]

FILES_EXPECTED: 3288
FILES_VERIFIED: [output of wc -l command from Phase 1]

ANSWERS_FROM_MEMORY: [count]
ANSWERS_FROM_LOOKUP: [count]
ANSWERS_FROM_BOTH: [count]

TOTAL_SCORE: [sum of scores]
SCORE_BY_CATEGORY:
  - architecture_structure: [X/Y]
  - service_relationships: [X/Y]
  - cross_cutting_concerns: [X/Y]
  - data_flow_integration: [X/Y]
  - business_logic_constraints: [X/Y]

CONTEXT_RETENTION_NOTES:
  - [what did you remember vs forget?]
  - [which categories were harder from memory?]
===
```
"""

    return prompt

def main():
    print("=" * 60)
    print("MDEMG BASELINE TEST v4 - CONTEXT RETENTION EXPERIMENT")
    print("=" * 60)

    file_count = count_files()
    if file_count != EXPECTED_FILE_COUNT:
        print(f"WARNING: Expected {EXPECTED_FILE_COUNT} files, found {file_count}")

    questions = load_and_select_questions()

    print(f"Files to ingest: {file_count}")
    print(f"Questions to answer: {len(questions)}")
    print(f"Output file: {OUTPUT_FILE}")

    # Generate Phase 1 prompt (ingestion only)
    phase1_prompt = generate_baseline_phase1_prompt(file_count)
    phase1_file = TEST_DIR / "baseline_prompt_v4_phase1.md"
    with open(phase1_file, 'w') as f:
        f.write(phase1_prompt)

    # Generate Phase 2 prompt (questions only)
    phase2_prompt = generate_baseline_phase2_prompt(questions)
    phase2_file = TEST_DIR / "baseline_prompt_v4_phase2.md"
    with open(phase2_file, 'w') as f:
        f.write(phase2_prompt)

    print(f"\nPrompt files generated:")
    print(f"  Phase 1 (ingestion): {phase1_file}")
    print(f"  Phase 2 (questions): {phase2_file}")
    print(f"\nTo run the baseline test:")
    print(f"  1. Start a new Claude Code session in /Users/reh3376/whk-wms")
    print(f"  2. Paste Phase 1 prompt from {phase1_file}")
    print(f"  3. Wait for agent to complete ingestion of {file_count} files")
    print(f"  4. Agent will ask: 'INGESTION COMPLETE. Please provide the test questions.'")
    print(f"  5. Then paste Phase 2 prompt from {phase2_file}")
    print(f"  6. Let agent answer all questions")
    print(f"  7. Save final report to: {OUTPUT_FILE}")
    print(f"\nKey metrics to capture:")
    print(f"  - Total elapsed time (START_TIME to END_TIME)")
    print(f"  - Verified file count (from wc -l, must be {file_count})")
    print(f"  - Memory vs Lookup answer ratio")

if __name__ == "__main__":
    main()
