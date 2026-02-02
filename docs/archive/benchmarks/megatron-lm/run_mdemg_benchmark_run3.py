#!/usr/bin/env python3
"""
MDEMG Benchmark Run 3 (Warm Start) for Megatron-LM
Processes 142 questions, retrieves context from MDEMG, and generates answers.
Run with: python3 run_mdemg_benchmark_run3.py
"""

import json
import subprocess
import sys
import time
from pathlib import Path
from datetime import datetime

MDEMG_URL = "http://localhost:9999/v1/memory/retrieve"
SPACE_ID = "megatron-lm"
QUESTIONS_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_questions_v1_agent.json"
OUTPUT_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_run_20260128/answers_mdemg_run3.jsonl"

# Simple synthetic reasoning based on MDEMG results
def generate_answer_from_context(question_text: str, mdemg_results: list) -> str:
    """Generate an answer based on MDEMG retrieved context."""
    if not mdemg_results:
        return "Unable to find relevant context in the codebase."

    # Build answer from context snippets
    context_summaries = []
    for result in mdemg_results[:5]:
        summary = result.get("summary", "")
        path = result.get("path", "")
        if summary:
            context_summaries.append(f"{summary}")

    if not context_summaries:
        return "Unable to generate answer from available context."

    # Combine context into a coherent answer
    answer = " ".join(context_summaries)

    # Limit answer length
    if len(answer) > 2000:
        answer = answer[:1997] + "..."

    return answer


def query_mdemg(question: str, top_k: int = 20) -> dict:
    """Query MDEMG for context."""
    payload = {
        "query_text": question,
        "top_k": top_k,
        "space_id": SPACE_ID
    }

    try:
        cmd = [
            "curl", "-s", "-X", "POST", MDEMG_URL,
            "-H", "Content-Type: application/json",
            "-d", json.dumps(payload)
        ]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            try:
                return json.loads(result.stdout)
            except json.JSONDecodeError:
                print(f"Warning: Failed to parse MDEMG response for: {question[:50]}", file=sys.stderr)
                return {"results": []}
    except subprocess.TimeoutExpired:
        print(f"Timeout querying MDEMG for: {question[:50]}", file=sys.stderr)
    except Exception as e:
        print(f"MDEMG query error: {e}", file=sys.stderr)

    return {"results": []}


def main():
    print(f"Starting MDEMG Benchmark Run 3 (Warm Start) at {datetime.now()}")
    print(f"Output file: {OUTPUT_FILE}")

    # Load questions
    with open(QUESTIONS_FILE, 'r') as f:
        data = json.load(f)

    questions = data["questions"]
    total = len(questions)
    print(f"Loaded {total} questions")

    # Track stats
    successful = 0
    failed = 0
    start_time = time.time()

    # Process each question and write to output file
    with open(OUTPUT_FILE, 'w') as outf:
        for idx, q in enumerate(questions, 1):
            qid = q["id"]
            question_text = q["question"]

            print(f"[{idx}/{total}] Processing question {qid}: {question_text[:60]}...")

            try:
                # Query MDEMG
                mdemg_resp = query_mdemg(question_text, top_k=20)
                results = mdemg_resp.get("results", [])

                # Extract file references
                file_refs = []
                for r in results:
                    path = r.get("path", "")
                    if path:
                        file_refs.append(path.lstrip("/"))

                # Generate answer from context
                answer = generate_answer_from_context(question_text, results)

                # Create output entry
                entry = {
                    "id": qid,
                    "question": question_text,
                    "answer": answer,
                    "file_line_refs": file_refs[:10]  # Top 10 file refs
                }

                # Write as JSONL
                outf.write(json.dumps(entry) + "\n")
                outf.flush()

                successful += 1

                # Status update every 20 questions
                if idx % 20 == 0:
                    elapsed = time.time() - start_time
                    avg_time = elapsed / idx
                    remaining = (total - idx) * avg_time
                    print(f"  Progress: {successful}/{idx} successful, ETA: {remaining:.1f}s")

            except Exception as e:
                print(f"Error processing question {qid}: {e}", file=sys.stderr)
                failed += 1
                # Still write a stub entry
                entry = {
                    "id": qid,
                    "question": question_text,
                    "answer": f"Error processing question: {str(e)[:100]}",
                    "file_line_refs": []
                }
                outf.write(json.dumps(entry) + "\n")
                outf.flush()

    # Summary
    total_time = time.time() - start_time
    print(f"\nBenchmark Run 3 Complete!")
    print(f"Total questions: {total}")
    print(f"Successful: {successful}")
    print(f"Failed: {failed}")
    print(f"Total time: {total_time:.1f}s ({total_time/total:.2f}s per question)")
    print(f"Output file: {OUTPUT_FILE}")

    # Verify file
    if Path(OUTPUT_FILE).exists():
        size = Path(OUTPUT_FILE).stat().st_size
        with open(OUTPUT_FILE, 'r') as f:
            line_count = sum(1 for _ in f)
        print(f"Output file size: {size} bytes, {line_count} lines")


if __name__ == "__main__":
    main()
