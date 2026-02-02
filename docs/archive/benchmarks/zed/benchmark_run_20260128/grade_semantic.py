#!/usr/bin/env python3
"""
Semantic-focused grading script for MDEMG benchmarks.
Adjusts weights to focus on answer quality rather than citation format.

Scoring:
- 50% semantic similarity (n-gram overlap, weighted tokens)
- 30% concept overlap (key technical terms)
- 20% evidence bonus (file references if present)

Usage:
    python grade_semantic.py answers.jsonl questions_master.json grades.json
"""

import json
import re
import sys
import statistics
import math
from pathlib import Path
from typing import Dict, List, Set, Tuple
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
        r'\b[A-Z][a-zA-Z]*(?:Id|Map|Tree|Buffer|Store|Manager|Handler|Context|State)\b',
        r'\b[A-Z][a-zA-Z]+\b',  # PascalCase identifiers
        r'\b[a-z]+(?:_[a-z]+)+\b',  # snake_case
        r'\bO\([^)]+\)',  # Big-O notation
    ]
    concepts = set()
    for pattern in patterns:
        matches = re.findall(pattern, text)
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

    # Unigram, bigram, trigram similarity
    unigram_sim = jaccard_similarity(set(tokens1), set(tokens2))
    bigram_sim = jaccard_similarity(get_ngrams(tokens1, 2), get_ngrams(tokens2, 2))
    trigram_sim = jaccard_similarity(get_ngrams(tokens1, 3), get_ngrams(tokens2, 3))

    # IDF-weighted overlap
    weighted_sim = weighted_overlap(tokens1, tokens2, idf) if idf else unigram_sim

    # Combined with higher weight on meaningful overlap
    combined = (0.25 * unigram_sim + 0.30 * bigram_sim + 0.20 * trigram_sim + 0.25 * weighted_sim)
    return min(combined * 1.8, 1.0)  # Scale up to use more of 0-1 range


def get_expected_answer(question: Dict) -> str:
    """Get expected answer from question."""
    return (question.get('expected_answer') or
            question.get('golden_answer') or
            question.get('answer') or '')


def has_file_references(answer: Dict) -> bool:
    """Check if answer has any file references."""
    # Check file_line_refs field
    refs = answer.get('file_line_refs', [])
    if refs and len(refs) > 0:
        return True

    # Check answer text for file patterns
    answer_text = answer.get('answer', '')
    # Look for .rs, .py, .ts, .go, .java files
    if re.search(r'\b[a-zA-Z_][a-zA-Z0-9_]*\.(rs|py|ts|go|java|js|cpp|c)\b', answer_text):
        return True
    # Look for crates/ or src/ paths
    if re.search(r'\b(crates|src)/[a-zA-Z0-9_/]+', answer_text):
        return True

    return False


