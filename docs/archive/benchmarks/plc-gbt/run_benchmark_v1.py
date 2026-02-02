#!/usr/bin/env python3
"""
PLC-GBT Benchmark Test v1 - Tests MDEMG retrieval on plc-gbt codebase

Tests 60 questions across 6 categories:
- control_loop_architecture
- data_models_schema
- api_services
- configuration_infrastructure
- business_logic_workflows
- ui_ux

Token Consumption Tracking:
- Tracks estimated tokens for MDEMG retrieval (summaries returned)
- Compares to estimated baseline (grep full file content)
- Reports token efficiency ratio
"""

import json
import urllib.request
import time
from pathlib import Path
from datetime import datetime
from collections import defaultdict
import statistics

TEST_DIR = Path(__file__).parent
QUESTIONS_FILE = TEST_DIR / "test_questions_v1.json"
OUTPUT_FILE = TEST_DIR / f"benchmark-v1-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
MDEMG_ENDPOINT = "http://localhost:8090"
SPACE_ID = "plc-gbt"

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

def get_stats() -> dict:
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/v1/memory/stats?space_id={SPACE_ID}")
        with urllib.request.urlopen(req, timeout=5) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except:
        return {}

def estimate_tokens(text: str) -> int:
    """Estimate token count using ~4 chars per token heuristic"""
    if not text:
        return 0
    return len(text) // 4

def calculate_token_metrics(results: list, nodes_per_query: list) -> dict:
    """Calculate token consumption metrics for MDEMG vs baseline comparison"""

    # MDEMG token estimates (summaries + paths returned)
    mdemg_tokens_per_query = []
    for nodes in nodes_per_query:
        query_tokens = 0
        for node in nodes:
            # Count tokens in returned context
            query_tokens += estimate_tokens(node.get('name', ''))
            query_tokens += estimate_tokens(node.get('path', ''))
            query_tokens += estimate_tokens(node.get('summary', ''))
        mdemg_tokens_per_query.append(query_tokens)

    # Baseline estimate: assume grep returns 5 full files @ 500 lines avg, 40 chars/line
    # This is conservative - real baseline often reads many more files
    BASELINE_FILES_PER_QUERY = 5
    BASELINE_AVG_FILE_LINES = 500
    BASELINE_AVG_LINE_CHARS = 40
    baseline_tokens_per_query = (BASELINE_FILES_PER_QUERY * BASELINE_AVG_FILE_LINES * BASELINE_AVG_LINE_CHARS) // 4

    total_mdemg_tokens = sum(mdemg_tokens_per_query)
    total_baseline_tokens = baseline_tokens_per_query * len(results)

    return {
        "mdemg_tokens_total": total_mdemg_tokens,
        "mdemg_tokens_avg": total_mdemg_tokens / len(results) if results else 0,
        "mdemg_tokens_p95": sorted(mdemg_tokens_per_query)[int(len(mdemg_tokens_per_query) * 0.95)] if mdemg_tokens_per_query else 0,
        "baseline_tokens_total": total_baseline_tokens,
        "baseline_tokens_avg": baseline_tokens_per_query,
        "token_savings_ratio": total_baseline_tokens / total_mdemg_tokens if total_mdemg_tokens > 0 else 0,
        "token_savings_pct": (1 - total_mdemg_tokens / total_baseline_tokens) * 100 if total_baseline_tokens > 0 else 0
    }

