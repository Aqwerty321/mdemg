#!/usr/bin/env python3
"""Apply all corrections from all verification waves to whk-wms-questions-final.json"""

import json
from pathlib import Path

def main():
    base_dir = Path(__file__).parent

    # Load existing corrections (already includes A-B)
    all_corrections_file = base_dir / "all_corrections.json"
    wave_cf_file = base_dir / "wave_c_f_corrections.json"
    questions_file = base_dir / "whk-wms-questions-final.json"

    print("Loading existing corrections (includes waves A-B)...")
    with open(all_corrections_file, 'r') as f:
        all_corrections_data = json.load(f)

    print("Loading Wave C-F corrections...")
    with open(wave_cf_file, 'r') as f:
        wave_cf_data = json.load(f)

    # Build corrections map
    corrections_map = {}

    # Add existing corrections (initial + waves 7-10 + waves A-B)
    for c in all_corrections_data['corrections']:
        corrections_map[c['id']] = c['corrected_answer']

    print(f"  Existing corrections: {len(all_corrections_data['corrections'])}")

    # Add Wave C-F corrections (may overwrite)
    new_count = 0
    for c in wave_cf_data['corrections']:
        if c['id'] not in corrections_map:
            new_count += 1
        corrections_map[c['id']] = c['corrected_answer']

    print(f"  Wave C-F corrections: {len(wave_cf_data['corrections'])} ({new_count} new)")
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
    questions_data['metadata']['verification_status'] = "fully_verified_batches_1_60"
    questions_data['metadata']['verification_waves'] = {
        "A": {"batches": "1-10", "verified": 25, "corrections": 15},
        "B": {"batches": "11-20", "verified": 16, "corrections": 24},
        "C": {"batches": "21-30", "verified": 21, "corrections": 19},
        "D": {"batches": "31-40", "verified": 22, "corrections": 18},
        "E": {"batches": "41-50", "verified": 28, "corrections": 12},
        "F": {"batches": "51-60", "verified": 21, "corrections": 19}
    }

    # Save updated questions
    print(f"\nSaving updated questions...")
    with open(questions_file, 'w') as f:
        json.dump(questions_data, f, indent=2)

    # Save merged corrections file
    merged_corrections = {
        "metadata": {
            "created": "2026-01-21",
            "description": "Complete merged corrections from all verification waves (initial + 7-10 + A-F)",
            "total_corrections": len(corrections_map),
            "sources": [
                "initial_review (19 corrections)",
                "waves_7_10 (29 corrections)",
                "waves_a_b_rerun (39 corrections)",
                "waves_c_f (32 corrections)"
            ],
            "verification_summary": {
                "total_questions_verified": 240,
                "total_corrections": len(corrections_map),
                "verification_rate": f"{len(corrections_map) / 240 * 100:.1f}%"
            }
        },
        "corrections": [{"id": k, "corrected_answer": v} for k, v in sorted(corrections_map.items())]
    }

    with open(all_corrections_file, 'w') as f:
        json.dump(merged_corrections, f, indent=2)

    print(f"\nDone!")
    print(f"  Applied {applied} corrections to {len(questions_data['questions'])} questions")
    print(f"  Correction rate: {applied / len(questions_data['questions']) * 100:.1f}%")
    print(f"\nVerification Summary (Batches 1-60):")
    print(f"  Total verified: 133 questions correct")
    print(f"  Total corrected: {len(corrections_map)} questions")
    print(f"  Remaining unverified: batches 61-95 (questions 285+)")

if __name__ == "__main__":
    main()
