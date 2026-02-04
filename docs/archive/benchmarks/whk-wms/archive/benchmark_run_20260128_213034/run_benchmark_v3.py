#!/usr/bin/env python3
"""
MDEMG Full Benchmark v3 - Standardized Framework
Runs 3 baseline + 3 MDEMG runs with proper isolation and grading.

Baseline: Uses ripgrep/glob-based code search (simulates agent search)
MDEMG: Uses MDEMG semantic retrieval with learning edges
"""

import json
import subprocess
import time
import urllib.request
import statistics
import re
import os
from datetime import datetime
from pathlib import Path

# Configuration
MDEMG_ENDPOINT = "http://localhost:8090"
SPACE_ID = "whk-wms"
REPO_PATH = "/Users/reh3376/whk-wms/apps/whk-wms"
BASE_DIR = Path(__file__).parent.parent
QUESTIONS_FILE = BASE_DIR / "test_questions_120_agent.json"
MASTER_FILE = BASE_DIR / "test_questions_120.json"
OUTPUT_DIR = Path(__file__).parent
GRADING_SCRIPT = BASE_DIR.parent / "grade_answers_v3.py"

def get_edge_count():
    """Get current learning edge count."""
    try:
        req = urllib.request.Request(
            f"{MDEMG_ENDPOINT}/v1/memory/retrieve",
            data=json.dumps({"space_id": SPACE_ID, "query_text": "test", "top_k": 1}).encode(),
            headers={'Content-Type': 'application/json'}
        )
        with urllib.request.urlopen(req, timeout=10) as resp:
            data = json.loads(resp.read().decode('utf-8'))
            return data.get('learning_activity', {}).get('co_activated_edges', 0)
    except:
        return 0

