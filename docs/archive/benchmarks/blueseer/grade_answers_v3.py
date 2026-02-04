#!/usr/bin/env python3
"""
MDEMG Benchmark Grading Script v3.1
Updated for standardized v3 question schema.

Schema Fields Expected:
- expected_answer (string) - normalized from answer/golden_answer
- requires_files (list) - file paths needed to answer
- evidence (list) - [{file, line_start, line_end, snippet}]
- difficulty (easy/medium/hard)

Scoring:
- 70% evidence score (file:line citations)
- 15% semantic similarity
- 15% concept overlap
- Bonus for correct file citations
- Difficulty weighting for aggregate stats

Usage:
    python grade_answers_v3.py answers.jsonl questions_master.json grades.json
"""

import json
import re
import sys
import statistics
import math
from pathlib import Path
from typing import Dict, List, Set, Tuple, Optional
from collections import Counter


def tokenize(text: str) -> List[str]:
    """Tokenize text into lowercase words."""
    return re.findall(r'\b[a-zA-Z][a-zA-Z0-9_]*\b', text.lower())


def get_ngrams(tokens: List[str], n: int) -> Set[Tuple[str, ...]]:
    """Get n-grams from token list."""
    if len(tokens) < n:
        return set()
    return {tuple(tokens[i:i+n]) for i in range(len(tokens) - n + 1)}


def jaccard_similarity(set1: Set, set2: Set) -> float:
    """Compute Jaccard similarity between two sets."""
    if not set1 or not set2:
        return 0.0
    intersection = len(set1 & set2)
    union = len(set1 | set2)
    return intersection / union if union > 0 else 0.0


def extract_key_concepts(text: str) -> Set[str]:
    """Extract key technical concepts from text."""
    patterns = [
        r'\b[A-Z][a-zA-Z]*(?:Service|Module|Controller|Guard|Maint|Data|Browse|Control|Handler|Manager|Utils)\b',
        r'\b[a-z]+(?:Service|Module|Controller|Guard|Maint|Data|Browse|Control|Handler|Manager|Utils)\b',
        r'\b[A-Z_]{2,}(?:_[A-Z_]+)*\b',  # CONSTANTS
        r'\b[a-z]+(?:_[a-z]+)+\b',  # snake_case
        r'\b\d+(?:ms|s|kb|mb|gb|%)\b',  # Numbers with units
        r'\b(?:true|false|null|undefined)\b',
        r'\.\w+\(',  # Method calls
    ]
    concepts = set()
    for pattern in patterns:
        matches = re.findall(pattern, text, re.IGNORECASE)
        concepts.update(m.lower() for m in matches)
    return concepts


def compute_idf_weights(corpus: List[str]) -> Dict[str, float]:
    """Compute pseudo-IDF weights from a corpus of texts."""
    doc_freq = Counter()
    n_docs = len(corpus)
    for text in corpus:
        tokens = set(tokenize(text))
        for token in tokens:
            doc_freq[token] += 1
    idf = {}
    for token, df in doc_freq.items():
        idf[token] = math.log(n_docs / df) + 1 if df > 0 else 1
    return idf


def weighted_overlap(tokens1: List[str], tokens2: List[str], idf: Dict[str, float]) -> float:
    """Compute weighted token overlap using IDF weights."""
    set1, set2 = set(tokens1), set(tokens2)
    if not set1 or not set2:
        return 0.0
    common = set1 & set2
    common_weight = sum(idf.get(t, 1.0) for t in common)
    total_weight = sum(idf.get(t, 1.0) for t in set1)
    return common_weight / total_weight if total_weight > 0 else 0.0


def semantic_similarity(text1: str, text2: str, idf: Dict[str, float] = None) -> float:
    """Compute semantic similarity between two texts."""
    if not text1 or not text2:
        return 0.0
    tokens1, tokens2 = tokenize(text1), tokenize(text2)
    if not tokens1 or not tokens2:
        return 0.0

    unigram_sim = jaccard_similarity(set(tokens1), set(tokens2))
    bigram_sim = jaccard_similarity(get_ngrams(tokens1, 2), get_ngrams(tokens2, 2))
    trigram_sim = jaccard_similarity(get_ngrams(tokens1, 3), get_ngrams(tokens2, 3))

    concepts1, concepts2 = extract_key_concepts(text1), extract_key_concepts(text2)
    concept_sim = jaccard_similarity(concepts1, concepts2) if concepts1 and concepts2 else 0.0
    weighted_sim = weighted_overlap(tokens1, tokens2, idf) if idf else unigram_sim

    combined = (0.15 * unigram_sim + 0.20 * bigram_sim + 0.15 * trigram_sim +
                0.30 * concept_sim + 0.20 * weighted_sim)
    return min(combined * 1.5, 1.0)


def get_expected_answer(question: Dict) -> str:
    """Get expected answer from question, handling schema variations."""
    return (question.get('expected_answer') or
            question.get('golden_answer') or
            question.get('answer') or '')


def get_requires_files(question: Dict) -> List[str]:
    """Get required files from question, handling schema variations."""
    return (question.get('requires_files') or
            question.get('required_files') or [])


