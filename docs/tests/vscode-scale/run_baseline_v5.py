#!/usr/bin/env python3
"""
V5 Comprehensive Benchmark: Baseline Test (No Symbol Extraction)
Tests MDEMG retrieval accuracy on 60 evidence-locked questions.

Scoring:
- exact_file_match: correct file returned in top-3
- correct_value: correct value found in summary (manual check)
"""

import json
import requests
import time
from datetime import datetime

MDEMG_URL = "http://localhost:8090/v1/memory/retrieve"
QUESTIONS_FILE = "/Users/reh3376/mdemg/docs/tests/vscode-scale/test_questions_v5_comprehensive.json"
OUTPUT_FILE = "/Users/reh3376/mdemg/docs/tests/vscode-scale/v5_baseline_results.json"

def load_questions():
    """Load the v5 comprehensive question set."""
    with open(QUESTIONS_FILE) as f:
        data = json.load(f)
    return data["questions"]

def query_mdemg(question: str, top_k: int = 5) -> dict:
    """Query MDEMG retrieval API."""
    try:
        response = requests.post(
            MDEMG_URL,
            json={
                "space_id": "vscode-scale",
                "query_text": question,
                "candidate_k": 100,
                "top_k": top_k,
                "jiminy_enabled": True
            },
            timeout=30
        )
        return response.json()
    except Exception as e:
        return {"error": str(e), "results": []}

def check_file_match(results: list, expected_file: str) -> tuple:
    """Check if expected file is in results."""
    for i, r in enumerate(results):
        path = r.get("path", "")
        if expected_file.lower() in path.lower():
            return True, i + 1, path
    return False, 0, None

def run_benchmark():
    """Run the v5 baseline benchmark."""
    questions = load_questions()
    results = []

    print(f"MDEMG V5 Baseline Benchmark")
    print(f"Questions: {len(questions)}")
    print("=" * 70)

    correct_file = 0
    total = len(questions)

    for i, q in enumerate(questions):
        qid = q["id"]
        question = q["question"]
        expected_file = q["expected"]["file"]
        expected_value = q["expected"].get("value", "")
        category = q.get("category", "unknown")

        print(f"\n[{i+1:02d}/{total}] {qid} ({category})")
        print(f"  Q: {question[:60]}...")
        print(f"  Expected: {expected_file} = {expected_value}")

        # Query MDEMG
        start = time.time()
        response = query_mdemg(question)
        elapsed = time.time() - start

        mdemg_results = response.get("results", [])

        # Check if correct file is in results
        file_match, rank, matched_path = check_file_match(mdemg_results, expected_file)

        if file_match:
            correct_file += 1
            print(f"  ✓ File found at rank {rank}: {matched_path}")
        else:
            top_paths = [r.get("path", "").split("/")[-1] for r in mdemg_results[:3]]
            print(f"  ✗ File NOT found. Top 3: {', '.join(top_paths)}")

        # Store result
        result = {
            "id": qid,
            "category": category,
            "question": question,
            "expected_file": expected_file,
            "expected_value": expected_value,
            "file_match": file_match,
            "file_rank": rank if file_match else None,
            "matched_path": matched_path,
            "top_results": [
                {
                    "path": r.get("path", ""),
                    "score": r.get("score", 0),
                    "summary": r.get("summary", "")[:200]
                }
                for r in mdemg_results[:5]
            ],
            "latency_ms": int(elapsed * 1000)
        }
        results.append(result)

    # Summary
    print("\n" + "=" * 70)
    print("SUMMARY")
    print("=" * 70)
    print(f"Total questions: {total}")
    print(f"Correct file in top-5: {correct_file}/{total} ({100*correct_file/total:.1f}%)")

    # By category
    categories = {}
    for r in results:
        cat = r["category"]
        if cat not in categories:
            categories[cat] = {"total": 0, "correct": 0}
        categories[cat]["total"] += 1
        if r["file_match"]:
            categories[cat]["correct"] += 1

    print("\nBy category:")
    for cat, stats in sorted(categories.items()):
        pct = 100 * stats["correct"] / stats["total"] if stats["total"] > 0 else 0
        print(f"  {cat}: {stats['correct']}/{stats['total']} ({pct:.0f}%)")

    # Save results
    output = {
        "metadata": {
            "test": "v5-baseline",
            "timestamp": datetime.now().isoformat(),
            "total_questions": total,
            "correct_file_matches": correct_file,
            "accuracy_pct": round(100 * correct_file / total, 1)
        },
        "by_category": categories,
        "results": results
    }

    with open(OUTPUT_FILE, "w") as f:
        json.dump(output, f, indent=2)

    print(f"\nResults saved to: {OUTPUT_FILE}")
    return output

if __name__ == "__main__":
    run_benchmark()
