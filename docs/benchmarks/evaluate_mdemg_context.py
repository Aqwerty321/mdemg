#!/usr/bin/env python3
"""
MDEMG Contextual Guidance Evaluator

Evaluates MDEMG's ability to provide useful contextual guidance,
focusing on metrics relevant to its purpose:
1. Response rate - Does MDEMG return results?
2. Path relevance - Are returned files in the right area of the codebase?
3. Summary quality - Do summaries provide useful context?
4. Layer diversity - Does MDEMG surface both L0 files and L1/L2 concepts?
"""

import json
import os
import sys
import urllib.request
import urllib.error
from pathlib import Path
from datetime import datetime
from typing import Dict, List, Tuple
from collections import defaultdict


def call_mdemg_retrieve(endpoint: str, space_id: str, query: str, top_k: int = 10) -> Dict:
    """Call MDEMG retrieval API."""
    url = f"{endpoint}/v1/memory/retrieve"
    data = {
        "space_id": space_id,
        "query_text": query,
        "top_k": top_k
    }

    req = urllib.request.Request(
        url,
        data=json.dumps(data).encode('utf-8'),
        headers={'Content-Type': 'application/json'},
        method='POST'
    )

    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except Exception as e:
        return {"error": str(e), "results": []}


def extract_path_components(path: str) -> List[str]:
    """Extract meaningful path components for area matching."""
    parts = path.replace("\\", "/").split("/")
    # Filter out common non-informative parts
    skip = {'src', 'lib', 'app', 'main', 'index', 'node_modules', '.git'}
    return [p for p in parts if p and p not in skip and not p.startswith('.')]


def calculate_path_overlap(returned_path: str, expected_paths: List[str]) -> float:
    """Calculate how much the returned path overlaps with expected areas."""
    if not expected_paths:
        return 0.0

    returned_parts = set(extract_path_components(returned_path))
    if not returned_parts:
        return 0.0

    max_overlap = 0.0
    for expected in expected_paths:
        expected_parts = set(extract_path_components(expected))
        if expected_parts:
            # Jaccard similarity
            intersection = len(returned_parts & expected_parts)
            union = len(returned_parts | expected_parts)
            overlap = intersection / union if union > 0 else 0.0
            max_overlap = max(max_overlap, overlap)

    return max_overlap


def evaluate_summary_quality(summary: str) -> Dict[str, float]:
    """Evaluate summary quality metrics."""
    if not summary:
        return {"length": 0, "specificity": 0, "has_methods": 0, "has_purpose": 0}

    # Length score (prefer 50-200 chars)
    length = len(summary)
    length_score = min(1.0, length / 100) if length < 200 else max(0.5, 1.0 - (length - 200) / 400)

    # Specificity - presence of specific terms vs generic
    generic_terms = {'handles', 'manages', 'processes', 'data', 'operations', 'various', 'module', 'component'}
    specific_indicators = {'method', 'function', 'class', 'interface', 'service', 'controller', 'repository'}

    words = summary.lower().split()
    generic_count = sum(1 for w in words if w in generic_terms)
    specific_count = sum(1 for w in words if w in specific_indicators)
    specificity = max(0, 1.0 - (generic_count * 0.1)) * min(1.0, 0.5 + specific_count * 0.2)

    # Method/function names present (camelCase or snake_case patterns)
    import re
    has_methods = 1.0 if re.search(r'[a-z]+[A-Z][a-z]+|[a-z]+_[a-z]+', summary) else 0.0

    # Purpose clarity (contains verbs suggesting action)
    purpose_verbs = {'implements', 'provides', 'handles', 'processes', 'validates', 'manages', 'creates', 'returns'}
    has_purpose = 1.0 if any(v in summary.lower() for v in purpose_verbs) else 0.0

    return {
        "length": length_score,
        "specificity": specificity,
        "has_methods": has_methods,
        "has_purpose": has_purpose
    }


