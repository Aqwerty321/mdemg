#!/usr/bin/env python3
"""
Benchmark Runner V3 - Mechanical MDEMG Enforcement

Each question is processed in a separate iteration:
1. Load question i
2. Call MDEMG API to get relevant files
3. Spawn CLI agent with question + MDEMG results
4. Agent reads files and writes answer
5. Repeat for all questions

Uses Claude CLI with --print --no-session-persistence for headless operation.
"""

import json
import os
import subprocess
import sys
import time
import urllib.request
import urllib.error
from pathlib import Path
from datetime import datetime
from typing import Dict, List, Optional
import argparse


def call_mdemg_api(endpoint: str, space_id: str, query: str, top_k: int = 5) -> List[Dict]:
    """Call MDEMG retrieval API and return results."""
    url = f"{endpoint}/v1/memory/retrieve"
    data = {
        "space_id": space_id,
        "query_text": query,
        "top_k": top_k
    }

    req = urllib.request.Request(
        url,
        data=json.dumps(data).encode('utf-8'),
        headers={'Content-Type': 'application/json'},
        method='POST'
    )

    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            result = json.loads(resp.read().decode('utf-8'))
            return result.get('results', [])
    except Exception as e:
        print(f"  MDEMG API error: {e}")
        return []


def format_mdemg_for_prompt(mdemg_results: List[Dict]) -> str:
    """Format MDEMG results for inclusion in prompt."""
    if not mdemg_results:
        return "No files found."

    lines = []
    for i, r in enumerate(mdemg_results[:5], 1):
        path = r.get('path', r.get('node_id', 'unknown'))
        score = r.get('score', 0)
        lines.append(f"{i}. {path} (score: {score:.3f})")

    return "\n".join(lines)


def run_cli_agent(
    question: Dict,
    mdemg_results: List[Dict],
    output_file: Path,
    repo_path: str,
    model: str
) -> bool:
    """Run Claude CLI to answer a single question."""

    question_id = question['id']
    question_text = question['question']
    question_escaped = question_text.replace('"', '\\"').replace('\n', ' ')

    mdemg_files = format_mdemg_for_prompt(mdemg_results)

    prompt = f"""Answer ONE question. Follow these steps exactly.

QUESTION ID: {question_id}
QUESTION: {question_text}

MDEMG FOUND THESE FILES:
{mdemg_files}

STEP 1: Read the top 2 files from MDEMG results above.
STEP 2: Find code that answers the question. Note file:line numbers.
STEP 3: Write answer to output file using this command:

printf '%s\\n' '{{"id": {question_id}, "question": "{question_escaped}", "answer": "YOUR_ANSWER", "files_consulted": ["file.ts"], "file_line_refs": ["file.ts:123"], "mdemg_used": true, "confidence": "HIGH"}}' >> {output_file}

START NOW - Read the first file from MDEMG results."""

    cmd = [
        "claude",
        "--model", model,
        "--print",
        "--no-session-persistence",
        "--no-chrome",
        "--allowedTools", "Read,Bash",
    ]

    try:
        result = subprocess.run(
            cmd,
            input=prompt,
            capture_output=True,
            text=True,
            timeout=300,
            cwd=repo_path
        )
        if result.returncode != 0:
            print(f"  CLI error (code {result.returncode})")
            if result.stderr:
                print(f"  stderr: {result.stderr[:200]}")
        else:
            print(f"  CLI completed OK")
        return result.returncode == 0
    except subprocess.TimeoutExpired:
        print(f"  Timeout on question {question_id}")
        return False
    except Exception as e:
        print(f"  Error: {e}")
        return False


def count_answers(output_file: Path) -> int:
    """Count valid answer lines in output file."""
    if not output_file.exists():
        return 0

    count = 0
    with open(output_file, 'r') as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    json.loads(line)
                    count += 1
                except:
                    pass
    return count


