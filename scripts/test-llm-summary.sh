#!/bin/bash
# test-llm-summary.sh - End-to-end test for LLM semantic summary generation
#
# This script tests the LLM summary integration with the ingest-codebase command.
# It runs ingestion on a small test directory, measures performance, and validates output.
#
# Usage: ./scripts/test-llm-summary.sh [options]
# Options:
#   --provider <openai|ollama>  LLM provider (default: openai)
#   --model <model-name>        Model to use (default: gpt-4o-mini)
#   --test-dir <path>           Directory to ingest (default: docs/tests/llm-summary/sample_files)
#   --verbose                   Enable verbose output
#   --dry-run                   Show what would be done without executing
#   --skip-ingest               Skip ingestion, only run analysis on existing data

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
PROVIDER="openai"
MODEL="gpt-4o-mini"
TEST_DIR="docs/tests/llm-summary/sample_files"
VERBOSE=false
DRY_RUN=false
SKIP_INGEST=false
SPACE_ID="llm-summary-test"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --provider)
            PROVIDER="$2"
            shift 2
            ;;
        --model)
            MODEL="$2"
            shift 2
            ;;
        --test-dir)
            TEST_DIR="$2"
            shift 2
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --skip-ingest)
            SKIP_INGEST=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  --provider <openai|ollama>  LLM provider (default: openai)"
            echo "  --model <model-name>        Model to use (default: gpt-4o-mini)"
            echo "  --test-dir <path>           Directory to ingest (default: docs/tests/llm-summary/sample_files)"
            echo "  --verbose                   Enable verbose output"
            echo "  --dry-run                   Show what would be done without executing"
            echo "  --skip-ingest               Skip ingestion, only run analysis"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

echo -e "${BLUE}=== LLM Summary Integration Test ===${NC}"
echo "Provider: $PROVIDER"
echo "Model: $MODEL"
echo "Test directory: $TEST_DIR"
echo "Space ID: $SPACE_ID"
echo ""

# Check prerequisites
check_prerequisites() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"

    # Check if ingest-codebase binary exists
    if [ ! -f "$PROJECT_ROOT/ingest-codebase" ]; then
        echo -e "${RED}Error: ingest-codebase binary not found.${NC}"
        echo "Run: go build -o ingest-codebase ./cmd/ingest-codebase"
        exit 1
    fi

    # Check if test directory exists
    if [ ! -d "$PROJECT_ROOT/$TEST_DIR" ]; then
        echo -e "${RED}Error: Test directory not found: $TEST_DIR${NC}"
        exit 1
    fi

    # Check API key for OpenAI
    if [ "$PROVIDER" = "openai" ] && [ -z "$OPENAI_API_KEY" ]; then
        echo -e "${RED}Error: OPENAI_API_KEY environment variable not set.${NC}"
        exit 1
    fi

    # Check Ollama availability
    if [ "$PROVIDER" = "ollama" ]; then
        OLLAMA_ENDPOINT="${OLLAMA_ENDPOINT:-http://localhost:11434}"
        if ! curl -s "$OLLAMA_ENDPOINT/api/tags" > /dev/null 2>&1; then
            echo -e "${RED}Error: Ollama not available at $OLLAMA_ENDPOINT${NC}"
            exit 1
        fi
    fi

    # Check if MDEMG server is running
    MDEMG_ENDPOINT="${LISTEN_ADDR:-:8090}"
    if [[ "$MDEMG_ENDPOINT" == :* ]]; then
        MDEMG_ENDPOINT="http://localhost$MDEMG_ENDPOINT"
    fi

    if ! curl -s "$MDEMG_ENDPOINT/health" > /dev/null 2>&1; then
        echo -e "${RED}Error: MDEMG server not available at $MDEMG_ENDPOINT${NC}"
        echo "Start the server with: ./mdemg-server"
        exit 1
    fi

    echo -e "${GREEN}Prerequisites OK${NC}"
    echo ""
}

