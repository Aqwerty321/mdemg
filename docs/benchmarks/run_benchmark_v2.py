#!/usr/bin/env python3
"""
MDEMG Benchmark Orchestration Script v2.0

This script is designed to be called by a Claude orchestrator that will spawn
Task agents to answer benchmark questions. It handles:
- Question file preparation
- Output file management
- Grading after agents complete
- Statistical analysis and reporting

The actual agent spawning is done by the Claude orchestrator using Task tool calls.

Usage:
    # Prepare benchmark session
    python run_benchmark_v2.py prepare \\
        --questions test_questions_120.json \\
        --questions-agent test_questions_120_agent.json \\
        --space-id whk-wms \\
        --repo-path /Users/reh3376/whk-wms \\
        --output-dir /tmp/benchmark_run_001

    # Grade a completed run
    python run_benchmark_v2.py grade \\
        --answers /tmp/benchmark_run_001/baseline/run_1_answers.jsonl \\
        --questions test_questions_120.json \\
        --output /tmp/benchmark_run_001/baseline/run_1_grades.json

    # Analyze all runs
    python run_benchmark_v2.py analyze \\
        --output-dir /tmp/benchmark_run_001
"""

import argparse
import json
import sys
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional

# Import local modules
from grader_v4 import Grader, load_answers_jsonl
from stats_analyzer import StatsAnalyzer
from report_generator import ReportGenerator, BenchmarkConfig


# =============================================================================
# Prompt Templates
# =============================================================================

BASELINE_PROMPT_TEMPLATE = """You are answering benchmark questions about the {codebase_name} codebase.

## STRICT RULES - VIOLATION = DISQUALIFICATION
1. You may ONLY access files within: {repo_path}
2. You may NOT use WebSearch or WebFetch
3. You MUST answer ALL {question_count} questions
4. You MUST include file:line references in every answer
5. Use EXACT question ID from input - DO NOT renumber

## AVAILABLE TOOLS
- Read: Read source files
- Glob: Find files by pattern
- Grep: Search file contents
- Bash: ONLY for file operations (ls, find) - NO curl, NO wget, NO network

## WORKFLOW
For each question in {questions_file}:
1. Search for relevant files using Glob/Grep
2. Read source code to find the answer
3. Write ONE answer immediately to {output_file} in JSONL format

## OUTPUT FORMAT (JSONL - one JSON per line)
{{"id": N, "question": "...", "answer": "...", "files_consulted": ["file.ts"], "file_line_refs": ["file.ts:123"], "confidence": "HIGH|MEDIUM|LOW"}}

## EVIDENCE REQUIREMENTS
- ALWAYS include file:line citations
- Example answer: "The value is 100, defined in config.ts:42"
- If uncertain, use confidence: "LOW"

## BEGIN
Read {questions_file} and answer ALL questions sequentially.
Write each answer to {output_file} immediately after finding it.
"""

MDEMG_PROMPT_TEMPLATE = """You are answering benchmark questions about the {codebase_name} codebase using MDEMG retrieval.

## STRICT RULES - VIOLATION = DISQUALIFICATION
1. You may ONLY access files within: {repo_path}
2. You may NOT use WebSearch or WebFetch
3. You MUST query MDEMG for EVERY question before manual search
4. You MUST answer ALL {question_count} questions
5. You MUST include file:line references in every answer
6. Use EXACT question ID from input - DO NOT renumber

## AVAILABLE TOOLS
- Read: Read source files
- Glob: Find files by pattern
- Grep: Search file contents
- Bash: For MDEMG API calls AND file operations

## MDEMG USAGE (REQUIRED for every question)
```bash
curl -s -X POST "{mdemg_endpoint}/v1/memory/retrieve" \\
  -H "Content-Type: application/json" \\
  -d '{{"space_id": "{space_id}", "query_text": "<question>", "top_k": 10}}'
```

## WORKFLOW
For each question in {questions_file}:
1. Query MDEMG with the question text
2. Read source files from MDEMG results
3. Synthesize YOUR OWN answer (do not dump raw MDEMG output)
4. Write ONE answer immediately to {output_file} in JSONL format

## OUTPUT FORMAT (JSONL - one JSON per line)
{{"id": N, "question": "...", "answer": "...", "files_consulted": ["file.ts"], "file_line_refs": ["file.ts:123"], "mdemg_used": true, "confidence": "HIGH|MEDIUM|LOW"}}

## EVIDENCE REQUIREMENTS
- ALWAYS include file:line citations
- MDEMG results often include file:line hints - verify by reading the files
- Example answer: "The value is 100, defined in config.ts:42"

## BEGIN
Read {questions_file} and answer ALL questions sequentially.
For EACH question: query MDEMG first, then read files, then write answer.
"""


