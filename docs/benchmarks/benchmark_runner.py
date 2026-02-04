#!/usr/bin/env python3
"""
MDEMG Benchmark Runner v1.0

Orchestrates baseline vs MDEMG benchmark runs with proper isolation and grading.
Tracks key metrics: token efficiency, consistency, and learning edge accumulation.

Key Metrics (beyond mean score):
1. Token Efficiency - Tokens saved vs exhaustive grep/read baseline
2. Consistency - CV across runs (lower = more reproducible)
3. Learning Persistence - Edge accumulation over time
4. Evidence Quality - file:line citation rates

Usage:
    # Full comparison benchmark (3 baseline + 3 MDEMG runs)
    python benchmark_runner.py \\
        --questions test_questions.json \\
        --space-id pytorch-benchmark-v4 \\
        --endpoint http://localhost:9999 \\
        --runs 3 \\
        --mode compare

    # Baseline only
    python benchmark_runner.py --mode baseline --runs 3

    # MDEMG only
    python benchmark_runner.py --mode mdemg --runs 3
"""

import argparse
import json
import random
import sys
import time
import urllib.request
import urllib.error
from dataclasses import dataclass, field, asdict
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Any

# Import local modules
from grader_v4 import Grader, load_answers_jsonl
from stats_analyzer import StatsAnalyzer
from report_generator import ReportGenerator, BenchmarkConfig


# =============================================================================
# Configuration
# =============================================================================

@dataclass
class BaselineParams:
    """Baseline retrieval parameters (vector only, no graph traversal)."""
    candidate_k: int = 50
    top_k: int = 10
    hop_depth: int = 0  # No graph traversal = vector similarity only
    include_evidence: bool = True
    code_only: bool = False


@dataclass
class MdemgParams:
    """MDEMG retrieval parameters (full graph traversal enabled)."""
    candidate_k: int = 50
    top_k: int = 10
    hop_depth: int = 2  # Enable graph traversal (learning edges, hidden layer)
    include_evidence: bool = True
    code_only: bool = False


@dataclass
class RunConfig:
    """Configuration for a benchmark run."""
    version: str = "1.0"
    created_at: str = ""
    endpoint: str = "http://localhost:9999"
    space_id: str = ""
    questions_master_file: str = ""  # Master file with expected answers (for grading)
    questions_agent_file: str = ""   # Agent file without answers (for retrieval)
    question_count: int = 0
    runs_per_mode: int = 3
    random_seed: int = 42
    warmup_questions: int = 5
    baseline_params: BaselineParams = field(default_factory=BaselineParams)
    mdemg_params: MdemgParams = field(default_factory=MdemgParams)

    def to_dict(self) -> Dict:
        return {
            'version': self.version,
            'created_at': self.created_at,
            'endpoint': self.endpoint,
            'space_id': self.space_id,
            'questions_master_file': self.questions_master_file,
            'questions_agent_file': self.questions_agent_file,
            'question_count': self.question_count,
            'runs_per_mode': self.runs_per_mode,
            'random_seed': self.random_seed,
            'warmup_questions': self.warmup_questions,
            'baseline_params': asdict(self.baseline_params),
            'mdemg_params': asdict(self.mdemg_params)
        }


# =============================================================================
# Token Estimation
# =============================================================================

def estimate_tokens(text: str) -> int:
    """Estimate token count using ~4 chars per token heuristic."""
    if not text:
        return 0
    return len(text) // 4


def calculate_baseline_token_estimate(n_queries: int, avg_files: int = 5,
                                       avg_file_lines: int = 500,
                                       avg_line_chars: int = 40) -> int:
    """Estimate tokens for baseline (exhaustive grep/read).

    Baseline typically reads 5+ full files per query.
    """
    tokens_per_file = (avg_file_lines * avg_line_chars) // 4
    return n_queries * avg_files * tokens_per_file


