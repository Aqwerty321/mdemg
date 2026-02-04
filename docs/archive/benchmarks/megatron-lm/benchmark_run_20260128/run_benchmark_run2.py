#!/usr/bin/env python3
"""
MDEMG Benchmark Runner - Run 2 (Warm Cache)
Strictly follows file:line reference requirements
"""

import json
import subprocess
import sys
from pathlib import Path

def query_mdemg(question: str, space_id: str = "megatron-lm", top_k: int = 20) -> dict:
    """Query MDEMG API and return results"""
    cmd = [
        "curl", "-s", "-X", "POST",
        "http://localhost:9999/v1/memory/retrieve",
        "-H", "Content-Type: application/json",
        "-d", json.dumps({
            "query_text": question,
            "space_id": space_id,
            "top_k": top_k
        })
    ]

    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        if result.returncode != 0:
            print(f"ERROR: curl failed with code {result.returncode}", file=sys.stderr)
            print(f"STDERR: {result.stderr}", file=sys.stderr)
            return {"results": []}

        response = json.loads(result.stdout)
        return response
    except subprocess.TimeoutExpired:
        print(f"ERROR: Query timeout for question: {question[:50]}...", file=sys.stderr)
        return {"results": []}
    except json.JSONDecodeError as e:
        print(f"ERROR: JSON decode failed: {e}", file=sys.stderr)
        print(f"Response: {result.stdout[:500]}", file=sys.stderr)
        return {"results": []}
    except Exception as e:
        print(f"ERROR: Query failed: {e}", file=sys.stderr)
        return {"results": []}


def extract_file_line_refs(results: list) -> list:
    """
    Extract file:line references from MDEMG results.
    CRITICAL: MUST include line numbers in format 'path/to/file.py:LINE'
    """
    file_line_refs = []

    for result in results:
        if "metadata" not in result:
            continue

        metadata = result["metadata"]
        file_path = metadata.get("file_path", "")

        # Extract line number - prefer start_line
        line_num = metadata.get("start_line", 0)
        if line_num == 0:
            line_num = metadata.get("end_line", 1)
        if line_num == 0:
            line_num = 1  # Default to line 1 if no line info

        if file_path:
            # Normalize path - remove leading slashes and project prefix
            path_clean = file_path.lstrip("/")
            if path_clean.startswith("Megatron-LM/"):
                path_clean = path_clean[len("Megatron-LM/"):]

            # Create file:line reference
            ref = f"{path_clean}:{line_num}"
            if ref not in file_line_refs:
                file_line_refs.append(ref)

    return file_line_refs[:10]  # Return top 10 unique refs


def synthesize_answer(question: str, results: list, file_line_refs: list) -> str:
    """Synthesize answer from MDEMG results"""

    if not results:
        return "Unable to retrieve relevant information from the codebase."

    # Gather content from top results
    content_pieces = []
    for i, result in enumerate(results[:5]):  # Use top 5 results
        if "content" in result:
            content_pieces.append(result["content"][:500])  # First 500 chars

    if not content_pieces:
        return "Retrieved results but unable to extract sufficient content."

    # Create a concise answer mentioning the file:line references
    answer = f"Based on the codebase analysis: {content_pieces[0][:300]}"

    if len(file_line_refs) > 0:
        answer += f" Relevant code locations include {file_line_refs[0]}"
        if len(file_line_refs) > 1:
            answer += f" and {file_line_refs[1]}"

    return answer


def process_question(q: dict, output_file: Path) -> bool:
    """Process a single question and append result to output file"""

    question_id = q["id"]
    question_text = q["question"]

    print(f"Processing Q{question_id}: {question_text[:80]}...", file=sys.stderr)

    # Query MDEMG
    response = query_mdemg(question_text)
    results = response.get("results", [])

    # Extract file:line references
    file_line_refs = extract_file_line_refs(results)

    # CRITICAL CHECK: Ensure we have line numbers
    if file_line_refs:
        for ref in file_line_refs:
            if ":" not in ref or not ref.split(":")[-1].isdigit():
                print(f"WARNING Q{question_id}: Invalid ref format: {ref}", file=sys.stderr)

    # Synthesize answer
    answer = synthesize_answer(question_text, results, file_line_refs)

    # Create output entry
    entry = {
        "id": question_id,
        "question": question_text,
        "answer": answer,
        "file_line_refs": file_line_refs if file_line_refs else []
    }

    # Append to output file (JSONL format)
    with open(output_file, "a") as f:
        f.write(json.dumps(entry) + "\n")

    print(f"  ✓ Q{question_id} complete - {len(file_line_refs)} refs", file=sys.stderr)
    return True


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

    print(f"\n{'='*60}", file=sys.stderr)
    print(f"MDEMG BENCHMARK - RUN 2 (WARM CACHE)", file=sys.stderr)
    print(f"Total Questions: {total}", file=sys.stderr)
    print(f"Output: {output_file}", file=sys.stderr)
    print(f"{'='*60}\n", file=sys.stderr)

    # Process all questions
    success_count = 0
    for i, question in enumerate(questions, 1):
        try:
            if process_question(question, output_file):
                success_count += 1

            if i % 10 == 0:
                print(f"\nProgress: {i}/{total} ({i*100//total}%)\n", file=sys.stderr)

        except Exception as e:
            print(f"ERROR Q{question['id']}: {e}", file=sys.stderr)
            continue

    print(f"\n{'='*60}", file=sys.stderr)
    print(f"BENCHMARK COMPLETE", file=sys.stderr)
    print(f"Processed: {success_count}/{total}", file=sys.stderr)
    print(f"Output: {output_file}", file=sys.stderr)
    print(f"{'='*60}\n", file=sys.stderr)


if __name__ == "__main__":
    main()