def prepare_benchmark(args) -> Dict:
    """Prepare benchmark session by creating output directories and config."""
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    (output_dir / 'baseline').mkdir(exist_ok=True)
    (output_dir / 'mdemg').mkdir(exist_ok=True)

    # Load questions to get count
    with open(args.questions) as f:
        data = json.load(f)
    questions = data.get('questions', data) if isinstance(data, dict) else data
    question_count = len(questions)

    repo_name = Path(args.repo_path).name

    # Generate prompts for orchestrator
    baseline_prompt = BASELINE_PROMPT_TEMPLATE.format(
        codebase_name=repo_name,
        repo_path=args.repo_path,
        question_count=question_count,
        questions_file=args.questions_agent or args.questions,
        output_file="{output_file}"  # Placeholder for orchestrator to fill
    )

    mdemg_prompt = MDEMG_PROMPT_TEMPLATE.format(
        codebase_name=repo_name,
        repo_path=args.repo_path,
        question_count=question_count,
        questions_file=args.questions_agent or args.questions,
        output_file="{output_file}",  # Placeholder for orchestrator to fill
        mdemg_endpoint=args.endpoint,
        space_id=args.space_id
    )

    config = {
        'benchmark_id': f"{args.space_id}-{datetime.now().strftime('%Y%m%d-%H%M%S')}",
        'created_at': datetime.utcnow().isoformat() + 'Z',
        'framework_version': '2.0',
        'repo_path': args.repo_path,
        'repo_name': repo_name,
        'space_id': args.space_id,
        'mdemg_endpoint': args.endpoint,
        'questions_master': str(Path(args.questions).absolute()),
        'questions_agent': str(Path(args.questions_agent).absolute()) if args.questions_agent else None,
        'question_count': question_count,
        'runs_per_mode': args.runs,
        'output_dir': str(output_dir.absolute()),
        'prompts': {
            'baseline_template': baseline_prompt,
            'mdemg_template': mdemg_prompt
        }
    }

    # Save config
    with open(output_dir / 'config.json', 'w') as f:
        json.dump(config, f, indent=2)

    # Copy agent questions to output dir
    if args.questions_agent:
        import shutil
        shutil.copy(args.questions_agent, output_dir / 'agent_questions.json')

    print(f"Benchmark prepared: {output_dir}")
    print(f"  Questions: {question_count}")
    print(f"  Space: {args.space_id}")
    print(f"  Repo: {args.repo_path}")
    print(f"\nOrchestrator should:")
    print(f"  1. Run baseline agents with prompts from config.json")
    print(f"  2. Run MDEMG agents sequentially")
    print(f"  3. Grade with: python run_benchmark_v2.py grade --answers <file> --questions {args.questions}")
    print(f"  4. Analyze with: python run_benchmark_v2.py analyze --output-dir {output_dir}")

    return config


def grade_run(args) -> Dict:
    """Grade a single benchmark run."""
    answers_file = Path(args.answers)
    questions_file = Path(args.questions)
    output_file = Path(args.output)

    if not answers_file.exists():
        print(f"ERROR: Answers file not found: {answers_file}")
        sys.exit(1)

    # Load questions (master with expected answers)
    with open(questions_file) as f:
        data = json.load(f)
    questions = data.get('questions', data) if isinstance(data, dict) else data

    # Load answers
    answers = load_answers_jsonl(answers_file)

    # Grade
    grader = Grader(questions)
    grades, aggregate = grader.grade_all(answers)

    # Build output
    result = {
        'answers_file': str(answers_file),
        'graded_at': datetime.utcnow().isoformat() + 'Z',
        'aggregate': {
            'total_questions': aggregate.total_questions,
            'mean': aggregate.mean,
            'std': aggregate.std,
            'cv_pct': aggregate.cv_pct,
            'median': aggregate.median,
            'high_score_rate': aggregate.high_score_rate,
            'evidence_rate': aggregate.evidence_rate,
            'by_difficulty': aggregate.by_difficulty,
            'by_category': aggregate.by_category
        },
        'per_question': [g.to_dict() for g in grades]
    }

    # Save
    with open(output_file, 'w') as f:
        json.dump(result, f, indent=2)

    print(f"Graded: {answers_file.name}")
    print(f"  Mean: {aggregate.mean:.3f}")
    print(f"  Std: {aggregate.std:.3f}")
    print(f"  Evidence rate: {aggregate.evidence_rate*100:.1f}%")
    print(f"  High score rate: {aggregate.high_score_rate*100:.1f}%")
    print(f"  Output: {output_file}")

    return result


