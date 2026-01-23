<!-- Copy and paste this entire content into a new GitHub issue -->

## 🐛 Bug Report: Two Critical Issues Blocking Spec Creation and PR Workflows

### 📋 Summary
Two high-severity bugs prevent core Auto-Claude functionality:
1. **API Token Limit Error** - Spec creation fails with `max_tokens: 64001 > 64000`
2. **gh CLI Not Found** - PR creation fails despite gh CLI being installed and authenticated

---

## Issue #1: API Token Limit Exceeded During Spec Creation

### 🔴 Severity: High (Blocks spec creation)

### Description
Spec creation consistently fails with API error during complexity assessment and spec writing phases.

### Error Log
```json
API Error: 400 {
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "max_tokens: 64001 > 64000, which is the maximum allowed number of output tokens for claude-opus-4-5-20251101"
  },
  "request_id": "req_011CXBrnRAQxus14vGhqB22c"
}
```

### Steps to Reproduce
1. Create a new specification task in Auto-Claude
2. Allow spec creation process to run
3. Observe failure at "COMPLEXITY ASSESSMENT" or "SPEC DOCUMENT CREATION" phases

### Expected vs Actual
- ✅ **Expected**: Spec creation completes with token request ≤ 64000
- ❌ **Actual**: Fails with request for 64001 tokens (1 over limit)

### Root Cause
`phase_config.py:28` sets `ultrathink` to exactly 64000 tokens. An off-by-one error somewhere in the SDK/API chain pushes this to 64001.

```python
# Current (broken)
"ultrathink": 64000,  # Maximum reasoning depth
```

### Suggested Fix
```python
# Fixed
"ultrathink": 62500,  # Maximum reasoning depth (safety margin for API overhead)
```

### Environment
- **Auto-Claude**: Latest (2026-01-16)
- **Model**: claude-opus-4-5-20251101
- **OS**: macOS (Darwin 25.2.0)
- **Affected Phases**: `discovery`, `spec_writing`, `self_critique`, `complexity_assessment`

---

## Issue #2: gh CLI Not Found When Creating Pull Requests

### 🔴 Severity: High (Blocks PR creation from GUI)

### Description
Creating PRs from the human review Kanban board fails with "gh CLI not found" error, even when gh CLI is properly installed and authenticated.

### Error Message
```
gh CLI not found. Install from https://cli.github.com/
```

### Steps to Reproduce
1. Complete a task and move to "Human Review" on Kanban
2. Click "Create Pull Request" button
3. Error appears despite `gh --version` working in terminal

### Expected vs Actual
- ✅ **Expected**: Auto-Claude finds installed gh CLI and creates PR
- ❌ **Actual**: `FileNotFoundError` - gh CLI not found

### Verification (All Pass ✓)
```bash
$ which gh
/opt/homebrew/bin/gh

$ gh --version
gh version 2.85.0 (2026-01-14)

$ gh auth status
✓ Logged in to github.com as username
✓ Git operations: Configured
✓ API: Authenticated
```

### Root Cause
**GUI PATH Inheritance Issue**

Auto-Claude (Electron app) doesn't inherit the full shell PATH. The gh CLI is typically at:
- `/opt/homebrew/bin/gh` (Apple Silicon Mac)
- `/usr/local/bin/gh` (Intel Mac)
- `%APPDATA%\npm\gh.cmd` (Windows)

Code uses hardcoded `"gh"` string in subprocess calls:
```python
# core/worktree.py:964
gh_args = ["gh", "pr", "create", ...]  # Fails - not in PATH

# runners/github/gh_client.py:132
cmd = ["gh"] + args  # Fails - not in PATH
```

### Suggested Fix
Create `core/gh_executable.py` following the existing `git_executable.py` pattern:

```python
def get_gh_executable() -> str:
    """Find gh CLI with fallbacks for common install locations."""

    # 1. Try PATH first
    gh_path = shutil.which("gh")
    if gh_path:
        return gh_path

    # 2. Check Homebrew (macOS)
    homebrew_paths = [
        "/opt/homebrew/bin/gh",  # Apple Silicon
        "/usr/local/bin/gh",      # Intel Mac
    ]
    for path in homebrew_paths:
        if os.path.isfile(path) and os.access(path, os.X_OK):
            return path

    # 3. Check Windows common locations
    # 4. Fall back to "gh" (will fail naturally if not found)
    return "gh"
```

Then update:
- `core/worktree.py` → `get_gh_executable()` in PR creation
- `runners/github/gh_client.py` → `get_gh_executable()` in all GitHub operations

### Environment
- **Auto-Claude**: Latest (2026-01-16)
- **gh CLI**: v2.85.0 (Homebrew)
- **OS**: macOS (Apple Silicon)
- **Authentication**: ✓ Configured and working

### Impact
Blocks:
- PR creation from human review UI
- All automated GitHub operations (reviews, comments, checks)
- GitHub integration workflows

### Workaround
Manually create PRs from terminal:
```bash
gh pr create --base main --title "..." --body "..."
```

---

## 💡 Additional Context

### Why These Are Related
Both issues stem from environment/configuration edge cases:
1. Token limit assumes no API overhead
2. PATH assumes shell environment inheritance

### Evidence of Fixes Working
Tested locally with both fixes applied:

**Issue #1**: Changed ultrathink to 62500 → spec creation succeeds ✓
**Issue #2**: Created gh_executable.py → PR creation succeeds ✓

```python
# Tested: Finds gh even with limited PATH
$ python3 -c "from core.gh_executable import get_gh_executable; print(get_gh_executable())"
/opt/homebrew/bin/gh  # ✓ Found successfully
```

### Historical Context
- **Issue #1**: Previously reduced from 65536→64000 but off-by-one persisted
- **Issue #2**: Common pattern in Electron apps; Git finder already solves this successfully

### Files Affected
```
backend/
├── phase_config.py              # Issue #1
├── core/
│   ├── gh_executable.py         # Issue #2 (new file)
│   ├── worktree.py              # Issue #2
└── runners/github/
    └── gh_client.py             # Issue #2
```

---

## 🔧 Request
Would appreciate maintainer review of these fixes. Both issues block critical workflows and have straightforward solutions following existing patterns in the codebase.

**Testing**: Both fixes verified working in production environment with multiple spec creation and PR creation cycles.

---

### Labels
`bug` `high-priority` `spec-creation` `github-integration` `macos` `electron`

### Affects
- [x] Spec Creation
- [x] PR Creation
- [x] GitHub Integration
- [ ] Other (please specify)
