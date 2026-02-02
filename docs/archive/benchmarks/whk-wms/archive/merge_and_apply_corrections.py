#!/usr/bin/env python3
"""Merge all corrections and apply to whk-wms-questions-final.json"""

import json
from pathlib import Path

def main():
    base_dir = Path(__file__).parent

    # Load existing corrections
    all_corrections_file = base_dir / "all_corrections.json"
    wave_ab_file = base_dir / "wave_ab_corrections.json"
    questions_file = base_dir / "whk-wms-questions-final.json"

    print("Loading existing corrections...")
    with open(all_corrections_file, 'r') as f:
        all_corrections_data = json.load(f)

    print("Loading Wave A-B corrections...")
    with open(wave_ab_file, 'r') as f:
        wave_ab_data = json.load(f)

    # Build corrections map (Wave A-B overwrites if duplicate IDs)
    corrections_map = {}

    # Add existing corrections
    for c in all_corrections_data['corrections']:
        corrections_map[c['id']] = c['corrected_answer']

    print(f"  Existing corrections: {len(all_corrections_data['corrections'])}")

    # Add Wave A-B corrections (may overwrite)
    new_count = 0
    for c in wave_ab_data['corrections']:
        if c['id'] not in corrections_map:
            new_count += 1
        corrections_map[c['id']] = c['corrected_answer']

    print(f"  Wave A-B corrections: {len(wave_ab_data['corrections'])} ({new_count} new)")
    print(f"  Total unique corrections: {len(corrections_map)}")

    # Load questions
    print(f"\nLoading questions from {questions_file}")
    with open(questions_file, 'r') as f:
        questions_data = json.load(f)

    # Apply corrections
    applied = 0
    for question in questions_data['questions']:
        qid = question['id']
        if qid in corrections_map:
            question['answer'] = corrections_map[qid]
            applied += 1

    # Update metadata
    questions_data['metadata']['corrections_applied'] = applied
    questions_data['metadata']['last_updated'] = "2026-01-21"
    questions_data['metadata']['verification_status'] = "partially_verified_waves_a_b"
    questions_data['metadata']['pending_verification'] = "waves_c_f_batches_21_60"

    # Save updated questions
    print(f"\nSaving updated questions...")
    with open(questions_file, 'w') as f:
        json.dump(questions_data, f, indent=2)

    # Save merged corrections file
    merged_corrections = {
        "metadata": {
            "created": "2026-01-21",
            "description": "Merged corrections from initial review + waves 7-10 + waves A-B re-verification",
            "total_corrections": len(corrections_map),
            "sources": [
                "initial_review (19 corrections)",
                "waves_7_10 (29 corrections)",
                "waves_a_b_rerun (39 corrections)"
            ]
        },
        "corrections": [{"id": k, "corrected_answer": v} for k, v in sorted(corrections_map.items())]
    }

    with open(all_corrections_file, 'w') as f:
        json.dump(merged_corrections, f, indent=2)

    print(f"\nDone!")
    print(f"  Applied {applied} corrections to {len(questions_data['questions'])} questions")
    print(f"  Correction rate: {applied / len(questions_data['questions']) * 100:.1f}%")
    print(f"\nPending: Waves C-F (batches 21-60) still need verification")

if __name__ == "__main__":
    main()
