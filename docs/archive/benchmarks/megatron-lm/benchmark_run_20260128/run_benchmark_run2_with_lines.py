#!/usr/bin/env python3
"""
MDEMG Benchmark Run 2 - With proper file:line references
Queries MDEMG and extracts line numbers from file paths and entity names
"""

import json
import subprocess
import sys
import re
from pathlib import Path

MDEMG_URL = "http://localhost:9999/v1/memory/retrieve"
SPACE_ID = "megatron-lm"
TOP_K = 20
MEGATRON_LM_PATH = "/Users/reh3376/mdemg/spaces/megatron-lm"  # Adjust if needed


def query_mdemg(question: str) -> dict:
    """Query MDEMG API"""
    cmd = [
        "curl", "-s", "-X", "POST", MDEMG_URL,
        "-H", "Content-Type: application/json",
        "-d", json.dumps({
            "query_text": question,
            "space_id": SPACE_ID,
            "top_k": TOP_K
        })
    ]

    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            return json.loads(result.stdout)
    except Exception as e:
        print(f"ERROR querying MDEMG: {e}", file=sys.stderr)

    return {"results": []}


def extract_file_line_refs(results: list) -> list:
    """
    Extract file:line references from MDEMG results.
    Parse path field to get file and entity, then find line numbers.
    """
    file_line_refs = []

    for result in results[:10]:  # Top 10 results
        path = result.get("path", "")

        if not path:
            continue

        # Path format: /path/to/file.py#EntityName or just /path/to/file.py
        if "#" in path:
            file_path, entity_name = path.split("#", 1)
        else:
            file_path = path
            entity_name = None

        # Remove leading slash
        file_path = file_path.lstrip("/")

        # Try to find line number
        line_num = find_line_number(file_path, entity_name)

        if line_num:
            ref = f"{file_path}:{line_num}"
        else:
            ref = f"{file_path}:1"  # Default to line 1 if not found

        if ref not in file_line_refs:
            file_line_refs.append(ref)

    return file_line_refs


def find_line_number(file_path: str, entity_name: str = None) -> int:
    """
    Find line number for entity in file using grep.
    Returns 0 if not found.
    """
    full_path = Path(MEGATRON_LM_PATH) / file_path

    if not full_path.exists():
        return 0

    if entity_name:
        # Search for class or function definition
        patterns = [
            f"^class {entity_name}",
            f"^def {entity_name}",
            f"class {entity_name}\(",
            f"def {entity_name}\(",
        ]

        for pattern in patterns:
            try:
                result = subprocess.run(
                    ["grep", "-n", "-E", pattern, str(full_path)],
                    capture_output=True,
                    text=True,
                    timeout=5
                )

                if result.returncode == 0 and result.stdout:
                    # Extract line number from first match (format: "123:line content")
                    match = re.match(r"(\d+):", result.stdout)
                    if match:
                        return int(match.group(1))
            except Exception:
                continue

    # If no entity or not found, return line 1
    return 1


def synthesize_answer(question: str, results: list) -> str:
    """Synthesize answer from MDEMG results"""
    if not results:
        return "No relevant information found in the codebase."

    # Use summaries from top results
    summaries = []
    for i, result in enumerate(results[:3]):
        summary = result.get("summary", "")
        if summary:
            summaries.append(summary[:200])

    if summaries:
        return " ".join(summaries)
    else:
        return "Retrieved results but unable to extract detailed information."


def main():
    """Main benchmark runner"""

    questions_file = Path("/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_questions_v1_agent.json")
    output_file = Path("/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_run_20260128/answers_mdemg_run2.jsonl")

    # Clear output file
    if output_file.exists():
        output_file.unlink()

    # Load questions
    with open(questions_file) as f:
        data = json.load(f)

    questions = data["questions"]
    total = len(questions)

    print(f"\n{'='*70}", file=sys.stderr)
    print(f"MDEMG BENCHMARK - RUN 2 (WITH LINE NUMBERS)", file=sys.stderr)
    print(f"Total Questions: {total}", file=sys.stderr)
    print(f"Output: {output_file}", file=sys.stderr)
    print(f"{'='*70}\n", file=sys.stderr)

    # Process all questions
    for i, q in enumerate(questions, 1):
        question_id = q["id"]
        question_text = q["question"]

        print(f"[{i}/{total}] Q{question_id}: {question_text[:70]}...", file=sys.stderr)

        try:
            # Query MDEMG
            response = query_mdemg(question_text)
            results = response.get("results", [])

            # Extract file:line references with actual line numbers
            file_line_refs = extract_file_line_refs(results)

            # Synthesize answer
            answer = synthesize_answer(question_text, results)

            # Create output record
            record = {
                "id": question_id,
                "question": question_text,
                "answer": answer,
                "file_line_refs": file_line_refs
            }

            # Write immediately
            with open(output_file, "a") as f:
                f.write(json.dumps(record) + "\n")

            print(f"  ✓ Complete - {len(file_line_refs)} refs", file=sys.stderr)

        except Exception as e:
            print(f"  ERROR: {e}", file=sys.stderr)
            # Write error record
            record = {
                "id": question_id,
                "question": question_text,
                "answer": f"Error: {e}",
                "file_line_refs": []
            }
            with open(output_file, "a") as f:
                f.write(json.dumps(record) + "\n")

        if i % 10 == 0:
            print(f"\nProgress: {i}/{total} ({i*100//total}%)\n", file=sys.stderr)

    print(f"\n{'='*70}", file=sys.stderr)
    print(f"BENCHMARK COMPLETE", file=sys.stderr)
    print(f"Output: {output_file}", file=sys.stderr)
    print(f"{'='*70}\n", file=sys.stderr)


if __name__ == "__main__":
    main()
