# Bug Report: Two Critical Issues in Auto-Claude

## Issue 1: API Token Limit Exceeded (max_tokens: 64001 > 64000)

### Description
When creating specifications with Auto-Claude, the system consistently fails during the spec creation process with an API error indicating that the requested token count exceeds Claude Opus 4.5's maximum limit by 1 token.

### Error Message
```
API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: 64001 > 64000, which is the maximum allowed number of output tokens for claude-opus-4-5-20251101"},"request_id":"req_011CXBrnRAQxus14vGhqB22c"}
```

### Reproduction Steps
1. Create a new specification task in Auto-Claude
2. Allow it to proceed to the spec creation phase
3. Phase fails at "COMPLEXITY ASSESSMENT" and "SPEC DOCUMENT CREATION" steps

### Expected Behavior
- Spec creation should complete successfully
- Token requests should stay within the Claude Opus 4.5 limit of 64000 tokens

### Actual Behavior
- Spec creation fails with HTTP 400 error
- System requests 64001 tokens (1 token over the limit)
- Error occurs in phases: `spec_writing`, `complexity_assessment`, `discovery`, `self_critique`

### Environment
- Auto-Claude version: Latest (as of 2026-01-16)
- Model: claude-opus-4-5-20251101
- Platform: macOS (Darwin 25.2.0)

### Root Cause
The `ultrathink` thinking level in `phase_config.py` is configured to use exactly 64000 tokens, which matches the API's maximum. However, somewhere in the SDK/API chain, an additional token is being added (possibly for formatting or control tokens), resulting in a request for 64001 tokens.

**File**: `/Applications/Auto-Claude.app/Contents/Resources/backend/phase_config.py`
**Line 28**: `"ultrathink": 64000`

### Proposed Fix
Reduce the `ultrathink` token budget to 62500, providing a 1500-token safety margin:

```python
THINKING_BUDGET_MAP: dict[str, int | None] = {
    "none": None,
    "low": 1024,
    "medium": 4096,
    "high": 16384,
    "ultrathink": 62500,  # Reduced from 64000 for safety margin
}
```

### Impact
- **Severity**: High - Blocks spec creation entirely
- **Affected Features**: All spec creation workflows using ultrathink mode
- **Workaround**: None available to users without code modification

---

## Issue 2: gh CLI Not Found When Creating Pull Requests

### Description
When attempting to create a pull request from Auto-Claude's human review section (Kanban board), the system fails with a "gh CLI not found" error, despite the GitHub CLI being properly installed and authenticated.

### Error Message
```
gh CLI not found. Install from https://cli.github.com/
```

### Reproduction Steps
1. Complete a task and move it to "Human Review" on the Kanban board
2. Click "Create Pull Request" button
3. Error appears: "gh CLI not found"

### Expected Behavior
- Auto-Claude should find the installed `gh` CLI
- PR creation should proceed normally

### Actual Behavior
- `FileNotFoundError` when trying to execute `gh` command
- PR creation fails completely
- Occurs even when `gh` is confirmed working from terminal (`gh --version` succeeds)

### Environment
- Auto-Claude version: Latest (as of 2026-01-16)
- Platform: macOS (Apple Silicon)
- gh CLI version: 2.85.0
- Installation: Homebrew (`/opt/homebrew/bin/gh`)
- Authentication: Confirmed working (`gh auth status` shows authenticated)

### Root Cause
Auto-Claude runs as a GUI application (Electron) and does not inherit the full shell `PATH` environment variable. The `gh` CLI is typically installed via Homebrew at `/opt/homebrew/bin/gh` (Apple Silicon) or `/usr/local/bin/gh` (Intel Mac), which are not in the limited PATH available to GUI applications on macOS.

The code uses hardcoded `"gh"` string in subprocess calls:
- `subprocess.run(["gh", "pr", "create", ...])` in `core/worktree.py`
- `cmd = ["gh"] + args` in `runners/github/gh_client.py`

This pattern relies on `gh` being found in PATH, which fails in GUI context.

### Proposed Fix
Create a cross-platform gh CLI finder module following the existing pattern used for Git executable detection:

**New file**: `core/gh_executable.py`
```python
def get_gh_executable() -> str:
    """Find the gh CLI executable with fallbacks for common locations."""
    # Check PATH first
    gh_path = shutil.which("gh")
    if gh_path:
        return gh_path

    # macOS: Check Homebrew locations
    homebrew_paths = [
        "/opt/homebrew/bin/gh",  # Apple Silicon
        "/usr/local/bin/gh",      # Intel Mac
        "~/.local/bin/gh",        # User local
    ]
    # ... additional platform-specific paths

    return "gh"  # Fallback
```

**Modified files**:
1. `core/worktree.py` - Update PR creation to use `get_gh_executable()`
2. `runners/github/gh_client.py` - Update all GitHub operations to use `get_gh_executable()`

### Impact
- **Severity**: High - Blocks PR creation entirely from GUI
- **Affected Features**:
  - PR creation from human review
  - All GitHub operations in automated workflows
  - PR reviews, comments, checks, etc.
- **Workaround**: Create PRs manually from terminal using `gh pr create`

### Additional Notes
This issue affects all GUI operations that use the `gh` CLI. It's particularly problematic because:
1. Users can verify `gh` works in terminal, making the error confusing
2. The GitHub integration appears to be configured correctly
3. There's no obvious way for users to fix it without modifying source code

---

## Verification
Both issues have been confirmed and fixed in a local installation:

**Issue 1**: Confirmed by inspecting error logs showing `max_tokens: 64001 > 64000`
**Issue 2**: Confirmed by:
- Checking `which gh` returns `/opt/homebrew/bin/gh`
- Verifying `gh --version` works (v2.85.0)
- Testing `get_gh_executable()` successfully finds gh even with limited PATH

---

## Additional Context
- Session date: 2026-01-16
- These issues appeared after updating to the latest version of Auto-Claude
- Issue #1 was partially addressed in a previous fix (reduced from 65536 to 64000) but the off-by-one error persisted
- Issue #2 is a longstanding PATH inheritance issue common to Electron/GUI applications on macOS

## Related
The Git executable finder (`core/git_executable.py`) already implements this pattern successfully, proving the approach works for similar CLI tools.