def evaluate_question(
    question: Dict,
    mdemg_response: Dict,
    codebase_path: str
) -> Dict:
    """Evaluate MDEMG response for a single question."""
    results = mdemg_response.get('results', [])

    eval_result = {
        "question_id": question.get('id', 'unknown'),
        "question": question.get('question', ''),
        "category": question.get('category', 'unknown'),
        "results_count": len(results),
        "has_results": len(results) > 0,
    }

    if not results:
        eval_result.update({
            "path_relevance": 0.0,
            "summary_quality": 0.0,
            "layer_diversity": 0.0,
            "top_score": 0.0,
            "score_spread": 0.0,
            "exact_match_count": 0,
            "has_exact_match": False,
        })
        return eval_result

    # Expected sources from question (if available)
    expected_sources = question.get('sources', question.get('required_files', []))
    if isinstance(expected_sources, str):
        expected_sources = [expected_sources]

    # Path relevance - how well do returned paths match expected area
    path_relevances = []
    for r in results[:5]:  # Top 5
        path = r.get('path', '')
        if expected_sources:
            relevance = calculate_path_overlap(path, expected_sources)
        else:
            # If no expected sources, check if path exists in codebase
            full_path = os.path.join(codebase_path, path.lstrip('/'))
            relevance = 1.0 if os.path.exists(full_path) else 0.5
        path_relevances.append(relevance)

    eval_result["path_relevance"] = sum(path_relevances) / len(path_relevances) if path_relevances else 0.0

    # Summary quality
    summary_scores = []
    for r in results[:5]:
        summary = r.get('summary', '')
        quality = evaluate_summary_quality(summary)
        avg_quality = sum(quality.values()) / len(quality)
        summary_scores.append(avg_quality)

    eval_result["summary_quality"] = sum(summary_scores) / len(summary_scores) if summary_scores else 0.0

    # Layer diversity - presence of L0 (files) and L1+ (concepts)
    layers = [r.get('layer', 0) for r in results]
    has_l0 = any(l == 0 for l in layers)
    has_l1_plus = any(l > 0 for l in layers)
    eval_result["layer_diversity"] = (0.5 if has_l0 else 0.0) + (0.5 if has_l1_plus else 0.0)

    # Score metrics
    scores = [r.get('score', 0) for r in results]
    eval_result["top_score"] = max(scores) if scores else 0.0
    eval_result["score_spread"] = max(scores) - min(scores) if len(scores) > 1 else 0.0

    # Exact match (for reference, not primary metric)
    returned_paths = [r.get('path', '').lower() for r in results]
    exact_matches = sum(1 for exp in expected_sources if any(exp.lower() in rp for rp in returned_paths))
    eval_result["exact_match_count"] = exact_matches
    eval_result["has_exact_match"] = exact_matches > 0

    return eval_result


def run_evaluation(
    questions_file: str,
    codebase_path: str,
    space_id: str,
    endpoint: str = "http://localhost:9999",
    limit: int = None
) -> Dict:
    """Run full evaluation on question set."""

    # Load questions
    with open(questions_file) as f:
        data = json.load(f)

    # Handle both flat list and nested structure
    if isinstance(data, dict) and 'questions' in data:
        questions = data['questions']
    elif isinstance(data, list):
        questions = data
    else:
        raise ValueError(f"Unknown questions file format")

    if limit:
        questions = questions[:limit]

    print(f"Evaluating {len(questions)} questions against MDEMG space '{space_id}'")
    print(f"Codebase: {codebase_path}")
    print("-" * 60)

    results = []
    by_category = defaultdict(list)

    for i, q in enumerate(questions):
        qid = q.get('id', f'Q{i+1}')
        print(f"[{i+1}/{len(questions)}] {qid}: {q.get('question', '')[:60]}...")

        # Call MDEMG
        mdemg_response = call_mdemg_retrieve(endpoint, space_id, q.get('question', ''))

        # Evaluate
        eval_result = evaluate_question(q, mdemg_response, codebase_path)
        results.append(eval_result)
        by_category[eval_result['category']].append(eval_result)

        # Brief status
        status = "✓" if eval_result['has_results'] else "✗"
        print(f"  {status} {eval_result['results_count']} results, path_rel={eval_result['path_relevance']:.2f}, summary={eval_result['summary_quality']:.2f}")

    # Aggregate metrics
    aggregate = {
        "total_questions": len(results),
        "response_rate": sum(1 for r in results if r['has_results']) / len(results),
        "avg_results_count": sum(r['results_count'] for r in results) / len(results),
        "avg_path_relevance": sum(r['path_relevance'] for r in results) / len(results),
        "avg_summary_quality": sum(r['summary_quality'] for r in results) / len(results),
        "avg_layer_diversity": sum(r['layer_diversity'] for r in results) / len(results),
        "avg_top_score": sum(r['top_score'] for r in results) / len(results),
        "exact_match_rate": sum(1 for r in results if r['has_exact_match']) / len(results),
    }

    # By category breakdown
    category_stats = {}
    for cat, cat_results in by_category.items():
        category_stats[cat] = {
            "count": len(cat_results),
            "response_rate": sum(1 for r in cat_results if r['has_results']) / len(cat_results),
            "avg_path_relevance": sum(r['path_relevance'] for r in cat_results) / len(cat_results),
            "avg_summary_quality": sum(r['summary_quality'] for r in cat_results) / len(cat_results),
        }

    return {
        "aggregate": aggregate,
        "by_category": category_stats,
        "individual_results": results,
        "evaluated_at": datetime.now().isoformat(),
        "space_id": space_id,
        "questions_file": questions_file,
    }