def mdemg_retrieve(query, top_k=10):
    """Call MDEMG retrieve API."""
    try:
        req = urllib.request.Request(
            f"{MDEMG_ENDPOINT}/v1/memory/retrieve",
            data=json.dumps({
                "space_id": SPACE_ID,
                "query_text": query,
                "top_k": top_k
            }).encode(),
            headers={'Content-Type': 'application/json'}
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except Exception as e:
        return {"error": str(e), "results": []}

def synthesize_answer_mdemg(question, retrieval_results):
    """Synthesize answer from MDEMG retrieval results."""
    results = retrieval_results.get('results', [])
    if not results:
        return "No relevant information found in memory.", [], []

    # Take top 5 results
    top_results = results[:5]

    # Build structured answer
    answer_parts = []
    file_refs = []
    files_consulted = []

    for r in top_results:
        name = r.get('name', '')
        path = r.get('path', '')
        summary = r.get('summary', '')[:300]
        score = r.get('score', 0)

        if path:
            files_consulted.append(path)
            # Add line reference (estimate from node position)
            file_refs.append(f"{path}:1")
            if summary:
                answer_parts.append(f"[{name}] {summary}")

    if answer_parts:
        answer = " | ".join(answer_parts[:3])
    else:
        answer = "Retrieved files but no summaries available."

    return answer, file_refs, files_consulted

def extract_keywords(question):
    """Extract searchable keywords from question."""
    # Remove common words
    stopwords = {'the', 'a', 'an', 'is', 'are', 'was', 'were', 'be', 'been', 'being',
                 'have', 'has', 'had', 'do', 'does', 'did', 'will', 'would', 'could',
                 'should', 'may', 'might', 'must', 'shall', 'can', 'need', 'dare',
                 'ought', 'used', 'to', 'of', 'in', 'for', 'on', 'with', 'at', 'by',
                 'from', 'as', 'into', 'through', 'during', 'before', 'after',
                 'above', 'below', 'between', 'under', 'again', 'further', 'then',
                 'once', 'here', 'there', 'when', 'where', 'why', 'how', 'all',
                 'each', 'few', 'more', 'most', 'other', 'some', 'such', 'no', 'nor',
                 'not', 'only', 'own', 'same', 'so', 'than', 'too', 'very', 'just',
                 'and', 'but', 'if', 'or', 'because', 'until', 'while', 'what',
                 'which', 'who', 'this', 'that', 'these', 'those', 'it', 'its'}

    # Extract words
    words = re.findall(r'\b[A-Za-z][A-Za-z0-9_]+\b', question)

    # Filter and prioritize
    keywords = []
    for w in words:
        lower = w.lower()
        if lower not in stopwords and len(w) > 2:
            # Prioritize PascalCase/camelCase (likely code symbols)
            if any(c.isupper() for c in w[1:]):
                keywords.insert(0, w)
            else:
                keywords.append(w)

    return keywords[:5]  # Top 5 keywords

def baseline_grep_search(question, repo_path):
    """Baseline: Use ripgrep to search code directly."""
    keywords = extract_keywords(question)
    if not keywords:
        return "No searchable keywords found.", [], []

    files_found = set()
    file_lines = {}

    for kw in keywords[:3]:  # Search top 3 keywords
        try:
            result = subprocess.run(
                ['rg', '-l', '-i', '--type', 'ts', '--type', 'js', kw, repo_path],
                capture_output=True, text=True, timeout=10
            )
            if result.returncode == 0:
                for line in result.stdout.strip().split('\n'):
                    if line:
                        files_found.add(line)
        except:
            pass

        # Also get line numbers for first keyword match
        if not file_lines:
            try:
                result = subprocess.run(
                    ['rg', '-n', '-i', '--type', 'ts', '--type', 'js', '-m', '3', kw, repo_path],
                    capture_output=True, text=True, timeout=10
                )
                if result.returncode == 0:
                    for line in result.stdout.strip().split('\n')[:5]:
                        if ':' in line:
                            parts = line.split(':', 2)
                            if len(parts) >= 2:
                                file_path = parts[0]
                                line_num = parts[1]
                                if file_path not in file_lines:
                                    file_lines[file_path] = line_num
            except:
                pass

    if not files_found:
        return f"No files found for keywords: {keywords}", [], []

    # Build answer from file names
    files_list = list(files_found)[:5]
    file_refs = []
    for f in files_list:
        if f in file_lines:
            file_refs.append(f"{f}:{file_lines[f]}")
        else:
            file_refs.append(f"{f}:1")

    # Basic answer
    basenames = [os.path.basename(f) for f in files_list[:3]]
    answer = f"Found in files: {', '.join(basenames)}. Keywords searched: {', '.join(keywords[:3])}"

    return answer, file_refs, files_list

def synthesize_answer_baseline(question, repo_path):
    """Baseline synthesis using grep search."""
    return baseline_grep_search(question, repo_path)

def run_single_benchmark(run_type, run_num, questions, output_file):
    """Run a single benchmark (baseline or mdemg)."""
    print(f"\n{'='*60}")
    print(f"Starting {run_type.upper()} Run {run_num}")
    print(f"Output: {output_file}")
    print(f"{'='*60}")

    edges_before = get_edge_count()
    start_time = time.time()

    results = []
    for i, q in enumerate(questions):
        qid = q['id']
        question = q['question']

        if run_type == "mdemg":
            # MDEMG: Use semantic retrieval
            retrieval = mdemg_retrieve(question, top_k=10)
            answer, file_refs, files_consulted = synthesize_answer_mdemg(question, retrieval)
            confidence = "HIGH" if file_refs else "LOW"
        else:
            # Baseline: Use grep-based search (no MDEMG)
            answer, file_refs, files_consulted = synthesize_answer_baseline(question, REPO_PATH)
            confidence = "MEDIUM" if file_refs else "LOW"

        result = {
            "id": qid,
            "question": question,
            "answer": answer,
            "files_consulted": files_consulted[:5],
            "file_line_refs": file_refs[:5],
            "confidence": confidence
        }
        results.append(result)

        if (i + 1) % 20 == 0:
            elapsed = time.time() - start_time
            rate = (i + 1) / elapsed if elapsed > 0 else 0
            print(f"  Progress: {i+1}/{len(questions)} ({rate:.2f} q/s)")

    # Write results
    with open(output_file, 'w') as f:
        for r in results:
            f.write(json.dumps(r) + '\n')

    edges_after = get_edge_count()
    duration = time.time() - start_time

    print(f"  Completed in {duration:.1f}s, edges: {edges_before} -> {edges_after} (+{edges_after - edges_before})")

    return {
        "run_id": f"{run_type}_run{run_num}",
        "type": run_type,
        "sequence": run_num,
        "status": "valid",
        "completion": {
            "questions_answered": len(results),
            "questions_expected": len(questions),
            "completion_rate": 1.0
        },
        "timing": {
            "duration_seconds": round(duration, 1)
        },
        "output": {
            "file_path": str(output_file)
        },
        "learning_edges": {
            "before": edges_before,
            "after": edges_after,
            "delta": edges_after - edges_before
        }
    }

def grade_run(answers_file, master_file, grades_file):
    """Grade a run using grade_answers_v3.py."""
    try:
        result = subprocess.run(
            ['python3', str(GRADING_SCRIPT), str(answers_file), str(master_file), str(grades_file)],
            capture_output=True,
            text=True,
            timeout=120
        )
        if result.returncode == 0 and grades_file.exists():
            with open(grades_file) as f:
                grades = json.load(f)
            agg = grades.get('aggregate', {})
            return {
                "graded": True,
                "grades_file": str(grades_file),
                "mean_score": agg.get("mean", 0),
                "std": agg.get("std", 0),
                "high_score_rate": agg.get("high_score_rate", 0),
                "evidence_rate": agg.get("evidence_rate", 0),
                "strong_evidence_count": agg.get("strong_evidence_count", 0),
                "weak_evidence_count": agg.get("weak_evidence_count", 0),
                "no_evidence_count": agg.get("no_evidence_count", 0)
            }
        else:
            print(f"  Grading stderr: {result.stderr[:200]}")
    except Exception as e:
        print(f"  Grading error: {e}")
    return {"graded": False, "mean_score": 0}

def main():
    print("="*60)
    print("MDEMG FULL BENCHMARK V3 - Standardized Framework")
    print("="*60)

    # Check MDEMG server
    try:
        req = urllib.request.urlopen(f"{MDEMG_ENDPOINT}/readyz", timeout=5)
        print(f"MDEMG Server: OK")
    except Exception as e:
        print(f"ERROR: MDEMG server not available at {MDEMG_ENDPOINT}: {e}")
        return

    # Load questions
    with open(QUESTIONS_FILE) as f:
        questions = json.load(f)['questions']
    print(f"Questions: {len(questions)}")
    print(f"Output directory: {OUTPUT_DIR}")

    all_runs = []

    # Run 3 baseline runs
    print("\n" + "="*60)
    print("PHASE 1: BASELINE RUNS (3x)")
    print("="*60)
    for run_num in range(1, 4):
        output_file = OUTPUT_DIR / f"answers_baseline_run{run_num}.jsonl"
        run_data = run_single_benchmark("baseline", run_num, questions, output_file)

        # Grade
        grades_file = OUTPUT_DIR / f"grades_baseline_run{run_num}.json"
        run_data["grading"] = grade_run(output_file, MASTER_FILE, grades_file)

        all_runs.append(run_data)
        print(f"  Baseline Run {run_num}: score={run_data['grading'].get('mean_score', 'N/A'):.3f}")

    # Run 3 MDEMG runs (sequential for learning edge accumulation)
    print("\n" + "="*60)
    print("PHASE 2: MDEMG RUNS (3x sequential)")
    print("="*60)
    for run_num in range(1, 4):
        output_file = OUTPUT_DIR / f"answers_mdemg_run{run_num}.jsonl"
        run_data = run_single_benchmark("mdemg", run_num, questions, output_file)

        # Grade
        grades_file = OUTPUT_DIR / f"grades_mdemg_run{run_num}.json"
        run_data["grading"] = grade_run(output_file, MASTER_FILE, grades_file)

        all_runs.append(run_data)
        edges = run_data.get('learning_edges', {})
        print(f"  MDEMG Run {run_num}: score={run_data['grading'].get('mean_score', 'N/A'):.3f}, edges={edges.get('before', 0)}->{edges.get('after', 0)}")

    # Calculate aggregates
    baseline_runs = [r for r in all_runs if r['type'] == 'baseline' and r.get('grading', {}).get('graded')]
    mdemg_runs = [r for r in all_runs if r['type'] == 'mdemg' and r.get('grading', {}).get('graded')]

    baseline_scores = [r['grading']['mean_score'] for r in baseline_runs]
    mdemg_scores = [r['grading']['mean_score'] for r in mdemg_runs]

    baseline_mean = statistics.mean(baseline_scores) if baseline_scores else 0
    baseline_std = statistics.stdev(baseline_scores) if len(baseline_scores) > 1 else 0
    mdemg_mean = statistics.mean(mdemg_scores) if mdemg_scores else 0
    mdemg_std = statistics.stdev(mdemg_scores) if len(mdemg_scores) > 1 else 0

    delta = mdemg_mean - baseline_mean
    delta_pct = (delta / baseline_mean * 100) if baseline_mean else 0

    # Generate summary
    summary = {
        "$schema": "benchmark_summary_v3",
        "metadata": {
            "benchmark_id": f"whk-wms-{datetime.now().strftime('%Y%m%d-%H%M%S')}",
            "date": datetime.utcnow().isoformat() + "Z",
            "framework_version": "3.0",
            "operator": "claude-code"
        },
        "environment": {
            "mdemg_endpoint": MDEMG_ENDPOINT,
            "target_repo": {
                "name": "whk-wms",
                "path": REPO_PATH
            },
            "space_id": SPACE_ID
        },
        "configuration": {
            "question_file": str(QUESTIONS_FILE),
            "question_count": len(questions),
            "runs_per_type": 3
        },
        "runs": all_runs,
        "aggregate": {
            "baseline": {
                "valid_runs": len(baseline_runs),
                "mean_score": round(baseline_mean, 4),
                "std_dev": round(baseline_std, 4),
                "scores": [round(s, 4) for s in baseline_scores]
            },
            "mdemg": {
                "valid_runs": len(mdemg_runs),
                "mean_score": round(mdemg_mean, 4),
                "std_dev": round(mdemg_std, 4),
                "scores": [round(s, 4) for s in mdemg_scores],
                "total_learning_edges": mdemg_runs[-1]['learning_edges']['after'] if mdemg_runs else 0
            },
            "comparison": {
                "score_delta": round(delta, 4),
                "score_delta_percent": round(delta_pct, 2)
            }
        }
    }

    # Write summary
    summary_file = OUTPUT_DIR / "BENCHMARK_SUMMARY_V3.json"
    with open(summary_file, 'w') as f:
        json.dump(summary, f, indent=2)

    # Print final results
    print("\n" + "="*60)
    print("BENCHMARK COMPLETE")
    print("="*60)
    print(f"\nBASELINE: {baseline_mean:.3f} +/- {baseline_std:.3f} (n={len(baseline_runs)})")
    print(f"MDEMG:    {mdemg_mean:.3f} +/- {mdemg_std:.3f} (n={len(mdemg_runs)})")
    print(f"DELTA:    {delta:+.3f} ({delta_pct:+.1f}%)")
    print(f"\nSummary: {summary_file}")

    # Generate markdown report
    report_file = OUTPUT_DIR / "BENCHMARK_RESULTS_V3.md"
    with open(report_file, 'w') as f:
        f.write(f"# MDEMG Benchmark Results - whk-wms\n\n")
        f.write(f"**Date:** {datetime.now().strftime('%Y-%m-%d %H:%M')}\n")
        f.write(f"**Framework:** V3.0\n")
        f.write(f"**Questions:** {len(questions)}\n\n")
        f.write(f"## Summary\n\n")
        f.write(f"| Metric | Baseline | MDEMG | Delta |\n")
        f.write(f"|--------|----------|-------|-------|\n")
        f.write(f"| Mean Score | {baseline_mean:.3f} | {mdemg_mean:.3f} | {delta:+.3f} ({delta_pct:+.1f}%) |\n")
        f.write(f"| Std Dev | {baseline_std:.3f} | {mdemg_std:.3f} | - |\n")
        f.write(f"| Valid Runs | {len(baseline_runs)} | {len(mdemg_runs)} | - |\n\n")

        f.write(f"## Individual Runs\n\n")
        f.write(f"| Run | Type | Score | Std | Evidence Rate | Edges |\n")
        f.write(f"|-----|------|-------|-----|---------------|-------|\n")
        for r in all_runs:
            score = r.get('grading', {}).get('mean_score', 0)
            std = r.get('grading', {}).get('std', 0)
            ev_rate = r.get('grading', {}).get('evidence_rate', 0)
            edges = r.get('learning_edges', {})
            edge_str = f"{edges.get('before', '-')}->{edges.get('after', '-')}" if edges else "-"
            f.write(f"| {r['run_id']} | {r['type']} | {score:.3f} | {std:.3f} | {ev_rate:.2f} | {edge_str} |\n")

        f.write(f"\n## Statistical Analysis\n\n")
        f.write(f"- **Baseline Mean:** {baseline_mean:.4f} (σ={baseline_std:.4f})\n")
        f.write(f"- **MDEMG Mean:** {mdemg_mean:.4f} (σ={mdemg_std:.4f})\n")
        f.write(f"- **Absolute Improvement:** {delta:+.4f}\n")
        f.write(f"- **Relative Improvement:** {delta_pct:+.2f}%\n")

        if baseline_std > 0 and mdemg_std > 0:
            # Calculate effect size (Cohen's d)
            pooled_std = ((baseline_std**2 + mdemg_std**2) / 2) ** 0.5
            cohens_d = delta / pooled_std if pooled_std > 0 else 0
            f.write(f"- **Effect Size (Cohen's d):** {cohens_d:.2f}\n")

        f.write(f"\n## Learning Edge Analysis\n\n")
        if mdemg_runs:
            first_edges = mdemg_runs[0]['learning_edges']
            last_edges = mdemg_runs[-1]['learning_edges']
            f.write(f"- **Initial Edges:** {first_edges['before']}\n")
            f.write(f"- **Final Edges:** {last_edges['after']}\n")
            f.write(f"- **Total New Edges:** {last_edges['after'] - first_edges['before']}\n")

    print(f"Report: {report_file}")
    return summary

if __name__ == "__main__":
    main()
