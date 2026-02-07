#!/usr/bin/env python3
"""
MDEMG Benchmark Answer Validator

Real-time validation framework for benchmark answers.
Ensures answers meet quality requirements before being written.

Key Validations:
- file_line_refs populated and properly formatted
- No duplicate question IDs
- Valid JSON structure
- Answer text non-empty

Usage:
    validator = AnswerValidator()
    is_valid, errors = validator.validate_answer(answer_dict)

    # Or for batch validation
    results = validator.validate_file("answers.jsonl")
"""

import json
import re
from pathlib import Path
from dataclasses import dataclass, field
from typing import Dict, List, Tuple, Optional, Set


# Pattern for valid file:line references
# Matches: filename.ext:123 or filename.ext:123-456
FILE_LINE_REF_PATTERN = re.compile(
    r'^[\w\-./]+\.\w+:\d+(?:-\d+)?(?:\s*\(.*\))?$'
)

# Pattern for extracting just file:line without annotation
FILE_LINE_EXTRACT_PATTERN = re.compile(
    r'([\w\-./]+\.\w+):(\d+)(?:-(\d+))?'
)


@dataclass
class ValidationResult:
    """Result of validating a single answer."""
    is_valid: bool
    errors: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)
    question_id: Optional[int] = None
    has_file_line_refs: bool = False
    file_line_ref_count: int = 0


@dataclass
class BatchValidationResult:
    """Result of validating a batch of answers."""
    total_answers: int = 0
    valid_answers: int = 0
    invalid_answers: int = 0
    answers_with_refs: int = 0
    answers_without_refs: int = 0
    duplicate_ids: List[int] = field(default_factory=list)
    malformed_lines: List[int] = field(default_factory=list)
    validation_results: List[ValidationResult] = field(default_factory=list)

    @property
    def ref_rate(self) -> float:
        """Percentage of answers with file_line_refs."""
        return self.answers_with_refs / self.total_answers if self.total_answers > 0 else 0.0

    @property
    def valid_rate(self) -> float:
        """Percentage of valid answers."""
        return self.valid_answers / self.total_answers if self.total_answers > 0 else 0.0

    def summary(self) -> str:
        """Return a summary string."""
        lines = [
            f"Total answers: {self.total_answers}",
            f"Valid: {self.valid_answers} ({self.valid_rate*100:.1f}%)",
            f"With file_line_refs: {self.answers_with_refs} ({self.ref_rate*100:.1f}%)",
        ]
        if self.duplicate_ids:
            lines.append(f"Duplicate IDs: {self.duplicate_ids}")
        if self.malformed_lines:
            lines.append(f"Malformed lines: {self.malformed_lines}")
        return "\n".join(lines)


