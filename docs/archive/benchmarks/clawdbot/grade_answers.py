#!/usr/bin/env python3
"""
MDEMG Benchmark Grading Script v3
Grades agent answers using SEMANTIC SIMILARITY:
- N-gram Jaccard similarity
- Weighted word overlap (pseudo-IDF)
- Key concept extraction
- Evidence citation bonus

No external dependencies required (pure Python).

Usage:
    python grade_answers.py answers.jsonl questions_master.json grades.json
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
    # Common technical patterns
    patterns = [
        r'\b[A-Z][a-zA-Z]*(?:Service|Module|Controller|Guard|Pipe|Interceptor|Processor|Provider|Factory|Repository|Handler|Manager|Resolver|Strategy)\b',
        r'\b[a-z]+(?:Service|Module|Controller|Guard|Pipe|Interceptor|Processor|Provider|Factory|Repository|Handler|Manager|Resolver|Strategy)\b',
        r'\b[A-Z_]{2,}(?:_[A-Z_]+)*\b',  # CONSTANTS_LIKE_THIS
        r'\b[a-z]+(?:_[a-z]+)+\b',  # snake_case_identifiers
        r'\b\d+(?:ms|s|kb|mb|gb|%)\b',  # Numbers with units
        r'\b(?:true|false|null|undefined)\b',
        r'@\w+',  # Decorators like @Injectable
        r'\.\w+\(',  # Method calls like .process(
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

    # IDF = log(N / df) + 1
    idf = {}
    for token, df in doc_freq.items():
        idf[token] = math.log(n_docs / df) + 1 if df > 0 else 1

    return idf


def weighted_overlap(tokens1: List[str], tokens2: List[str], idf: Dict[str, float]) -> float:
    """Compute weighted token overlap using IDF weights."""
    set1 = set(tokens1)
    set2 = set(tokens2)

    if not set1 or not set2:
        return 0.0

    common = set1 & set2

    # Weight by IDF
    common_weight = sum(idf.get(t, 1.0) for t in common)
    total_weight = sum(idf.get(t, 1.0) for t in set1)

    return common_weight / total_weight if total_weight > 0 else 0.0


def semantic_similarity(text1: str, text2: str, idf: Dict[str, float] = None) -> float:
    """
    Compute semantic similarity between two texts using multiple methods.
    Returns a score between 0.0 and 1.0.
    """
    if not text1 or not text2:
        return 0.0

    tokens1 = tokenize(text1)
    tokens2 = tokenize(text2)

    if not tokens1 or not tokens2:
        return 0.0

    # Method 1: Unigram Jaccard
    unigram_sim = jaccard_similarity(set(tokens1), set(tokens2))

    # Method 2: Bigram Jaccard
    bigrams1 = get_ngrams(tokens1, 2)
    bigrams2 = get_ngrams(tokens2, 2)
    bigram_sim = jaccard_similarity(bigrams1, bigrams2)

    # Method 3: Trigram Jaccard
    trigrams1 = get_ngrams(tokens1, 3)
    trigrams2 = get_ngrams(tokens2, 3)
    trigram_sim = jaccard_similarity(trigrams1, trigrams2)

    # Method 4: Key concept overlap
    concepts1 = extract_key_concepts(text1)
    concepts2 = extract_key_concepts(text2)
    concept_sim = jaccard_similarity(concepts1, concepts2) if concepts1 and concepts2 else 0.0

    # Method 5: Weighted overlap (if IDF provided)
    weighted_sim = weighted_overlap(tokens1, tokens2, idf) if idf else unigram_sim

    # Combine methods with weights
    # Emphasize concept and n-gram matches over simple word overlap
    combined = (
        0.15 * unigram_sim +
        0.20 * bigram_sim +
        0.15 * trigram_sim +
        0.30 * concept_sim +
        0.20 * weighted_sim
    )

    return min(combined * 1.5, 1.0)  # Scale up slightly, cap at 1.0


def grade_answer(answer: Dict, expected: Dict, idf: Dict[str, float]) -> Dict:
    """Grade a single answer against expected using semantic similarity."""
    answer_text = answer.get('answer', '')
    expected_answer = expected.get('answer', '')
    expected_files = expected.get('required_files', [])

    # Handle NOT_FOUND answers
    if answer_text.upper() == 'NOT_FOUND':
        return {
            'id': answer.get('id'),
            'semantic_score': 0.0,
            'concept_score': 0.0,
            'evidence_score': 0.0,
            'score': 0.0,
            'expected_preview': str(expected_answer)[:100] if expected_answer else None,
            'concepts_expected': [],
            'concepts_found': [],
            'evidence_found': False,
            'evidence_accurate': False,
            'correct_file_cited': False,
            'cross_repo_contamination': False,
            'compaction_index': answer.get('compaction_index', 0),
            'answer_preview': answer_text[:200] if answer_text else ''
        }

    # Semantic similarity score
    semantic_score = semantic_similarity(answer_text, expected_answer, idf)

    # Concept overlap score
    expected_concepts = extract_key_concepts(expected_answer)
    answer_concepts = extract_key_concepts(answer_text)
    concept_score = jaccard_similarity(expected_concepts, answer_concepts)

    # Evidence score - bonus for citing file:line references
    cited_files = answer.get('file_line_refs', [])
    files_consulted = answer.get('files_consulted', [])

    evidence_found = bool(cited_files) and any(':' in str(c) for c in cited_files)
    has_files = bool(files_consulted) or bool(cited_files)

    evidence_score = 0.0
    if evidence_found:
        evidence_score = 1.0
    elif has_files:
        evidence_score = 0.5

    # Check if correct file was cited
    correct_file_cited = False
    all_cited = [str(f) for f in (cited_files + files_consulted)]
    for expected_file in expected_files:
        expected_basename = Path(expected_file).name.lower()
        for cited in all_cited:
            if expected_basename in cited.lower():
                correct_file_cited = True
                break

    # Bonus for correct file citation
    file_bonus = 0.1 if correct_file_cited else 0.0

    # Isolation & Reliability Metrics (RAA/CRCR)
    # RAA: Repo Attribution Accuracy
    # CRCR: Cross-Repo Contamination Rate
    source_space = answer.get('space_id', '')
    expected_space = expected.get('space_id', source_space)
    
    cross_repo_contamination = False
    if source_space and expected_space and source_space != expected_space:
        cross_repo_contamination = True
    
    # E-Acc: Evidence Accuracy (Upgrade from ECR)
    # Placeholder for future implementation: actual verification of cited lines
    evidence_accurate = evidence_found # Default to found for now, E-Acc needs manual/LLM check

    # Final score: 70% evidence, 15% concept, 15% semantic + bonus
    final_score = min(
        0.15 * semantic_score +
        0.15 * concept_score +
        0.70 * evidence_score +
        file_bonus,
        1.0
    )

    return {
        'id': answer.get('id'),
        'semantic_score': round(semantic_score, 3),
        'concept_score': round(concept_score, 3),
        'evidence_score': round(evidence_score, 3),
        'score': round(final_score, 3),
        'expected_preview': str(expected_answer)[:100] if expected_answer else None,
        'concepts_expected': list(expected_concepts)[:10],
        'concepts_found': list(expected_concepts & answer_concepts)[:10],
        'evidence_found': evidence_found,
        'evidence_accurate': evidence_accurate,
        'correct_file_cited': correct_file_cited,
        'cross_repo_contamination': cross_repo_contamination,
        'compaction_index': answer.get('compaction_index', 0),
        'answer_preview': answer_text[:200] if answer_text else ''
    }


def grade_all(answers_file: Path, questions_file: Path, output_file: Path):
    """Grade all answers and compute aggregate metrics."""

    # Load questions with expected answers
    with open(questions_file) as f:
        questions_data = json.load(f)

    # Handle both formats: {"questions": [...]} or just [...]
    if isinstance(questions_data, dict):
        questions_list = questions_data.get('questions', [])
    else:
        questions_list = questions_data

    questions = {str(q['id']): q for q in questions_list}

    # Load answers (JSONL format)
    answers = []
    with open(answers_file) as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    answers.append(json.loads(line))
                except json.JSONDecodeError as e:
                    print(f"WARNING: Could not parse line: {line[:50]}... - {e}")

    print(f"Loaded {len(questions)} questions, {len(answers)} answers")

    # Build IDF from all expected answers (corpus)
    corpus = [q.get('answer', '') for q in questions_list if q.get('answer')]
    idf = compute_idf_weights(corpus) if corpus else {}

    # Grade each answer
    grades = []
    for answer in answers:
        qid = str(answer.get('id'))
        if qid in questions:
            grade = grade_answer(answer, questions[qid], idf)
            grades.append(grade)
        else:
            print(f"WARNING: Question {qid} not found in master set")

    # Compute aggregate metrics
    if not grades:
        print("ERROR: No grades computed")
        return None

    scores = [g['score'] for g in grades]
    semantic_scores = [g['semantic_score'] for g in grades]
    concept_scores = [g['concept_score'] for g in grades]
    evidence_scores = [g['evidence_score'] for g in grades]

    # Compute compaction metrics
    compactions = [g['compaction_index'] for g in grades]
    max_k = max(compactions) if compactions else 0
    
    # Calculate PCD (Post-Compaction Delta)
    pcd_mean = 0.0
    if max_k > 0:
        # Simple heuristic: compare avg score before vs after first compaction
        pre_k = [g['score'] for g in grades if g['compaction_index'] == 0]
        post_k = [g['score'] for g in grades if g['compaction_index'] > 0]
        if pre_k and post_k:
            pcd_mean = round(statistics.mean(post_k) - statistics.mean(pre_k), 3)

    # Compute evidence quality tiers
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
        'pcd_mean': pcd_mean,
        'compaction_count': max_k,
        'high_score_rate': round(sum(1 for s in scores if s >= 0.7) / len(scores), 3),
        'crcr_rate': round(sum(1 for g in grades if g['cross_repo_contamination']) / len(grades), 3),
        'avg_semantic_score': round(statistics.mean(semantic_scores), 3),
        'avg_concept_score': round(statistics.mean(concept_scores), 3),
        'avg_evidence_score': round(statistics.mean(evidence_scores), 3),
        'evidence_rate': round(sum(1 for g in grades if g['evidence_found']) / len(grades), 3),
        'correct_file_rate': round(sum(1 for g in grades if g['correct_file_cited']) / len(grades), 3),
        # Evidence quality tiers (for skepticism reduction)
        'strong_evidence_rate': round(strong_evidence / len(grades), 3),
        'weak_evidence_rate': round(weak_evidence / len(grades), 3),
        'no_evidence_rate': round(no_evidence / len(grades), 3),
        'strong_evidence_count': strong_evidence,
        'weak_evidence_count': weak_evidence,
        'no_evidence_count': no_evidence,
    }

    # Write output
    output = {
        'aggregate': aggregate,
        'per_question': grades
    }

    with open(output_file, 'w') as f:
        json.dump(output, f, indent=2)

    print(f"\nGrading complete: {output_file}")
    print(f"  Total graded: {aggregate['total_questions']}")
    print(f"  Mean: {aggregate['mean']}")
    print(f"  Std:  {aggregate['std']}")
    print(f"  CV:   {aggregate['cv_pct']}%")
    print(f"  High score rate (>=0.7): {aggregate['high_score_rate']*100:.1f}%")
    print(f"  Avg semantic score: {aggregate['avg_semantic_score']}")
    print(f"  Avg concept score: {aggregate['avg_concept_score']}")
    print(f"  Avg evidence score: {aggregate['avg_evidence_score']}")
    print(f"\n  === Evidence Quality Tiers ===")
    print(f"  Strong (file:line + value): {aggregate['strong_evidence_rate']*100:.1f}% ({aggregate['strong_evidence_count']}/{aggregate['total_questions']})")
    print(f"  Weak (files but no line):   {aggregate['weak_evidence_rate']*100:.1f}% ({aggregate['weak_evidence_count']}/{aggregate['total_questions']})")
    print(f"  None (narrative only):      {aggregate['no_evidence_rate']*100:.1f}% ({aggregate['no_evidence_count']}/{aggregate['total_questions']})")

    return aggregate


if __name__ == '__main__':
    if len(sys.argv) != 4:
        print("Usage: python grade_answers.py answers.jsonl questions_master.json grades.json")
        sys.exit(1)

    grade_all(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))
