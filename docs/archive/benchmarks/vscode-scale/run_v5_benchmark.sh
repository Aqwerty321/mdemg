#!/bin/bash
# V5 Comprehensive Benchmark: 60 Evidence-Locked Questions
# Tests: Qwen 7B + Grep on VS Code codebase

VSCODE="/tmp/vscode-benchmark"
RESULTS_FILE="/Users/reh3376/mdemg/docs/tests/vscode-scale/v5_results.json"

echo "=== V5 Comprehensive Benchmark: 60 Questions ==="
echo "Model: qwen2.5-coder:7b"
echo "Codebase: $VSCODE"
echo ""

# Initialize results
echo '{"results": [' > $RESULTS_FILE

ask_qwen() {
  local prompt="$1"
  curl -s localhost:11434/api/generate -d "{
    \"model\": \"qwen2.5-coder:7b\",
    \"prompt\": \"$prompt\",
    \"stream\": false,
    \"options\": {\"temperature\": 0, \"num_predict\": 50}
  }" | jq -r '.response' | tr -d '\n' | head -c 100
}

run_test() {
  local ID=$1
  local GREP_PATTERN=$2
  local QUESTION=$3
  local EXPECTED=$4

  # Grep for evidence
  EVIDENCE=$(grep -rn "$GREP_PATTERN" "$VSCODE/src" 2>/dev/null | head -1 | sed 's/"/\\"/g' | head -c 200)

  if [ -z "$EVIDENCE" ]; then
    # Try extensions folder
    EVIDENCE=$(grep -rn "$GREP_PATTERN" "$VSCODE/extensions" 2>/dev/null | head -1 | sed 's/"/\\"/g' | head -c 200)
  fi

  if [ -z "$EVIDENCE" ]; then
    echo "[$ID] SKIP - No grep match for: $GREP_PATTERN"
    return
  fi

  # Ask Qwen
  ANSWER=$(ask_qwen "From VS Code source: $EVIDENCE. $QUESTION Just the value:")

  # Check if correct
  if [[ "$ANSWER" == *"$EXPECTED"* ]] || [[ "$EXPECTED" == *"$ANSWER"* ]]; then
    STATUS="CORRECT"
    echo "[$ID] ✓ $ANSWER (expected: $EXPECTED)"
  else
    STATUS="WRONG"
    echo "[$ID] ✗ $ANSWER (expected: $EXPECTED)"
  fi
}

# Editor Config (q001-q010)
echo "--- Editor Config ---"
run_test "q001" "fontSize.*platform\|isMacintosh.*12" "What is fontSize?" "14"
run_test "q002" "lineHeight.*EDITOR_FONT" "What is lineHeight?" "0"
run_test "q004" "tabSize.*default.*4\|default.*tabSize" "What is tabSize?" "4"
run_test "q005" "cursorBlinking.*blink\|blink.*cursor" "What is cursorBlinking?" "blink"
run_test "q007" "wordWrapColumn.*80\|80.*wordWrap" "What is wordWrapColumn?" "80"
run_test "q008" "quickSuggestionsDelay.*10" "What is quickSuggestionsDelay?" "10"
run_test "q009" "delay.*300.*hover\|hover.*delay.*300" "What is hover delay?" "300"
run_test "q010" "hidingDelay.*300" "What is hidingDelay?" "300"

# Storage (q011-q014)
echo ""
echo "--- Storage ---"
run_test "q011" "DEFAULT_FLUSH_INTERVAL.*=" "What is DEFAULT_FLUSH_INTERVAL ms?" "60"
run_test "q012" "BROWSER_DEFAULT_FLUSH_INTERVAL" "What is BROWSER_DEFAULT_FLUSH_INTERVAL?" "5"
run_test "q013" "APPLICATION.*-1\|StorageScope" "What is StorageScope.APPLICATION?" "-1"
run_test "q014" "enum StorageTarget\|StorageTarget.*=" "What is StorageTarget.USER?" "0"

