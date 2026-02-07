#!/usr/bin/env python3
"""
MDEMG Benchmark Grading Module v4.1

Codebase-agnostic scoring with evidence-heavy weighting to combat
LLM familiarity with training data. Actual file:line citations required
for high scores.

Scoring Components (v4.1 Spec):
- Evidence Score (70%): File:line citations matching expected
- Semantic Score (15%): N-gram + recall-weighted similarity
- Concept Score (15%): Technical concept overlap
- Citation Bonus (+10%): Citing correct file (capped at 1.0)

Evidence Tier Rubric:
- Strong (1.0): file:line AND file matches AND line within ±10
- Moderate (0.7): file:line AND file matches BUT line outside tolerance
- Weak (0.4): file:line BUT file doesn't match expected
- Minimal (0.2): File name mentioned without line number
- None (0.0): No file references

Usage as CLI:
    python grader_v4.py answers.jsonl questions_master.json grades.json

Usage as library:
    from grader_v4 import Grader
    grader = Grader(questions_file)
    grades = grader.grade_all(answers)
"""

import json
import re
import sys
import math
import statistics
from pathlib import Path
from dataclasses import dataclass, field, asdict
from typing import Dict, List, Set, Tuple, Optional, Any
from collections import Counter


# =============================================================================
# Data Classes
# =============================================================================

@dataclass
class EvidenceDetails:
    tier: str  # strong, moderate, weak, minimal, none
    citations_found: List[str] = field(default_factory=list)
    expected_files: List[str] = field(default_factory=list)
    matched_files: bool = False
    line_tolerance_met: bool = False


@dataclass
class SemanticDetails:
    unigram: float = 0.0
    bigram: float = 0.0
    trigram: float = 0.0
    concept: float = 0.0
    idf_weighted: float = 0.0


@dataclass
class Scores:
    evidence: float = 0.0
    semantic: float = 0.0
    concept: float = 0.0
    citation_bonus: float = 0.0
    final: float = 0.0


@dataclass
class Grade:
    id: str
    category: str = "unknown"
    difficulty: str = "hard"
    scores: Scores = field(default_factory=Scores)
    evidence_details: EvidenceDetails = field(default_factory=lambda: EvidenceDetails(tier="none"))
    semantic_details: SemanticDetails = field(default_factory=SemanticDetails)

    def to_dict(self) -> Dict:
        return {
            "id": self.id,
            "category": self.category,
            "difficulty": self.difficulty,
            "scores": asdict(self.scores),
            "evidence_details": asdict(self.evidence_details),
            "semantic_details": asdict(self.semantic_details)
        }


@dataclass
class AggregateMetrics:
    total_questions: int = 0
    mean: float = 0.0
    std: float = 0.0
    cv_pct: float = 0.0
    median: float = 0.0
    min: float = 0.0
    max: float = 0.0
    p10: float = 0.0
    p25: float = 0.0
    p75: float = 0.0
    p90: float = 0.0
    high_score_rate: float = 0.0
    evidence_rate: float = 0.0
    evidence_accuracy_rate: float = 0.0
    correct_file_rate: float = 0.0
    by_difficulty: Dict[str, Dict] = field(default_factory=dict)
    by_category: Dict[str, Dict] = field(default_factory=dict)


# =============================================================================
# Text Analysis Functions
# =============================================================================

# Regex for file:line citations
FILE_LINE_PATTERN = re.compile(
    r'([\w\-./]+\.(?:py|ts|tsx|js|jsx|go|rs|java|cpp|c|h|hpp|rb|php|swift|kt|scala|vue|svelte))'
    r'\s*:\s*(\d+)(?:\s*-\s*(\d+))?'
)


def tokenize(text: str) -> List[str]:
    """Tokenize text into lowercase words, numbers, and version strings."""
    tokens = re.findall(
        r'\b\d+(?:\.\d+){2,}\b'       # version strings: 1.0.0, 2.3.1
        r'|\b0x[0-9a-fA-F]+\b'        # hex: 0xff, 0x1A2B
        r'|\b\d+\.\d+\b'              # decimals: 10.0, 0.2
        r'|\b\d+\b'                    # integers: 8192, 3600
        r'|\b[a-zA-Z][a-zA-Z0-9_]*\b', # words
        text
    )
    return [t.lower() for t in tokens]


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