def print_report(evaluation: Dict):
    """Print human-readable evaluation report."""
    agg = evaluation['aggregate']

    print("\n" + "=" * 60)
    print("MDEMG CONTEXTUAL GUIDANCE EVALUATION REPORT")
    print("=" * 60)

    print(f"\nSpace: {evaluation['space_id']}")
    print(f"Questions: {agg['total_questions']}")
    print(f"Evaluated: {evaluation['evaluated_at']}")

    print("\n--- CORE METRICS (MDEMG Purpose-Aligned) ---")
    print(f"Response Rate:      {agg['response_rate']*100:.1f}%  (MDEMG returns results)")
    print(f"Path Relevance:     {agg['avg_path_relevance']*100:.1f}%  (files in right area)")
    print(f"Summary Quality:    {agg['avg_summary_quality']*100:.1f}%  (useful context)")
    print(f"Layer Diversity:    {agg['avg_layer_diversity']*100:.1f}%  (L0 + L1/L2 concepts)")
    print(f"Avg Top Score:      {agg['avg_top_score']:.3f}  (retrieval confidence)")

    print("\n--- REFERENCE METRICS (Less Critical for MDEMG) ---")
    print(f"Exact Match Rate:   {agg['exact_match_rate']*100:.1f}%  (exact file found)")
    print(f"Avg Results/Query:  {agg['avg_results_count']:.1f}")

    print("\n--- BY CATEGORY ---")
    for cat, stats in sorted(evaluation['by_category'].items()):
        print(f"  {cat}: {stats['count']} questions, "
              f"resp={stats['response_rate']*100:.0f}%, "
              f"path={stats['avg_path_relevance']*100:.0f}%, "
              f"summary={stats['avg_summary_quality']*100:.0f}%")

    print("\n" + "=" * 60)

    # Overall assessment
    context_score = (agg['response_rate'] * 0.3 +
                     agg['avg_path_relevance'] * 0.3 +
                     agg['avg_summary_quality'] * 0.25 +
                     agg['avg_layer_diversity'] * 0.15)

    print(f"\nOVERALL CONTEXTUAL GUIDANCE SCORE: {context_score*100:.1f}%")

    if context_score >= 0.7:
        print("Assessment: GOOD - MDEMG provides useful contextual guidance")
    elif context_score >= 0.5:
        print("Assessment: MODERATE - MDEMG provides some useful context")
    else:
        print("Assessment: NEEDS IMPROVEMENT - Context quality is low")


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="MDEMG Contextual Guidance Evaluator")
    parser.add_argument("--questions", required=True, help="Questions JSON file")
    parser.add_argument("--codebase", required=True, help="Path to codebase")
    parser.add_argument("--space-id", required=True, help="MDEMG space ID")
    parser.add_argument("--endpoint", default="http://localhost:9999", help="MDEMG API endpoint")
    parser.add_argument("--limit", type=int, help="Limit number of questions")
    parser.add_argument("--output", help="Output JSON file")

    args = parser.parse_args()

    evaluation = run_evaluation(
        questions_file=args.questions,
        codebase_path=args.codebase,
        space_id=args.space_id,
        endpoint=args.endpoint,
        limit=args.limit
    )

    print_report(evaluation)

    if args.output:
        with open(args.output, 'w') as f:
            json.dump(evaluation, f, indent=2)
        print(f"\nFull results saved to: {args.output}")
