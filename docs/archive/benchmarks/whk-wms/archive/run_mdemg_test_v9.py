#!/usr/bin/env python3
"""
MDEMG Test v9 - Hybrid Retrieval (Vector + BM25)

Tests retrieval quality with hybrid retrieval:
- Vector similarity search (embeddings)
- BM25 full-text search (keywords)
- Reciprocal Rank Fusion (RRF) to combine results
"""

import json
import urllib.request
import urllib.error
import time
from pathlib import Path
from datetime import datetime
from collections import defaultdict

# Configuration
TEST_DIR = Path(__file__).parent
QUESTIONS_FILE = TEST_DIR / "test_questions_v4_selected.json"
OUTPUT_FILE = TEST_DIR / f"mdemg-test-v9-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
MDEMG_ENDPOINT = "http://localhost:8090"
SPACE_ID = "whk-wms"

def load_questions():
    with open(QUESTIONS_FILE) as f:
        data = json.load(f)
    return data['questions']

def query_mdemg(question: str) -> dict:
    """Query MDEMG for relevant context"""
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

def analyze_results(results: list) -> dict:
    """Analyze test results"""
    scores = [r['score'] for r in results]
    by_category = defaultdict(list)
    for r in results:
        by_category[r['category']].append(r['score'])

    return {
        "total_questions": len(results),
        "avg_score": sum(scores) / len(scores) if scores else 0,
        "max_score": max(scores) if scores else 0,
        "min_score": min(scores) if scores else 0,
        "score_dist": {
            ">0.7": sum(1 for s in scores if s > 0.7),
            "0.6-0.7": sum(1 for s in scores if 0.6 <= s < 0.7),
            "0.5-0.6": sum(1 for s in scores if 0.5 <= s < 0.6),
            "0.4-0.5": sum(1 for s in scores if 0.4 <= s < 0.5),
            "<0.4": sum(1 for s in scores if s < 0.4),
        },
        "by_category": {
            cat: {
                "avg": sum(vals)/len(vals),
                "count": len(vals)
            }
            for cat, vals in by_category.items()
        }
    }

def run_test():
    print("=" * 60)
    print("MDEMG TEST v9 - HYBRID RETRIEVAL (Vector + BM25)")
    print("=" * 60)

    # Check MDEMG health
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/healthz")
        with urllib.request.urlopen(req, timeout=5) as resp:
            print(f"MDEMG Status: {resp.read().decode('utf-8')}")
    except Exception as e:
        print(f"ERROR: MDEMG not reachable: {e}")
        return

    questions = load_questions()
    print(f"Loaded {len(questions)} questions")
    print(f"Space ID: {SPACE_ID}")
    print("-" * 60)

    results = []
    start_time = time.time()

    # Track debug info
    hybrid_info = {"vector_total": 0, "bm25_total": 0, "fused_total": 0}

    for i, q in enumerate(questions, 1):
        qtext = q['question']
        category = q['category']

        # Query MDEMG
        resp = query_mdemg(qtext)

        if "error" in resp:
            print(f"Q{i}: ERROR - {resp['error']}")
            results.append({
                "id": q['id'],
                "category": category,
                "question": qtext,
                "score": 0,
                "nodes": 0,
                "error": resp['error']
            })
            continue

        # Response format
        nodes = resp.get('results', [])
        debug = resp.get('debug', {})

        # Track hybrid stats
        hybrid_info["vector_total"] += debug.get("vector_count", 0)
        hybrid_info["bm25_total"] += debug.get("bm25_count", 0)
        hybrid_info["fused_total"] += debug.get("fused_count", 0)

        # Calculate retrieval score - use TOP node score
        if nodes:
            top_score = nodes[0].get('score', 0)
        else:
            top_score = 0

        results.append({
            "id": q['id'],
            "category": category,
            "question": qtext,
            "score": top_score,
            "nodes": len(nodes),
            "hybrid_enabled": debug.get("hybrid_enabled", False),
            "rerank_enabled": debug.get("rerank_enabled", False),
        })

        # Progress
        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            print(f"Progress: {i}/{len(questions)} ({rate:.1f} q/s) - Last score: {top_score:.3f}")

    total_time = time.time() - start_time

    # Analyze results
    analysis = analyze_results(results)

    # Generate report
    report = f"""# MDEMG Test v9 Results - Hybrid Retrieval (Vector + BM25)

**Generated**: {datetime.now().isoformat()}
**Space**: {SPACE_ID}
**Questions**: {len(questions)}
**Duration**: {total_time:.1f}s

---

## Summary

| Metric | v9 (Hybrid) | v8 (Baseline) | Change |
|--------|-------------|---------------|--------|
| Avg Score | {analysis['avg_score']:.3f} | 0.562 | {(analysis['avg_score'] - 0.562):+.3f} ({((analysis['avg_score']/0.562)-1)*100:+.1f}%) |
| Max Score | {analysis['max_score']:.3f} | 0.836 | {(analysis['max_score'] - 0.836):+.3f} |
| Min Score | {analysis['min_score']:.3f} | 0.394 | {(analysis['min_score'] - 0.394):+.3f} |

## Hybrid Retrieval Stats

| Metric | Value |
|--------|-------|
| Avg Vector Candidates | {hybrid_info['vector_total']/len(questions):.1f} |
| Avg BM25 Candidates | {hybrid_info['bm25_total']/len(questions):.1f} |
| Avg Fused Candidates | {hybrid_info['fused_total']/len(questions):.1f} |

## Score Distribution

| Range | Count | Percentage |
|-------|-------|------------|
| > 0.7 | {analysis['score_dist']['>0.7']} | {analysis['score_dist']['>0.7']}% |
| 0.6-0.7 | {analysis['score_dist']['0.6-0.7']} | {analysis['score_dist']['0.6-0.7']}% |
| 0.5-0.6 | {analysis['score_dist']['0.5-0.6']} | {analysis['score_dist']['0.5-0.6']}% |
| 0.4-0.5 | {analysis['score_dist']['0.4-0.5']} | {analysis['score_dist']['0.4-0.5']}% |
| < 0.4 | {analysis['score_dist']['<0.4']} | {analysis['score_dist']['<0.4']}% |

## By Category

| Category | v9 Score | v8 Score | Change |
|----------|----------|----------|--------|
"""

    v8_scores = {
        "architecture_structure": 0.553,
        "service_relationships": 0.583,
        "cross_cutting_concerns": 0.549,
        "data_flow_integration": 0.540,
        "business_logic_constraints": 0.589
    }

    for cat, stats in sorted(analysis['by_category'].items()):
        v8 = v8_scores.get(cat, 0.562)
        change = stats['avg'] - v8
        pct_change = ((stats['avg'] / v8) - 1) * 100 if v8 > 0 else 0
        report += f"| {cat} | {stats['avg']:.3f} | {v8:.3f} | {change:+.3f} ({pct_change:+.1f}%) |\n"

    report += f"""
## Detailed Results

"""
    for r in results:
        report += f"- Q{r['id']} ({r['category']}): {r['score']:.3f}\n"

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(report)

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {analysis['avg_score']:.3f} (v8: 0.562, change: {(analysis['avg_score'] - 0.562):+.3f})")
    print(f"\nBy Category:")
    for cat, stats in sorted(analysis['by_category'].items()):
        v8 = v8_scores.get(cat, 0.562)
        change = stats['avg'] - v8
        print(f"  {cat}: {stats['avg']:.3f} (v8: {v8:.3f}, change: {change:+.3f})")
    print(f"\nReport: {OUTPUT_FILE}")

    return analysis

if __name__ == "__main__":
    run_test()
