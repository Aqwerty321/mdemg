#!/usr/bin/env python3
"""
MDEMG Benchmark Run 2 (Warm Start) for Megatron-LM.
Processes 142 questions, queries MDEMG retrieve endpoint, and outputs answers in JSONL format.

Run: 2 (Warm start - caches are warmed from Run 1)
Questions: /Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_questions_v1_agent.json
Output: /Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_run_20260128/answers_mdemg_run2.jsonl
"""

import json
import requests
import sys
import time
from pathlib import Path
from datetime import datetime

MDEMG_URL = "http://localhost:9999/v1/memory/retrieve"
SPACE_ID = "megatron-lm"
TOP_K = 20
QUESTIONS_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_questions_v1_agent.json"
OUTPUT_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_run_20260128/answers_mdemg_run2.jsonl"


def query_mdemg(question: str, top_k: int = TOP_K) -> dict:
    """
    Query MDEMG retrieve endpoint.

    Args:
        question: The question text to retrieve context for
        top_k: Number of top results to retrieve

    Returns:
        Dictionary with results or empty dict if query fails
    """
    payload = {
        "query_text": question,
        "top_k": top_k,
        "space_id": SPACE_ID
    }

    try:
        response = requests.post(MDEMG_URL, json=payload, timeout=30)
        if response.status_code == 200:
            return response.json()
    except requests.exceptions.RequestException as e:
        print(f"MDEMG query error: {e}", file=sys.stderr)

    return {"results": [], "error": "Failed to query MDEMG"}


def extract_file_refs(mdemg_results: list) -> list:
    """
    Extract file path references from MDEMG results.

    Args:
        mdemg_results: List of result objects from MDEMG

    Returns:
        List of file paths with line numbers if available
    """
    refs = []
    for result in mdemg_results:
        # MDEMG results should have 'path' and optional 'line_number'
        path = result.get("path", "")
        if path:
            # Format as path:line_number if line number exists
            line_num = result.get("line_number")
            if line_num:
                refs.append(f"{path}:{line_num}")
            else:
                refs.append(path)

    return refs


def formulate_answer(question: str, mdemg_results: list) -> str:
    """
    Formulate an answer based on MDEMG retrieved context.

    Args:
        question: The question text
        mdemg_results: List of retrieved results from MDEMG

    Returns:
        A brief answer based on the retrieved context
    """
    if not mdemg_results:
        return "No relevant context found in MDEMG."

    # Build answer from top 3 results
    answer_parts = []

    for i, result in enumerate(mdemg_results[:3], 1):
        path = result.get("path", "")
        summary = result.get("summary", "")
        content = result.get("content", "")

        # Use summary if available, otherwise use first part of content
        context_text = summary if summary else (content[:200] + "..." if content else "")

        if context_text:
            answer_parts.append(f"[From {path}] {context_text}")

    if answer_parts:
        return " ".join(answer_parts)
    else:
        return "Retrieved context but unable to formulate complete answer."


def main():
    """Main benchmark runner."""
    print(f"Starting MDEMG Benchmark Run 2 (Warm Start) at {datetime.now().isoformat()}")
    print(f"Questions file: {QUESTIONS_FILE}")
    print(f"Output file: {OUTPUT_FILE}")
    print(f"MDEMG endpoint: {MDEMG_URL}")
    print(f"Space ID: {SPACE_ID}")
    print(f"Top K: {TOP_K}")
    print()

    # Load questions
    try:
        with open(QUESTIONS_FILE, 'r') as f:
            data = json.load(f)
        questions = data["questions"]
    except Exception as e:
        print(f"Error loading questions: {e}", file=sys.stderr)
        sys.exit(1)

    print(f"Loaded {len(questions)} questions")
    print()

    # Track statistics
    total_questions = len(questions)
    successful = 0
    failed = 0
    start_time = time.time()

    # Process each question and write to output file
    try:
        with open(OUTPUT_FILE, 'w') as outf:
            for idx, question_obj in enumerate(questions, 1):
                qid = question_obj["id"]
                question_text = question_obj["question"]
                category = question_obj.get("category", "unknown")
                difficulty = question_obj.get("difficulty", "unknown")

                # Show progress
                print(f"[{idx}/{total_questions}] Q{qid} ({category}, {difficulty}): {question_text[:60]}...", end=" ", flush=True)

                try:
                    # Query MDEMG
                    mdemg_response = query_mdemg(question_text, top_k=TOP_K)
                    mdemg_results = mdemg_response.get("results", [])

                    # Extract file references
                    file_refs = extract_file_refs(mdemg_results)

                    # Formulate answer based on retrieved context
                    answer = formulate_answer(question_text, mdemg_results)

                    # Create output record
                    record = {
                        "id": qid,
                        "question": question_text,
                        "category": category,
                        "difficulty": difficulty,
                        "answer": answer,
                        "file_line_refs": file_refs,
                        "num_results": len(mdemg_results),
                        "timestamp": datetime.now().isoformat()
                    }

                    # Write as JSONL
                    outf.write(json.dumps(record) + "\n")

                    successful += 1
                    print("OK")

                except Exception as e:
                    print(f"ERROR: {e}")
                    failed += 1

                    # Still write a record with error
                    record = {
                        "id": qid,
                        "question": question_text,
                        "category": category,
                        "difficulty": difficulty,
                        "answer": f"Error processing question: {str(e)}",
                        "file_line_refs": [],
                        "num_results": 0,
                        "error": str(e),
                        "timestamp": datetime.now().isoformat()
                    }
                    outf.write(json.dumps(record) + "\n")

    except Exception as e:
        print(f"Fatal error writing to output file: {e}", file=sys.stderr)
        sys.exit(1)

    # Print summary
    elapsed_time = time.time() - start_time
    print()
    print("=" * 70)
    print("BENCHMARK RUN 2 SUMMARY")
    print("=" * 70)
    print(f"Total questions: {total_questions}")
    print(f"Successful: {successful}")
    print(f"Failed: {failed}")
    print(f"Success rate: {100.0 * successful / total_questions:.1f}%")
    print(f"Elapsed time: {elapsed_time:.1f} seconds")
    print(f"Average time per question: {elapsed_time / total_questions:.2f} seconds")
    print(f"Output file: {OUTPUT_FILE}")
    print()

    # Verify output file
    output_path = Path(OUTPUT_FILE)
    if output_path.exists():
        with open(OUTPUT_FILE, 'r') as f:
            lines = f.readlines()
            print(f"Output file contains {len(lines)} lines")
            if lines:
                # Validate first and last lines are valid JSON
                try:
                    json.loads(lines[0])
                    json.loads(lines[-1])
                    print("Output file format: Valid JSONL")
                except:
                    print("Output file format: Invalid JSONL")

    print("=" * 70)
    print(f"Completed at {datetime.now().isoformat()}")


if __name__ == "__main__":
    main()
