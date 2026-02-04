#!/usr/bin/env python3
"""
20-Question MDEMG Benchmark Test
Tests MDEMG retrieval against whk-wms codebase questions.

Grading approach (same as grade_answers_v3.py):
- 70% weight on file:line evidence matching
- 15% weight on semantic similarity
- 15% weight on concept overlap
"""

import json
import re
import requests
import statistics
from pathlib import Path
from datetime import datetime
from typing import Dict, List, Set, Tuple, Optional


# --- Grading Functions (from grade_answers_v3.py) ---

def tokenize(text: str) -> List[str]:
    """Tokenize text into lowercase words, numbers, and version strings."""
    tokens = re.findall(
        r'\b\d+(?:\.\d+){2,}\b'       # version strings: 1.0.0, 2.3.1
        r'|\b0x[0-9a-fA-F]+\b'        # hex: 0xff, 0x1A2B
        r'|\b\d+\.\d+\b'              # decimals: 10.0, 0.2
        r'|\b\d+\b'                    # integers: 8192, 3600
        r'|\b[a-zA-Z][a-zA-Z0-9_]*\b' # words: ladder_logic_standard_pid
        , text
    )
    return [t.lower() for t in tokens]


def extract_key_concepts(text: str) -> Set[str]:
    """Extract key technical concepts from text."""
    patterns = [
        r'\b[A-Z][a-zA-Z]*(?:Service|Module|Controller|Guard|Maint|Data|Browse|Control|Handler|Manager|Utils|Config|Schema|Type|Level|Status|Request|Response|Store|Provider|Component)\b',
        r'\b[a-z]+(?:Service|Module|Controller|Guard|Maint|Data|Browse|Control|Handler|Manager|Utils|Config|Schema|Type|Level|Status|Request|Response|Store|Provider|Component)\b',
        r'\b[A-Z][a-z]+(?:[A-Z][a-z]+)+\b',
        r'\b[A-Z_]{2,}(?:_[A-Z_]+)*\b',
        r'\b[a-z]+(?:_[a-z0-9]+)+\b',
        r'\bv?\d+(?:\.\d+){2,}\b',
        r'\b\d+\.\d+\b',
        r'\b\d{4,}\b',
        r'\b\d+(?:\.\d+)?(?:ms|s|kb|mb|gb|%)\b',
        r'\b0x[0-9a-fA-F]+\b',
        r'\b(?:true|false|null|undefined|none)\b',
        r'\.\w+\(',
    ]
    concepts = set()
    for pattern in patterns:
        matches = re.findall(pattern, text, re.IGNORECASE)
        concepts.update(m.lower().strip() for m in matches if m.strip())
    return concepts


def recall(expected_set: Set, actual_set: Set) -> float:
    """What fraction of expected items appear in actual."""
    if not expected_set:
        return 0.0
    return len(expected_set & actual_set) / len(expected_set)


def jaccard_similarity(set1: Set, set2: Set) -> float:
    """Compute Jaccard similarity between two sets."""
    if not set1 or not set2:
        return 0.0
    intersection = len(set1 & set2)
    union = len(set1 | set2)
    return intersection / union if union > 0 else 0.0


def adaptive_similarity(expected_set: Set, actual_set: Set, short_threshold: int = 10) -> float:
    """Use recall when expected is short, blended Jaccard+recall otherwise."""
    if not expected_set or not actual_set:
        return 0.0
    r = recall(expected_set, actual_set)
    j = jaccard_similarity(expected_set, actual_set)
    if len(expected_set) <= short_threshold:
        return 0.8 * r + 0.2 * j
    return 0.5 * r + 0.5 * j


def semantic_similarity(text1: str, text2: str) -> float:
    """Compute semantic similarity between two texts."""
    if not text1 or not text2:
        return 0.0
    tokens1, tokens2 = tokenize(text1), tokenize(text2)
    if not tokens1 or not tokens2:
        return 0.0

    set1, set2 = set(tokens1), set(tokens2)

    unigram_sim = adaptive_similarity(set2, set1)

    # Get n-grams
    def get_ngrams(tokens, n):
        if len(tokens) < n:
            return set()
        return {tuple(tokens[i:i+n]) for i in range(len(tokens) - n + 1)}

    bigram_sim = adaptive_similarity(get_ngrams(tokens2, 2), get_ngrams(tokens1, 2))
    trigram_sim = adaptive_similarity(get_ngrams(tokens2, 3), get_ngrams(tokens1, 3))

    concepts1, concepts2 = extract_key_concepts(text1), extract_key_concepts(text2)
    concept_sim = adaptive_similarity(concepts2, concepts1) if concepts1 and concepts2 else 0.0

    combined = (0.15 * unigram_sim + 0.20 * bigram_sim + 0.15 * trigram_sim +
                0.30 * concept_sim + 0.20 * unigram_sim)
    return min(combined * 1.5, 1.0)


