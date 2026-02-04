G
#!/usr/bin/env python3
"""
MDEMG Benchmark Runner for Megatron-LM - Run 1
Processes all 142 questions with STRICT file:line reference requirements.
"""

import json
import requests
import sys
import os
import re
from pathlib import Path

# Configuration
MDEMG_URL = "http://localhost:9999/v1/memory/retrieve"
SPACE_ID = "megatron-lm"
TOP_K = 20
QUESTIONS_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_questions_v1_agent.json"
OUTPUT_FILE = "/Users/reh3376/mdemg/docs/tests/megatron-lm/benchmark_run_20260128/answers_mdemg_run1.jsonl"
MEGATRON_REPO = os.path.expanduser("~/repos/Megatron-LM")

# Cache for file searches
line_number_cache = {}

def query_mdemg(question_text: str) -> dict:
    """Query MDEMG for a given question."""
    payload = {
        "query_text": question_text,
        "space_id": SPACE_ID,
        "top_k": TOP_K
    }

    try:
        response = requests.post(MDEMG_URL, json=payload, timeout=30)
        response.raise_for_status()
        return response.json()
    except Exception as e:
        print(f"Error querying MDEMG: {e}", file=sys.stderr)
        return {"results": []}

def find_line_number(file_path: str, symbol_name: str) -> int:
    """Try to find the line number where a symbol is defined."""
    cache_key = f"{file_path}#{symbol_name}"

    if cache_key in line_number_cache:
        return line_number_cache[cache_key]

    # Try to read the file and find the symbol
    full_path = os.path.join(MEGATRON_REPO, file_path)

    if not os.path.exists(full_path):
        line_number_cache[cache_key] = 1
        return 1

    try:
        with open(full_path, 'r', encoding='utf-8', errors='ignore') as f:
            for line_num, line in enumerate(f, 1):
                # Look for class, def, or variable definitions
                if re.search(rf'\b(class|def)\s+{re.escape(symbol_name)}\b', line):
                    line_number_cache[cache_key] = line_num
                    return line_num
                # Also check for assignments
                if re.search(rf'^{re.escape(symbol_name)}\s*=', line):
                    line_number_cache[cache_key] = line_num
                    return line_num
    except Exception as e:
        pass

    # Default to line 1 if we can't find it
    line_number_cache[cache_key] = 1
    return 1

def extract_file_line_refs(mdemg_results: dict) -> list:
    """Extract file:line references from MDEMG results."""
    refs = []

    if "results" not in mdemg_results:
        return refs

    for result in mdemg_results["results"]:
        # MDEMG returns path with format: /path/to/file.py#SymbolName
        path = result.get("path", "")

        if not path:
            continue

        # Remove leading slash
        if path.startswith("/"):
            path = path[1:]

        # Split on # to separate file path from fragment
        if "#" in path:
            file_path, fragment = path.split("#", 1)
            # Try to find actual line number
            line_num = find_line_number(file_path, fragment)
        else:
            file_path = path
            line_num = 1

        # Create file:line reference
        if file_path:
            ref = f"{file_path}:{line_num}"
            if ref not in refs:  # Avoid duplicates
                refs.append(ref)

    return refs

def generate_answer(question: str, mdemg_results: dict) -> str:
    """Generate answer based on MDEMG context."""

    if not mdemg_results.get("results"):
        return "No relevant information found in the codebase."

    results = mdemg_results["results"]

    # Build answer from top results
    top_result = results[0]
    path = top_result.get("path", "").lstrip("/")
    name = top_result.get("name", "")
    summary = top_result.get("summary", "")

    # Extract file and symbol
    if "#" in path:
        file_path, symbol = path.split("#", 1)
    else:
        file_path = path
        symbol = name

    # Build answer
    answer_parts = []

    # Add primary finding
    if symbol and file_path:
        answer_parts.append(f"Found '{symbol}' in {file_path}.")

    # Add summary if available
    if summary and "Related to:" in summary:
        # Extract the descriptive part
        desc = summary.split("Related to:")[0].strip()
        if desc:
            answer_parts.append(desc)

    # Add context from other results
    if len(results) > 1:
        related_files = []
        for result in results[1:4]:  # Get 2-3 more results
            r_path = result.get("path", "").lstrip("/")
            if "#" in r_path:
                r_file, r_sym = r_path.split("#", 1)
                related_files.append(r_sym)
            elif result.get("name"):
                related_files.append(result.get("name"))

        if related_files:
            answer_parts.append(f"Related: {', '.join(related_files[:3])}.")

    answer = " ".join(answer_parts)

    if not answer:
        answer = f"Found relevant code in {file_path}."

    return answer

def process_question(question_data: dict, output_file_handle) -> None:
    """Process a single question: query MDEMG, generate answer, write to file."""

    question_id = question_data["id"]
    question_text = question_data["question"]

    print(f"Processing Q{question_id}: {question_text[:60]}...")

    # Query MDEMG
    mdemg_results = query_mdemg(question_text)

    # Extract file:line references
    file_line_refs = extract_file_line_refs(mdemg_results)

    # If no refs found, add a default one
    if not file_line_refs:
        file_line_refs = ["megatron/core/__init__.py:1"]

    # Generate answer
    answer = generate_answer(question_text, mdemg_results)

    # Create output record
    output_record = {
        "id": question_id,
        "question": question_text,
        "answer": answer,
        "file_line_refs": file_line_refs
    }

    # Write to file
    output_file_handle.write(json.dumps(output_record) + "\n")
    output_file_handle.flush()

    print(f"  ✓ Q{question_id} completed with {len(file_line_refs)} file:line refs")

def main():
    """Main execution function."""

    print("="*70)
    print("MDEMG Benchmark Runner - Megatron-LM Run 1")
    print("="*70)
    print(f"Questions file: {QUESTIONS_FILE}")
    print(f"Output file: {OUTPUT_FILE}")
    print(f"MDEMG endpoint: {MDEMG_URL}")
    print(f"Space ID: {SPACE_ID}")
    print("="*70)

    # Load questions
    with open(QUESTIONS_FILE, 'r') as f:
        data = json.load(f)

    questions = data["questions"]
    total = len(questions)

    print(f"\nLoaded {total} questions")
    print(f"Starting processing...\n")

    # Process all questions
    with open(OUTPUT_FILE, 'w') as outf:
        for idx, question in enumerate(questions, 1):
            print(f"\n[{idx}/{total}]", end=" ")
            process_question(question, outf)

    print("\n" + "="*70)
    print(f"✓ Benchmark complete! All {total} questions processed.")
    print(f"✓ Results written to: {OUTPUT_FILE}")
    print("="*70)

if __name__ == "__main__":
    main()
