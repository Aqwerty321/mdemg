# MDEMG Makefile
# Build, test, and utility targets

.PHONY: all build build-parser test test-parsers clean

# Default target
all: build

# Build all binaries
build: build-parser
	@echo "Build complete"

# Build the parser tools
build-parser:
	@echo "Building extract-symbols..."
	@mkdir -p bin
	go build -o bin/extract-symbols ./cmd/extract-symbols
	@echo "Building ingest-codebase..."
	go build -o bin/ingest-codebase ./cmd/ingest-codebase

# Run all tests
test: test-parsers
	@echo "All tests complete"

# Run UPTS parser validation tests
# Validates language parsers against Universal Parser Test Specifications
test-parsers: build-parser
	@echo "Running UPTS parser validation..."
	python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate-all \
		--spec-dir docs/lang-parser/lang-parse-spec/upts/specs/ \
		--parser "./bin/extract-symbols --json" \
		--report /tmp/parser-report.json
	@echo "Report saved to /tmp/parser-report.json"

# Validate single language parser
# Usage: make test-parser-go, test-parser-python, test-parser-typescript
test-parser-%: build-parser
	@echo "Validating $* parser..."
	python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate \
		--spec docs/lang-parser/lang-parse-spec/upts/specs/$*.upts.json \
		--parser "./bin/extract-symbols --json"

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f /tmp/parser-report.json

# Install development dependencies
dev-setup:
	@echo "Setting up development environment..."
	go mod download
	@echo "Done"

# Run the MDEMG server
run:
	go run ./cmd/mdemg

# Help target
help:
	@echo "MDEMG Makefile targets:"
	@echo "  build          - Build all binaries"
	@echo "  build-parser   - Build ingest-codebase parser"
	@echo "  test           - Run all tests"
	@echo "  test-parsers   - Run UPTS parser validation (all languages)"
	@echo "  test-parser-X  - Run UPTS validation for language X (go, python, typescript)"
	@echo "  clean          - Remove build artifacts"
	@echo "  dev-setup      - Install dependencies"
	@echo "  run            - Run MDEMG server"