# Run ingestion with LLM summaries
run_ingest() {
    echo -e "${YELLOW}Running ingestion with LLM summaries...${NC}"

    INGEST_ARGS=(
        "--path=$PROJECT_ROOT/$TEST_DIR"
        "--space-id=$SPACE_ID"
        "--llm-summary"
        "--llm-summary-provider=$PROVIDER"
        "--llm-summary-model=$MODEL"
        "--llm-summary-batch=5"
        "--consolidate=false"
        "--batch=50"
        "--workers=2"
    )

    if [ "$VERBOSE" = true ]; then
        INGEST_ARGS+=("--verbose")
    fi

    if [ "$DRY_RUN" = true ]; then
        INGEST_ARGS+=("--dry-run")
        echo "Would run: ./ingest-codebase ${INGEST_ARGS[*]}"
        return 0
    fi

    # Capture timing and output
    START_TIME=$(date +%s%N)

    INGEST_OUTPUT=$(./ingest-codebase "${INGEST_ARGS[@]}" 2>&1)
    INGEST_EXIT=$?

    END_TIME=$(date +%s%N)
    DURATION_MS=$(( (END_TIME - START_TIME) / 1000000 ))

    if [ $INGEST_EXIT -ne 0 ]; then
        echo -e "${RED}Ingestion failed:${NC}"
        echo "$INGEST_OUTPUT"
        exit 1
    fi

    echo "$INGEST_OUTPUT"
    echo ""
    echo -e "${GREEN}Ingestion completed in ${DURATION_MS}ms${NC}"

    # Extract metrics from output
    TOTAL=$(echo "$INGEST_OUTPUT" | grep -oP 'Total: \K\d+' || echo "0")
    INGESTED=$(echo "$INGEST_OUTPUT" | grep -oP 'Ingested: \K\d+' || echo "0")
    ERRORS=$(echo "$INGEST_OUTPUT" | grep -oP 'Errors: \K\d+' || echo "0")

    # Extract LLM summary stats
    LLM_CALLS=$(echo "$INGEST_OUTPUT" | grep -oP 'calls=\K\d+' || echo "0")
    CACHE_HITS=$(echo "$INGEST_OUTPUT" | grep -oP 'cache_hits=\K\d+' || echo "0")
    CACHE_SIZE=$(echo "$INGEST_OUTPUT" | grep -oP 'cache_size=\K\d+' || echo "0")

    # Report metrics
    echo ""
    echo -e "${BLUE}=== Ingestion Metrics ===${NC}"
    echo "Elements: $TOTAL total, $INGESTED ingested, $ERRORS errors"
    echo "Duration: ${DURATION_MS}ms"
    if [ "$TOTAL" != "0" ]; then
        RATE=$(echo "scale=2; $TOTAL * 1000 / $DURATION_MS" | bc 2>/dev/null || echo "N/A")
        echo "Rate: $RATE elements/sec"
    fi
    echo ""
    echo -e "${BLUE}=== LLM Summary Stats ===${NC}"
    echo "API calls: $LLM_CALLS"
    echo "Cache hits: $CACHE_HITS"
    echo "Cache size: $CACHE_SIZE"
    if [ "$LLM_CALLS" != "0" ]; then
        HIT_RATE=$(echo "scale=2; $CACHE_HITS * 100 / $LLM_CALLS" | bc 2>/dev/null || echo "N/A")
        echo "Cache hit rate: $HIT_RATE%"
    fi

    # Store metrics for comparison
    echo "$DURATION_MS" > /tmp/llm-summary-test-duration.txt
    echo "$LLM_CALLS,$CACHE_HITS,$CACHE_SIZE" > /tmp/llm-summary-test-stats.txt
}

# Run baseline ingestion without LLM summaries for comparison
run_baseline() {
    echo ""
    echo -e "${YELLOW}Running baseline ingestion (no LLM summaries)...${NC}"

    BASELINE_ARGS=(
        "--path=$PROJECT_ROOT/$TEST_DIR"
        "--space-id=${SPACE_ID}-baseline"
        "--consolidate=false"
        "--batch=50"
        "--workers=2"
    )

    if [ "$DRY_RUN" = true ]; then
        echo "Would run: ./ingest-codebase ${BASELINE_ARGS[*]}"
        return 0
    fi

    START_TIME=$(date +%s%N)

    BASELINE_OUTPUT=$(./ingest-codebase "${BASELINE_ARGS[@]}" 2>&1)
    BASELINE_EXIT=$?

    END_TIME=$(date +%s%N)
    BASELINE_DURATION_MS=$(( (END_TIME - START_TIME) / 1000000 ))

    if [ $BASELINE_EXIT -ne 0 ]; then
        echo -e "${RED}Baseline ingestion failed${NC}"
        return 1
    fi

    echo -e "${GREEN}Baseline completed in ${BASELINE_DURATION_MS}ms${NC}"

    # Compare with LLM summary run
    if [ -f /tmp/llm-summary-test-duration.txt ]; then
        LLM_DURATION=$(cat /tmp/llm-summary-test-duration.txt)
        OVERHEAD=$(echo "scale=2; ($LLM_DURATION - $BASELINE_DURATION_MS) * 100 / $BASELINE_DURATION_MS" | bc 2>/dev/null || echo "N/A")
        echo ""
        echo -e "${BLUE}=== Performance Comparison ===${NC}"
        echo "Baseline duration: ${BASELINE_DURATION_MS}ms"
        echo "LLM summary duration: ${LLM_DURATION}ms"
        echo "LLM overhead: $OVERHEAD%"
    fi
}

