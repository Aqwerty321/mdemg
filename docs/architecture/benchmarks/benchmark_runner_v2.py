#!/usr/bin/env python3
"""
MDEMG Benchmark Runner v2.0

Spawns Claude Task agents to answer benchmark questions about a target codebase.
Compares baseline agents (no MDEMG) vs MDEMG agents (with MDEMG API access).

Key Architecture:
- Baseline agents: Search codebase using Read/Glob/Grep (NO MDEMG access)
- MDEMG agents: Same tools PLUS MDEMG API access via curl

Key Metrics:
1. Token Efficiency - Compare token usage between modes
2. Consistency - CV across runs (lower = more reproducible)
3. Learning Persistence - Edge accumulation over MDEMG runs
4. Evidence Quality - file:line citation rates

Usage:
    # Full comparison benchmark (3 baseline + 3 MDEMG runs)
    python benchmark_runner_v2.py \\
        --questions questions_master.json \\
        --repo-path /path/to/target/codebase \\
        --space-id my-space \\
        --runs 3 \\
        --mode compare

    # Baseline only
    python benchmark_runner_v2.py --mode baseline --runs 3 ...

    # MDEMG only
    python benchmark_runner_v2.py --mode mdemg --runs 3 ...
"""

import argparse
import json
import os
import subprocess
import sys
import time
import urllib.request
import urllib.error
from dataclasses import dataclass, asdict
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Any

# Import local modules for grading
from grader_v4 import Grader, load_answers_jsonl
from stats_analyzer import StatsAnalyzer
from report_generator import ReportGenerator, BenchmarkConfig


# =============================================================================
# Configuration
# =============================================================================

@dataclass
class BenchmarkRunConfig:
    """Configuration for a benchmark session."""
    version: str = "2.0"
    created_at: str = ""

    # Target codebase
    repo_path: str = ""
    repo_name: str = ""

    # MDEMG settings (for MDEMG agents only)
    mdemg_endpoint: str = "http://localhost:9999"
    space_id: str = ""

    # Questions
    questions_master_file: str = ""
    questions_agent_file: str = ""
    question_count: int = 0

    # Run settings
    runs_per_mode: int = 3
    model: str = "haiku"  # haiku for cost efficiency

    def to_dict(self) -> Dict:
        return asdict(self)


# =============================================================================
# Prompt Templates
# =============================================================================

BASELINE_PROMPT_TEMPLATE = """You are answering benchmark questions about the {codebase_name} codebase.

## STRICT RULES - VIOLATION = DISQUALIFICATION
1. You may ONLY access files within: {repo_path}
2. You may NOT use WebSearch or WebFetch
3. You MUST answer ALL {question_count} questions
4. You MUST include file:line references in every answer
5. **CRITICAL: Use the EXACT "id" field from each question object - DO NOT use array indices (0,1,2...)**

## AVAILABLE TOOLS
- Read: Read source files
- Glob: Find files by pattern
- Grep: Search file contents
- Bash: For file operations AND writing answers to output file

## WORKFLOW (repeat for each question)
1. Read ONE question from the questions array - note its "id" field value (e.g., 379, 77, 258)
2. Search for relevant files (Glob/Grep within {repo_path})
3. Read source code to find the answer
4. Write answer to output file using Bash with printf/echo - use the question's original "id" field value
5. Move to next question

## HOW TO WRITE ANSWERS
**CRITICAL: Each answer MUST be a single-line JSON object (no newlines or pretty-printing!)**

Each question has an "id" field (e.g., {{"id": 379, "question": "..."}}). Use THAT id in your answer:
```bash
# If question has "id": 379, your answer must also have "id": 379
# IMPORTANT: The entire JSON object must be on ONE LINE - no line breaks inside the JSON!
printf '%s\\n' '{{"id": 379, "question": "...", "answer": "Your answer here all on one line", "files_consulted": ["file1.py"], "file_line_refs": ["file.py:123"], "confidence": "HIGH"}}' >> {output_file}
```

## OUTPUT FORMAT (JSONL - each line is a complete JSON object)
File: {output_file}
**Format: ONE JSON object per line - no newlines inside the JSON, no pretty-printing!**
{{"id": <question's original id>, "question": "...", "answer": "...", "files_consulted": ["file1.py", "file2.py"], "file_line_refs": ["file.py:123"], "confidence": "HIGH|MEDIUM|LOW"}}

## EVIDENCE REQUIREMENTS
- ALWAYS cite specific file:line references
- Example: "The value is 100, defined in config.py:42"
- If you cannot find the answer, state that clearly with confidence: "LOW"

## BEGIN - FIRST READ THE QUESTIONS FILE
1. Use the Read tool to read: {questions_file}
2. Parse the JSON to extract the "questions" array
3. Answer ONLY questions from that file - do not make up your own questions
4. For each question object like {{"id": 379, "question": "..."}}, use the id field (379) in your answer

Answer ALL {question_count} questions from the file sequentially, writing each answer immediately using Bash to append to the output file.
"""