class AnswerValidator:
    """Validator for MDEMG benchmark answers."""

    def __init__(self, strict: bool = True):
        """
        Initialize validator.

        Args:
            strict: If True, missing file_line_refs is an error. If False, it's a warning.
        """
        self.strict = strict
        self.seen_ids: Set[int] = set()

    def reset(self):
        """Reset state for a new validation run."""
        self.seen_ids = set()

    def validate_file_line_ref(self, ref: str) -> Tuple[bool, Optional[str]]:
        """
        Validate a single file:line reference.

        Valid formats:
        - "filename.ts:123"
        - "filename.ts:123-456"
        - "filename.ts:123 (description)"
        - "path/to/filename.ts:123-456 (description)"

        Returns:
            Tuple of (is_valid, error_message or None)
        """
        if not ref or not isinstance(ref, str):
            return False, "Empty or non-string reference"

        ref = ref.strip()

        # Must contain a colon
        if ':' not in ref:
            return False, f"No colon found in '{ref}' - expected 'filename:line'"

        # Extract the file:line part (ignore annotations in parens)
        match = FILE_LINE_EXTRACT_PATTERN.search(ref)
        if not match:
            return False, f"Invalid format '{ref}' - expected 'filename.ext:line'"

        filename = match.group(1)
        line_start = match.group(2)
        line_end = match.group(3)

        # Validate filename has extension
        if '.' not in Path(filename).name:
            return False, f"Filename '{filename}' has no extension"

        # Validate line numbers are positive
        if int(line_start) <= 0:
            return False, f"Line number must be positive, got {line_start}"

        if line_end and int(line_end) < int(line_start):
            return False, f"Line end ({line_end}) must be >= line start ({line_start})"

        return True, None

    def validate_answer(self, answer: Dict) -> ValidationResult:
        """
        Validate a single answer dictionary.

        Required fields:
        - id: Question ID (int)
        - answer: Answer text (non-empty string)
        - file_line_refs: List of file:line references (at least 1)

        Optional fields:
        - question: Original question text
        - files_consulted: List of file paths
        - mdemg_used: Boolean
        - confidence: String or float

        Returns:
            ValidationResult with is_valid, errors, and warnings
        """
        result = ValidationResult(is_valid=True)

        # Check ID
        qid = answer.get('id')
        if qid is None:
            result.errors.append("Missing 'id' field")
            result.is_valid = False
        else:
            result.question_id = qid
            if qid in self.seen_ids:
                result.errors.append(f"Duplicate question ID: {qid}")
                result.is_valid = False
            else:
                self.seen_ids.add(qid)

        # Check answer text
        answer_text = answer.get('answer', '')
        if not answer_text or not isinstance(answer_text, str):
            result.errors.append("Missing or empty 'answer' field")
            result.is_valid = False
        elif len(answer_text) < 20:
            result.warnings.append(f"Answer text very short ({len(answer_text)} chars)")

        # Check file_line_refs (critical)
        file_line_refs = answer.get('file_line_refs', [])

        if not file_line_refs:
            msg = "Missing or empty 'file_line_refs' - evidence score will be 0.0"
            if self.strict:
                result.errors.append(msg)
                result.is_valid = False
            else:
                result.warnings.append(msg)
            result.has_file_line_refs = False
            result.file_line_ref_count = 0
        else:
            result.has_file_line_refs = True
            result.file_line_ref_count = len(file_line_refs)

            # Validate each reference
            valid_refs = 0
            for ref in file_line_refs:
                is_valid, error = self.validate_file_line_ref(ref)
                if is_valid:
                    valid_refs += 1
                else:
                    result.warnings.append(f"Invalid ref '{ref}': {error}")

            if valid_refs == 0:
                msg = "No valid file_line_refs found"
                if self.strict:
                    result.errors.append(msg)
                    result.is_valid = False
                else:
                    result.warnings.append(msg)

        # Check files_consulted (optional but good to have)
        files_consulted = answer.get('files_consulted', [])
        if not files_consulted:
            result.warnings.append("No files_consulted listed")

        return result

    def validate_jsonl_line(self, line: str, line_number: int) -> Tuple[Optional[Dict], ValidationResult]:
        """
        Validate a single JSONL line.

        Returns:
            Tuple of (parsed_dict or None, ValidationResult)
        """
        result = ValidationResult(is_valid=True)

        line = line.strip()
        if not line:
            result.is_valid = False
            result.errors.append(f"Line {line_number}: Empty line")
            return None, result

        try:
            answer = json.loads(line)
        except json.JSONDecodeError as e:
            result.is_valid = False
            result.errors.append(f"Line {line_number}: JSON parse error: {e}")
            return None, result

        return answer, self.validate_answer(answer)

    def validate_file(self, filepath: Path) -> BatchValidationResult:
        """
        Validate an entire JSONL file.

        Returns:
            BatchValidationResult with aggregate statistics
        """
        self.reset()
        result = BatchValidationResult()

        filepath = Path(filepath)
        if not filepath.exists():
            result.malformed_lines.append(0)
            return result

        with open(filepath, 'r') as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue

                result.total_answers += 1

                answer, validation = self.validate_jsonl_line(line, line_num)
                result.validation_results.append(validation)

                if validation.is_valid:
                    result.valid_answers += 1
                else:
                    result.invalid_answers += 1

                if validation.has_file_line_refs:
                    result.answers_with_refs += 1
                else:
                    result.answers_without_refs += 1

                # Check for duplicate ID error in validation result
                if validation.question_id and any('Duplicate' in e for e in validation.errors):
                    if validation.question_id not in result.duplicate_ids:
                        result.duplicate_ids.append(validation.question_id)

                if not answer:
                    result.malformed_lines.append(line_num)

        return result


def validate_in_progress(output_file: Path, question_count: int = 120) -> Dict:
    """
    Validate an in-progress benchmark output file.

    Returns a status dict suitable for monitoring.
    """
    validator = AnswerValidator(strict=True)
    result = validator.validate_file(output_file)

    progress = result.total_answers / question_count if question_count > 0 else 0

    return {
        'progress_pct': round(progress * 100, 1),
        'answers_written': result.total_answers,
        'expected_total': question_count,
        'valid_answers': result.valid_answers,
        'ref_rate_pct': round(result.ref_rate * 100, 1),
        'empty_refs_count': result.answers_without_refs,
        'status': 'OK' if result.ref_rate >= 0.95 else 'WARNING' if result.ref_rate >= 0.80 else 'CRITICAL',
        'summary': result.summary()
    }


def main():
    """CLI for validating answer files."""
    import sys

    if len(sys.argv) < 2:
        print("Usage: python validator.py <answers.jsonl> [--strict|--lenient]")
        sys.exit(1)

    filepath = Path(sys.argv[1])
    strict = '--lenient' not in sys.argv

    validator = AnswerValidator(strict=strict)
    result = validator.validate_file(filepath)

    print(f"\n{'='*60}")
    print(f"VALIDATION RESULTS: {filepath.name}")
    print(f"{'='*60}")
    print(result.summary())

    # Show detailed errors for invalid answers
    invalid = [r for r in result.validation_results if not r.is_valid]
    if invalid:
        print(f"\n{'='*60}")
        print(f"INVALID ANSWERS ({len(invalid)}):")
        print(f"{'='*60}")
        for r in invalid[:10]:  # Show first 10
            print(f"\nQ{r.question_id}:")
            for e in r.errors:
                print(f"  ERROR: {e}")
            for w in r.warnings:
                print(f"  WARN: {w}")

    # Exit code
    if result.valid_rate < 0.95:
        print(f"\nFAILED: Valid rate {result.valid_rate*100:.1f}% < 95%")
        sys.exit(1)
    elif result.ref_rate < 0.95:
        print(f"\nFAILED: Ref rate {result.ref_rate*100:.1f}% < 95%")
        sys.exit(1)
    else:
        print(f"\nPASSED: {result.valid_answers}/{result.total_answers} valid with refs")
        sys.exit(0)


if __name__ == '__main__':
    main()
