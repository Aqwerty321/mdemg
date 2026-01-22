#!/usr/bin/env python3
"""
MDEMG Test v8 - Path-Boost and Comparison-Boost Scoring

Tests retrieval quality after scoring improvements:
- Path-boost: queries mentioning paths get boosted results for matching paths
- Comparison-boost: queries comparing modules get boosted results for comparison nodes
- Focus on architecture_structure category improvement
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
OUTPUT_FILE = TEST_DIR / f"mdemg-test-v8-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
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
    comparison_hits = sum(1 for r in results if r.get('comparison_nodes', 0) > 0)
    path_match_hits = sum(1 for r in results if r.get('path_match', False))

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
        "config_hit_rate": config_hits / len(results) if results else 0,
        "comparison_node_hits": comparison_hits,
        "comparison_hit_rate": comparison_hits / len(results) if results else 0,
        "path_match_hits": path_match_hits,
        "path_match_rate": path_match_hits / len(results) if results else 0
    }

def detect_query_type(question: str) -> dict:
    """Detect if query has path mentions or comparison structure"""
    question_lower = question.lower()

    # Path detection patterns
    has_path = any(pattern in question_lower for pattern in [
        'lib/', 'src/', 'cmd/', 'internal/', 'pkg/',
        'services/', 'modules/', 'components/', 'frontend/', 'backend/',
        '/graphql', '/api', '/config', '/utils'
    ])

    # Comparison detection patterns
    is_comparison = any(pattern in question_lower for pattern in [
        'difference between', 'vs ', 'versus', 'compared to',
        'both ', 'having both', 'why have', 'relationship between'
    ])

    return {
        "has_path": has_path,
        "is_comparison": is_comparison
    }

def run_test():
    print("=" * 60)
    print("MDEMG TEST v8 - PATH & COMPARISON BOOST SCORING")
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

    path_queries = 0
    comparison_queries = 0

    for i, q in enumerate(questions, 1):
        qtext = q['question']
        category = q['category']
        query_type = detect_query_type(qtext)

        if query_type['has_path']:
            path_queries += 1
        if query_type['is_comparison']:
            comparison_queries += 1

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
                "comparison_nodes": 0,
                "path_match": False,
                "query_has_path": query_type['has_path'],
                "query_is_comparison": query_type['is_comparison'],
                "error": resp['error']
            })
            continue

        # Response format: {space_id, results, debug} or {data: {...}}
        nodes = resp.get('results', []) or resp.get('data', {}).get('results', [])

        # Calculate retrieval score - use TOP node score
        if nodes:
            top_score = nodes[0].get('score', 0) if nodes else 0
            # Count special node types
            concern_count = sum(1 for n in nodes if 'concern:' in n.get('name', ''))
            comparison_count = sum(1 for n in nodes if 'comparison:' in n.get('name', ''))
            config_count = sum(1 for n in nodes if any(
                pattern in n.get('path', '').lower()
                for pattern in ['config', '.env', 'docker-compose', 'package.json', 'tsconfig']
            ))
            # Check if any result path matches a path mentioned in query
            path_match = False
            if query_type['has_path']:
                for n in nodes:
                    node_path = n.get('path', '').lower()
                    if any(p in node_path for p in ['lib/', 'src/', 'services/', 'modules/', 'components/']):
                        if any(p in qtext.lower() and p in node_path for p in ['graphql', 'api', 'sync', 'auth', 'config']):
                            path_match = True
                            break
        else:
            top_score = 0
            concern_count = 0
            config_count = 0
            comparison_count = 0
            path_match = False

        results.append({
            "id": q['id'],
            "category": category,
            "question": qtext,
            "score": top_score,
            "nodes": len(nodes),
            "concern_nodes": concern_count,
            "config_nodes": config_count,
            "comparison_nodes": comparison_count,
            "path_match": path_match,
            "query_has_path": query_type['has_path'],
            "query_is_comparison": query_type['is_comparison'],
        })

        # Progress
        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            print(f"Progress: {i}/{len(questions)} ({rate:.1f} q/s) - Last score: {top_score:.3f}")

    total_time = time.time() - start_time

    # Analyze results
    analysis = analyze_results(results)

    # Generate report - compare against v6 baseline
    report = f"""# MDEMG Test v8 Results - Path & Comparison Boost Scoring

**Generated**: {datetime.now().isoformat()}
**Space**: {SPACE_ID}
**Questions**: {len(questions)}
**Duration**: {total_time:.1f}s

---

## Summary

| Metric | v8 (Boost Scoring) | v6 (Baseline) | Change |
|--------|-------------------|---------------|--------|
| Avg Score | {analysis['avg_score']:.3f} | 0.560 | {(analysis['avg_score'] - 0.560):+.3f} ({((analysis['avg_score']/0.560)-1)*100:+.1f}%) |
| Max Score | {analysis['max_score']:.3f} | 0.824 | {(analysis['max_score'] - 0.824):+.3f} |
| Min Score | {analysis['min_score']:.3f} | 0.344 | {(analysis['min_score'] - 0.344):+.3f} |