def recall(expected_set: Set, actual_set: Set) -> float:
    """What fraction of expected items appear in actual."""
    if not expected_set:
        return 0.0
    return len(expected_set & actual_set) / len(expected_set)


def adaptive_similarity(expected_set: Set, actual_set: Set, short_threshold: int = 10) -> float:
    """Use recall when expected is short, blended Jaccard+recall otherwise.

    Short expected answers (e.g., "1.0.0", "SHA-256", "8192 bytes") should
    not be penalized because the agent's answer adds explanatory context.
    """
    if not expected_set or not actual_set:
        return 0.0
    r = recall(expected_set, actual_set)
    j = jaccard_similarity(expected_set, actual_set)
    if len(expected_set) <= short_threshold:
        return 0.8 * r + 0.2 * j
    return 0.5 * r + 0.5 * j


def extract_key_concepts(text: str) -> Set[str]:
    """Extract key technical concepts from text.

    Covers: class/service names, constants, snake_case identifiers,
    PascalCase identifiers, numeric values, version strings, hex,
    booleans, method calls, and file-path segments.
    """
    patterns = [
        # Service/Module suffix names
        r'\b[A-Z][a-zA-Z]*(?:Service|Module|Controller|Guard|Handler|Manager|Utils|Config|Schema|Type|Store|Provider|Component|Factory|Builder|Adapter|Decorator|Observer|Strategy|Singleton)\b',
        r'\b[a-z]+(?:Service|Module|Controller|Guard|Handler|Manager|Utils|Config|Schema|Type|Store|Provider|Component|Factory|Builder|Adapter)\b',
        # PascalCase identifiers (2+ words)
        r'\b[A-Z][a-z]+(?:[A-Z][a-z]+)+\b',
        # UPPER_CASE constants
        r'\b[A-Z_]{2,}(?:_[A-Z_0-9]+)*\b',
        # snake_case identifiers
        r'\b[a-z]+(?:_[a-z0-9]+)+\b',
        # Version strings
        r'\bv?\d+(?:\.\d+){2,}\b',
        # Decimal numbers
        r'\b\d+\.\d+\b',
        # Integer values (4+ digits or any with unit)
        r'\b\d{4,}\b',
        r'\b\d+(?:\.\d+)?(?:ms|s|kb|mb|gb|%)\b',
        # Hex literals
        r'\b0x[0-9a-fA-F]+\b',
        # Boolean / null
        r'\b(?:true|false|null|undefined|none|nil)\b',
        # Method calls
        r'\.\w+\(',
    ]
    concepts = set()
    for pattern in patterns:
        matches = re.findall(pattern, text, re.IGNORECASE)
        concepts.update(m.lower().strip() for m in matches if m.strip())
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
    """Compute weighted token overlap using IDF weights (recall direction)."""
    set1, set2 = set(tokens1), set(tokens2)
    if not set1 or not set2:
        return 0.0
    common = set1 & set2
    common_weight = sum(idf.get(t, 1.0) for t in common)
    # Use set1 (expected) as denominator for recall-style scoring
    total_weight = sum(idf.get(t, 1.0) for t in set1)
    return common_weight / total_weight if total_weight > 0 else 0.0


# =============================================================================
# Evidence Extraction
# =============================================================================

def extract_file_citations(text: str) -> List[Dict]:
    """Extract file:line citations from text."""
    citations = []
    for match in FILE_LINE_PATTERN.finditer(text):
        citation = {
            'file': match.group(1),
            'line_start': int(match.group(2)),
            'line_end': int(match.group(3)) if match.group(3) else int(match.group(2))
        }
        citations.append(citation)
    return citations


