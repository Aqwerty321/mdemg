#!/usr/bin/env python3
"""
Comparison Test - Test retrieval on current vs fresh state
"""

import json
import urllib.request
import urllib.error
import time
import random
from pathlib import Path
from datetime import datetime
from collections import defaultdict

TEST_DIR = Path(__file__).parent
QUESTIONS_FILE = TEST_DIR / "whk-wms-questions-final.json"
MDEMG_ENDPOINT = "http://localhost:8090"
SPACE_ID = "whk-wms"

def load_questions(n=100):
    with open(QUESTIONS_FILE) as f:
        data = json.load(f)
    questions = data.get('questions', data)  # Handle both formats
    # Random sample of 100
    if len(questions) > n:
        random.seed(42)  # Fixed seed for reproducibility
        questions = random.sample(questions, n)
    return questions

def query_mdemg(question: str) -> dict:
    try:
        data = json.dumps({
            "space_id": SPACE_ID,
            "query_text": question,
            "candidate_k": 50,
            "top_k": 10,
            "hop_depth": 2
        }).encode('utf-8')
        req = urllib.request.Request(
            f"{MDEMG_ENDPOINT}/v1/memory/retrieve",
            data=data,
            headers={'Content-Type': 'application/json'}
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except Exception as e:
        return {"error": str(e)}

def run_test(test_name: str, output_suffix: str):
    print("=" * 60)
    print(f"COMPARISON TEST - {test_name}")
    print("=" * 60)

    # Check health
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/healthz")
        with urllib.request.urlopen(req, timeout=5) as resp:
            print(f"MDEMG Status: {resp.read().decode('utf-8')}")
    except Exception as e:
        print(f"ERROR: MDEMG not reachable: {e}")
        return None

    questions = load_questions(100)
    print(f"Loaded {len(questions)} questions (seed=42)")
    print("-" * 60)

    results = []
    scores = []
    start_time = time.time()

    for i, q in enumerate(questions, 1):
        qid = q.get('id', i)
        qtext = q.get('question', q.get('text', str(q)))
        category = q.get('category', 'unknown')

        resp = query_mdemg(qtext)

        if "error" in resp:
            score = 0
        else:
            nodes = resp.get('results', []) or resp.get('data', {}).get('results', [])
            score = nodes[0].get('score', 0) if nodes else 0

        scores.append(score)
        results.append({
            "id": qid,
            "question": qtext[:100],
            "score": score,
            "category": category
        })

        if i % 20 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            avg = sum(scores) / len(scores)
            print(f"Progress: {i}/{len(questions)} ({rate:.1f} q/s) - Avg: {avg:.3f}")

    total_time = time.time() - start_time

    # Calculate stats
    avg_score = sum(scores) / len(scores)
    max_score = max(scores)
    min_score = min(scores)

    by_category = defaultdict(list)
    for r in results:
        by_category[r['category']].append(r['score'])

    # Write results
    output_file = TEST_DIR / f"comparison-{output_suffix}-{datetime.now().strftime('%Y%m%d-%H%M%S')}.json"
    with open(output_file, 'w') as f:
        json.dump({
            "test_name": test_name,
            "timestamp": datetime.now().isoformat(),
            "duration_s": total_time,
            "total_questions": len(questions),
            "avg_score": avg_score,
            "max_score": max_score,
            "min_score": min_score,
            "by_category": {cat: sum(vals)/len(vals) for cat, vals in by_category.items()},
            "results": results
        }, f, indent=2)

    print("\n" + "=" * 60)
    print(f"TEST COMPLETE: {test_name}")
    print("=" * 60)
    print(f"Avg Score: {avg_score:.3f}")
    print(f"Max Score: {max_score:.3f}")
    print(f"Min Score: {min_score:.3f}")
    print(f"Duration: {total_time:.1f}s")
    print(f"Output: {output_file}")

    return {
        "avg_score": avg_score,
        "max_score": max_score,
        "min_score": min_score,
        "results": results,
        "output_file": str(output_file)
    }

if __name__ == "__main__":
    import sys
    test_name = sys.argv[1] if len(sys.argv) > 1 else "Test"
    suffix = sys.argv[2] if len(sys.argv) > 2 else "test"
    run_test(test_name, suffix)