def extract_file_refs_from_results(results: List[Dict]) -> List[str]:
    """Extract file:line references from MDEMG results."""
    refs = []
    for r in results:
        path = r.get('path', '')
        if not path:
            continue
        # Check if path has symbol suffix like file.ts#ClassName
        if '#' in path:
            file_part = path.split('#')[0]
        else:
            file_part = path

        # Look for line number in evidence
        evidence = r.get('evidence', [])
        if evidence:
            for ev in evidence:
                line = ev.get('line_number', 0)
                if line > 0:
                    refs.append(f"{file_part}:{line}")
        else:
            # No evidence, use file path only
            refs.append(file_part)
    return refs


def check_file_match(expected_files: List[str], retrieved_refs: List[str]) -> Tuple[bool, bool]:
    """Check if retrieved files match expected files.
    Returns (correct_file_cited, has_line_numbers)
    """
    if not expected_files or not retrieved_refs:
        return False, False

    correct_file = False
    has_lines = False

    for expected in expected_files:
        expected_basename = Path(expected).name.lower()
        expected_path = expected.lower()

        for ref in retrieved_refs:
            ref_lower = ref.lower()
            # Check for line numbers
            if ':' in ref and any(c.isdigit() for c in ref.split(':')[-1]):
                has_lines = True
            # Check file match
            if expected_basename in ref_lower or expected_path in ref_lower:
                correct_file = True
                break
        if correct_file:
            break

    return correct_file, has_lines


def synthesize_answer(question: str, results: List[Dict]) -> str:
    """Synthesize an answer from MDEMG retrieval results."""
    if not results:
        return "NOT_FOUND"

    # Build context from top results
    context_parts = []
    for i, r in enumerate(results[:5]):
        path = r.get('path', '')
        name = r.get('name', '')
        summary = r.get('summary', '')
        score = r.get('score', 0)

        evidence_info = ""
        if r.get('evidence'):
            ev_lines = [f"{e['symbol_name']}:{e.get('line_number',0)}" for e in r['evidence'][:3]]
            evidence_info = f" [evidence: {', '.join(ev_lines)}]"

        context_parts.append(f"[{i+1}] {path} ({score:.3f}){evidence_info}: {summary}")

    # Simple answer synthesis based on retrieved context
    answer = f"Based on retrieved context:\n" + "\n".join(context_parts)

    # Add file references
    refs = extract_file_refs_from_results(results)
    if refs:
        answer += f"\n\nFile references: {', '.join(refs[:5])}"

    return answer


def call_mdemg_retrieve(question: str, space_id: str = "whk-wms", top_k: int = 10) -> Dict:
    """Call MDEMG retrieve API."""
    url = "http://localhost:8090/v1/memory/retrieve"
    payload = {
        "space_id": space_id,
        "query_text": question,
        "top_k": top_k,
        "include_evidence": True,
        "jiminy_enabled": False
    }

    try:
        resp = requests.post(url, json=payload, timeout=30)
        resp.raise_for_status()
        return resp.json()
    except Exception as e:
        return {"error": str(e), "results": []}


def grade_single(question: Dict, retrieval_response: Dict) -> Dict:
    """Grade a single question based on MDEMG retrieval."""
    qid = question.get('id')
    expected_answer = question.get('answer', '')
    required_files = question.get('required_files', [])

    results = retrieval_response.get('results', [])
    evidence_metrics = retrieval_response.get('evidence_metrics', {})

    # Synthesize answer from results
    synthesized = synthesize_answer(question['question'], results)

    # Extract file references
    file_refs = extract_file_refs_from_results(results)

    # Check file matching
    correct_file, has_lines = check_file_match(required_files, file_refs)

    # Evidence score (70% weight)
    if has_lines:
        evidence_score = 1.0
    elif file_refs:
        evidence_score = 0.5
    else:
        evidence_score = 0.0

    # Semantic similarity (15% weight)
    sem_score = semantic_similarity(synthesized, expected_answer)

    # Concept overlap (15% weight)
    expected_concepts = extract_key_concepts(expected_answer)
    answer_concepts = extract_key_concepts(synthesized)
    concept_score = adaptive_similarity(expected_concepts, answer_concepts)

    # File bonus
    file_bonus = 0.1 if correct_file else 0.0

    # Final score
    final_score = min(
        0.70 * evidence_score +
        0.15 * sem_score +
        0.15 * concept_score +
        file_bonus,
        1.0
    )

    return {
        'id': qid,
        'category': question.get('category', 'unknown'),
        'complexity': question.get('complexity', 'unknown'),
        'score': round(final_score, 3),
        'evidence_score': round(evidence_score, 3),
        'semantic_score': round(sem_score, 3),
        'concept_score': round(concept_score, 3),
        'correct_file_cited': correct_file,
        'has_line_numbers': has_lines,
        'results_count': len(results),
        'file_refs_found': file_refs[:5],
        'evidence_compliance': evidence_metrics.get('compliance_rate', 0),
        'question_preview': question['question'][:80],
        'synthesized_preview': synthesized[:200]
    }


