#!/usr/bin/env python3
"""
MDEMG Baseline Test Runner v3

Protocol:
1. PHASE 1: Force the baseline agent to read EVERY file in whk-wms-file-list.txt
2. PHASE 2: Only AFTER all files are read, provide the 100 questions
3. Agent must answer from memory/compressed context only - NO file reading in Phase 2

This tests context retention after massive context consumption and compression.
"""

import json
import subprocess
import sys
import time
from pathlib import Path
from datetime import datetime

# Configuration
TEST_DIR = Path(__file__).parent
FILE_LIST = TEST_DIR / "whk-wms-file-list.txt"
QUESTIONS_FILE = TEST_DIR / "test_questions_100.json"
OUTPUT_FILE = TEST_DIR / f"baseline-test-v3-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"

def count_files():
    """Count total files to be read."""
    with open(FILE_LIST) as f:
        return len([l for l in f.readlines() if l.strip()])

def load_questions():
    """Load the 100 test questions."""
    with open(QUESTIONS_FILE) as f:
        data = json.load(f)
    return data['questions']

def generate_baseline_prompt(file_count: int, questions: list) -> str:
    """Generate the baseline agent prompt."""
    # First part: File reading phase
    prompt = f"""# MDEMG Baseline Test - Context Retention Experiment

## CRITICAL INSTRUCTIONS - READ CAREFULLY

You are the BASELINE test agent for the MDEMG context retention experiment.
Your task has TWO PHASES that MUST be executed IN ORDER.

---

## PHASE 1: CODEBASE INGESTION (MANDATORY)

You MUST read EVERY file in the whk-wms codebase BEFORE seeing any questions.
There are {file_count} files in /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt

### Instructions for Phase 1:
1. Read the file list from: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
2. For EACH file in the list:
   - Use the Read tool to read the file
   - Note key information: exports, functions, classes, business logic
   - DO NOT skip any files
3. You will need to read files in parallel batches of 10-20 to be efficient
4. Track your progress: report every 100 files read
5. This WILL cause context compression - that is the point

### Phase 1 Completion Criteria:
- You have read ALL {file_count} files
- You have noted key patterns, modules, and relationships
- Only then proceed to Phase 2

---

## PHASE 2: ANSWER QUESTIONS (AFTER PHASE 1 COMPLETE)

After you have confirmed reading ALL files, you will answer 100 questions.

### CRITICAL CONSTRAINT FOR PHASE 2:
- You are NOT ALLOWED to read any additional files
- You are NOT ALLOWED to use Glob or Grep tools
- You must answer ONLY from memory/compressed context
- If you cannot remember, say "Unable to recall from ingested context"

### Scoring:
- 1.0 = Completely correct
- 0.5 = Partially correct (right concept, wrong details)
- 0.0 = Unable to answer
- -1.0 = Confidently wrong

### Output Format:
For each question, output:
```
Q[number]: [question]
Answer: [your answer]
Score: [self-assessed score]
Reasoning: [why you scored it this way]
```

---

## BEGIN PHASE 1 NOW

Start by reading the file list at /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
Then read EVERY file listed before proceeding to questions.

Remember: The purpose is to test context retention through compression.
Reading all files FIRST is mandatory - this is not optional.

"""
    return prompt

def generate_questions_prompt(questions: list) -> str:
    """Generate the questions-only prompt for Phase 2."""
    prompt = """# PHASE 2: QUESTIONS (Read-Only Phase)

You have now read all files. Below are 100 questions to answer.

REMINDER:
- Do NOT read any files
- Do NOT use Glob or Grep
- Answer from memory ONLY

---

## Questions

"""
    for i, q in enumerate(questions, 1):
        prompt += f"""### Question {i} (Category: {q['category']})
{q['question']}

Expected Answer (for scoring):
{q['answer']}

---

"""
    return prompt

def main():
    print("=" * 60)
    print("MDEMG BASELINE TEST v3 - CONTEXT RETENTION EXPERIMENT")
    print("=" * 60)

    file_count = count_files()
    questions = load_questions()

    print(f"Files to read: {file_count}")
    print(f"Questions to answer: {len(questions)}")
    print(f"Output file: {OUTPUT_FILE}")

    # Generate the initial prompt
    initial_prompt = generate_baseline_prompt(file_count, questions)

    # Save prompt for manual execution
    prompt_file = TEST_DIR / "baseline_prompt_v3.md"
    with open(prompt_file, 'w') as f:
        f.write(initial_prompt)

    questions_prompt = generate_questions_prompt(questions)
    questions_prompt_file = TEST_DIR / "baseline_questions_v3.md"
    with open(questions_prompt_file, 'w') as f:
        f.write(questions_prompt)

    print(f"\nPrompt files generated:")
    print(f"  Phase 1: {prompt_file}")
    print(f"  Phase 2: {questions_prompt_file}")
    print(f"\nTo run the baseline test:")
    print(f"  1. Start a new Claude Code session")
    print(f"  2. Paste the Phase 1 prompt")
    print(f"  3. Wait for all files to be read")
    print(f"  4. Then paste the Phase 2 prompt with questions")
    print(f"  5. Collect answers and save to: {OUTPUT_FILE}")

if __name__ == "__main__":
    main()
