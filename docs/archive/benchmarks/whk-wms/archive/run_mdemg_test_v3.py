#!/usr/bin/env python3
"""
MDEMG Test Runner v3

Protocol:
1. NO file reading from whk-wms codebase
2. For each question, query MDEMG API for relevant context
3. Answer using ONLY retrieved context from MDEMG
4. Hidden layer concepts should provide cross-module understanding

This tests memory-augmented retrieval vs raw context consumption.
"""

import json
import subprocess
import sys
import time
from pathlib import Path
from datetime import datetime

# Configuration
TEST_DIR = Path(__file__).parent
QUESTIONS_FILE = TEST_DIR / "test_questions_100.json"
OUTPUT_FILE = TEST_DIR / f"mdemg-test-v3-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
MDEMG_ENDPOINT = "http://localhost:8082"
SPACE_ID = "whk-wms"

def load_questions():
    """Load the 100 test questions."""
    with open(QUESTIONS_FILE) as f:
        data = json.load(f)
    return data['questions']

def generate_mdemg_prompt(questions: list) -> str:
    """Generate the MDEMG agent prompt."""
    prompt = f"""# MDEMG Test - Memory-Augmented Retrieval Experiment

## CRITICAL INSTRUCTIONS - READ CAREFULLY

You are the MDEMG test agent for the context retention experiment.
Your task is to answer questions using ONLY the MDEMG memory API.

---

## CONSTRAINTS (MANDATORY)

1. You MUST NOT read any files from /Users/reh3376/whk-wms/
2. You MUST NOT use Glob or Grep to search the filesystem
3. You MUST NOT access any files except for writing the report

For each question, you will:
1. Query MDEMG for relevant context
2. Analyze the retrieved nodes and hidden layer concepts
3. Answer based ONLY on what MDEMG returns

---

## HOW TO QUERY MDEMG

For each question, run this curl command to get context:

```bash
curl -s '{MDEMG_ENDPOINT}/v1/memory/retrieve' \\
  -H 'content-type: application/json' \\
  -d '{{"space_id":"{SPACE_ID}","query_text":"<QUESTION>","candidate_k":50,"top_k":10,"hop_depth":2}}' | jq
```

The response includes:
- `nodes`: Relevant memory nodes with paths, summaries, and similarity scores
- `hidden_nodes`: Cross-cutting concept clusters from hidden layer
- `edges`: Relationships between nodes

---

## SCORING

- 1.0 = Completely correct
- 0.5 = Partially correct (right concept, wrong details)
- 0.0 = Unable to answer (context not sufficient)
- -1.0 = Confidently wrong

---

## OUTPUT FORMAT

For each question, output:

```
Q[number]: [question]
MDEMG Query: [the query you used]
Retrieved Context:
  - [node1 path]: [summary] (similarity: X.XX)
  - [node2 path]: [summary] (similarity: X.XX)
  [hidden layer concepts if relevant]
Answer: [your answer based on retrieved context]
Score: [self-assessed score]
Reasoning: [why you scored it this way, what context was helpful/missing]
```

---

## QUESTIONS ({len(questions)} total)

Answer ALL questions in order. Track:
- Total tokens used for retrieval
- Which questions had insufficient context
- Which hidden layer concepts were most useful

"""

    for i, q in enumerate(questions, 1):
        prompt += f"""
### Question {i} (Category: {q['category']})
{q['question']}

Expected Answer (for scoring):
{q['answer']}

---
"""

    prompt += """

## FINAL REPORT

After answering all questions, provide:
1. Total score: X/100
2. Breakdown by category
3. Questions where MDEMG context was insufficient
4. Hidden layer concepts that provided useful cross-module insights
5. Recommendations for improving MDEMG retrieval
"""

    return prompt

def main():
    print("=" * 60)
    print("MDEMG TEST v3 - MEMORY-AUGMENTED RETRIEVAL EXPERIMENT")
    print("=" * 60)

    questions = load_questions()

    print(f"Questions to answer: {len(questions)}")
    print(f"MDEMG endpoint: {MDEMG_ENDPOINT}")
    print(f"Space ID: {SPACE_ID}")
    print(f"Output file: {OUTPUT_FILE}")

    # Generate the prompt
    prompt = generate_mdemg_prompt(questions)

    # Save prompt for manual execution
    prompt_file = TEST_DIR / "mdemg_prompt_v3.md"
    with open(prompt_file, 'w') as f:
        f.write(prompt)

    print(f"\nPrompt file generated: {prompt_file}")
    print(f"\nTo run the MDEMG test:")
    print(f"  1. Verify MDEMG service is running with whk-wms data")
    print(f"  2. Verify consolidation has been run for hidden layer")
    print(f"  3. Start a new Claude Code session")
    print(f"  4. Paste the prompt")
    print(f"  5. Collect answers and save to: {OUTPUT_FILE}")

    # Quick connectivity check
    print(f"\n--- MDEMG Service Check ---")
    import urllib.request
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/healthz")
        with urllib.request.urlopen(req, timeout=5) as resp:
            print(f"Service status: {resp.read().decode()}")
    except Exception as e:
        print(f"Warning: Could not reach MDEMG at {MDEMG_ENDPOINT}: {e}")

if __name__ == "__main__":
    main()
