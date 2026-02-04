#!/usr/bin/env python3
"""Select 100 random questions for testing, balanced by category"""

import json
import random
from pathlib import Path
from collections import defaultdict

def main():
    base_dir = Path(__file__).parent
    questions_file = base_dir / "whk-wms-questions-final.json"
    output_file = base_dir / "test_questions_100.json"

    # Load questions
    with open(questions_file, 'r') as f:
        data = json.load(f)

    questions = data['questions']
    print(f"Total questions: {len(questions)}")

    # Group by category
    by_category = defaultdict(list)
    for q in questions:
        by_category[q['category']].append(q)

    print("\nQuestions per category:")
    for cat, qs in sorted(by_category.items()):
        print(f"  {cat}: {len(qs)}")

    # Select 20 per category (100 total)
    random.seed(2026)  # Reproducible selection
    selected = []

    for cat in sorted(by_category.keys()):
        cat_questions = by_category[cat]
        n_select = min(20, len(cat_questions))
        selected.extend(random.sample(cat_questions, n_select))

    # Shuffle final selection
    random.shuffle(selected)

    print(f"\nSelected {len(selected)} questions")

    # Save test questions
    test_data = {
        "metadata": {
            "created": "2026-01-21",
            "seed": 2026,
            "total_questions": len(selected),
            "source": "whk-wms-questions-final.json",
            "selection": "20 per category, randomized"
        },
        "questions": selected
    }

    with open(output_file, 'w') as f:
        json.dump(test_data, f, indent=2)

    print(f"\nSaved to {output_file}")

    # Print question IDs for reference
    print("\nSelected question IDs:")
    ids_by_cat = defaultdict(list)
    for q in selected:
        ids_by_cat[q['category']].append(q['id'])

    for cat in sorted(ids_by_cat.keys()):
        print(f"  {cat}: {sorted(ids_by_cat[cat])}")

if __name__ == "__main__":
    main()