MDEMG_PROMPT_TEMPLATE = """You are answering benchmark questions about the {codebase_name} codebase.

**THIS IS AN MDEMG BENCHMARK TEST.** You MUST use the MDEMG retrieval API for EVERY question.

## YOUR TASK
Answer {question_count} questions from: {questions_file}
Write answers to: {output_file}

## CONSTRAINTS
- Only access files within: {repo_path}
- No WebSearch or WebFetch
- **Glob and Grep tools are DISABLED** - you cannot search the filesystem directly
- **You MUST use the MDEMG API** (via curl) to discover which files to read
- Available tools: Read (for files), Bash (for curl commands and writing output)

---

## STEP 1: READ THE QUESTIONS FILE

**Example - How to read questions:**
```
Tool: Read
Path: {questions_file}
```

**Example - What you'll see:**
```json
{{"questions": [
  {{"id": 379, "question": "What validation does BarrelService perform?", "category": "business_logic"}},
  {{"id": 77, "question": "How is user authentication handled?", "category": "architecture"}}
]}}
```

**CRITICAL:** Note the "id" field (379, 77, etc.) - you must use these exact IDs in your answers.

---

## STEP 2: FOR EACH QUESTION, CALL MDEMG API

**Example - MDEMG API call for question id=379:**
```bash
curl -s -X POST "{mdemg_endpoint}/v1/memory/retrieve" -H "Content-Type: application/json" -d '{{"space_id": "{space_id}", "query_text": "What validation does BarrelService perform?", "top_k": 5}}'
```

**Example - MDEMG response:**
```json
{{"results": [
  {{"node_id": "abc123", "path": "{repo_path}/src/barrel/barrel.service.ts", "score": 0.92, "content_preview": "validateBarrel method..."}},
  {{"node_id": "def456", "path": "{repo_path}/src/barrel/dto/create-barrel.dto.ts", "score": 0.87, "content_preview": "class CreateBarrelDto..."}}
]}}
```

**What to extract:** The "path" values - these are the files you must read next.

---

## STEP 3: READ FILES FROM MDEMG RESULTS

**Example - Read file from MDEMG result:**
```
Tool: Read
Path: {repo_path}/src/barrel/barrel.service.ts
```

**Example - What you find in the code:**
```typescript
// Line 45-52
async validateBarrel(dto: CreateBarrelDto): Promise<void> {{
  if (!dto.capacity || dto.capacity <= 0) {{
    throw new BadRequestException('Capacity must be positive');
  }}
  if (!dto.location) {{
    throw new BadRequestException('Location is required');
  }}
}}
```

---

## STEP 4: WRITE YOUR ANSWER

**Example - Correct answer format (SINGLE LINE, no line breaks inside JSON):**
```bash
printf '%s\\n' '{{"id": 379, "question": "What validation does BarrelService perform?", "answer": "BarrelService validates capacity (must be positive) and location (required) in barrel.service.ts:45-52. Throws BadRequestException for invalid input.", "files_consulted": ["src/barrel/barrel.service.ts"], "file_line_refs": ["barrel.service.ts:45", "barrel.service.ts:48", "barrel.service.ts:51"], "mdemg_used": true, "confidence": "HIGH"}}' >> {output_file}
```

**WRONG - Multi-line JSON (breaks JSONL format):**
```bash
printf '%s\\n' '{{
  "id": 379,
  "answer": "..."
}}' >> {output_file}
```

**WRONG - Using array index instead of question id:**
```bash
printf '%s\\n' '{{"id": 0, ...}}' >> {output_file}  # WRONG! Use 379, not 0
```

**WRONG - Copying MDEMG summary as answer:**
```bash
printf '%s\\n' '{{"id": 379, "answer": "validateBarrel method...", ...}}' >> {output_file}  # WRONG! Read the actual code
```

---

## COMPLETE WORKED EXAMPLE

**Question from file:** {{"id": 77, "question": "How is user authentication handled?"}}

**Step 2a - Call MDEMG:**
```bash
curl -s -X POST "{mdemg_endpoint}/v1/memory/retrieve" -H "Content-Type: application/json" -d '{{"space_id": "{space_id}", "query_text": "How is user authentication handled?", "top_k": 5}}'
```

**Step 2b - MDEMG returns:**
```json
{{"results": [{{"path": "{repo_path}/src/auth/auth.service.ts", "score": 0.95}}]}}
```

**Step 3 - Read the file:**
```
Tool: Read
Path: {repo_path}/src/auth/auth.service.ts
```

**Step 3b - Find relevant code (lines 23-30):**
```typescript
async validateUser(email: string, password: string): Promise<User | null> {{
  const user = await this.usersService.findByEmail(email);
  if (user && await bcrypt.compare(password, user.passwordHash)) {{
    return user;
  }}
  return null;
}}
```

**Step 4 - Write answer:**
```bash
printf '%s\\n' '{{"id": 77, "question": "How is user authentication handled?", "answer": "Authentication is handled in auth.service.ts:23-30. The validateUser method finds user by email, then uses bcrypt.compare to verify password against stored hash. Returns User on success, null on failure.", "files_consulted": ["src/auth/auth.service.ts"], "file_line_refs": ["auth.service.ts:23", "auth.service.ts:25"], "mdemg_used": true, "confidence": "HIGH"}}' >> {output_file}
```

---

## CHECKLIST FOR EVERY QUESTION

[ ] 1. Get question from file (note the "id" field)
[ ] 2. Call MDEMG API with curl (MANDATORY)
[ ] 3. Extract file paths from MDEMG response
[ ] 4. Read those files with Read tool
[ ] 5. Find specific lines that answer the question
[ ] 6. Write single-line JSON with correct id, file:line refs, mdemg_used=true
[ ] 7. Append to {output_file}

**Total questions to answer: {question_count}**

Begin by reading the questions file, then process each question following the steps above.
"""


