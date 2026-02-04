#!/usr/bin/env python3
"""
Benchmark question processor for Megatron-LM using MDEMG.
Processes questions, queries MDEMG, and outputs answers in JSONL format.
"""

import json
import subprocess
import sys
from pathlib import Path

MDEMG_URL = "http://localhost:9999/v1/memory/retrieve"
SPACE_ID = "megatron-lm"
MEGATRON_PATH = "/Users/reh3376/repos/Megatron-LM"
OUTPUT_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_run_20260128/answers_mdemg_run1.jsonl"


def query_mdemg(question: str, top_k: int = 10) -> dict:
    """Query MDEMG for context."""
    payload = {
        "query_text": question,
        "space_id": SPACE_ID,
        "top_k": top_k
    }

    try:
        cmd = [
            "curl", "-s", "-X", "POST", MDEMG_URL,
            "-H", "Content-Type: application/json",
            "-d", json.dumps(payload)
        ]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            return json.loads(result.stdout)
    except Exception as e:
        print(f"MDEMG query error: {e}", file=sys.stderr)

    return {"results": []}


def main():
    # Load questions
    questions_file = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_questions_v1_agent.json"
    with open(questions_file, 'r') as f:
        data = json.load(f)

    questions = data["questions"]
    print(f"Loaded {len(questions)} questions")

    # Process each question
    with open(OUTPUT_FILE, 'w') as outf:
        for q in questions:
            qid = q["id"]
            question_text = q["question"]

            print(f"Processing question {qid}/{len(questions)}: {question_text[:50]}...")

            # Query MDEMG
            mdemg_resp = query_mdemg(question_text)

            # Extract file paths from results
            file_refs = []
            context_parts = []
            for r in mdemg_resp.get("results", [])[:5]:
                path = r.get("path", "")
                summary = r.get("summary", "")
                if path:
                    file_refs.append(path.lstrip("/"))
                    context_parts.append(f"{path}: {summary}")

            # Format answer stub (agent will fill in details)
            answer = {
                "id": qid,
                "question": question_text,
                "answer": "",  # To be filled by agent
                "file_line_refs": file_refs,
                "mdemg_context_used": len(mdemg_resp.get("results", [])) > 0,
                "mdemg_context": context_parts[:3]  # Store top 3 for reference
            }

            outf.write(json.dumps(answer) + "\n")

    print(f"Wrote {len(questions)} entries to {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
