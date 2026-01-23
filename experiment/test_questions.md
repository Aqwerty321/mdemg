# MDEMG Context Retention Experiment - Test Questions

## Scoring Guide
- **1.0**: Completely correct with specific details (file paths, function names, exact behavior)
- **0.5**: Partially correct, missing some specifics
- **0.0**: Unable to answer or completely wrong
- **-1.0**: Confidently wrong answer (hallucination)

---

## Architecture Questions (10 questions)

### Q1: Agent Pipeline Order
What is the exact order of agents in the aci-claude-go pipeline, and what does each agent do?

### Q2: Memory Interface Methods
What are the key methods defined in the `agent.Memory` interface? List at least 4.

### Q3: Git Worktree Strategy
How does aci-claude-go isolate spec work using git worktrees? What is the branch naming convention?

### Q4: TUI Framework
What UI framework does aci-claude-go use for its terminal interface, and how many views does it have?

### Q5: MDEMG Purpose
According to the design philosophy, what should MDEMG store and what should it NOT store?

---

## Implementation Questions (10 questions)

### Q6: Agent Interface
What methods must an agent implement to satisfy the `agent.Agent` interface?

### Q7: Environment Variables
What environment variable configures the Claude model to use, and what is its default value?

### Q8: Spec Directory
What is the default directory for storing specs in aci-claude-go?

### Q9: Server Package Location
Where is the API server entry point located in the codebase?

### Q10: Orchestrator Package
What is the purpose of the `internal/orchestrator` package?

---

## Specific Code Questions (10 questions)

### Q11: Main Entry Point
What is the path to the main CLI entry point?

### Q12: TUI Styles
What color theme/palette is used for the TUI styles?

### Q13: Drift Detection
Where is the drift detection system implemented?

### Q14: GitHub Integration
Where is the GitHub integration code located?

### Q15: Config Package
What is the full package path for configuration management?

---

## Deep Knowledge Questions (5 questions)

### Q16: Worktree Path
What is the path pattern for worktree directories?

### Q17: Branch Prefix
What prefix is used for aci-claude branches?

### Q18: MDEMG Endpoint Default
What is the default endpoint for the MDEMG service?

### Q19: Spec ID Format
How are spec IDs formatted (what pattern)?

### Q20: TUI Components Location
Where are reusable TUI components stored?

---

## Experiment Metadata

- **Codebase**: aci-claude-go
- **Total Files**: ~2,354 (.go + .md files)
- **Experiment Date**: 2026-01-21
- **Scoring Range**: -1 to 1

## Results Template

| Question | Baseline Score | MDEMG Score | Notes |
|----------|---------------|-------------|-------|
| Q1 | | | |
| Q2 | | | |
| Q3 | | | |
| Q4 | | | |
| Q5 | | | |
| Q6 | | | |
| Q7 | | | |
| Q8 | | | |
| Q9 | | | |
| Q10 | | | |
| Q11 | | | |
| Q12 | | | |
| Q13 | | | |
| Q14 | | | |
| Q15 | | | |
| Q16 | | | |
| Q17 | | | |
| Q18 | | | |
| Q19 | | | |
| Q20 | | | |
| **Average** | | | |