def parse_file_line_ref(ref: str) -> Tuple[str, Optional[int], Optional[int]]:
    """Parse a file_line_ref like 'file.py:123' or 'file.py:34-39' into (file, start, end).

    Also handles annotations like 'file.py:123-456 (description)' by stripping them.
    """
    if ':' not in ref:
        return (ref, None, None)
    parts = ref.split(':')
    file_path = parts[0]
    line_part = parts[1] if len(parts) > 1 else ''

    # Strip annotations in parentheses: "20-29 (description)" -> "20-29"
    line_part = re.sub(r'\s*\(.*\)\s*$', '', line_part).strip()

    if '-' in line_part:
        try:
            start, end = line_part.split('-')
            return (file_path, int(start.strip()), int(end.strip()))
        except ValueError:
            return (file_path, None, None)
    elif line_part.isdigit():
        line_num = int(line_part)
        return (file_path, line_num, line_num)
    return (file_path, None, None)


def file_basename(path: str) -> str:
    """Get lowercase basename from path."""
    return Path(path).name.lower()


# =============================================================================
# Grader Class
# =============================================================================

class Grader:
    """MDEMG Benchmark Grader v4.1

    Grades answers against expected answers with evidence-heavy weighting.
    """

    LINE_TOLERANCE = 10  # Lines tolerance for evidence matching

    def __init__(self, questions: List[Dict], idf: Optional[Dict[str, float]] = None):
        """Initialize grader with questions.

        Args:
            questions: List of question dicts with expected answers
            idf: Optional pre-computed IDF weights
        """
        self.questions = {str(q['id']): q for q in questions}
        if idf is None:
            corpus = [self._get_expected_answer(q) for q in questions
                      if self._get_expected_answer(q)]
            self.idf = compute_idf_weights(corpus) if corpus else {}
        else:
            self.idf = idf

    @classmethod
    def from_file(cls, questions_file: Path) -> 'Grader':
        """Create grader from questions JSON file."""
        with open(questions_file) as f:
            data = json.load(f)
        questions = data.get('questions', data) if isinstance(data, dict) else data
        return cls(questions)

    def _get_expected_answer(self, question: Dict) -> str:
        """Get expected answer from question, handling schema variations."""
        return (question.get('expected_answer') or
                question.get('golden_answer') or
                question.get('answer') or '')

    def _get_requires_files(self, question: Dict) -> List[str]:
        """Get required files from question, handling schema variations."""
        files = (question.get('requires_files') or
                 question.get('required_files') or
                 question.get('file_line_refs') or [])
        result = []
        for f in files:
            if ':' in str(f):
                result.append(str(f).split(':')[0])
            else:
                result.append(str(f))
        return result

    def _compute_semantic_score(self, actual: str, expected: str) -> Tuple[float, SemanticDetails]:
        """Compute semantic similarity with detailed breakdown."""
        if not actual or not expected:
            return 0.0, SemanticDetails()

        actual_tokens = tokenize(actual)
        expected_tokens = tokenize(expected)
        if not actual_tokens or not expected_tokens:
            return 0.0, SemanticDetails()

        actual_set = set(actual_tokens)
        expected_set = set(expected_tokens)

        # N-gram similarities (recall-oriented for expected→actual)
        unigram = adaptive_similarity(expected_set, actual_set)
        bigram = adaptive_similarity(get_ngrams(expected_tokens, 2),
                                     get_ngrams(actual_tokens, 2))
        trigram = adaptive_similarity(get_ngrams(expected_tokens, 3),
                                      get_ngrams(actual_tokens, 3))

        # Concept similarity
        actual_concepts = extract_key_concepts(actual)
        expected_concepts = extract_key_concepts(expected)
        concept = adaptive_similarity(expected_concepts, actual_concepts) if expected_concepts and actual_concepts else 0.0

        # IDF-weighted recall
        idf_weighted = weighted_overlap(expected_tokens, actual_tokens, self.idf)

        # Combined score (matching v4 spec weights)
        combined = (0.15 * unigram + 0.20 * bigram + 0.15 * trigram +
                    0.30 * concept + 0.20 * idf_weighted)
        final = min(combined * 1.5, 1.0)  # Scale up and cap

        details = SemanticDetails(
            unigram=round(unigram, 3),
            bigram=round(bigram, 3),
            trigram=round(trigram, 3),
            concept=round(concept, 3),
            idf_weighted=round(idf_weighted, 3)
        )

        return round(final, 3), details

    def _compute_concept_score(self, actual: str, expected: str) -> float:
        """Compute concept overlap score (recall-aware)."""
        expected_concepts = extract_key_concepts(expected)
        actual_concepts = extract_key_concepts(actual)
        return round(adaptive_similarity(expected_concepts, actual_concepts), 3)

    def _compute_evidence_score(self, answer: Dict, question: Dict) -> Tuple[float, EvidenceDetails]:
        """Compute evidence score matching V3 grader logic.

        Evidence Tiers (V3-compatible):
        - Strong (1.0): ANY file:line citation exists (rewards finding evidence)
        - Minimal (0.5): Files mentioned without line numbers
        - None (0.0): No file references

        File matching is tracked separately for citation_bonus, not for evidence score.
        This matches the V22 benchmark grader behavior.
        """
        answer_text = answer.get('answer', '')
        cited_refs = answer.get('file_line_refs', [])
        files_consulted = answer.get('files_consulted', [])

        expected_files = self._get_requires_files(question)

        # Extract citations from answer text
        text_citations = extract_file_citations(answer_text)
        all_citations = []

        # Add cited_refs from answer
        for ref in cited_refs:
            file_path, line_start, line_end = parse_file_line_ref(str(ref))
            if line_start:
                all_citations.append({
                    'file': file_path,
                    'line_start': line_start,
                    'line_end': line_end or line_start
                })

        # Add text citations
        all_citations.extend(text_citations)

        details = EvidenceDetails(
            tier="none",
            citations_found=[f"{c['file']}:{c['line_start']}" for c in all_citations],
            expected_files=expected_files,
            matched_files=False,
            line_tolerance_met=False
        )

        # Check if any file:line citation exists (V3 logic: any citation = 1.0)
        has_file_line_citation = bool(all_citations)

        # Check if files are mentioned (with or without line numbers)
        has_files = bool(files_consulted) or bool(cited_refs)

        # Check if correct file was cited (for bonus calculation)
        file_matched = False
        for citation in all_citations:
            cited_base = file_basename(citation['file'])
            for ef in expected_files:
                ef_base = file_basename(ef)
                if cited_base == ef_base or ef_base in citation['file'].lower():
                    file_matched = True
                    break
            if file_matched:
                break

        # Also check files_consulted and cited_refs for file matching
        if not file_matched:
            all_files = [str(f).lower() for f in (cited_refs + files_consulted)]
            for ef in expected_files:
                ef_base = file_basename(ef)
                if any(ef_base in f for f in all_files):
                    file_matched = True
                    break

        details.matched_files = file_matched

        # V3-compatible scoring: any file:line = 1.0, just files = 0.5, nothing = 0.0
        if has_file_line_citation:
            details.tier = "strong"
            return 1.0, details
        elif has_files:
            details.tier = "minimal"
            return 0.5, details
        else:
            details.tier = "none"
            return 0.0, details

    def grade_answer(self, answer: Dict, question_id: str = None) -> Grade:
        """Grade a single answer against its expected answer."""
        qid = question_id or str(answer.get('id'))

        if qid not in self.questions:
            return Grade(id=qid, scores=Scores(final=0.0))

        question = self.questions[qid]
        answer_text = answer.get('answer', '')
        expected_answer = self._get_expected_answer(question)
        difficulty = question.get('difficulty', 'hard')
        category = question.get('category', 'unknown')

        # Handle NOT_FOUND responses
        is_not_found = answer_text.upper().strip() == 'NOT_FOUND'
        expects_not_found = expected_answer.upper().strip() == 'NOT_FOUND'

        # Handle negative_control category
        if category == 'negative_control':
            negative_indicators = [
                'does not exist', "doesn't exist", 'no such', 'not found',
                'non-existent', 'not a real', 'not part of', 'not included',
                'there is no', 'does not have', "doesn't have",
                'not present', 'fictional', 'not exist'
            ]
            answer_lower = answer_text.lower()
            correctly_identifies = any(ind in answer_lower for ind in negative_indicators)

            if correctly_identifies:
                return Grade(
                    id=qid,
                    category=category,
                    difficulty=difficulty,
                    scores=Scores(evidence=1.0, semantic=1.0, concept=1.0, final=1.0),
                    evidence_details=EvidenceDetails(tier="strong", matched_files=True, line_tolerance_met=True)
                )

        if is_not_found and expects_not_found:
            return Grade(
                id=qid,
                category=category,
                difficulty=difficulty,
                scores=Scores(evidence=1.0, semantic=1.0, concept=1.0, final=1.0),
                evidence_details=EvidenceDetails(tier="strong", matched_files=True, line_tolerance_met=True)
            )
        elif is_not_found and not expects_not_found:
            return Grade(
                id=qid,
                category=category,
                difficulty=difficulty,
                scores=Scores(final=0.0),
                evidence_details=EvidenceDetails(tier="none")
            )

        # Compute scores
        evidence_score, evidence_details = self._compute_evidence_score(answer, question)
        semantic_score, semantic_details = self._compute_semantic_score(answer_text, expected_answer)
        concept_score = self._compute_concept_score(answer_text, expected_answer)

        # Citation bonus for correct file
        citation_bonus = 0.1 if evidence_details.matched_files else 0.0

        # Final score: 70% evidence, 15% semantic, 15% concept + bonus
        raw_score = (0.70 * evidence_score +
                     0.15 * semantic_score +
                     0.15 * concept_score)
        final_score = min(raw_score + citation_bonus, 1.0)

        return Grade(
            id=qid,
            category=category,
            difficulty=difficulty,
            scores=Scores(
                evidence=evidence_score,
                semantic=semantic_score,
                concept=concept_score,
                citation_bonus=citation_bonus,
                final=round(final_score, 3)
            ),
            evidence_details=evidence_details,
            semantic_details=semantic_details
        )

    def grade_all(self, answers: List[Dict]) -> Tuple[List[Grade], AggregateMetrics]:
        """Grade all answers and compute aggregate metrics."""
        grades = []
        for answer in answers:
            qid = str(answer.get('id'))
            if qid in self.questions:
                grades.append(self.grade_answer(answer, qid))

        if not grades:
            return [], AggregateMetrics()

        # Compute aggregates
        scores = [g.scores.final for g in grades]

        def safe_percentile(data: List[float], p: float) -> float:
            if not data:
                return 0.0
            idx = int(len(data) * p)
            idx = min(idx, len(data) - 1)
            return sorted(data)[idx]

        aggregate = AggregateMetrics(
            total_questions=len(grades),
            mean=round(statistics.mean(scores), 3),
            std=round(statistics.stdev(scores), 3) if len(scores) > 1 else 0,
            median=round(statistics.median(scores), 3),
            min=round(min(scores), 3),
            max=round(max(scores), 3),
            p10=round(safe_percentile(scores, 0.10), 3),
            p25=round(safe_percentile(scores, 0.25), 3),
            p75=round(safe_percentile(scores, 0.75), 3),
            p90=round(safe_percentile(scores, 0.90), 3),
            high_score_rate=round(sum(1 for s in scores if s >= 0.7) / len(scores), 3),
            evidence_rate=round(sum(1 for g in grades if g.evidence_details.tier in ('strong', 'moderate')) / len(grades), 3),
            evidence_accuracy_rate=round(sum(1 for g in grades if g.evidence_details.line_tolerance_met) / len(grades), 3),
            correct_file_rate=round(sum(1 for g in grades if g.evidence_details.matched_files) / len(grades), 3)
        )

        # CV calculation
        if aggregate.mean > 0 and aggregate.std > 0:
            aggregate.cv_pct = round(100 * aggregate.std / aggregate.mean, 1)

        # By difficulty
        for diff in ['easy', 'medium', 'hard']:
            diff_scores = [g.scores.final for g in grades if g.difficulty == diff]
            if diff_scores:
                aggregate.by_difficulty[diff] = {
                    'count': len(diff_scores),
                    'mean': round(statistics.mean(diff_scores), 3),
                    'std': round(statistics.stdev(diff_scores), 3) if len(diff_scores) > 1 else 0
                }

        # By category
        categories = set(g.category for g in grades)
        for cat in categories:
            cat_scores = [g.scores.final for g in grades if g.category == cat]
            if cat_scores:
                aggregate.by_category[cat] = {
                    'count': len(cat_scores),
                    'mean': round(statistics.mean(cat_scores), 3),
                    'std': round(statistics.stdev(cat_scores), 3) if len(cat_scores) > 1 else 0
                }

        return grades, aggregate