# =============================================================================
# MDEMG Client (for stats and edge tracking)
# =============================================================================

class MdemgClient:
    """Client for MDEMG API (used for stats, not retrieval)."""

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


# =============================================================================
# Agent Execution
# =============================================================================

def run_agent(
    prompt: str,
    output_file: Path,
    allowed_tools: List[str],
    model: str = "haiku",
    run_in_background: bool = False,
    description: str = "Benchmark agent"
) -> Dict:
    """
    Spawn a Claude Task agent to answer benchmark questions.

    Returns dict with:
        - agent_id: Task ID
        - status: running | completed | error
        - output_file: Path to JSONL answers
        - token_count: Estimated tokens used (if available)
    """
    # Use subprocess to call claude CLI
    # --print mode expects input via stdin

    cmd = [
        "claude",
        "--model", model,
        "--print",  # Non-interactive mode, reads from stdin
        "--allowedTools", ",".join(allowed_tools),
    ]

    start_time = time.time()

    try:
        result = subprocess.run(
            cmd,
            input=prompt,  # Pass prompt via stdin for --print mode
            capture_output=True,
            text=True,
            timeout=3600  # 1 hour timeout
        )

        duration = time.time() - start_time

        return {
            'status': 'completed' if result.returncode == 0 else 'error',
            'output_file': str(output_file),
            'duration_seconds': round(duration, 1),
            'returncode': result.returncode,
            'stdout_preview': result.stdout[:500] if result.stdout else '',
            'stderr_preview': result.stderr[:500] if result.stderr else ''
        }

    except subprocess.TimeoutExpired:
        return {
            'status': 'timeout',
            'output_file': str(output_file),
            'duration_seconds': 3600
        }
    except Exception as e:
        return {
            'status': 'error',
            'error': str(e),
            'output_file': str(output_file)
        }


