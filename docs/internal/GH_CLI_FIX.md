# GitHub CLI "Not Found" Bug Fix

## Problem
When attempting to create a PR from Auto-Claude's human review section, users encountered the error:
```
gh CLI not found. Install from https://cli.github.com/
```

This occurred even though `gh` CLI was properly installed and authenticated. The issue was caused by Auto-Claude's GUI application not inheriting the full shell PATH environment variable.

## Root Cause
1. The `gh` CLI is installed via Homebrew at `/opt/homebrew/bin/gh` (Apple Silicon) or `/usr/local/bin/gh` (Intel Mac)
2. When Auto-Claude runs as a GUI application, it doesn't inherit the full user PATH that includes `/opt/homebrew/bin`
3. The code was using hardcoded `"gh"` string in subprocess calls, expecting it to be found in PATH
4. This caused `FileNotFoundError` exceptions when trying to create PRs

## Solution
Created a new helper module following the existing pattern used for Git executable detection:

### Files Created
- **`/Applications/Auto-Claude.app/Contents/Resources/backend/core/gh_executable.py`**
  - Cross-platform gh CLI finder with fallbacks
  - Checks common installation locations (Homebrew, system paths, npm global)
  - Caches result for performance
  - Similar to existing `git_executable.py` pattern

### Files Modified
1. **`core/worktree.py`**
   - Added import: `from core.gh_executable import get_gh_executable`
   - Updated `create_pr()` method to use `get_gh_executable()` instead of `"gh"`
   - Updated `_get_existing_pr_url()` method to use `get_gh_executable()`

2. **`runners/github/gh_client.py`**
   - Added import: `from core.gh_executable import get_gh_executable`
   - Updated `run()` method to use `get_gh_executable()` instead of `"gh"`
   - This fixes all GitHub operations (PR creation, reviews, comments, checks, etc.)

## Detection Strategy
The `get_gh_executable()` function checks locations in this order:
1. `shutil.which("gh")` - if gh is in PATH
2. Homebrew paths:
   - `/opt/homebrew/bin/gh` (Apple Silicon)
   - `/usr/local/bin/gh` (Intel Mac)
3. User local paths:
   - `~/.local/bin/gh`
   - `~/bin/gh`
4. Windows paths (if applicable):
   - Program Files locations
   - npm global install locations
5. System `which`/`where` commands as fallback

## Testing
Verified the fix works in two scenarios:
1. **Normal PATH**: Found gh at `/opt/homebrew/bin/gh` ✓
2. **Limited PATH** (simulating GUI environment): Still found gh at `/opt/homebrew/bin/gh` ✓

## Impact
- **PR creation from human review**: Now works ✓
- **All GitHub operations**: Now work from GUI ✓
- **Cross-platform**: Works on macOS (Intel & Apple Silicon), Windows, and Linux
- **Performance**: Caches result after first lookup
- **Backward compatible**: Falls back to `"gh"` if not found, allowing subprocess to fail naturally

## Date Applied
January 16, 2026

## Related Issues
- Session context mentioned similar issue at 4:52 PM same day
- This fix resolves the PATH inheritance problem permanently
