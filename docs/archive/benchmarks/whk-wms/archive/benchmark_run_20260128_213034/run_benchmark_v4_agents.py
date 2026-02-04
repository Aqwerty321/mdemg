#!/usr/bin/env python3
"""
MDEMG Full Benchmark v4 - Agent-Based Answer Synthesis
Runs baseline + MDEMG with agent-synthesized answers.

This script prepares context files that Claude Code agents will use
to synthesize answers. Run this to generate context, then use Claude
to process each batch.
"""

import json
import subprocess
import time
import urllib.request
import re
import os
from datetime import datetime, timezone
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
    """Get current learning edge count from Neo4j directly."""
    try:
        result = subprocess.run(
            ['curl', '-s', '-X', 'POST', 'http://localhost:7474/db/neo4j/tx/commit',
             '-H', 'Content-Type: application/json',
             '-u', 'neo4j:testpassword',
             '-d', json.dumps({"statements": [{"statement":
                 f"MATCH (:MemoryNode {{space_id:'{SPACE_ID}'}})-[r:CO_ACTIVATED_WITH]-(:MemoryNode {{space_id:'{SPACE_ID}'}}) RETURN count(r) as edges"}]})],
            capture_output=True, text=True, timeout=10
        )
        if result.returncode == 0:
            data = json.loads(result.stdout)
            return data['results'][0]['data'][0]['row'][0]
    except:
        pass
    return 0

def mdemg_retrieve(query, top_k=10):
    """Call MDEMG retrieve API with evidence for line numbers."""
    try:
        req = urllib.request.Request(
            f"{MDEMG_ENDPOINT}/v1/memory/retrieve",
            data=json.dumps({
                "space_id": SPACE_ID,
                "query_text": query,
                "top_k": top_k,
                "include_evidence": True  # Get symbol line numbers
            }).encode(),
            headers={'Content-Type': 'application/json'}
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except Exception as e:
        return {"error": str(e), "results": []}

def extract_keywords(question):
    """Extract searchable keywords from question."""
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

    words = re.findall(r'\b[A-Za-z][A-Za-z0-9_]+\b', question)
    keywords = []
    for w in words:
        lower = w.lower()
        if lower not in stopwords and len(w) > 2:
            if any(c.isupper() for c in w[1:]):
                keywords.insert(0, w)
            else:
                keywords.append(w)
    return keywords[:5]

def baseline_grep_search(question, repo_path):
    """Use ripgrep to find relevant files and extract content."""
    keywords = extract_keywords(question)
    if not keywords:
        return [], []

    files_found = {}

    for kw in keywords[:3]:
        try:
            result = subprocess.run(
                ['rg', '-n', '-i', '--type', 'ts', '--type', 'js', '-C', '3', '-m', '5', kw, repo_path],
                capture_output=True, text=True, timeout=15
            )
            if result.returncode == 0:
                for line in result.stdout.strip().split('\n'):
                    if line.startswith(repo_path):
                        parts = line.split(':', 2)
                        if len(parts) >= 3:
                            file_path = parts[0]
                            line_num = parts[1]
                            content = parts[2]
                            if file_path not in files_found:
                                files_found[file_path] = {'lines': {}, 'first_line': int(line_num) if line_num.isdigit() else 1}
                            if line_num.isdigit():
                                files_found[file_path]['lines'][int(line_num)] = content
        except:
            pass

    file_contexts = []
    file_refs = []

    for file_path, data in list(files_found.items())[:5]:
        lines = data['lines']
        first_line = data['first_line']

        sorted_lines = sorted(lines.keys()) if lines else [first_line]
        if sorted_lines:
            start = max(1, min(sorted_lines) - 10)
            end = max(sorted_lines) + 10

            try:
                with open(file_path, 'r') as f:
                    all_lines = f.readlines()
                    snippet = ''.join(all_lines[start-1:min(end, len(all_lines))])[:2000]
                    rel_path = file_path.replace(repo_path, '')
                    file_contexts.append({
                        'path': rel_path,
                        'line': first_line,
                        'snippet': snippet
                    })
                    file_refs.append(f"{rel_path}:{first_line}")
            except:
                pass

    return file_contexts, file_refs

def mdemg_get_context(question):
    """Get context from MDEMG retrieval with real line numbers from evidence."""
    retrieval = mdemg_retrieve(question, top_k=10)
    results = retrieval.get('results', [])

    file_contexts = []
    file_refs = []

    for r in results[:5]:
        path = r.get('path', '')
        name = r.get('name', '')
        summary = r.get('summary', '')
        evidence = r.get('evidence', [])

        if path:
            # Extract file path without fragment (e.g., /path/file.ts#ClassName -> /path/file.ts)
            base_path = path.split('#')[0] if '#' in path else path

            # Get line number from evidence if available
            line_num = 1
            if evidence:
                # Use first evidence's line number
                line_num = evidence[0].get('line_number', 1)

            file_contexts.append({
                'path': path,
                'name': name,
                'summary': summary,
                'score': r.get('score', 0),
                'evidence': evidence  # Include full evidence
            })
            file_refs.append(f"{base_path}:{line_num}")

    return file_contexts, file_refs

def prepare_batch(questions, run_type, run_num, output_dir):
    """Prepare a batch of questions with retrieval context."""
    print(f"Preparing {run_type} run {run_num}...")

    batch = []
    edges_before = get_edge_count()

    for i, q in enumerate(questions):
        qid = q['id']
        question = q['question']

        if run_type == "mdemg":
            file_contexts, file_refs = mdemg_get_context(question)
            context_type = "summaries"
        else:
            file_contexts, file_refs = baseline_grep_search(question, REPO_PATH)
            context_type = "code"

        batch.append({
            'id': qid,
            'question': question,
            'context': file_contexts,
            'file_refs': file_refs,
            'context_type': context_type
        })

        if (i + 1) % 20 == 0:
            print(f"  Prepared {i+1}/{len(questions)}")

    edges_after = get_edge_count()

    # Save batch
    batch_file = output_dir / f"batch_{run_type}_run{run_num}.json"
    with open(batch_file, 'w') as f:
        json.dump({
            'run_type': run_type,
            'run_num': run_num,
            'edges_before': edges_before,
            'edges_after': edges_after,
            'questions': batch
        }, f, indent=2)

    print(f"  Saved to {batch_file}")
    print(f"  Edges: {edges_before} -> {edges_after}")
    return batch_file

def main():
    print("="*60)
    print("MDEMG Benchmark V4 - Preparing Agent Batches")
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

    # Prepare batches
    batches = []

    # 3 baseline runs
    for run_num in range(1, 4):
        batch_file = prepare_batch(questions, "baseline", run_num, OUTPUT_DIR)
        batches.append(batch_file)

    # 3 MDEMG runs
    for run_num in range(1, 4):
        batch_file = prepare_batch(questions, "mdemg", run_num, OUTPUT_DIR)
        batches.append(batch_file)

    print("\n" + "="*60)
    print("BATCHES PREPARED")
    print("="*60)
    print("\nBatch files created:")
    for b in batches:
        print(f"  {b}")

    print("\nNext: Use Claude agents to process each batch")
    print("Run: process_batch.py <batch_file> to generate answers")

if __name__ == "__main__":
    main()