def validate_output(output_file: Path, expected_count: int) -> Dict:
    """Validate JSONL output from an agent run."""
    errors = []
    warnings = []
    ids = []

    if not output_file.exists():
        return {
            'valid': False,
            'errors': ['Output file does not exist'],
            'warnings': [],
            'answer_count': 0,
            'unique_ids': 0
        }

    with open(output_file) as f:
        lines = f.readlines()

    for i, line in enumerate(lines, 1):
        try:
            obj = json.loads(line.strip())
            ids.append(obj.get('id'))

            # Check required fields
            for field in ['id', 'question', 'answer']:
                if field not in obj:
                    errors.append(f"Line {i}: Missing field '{field}'")

            # Check evidence
            refs = obj.get('file_line_refs', [])
            if not refs:
                warnings.append(f"ID {obj.get('id')}: No file references")
            elif not any(':' in str(ref) for ref in refs):
                warnings.append(f"ID {obj.get('id')}: No line numbers in refs")

        except json.JSONDecodeError as e:
            errors.append(f"Line {i}: Invalid JSON - {e}")

    # Check for duplicates
    from collections import Counter
    counts = Counter(ids)
    duplicates = {k: v for k, v in counts.items() if v > 1}
    if duplicates:
        errors.append(f"Duplicate IDs: {duplicates}")

    # Check for missing IDs
    unique_ids = set(ids)
    expected_ids = set(range(1, expected_count + 1))
    missing = expected_ids - unique_ids
    if missing:
        errors.append(f"Missing IDs: {sorted(missing)[:10]}{'...' if len(missing) > 10 else ''}")

    return {
        'valid': len(errors) == 0,
        'errors': errors,
        'warnings': warnings[:10],
        'answer_count': len(lines),
        'unique_ids': len(unique_ids),
        'completion_rate': len(unique_ids) / expected_count if expected_count > 0 else 0
    }


# =============================================================================
# Benchmark Runner
# =============================================================================