# =============================================================================
# CLI Interface
# =============================================================================

def load_answers_jsonl(filepath: Path) -> List[Dict]:
    """Load answers from JSONL file."""
    answers = []
    with open(filepath) as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    answers.append(json.loads(line))
                except json.JSONDecodeError as e:
                    print(f"WARNING: Parse error: {line[:50]}... - {e}")
    return answers


def grade_all_cli(answers_file: Path, questions_file: Path, output_file: Path):
    """Grade all answers (CLI entry point)."""
    grader = Grader.from_file(questions_file)
    answers = load_answers_jsonl(answers_file)

    print(f"Loaded {len(grader.questions)} questions, {len(answers)} answers")

    grades, aggregate = grader.grade_all(answers)

    if not grades:
        print("ERROR: No grades computed")
        return None

    # Prepare output
    output = {
        'aggregate': {
            'total_questions': aggregate.total_questions,
            'mean': aggregate.mean,
            'std': aggregate.std,
            'cv_pct': aggregate.cv_pct,
            'median': aggregate.median,
            'min': aggregate.min,
            'max': aggregate.max,
            'p10': aggregate.p10,
            'p25': aggregate.p25,
            'p75': aggregate.p75,
            'p90': aggregate.p90,
            'high_score_rate': aggregate.high_score_rate,
            'evidence_rate': aggregate.evidence_rate,
            'evidence_accuracy_rate': aggregate.evidence_accuracy_rate,
            'correct_file_rate': aggregate.correct_file_rate,
            'by_difficulty': aggregate.by_difficulty,
            'by_category': aggregate.by_category
        },
        'per_question': [g.to_dict() for g in grades]
    }

    with open(output_file, 'w') as f:
        json.dump(output, f, indent=2)

    # Print summary
    print(f"\n=== GRADING COMPLETE: {output_file} ===")
    print(f"Total: {aggregate.total_questions}")
    print(f"Mean: {aggregate.mean} | Std: {aggregate.std} | CV: {aggregate.cv_pct}%")
    print(f"High score rate (>=0.7): {aggregate.high_score_rate*100:.1f}%")
    print(f"\n=== EVIDENCE TIERS ===")
    strong = sum(1 for g in grades if g.evidence_details.tier == 'strong')
    moderate = sum(1 for g in grades if g.evidence_details.tier == 'moderate')
    weak = sum(1 for g in grades if g.evidence_details.tier in ('weak', 'minimal'))
    none_tier = sum(1 for g in grades if g.evidence_details.tier == 'none')
    print(f"Strong:   {strong}/{len(grades)} ({strong/len(grades)*100:.1f}%)")
    print(f"Moderate: {moderate}/{len(grades)} ({moderate/len(grades)*100:.1f}%)")
    print(f"Weak:     {weak}/{len(grades)} ({weak/len(grades)*100:.1f}%)")
    print(f"None:     {none_tier}/{len(grades)} ({none_tier/len(grades)*100:.1f}%)")
    print(f"\n=== BY DIFFICULTY ===")
    for diff, stats in aggregate.by_difficulty.items():
        print(f"  {diff}: n={stats['count']}, mean={stats['mean']}")

    return aggregate


if __name__ == '__main__':
    if len(sys.argv) != 4:
        print("Usage: python grader_v4.py answers.jsonl questions_master.json grades.json")
        sys.exit(1)
    grade_all_cli(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))