def calculate_mdemg_token_usage(results: List[Dict]) -> Dict[str, int]:
    """Calculate actual tokens returned by MDEMG retrieval."""
    total = 0
    max_per_query = 0

    for r in results:
        query_tokens = 0
        nodes = r.get('nodes', [])
        for node in nodes:
            query_tokens += estimate_tokens(node.get('name', ''))
            query_tokens += estimate_tokens(node.get('path', ''))
            query_tokens += estimate_tokens(node.get('summary', ''))
        total += query_tokens
        max_per_query = max(max_per_query, query_tokens)

    return {
        'total': total,
        'avg_per_query': total // len(results) if results else 0,
        'max_per_query': max_per_query
    }


# =============================================================================
# API Client
# =============================================================================

class MdemgClient:
    """Client for MDEMG API."""

    def __init__(self, endpoint: str, space_id: str, timeout: int = 30):
        self.endpoint = endpoint.rstrip('/')
        self.space_id = space_id
        self.timeout = timeout

    def _request(self, method: str, path: str, data: Optional[Dict] = None) -> Dict:
        """Make HTTP request to MDEMG API."""
        url = f"{self.endpoint}{path}"

        if data:
            req = urllib.request.Request(
                url,
                data=json.dumps(data).encode('utf-8'),
                headers={'Content-Type': 'application/json'},
                method=method
            )
        else:
            req = urllib.request.Request(url, method=method)

        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return json.loads(resp.read().decode('utf-8'))
        except urllib.error.HTTPError as e:
            body = e.read().decode('utf-8') if e.fp else ''
            return {'error': f"HTTP {e.code}: {body[:200]}"}
        except Exception as e:
            return {'error': str(e)}

    def health_check(self) -> bool:
        """Check if MDEMG server is healthy."""
        try:
            req = urllib.request.urlopen(f"{self.endpoint}/readyz", timeout=5)
            return req.status == 200
        except:
            return False

    def get_stats(self) -> Dict:
        """Get memory stats for space."""
        return self._request('GET', f'/v1/memory/stats?space_id={self.space_id}')

    def get_learning_edge_count(self) -> int:
        """Get current learning edge count."""
        stats = self.get_stats()
        return stats.get('learning_activity', {}).get('co_activated_edges', 0)

    def retrieve(self, query: str, params: BaselineParams | MdemgParams) -> Dict:
        """Execute retrieval query."""
        data = {
            'space_id': self.space_id,
            'query_text': query,
            'candidate_k': params.candidate_k,
            'top_k': params.top_k,
            'hop_depth': params.hop_depth,
            'include_evidence': getattr(params, 'include_evidence', True),
            'code_only': getattr(params, 'code_only', False)
        }
        return self._request('POST', '/v1/memory/retrieve', data)


# =============================================================================
# Benchmark Runner
# =============================================================================

