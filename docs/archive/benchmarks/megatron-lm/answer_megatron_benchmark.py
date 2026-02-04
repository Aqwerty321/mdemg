#!/usr/bin/env python3
"""
Answer Megatron-LM benchmark questions using MDEMG retrieval.
This script processes all 142 questions systematically.
"""

import json
import subprocess
import urllib.parse
from pathlib import Path

# Configuration
QUESTIONS_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_questions_v1_agent.json"
OUTPUT_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_run_20260128/answers_mdemg_run1.jsonl"
CODEBASE_PATH = "/Users/reh3376/repos/Megatron-LM"
MDEMG_ENDPOINT = "http://localhost:9999"
SPACE_ID = "megatron-lm"
TOP_K = 10

def query_mdemg(query_text):
    """Query MDEMG and return results."""
    encoded_query = urllib.parse.quote(query_text)
    url = f"{MDEMG_ENDPOINT}/v1/query?q={encoded_query}&space_id={SPACE_ID}&top_k={TOP_K}"

    try:
        result = subprocess.run(
            ["curl", "-s", url],
            capture_output=True,
            text=True,
            timeout=30
        )
        if result.returncode == 0 and result.stdout:
            return json.loads(result.stdout)
        return None
    except Exception as e:
        print(f"Error querying MDEMG: {e}")
        return None

def read_file_lines(file_path, line_num, context_lines=5):
    """Read specific lines from a file with context."""
    try:
        full_path = Path(CODEBASE_PATH) / file_path
        with open(full_path, 'r') as f:
            lines = f.readlines()

        start = max(0, line_num - context_lines - 1)
        end = min(len(lines), line_num + context_lines)
        return ''.join(lines[start:end])
    except Exception as e:
        return f"Error reading file: {e}"

def answer_question(question_obj):
    """Answer a single question using MDEMG retrieval."""
    q_id = question_obj["id"]
    category = question_obj["category"]
    question = question_obj["question"]

    print(f"\n[Q{q_id}] {category}: {question}")

    # Query MDEMG
    mdemg_results = query_mdemg(question)

    if not mdemg_results or "results" not in mdemg_results:
        return {
            "id": q_id,
            "question": question,
            "answer": "Unable to retrieve information from MDEMG.",
            "file_line_refs": []
        }

    # Extract relevant file locations
    file_line_refs = []
    context_snippets = []

    for result in mdemg_results.get("results", [])[:5]:  # Top 5 results
        if "file_path" in result and "line_number" in result:
            file_path = result["file_path"]
            line_num = result["line_number"]
            file_line_refs.append(f"{file_path}:{line_num}")

            # Get context
            snippet = read_file_lines(file_path, line_num)
            context_snippets.append({
                "file": file_path,
                "line": line_num,
                "content": snippet,
                "text": result.get("text", "")
            })

    # Generate answer based on category and question
    answer = generate_answer(question_obj, context_snippets, mdemg_results)

    return {
        "id": q_id,
        "question": question,
        "answer": answer,
        "file_line_refs": file_line_refs[:3]  # Limit to top 3
    }

def generate_answer(question_obj, context_snippets, mdemg_results):
    """Generate a synthesized answer from context."""
    q_id = question_obj["id"]
    category = question_obj["category"]
    question = question_obj["question"]

    # For negative control questions (133-142)
    if category == "negative_control":
        # These need careful evaluation - if feature doesn't exist, say so
        return f"This requires manual evaluation of context. Question ID: {q_id}"

    # For calibration questions (118-132) - these are straightforward facts
    if category == "calibration":
        return f"This requires specific factual answer. Question ID: {q_id}"

    # For all other categories, synthesize from context
    if not context_snippets:
        return "Unable to find relevant information in the codebase."

    # Placeholder - will be filled in manually or with more sophisticated logic
    return f"Answer based on retrieved context for Q{q_id}. Manual synthesis needed."

def main():
    # Load questions
    with open(QUESTIONS_FILE, 'r') as f:
        data = json.load(f)

    questions = data["questions"]

    # Ensure output directory exists
    output_dir = Path(OUTPUT_FILE).parent
    output_dir.mkdir(parents=True, exist_ok=True)

    # Process questions
    with open(OUTPUT_FILE, 'w') as out_f:
        for i, question_obj in enumerate(questions, 1):
            print(f"\nProcessing {i}/{len(questions)}")
            answer_obj = answer_question(question_obj)
            out_f.write(json.dumps(answer_obj) + '\n')
            out_f.flush()

    print(f"\n\nCompleted! Answers written to: {OUTPUT_FILE}")

if __name__ == "__main__":
    main()
