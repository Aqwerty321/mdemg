#!/usr/bin/env python3
"""
MDEMG Benchmark Run 3 (Warm Start) for Zed Codebase
Queries MDEMG for context and generates answers for benchmark questions.
"""

import json
import requests
import sys
from pathlib import Path

MDEMG_URL = "http://localhost:9999/v1/memory/retrieve"
QUESTIONS_FILE = "/Users/reh3376/mdemg/docs/tests/zed/benchmark_questions_v1_agent.json"
OUTPUT_FILE = "/Users/reh3376/mdemg/docs/tests/zed/benchmark_run_20260128/answers_mdemg_run3.jsonl"

def query_mdemg(question: str, space_id: str = "zed", top_k: int = 10) -> dict:
    """Query MDEMG for context related to the question."""
    payload = {
        "query_text": question,
        "space_id": space_id,
        "top_k": top_k,
        "include_evidence": True
    }
    try:
        resp = requests.post(MDEMG_URL, json=payload, timeout=30)
        resp.raise_for_status()
        return resp.json()
    except Exception as e:
        print(f"MDEMG query error: {e}")
        return {"results": [], "error": str(e)}

def format_context(mdemg_response: dict) -> str:
    """Format MDEMG results as context string."""
    results = mdemg_response.get("results", [])
    if not results:
        return ""

    context_parts = []
    for r in results[:10]:  # Top 10 results
        path = r.get("path", "")
        name = r.get("name", "")
        summary = r.get("summary", "")
        score = r.get("score", 0)
        context_parts.append(f"- {path}: {summary} (score: {score:.2f})")

    return "\n".join(context_parts)

def main():
    # Load questions
    with open(QUESTIONS_FILE) as f:
        data = json.load(f)

    questions = data["questions"]
    print(f"Processing {len(questions)} questions for Run 3 (warm start)...")

    # Clear output file
    with open(OUTPUT_FILE, 'w') as f:
        pass

    for q in questions:
        qid = q["id"]
        question = q["question"]
        category = q.get("category", "unknown")

        print(f"Processing Q{qid}/{len(questions)}: {question[:60]}...")

        # Query MDEMG
        mdemg_result = query_mdemg(question)
        context = format_context(mdemg_result)

        # Extract file references
        file_refs = [r.get("path", "") for r in mdemg_result.get("results", [])[:5]]

        # Write result
        result = {
            "id": qid,
            "question": question,
            "category": category,
            "mdemg_context": context,
            "file_refs": file_refs,
            "context_used": bool(context),
            "debug": mdemg_result.get("debug", {})
        }

        with open(OUTPUT_FILE, 'a') as f:
            f.write(json.dumps(result) + "\n")

        print(f"  -> {len(file_refs)} files referenced")

    print(f"\nComplete! Results written to {OUTPUT_FILE}")

if __name__ == "__main__":
    main()