def run_test():
    print("=" * 60)
    print("PLC-GBT BENCHMARK TEST v1")
    print("=" * 60)

    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/healthz")
        with urllib.request.urlopen(req, timeout=5) as resp:
            print(f"MDEMG Status: {resp.read().decode('utf-8')}")
    except Exception as e:
        print(f"ERROR: MDEMG not reachable: {e}")
        return

    # Get initial stats
    stats = get_stats()
    print(f"Space: {SPACE_ID}")
    print(f"Memory count: {stats.get('memory_count', 0)}")
    print(f"Memories by layer: {stats.get('memories_by_layer', {})}")
    print(f"Initial CO_ACTIVATED_WITH edges: {stats.get('learning_activity', {}).get('co_activated_edges', 0)}")

    questions = load_questions()
    print(f"Loaded {len(questions)} questions")
    print("-" * 60)

    results = []
    nodes_per_query = []  # Track nodes for token calculation
    start_time = time.time()
    total_rerank_latency = 0

    for i, q in enumerate(questions, 1):
        qtext = q['question']
        category = q['category']
        difficulty = q.get('difficulty', 'unknown')
        resp = query_mdemg(qtext)

        if "error" in resp:
            print(f"Q{i}: ERROR - {resp['error']}")
            results.append({"id": q['id'], "category": category, "difficulty": difficulty, "score": 0})
            nodes_per_query.append([])
            continue

        nodes = resp.get('results', [])
        nodes_per_query.append(nodes)  # Track for token calculation
        debug = resp.get('debug', {})

        top_score = nodes[0].get('score', 0) if nodes else 0
        rerank_latency = debug.get('rerank_latency_ms', 0)
        total_rerank_latency += rerank_latency

        # Count nodes with activation > 0.20
        activated_nodes = sum(1 for n in nodes if n.get('activation', 0) >= 0.20)
        layer1_nodes = sum(1 for n in nodes if n.get('layer', 0) == 1)

        results.append({
            "id": q['id'],
            "category": category,
            "difficulty": difficulty,
            "score": top_score,
            "rerank_latency": rerank_latency,
            "activated_nodes": activated_nodes,
            "layer1_nodes": layer1_nodes
        })

        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            print(f"Progress: {i}/{len(questions)} ({rate:.2f} q/s) - Score: {top_score:.3f}")

    total_time = time.time() - start_time

    # Get final stats
    final_stats = get_stats()
    final_edges = final_stats.get('learning_activity', {}).get('co_activated_edges', 0)
    initial_edges = stats.get('learning_activity', {}).get('co_activated_edges', 0)
    new_edges_total = final_edges - initial_edges

    # Analysis
    scores = [r['score'] for r in results]
    by_category = defaultdict(list)
    by_difficulty = defaultdict(list)
    for r in results:
        by_category[r['category']].append(r['score'])
        by_difficulty[r['difficulty']].append(r['score'])

    avg_score = sum(scores) / len(scores)
    avg_activated = sum(r.get('activated_nodes', 0) for r in results) / len(results)
    avg_layer1 = sum(r.get('layer1_nodes', 0) for r in results) / len(results)

    # Score distribution
    score_dist = {
        ">0.8": sum(1 for s in scores if s > 0.8),
        "0.7-0.8": sum(1 for s in scores if 0.7 <= s < 0.8),
        "0.6-0.7": sum(1 for s in scores if 0.6 <= s < 0.7),
        "0.5-0.6": sum(1 for s in scores if 0.5 <= s < 0.6),
        "0.4-0.5": sum(1 for s in scores if 0.4 <= s < 0.5),
        "<0.4": sum(1 for s in scores if s < 0.4),
    }

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {avg_score:.3f}")
    print(f"Duration: {total_time:.1f}s ({total_time/len(questions):.1f}s per query)")
    print(f"Avg Rerank Latency: {total_rerank_latency/len(questions):.0f}ms")
    print(f"Avg Activated Nodes: {avg_activated:.1f}/10")
    print(f"Avg Layer 1 Nodes in Results: {avg_layer1:.2f}/10")
    print(f"Learning Edges: {initial_edges} -> {final_edges} (+{new_edges_total})")
    print(f"\nScore Distribution:")
    for bucket, count in score_dist.items():
        pct = count / len(scores) * 100
        print(f"  {bucket}: {count} ({pct:.0f}%)")
    print(f"\nBy Category:")
    for cat, vals in sorted(by_category.items()):
        cat_avg = sum(vals) / len(vals)
        print(f"  {cat}: {cat_avg:.3f} (n={len(vals)})")
    print(f"\nBy Difficulty:")
    for diff, vals in sorted(by_difficulty.items()):
        diff_avg = sum(vals) / len(vals)
        print(f"  {diff}: {diff_avg:.3f} (n={len(vals)})")

    # Token consumption metrics
    token_metrics = calculate_token_metrics(results, nodes_per_query)
    print(f"\nToken Consumption:")
    print(f"  MDEMG Total: {token_metrics['mdemg_tokens_total']:,} tokens")
    print(f"  MDEMG Avg/Query: {token_metrics['mdemg_tokens_avg']:.0f} tokens")
    print(f"  MDEMG p95: {token_metrics['mdemg_tokens_p95']} tokens")
    print(f"  Baseline Estimate: {token_metrics['baseline_tokens_total']:,} tokens")
    print(f"  Token Savings: {token_metrics['token_savings_pct']:.1f}% ({token_metrics['token_savings_ratio']:.1f}x reduction)")

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(f"# PLC-GBT Benchmark Test v1 Results\n\n")
        f.write(f"**Date**: {datetime.now().isoformat()}\n")
        f.write(f"**Duration**: {total_time:.1f}s\n")
        f.write(f"**Codebase**: plc-gbt ({stats.get('memory_count', 0)} memories)\n\n")
        f.write(f"## Summary\n\n")
        f.write(f"| Metric | Value |\n")
        f.write(f"|--------|-------|\n")
        f.write(f"| Avg Score | **{avg_score:.3f}** |\n")
        f.write(f"| Max Score | {max(scores):.3f} |\n")
        f.write(f"| Min Score | {min(scores):.3f} |\n")
        f.write(f"| >0.7 Rate | {(score_dist['>0.8'] + score_dist['0.7-0.8']) / len(scores) * 100:.0f}% |\n")
        f.write(f"| Avg Rerank Latency | {total_rerank_latency/len(questions):.0f}ms |\n\n")
        f.write(f"## Learning Edge Statistics\n\n")
        f.write(f"| Metric | Value |\n")
        f.write(f"|--------|-------|\n")
        f.write(f"| Initial Edges | {initial_edges} |\n")
        f.write(f"| Final Edges | {final_edges} |\n")
        f.write(f"| New Edges Created | {new_edges_total} |\n")
        f.write(f"| Edges per Query | {new_edges_total/len(questions):.1f} |\n")
        f.write(f"| Avg Activated Nodes | {avg_activated:.1f}/10 |\n")
        f.write(f"| Avg Layer 1 Nodes | {avg_layer1:.2f}/10 |\n\n")
        f.write(f"## Score Distribution\n\n")
        f.write(f"| Range | Count | Percentage |\n")
        f.write(f"|-------|-------|------------|\n")
        for bucket, count in score_dist.items():
            pct = count / len(scores) * 100
            f.write(f"| {bucket} | {count} | {pct:.0f}% |\n")
        f.write(f"\n## By Category\n\n")
        f.write(f"| Category | Avg Score | Count |\n")
        f.write(f"|----------|-----------|-------|\n")
        for cat, vals in sorted(by_category.items()):
            cat_avg = sum(vals) / len(vals)
            f.write(f"| {cat} | {cat_avg:.3f} | {len(vals)} |\n")
        f.write(f"\n## By Difficulty\n\n")
        f.write(f"| Difficulty | Avg Score | Count |\n")
        f.write(f"|------------|-----------|-------|\n")
        for diff, vals in sorted(by_difficulty.items()):
            diff_avg = sum(vals) / len(vals)
            f.write(f"| {diff} | {diff_avg:.3f} | {len(vals)} |\n")
        f.write(f"\n## Comparison with whk-wms (v11)\n\n")
        f.write(f"| Metric | plc-gbt | whk-wms (v11) | Delta |\n")
        f.write(f"|--------|---------|---------------|-------|\n")
        whk_avg = 0.733
        f.write(f"| Avg Score | {avg_score:.3f} | {whk_avg:.3f} | {avg_score - whk_avg:+.3f} |\n")
        whk_gt7 = 75
        plc_gt7 = (score_dist['>0.8'] + score_dist['0.7-0.8']) / len(scores) * 100
        f.write(f"| >0.7 Rate | {plc_gt7:.0f}% | {whk_gt7}% | {plc_gt7 - whk_gt7:+.0f}% |\n")

    print(f"\nReport: {OUTPUT_FILE}")
    return {"avg": avg_score, "by_category": {c: sum(v)/len(v) for c,v in by_category.items()}}

if __name__ == "__main__":
    run_test()