def analyze_benchmark(args) -> Dict:
    """Analyze all runs in a benchmark session."""
    output_dir = Path(args.output_dir)

    # Load config
    config_file = output_dir / 'config.json'
    if not config_file.exists():
        print(f"ERROR: Config not found: {config_file}")
        sys.exit(1)

    with open(config_file) as f:
        config = json.load(f)

    # Collect grades
    baseline_grades = []
    mdemg_grades = []

    for run_dir, grades_list in [('baseline', baseline_grades), ('mdemg', mdemg_grades)]:
        run_path = output_dir / run_dir
        if run_path.exists():
            for grades_file in sorted(run_path.glob('run_*_grades.json')):
                with open(grades_file) as f:
                    data = json.load(f)
                grades_list.append(data.get('per_question', []))
                print(f"Loaded: {grades_file.name}")

    if not baseline_grades and not mdemg_grades:
        print("ERROR: No grade files found")
        sys.exit(1)

    # Run comparison if both modes have data
    comparison = None
    if baseline_grades and mdemg_grades:
        analyzer = StatsAnalyzer()
        comparison_result = analyzer.compare(baseline_grades, mdemg_grades)
        comparison = comparison_result.to_dict()

        # Save comparison
        with open(output_dir / 'comparison.json', 'w') as f:
            json.dump(comparison, f, indent=2)

        # Generate report
        generator = ReportGenerator()
        report_config = BenchmarkConfig(
            endpoint=config.get('mdemg_endpoint', ''),
            space_id=config.get('space_id', ''),
            questions_file=config.get('questions_master', ''),
            question_count=config.get('question_count', 0),
            runs_per_mode=config.get('runs_per_mode', 3),
            created_at=config.get('created_at', '')
        )
        report_md = generator.generate(comparison, report_config)

        with open(output_dir / 'report.md', 'w') as f:
            f.write(report_md)

        print(f"\n{generator.generate_quick_summary(comparison)}")
        print(f"\nReport: {output_dir / 'report.md'}")

    # Build summary
    summary = {
        '$schema': 'benchmark_summary_v2',
        'metadata': config,
        'baseline_runs': len(baseline_grades),
        'mdemg_runs': len(mdemg_grades),
        'comparison': comparison
    }

    with open(output_dir / 'summary.json', 'w') as f:
        json.dump(summary, f, indent=2)

    print(f"Summary: {output_dir / 'summary.json'}")

    return summary


def main():
    parser = argparse.ArgumentParser(description='MDEMG Benchmark Helper v2.0')
    subparsers = parser.add_subparsers(dest='command', required=True)

    # Prepare command
    prep = subparsers.add_parser('prepare', help='Prepare benchmark session')
    prep.add_argument('--questions', '-q', required=True, help='Master questions file (with answers)')
    prep.add_argument('--questions-agent', '-a', help='Agent questions file (without answers)')
    prep.add_argument('--space-id', '-s', required=True, help='MDEMG space ID')
    prep.add_argument('--repo-path', '-r', required=True, help='Target codebase path')
    prep.add_argument('--endpoint', '-e', default='http://localhost:9999', help='MDEMG endpoint')
    prep.add_argument('--runs', type=int, default=3, help='Runs per mode')
    prep.add_argument('--output-dir', '-o', required=True, help='Output directory')

    # Grade command
    grade = subparsers.add_parser('grade', help='Grade a benchmark run')
    grade.add_argument('--answers', required=True, help='Answers JSONL file')
    grade.add_argument('--questions', '-q', required=True, help='Master questions file (with answers)')
    grade.add_argument('--output', '-o', required=True, help='Output grades JSON file')

    # Analyze command
    analyze = subparsers.add_parser('analyze', help='Analyze all runs')
    analyze.add_argument('--output-dir', '-o', required=True, help='Benchmark output directory')

    args = parser.parse_args()

    if args.command == 'prepare':
        prepare_benchmark(args)
    elif args.command == 'grade':
        grade_run(args)
    elif args.command == 'analyze':
        analyze_benchmark(args)


if __name__ == '__main__':
    main()
