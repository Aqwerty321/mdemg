#!/usr/bin/env python3
"""
MDEMG Test v5 - P1 Improvement Validation

Tests retrieval quality after P1 (Cross-Cutting Concern Nodes) implementation.
Uses existing whk-wms space with embeddings and concern nodes.
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
OUTPUT_FILE = TEST_DIR / f"mdemg-test-v5-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
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

    concern_hits = sum(1 for r in results if r.get('concern_nodes', 0) > 0)

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
        },
        "concern_node_hits": concern_hits,
        "concern_hit_rate": concern_hits / len(results) if results else 0
    }

def run_test():
    print("=" * 60)
    print("MDEMG TEST v5 - P1 IMPROVEMENT VALIDATION")
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
                "concern_nodes": 0,
                "error": resp['error']
            })
            continue

        # Response format: {space_id, results, debug} - results is directly at root or under data
        nodes = resp.get('results', []) or resp.get('data', {}).get('results', [])

        # Calculate retrieval score - use TOP node score (matches v4 methodology)
        if nodes:
            top_score = nodes[0].get('score', 0) if nodes else 0
            # Also compute average for comparison
            node_scores = [n.get('score', 0) for n in nodes]
            avg_score = sum(node_scores) / len(node_scores) if node_scores else 0

            # Count concern nodes in results
            concern_count = sum(1 for n in nodes if 'concern:' in n.get('name', ''))
        else:
            top_score = 0
            avg_score = 0
            concern_count = 0

        results.append({
            "id": q['id'],
            "category": category,
            "question": qtext,
            "score": top_score,  # Use top score to match v4
            "avg_score": avg_score,
            "nodes": len(nodes),
            "concern_nodes": concern_count,
        })

        # Progress
        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            print(f"Progress: {i}/{len(questions)} ({rate:.1f} q/s) - Last score: {avg_score:.3f}")

    total_time = time.time() - start_time

    # Analyze results
    analysis = analyze_results(results)

    # Generate report
    report = f"""# MDEMG Test v5 Results - P1 Improvement Validation

**Generated**: {datetime.now().isoformat()}
**Space**: {SPACE_ID}
**Questions**: {len(questions)}
**Duration**: {total_time:.1f}s

---

## Summary

| Metric | v5 (P1) | v4 (Baseline) | Change |
|--------|---------|---------------|--------|
| Avg Score | {analysis['avg_score']:.3f} | 0.567 | {(analysis['avg_score'] - 0.567):+.3f} |
| Max Score | {analysis['max_score']:.3f} | 0.750 | {(analysis['max_score'] - 0.750):+.3f} |
| Min Score | {analysis['min_score']:.3f} | 0.449 | {(analysis['min_score'] - 0.449):+.3f} |

## Score Distribution

| Range | Count | Percentage |
|-------|-------|------------|
| > 0.7 | {analysis['score_dist']['>0.7']} | {analysis['score_dist']['>0.7']}% |
| 0.6-0.7 | {analysis['score_dist']['0.6-0.7']} | {analysis['score_dist']['0.6-0.7']}% |
| 0.5-0.6 | {analysis['score_dist']['0.5-0.6']} | {analysis['score_dist']['0.5-0.6']}% |
| 0.4-0.5 | {analysis['score_dist']['0.4-0.5']} | {analysis['score_dist']['0.4-0.5']}% |
| < 0.4 | {analysis['score_dist']['<0.4']} | {analysis['score_dist']['<0.4']}% |

## P1 Impact: Concern Node Retrieval

- **Questions with concern nodes in results**: {analysis['concern_node_hits']}
- **Concern hit rate**: {analysis['concern_hit_rate']:.1%}

## By Category

| Category | Avg Score | Count | v4 Score | Change |
|----------|-----------|-------|----------|--------|
"""

    v4_scores = {
        "architecture_structure": 0.55,
        "service_relationships": 0.56,
        "cross_cutting_concerns": 0.46,
        "data_flow_integration": 0.75,
        "business_logic_constraints": 0.52
    }

    for cat, stats in sorted(analysis['by_category'].items()):
        v4 = v4_scores.get(cat, 0.567)
        change = stats['avg'] - v4
        report += f"| {cat} | {stats['avg']:.3f} | {stats['count']} | {v4:.2f} | {change:+.3f} |\n"

    report += f"""
## Detailed Results

"""
    for r in results:
        concern_marker = " 🎯" if r.get('concern_nodes', 0) > 0 else ""
        report += f"- Q{r['id']} ({r['category']}): {r['score']:.3f} ({r['nodes']} nodes{concern_marker})\n"

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(report)

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {analysis['avg_score']:.3f} (v4: 0.567, change: {(analysis['avg_score'] - 0.567):+.3f})")
    print(f"Concern Node Hits: {analysis['concern_node_hits']}/{len(questions)}")
    print(f"Report: {OUTPUT_FILE}")

    return analysis

if __name__ == "__main__":
    run_test()