def extract_file_citations(answer_text: str) -> List[Dict]:
    """Extract file:line citations from answer text."""
    citations = []
    # Pattern: filename.java:123 or path/to/file.java:45
    pattern = r'([a-zA-Z0-9_/\-\.]+\.[a-zA-Z]+):(\d+)'
    for match in re.finditer(pattern, answer_text):
        citations.append({
            'file': match.group(1),
            'line': int(match.group(2))
        })
    return citations


def validate_evidence(answer: Dict, question: Dict) -> Dict:
    """Validate evidence citations in answer against question's expected evidence."""
    answer_text = answer.get('answer', '')
    cited_files = answer.get('file_line_refs', [])
    files_consulted = answer.get('files_consulted', [])

    expected_files = get_requires_files(question)
    expected_evidence = question.get('evidence', [])

    # Extract citations from answer text
    text_citations = extract_file_citations(answer_text)

    # Check if answer has file:line format citations
    has_file_line_refs = bool(cited_files) and any(':' in str(c) for c in cited_files)
    has_any_files = bool(files_consulted) or bool(cited_files) or bool(text_citations)

    # Check correct file citation
    correct_file_cited = False
    all_cited = [str(f).lower() for f in (cited_files + files_consulted)]
    all_cited.extend([c['file'].lower() for c in text_citations])

    for expected_file in expected_files:
        expected_basename = Path(expected_file).name.lower()
        for cited in all_cited:
            if expected_basename in cited:
                correct_file_cited = True
                break

    # Check evidence accuracy (do cited lines match expected?)
    evidence_accurate = False
    if expected_evidence and text_citations:
        for tc in text_citations:
            for ee in expected_evidence:
                ee_file = ee.get('file', '')
                if Path(ee_file).name.lower() in tc['file'].lower():
                    ee_start = ee.get('line_start', 0)
                    ee_end = ee.get('line_end', ee_start)
                    if ee_start <= tc['line'] <= ee_end + 5:  # Allow 5 line tolerance
                        evidence_accurate = True
                        break

    return {
        'has_file_line_refs': has_file_line_refs,
        'has_any_files': has_any_files,
        'correct_file_cited': correct_file_cited,
        'evidence_accurate': evidence_accurate,
        'citations_found': len(text_citations) + len(cited_files)
    }


def grade_answer(answer: Dict, question: Dict, idf: Dict[str, float]) -> Dict:
    """Grade a single answer against expected using v3 schema."""
    answer_text = answer.get('answer', '')
    expected_answer = get_expected_answer(question)
    difficulty = question.get('difficulty', 'hard')
    category = question.get('category', 'unknown')

    # Handle NOT_FOUND answers
    is_not_found = answer_text.upper().strip() == 'NOT_FOUND'
    expects_not_found = expected_answer.upper().strip() == 'NOT_FOUND'

    if is_not_found and expects_not_found:
        # Correct NOT_FOUND response (negative control)
        return {
            'id': answer.get('id'),
            'category': category,
            'difficulty': difficulty,
            'semantic_score': 1.0,
            'concept_score': 1.0,
            'evidence_score': 1.0,
            'score': 1.0,
            'correct_not_found': True,
            'evidence_found': True,
            'evidence_accurate': True,
            'correct_file_cited': True
        }
    elif is_not_found and not expects_not_found:
        # Wrong NOT_FOUND response
        return {
            'id': answer.get('id'),
            'category': category,
            'difficulty': difficulty,
            'semantic_score': 0.0,
            'concept_score': 0.0,
            'evidence_score': 0.0,
            'score': 0.0,
            'correct_not_found': False,
            'evidence_found': False,
            'evidence_accurate': False,
            'correct_file_cited': False
        }

    # Semantic similarity
    semantic_score = semantic_similarity(answer_text, expected_answer, idf)

    # Concept overlap
    expected_concepts = extract_key_concepts(expected_answer)
    answer_concepts = extract_key_concepts(answer_text)
    concept_score = jaccard_similarity(expected_concepts, answer_concepts)

    # Evidence validation
    ev = validate_evidence(answer, question)

    # Evidence score
    if ev['has_file_line_refs']:
        evidence_score = 1.0
    elif ev['has_any_files']:
        evidence_score = 0.5
    else:
        evidence_score = 0.0

    # Bonus for correct file
    file_bonus = 0.1 if ev['correct_file_cited'] else 0.0

    # Final score: 70% evidence, 15% semantic, 15% concept + bonus
    final_score = min(
        0.70 * evidence_score +
        0.15 * semantic_score +
        0.15 * concept_score +
        file_bonus,
        1.0
    )

    return {
        'id': answer.get('id'),
        'category': category,
        'difficulty': difficulty,
        'semantic_score': round(semantic_score, 3),
        'concept_score': round(concept_score, 3),
        'evidence_score': round(evidence_score, 3),
        'score': round(final_score, 3),
        'evidence_found': ev['has_file_line_refs'],
        'evidence_accurate': ev['evidence_accurate'],
        'correct_file_cited': ev['correct_file_cited'],
        'citations_count': ev['citations_found'],
        'expected_preview': expected_answer[:100] if expected_answer else '',
        'answer_preview': answer_text[:200] if answer_text else ''
    }