class BenchmarkRunner:
    """Orchestrates benchmark runs."""

    def __init__(self, config: RunConfig):
        self.config = config
        self.client = MdemgClient(config.endpoint, config.space_id)
        self.grader = None  # Initialized when questions loaded
        self.questions = []

    def load_questions(self) -> List[Dict]:
        """Load questions from JSON files.

        Loads both master (with answers) and agent (without answers) versions.
        The master is used for grading, the agent is used during retrieval.
        """
        # Load master questions (with expected answers)
        with open(self.config.questions_master_file) as f:
            master_data = json.load(f)
        master_questions = master_data.get('questions', master_data) if isinstance(master_data, dict) else master_data

        # Load agent questions (without answers for retrieval)
        if self.config.questions_agent_file:
            with open(self.config.questions_agent_file) as f:
                agent_data = json.load(f)
            agent_questions = agent_data.get('questions', agent_data) if isinstance(agent_data, dict) else agent_data
        else:
            # Use master questions but strip answer fields
            agent_questions = []
            for q in master_questions:
                agent_q = {k: v for k, v in q.items()
                           if k not in ('expected_answer', 'golden_answer', 'answer',
                                       'requires_files', 'required_files', 'evidence',
                                       'file_line_refs')}
                agent_questions.append(agent_q)

        # Create lookup from ID -> master question (for grading)
        self.master_questions = {str(q['id']): q for q in master_questions}

        # Store agent questions (for retrieval - no answers)
        self.questions = agent_questions
        self.config.question_count = len(agent_questions)

        # Grader uses master questions (has expected answers)
        self.grader = Grader(master_questions)

        return agent_questions

    def shuffle_questions(self, seed: int) -> List[Dict]:
        """Shuffle questions with given seed for reproducibility."""
        shuffled = self.questions.copy()
        random.seed(seed)
        random.shuffle(shuffled)
        return shuffled

    def _format_answer_from_results(self, results: List[Dict]) -> tuple[str, List[str], List[str]]:
        """Format answer text and file refs from retrieval results."""
        if not results:
            return "No results from retrieval.", [], []

        top_results = results[:5]
        answer_parts = []
        file_refs = []
        files_consulted = []

        for r in top_results:
            name = r.get('name', '')
            path = r.get('path', '')
            summary = r.get('summary', '')[:300]

            # Extract line number from evidence array (symbol-level precision)
            evidence = r.get('evidence', [])
            if evidence and isinstance(evidence, list) and len(evidence) > 0:
                # Use first evidence item's line number
                line = evidence[0].get('line', 1)
                # Also get the actual file path from evidence if available
                ev_path = evidence[0].get('file_path', '')
                if ev_path:
                    path = ev_path  # Use evidence path (cleaner, no #symbol suffix)
            else:
                line = 1

            if path:
                # Clean path - remove #SymbolName suffix if present
                clean_path = path.split('#')[0] if '#' in path else path
                files_consulted.append(clean_path)
                file_refs.append(f"{clean_path}:{line}")
                if summary:
                    answer_parts.append(f"{name} ({clean_path}:{line}): {summary}")
                else:
                    answer_parts.append(f"{name} at {clean_path}:{line}")

        answer = " ".join(answer_parts) if answer_parts else "Could not find relevant information."
        return answer, file_refs, files_consulted

    def run_single(self, run_type: str, run_num: int, questions: List[Dict],
                   output_file: Path, params: BaselineParams | MdemgParams) -> Dict:
        """Run a single benchmark (baseline or mdemg).

        Returns:
            Run metadata dict with timing, edges, and token metrics
        """
        print(f"\n{'='*60}")
        print(f"Starting {run_type.upper()} Run {run_num}")
        print(f"Output: {output_file}")
        print(f"{'='*60}")

        edges_before = self.client.get_learning_edge_count() if run_type == "mdemg" else 0
        start_time = time.time()

        results = []
        retrieval_data = []  # For token calculation
        total_latency_ms = 0

        for i, q in enumerate(questions):
            qid = q['id']
            question = q['question']

            query_start = time.time()
            response = self.client.retrieve(question, params)
            latency_ms = (time.time() - query_start) * 1000
            total_latency_ms += latency_ms

            if 'error' in response:
                print(f"  Q{i+1}: ERROR - {response['error'][:50]}")
                results.append({
                    'id': qid,
                    'question': question,
                    'answer': 'ERROR: ' + response.get('error', 'Unknown error'),
                    'files_consulted': [],
                    'file_line_refs': [],
                    'latency_ms': latency_ms
                })
                retrieval_data.append({'nodes': []})
                continue

            nodes = response.get('results', [])
            answer, file_refs, files_consulted = self._format_answer_from_results(nodes)

            results.append({
                'id': qid,
                'question': question,
                'answer': answer,
                'files_consulted': files_consulted[:5],
                'file_line_refs': file_refs[:5],
                'latency_ms': round(latency_ms, 1),
                'activated_nodes': sum(1 for n in nodes if n.get('activation', 0) >= 0.2),
                'l1_count': sum(1 for n in nodes if n.get('layer', 0) == 1),
                'rerank_applied': response.get('debug', {}).get('rerank_applied', False)
            })

            retrieval_data.append({'nodes': nodes})

            if (i + 1) % 20 == 0:
                elapsed = time.time() - start_time
                rate = (i + 1) / elapsed if elapsed > 0 else 0
                print(f"  Progress: {i+1}/{len(questions)} ({rate:.2f} q/s)")

        # Write results as JSONL
        with open(output_file, 'w') as f:
            for r in results:
                f.write(json.dumps(r) + '\n')

        edges_after = self.client.get_learning_edge_count() if run_type == "mdemg" else 0
        duration = time.time() - start_time

        # Token metrics
        token_usage = calculate_mdemg_token_usage(retrieval_data)
        baseline_estimate = calculate_baseline_token_estimate(len(questions))

        return {
            'run_id': f"{run_type}_run{run_num}",
            'type': run_type,
            'sequence': run_num,
            'timestamp': datetime.utcnow().isoformat() + 'Z',
            'questions_total': len(questions),
            'timing': {
                'duration_seconds': round(duration, 1),
                'avg_latency_ms': round(total_latency_ms / len(questions), 1) if questions else 0
            },
            'output': {
                'file_path': str(output_file)
            },
            'learning_edges': {
                'before': edges_before,
                'after': edges_after,
                'delta': edges_after - edges_before
            } if run_type == 'mdemg' else {},
            'token_metrics': {
                'mdemg_tokens': token_usage['total'],
                'mdemg_tokens_avg': token_usage['avg_per_query'],
                'baseline_estimate': baseline_estimate,
                'token_savings_pct': round((1 - token_usage['total'] / baseline_estimate) * 100, 1) if baseline_estimate > 0 else 0,
                'efficiency_ratio': round(baseline_estimate / token_usage['total'], 1) if token_usage['total'] > 0 else 0
            }
        }

    def grade_run(self, answers_file: Path, grades_file: Path) -> Dict:
        """Grade a run using grader_v4."""
        answers = load_answers_jsonl(answers_file)
        grades, aggregate = self.grader.grade_all(answers)

        # Save grades
        output = {
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

        with open(grades_file, 'w') as f:
            json.dump(output, f, indent=2)

        return {
            'graded': True,
            'grades_file': str(grades_file),
            'mean': aggregate.mean,
            'std': aggregate.std,
            'cv_pct': aggregate.cv_pct,
            'high_score_rate': aggregate.high_score_rate,
            'evidence_rate': aggregate.evidence_rate
        }

    def run_benchmark(self, output_dir: Path, mode: str = 'compare') -> Dict:
        """Run full benchmark.

        Args:
            output_dir: Directory for output files
            mode: 'baseline', 'mdemg', or 'compare' (both)

        Returns:
            Complete benchmark results
        """
        output_dir.mkdir(parents=True, exist_ok=True)

        # Load questions (master + agent versions)
        questions = self.load_questions()
        print(f"Loaded {len(questions)} questions")
        print(f"  Master (for grading): {self.config.questions_master_file}")
        print(f"  Agent (for retrieval): {self.config.questions_agent_file or '(stripped from master)'}")

        # Save config
        self.config.created_at = datetime.utcnow().isoformat() + 'Z'
        with open(output_dir / 'config.json', 'w') as f:
            json.dump(self.config.to_dict(), f, indent=2)

        # Copy questions for reference
        with open(output_dir / 'questions.json', 'w') as f:
            json.dump({'questions': questions}, f, indent=2)

        all_runs = []
        baseline_grades = []
        mdemg_grades = []

        # Baseline runs
        if mode in ('baseline', 'compare'):
            baseline_dir = output_dir / 'baseline'
            baseline_dir.mkdir(exist_ok=True)

            print(f"\n{'='*60}")
            print(f"PHASE 1: BASELINE RUNS ({self.config.runs_per_mode}x)")
            print(f"{'='*60}")

            for run_num in range(1, self.config.runs_per_mode + 1):
                # Shuffle with different seed per run for variance measurement
                shuffled = self.shuffle_questions(self.config.random_seed + run_num)

                answers_file = baseline_dir / f'run_{run_num}_answers.jsonl'
                grades_file = baseline_dir / f'run_{run_num}_grades.json'

                run_data = self.run_single('baseline', run_num, shuffled, answers_file,
                                           self.config.baseline_params)
                run_data['grading'] = self.grade_run(answers_file, grades_file)

                all_runs.append(run_data)
                with open(grades_file) as f:
                    baseline_grades.append(json.load(f).get('per_question', []))

                print(f"  Baseline Run {run_num}: mean={run_data['grading']['mean']:.3f}, "
                      f"evidence={run_data['grading']['evidence_rate']*100:.1f}%")

        # MDEMG runs
        if mode in ('mdemg', 'compare'):
            mdemg_dir = output_dir / 'mdemg'
            mdemg_dir.mkdir(exist_ok=True)

            print(f"\n{'='*60}")
            print(f"PHASE 2: MDEMG RUNS ({self.config.runs_per_mode}x)")
            print(f"{'='*60}")

            for run_num in range(1, self.config.runs_per_mode + 1):
                shuffled = self.shuffle_questions(self.config.random_seed + run_num)

                answers_file = mdemg_dir / f'run_{run_num}_answers.jsonl'
                grades_file = mdemg_dir / f'run_{run_num}_grades.json'

                run_data = self.run_single('mdemg', run_num, shuffled, answers_file,
                                           self.config.mdemg_params)
                run_data['grading'] = self.grade_run(answers_file, grades_file)

                all_runs.append(run_data)
                with open(grades_file) as f:
                    mdemg_grades.append(json.load(f).get('per_question', []))

                edges = run_data.get('learning_edges', {})
                tokens = run_data.get('token_metrics', {})
                print(f"  MDEMG Run {run_num}: mean={run_data['grading']['mean']:.3f}, "
                      f"evidence={run_data['grading']['evidence_rate']*100:.1f}%, "
                      f"edges={edges.get('before', 0)}->{edges.get('after', 0)}, "
                      f"token_savings={tokens.get('token_savings_pct', 0)}%")

        # Statistical comparison
        comparison = None
        if mode == 'compare' and baseline_grades and mdemg_grades:
            print(f"\n{'='*60}")
            print("PHASE 3: STATISTICAL COMPARISON")
            print(f"{'='*60}")

            analyzer = StatsAnalyzer()
            comparison_result = analyzer.compare(baseline_grades, mdemg_grades)
            comparison = comparison_result.to_dict()

            with open(output_dir / 'comparison.json', 'w') as f:
                json.dump(comparison, f, indent=2)

            # Generate report
            generator = ReportGenerator()
            report_config = BenchmarkConfig(
                endpoint=self.config.endpoint,
                space_id=self.config.space_id,
                questions_file=self.config.questions_master_file,
                question_count=self.config.question_count,
                runs_per_mode=self.config.runs_per_mode,
                created_at=self.config.created_at
            )
            report_md = generator.generate(comparison, report_config)

            with open(output_dir / 'report.md', 'w') as f:
                f.write(report_md)

            print(generator.generate_quick_summary(comparison))

        # Final summary
        summary = {
            '$schema': 'benchmark_summary_v1',
            'metadata': {
                'benchmark_id': f"{self.config.space_id}-{datetime.now().strftime('%Y%m%d')}",
                'created_at': self.config.created_at,
                'framework_version': '1.0',
                'mode': mode
            },
            'config': self.config.to_dict(),
            'runs': all_runs,
            'comparison': comparison
        }

        with open(output_dir / 'summary.json', 'w') as f:
            json.dump(summary, f, indent=2)

        print(f"\n{'='*60}")
        print("BENCHMARK COMPLETE")
        print(f"{'='*60}")
        print(f"Output: {output_dir}")
        print(f"Report: {output_dir / 'report.md'}")

        return summary


# =============================================================================
# CLI
# =============================================================================

def main():
    parser = argparse.ArgumentParser(
        description='MDEMG Benchmark Runner - Compare baseline vs MDEMG retrieval',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Full comparison benchmark
    python benchmark_runner.py --questions test_questions.json --space-id my-space --runs 3 --mode compare

    # Baseline only
    python benchmark_runner.py --questions test_questions.json --space-id my-space --mode baseline

    # MDEMG only with custom endpoint
    python benchmark_runner.py --questions test_questions.json --space-id my-space --endpoint http://localhost:8090 --mode mdemg
"""
    )

    parser.add_argument('--questions', '-q', required=True,
                        help='Path to master questions JSON file (with expected answers)')
    parser.add_argument('--questions-agent', '-a',
                        help='Path to agent questions JSON file (without answers). '
                             'If not provided, will look for *_agent.json variant of master file.')
    parser.add_argument('--space-id', '-s', required=True,
                        help='MDEMG space ID')
    parser.add_argument('--endpoint', '-e', default='http://localhost:9999',
                        help='MDEMG server endpoint (default: http://localhost:9999)')
    parser.add_argument('--runs', '-r', type=int, default=3,
                        help='Number of runs per mode (default: 3)')
    parser.add_argument('--mode', '-m', choices=['baseline', 'mdemg', 'compare'],
                        default='compare',
                        help='Benchmark mode (default: compare)')
    parser.add_argument('--output-dir', '-o',
                        help='Output directory (default: benchmark_run_YYYYMMDD_HHMMSS)')
    parser.add_argument('--seed', type=int, default=42,
                        help='Random seed for reproducibility (default: 42)')

    args = parser.parse_args()

    # Validate master questions file
    master_path = Path(args.questions)
    if not master_path.exists():
        print(f"ERROR: Master questions file not found: {master_path}")
        sys.exit(1)

    # Find or validate agent questions file
    if args.questions_agent:
        agent_path = Path(args.questions_agent)
        if not agent_path.exists():
            print(f"ERROR: Agent questions file not found: {agent_path}")
            sys.exit(1)
    else:
        # Try to find *_agent.json variant
        agent_path = master_path.parent / master_path.name.replace('_master.json', '_agent.json')
        if not agent_path.exists():
            agent_path = master_path.parent / (master_path.stem + '_agent.json')
        if not agent_path.exists():
            # No agent file - will strip answers from master
            agent_path = None
            print(f"Note: No agent file found. Will strip answers from master file.")

    # Setup output directory
    if args.output_dir:
        output_dir = Path(args.output_dir)
    else:
        output_dir = Path(f"benchmark_run_{datetime.now().strftime('%Y%m%d_%H%M%S')}")

    # Create config
    config = RunConfig(
        endpoint=args.endpoint,
        space_id=args.space_id,
        questions_master_file=str(master_path),
        questions_agent_file=str(agent_path) if agent_path else "",
        runs_per_mode=args.runs,
        random_seed=args.seed
    )

    # Create runner
    runner = BenchmarkRunner(config)

    # Check server health
    print(f"Checking MDEMG server at {args.endpoint}...")
    if not runner.client.health_check():
        print(f"ERROR: MDEMG server not available at {args.endpoint}")
        sys.exit(1)
    print("Server: OK")

    # Get initial stats
    stats = runner.client.get_stats()
    print(f"Space: {args.space_id}")
    print(f"Memory count: {stats.get('memory_count', 0)}")
    print(f"Learning edges: {stats.get('learning_activity', {}).get('co_activated_edges', 0)}")

    # Run benchmark
    runner.run_benchmark(output_dir, args.mode)


if __name__ == '__main__':
    main()
