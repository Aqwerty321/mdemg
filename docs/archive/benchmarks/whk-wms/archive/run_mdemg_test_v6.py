#!/usr/bin/env python3
"""
MDEMG Test v6 - P2 Track 4 (Configuration Boost) Validation

Tests retrieval quality after P2 Track 4 implementation:
- Config file detection during ingestion
- Config score boost (1.15x) in retrieval
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
OUTPUT_FILE = TEST_DIR / f"mdemg-test-v6-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
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
    config_hits = sum(1 for r in results if r.get('config_nodes', 0) > 0)

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
        "concern_hit_rate": concern_hits / len(results) if results else 0,
        "config_node_hits": config_hits,
        "config_hit_rate": config_hits / len(results) if results else 0
    }

def run_test():
    print("=" * 60)
    print("MDEMG TEST v6 - P2 TRACK 4 VALIDATION")
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
                "config_nodes": 0,
                "error": resp['error']
            })
            continue

        # Response format: {space_id, results, debug}
        nodes = resp.get('results', []) or resp.get('data', {}).get('results', [])

        # Calculate retrieval score - use TOP node score
        if nodes:
            top_score = nodes[0].get('score', 0) if nodes else 0
            # Count concern and config nodes in results
            concern_count = sum(1 for n in nodes if 'concern:' in n.get('name', ''))
            # Check for config nodes by looking at path patterns
            config_count = sum(1 for n in nodes if any(
                pattern in n.get('path', '').lower()
                for pattern in ['config', '.env', 'docker-compose', 'package.json', 'tsconfig']
            ))
        else:
            top_score = 0
            concern_count = 0
            config_count = 0

        results.append({
            "id": q['id'],
            "category": category,
            "question": qtext,
            "score": top_score,
            "nodes": len(nodes),
            "concern_nodes": concern_count,
            "config_nodes": config_count,
        })

        # Progress
        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            print(f"Progress: {i}/{len(questions)} ({rate:.1f} q/s) - Last score: {top_score:.3f}")

    total_time = time.time() - start_time

    # Analyze results
    analysis = analyze_results(results)

    # Generate report - compare against v5 baseline
    report = f"""# MDEMG Test v6 Results - P2 Track 4 (Config Boost) Validation

**Generated**: {datetime.now().isoformat()}
**Space**: {SPACE_ID}
**Questions**: {len(questions)}
**Duration**: {total_time:.1f}s

---

## Summary

| Metric | v6 (P2) | v5 (P1) | Change |
|--------|---------|---------|--------|
| Avg Score | {analysis['avg_score']:.3f} | 0.569 | {(analysis['avg_score'] - 0.569):+.3f} |
| Max Score | {analysis['max_score']:.3f} | 0.838 | {(analysis['max_score'] - 0.838):+.3f} |
| Min Score | {analysis['min_score']:.3f} | 0.345 | {(analysis['min_score'] - 0.345):+.3f} |

## Score Distribution

| Range | Count | Percentage |
|-------|-------|------------|
| > 0.7 | {analysis['score_dist']['>0.7']} | {analysis['score_dist']['>0.7']}% |
| 0.6-0.7 | {analysis['score_dist']['0.6-0.7']} | {analysis['score_dist']['0.6-0.7']}% |
| 0.5-0.6 | {analysis['score_dist']['0.5-0.6']} | {analysis['score_dist']['0.5-0.6']}% |
| 0.4-0.5 | {analysis['score_dist']['0.4-0.5']} | {analysis['score_dist']['0.4-0.5']}% |
| < 0.4 | {analysis['score_dist']['<0.4']} | {analysis['score_dist']['<0.4']}% |

## P2 Track 4 Impact: Config Node Retrieval

- **Questions with config nodes in results**: {analysis['config_node_hits']}
- **Config hit rate**: {analysis['config_hit_rate']:.1%}

## Concern Node Retrieval (P1)

- **Questions with concern nodes in results**: {analysis['concern_node_hits']}
- **Concern hit rate**: {analysis['concern_hit_rate']:.1%}

## By Category

| Category | Avg Score | Count | v5 Score | Change |
|----------|-----------|-------|----------|--------|
"""

    v5_scores = {
        "architecture_structure": 0.554,
        "service_relationships": 0.583,
        "cross_cutting_concerns": 0.560,
        "data_flow_integration": 0.561,
        "business_logic_constraints": 0.593
    }

    for cat, stats in sorted(analysis['by_category'].items()):
        v5 = v5_scores.get(cat, 0.569)
        change = stats['avg'] - v5
        report += f"| {cat} | {stats['avg']:.3f} | {stats['count']} | {v5:.3f} | {change:+.3f} |\n"

    report += f"""
## Detailed Results

"""
    for r in results:
        concern_marker = " 🎯" if r.get('concern_nodes', 0) > 0 else ""
        config_marker = " ⚙️" if r.get('config_nodes', 0) > 0 else ""
        report += f"- Q{r['id']} ({r['category']}): {r['score']:.3f} ({r['nodes']} nodes{concern_marker}{config_marker})\n"

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(report)

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {analysis['avg_score']:.3f} (v5: 0.569, change: {(analysis['avg_score'] - 0.569):+.3f})")
    print(f"Config Node Hits: {analysis['config_node_hits']}/{len(questions)}")
    print(f"Concern Node Hits: {analysis['concern_node_hits']}/{len(questions)}")
    print(f"Report: {OUTPUT_FILE}")

    return analysis

if __name__ == "__main__":
    run_test()
