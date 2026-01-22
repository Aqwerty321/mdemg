#!/usr/bin/env python3
"""
MDEMG Test v9 Rerank - LLM Re-ranking

Tests retrieval quality with LLM re-ranking:
- Initial retrieval via vector search
- Top candidates re-scored by GPT-4o-mini
"""

import json
import urllib.request
import time
from pathlib import Path
from datetime import datetime
from collections import defaultdict

TEST_DIR = Path(__file__).parent
QUESTIONS_FILE = TEST_DIR / "test_questions_v4_selected.json"
OUTPUT_FILE = TEST_DIR / f"mdemg-test-v9-rerank-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
MDEMG_ENDPOINT = "http://localhost:8090"
SPACE_ID = "whk-wms"

def load_questions():
    with open(QUESTIONS_FILE) as f:
        return json.load(f)['questions']

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

def run_test():
    print("=" * 60)
    print("MDEMG TEST v9 RERANK - LLM Re-ranking")
    print("=" * 60)

    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/healthz")
        with urllib.request.urlopen(req, timeout=5) as resp:
            print(f"MDEMG Status: {resp.read().decode('utf-8')}")
    except Exception as e:
        print(f"ERROR: MDEMG not reachable: {e}")
        return

    questions = load_questions()
    print(f"Loaded {len(questions)} questions")
    print("-" * 60)

    results = []
    start_time = time.time()
    total_rerank_latency = 0
    total_rerank_tokens = 0

    for i, q in enumerate(questions, 1):
        qtext = q['question']
        category = q['category']
        resp = query_mdemg(qtext)

        if "error" in resp:
            print(f"Q{i}: ERROR - {resp['error']}")
            results.append({"id": q['id'], "category": category, "score": 0})
            continue

        nodes = resp.get('results', [])
        debug = resp.get('debug', {})

        top_score = nodes[0].get('score', 0) if nodes else 0
        rerank_latency = debug.get('rerank_latency_ms', 0)
        rerank_tokens = debug.get('rerank_tokens', 0)

        total_rerank_latency += rerank_latency
        total_rerank_tokens += rerank_tokens

        results.append({
            "id": q['id'],
            "category": category,
            "score": top_score,
            "rerank_latency": rerank_latency,
            "rerank_tokens": rerank_tokens
        })

        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            avg_latency = total_rerank_latency / i
            print(f"Progress: {i}/{len(questions)} ({rate:.2f} q/s) - Score: {top_score:.3f}, Rerank: {avg_latency:.0f}ms avg")

    total_time = time.time() - start_time

    # Analysis
    scores = [r['score'] for r in results]
    by_category = defaultdict(list)
    for r in results:
        by_category[r['category']].append(r['score'])

    avg_score = sum(scores) / len(scores)

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {avg_score:.3f} (v8: 0.562, change: {avg_score - 0.562:+.3f}, {((avg_score/0.562)-1)*100:+.1f}%)")
    print(f"Duration: {total_time:.1f}s ({total_time/len(questions):.1f}s per query)")
    print(f"Avg Rerank Latency: {total_rerank_latency/len(questions):.0f}ms")
    print(f"Total Rerank Tokens: {total_rerank_tokens}")
    print(f"\nBy Category:")

    v8_scores = {
        "architecture_structure": 0.553,
        "service_relationships": 0.583,
        "cross_cutting_concerns": 0.549,
        "data_flow_integration": 0.540,
        "business_logic_constraints": 0.589
    }

    for cat, vals in sorted(by_category.items()):
        cat_avg = sum(vals) / len(vals)
        v8 = v8_scores.get(cat, 0.562)
        print(f"  {cat}: {cat_avg:.3f} (v8: {v8:.3f}, change: {cat_avg - v8:+.3f})")

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(f"# MDEMG Test v9 Rerank Results\n\n")
        f.write(f"**Date**: {datetime.now().isoformat()}\n")
        f.write(f"**Duration**: {total_time:.1f}s\n\n")
        f.write(f"## Summary\n\n")
        f.write(f"| Metric | v9 Rerank | v8 | Change |\n")
        f.write(f"|--------|-----------|-----|--------|\n")
        f.write(f"| Avg Score | {avg_score:.3f} | 0.562 | {avg_score - 0.562:+.3f} ({((avg_score/0.562)-1)*100:+.1f}%) |\n")
        f.write(f"| Max Score | {max(scores):.3f} | 0.836 | {max(scores) - 0.836:+.3f} |\n")
        f.write(f"| Min Score | {min(scores):.3f} | 0.394 | {min(scores) - 0.394:+.3f} |\n\n")
        f.write(f"## By Category\n\n")
        f.write(f"| Category | v9 Rerank | v8 | Change |\n")
        f.write(f"|----------|-----------|-----|--------|\n")
        for cat, vals in sorted(by_category.items()):
            cat_avg = sum(vals) / len(vals)
            v8 = v8_scores.get(cat, 0.562)
            f.write(f"| {cat} | {cat_avg:.3f} | {v8:.3f} | {cat_avg - v8:+.3f} |\n")

    print(f"\nReport: {OUTPUT_FILE}")
    return {"avg": avg_score, "by_category": {c: sum(v)/len(v) for c,v in by_category.items()}}

if __name__ == "__main__":
    run_test()
