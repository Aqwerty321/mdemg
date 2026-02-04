#!/usr/bin/env python3
"""
MDEMG Test v10 Learning - Measures impact of improved learning edge creation

Tests retrieval quality with:
- LLM re-ranking (from v9)
- Improved learning edge creation (all candidates seeded)
- Tracks edge accumulation during test
"""

import json
import urllib.request
import time
from pathlib import Path
from datetime import datetime
from collections import defaultdict

TEST_DIR = Path(__file__).parent
QUESTIONS_FILE = TEST_DIR / "test_questions_v4_selected.json"
OUTPUT_FILE = TEST_DIR / f"mdemg-test-v10-learning-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
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

def get_edge_count() -> int:
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/v1/memory/stats?space_id={SPACE_ID}")
        with urllib.request.urlopen(req, timeout=5) as resp:
            data = json.loads(resp.read().decode('utf-8'))
            return data.get('learning_activity', {}).get('co_activated_edges', 0)
    except:
        return 0

def run_test():
    print("=" * 60)
    print("MDEMG TEST v10 LEARNING - Learning Edge Impact")
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

    # Get initial edge count
    initial_edges = get_edge_count()
    print(f"Initial CO_ACTIVATED_WITH edges: {initial_edges}")
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

        # Count nodes with activation > 0.20
        activated_nodes = sum(1 for n in nodes if n.get('activation', 0) >= 0.20)

        results.append({
            "id": q['id'],
            "category": category,
            "score": top_score,
            "rerank_latency": rerank_latency,
            "rerank_tokens": rerank_tokens,
            "activated_nodes": activated_nodes
        })

        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            current_edges = get_edge_count()
            new_edges = current_edges - initial_edges
            avg_latency = total_rerank_latency / i
            print(f"Progress: {i}/{len(questions)} ({rate:.2f} q/s) - Score: {top_score:.3f}, Edges: +{new_edges}")

    total_time = time.time() - start_time
    final_edges = get_edge_count()
    new_edges_total = final_edges - initial_edges

    # Analysis
    scores = [r['score'] for r in results]
    by_category = defaultdict(list)
    for r in results:
        by_category[r['category']].append(r['score'])

    avg_score = sum(scores) / len(scores)
    avg_activated = sum(r.get('activated_nodes', 0) for r in results) / len(results)

    # Score distribution
    score_dist = {
        ">0.7": sum(1 for s in scores if s > 0.7),
        "0.6-0.7": sum(1 for s in scores if 0.6 <= s < 0.7),
        "0.5-0.6": sum(1 for s in scores if 0.5 <= s < 0.6),
        "0.4-0.5": sum(1 for s in scores if 0.4 <= s < 0.5),
        "<0.4": sum(1 for s in scores if s < 0.4),
    }

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {avg_score:.3f} (v9: 0.619, change: {avg_score - 0.619:+.3f}, {((avg_score/0.619)-1)*100:+.1f}%)")
    print(f"Duration: {total_time:.1f}s ({total_time/len(questions):.1f}s per query)")
    print(f"Avg Rerank Latency: {total_rerank_latency/len(questions):.0f}ms")
    print(f"Avg Activated Nodes: {avg_activated:.1f}/10")
    print(f"Learning Edges: {initial_edges} -> {final_edges} (+{new_edges_total})")
    print(f"\nScore Distribution:")
    for bucket, count in score_dist.items():
        print(f"  {bucket}: {count} ({count}%)")
    print(f"\nBy Category:")

    v9_scores = {
        "architecture_structure": 0.620,
        "service_relationships": 0.626,
        "cross_cutting_concerns": 0.606,
        "data_flow_integration": 0.631,
        "business_logic_constraints": 0.604
    }

    for cat, vals in sorted(by_category.items()):
        cat_avg = sum(vals) / len(vals)
        v9 = v9_scores.get(cat, 0.619)
        print(f"  {cat}: {cat_avg:.3f} (v9: {v9:.3f}, change: {cat_avg - v9:+.3f})")

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(f"# MDEMG Test v10 Learning Results\n\n")
        f.write(f"**Date**: {datetime.now().isoformat()}\n")
        f.write(f"**Duration**: {total_time:.1f}s\n\n")
        f.write(f"## Summary\n\n")
        f.write(f"| Metric | v10 Learning | v9 Rerank | Change |\n")
        f.write(f"|--------|--------------|-----------|--------|\n")
        f.write(f"| Avg Score | {avg_score:.3f} | 0.619 | {avg_score - 0.619:+.3f} ({((avg_score/0.619)-1)*100:+.1f}%) |\n")
        f.write(f"| Max Score | {max(scores):.3f} | 0.804 | {max(scores) - 0.804:+.3f} |\n")
        f.write(f"| Min Score | {min(scores):.3f} | 0.368 | {min(scores) - 0.368:+.3f} |\n\n")
        f.write(f"## Learning Edge Statistics\n\n")
        f.write(f"| Metric | Value |\n")
        f.write(f"|--------|-------|\n")
        f.write(f"| Initial Edges | {initial_edges} |\n")
        f.write(f"| Final Edges | {final_edges} |\n")
        f.write(f"| New Edges Created | {new_edges_total} |\n")
        f.write(f"| Edges per Query | {new_edges_total/len(questions):.1f} |\n")
        f.write(f"| Avg Activated Nodes | {avg_activated:.1f}/10 |\n\n")
        f.write(f"## Score Distribution\n\n")
        f.write(f"| Range | Count | Percentage |\n")
        f.write(f"|-------|-------|------------|\n")
        for bucket, count in score_dist.items():
            f.write(f"| {bucket} | {count} | {count}% |\n")
        f.write(f"\n## By Category\n\n")
        f.write(f"| Category | v10 Learning | v9 Rerank | Change |\n")
        f.write(f"|----------|--------------|-----------|--------|\n")
        for cat, vals in sorted(by_category.items()):
            cat_avg = sum(vals) / len(vals)
            v9 = v9_scores.get(cat, 0.619)
            f.write(f"| {cat} | {cat_avg:.3f} | {v9:.3f} | {cat_avg - v9:+.3f} |\n")

    print(f"\nReport: {OUTPUT_FILE}")
    return {"avg": avg_score, "by_category": {c: sum(v)/len(v) for c,v in by_category.items()}}

if __name__ == "__main__":
    run_test()