def run_benchmark(questions_file: str, num_questions: int = 20):
    """Run benchmark on first N questions."""
    print(f"=== MDEMG 20-Question Benchmark ===")
    print(f"Timestamp: {datetime.now().isoformat()}")
    print(f"Questions file: {questions_file}")
    print(f"Number of questions: {num_questions}\n")

    # Load questions
    with open(questions_file) as f:
        data = json.load(f)

    questions = data.get('questions', data)[:num_questions]
    print(f"Loaded {len(questions)} questions\n")

    grades = []
    sample_refs = []

    for i, q in enumerate(questions):
        qid = q.get('id')
        print(f"[{i+1}/{num_questions}] Q{qid}: {q['question'][:60]}...")

        # Call MDEMG
        response = call_mdemg_retrieve(q['question'])

        if response.get('error'):
            print(f"  ERROR: {response['error']}")
            continue

        # Grade
        grade = grade_single(q, response)
        grades.append(grade)

        # Collect sample refs
        if grade['file_refs_found']:
            sample_refs.extend(grade['file_refs_found'][:2])

        print(f"  Score: {grade['score']:.3f} | Evidence: {grade['evidence_score']:.1f} | Files: {grade['correct_file_cited']}")

    # Aggregate metrics
    if not grades:
        print("\nERROR: No grades computed")
        return

    scores = [g['score'] for g in grades]
    evidence_scores = [g['evidence_score'] for g in grades]

    # Evidence coverage (% with non-zero line numbers)
    with_lines = sum(1 for g in grades if g['has_line_numbers'])
    with_files = sum(1 for g in grades if g['evidence_score'] > 0)
    correct_files = sum(1 for g in grades if g['correct_file_cited'])

    print("\n" + "="*60)
    print("=== BENCHMARK SUMMARY ===")
    print("="*60)
    print(f"Questions Answered: {len(grades)}/{num_questions}")
    print(f"Average Score: {statistics.mean(scores):.3f}")
    print(f"Median Score: {statistics.median(scores):.3f}")
    print(f"Score Range: {min(scores):.3f} - {max(scores):.3f}")
    if len(scores) > 1:
        print(f"Std Dev: {statistics.stdev(scores):.3f}")

    print(f"\n=== EVIDENCE METRICS ===")
    print(f"Results with file:line refs: {with_lines}/{len(grades)} ({100*with_lines/len(grades):.1f}%)")
    print(f"Results with any files: {with_files}/{len(grades)} ({100*with_files/len(grades):.1f}%)")
    print(f"Correct file cited: {correct_files}/{len(grades)} ({100*correct_files/len(grades):.1f}%)")
    print(f"Avg Evidence Score: {statistics.mean(evidence_scores):.3f}")

    print(f"\n=== SAMPLE FILE:LINE REFERENCES ===")
    unique_refs = list(dict.fromkeys(sample_refs))[:10]
    for ref in unique_refs:
        print(f"  - {ref}")

    print(f"\n=== SCORE DISTRIBUTION ===")
    high = sum(1 for s in scores if s >= 0.7)
    med = sum(1 for s in scores if 0.4 <= s < 0.7)
    low = sum(1 for s in scores if s < 0.4)
    print(f"High (>=0.7): {high} ({100*high/len(scores):.1f}%)")
    print(f"Medium (0.4-0.7): {med} ({100*med/len(scores):.1f}%)")
    print(f"Low (<0.4): {low} ({100*low/len(scores):.1f}%)")

    # By category
    print(f"\n=== BY CATEGORY ===")
    categories = {}
    for g in grades:
        cat = g['category']
        if cat not in categories:
            categories[cat] = []
        categories[cat].append(g['score'])

    for cat, cat_scores in sorted(categories.items()):
        print(f"  {cat}: n={len(cat_scores)}, mean={statistics.mean(cat_scores):.3f}")

    # Save results
    output_file = Path(questions_file).parent / f"benchmark_20q_results_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
    output = {
        'metadata': {
            'timestamp': datetime.now().isoformat(),
            'questions_file': questions_file,
            'num_questions': num_questions,
            'space_id': 'whk-wms'
        },
        'aggregate': {
            'total': len(grades),
            'mean_score': round(statistics.mean(scores), 3),
            'median_score': round(statistics.median(scores), 3),
            'std_score': round(statistics.stdev(scores), 3) if len(scores) > 1 else 0,
            'with_file_line_refs_pct': round(100*with_lines/len(grades), 1),
            'with_any_files_pct': round(100*with_files/len(grades), 1),
            'correct_file_pct': round(100*correct_files/len(grades), 1),
            'high_score_rate': round(high/len(grades), 3),
            'by_category': {cat: round(statistics.mean(s), 3) for cat, s in categories.items()}
        },
        'grades': grades
    }

    with open(output_file, 'w') as f:
        json.dump(output, f, indent=2)

    print(f"\nResults saved to: {output_file}")


if __name__ == '__main__':
    questions_file = '/Users/reh3376/mdemg/docs/tests/whk-wms/test_questions_120.json'
    run_benchmark(questions_file, num_questions=20)
