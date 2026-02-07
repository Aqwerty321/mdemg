#!/usr/bin/env python3
"""
MDEMG Benchmark Runner v1.0

Locked benchmark framework for validating MDEMG improvements.
Enforces checksum validation to ensure reproducibility.

Usage:
    python run_benchmark.py grade <answers.jsonl> <questions.json> <output.json>
    python run_benchmark.py verify
    python run_benchmark.py baseline whk-wms
"""

import sys
import json
import hashlib
import subprocess
from pathlib import Path

# Locked checksums - DO NOT MODIFY
CHECKSUMS = {
    "grader": "24dc39216748b79bed0b06a7f998419aefd1fc0b6da4aea385e4d79f7124aa41",
    "whk-wms-master": "43201b16c0981fac000a6e15c95468fd8dd3ec55ae5978c365a8c21f669caaf4",
    "whk-wms-agent": "24aa17a215e4e58b8b44c7faef9f14228edb0e6d3f8f657d867b1bfa850f7e9e",
}

# Established baselines
BASELINES = {
    "whk-wms": {
        "baseline": {"mean_low": 0.82, "mean_high": 0.86},
        "mdemg": {"mean_low": 0.78, "mean_high": 0.81},
        "questions": 120,
    }
}

FRAMEWORK_DIR = Path(__file__).parent
GRADER_PATH = FRAMEWORK_DIR / "grader_v4_locked.py"


def sha256_file(filepath: Path) -> str:
    """Compute SHA-256 hash of a file."""
    h = hashlib.sha256()
    with open(filepath, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            h.update(chunk)
    return h.hexdigest()


def verify_checksums() -> bool:
    """Verify all framework file checksums."""
    print("=== Verifying Framework Integrity ===\n")
    all_valid = True

    # Check grader
    grader_hash = sha256_file(GRADER_PATH)
    if grader_hash == CHECKSUMS["grader"]:
        print(f"✓ Grader: {grader_hash[:16]}...")
    else:
        print(f"✗ Grader MISMATCH!")
        print(f"  Expected: {CHECKSUMS['grader'][:16]}...")
        print(f"  Got:      {grader_hash[:16]}...")
        all_valid = False

    # Check whk-wms questions
    whk_master = FRAMEWORK_DIR.parent / "whk-wms" / "test_questions_120.json"
    if whk_master.exists():
        master_hash = sha256_file(whk_master)
        if master_hash == CHECKSUMS["whk-wms-master"]:
            print(f"✓ whk-wms master: {master_hash[:16]}...")
        else:
            print(f"✗ whk-wms master MISMATCH!")
            all_valid = False

    whk_agent = FRAMEWORK_DIR.parent / "whk-wms" / "test_questions_120_agent.json"
    if whk_agent.exists():
        agent_hash = sha256_file(whk_agent)
        if agent_hash == CHECKSUMS["whk-wms-agent"]:
            print(f"✓ whk-wms agent: {agent_hash[:16]}...")
        else:
            print(f"✗ whk-wms agent MISMATCH!")
            all_valid = False

    print()
    if all_valid:
        print("Framework integrity: VALID")
    else:
        print("Framework integrity: FAILED - files have been modified!")

    return all_valid


def grade(answers_path: str, questions_path: str, output_path: str) -> dict:
    """Run grading with locked grader."""
    # Verify grader integrity first
    grader_hash = sha256_file(GRADER_PATH)
    if grader_hash != CHECKSUMS["grader"]:
        print("ERROR: Grader file has been modified!")
        print("Run 'python run_benchmark.py verify' for details.")
        sys.exit(1)

    # Run grader
    result = subprocess.run(
        [sys.executable, str(GRADER_PATH), answers_path, questions_path, output_path],
        capture_output=True,
        text=True
    )

    print(result.stdout)
    if result.stderr:
        print(result.stderr, file=sys.stderr)

    # Load and return results
    if Path(output_path).exists():
        with open(output_path) as f:
            return json.load(f)
    return {}


def show_baseline(codebase: str):
    """Show established baselines for a codebase."""
    if codebase not in BASELINES:
        print(f"Unknown codebase: {codebase}")
        print(f"Available: {', '.join(BASELINES.keys())}")
        return

    b = BASELINES[codebase]
    print(f"=== {codebase} Baselines ({b['questions']} questions) ===\n")
    print(f"Baseline: {b['baseline']['mean_low']:.2f} - {b['baseline']['mean_high']:.2f}")
    print(f"MDEMG:    {b['mdemg']['mean_low']:.2f} - {b['mdemg']['mean_high']:.2f}")
    print()
    print("Interpret results:")
    print("  Score > baseline high: IMPROVEMENT")
    print("  Score in baseline range: NO CHANGE")
    print("  Score < baseline low: REGRESSION")


def compare_to_baseline(score: float, mode: str, codebase: str = "whk-wms") -> str:
    """Compare a score to established baseline."""
    if codebase not in BASELINES:
        return "Unknown codebase"

    b = BASELINES[codebase][mode]
    if score > b["mean_high"]:
        delta = score - b["mean_high"]
        return f"IMPROVEMENT (+{delta:.3f} above baseline)"
    elif score < b["mean_low"]:
        delta = b["mean_low"] - score
        return f"REGRESSION (-{delta:.3f} below baseline)"
    else:
        return "WITHIN BASELINE (no significant change)"


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    cmd = sys.argv[1]

    if cmd == "verify":
        valid = verify_checksums()
        sys.exit(0 if valid else 1)

    elif cmd == "grade":
        if len(sys.argv) != 5:
            print("Usage: python run_benchmark.py grade <answers.jsonl> <questions.json> <output.json>")
            sys.exit(1)
        results = grade(sys.argv[2], sys.argv[3], sys.argv[4])
        if results:
            mean = results.get("aggregate", {}).get("mean", 0)
            print(f"\nBaseline comparison: {compare_to_baseline(mean, 'baseline')}")
            print(f"MDEMG comparison:    {compare_to_baseline(mean, 'mdemg')}")

    elif cmd == "baseline":
        codebase = sys.argv[2] if len(sys.argv) > 2 else "whk-wms"
        show_baseline(codebase)

    else:
        print(f"Unknown command: {cmd}")
        print(__doc__)
        sys.exit(1)


if __name__ == "__main__":
    main()