## Query Analysis

| Type | Count | % of Total |
|------|-------|------------|
| Path-mentioning queries | {path_queries} | {path_queries/len(questions)*100:.1f}% |
| Comparison queries | {comparison_queries} | {comparison_queries/len(questions)*100:.1f}% |

## Score Distribution

| Range | Count | Percentage |
|-------|-------|------------|
| > 0.7 | {analysis['score_dist']['>0.7']} | {analysis['score_dist']['>0.7']}% |
| 0.6-0.7 | {analysis['score_dist']['0.6-0.7']} | {analysis['score_dist']['0.6-0.7']}% |
| 0.5-0.6 | {analysis['score_dist']['0.5-0.6']} | {analysis['score_dist']['0.5-0.6']}% |
| 0.4-0.5 | {analysis['score_dist']['0.4-0.5']} | {analysis['score_dist']['0.4-0.5']}% |
| < 0.4 | {analysis['score_dist']['<0.4']} | {analysis['score_dist']['<0.4']}% |

## Special Node Retrieval

| Node Type | Hits | Hit Rate |
|-----------|------|----------|
| Comparison nodes | {analysis['comparison_node_hits']} | {analysis['comparison_hit_rate']:.1%} |
| Concern nodes | {analysis['concern_node_hits']} | {analysis['concern_hit_rate']:.1%} |
| Config nodes | {analysis['config_node_hits']} | {analysis['config_hit_rate']:.1%} |
| Path matches | {analysis['path_match_hits']} | {analysis['path_match_rate']:.1%} |

## By Category

| Category | v8 Score | v6 Score | Change |
|----------|----------|----------|--------|
"""

    v6_scores = {
        "architecture_structure": 0.541,
        "service_relationships": 0.564,
        "cross_cutting_concerns": 0.568,
        "data_flow_integration": 0.554,
        "business_logic_constraints": 0.593
    }

    for cat, stats in sorted(analysis['by_category'].items()):
        v6 = v6_scores.get(cat, 0.560)
        change = stats['avg'] - v6
        pct_change = ((stats['avg'] / v6) - 1) * 100 if v6 > 0 else 0
        report += f"| {cat} | {stats['avg']:.3f} | {v6:.3f} | {change:+.3f} ({pct_change:+.1f}%) |\n"

    # Focus section on architecture_structure
    arch_results = [r for r in results if r['category'] == 'architecture_structure']
    arch_path_results = [r for r in arch_results if r['query_has_path']]

    report += f"""
## Architecture Structure Focus

Target category for improvement: **architecture_structure**

| Subset | Count | Avg Score |
|--------|-------|-----------|
| All architecture_structure | {len(arch_results)} | {sum(r['score'] for r in arch_results)/len(arch_results):.3f} |
| Path-mentioning only | {len(arch_path_results)} | {sum(r['score'] for r in arch_path_results)/len(arch_path_results) if arch_path_results else 0:.3f} |

### Low Scoring Architecture Questions (< 0.5)

"""
    low_arch = [r for r in arch_results if r['score'] < 0.5]
    for r in sorted(low_arch, key=lambda x: x['score']):
        markers = []
        if r['query_has_path']: markers.append("PATH")
        if r['query_is_comparison']: markers.append("COMP")
        marker_str = f" [{', '.join(markers)}]" if markers else ""
        report += f"- **{r['score']:.3f}**{marker_str}: {r['question'][:80]}...\n"

    report += f"""
## Detailed Results

"""
    for r in results:
        markers = []
        if r.get('concern_nodes', 0) > 0: markers.append("C")
        if r.get('config_nodes', 0) > 0: markers.append("Cfg")
        if r.get('comparison_nodes', 0) > 0: markers.append("Cmp")
        if r['query_has_path']: markers.append("P")
        marker_str = f" [{','.join(markers)}]" if markers else ""
        report += f"- Q{r['id']} ({r['category']}): {r['score']:.3f}{marker_str}\n"

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(report)

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {analysis['avg_score']:.3f} (v6: 0.560, change: {(analysis['avg_score'] - 0.560):+.3f})")
    print(f"\nBy Category:")
    for cat, stats in sorted(analysis['by_category'].items()):
        v6 = v6_scores.get(cat, 0.560)
        change = stats['avg'] - v6
        print(f"  {cat}: {stats['avg']:.3f} (v6: {v6:.3f}, change: {change:+.3f})")
    print(f"\nComparison Node Hits: {analysis['comparison_node_hits']}/{len(questions)}")
    print(f"Path Match Hits: {analysis['path_match_hits']}/{len(questions)}")
    print(f"Report: {OUTPUT_FILE}")

    return analysis

if __name__ == "__main__":
    run_test()