def run_benchmark(
    questions_file: Path,
    output_dir: Path,
    repo_path: str,
    codebase_name: str,
    mdemg_endpoint: str,
    space_id: str,
    model: str = "sonnet",
    start_from: int = 0
):
    """Run the benchmark, one question at a time."""

    # Load questions
    with open(questions_file, 'r') as f:
        data = json.load(f)
    questions = data.get('questions', data)

    print(f"Loaded {len(questions)} questions")

    # Setup output
    output_dir = Path(output_dir)
    mdemg_dir = output_dir / "mdemg"
    mdemg_dir.mkdir(parents=True, exist_ok=True)

    output_file = mdemg_dir / "run_1_answers.jsonl"

    # Track progress
    start_time = time.time()
    successful = 0
    failed = 0

    print(f"\nStarting benchmark at {datetime.now().strftime('%H:%M:%S')}")
    print(f"Output: {output_file}")
    print("-" * 60)

    for i, question in enumerate(questions):
        if i < start_from:
            continue

        q_id = question['id']
        q_text = question['question'][:60] + "..." if len(question['question']) > 60 else question['question']

        print(f"\n[{i+1}/{len(questions)}] Q{q_id}: {q_text}")

        # Step 1: Call MDEMG API
        print(f"  Calling MDEMG API...")
        mdemg_results = call_mdemg_api(
            mdemg_endpoint,
            space_id,
            question['question'],
            top_k=5
        )

        if mdemg_results:
            print(f"  MDEMG returned {len(mdemg_results)} results")
            for r in mdemg_results[:3]:
                path = r.get('path', r.get('node_id', '?'))
                score = r.get('score', 0)
                print(f"    - {Path(path).name} ({score:.3f})")
        else:
            print(f"  MDEMG returned no results")

        # Step 2: Run CLI agent with MDEMG results
        print(f"  Running CLI agent...")
        answers_before = count_answers(output_file)

        success = run_cli_agent(
            question=question,
            mdemg_results=mdemg_results,
            output_file=output_file,
            repo_path=repo_path,
            model=model
        )

        answers_after = count_answers(output_file)

        if answers_after > answers_before:
            print(f"  ✓ Answer written ({answers_after} total)")
            successful += 1
        else:
            print(f"  ✗ No answer written")
            failed += 1

        # Progress summary every 10 questions
        if (i + 1) % 10 == 0:
            elapsed = time.time() - start_time
            rate = (i + 1) / elapsed * 60
            print(f"\n  --- Progress: {i+1}/{len(questions)} ({successful} ok, {failed} failed) - {rate:.1f} q/min ---\n")

    # Final summary
    elapsed = time.time() - start_time
    print("\n" + "=" * 60)
    print(f"BENCHMARK COMPLETE")
    print(f"  Total questions: {len(questions)}")
    print(f"  Answers written: {count_answers(output_file)}")
    print(f"  Successful: {successful}")
    print(f"  Failed: {failed}")
    print(f"  Duration: {elapsed/60:.1f} minutes")
    print("=" * 60)

    return {
        'total': len(questions),
        'successful': successful,
        'failed': failed,
        'duration_seconds': elapsed
    }


def main():
    parser = argparse.ArgumentParser(description='Benchmark Runner V3 - Mechanical MDEMG Enforcement')
    parser.add_argument('--questions', required=True, help='Questions JSON file')
    parser.add_argument('--output-dir', required=True, help='Output directory')
    parser.add_argument('--codebase', required=True, help='Path to codebase')
    parser.add_argument('--codebase-name', required=True, help='Name of codebase')
    parser.add_argument('--mdemg-endpoint', default='http://localhost:9999', help='MDEMG API endpoint')
    parser.add_argument('--space-id', required=True, help='MDEMG space ID')
    parser.add_argument('--model', default='sonnet', help='Model to use (sonnet, opus, haiku)')
    parser.add_argument('--start-from', type=int, default=0, help='Start from question index (for resume)')

    args = parser.parse_args()

    result = run_benchmark(
        questions_file=Path(args.questions),
        output_dir=Path(args.output_dir),
        repo_path=args.codebase,
        codebase_name=args.codebase_name,
        mdemg_endpoint=args.mdemg_endpoint,
        space_id=args.space_id,
        model=args.model,
        start_from=args.start_from
    )

    # Write summary
    summary_file = Path(args.output_dir) / "mdemg" / "run_summary.json"
    with open(summary_file, 'w') as f:
        json.dump(result, f, indent=2)

    print(f"\nSummary written to: {summary_file}")


if __name__ == '__main__':
    main()
