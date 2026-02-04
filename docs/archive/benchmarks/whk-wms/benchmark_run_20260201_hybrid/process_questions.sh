#!/bin/bash

# This script processes all 120 benchmark questions using MDEMG API
# Output: /Users/reh3376/mdemg/docs/benchmarks/whk-wms/benchmark_run_20260201_hybrid/mdemg/run_3_answers.jsonl

QUESTIONS_FILE="/Users/reh3376/mdemg/docs/benchmarks/whk-wms/benchmark_run_20260201_hybrid/agent_questions.json"
OUTPUT_FILE="/Users/reh3376/mdemg/docs/benchmarks/whk-wms/benchmark_run_20260201_hybrid/mdemg/run_3_answers.jsonl"
CODEBASE="/Users/reh3376/whk-wms/apps/whk-wms"

# Clear output file if it exists
> "$OUTPUT_FILE"

echo "Starting benchmark processing at $(date)"
echo "Questions: $QUESTIONS_FILE"
echo "Output: $OUTPUT_FILE"
echo ""

# Count questions
TOTAL=$(jq '.questions | length' "$QUESTIONS_FILE")
echo "Total questions to process: $TOTAL"
