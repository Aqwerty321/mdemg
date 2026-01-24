#!/usr/bin/env python3
"""
Combined Benchmark: v4 (multi-hop) + v3 (hard) + v5 (simple constants)
Tests MDEMG retrieval accuracy on 41 questions across difficulty levels.

Composition:
- 15 multi-hop evidence-locked questions (v4)
- 20 hard multi-file correlation questions (v3)
- 6 simple constant lookup questions (v5 sample)
"""

import json
import requests
import time
from datetime import datetime

MDEMG_URL = "http://localhost:8090/v1/memory/retrieve"
QUESTIONS_FILE = "/Users/reh3376/mdemg/docs/tests/vscode-scale/test_questions_combined.json"
OUTPUT_FILE = "/Users/reh3376/mdemg/docs/tests/vscode-scale/combined_benchmark_results.json"

def load_questions():
    """Load the combined question set."""
    with open(QUESTIONS_FILE) as f:
        data = json.load(f)
    return data["questions"], data["metadata"]

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

def check_evidence(results: list, q: dict) -> dict:
    """Check if required evidence is found in results."""
    evidence_found = {}

    # Get expected evidence
    expected = q.get("required_evidence", q.get("expected", {}))
    if not expected:
        return {"score": 0, "found": {}, "missing": ["no expected evidence defined"]}

    # Check each piece of expected evidence
    found = {}
    missing = []

    for key, expected_value in expected.items():
        # Skip complex nested structures for now
        if isinstance(expected_value, (list, dict)):
            continue

        # Search in results
        value_found = False
        for r in results:
            path = r.get("path", "").lower()
            summary = r.get("summary", "").lower()
            name = r.get("name", "").lower()

            expected_str = str(expected_value).lower()

            # Check if expected value appears in results
            if expected_str in path or expected_str in summary or expected_str in name:
                value_found = True
                found[key] = {"expected": expected_value, "found_in": r.get("path", "")}
                break

            # For file matches, check if file name is in path
            if "file" in key.lower() and expected_str in path:
                value_found = True
                found[key] = {"expected": expected_value, "found_in": r.get("path", "")}
                break

        if not value_found:
            missing.append(key)

    # Calculate score
    total = len([k for k, v in expected.items() if not isinstance(v, (list, dict))])
    if total == 0:
        score = 0
    else:
        score = len(found) / total

    return {
        "score": score,
        "found": found,
        "missing": missing
    }

def run_benchmark():
    """Run the combined benchmark."""
    questions, metadata = load_questions()
    results = []

    print(f"MDEMG Combined Benchmark")
    print(f"Composition: {metadata['composition']}")
    print(f"Total questions: {len(questions)}")
    print("=" * 70)

    # Track by source
    by_source = {"v4_evidence_locked": [], "v3_hard": [], "v5_comprehensive": []}

    for i, q in enumerate(questions):
        qid = q["id"]
        source = q["source"]
        qtype = q.get("type", "unknown")
        category = q.get("category", "unknown")
        question = q["question"]

        print(f"\n[{i+1:02d}/{len(questions)}] {qid} ({source}/{qtype})")
        print(f"  Category: {category}")
        print(f"  Q: {question[:70]}...")

        # Query MDEMG
        start = time.time()
        response = query_mdemg(question)
        elapsed = time.time() - start

        mdemg_results = response.get("results", [])

        # Check evidence
        evidence = check_evidence(mdemg_results, q)

        if evidence["score"] > 0:
            print(f"  ✓ Evidence score: {evidence['score']:.0%}")
            for key, val in evidence["found"].items():
                print(f"    - {key}: found in {val['found_in'][:50]}...")
        else:
            print(f"  ✗ No evidence found")
            if evidence["missing"]:
                print(f"    Missing: {', '.join(evidence['missing'][:3])}")

        top_paths = [r.get("path", "").split("/")[-1] for r in mdemg_results[:3]]
        print(f"  Top 3: {', '.join(top_paths)}")

        # Store result
        result = {
            "id": qid,
            "source": source,
            "type": qtype,
            "category": category,
            "question": question,
            "evidence_score": evidence["score"],
            "evidence_found": evidence["found"],
            "evidence_missing": evidence["missing"],
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
        by_source[source].append(result)

    # Summary
    print("\n" + "=" * 70)
    print("SUMMARY")
    print("=" * 70)

    def calc_stats(res_list):
        if not res_list:
            return {"avg_score": 0, "count": 0, "any_evidence": 0}
        scores = [r["evidence_score"] for r in res_list]
        any_ev = len([s for s in scores if s > 0])
        return {
            "avg_score": sum(scores) / len(scores),
            "count": len(scores),
            "any_evidence": any_ev,
            "pct_any_evidence": 100 * any_ev / len(scores)
        }

    print(f"\nTotal questions: {len(questions)}")

    total_any = sum(1 for r in results if r["evidence_score"] > 0)
    print(f"Questions with ANY evidence: {total_any}/{len(results)} ({100*total_any/len(results):.1f}%)")

    print("\nBy source:")
    for source, res_list in by_source.items():
        stats = calc_stats(res_list)
        print(f"  {source}: {stats['any_evidence']}/{stats['count']} ({stats['pct_any_evidence']:.0f}%) - avg score: {stats['avg_score']:.2f}")

    # By category
    categories = {}
    for r in results:
        cat = r["category"]
        if cat not in categories:
            categories[cat] = []
        categories[cat].append(r)

    print("\nBy category:")
    for cat, res_list in sorted(categories.items(), key=lambda x: -calc_stats(x[1])["avg_score"]):
        stats = calc_stats(res_list)
        print(f"  {cat}: {stats['any_evidence']}/{stats['count']} ({stats['pct_any_evidence']:.0f}%)")

    # Save results
    output = {
        "metadata": {
            "test": "combined-benchmark",
            "timestamp": datetime.now().isoformat(),
            "total_questions": len(questions),
            "composition": metadata["composition"],
            "questions_with_evidence": total_any,
            "overall_evidence_rate": round(100 * total_any / len(results), 1)
        },
        "by_source": {
            source: calc_stats(res_list)
            for source, res_list in by_source.items()
        },
        "by_category": {
            cat: calc_stats(res_list)
            for cat, res_list in categories.items()
        },
        "results": results
    }

    with open(OUTPUT_FILE, "w") as f:
        json.dump(output, f, indent=2)

    print(f"\nResults saved to: {OUTPUT_FILE}")
    return output

if __name__ == "__main__":
    run_benchmark()