def grade_all(answers_file: Path, questions_file: Path, output_file: Path):
    """Grade all answers and compute aggregate metrics."""

    # Load questions
    with open(questions_file) as f:
        questions_data = json.load(f)
    questions_list = questions_data.get('questions', questions_data) if isinstance(questions_data, dict) else questions_data
    questions = {str(q['id']): q for q in questions_list}

    # Load answers (JSONL)
    answers = []
    with open(answers_file) as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    answers.append(json.loads(line))
                except json.JSONDecodeError as e:
                    print(f"WARNING: Parse error: {line[:50]}... - {e}")

    print(f"Loaded {len(questions)} questions, {len(answers)} answers")

    # Build IDF corpus
    corpus = [get_expected_answer(q) for q in questions_list if get_expected_answer(q)]
    idf = compute_idf_weights(corpus) if corpus else {}

    # Grade each answer
    grades = []
    for answer in answers:
        qid = str(answer.get('id'))
        if qid in questions:
            grade = grade_answer(answer, questions[qid], idf)
            grades.append(grade)
        else:
            print(f"WARNING: Question {qid} not in master set")

    if not grades:
        print("ERROR: No grades computed")
        return None

    # Aggregate metrics
    scores = [g['score'] for g in grades]

    # By difficulty
    by_difficulty = {}
    for diff in ['easy', 'medium', 'hard']:
        diff_scores = [g['score'] for g in grades if g['difficulty'] == diff]
        if diff_scores:
            by_difficulty[diff] = {
                'count': len(diff_scores),
                'mean': round(statistics.mean(diff_scores), 3),
                'std': round(statistics.stdev(diff_scores), 3) if len(diff_scores) > 1 else 0
            }

    # By category
    by_category = {}
    for cat in set(g['category'] for g in grades):
        cat_scores = [g['score'] for g in grades if g['category'] == cat]
        if cat_scores:
            by_category[cat] = {
                'count': len(cat_scores),
                'mean': round(statistics.mean(cat_scores), 3)
            }

    # Evidence quality tiers
    strong_evidence = sum(1 for g in grades if g['evidence_found'] and g['evidence_score'] == 1.0)
    weak_evidence = sum(1 for g in grades if g['evidence_score'] == 0.5)
    no_evidence = sum(1 for g in grades if g['evidence_score'] == 0.0)

    aggregate = {
        'total_questions': len(grades),
        'mean': round(statistics.mean(scores), 3),
        'std': round(statistics.stdev(scores), 3) if len(scores) > 1 else 0,
        'cv_pct': round(100 * statistics.stdev(scores) / statistics.mean(scores), 1) if len(scores) > 1 and statistics.mean(scores) > 0 else 0,
        'median': round(statistics.median(scores), 3),
        'min': round(min(scores), 3),
        'max': round(max(scores), 3),
        'p10': round(sorted(scores)[int(len(scores) * 0.1)], 3),
        'p90': round(sorted(scores)[int(len(scores) * 0.9)], 3),
        'high_score_rate': round(sum(1 for s in scores if s >= 0.7) / len(scores), 3),
        'evidence_rate': round(sum(1 for g in grades if g['evidence_found']) / len(grades), 3),
        'evidence_accuracy_rate': round(sum(1 for g in grades if g['evidence_accurate']) / len(grades), 3),
        'correct_file_rate': round(sum(1 for g in grades if g['correct_file_cited']) / len(grades), 3),
        'strong_evidence_count': strong_evidence,
        'weak_evidence_count': weak_evidence,
        'no_evidence_count': no_evidence,
        'by_difficulty': by_difficulty,
        'by_category': by_category
    }

    # Write output
    output = {'aggregate': aggregate, 'per_question': grades}
    with open(output_file, 'w') as f:
        json.dump(output, f, indent=2)

    print(f"\n=== GRADING COMPLETE: {output_file} ===")
    print(f"Total: {aggregate['total_questions']}")
    print(f"Mean: {aggregate['mean']} | Std: {aggregate['std']} | CV: {aggregate['cv_pct']}%")
    print(f"High score rate (>=0.7): {aggregate['high_score_rate']*100:.1f}%")
    print(f"\n=== EVIDENCE TIERS ===")
    print(f"Strong (file:line): {strong_evidence}/{len(grades)} ({strong_evidence/len(grades)*100:.1f}%)")
    print(f"Weak (files only):  {weak_evidence}/{len(grades)} ({weak_evidence/len(grades)*100:.1f}%)")
    print(f"None:               {no_evidence}/{len(grades)} ({no_evidence/len(grades)*100:.1f}%)")
    print(f"\n=== BY DIFFICULTY ===")
    for diff, stats in by_difficulty.items():
        print(f"  {diff}: n={stats['count']}, mean={stats['mean']}")

    return aggregate


if __name__ == '__main__':
    if len(sys.argv) != 4:
        print("Usage: python grade_answers_v3.py answers.jsonl questions_master.json grades.json")
        sys.exit(1)
    grade_all(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))