def grade_answer(answer: Dict, question: Dict, idf: Dict[str, float]) -> Dict:
    """Grade a single answer with semantic-focused scoring."""
    answer_text = answer.get('answer', '')
    expected_answer = get_expected_answer(question)
    difficulty = question.get('difficulty', 'hard')
    category = question.get('category', 'unknown')

    # Handle NOT_FOUND answers
    is_not_found = answer_text.upper().strip() == 'NOT_FOUND'
    expects_not_found = expected_answer.upper().strip() == 'NOT_FOUND'

    # Negative control - check for correct identification
    if category == 'negative_control':
        # Check if answer correctly identifies non-existence
        negative_indicators = ['not exist', 'doesn\'t exist', 'does not exist', 'no such',
                               'not found', 'non-existent', 'fictional', 'not a real',
                               'not part of', 'not included', 'not present']
        correctly_negative = any(ind in answer_text.lower() for ind in negative_indicators)
        if correctly_negative or is_not_found:
            return {
                'id': answer.get('id'),
                'category': category,
                'difficulty': difficulty,
                'semantic_score': 1.0,
                'concept_score': 1.0,
                'evidence_score': 1.0,
                'score': 1.0,
                'correct_negative': True
            }

    if is_not_found and expects_not_found:
        return {
            'id': answer.get('id'),
            'category': category,
            'difficulty': difficulty,
            'semantic_score': 1.0,
            'concept_score': 1.0,
            'evidence_score': 1.0,
            'score': 1.0,
            'correct_negative': True
        }
    elif is_not_found and not expects_not_found:
        return {
            'id': answer.get('id'),
            'category': category,
            'difficulty': difficulty,
            'semantic_score': 0.0,
            'concept_score': 0.0,
            'evidence_score': 0.0,
            'score': 0.0,
            'correct_negative': False
        }

    # For short expected answers (calibration), check exact containment
    expected_tokens = tokenize(expected_answer)
    if len(expected_tokens) <= 3:
        # Short answer - check if all expected tokens appear in answer
        answer_tokens_set = set(tokenize(answer_text))
        if all(t in answer_tokens_set for t in expected_tokens):
            # Perfect match on key content
            semantic_score = 1.0
            concept_score = 1.0
        else:
            semantic_score = semantic_similarity(answer_text, expected_answer, idf)
            expected_concepts = extract_key_concepts(expected_answer)
            answer_concepts = extract_key_concepts(answer_text)
            concept_score = jaccard_similarity(expected_concepts, answer_concepts)
    else:
        # Standard semantic comparison for longer answers
        semantic_score = semantic_similarity(answer_text, expected_answer, idf)
        expected_concepts = extract_key_concepts(expected_answer)
        answer_concepts = extract_key_concepts(answer_text)
        concept_score = jaccard_similarity(expected_concepts, answer_concepts)

    # Evidence bonus (20%)
    evidence_score = 1.0 if has_file_references(answer) else 0.0

    # Final score: 50% semantic, 30% concept, 20% evidence
    final_score = (0.50 * semantic_score + 0.30 * concept_score + 0.20 * evidence_score)

    return {
        'id': answer.get('id'),
        'category': category,
        'difficulty': difficulty,
        'semantic_score': round(semantic_score, 3),
        'concept_score': round(concept_score, 3),
        'evidence_score': round(evidence_score, 3),
        'score': round(final_score, 3)
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
    semantic_scores = [g['semantic_score'] for g in grades]
    concept_scores = [g['concept_score'] for g in grades]

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

    aggregate = {
        'total_questions': len(grades),
        'mean': round(statistics.mean(scores), 3),
        'std': round(statistics.stdev(scores), 3) if len(scores) > 1 else 0,
        'semantic_mean': round(statistics.mean(semantic_scores), 3),
        'concept_mean': round(statistics.mean(concept_scores), 3),
        'median': round(statistics.median(scores), 3),
        'min': round(min(scores), 3),
        'max': round(max(scores), 3),
        'high_score_rate': round(sum(1 for s in scores if s >= 0.5) / len(scores), 3),
        'by_difficulty': by_difficulty,
        'by_category': by_category
    }

    # Write output
    output = {'aggregate': aggregate, 'per_question': grades}
    with open(output_file, 'w') as f:
        json.dump(output, f, indent=2)

    print(f"\n=== SEMANTIC GRADING: {output_file} ===")
    print(f"Total: {aggregate['total_questions']}")
    print(f"Mean Score: {aggregate['mean']}")
    print(f"Semantic Mean: {aggregate['semantic_mean']}")
    print(f"Concept Mean: {aggregate['concept_mean']}")
    print(f"High score rate (>=0.5): {aggregate['high_score_rate']*100:.1f}%")
    print(f"\n=== BY DIFFICULTY ===")
    for diff, stats in by_difficulty.items():
        print(f"  {diff}: n={stats['count']}, mean={stats['mean']}")

    return aggregate


if __name__ == '__main__':
    if len(sys.argv) != 4:
        print("Usage: python grade_semantic.py answers.jsonl questions_master.json grades.json")
        sys.exit(1)
    grade_all(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))