class BenchmarkRunnerV2:
    """Orchestrates benchmark runs using Claude Task agents."""

    def __init__(self, config: BenchmarkRunConfig):
        self.config = config
        self.client = MdemgClient(config.mdemg_endpoint, config.space_id)
        self.grader = None
        self.questions = []
        self.master_questions = {}

    def load_questions(self) -> List[Dict]:
        """Load questions from JSON files."""
        # Load master questions (with expected answers - for grading)
        with open(self.config.questions_master_file) as f:
            master_data = json.load(f)
        master_questions = master_data.get('questions', master_data) if isinstance(master_data, dict) else master_data

        # Load agent questions (without answers - given to agents)
        if self.config.questions_agent_file and Path(self.config.questions_agent_file).exists():
            with open(self.config.questions_agent_file) as f:
                agent_data = json.load(f)
            agent_questions = agent_data.get('questions', agent_data) if isinstance(agent_data, dict) else agent_data
        else:
            # Strip answer fields from master
            agent_questions = []
            for q in master_questions:
                agent_q = {k: v for k, v in q.items()
                           if k not in ('expected_answer', 'golden_answer', 'answer',
                                       'requires_files', 'required_files', 'evidence',
                                       'file_line_refs')}
                agent_questions.append(agent_q)

        self.master_questions = {str(q['id']): q for q in master_questions}
        self.questions = agent_questions
        self.config.question_count = len(agent_questions)
        self.grader = Grader(master_questions)

        return agent_questions

    def prepare_agent_questions_file(self, output_dir: Path) -> Path:
        """Create temporary questions file for agent (no answers)."""
        agent_questions_file = output_dir / 'agent_questions.json'
        with open(agent_questions_file, 'w') as f:
            json.dump({'questions': self.questions}, f, indent=2)
        return agent_questions_file

    def run_baseline(self, run_num: int, output_dir: Path, questions_file: Path) -> Dict:
        """Run a single baseline agent."""
        print(f"\n--- Baseline Run {run_num} ---")

        answers_file = output_dir / f'run_{run_num}_answers.jsonl'

        # Ensure output file is empty
        if answers_file.exists():
            answers_file.unlink()
        answers_file.touch()

        prompt = BASELINE_PROMPT_TEMPLATE.format(
            codebase_name=self.config.repo_name,
            repo_path=self.config.repo_path,
            question_count=self.config.question_count,
            output_file=str(answers_file),
            questions_file=str(questions_file)
        )

        allowed_tools = ['Read', 'Glob', 'Grep', 'Bash']

        start_time = time.time()

        # Execute agent
        result = run_agent(
            prompt=prompt,
            output_file=answers_file,
            allowed_tools=allowed_tools,
            model=self.config.model,
            description=f"Baseline Run {run_num}"
        )

        duration = time.time() - start_time

        # Validate output
        validation = validate_output(answers_file, self.config.question_count)

        return {
            'run_id': f'baseline_run{run_num}',
            'type': 'baseline',
            'sequence': run_num,
            'status': 'valid' if validation['valid'] else 'partial' if validation['completion_rate'] > 0.9 else 'invalid',
            'timestamp': datetime.utcnow().isoformat() + 'Z',
            'completion': {
                'questions_answered': validation['answer_count'],
                'questions_expected': self.config.question_count,
                'completion_rate': validation['completion_rate']
            },
            'timing': {
                'duration_seconds': round(duration, 1)
            },
            'output': {
                'file_path': str(answers_file)
            },
            'validation': validation,
            'agent_result': result
        }

    def run_mdemg(self, run_num: int, output_dir: Path, questions_file: Path) -> Dict:
        """Run a single MDEMG agent."""
        print(f"\n--- MDEMG Run {run_num} ---")

        answers_file = output_dir / f'run_{run_num}_answers.jsonl'

        # Ensure output file is empty
        if answers_file.exists():
            answers_file.unlink()
        answers_file.touch()

        # Get learning edges before
        edges_before = self.client.get_learning_edge_count()

        prompt = MDEMG_PROMPT_TEMPLATE.format(
            codebase_name=self.config.repo_name,
            repo_path=self.config.repo_path,
            question_count=self.config.question_count,
            output_file=str(answers_file),
            questions_file=str(questions_file),
            mdemg_endpoint=self.config.mdemg_endpoint,
            space_id=self.config.space_id
        )

        # MDEMG benchmark: Only Read and Bash (curl) allowed
        # Glob/Grep DISABLED to force MDEMG API usage for file discovery
        allowed_tools = ['Read', 'Bash']

        start_time = time.time()

        # Execute agent
        result = run_agent(
            prompt=prompt,
            output_file=answers_file,
            allowed_tools=allowed_tools,
            model=self.config.model,
            description=f"MDEMG Run {run_num}"
        )

        duration = time.time() - start_time

        # Get learning edges after
        edges_after = self.client.get_learning_edge_count()

        # Validate output
        validation = validate_output(answers_file, self.config.question_count)

        return {
            'run_id': f'mdemg_run{run_num}',
            'type': 'mdemg',
            'sequence': run_num,
            'status': 'valid' if validation['valid'] else 'partial' if validation['completion_rate'] > 0.9 else 'invalid',
            'timestamp': datetime.utcnow().isoformat() + 'Z',
            'completion': {
                'questions_answered': validation['answer_count'],
                'questions_expected': self.config.question_count,
                'completion_rate': validation['completion_rate']
            },
            'timing': {
                'duration_seconds': round(duration, 1)
            },
            'output': {
                'file_path': str(answers_file)
            },
            'learning_edges': {
                'before': edges_before,
                'after': edges_after,
                'delta': edges_after - edges_before
            },
            'validation': validation,
            'agent_result': result
        }

    def grade_run(self, run_data: Dict, grades_file: Path) -> Dict:
        """Grade a completed run."""
        answers_file = Path(run_data['output']['file_path'])

        if not answers_file.exists() or run_data['status'] == 'invalid':
            return {'graded': False, 'reason': 'Invalid or incomplete run'}

        try:
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
                'mean_score': aggregate.mean,
                'std_dev': aggregate.std,
                'cv_pct': aggregate.cv_pct,
                'high_score_rate': aggregate.high_score_rate,
                'evidence_rate': aggregate.evidence_rate,
                'evidence_tiers': {
                    'strong': sum(1 for g in grades if g.evidence_details.tier == 'strong'),
                    'moderate': sum(1 for g in grades if g.evidence_details.tier == 'moderate'),
                    'weak': sum(1 for g in grades if g.evidence_details.tier == 'weak'),
                    'none': sum(1 for g in grades if g.evidence_details.tier == 'none')
                }
            }
        except Exception as e:
            return {'graded': False, 'error': str(e)}

    def run_benchmark(self, output_dir: Path, mode: str = 'compare') -> Dict:
        """Run the full benchmark.

        Args:
            output_dir: Directory for all output files
            mode: 'baseline', 'mdemg', or 'compare'

        Returns:
            Complete benchmark summary
        """
        output_dir.mkdir(parents=True, exist_ok=True)

        # Load questions
        questions = self.load_questions()
        print(f"Loaded {len(questions)} questions")
        print(f"  Master file: {self.config.questions_master_file}")
        print(f"  Agent file: {self.config.questions_agent_file or '(auto-generated)'}")

        # Save config
        self.config.created_at = datetime.utcnow().isoformat() + 'Z'
        with open(output_dir / 'config.json', 'w') as f:
            json.dump(self.config.to_dict(), f, indent=2)

        # Create agent questions file (no answers)
        questions_file = self.prepare_agent_questions_file(output_dir)

        all_runs = []
        baseline_grades_data = []
        mdemg_grades_data = []

        # ==== PHASE 1: BASELINE RUNS (concurrent is OK) ====
        if mode in ('baseline', 'compare'):
            baseline_dir = output_dir / 'baseline'
            baseline_dir.mkdir(exist_ok=True)

            print(f"\n{'='*60}")
            print(f"PHASE 1: BASELINE RUNS ({self.config.runs_per_mode} runs)")
            print(f"{'='*60}")
            print("Baseline agents search codebase directly - NO MDEMG access")

            for run_num in range(1, self.config.runs_per_mode + 1):
                run_data = self.run_baseline(run_num, baseline_dir, questions_file)

                # Grade immediately
                grades_file = baseline_dir / f'run_{run_num}_grades.json'
                run_data['grading'] = self.grade_run(run_data, grades_file)

                all_runs.append(run_data)

                if run_data['grading'].get('graded'):
                    with open(grades_file) as f:
                        baseline_grades_data.append(json.load(f).get('per_question', []))
                    print(f"  Run {run_num}: mean={run_data['grading']['mean_score']:.3f}, "
                          f"evidence={run_data['grading']['evidence_rate']*100:.1f}%")
                else:
                    print(f"  Run {run_num}: FAILED TO GRADE - {run_data['grading']}")

        # ==== PHASE 2: MDEMG RUNS (SEQUENTIAL - for activation strengthening) ====
        if mode in ('mdemg', 'compare'):
            mdemg_dir = output_dir / 'mdemg'
            mdemg_dir.mkdir(exist_ok=True)

            print(f"\n{'='*60}")
            print(f"PHASE 2: MDEMG RUNS ({self.config.runs_per_mode} runs - SEQUENTIAL)")
            print(f"{'='*60}")
            print("MDEMG agents use retrieval API + file reading")
            print("Sequential execution allows learning edge accumulation")

            for run_num in range(1, self.config.runs_per_mode + 1):
                # MUST run sequentially - wait for each to complete
                run_data = self.run_mdemg(run_num, mdemg_dir, questions_file)

                # Grade immediately
                grades_file = mdemg_dir / f'run_{run_num}_grades.json'
                run_data['grading'] = self.grade_run(run_data, grades_file)

                all_runs.append(run_data)

                if run_data['grading'].get('graded'):
                    with open(grades_file) as f:
                        mdemg_grades_data.append(json.load(f).get('per_question', []))

                    edges = run_data.get('learning_edges', {})
                    print(f"  Run {run_num}: mean={run_data['grading']['mean_score']:.3f}, "
                          f"evidence={run_data['grading']['evidence_rate']*100:.1f}%, "
                          f"edges={edges.get('before', 0)}->{edges.get('after', 0)} (+{edges.get('delta', 0)})")
                else:
                    print(f"  Run {run_num}: FAILED TO GRADE - {run_data['grading']}")

        # ==== PHASE 3: STATISTICAL COMPARISON ====
        comparison = None
        if mode == 'compare' and baseline_grades_data and mdemg_grades_data:
            print(f"\n{'='*60}")
            print("PHASE 3: STATISTICAL COMPARISON")
            print(f"{'='*60}")

            try:
                analyzer = StatsAnalyzer()
                comparison_result = analyzer.compare(baseline_grades_data, mdemg_grades_data)
                comparison = comparison_result.to_dict()

                with open(output_dir / 'comparison.json', 'w') as f:
                    json.dump(comparison, f, indent=2)

                # Generate markdown report
                generator = ReportGenerator()
                report_config = BenchmarkConfig(
                    endpoint=self.config.mdemg_endpoint,
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

            except Exception as e:
                print(f"Error during comparison: {e}")
                comparison = {'error': str(e)}

        # ==== FINAL SUMMARY ====
        summary = {
            '$schema': 'benchmark_summary_v2',
            'metadata': {
                'benchmark_id': f"{self.config.space_id}-{datetime.now().strftime('%Y%m%d-%H%M%S')}",
                'date': self.config.created_at,
                'framework_version': '2.0',
                'mode': mode
            },
            'environment': {
                'mdemg_endpoint': self.config.mdemg_endpoint,
                'space_id': self.config.space_id,
                'model': self.config.model,
                'target_repo': {
                    'name': self.config.repo_name,
                    'path': self.config.repo_path
                }
            },
            'configuration': {
                'question_count': self.config.question_count,
                'runs_per_mode': self.config.runs_per_mode,
                'questions_master': self.config.questions_master_file,
                'questions_agent': self.config.questions_agent_file
            },
            'runs': all_runs,
            'aggregate': self._compute_aggregate(all_runs),
            'comparison': comparison
        }

        with open(output_dir / 'summary.json', 'w') as f:
            json.dump(summary, f, indent=2)

        print(f"\n{'='*60}")
        print("BENCHMARK COMPLETE")
        print(f"{'='*60}")
        print(f"Output directory: {output_dir}")
        print(f"Summary: {output_dir / 'summary.json'}")
        if comparison:
            print(f"Report: {output_dir / 'report.md'}")

        return summary

    def _compute_aggregate(self, runs: List[Dict]) -> Dict:
        """Compute aggregate metrics from runs."""
        baseline_runs = [r for r in runs if r['type'] == 'baseline' and r['grading'].get('graded')]
        mdemg_runs = [r for r in runs if r['type'] == 'mdemg' and r['grading'].get('graded')]

        def avg(values):
            return round(sum(values) / len(values), 4) if values else None

        aggregate = {
            'baseline': {
                'valid_runs': len(baseline_runs),
                'mean_score': avg([r['grading']['mean_score'] for r in baseline_runs]),
                'evidence_rate': avg([r['grading']['evidence_rate'] for r in baseline_runs])
            },
            'mdemg': {
                'valid_runs': len(mdemg_runs),
                'mean_score': avg([r['grading']['mean_score'] for r in mdemg_runs]),
                'evidence_rate': avg([r['grading']['evidence_rate'] for r in mdemg_runs]),
                'total_learning_edges': sum(r.get('learning_edges', {}).get('delta', 0) for r in mdemg_runs)
            }
        }

        # Comparison delta
        if aggregate['baseline']['mean_score'] and aggregate['mdemg']['mean_score']:
            delta = aggregate['mdemg']['mean_score'] - aggregate['baseline']['mean_score']
            aggregate['comparison'] = {
                'score_delta': round(delta, 4),
                'score_delta_percent': round(delta / aggregate['baseline']['mean_score'] * 100, 2) if aggregate['baseline']['mean_score'] > 0 else 0
            }

        return aggregate


# =============================================================================
# CLI
# =============================================================================

def main():
    parser = argparse.ArgumentParser(
        description='MDEMG Benchmark Runner v2 - Agent-based comparison',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Full comparison (3 baseline + 3 MDEMG runs)
    python benchmark_runner_v2.py \\
        --questions questions_master.json \\
        --repo-path /path/to/codebase \\
        --space-id my-space \\
        --runs 3 \\
        --mode compare

    # Baseline only
    python benchmark_runner_v2.py \\
        --questions questions_master.json \\
        --repo-path /path/to/codebase \\
        --mode baseline --runs 3

    # MDEMG only with custom endpoint
    python benchmark_runner_v2.py \\
        --questions questions_master.json \\
        --repo-path /path/to/codebase \\
        --space-id my-space \\
        --endpoint http://localhost:8080 \\
        --mode mdemg --runs 3
"""
    )

    parser.add_argument('--questions', '-q', required=True,
                        help='Path to master questions JSON (with expected answers)')
    parser.add_argument('--questions-agent', '-a',
                        help='Path to agent questions JSON (without answers). Auto-generated if not provided.')
    parser.add_argument('--repo-path', '-r', required=True,
                        help='Path to target codebase')
    parser.add_argument('--repo-name', '-n',
                        help='Name of target codebase (default: directory name)')
    parser.add_argument('--space-id', '-s',
                        help='MDEMG space ID (required for MDEMG mode)')
    parser.add_argument('--endpoint', '-e', default='http://localhost:9999',
                        help='MDEMG endpoint (default: http://localhost:9999)')
    parser.add_argument('--runs', type=int, default=3,
                        help='Runs per mode (default: 3)')
    parser.add_argument('--mode', '-m', choices=['baseline', 'mdemg', 'compare'],
                        default='compare',
                        help='Benchmark mode (default: compare)')
    parser.add_argument('--model', default='haiku',
                        choices=['haiku', 'sonnet', 'opus'],
                        help='Model for agents (default: haiku)')
    parser.add_argument('--output-dir', '-o',
                        help='Output directory (default: benchmark_run_TIMESTAMP)')

    args = parser.parse_args()

    # Validate inputs
    questions_path = Path(args.questions)
    if not questions_path.exists():
        print(f"ERROR: Questions file not found: {questions_path}")
        sys.exit(1)

    repo_path = Path(args.repo_path)
    if not repo_path.exists():
        print(f"ERROR: Repo path not found: {repo_path}")
        sys.exit(1)

    if args.mode in ('mdemg', 'compare') and not args.space_id:
        print("ERROR: --space-id required for MDEMG mode")
        sys.exit(1)

    # Setup output directory
    if args.output_dir:
        output_dir = Path(args.output_dir)
    else:
        output_dir = Path(f"benchmark_run_{datetime.now().strftime('%Y%m%d_%H%M%S')}")

    # Create config
    config = BenchmarkRunConfig(
        repo_path=str(repo_path.absolute()),
        repo_name=args.repo_name or repo_path.name,
        mdemg_endpoint=args.endpoint,
        space_id=args.space_id or '',
        questions_master_file=str(questions_path),
        questions_agent_file=args.questions_agent or '',
        runs_per_mode=args.runs,
        model=args.model
    )

    # Check MDEMG server if needed
    if args.mode in ('mdemg', 'compare'):
        client = MdemgClient(args.endpoint, args.space_id)
        print(f"Checking MDEMG server at {args.endpoint}...")
        if not client.health_check():
            print(f"ERROR: MDEMG server not available at {args.endpoint}")
            sys.exit(1)
        print("MDEMG Server: OK")

        stats = client.get_stats()
        print(f"Space: {args.space_id}")
        print(f"Memory count: {stats.get('memory_count', 0)}")
        print(f"Learning edges: {stats.get('learning_activity', {}).get('co_activated_edges', 0)}")

    # Run benchmark
    runner = BenchmarkRunnerV2(config)
    runner.run_benchmark(output_dir, args.mode)


if __name__ == '__main__':
    main()
