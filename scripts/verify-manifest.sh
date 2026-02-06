#!/usr/bin/env bash
# verify-manifest.sh — Validates all spec files against their SHA256 checksums.
# Used by CI to ensure spec files haven't been modified without updating the manifest.
#
# Exit codes:
#   0 — all hashes match (or manifest is empty)
#   1 — one or more mismatches or missing files

set -euo pipefail

MANIFEST="docs/specs/manifest.sha256"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

if [ ! -f "$MANIFEST" ]; then
  echo "ERROR: Manifest file not found: $MANIFEST"
  exit 1
fi

# Filter out comments and blank lines
entries=$(grep -v '^#' "$MANIFEST" | grep -v '^\s*$' || true)

if [ -z "$entries" ]; then
  echo "Manifest is empty (no entries to verify). OK."
  exit 0
fi

failures=0
checked=0

while IFS= read -r line; do
  # Parse sha256sum format: <hash>  <filepath>
  expected_hash=$(echo "$line" | awk '{print $1}')
  filepath=$(echo "$line" | awk '{print $2}')

  if [ -z "$expected_hash" ] || [ -z "$filepath" ]; then
    echo "WARN: Skipping malformed line: $line"
    continue
  fi

  if [ ! -f "$filepath" ]; then
    echo "FAIL: File not found: $filepath"
    failures=$((failures + 1))
    continue
  fi

  # Compute actual hash (macOS uses shasum, Linux uses sha256sum)
  if command -v sha256sum &>/dev/null; then
    actual_hash=$(sha256sum "$filepath" | awk '{print $1}')
  elif command -v shasum &>/dev/null; then
    actual_hash=$(shasum -a 256 "$filepath" | awk '{print $1}')
  else
    echo "ERROR: Neither sha256sum nor shasum found"
    exit 1
  fi

  if [ "$expected_hash" = "$actual_hash" ]; then
    echo "OK:   $filepath"
  else
    echo "FAIL: $filepath"
    echo "      expected: $expected_hash"
    echo "      actual:   $actual_hash"
    failures=$((failures + 1))
  fi
  checked=$((checked + 1))
done <<< "$entries"

echo ""
echo "Verified $checked file(s), $failures failure(s)."

if [ "$failures" -gt 0 ]; then
  echo "Manifest verification FAILED."
  exit 1
fi

echo "Manifest verification PASSED."
exit 0