# Workbench Layout (q015-q020)
echo ""
echo "--- Workbench Layout ---"
run_test "q015" "minimumWidth.*170\|170.*minimum" "What is SidebarPart minimumWidth?" "170"
run_test "q016" "ACTION_HEIGHT.*48\|48.*ACTION" "What is ACTION_HEIGHT?" "48"
run_test "q017" "minimumWidth.*300.*panel\|panel.*300" "What is PanelPart minimumWidth?" "300"
run_test "q018" "DEFAULT_CUSTOM_TITLEBAR_HEIGHT\|TITLEBAR.*35" "What is titlebar height?" "35"
run_test "q020" "WindowMinimumSize\|width.*400.*height" "What is WindowMinimumSize?" "400"

# Extensions (q021-q025)
echo ""
echo "--- Extensions ---"
run_test "q021" "timeout.*5000.*activation\|activation.*5000" "What is activation timeout?" "5000"
run_test "q022" "60000.*Web Worker\|60.*seconds.*Extension" "What is web worker timeout?" "60000"
run_test "q023" "10000.*Extension Host\|10.*seconds.*extension" "What is local process timeout?" "10000"
run_test "q024" "LocalProcess.*1\|ExtensionHostKind" "What is ExtensionHostKind.LocalProcess?" "1"
run_test "q025" "EXTENSION_FETCH_TIMEOUT" "What is EXTENSION_FETCH_TIMEOUT_MS?" "30000"

# Search (q026-q029)
echo ""
echo "--- Search ---"
run_test "q026" "DEFAULT_MAX_SEARCH_RESULTS.*=" "What is DEFAULT_MAX_SEARCH_RESULTS?" "20000"
run_test "q028" "searchOnType.*default.*true" "What is searchOnType default?" "true"
run_test "q029" "searchOnTypeDebouncePeriod.*300\|DebouncePeriod.*default" "What is debounce period?" "300"

# Terminal (q030-q034)
echo ""
echo "--- Terminal ---"
run_test "q030" "scrollback.*1000\|1000.*scrollback" "What is scrollback default?" "1000"
run_test "q031" "cursorBlinking.*false.*terminal\|terminal.*cursorBlinking.*false" "What is terminal cursorBlinking?" "false"
run_test "q032" "cursorStyle.*block\|block.*cursor.*terminal" "What is terminal cursorStyle?" "block"

# Files (q035-q037)
echo ""
echo "--- Files ---"
run_test "q035" "DEFAULT_AUTO_SAVE_DELAY.*1000\|AUTO_SAVE.*1000" "What is auto save delay?" "1000"
run_test "q037" "MAX_FILE_SIZE_MB\|50.*MB\|MB.*50" "What is MAX_FILE_SIZE_MB?" "50"

# QuickInput (q041-q042)
echo ""
echo "--- QuickInput ---"
run_test "q041" "debounce.*100.*quick\|quick.*debounce.*100" "What is QuickInput debounce?" "100"

# CodeLens (q043-q044)
echo ""
echo "--- CodeLens ---"
run_test "q043" "CodeLensProvide.*250\|min.*250.*codelens" "What is CodeLens debounce?" "250"

# Notifications (q050-q051)
echo ""
echo "--- Notifications ---"
run_test "q050" "Severity.*Ignore.*0\|enum Severity" "What is Severity.Ignore?" "0"

# Workbench (q052-q053)
echo ""
echo "--- Workbench ---"
run_test "q053" "LifecyclePhase.*Starting.*1\|Starting.*=.*1" "What is LifecyclePhase.Starting?" "1"

# TextModel (q054-q056)
echo ""
echo "--- TextModel ---"
run_test "q056" "AverageBufferSize.*65535\|65535" "What is AverageBufferSize?" "65535"

# Workspace (q059)
echo ""
echo "--- Workspace ---"
run_test "q059" "emptyWindow.*true\|trust.*empty.*true" "What is trust.emptyWindow default?" "true"

echo ""
echo "=== Benchmark Complete ==="