# Query and validate summaries in Neo4j
validate_summaries() {
    echo ""
    echo -e "${YELLOW}Validating summaries in Neo4j...${NC}"

    if [ "$DRY_RUN" = true ]; then
        echo "Would query Neo4j for summary validation"
        return 0
    fi

    MDEMG_ENDPOINT="${LISTEN_ADDR:-:8090}"
    if [[ "$MDEMG_ENDPOINT" == :* ]]; then
        MDEMG_ENDPOINT="http://localhost$MDEMG_ENDPOINT"
    fi

    # Query for nodes with summaries
    QUERY_RESPONSE=$(curl -s -X POST "$MDEMG_ENDPOINT/v1/memory/retrieve" \
        -H "Content-Type: application/json" \
        -d "{
            \"space_id\": \"$SPACE_ID\",
            \"query\": \"code summary\",
            \"limit\": 20
        }")

    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to query MDEMG${NC}"
        return 1
    fi

    # Count nodes with SEMANTIC marker in summary
    SEMANTIC_COUNT=$(echo "$QUERY_RESPONSE" | grep -o "SEMANTIC:" | wc -l | tr -d ' ')
    TOTAL_NODES=$(echo "$QUERY_RESPONSE" | grep -o '"name":' | wc -l | tr -d ' ')

    echo ""
    echo -e "${BLUE}=== Summary Validation ===${NC}"
    echo "Total nodes retrieved: $TOTAL_NODES"
    echo "Nodes with semantic summaries: $SEMANTIC_COUNT"

    if [ "$SEMANTIC_COUNT" -gt 0 ]; then
        echo -e "${GREEN}LLM summaries successfully stored in Neo4j${NC}"
    else
        echo -e "${YELLOW}Warning: No semantic summaries found in retrieved nodes${NC}"
    fi

    # Show sample summaries
    echo ""
    echo -e "${BLUE}=== Sample Summaries ===${NC}"
    echo "$QUERY_RESPONSE" | jq -r '.data.results[]? | "\(.name): \(.summary // "N/A")"' 2>/dev/null | head -5 || \
        echo "Could not parse response (jq may not be installed)"
}

# Run unit and integration tests
run_tests() {
    echo ""
    echo -e "${YELLOW}Running summarize package tests...${NC}"

    if [ "$DRY_RUN" = true ]; then
        echo "Would run: go test ./internal/summarize/..."
        return 0
    fi

    # Run unit tests
    echo "Running unit tests..."
    go test -v ./internal/summarize/... -count=1 2>&1 | tail -20

    # Run integration tests if API key is available
    if [ -n "$OPENAI_API_KEY" ]; then
        echo ""
        echo "Running integration tests..."
        go test -v -tags=integration ./internal/summarize/... -count=1 -timeout=120s 2>&1 | tail -30
    else
        echo "Skipping integration tests (OPENAI_API_KEY not set)"
    fi
}

# Run benchmarks
run_benchmarks() {
    echo ""
    echo -e "${YELLOW}Running benchmarks...${NC}"

    if [ "$DRY_RUN" = true ]; then
        echo "Would run: go test -bench=. ./internal/summarize/..."
        return 0
    fi

    go test -bench=. -benchmem ./internal/summarize/... -run=^$ 2>&1 | grep -E "Benchmark|ns/op|B/op"
}

# Cleanup test data
cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up test data...${NC}"

    if [ "$DRY_RUN" = true ]; then
        echo "Would clean up test spaces: $SPACE_ID, ${SPACE_ID}-baseline"
        return 0
    fi

    # Note: This would require an API endpoint to delete spaces
    echo "Test data remains in spaces: $SPACE_ID, ${SPACE_ID}-baseline"
    echo "Manual cleanup may be required."
}

# Generate report
generate_report() {
    echo ""
    echo -e "${BLUE}======================================${NC}"
    echo -e "${BLUE}=== LLM Summary Test Report ===${NC}"
    echo -e "${BLUE}======================================${NC}"
    echo ""
    echo "Test completed at: $(date)"
    echo "Provider: $PROVIDER"
    echo "Model: $MODEL"
    echo "Test directory: $TEST_DIR"
    echo ""

    if [ -f /tmp/llm-summary-test-duration.txt ] && [ -f /tmp/llm-summary-test-stats.txt ]; then
        LLM_DURATION=$(cat /tmp/llm-summary-test-duration.txt)
        IFS=',' read -r CALLS HITS SIZE < /tmp/llm-summary-test-stats.txt

        echo "Metrics:"
        echo "  - Total duration: ${LLM_DURATION}ms"
        echo "  - LLM API calls: $CALLS"
        echo "  - Cache hits: $HITS"
        echo "  - Cache size: $SIZE"

        if [ "$CALLS" != "0" ]; then
            HIT_RATE=$(echo "scale=2; $HITS * 100 / $CALLS" | bc 2>/dev/null || echo "N/A")
            AVG_TIME=$(echo "scale=2; $LLM_DURATION / $CALLS" | bc 2>/dev/null || echo "N/A")
            echo "  - Cache hit rate: $HIT_RATE%"
            echo "  - Avg time per call: ${AVG_TIME}ms"
        fi
    fi

    echo ""
    echo -e "${GREEN}Test completed successfully${NC}"
}

# Main execution
main() {
    check_prerequisites

    if [ "$SKIP_INGEST" = false ]; then
        run_ingest
        run_baseline
    fi

    validate_summaries
    run_tests
    run_benchmarks
    cleanup
    generate_report
}

main
