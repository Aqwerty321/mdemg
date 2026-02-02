#!/usr/bin/env python3
"""
Generate Agent Questions File from Master

Creates a version of the questions file without answers for use during retrieval.
This prevents answer leakage to the LLM during benchmark testing.

Usage:
    python generate_agent_questions.py master.json agent.json
"""

import json
import sys
from pathlib import Path


# Fields to strip from questions (answer-related)
ANSWER_FIELDS = {
    'expected_answer',
    'golden_answer',
    'answer',
    'requires_files',
    'required_files',
    'evidence',
    'file_line_refs'
}


def generate_agent_questions(master_path: Path, agent_path: Path):
    """Generate agent questions from master questions.

    Args:
        master_path: Path to master questions file (with answers)
        agent_path: Path to output agent questions file (without answers)
    """
    with open(master_path) as f:
        data = json.load(f)

    # Handle both formats: {"questions": [...]} and [...]
    if isinstance(data, dict):
        questions = data.get('questions', data)
        metadata = {k: v for k, v in data.items() if k != 'questions'}
    else:
        questions = data
        metadata = {}

    # Strip answer fields
    agent_questions = []
    for q in questions:
        agent_q = {k: v for k, v in q.items() if k not in ANSWER_FIELDS}
        agent_questions.append(agent_q)

    # Build output
    if metadata:
        output = {
            **metadata,
            'questions': agent_questions,
            '_note': 'Agent version - answers stripped for retrieval testing'
        }
    else:
        output = {
            'questions': agent_questions,
            '_note': 'Agent version - answers stripped for retrieval testing'
        }

    with open(agent_path, 'w') as f:
        json.dump(output, f, indent=2)

    print(f"Generated agent questions: {agent_path}")
    print(f"  Questions: {len(agent_questions)}")
    print(f"  Fields stripped: {', '.join(sorted(ANSWER_FIELDS))}")


def main():
    if len(sys.argv) != 3:
        print("Usage: python generate_agent_questions.py master.json agent.json")
        print()
        print("Strips answer-related fields from questions for retrieval testing.")
        sys.exit(1)

    master_path = Path(sys.argv[1])
    agent_path = Path(sys.argv[2])

    if not master_path.exists():
        print(f"ERROR: Master file not found: {master_path}")
        sys.exit(1)

    generate_agent_questions(master_path, agent_path)


if __name__ == '__main__':
    main()
