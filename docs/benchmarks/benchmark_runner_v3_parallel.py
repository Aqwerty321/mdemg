#!/usr/bin/env python3
"""
Benchmark Runner V3 Parallel - Spawns multiple agents for speed

Splits questions into batches and runs them in parallel.
Example: 120 questions / 12 agents = 10 questions each
Expected time: ~10 minutes instead of ~100 minutes
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
from concurrent.futures import ProcessPoolExecutor, as_completed
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
        return []


def format_mdemg_for_prompt(mdemg_results: List[Dict]) -> str:
    """Format MDEMG results for inclusion in prompt.

    Transforms MDEMG paths to be relative to cwd:
    - Strips leading '/' to make paths relative
    - Strips '#SymbolName' suffixes from symbol-level nodes
    """
    if not mdemg_results:
        return "No files found."

    lines = []
    seen_paths = set()
    for i, r in enumerate(mdemg_results[:5], 1):
        path = r.get('path', r.get('node_id', 'unknown'))
        score = r.get('score', 0)

        # Strip symbol suffix (e.g., "file.ts#ClassName" -> "file.ts")
        if '#' in path:
            path = path.split('#')[0]

        # Strip leading '/' to make path relative to cwd
        if path.startswith('/'):
            path = path[1:]

        # Deduplicate (same file may appear for multiple symbols)
        if path in seen_paths:
            continue
        seen_paths.add(path)

        lines.append(f"{i}. {path} (score: {score:.3f})")

    return "\n".join(lines)


def run_single_question(
    question: Dict,
    mdemg_endpoint: str,
    space_id: str,
    output_file: Path,
    repo_path: str,
    model: str
) -> Dict:
    """Process a single question and return result."""

    question_id = question['id']
    question_text = question['question']
    question_escaped = question_text.replace('"', '\\"').replace('\n', ' ')

    # Call MDEMG API
    mdemg_results = call_mdemg_api(mdemg_endpoint, space_id, question_text, top_k=5)
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
        return {
            'question_id': question_id,
            'success': result.returncode == 0,
            'error': None
        }
    except subprocess.TimeoutExpired:
        return {'question_id': question_id, 'success': False, 'error': 'timeout'}
    except Exception as e:
        return {'question_id': question_id, 'success': False, 'error': str(e)}


def run_batch(
    batch_id: int,
    questions: List[Dict],
    mdemg_endpoint: str,
    space_id: str,
    output_dir: Path,
    repo_path: str,
    model: str
) -> Dict:
    """Run a batch of questions sequentially."""

    output_file = output_dir / f"batch_{batch_id}_answers.jsonl"
    results = []

    for i, question in enumerate(questions):
        print(f"  [Batch {batch_id}] Q{i+1}/{len(questions)}: {question['id']}")
        result = run_single_question(
            question=question,
            mdemg_endpoint=mdemg_endpoint,
            space_id=space_id,
            output_file=output_file,
            repo_path=repo_path,
            model=model
        )
        results.append(result)

    return {
        'batch_id': batch_id,
        'total': len(questions),
        'successful': sum(1 for r in results if r['success']),
        'failed': sum(1 for r in results if not r['success']),
        'output_file': str(output_file)
    }


def merge_batch_files(output_dir: Path, num_batches: int) -> Path:
    """Merge all batch output files into one."""
    merged_file = output_dir / "run_1_answers.jsonl"

    with open(merged_file, 'w') as outf:
        for batch_id in range(num_batches):
            batch_file = output_dir / f"batch_{batch_id}_answers.jsonl"
            if batch_file.exists():
                with open(batch_file, 'r') as inf:
                    outf.write(inf.read())

    return merged_file


def run_parallel_benchmark(
    questions_file: Path,
    output_dir: Path,
    repo_path: str,
    mdemg_endpoint: str,
    space_id: str,
    model: str = "sonnet",
    num_workers: int = 12
):
    """Run benchmark with parallel workers."""

    # Load questions
    with open(questions_file, 'r') as f:
        data = json.load(f)
    questions = data.get('questions', data)

    print(f"Loaded {len(questions)} questions")
    print(f"Running with {num_workers} parallel workers")

    # Setup output
    output_dir = Path(output_dir)
    mdemg_dir = output_dir / "mdemg"
    mdemg_dir.mkdir(parents=True, exist_ok=True)

    # Split into batches
    batch_size = (len(questions) + num_workers - 1) // num_workers
    batches = []
    for i in range(0, len(questions), batch_size):
        batches.append(questions[i:i + batch_size])

    print(f"Split into {len(batches)} batches of ~{batch_size} questions each")

    start_time = time.time()
    print(f"\nStarting parallel benchmark at {datetime.now().strftime('%H:%M:%S')}")
    print("-" * 60)

    # Run batches in parallel
    batch_results = []
    with ProcessPoolExecutor(max_workers=num_workers) as executor:
        futures = {}
        for batch_id, batch_questions in enumerate(batches):
            future = executor.submit(
                run_batch,
                batch_id,
                batch_questions,
                mdemg_endpoint,
                space_id,
                mdemg_dir,
                repo_path,
                model
            )
            futures[future] = batch_id

        for future in as_completed(futures):
            batch_id = futures[future]
            try:
                result = future.result()
                batch_results.append(result)
                print(f"\n=== Batch {batch_id} complete: {result['successful']}/{result['total']} successful ===")
            except Exception as e:
                print(f"\n=== Batch {batch_id} failed: {e} ===")
                batch_results.append({
                    'batch_id': batch_id,
                    'total': len(batches[batch_id]),
                    'successful': 0,
                    'failed': len(batches[batch_id]),
                    'error': str(e)
                })

    # Merge results
    print("\nMerging batch files...")
    merged_file = merge_batch_files(mdemg_dir, len(batches))

    # Count final answers
    answer_count = 0
    if merged_file.exists():
        with open(merged_file, 'r') as f:
            for line in f:
                if line.strip():
                    try:
                        json.loads(line)
                        answer_count += 1
                    except:
                        pass

    elapsed = time.time() - start_time
    total_successful = sum(r['successful'] for r in batch_results)
    total_failed = sum(r['failed'] for r in batch_results)

    print("\n" + "=" * 60)
    print("PARALLEL BENCHMARK COMPLETE")
    print(f"  Total questions: {len(questions)}")
    print(f"  Answers written: {answer_count}")
    print(f"  Successful: {total_successful}")
    print(f"  Failed: {total_failed}")
    print(f"  Duration: {elapsed/60:.1f} minutes")
    print(f"  Speed: {len(questions)/elapsed*60:.1f} q/min")
    print("=" * 60)

    summary = {
        'total': len(questions),
        'successful': total_successful,
        'failed': total_failed,
        'duration_seconds': elapsed,
        'num_workers': num_workers,
        'batch_results': batch_results
    }

    summary_file = mdemg_dir / "run_summary.json"
    with open(summary_file, 'w') as f:
        json.dump(summary, f, indent=2)

    print(f"\nSummary: {summary_file}")
    print(f"Answers: {merged_file}")

    return summary


def main():
    parser = argparse.ArgumentParser(description='Benchmark Runner V3 Parallel')
    parser.add_argument('--questions', required=True, help='Questions JSON file')
    parser.add_argument('--output-dir', required=True, help='Output directory')
    parser.add_argument('--codebase', required=True, help='Path to codebase')
    parser.add_argument('--mdemg-endpoint', default='http://localhost:9999', help='MDEMG API endpoint')
    parser.add_argument('--space-id', required=True, help='MDEMG space ID')
    parser.add_argument('--model', default='sonnet', help='Model to use')
    parser.add_argument('--workers', type=int, default=12, help='Number of parallel workers')

    args = parser.parse_args()

    run_parallel_benchmark(
        questions_file=Path(args.questions),
        output_dir=Path(args.output_dir),
        repo_path=args.codebase,
        mdemg_endpoint=args.mdemg_endpoint,
        space_id=args.space_id,
        model=args.model,
        num_workers=args.workers
    )


if __name__ == '__main__':
    main()
