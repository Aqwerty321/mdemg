#!/usr/bin/env python3
"""Apply all verified corrections to whk-wms-questions-final.json"""

import json
from pathlib import Path

def main():
    base_dir = Path(__file__).parent

    # Load questions file
    questions_file = base_dir / "whk-wms-questions-final.json"
    corrections_file = base_dir / "all_corrections.json"

    print(f"Loading questions from {questions_file}")
    with open(questions_file, 'r') as f:
        questions_data = json.load(f)

    print(f"Loading corrections from {corrections_file}")
    with open(corrections_file, 'r') as f:
        corrections_data = json.load(f)

    # Build corrections lookup
    corrections_map = {c['id']: c['corrected_answer'] for c in corrections_data['corrections']}
    print(f"Loaded {len(corrections_map)} corrections")

    # Apply corrections
    applied = 0
    not_found = []

    for question in questions_data['questions']:
        qid = question['id']
        if qid in corrections_map:
            old_answer = question['answer']
            new_answer = corrections_map[qid]
            question['answer'] = new_answer
            applied += 1
            print(f"  Applied correction to Q{qid}")

    # Check for corrections that didn't match any question
    question_ids = {q['id'] for q in questions_data['questions']}
    for cid in corrections_map:
        if cid not in question_ids:
            not_found.append(cid)

    if not_found:
        print(f"\nWarning: {len(not_found)} corrections did not match any question: {not_found}")

    # Update metadata
    questions_data['metadata']['corrections_applied'] = applied
    questions_data['metadata']['last_updated'] = "2026-01-21"
    questions_data['metadata']['verification_status'] = "verified_and_corrected"

    # Save updated file
    output_file = base_dir / "whk-wms-questions-final.json"
    print(f"\nSaving updated questions to {output_file}")
    with open(output_file, 'w') as f:
        json.dump(questions_data, f, indent=2)

    print(f"\nDone! Applied {applied} corrections to {len(questions_data['questions'])} questions")
    print(f"Correction rate: {applied / len(questions_data['questions']) * 100:.1f}%")

if __name__ == "__main__":
    main()
