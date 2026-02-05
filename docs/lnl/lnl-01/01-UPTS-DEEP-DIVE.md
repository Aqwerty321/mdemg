# UPTS: Universal Parser Test Specification

## What is UPTS?

UPTS is a **language-agnostic test specification format** for validating MDEMG language parsers. It ensures every parser extracts symbols correctly and consistently.

---

## The Problem UPTS Solves

Before UPTS, parser tests were inconsistent:

| Field | Go Parser | Python Parser | TypeScript Parser |
|-------|-----------|---------------|-------------------|
| Line number | `line` | `line_number` | `line` |
| Has signature? | No | Yes | No |
| Has value? | No | Yes | No |

**Result:** Each parser had its own test format. Adding a new language meant inventing a new test structure.

---

## The UPTS Solution

```
┌─────────────────────────────────────────────────────────────┐
│                    UPTS Spec (JSON)                          │
│  - Language name                                             │
│  - File extensions                                           │
│  - Fixture path                                              │
│  - Expected symbols (name, type, line, exported, parent)     │
│  - Validation config                                         │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                    Test Runner                               │
│  1. Load spec                                                │
│  2. Parse fixture with language parser                       │
│  3. Compare actual symbols vs expected                       │
│  4. Report pass/fail with details                            │
└─────────────────────────────────────────────────────────────┘
```

---

## Directory Structure

```
docs/lang-parser/lang-parse-spec/upts/
├── schema/
│   └── upts.schema.json           # JSON Schema definition
│
├── specs/                          # One spec per language
│   ├── go.upts.json
│   ├── rust.upts.json
│   ├── kotlin.upts.json           # ← We'll examine this
│   └── ... (20 total)
│
├── fixtures/                       # Test source files
│   ├── go_test_fixture.go
│   ├── kotlin_test_fixture.kt     # ← Parsed by Kotlin parser
│   └── ... (20 total)
│
└── runners/
    └── upts_runner.py              # Python cross-validator
```

**Go-native harness:** `cmd/ingest-codebase/languages/upts_test.go`

---

## Anatomy of a UPTS Spec

```json
{
  "upts_version": "1.0.0",
  "language": "kotlin",
  "variants": [".kt", ".kts"],

  "metadata": {
    "author": "auto-generated",
    "created": "2026-02-05",
    "description": "Kotlin parser spec"
  },

  "config": {
    "line_tolerance": 2,        // Allow ±2 lines
    "require_all_symbols": true, // All expected must be found
    "allow_extra_symbols": true, // Parser can find more
    "validate_parent": true      // Check parent field
  },

  "fixture": {
    "type": "file",
    "path": "../fixtures/kotlin_test_fixture.kt"
  },

  "expected": {
    "symbol_count": {"min": 50, "max": 60},
    "symbols": [
      {
        "name": "User",
        "type": "class",
        "line": 18,
        "exported": true
      },
      {
        "name": "findById",
        "type": "function",
        "line": 45,
        "exported": true,
        "parent": "UserRepository"
      }
    ]
  }
}
```

---

## Key Concepts

### 1. Symbol Types

Parsers extract these symbol types:

| Type | Examples |
|------|----------|
| `constant` | `const val MAX = 100` |
| `function` | `fun calculate()` |
| `class` | `class User` |
| `struct` | `struct Point` |
| `interface` | `interface Repository` |
| `enum` | `enum class Status` |
| `enum_value` | `ACTIVE, INACTIVE` |
| `method` | `fun User.validate()` |
| `type` | `typealias UserId = String` |

### 2. The 7 Canonical Patterns

Every parser should handle these patterns:

| Pattern | Code | Description |
|---------|------|-------------|
| P1_CONSTANT | Named constants | `const val X = 1` |
| P2_FUNCTION | Standalone functions | `fun calc()` |
| P3_CLASS_STRUCT | Classes/structs | `class User` |
| P4_INTERFACE_TRAIT | Interfaces/traits | `interface Repo` |
| P5_ENUM | Enumerations | `enum Status` |
| P6_METHOD | Methods in classes | `fun User.save()` |
| P7_TYPE_ALIAS | Type aliases | `typealias ID = Int` |

### 3. Line Tolerance

```json
"config": { "line_tolerance": 2 }
```

Allows the actual line number to be within ±2 of expected. Useful when comments shift lines.

### 4. Parent Matching

For methods, `parent` must match the containing class/struct:

```json
{
  "name": "findById",
  "type": "method",
  "parent": "UserRepository"  // Must match exactly
}
```

---

## Running UPTS Tests

### Go-Native (Recommended)

```bash
# All 20 parsers
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v

# Single language
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/kotlin -v
```

### Python Cross-Validator

```bash
python runners/upts_runner.py validate \
    --spec specs/kotlin.upts.json \
    --parser ./bin/ingest-codebase
```

---

## Walkthrough: Adding a New Language Parser

Let's walk through adding a hypothetical **"Zig"** parser.

### Step 1: Create the Parser

Create `cmd/ingest-codebase/languages/zig_parser.go`:

```go
package languages

import (
    "regexp"
    "strings"
)

func init() {
    Register(&ZigParser{})  // Auto-register
}

type ZigParser struct{}

func (p *ZigParser) Name() string        { return "zig" }
func (p *ZigParser) Extensions() []string { return []string{".zig"} }

func (p *ZigParser) CanParse(path string) bool {
    return strings.HasSuffix(strings.ToLower(path), ".zig")
}

func (p *ZigParser) IsTestFile(path string) bool {
    return strings.Contains(path, "_test.zig") ||
           strings.Contains(path, "/test/")
}

func (p *ZigParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
    content, err := ReadFileContent(path)
    if err != nil {
        return nil, err
    }

    var symbols []Symbol
    if extractSymbols {
        symbols = p.extractSymbols(content)
    }

    // ... build CodeElement
    return elements, nil
}

func (p *ZigParser) extractSymbols(content string) []Symbol {
    var symbols []Symbol
    lines := strings.Split(content, "\n")

    // Pattern: pub const NAME = value;
    constRe := regexp.MustCompile(`^(pub\s+)?const\s+(\w+)\s*=`)

    // Pattern: pub fn name(...)
    funcRe := regexp.MustCompile(`^(pub\s+)?fn\s+(\w+)\s*\(`)

    // Pattern: const Name = struct { }
    structRe := regexp.MustCompile(`^(pub\s+)?const\s+(\w+)\s*=\s*struct`)

    for i, line := range lines {
        lineNum := i + 1
        trimmed := strings.TrimSpace(line)

        if m := constRe.FindStringSubmatch(trimmed); m != nil {
            symbols = append(symbols, Symbol{
                Name:     m[2],
                Type:     "constant",
                Line:     lineNum,
                Exported: strings.HasPrefix(trimmed, "pub"),
            })
        }
        // ... more patterns
    }

    return symbols
}
```

### Step 2: Create the Fixture

Create `docs/lang-parser/lang-parse-spec/upts/fixtures/zig_test_fixture.zig`:

```zig
// Zig Test Fixture - Covers all canonical patterns

const std = @import("std");

// P1: Constants
pub const MAX_BUFFER_SIZE: usize = 4096;
pub const VERSION = "1.0.0";
const INTERNAL_LIMIT: u32 = 100;

// P3: Struct
pub const User = struct {
    id: u64,
    name: []const u8,
    active: bool,

    // P6: Method
    pub fn init(id: u64, name: []const u8) User {
        return User{ .id = id, .name = name, .active = true };
    }

    pub fn deactivate(self: *User) void {
        self.active = false;
    }
};

// P2: Functions
pub fn calculateSum(a: i32, b: i32) i32 {
    return a + b;
}

fn internalHelper() void {
    // Private function
}

// P5: Enum
pub const Status = enum {
    active,
    inactive,
    pending,
};

// P4: Error set (Zig's interface-like pattern)
pub const FileError = error{
    NotFound,
    PermissionDenied,
    Timeout,
};
```

### Step 3: Create the UPTS Spec

Create `docs/lang-parser/lang-parse-spec/upts/specs/zig.upts.json`:

```json
{
  "upts_version": "1.0.0",
  "language": "zig",
  "variants": [".zig"],
  "metadata": {
    "author": "your-name",
    "created": "2026-02-06",
    "description": "Zig parser - all canonical patterns"
  },
  "config": {
    "line_tolerance": 2,
    "require_all_symbols": true,
    "allow_extra_symbols": true,
    "validate_parent": true
  },
  "fixture": {
    "type": "file",
    "path": "../fixtures/zig_test_fixture.zig"
  },
  "expected": {
    "symbol_count": {"min": 12, "max": 18},
    "symbols": [
      {
        "name": "MAX_BUFFER_SIZE",
        "type": "constant",
        "line": 7,
        "exported": true,
        "pattern": "P1_CONSTANT"
      },
      {
        "name": "VERSION",
        "type": "constant",
        "line": 8,
        "exported": true
      },
      {
        "name": "INTERNAL_LIMIT",
        "type": "constant",
        "line": 9,
        "exported": false
      },
      {
        "name": "User",
        "type": "struct",
        "line": 12,
        "exported": true,
        "pattern": "P3_CLASS_STRUCT"
      },
      {
        "name": "init",
        "type": "method",
        "line": 18,
        "exported": true,
        "parent": "User",
        "pattern": "P6_METHOD"
      },
      {
        "name": "deactivate",
        "type": "method",
        "line": 22,
        "exported": true,
        "parent": "User"
      },
      {
        "name": "calculateSum",
        "type": "function",
        "line": 28,
        "exported": true,
        "pattern": "P2_FUNCTION"
      },
      {
        "name": "internalHelper",
        "type": "function",
        "line": 32,
        "exported": false
      },
      {
        "name": "Status",
        "type": "enum",
        "line": 37,
        "exported": true,
        "pattern": "P5_ENUM"
      },
      {
        "name": "active",
        "type": "enum_value",
        "line": 38,
        "exported": true,
        "parent": "Status"
      },
      {
        "name": "inactive",
        "type": "enum_value",
        "line": 39,
        "exported": true,
        "parent": "Status"
      },
      {
        "name": "pending",
        "type": "enum_value",
        "line": 40,
        "exported": true,
        "parent": "Status"
      }
    ]
  },
  "patterns_covered": ["P1_CONSTANT", "P2_FUNCTION", "P3_CLASS_STRUCT", "P5_ENUM", "P6_METHOD"]
}
```

### Step 4: Run Validation

```bash
# Build
go build ./cmd/ingest-codebase/...

# Test
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/zig -v
```

### Step 5: Iterate Until Passing

Common issues to fix:
- **Line numbers off:** Adjust spec or check parser line counting
- **Missing symbols:** Add regex pattern to parser
- **Wrong parent:** Fix scope tracking in parser
- **Type mismatch:** Ensure parser uses correct symbol type string

### Step 6: Update Documentation

1. Add to table in `docs/lang-parser/lang-parse-spec/upts/README.md`
2. Add to table in `cmd/ingest-codebase/languages/README.md`
3. Add to `CHANGELOG.md`

---

## Common Pitfalls

### 1. Fixture Path is Relative to Spec File

```json
// WRONG - relative to upts/ root
"path": "fixtures/zig_test_fixture.zig"

// CORRECT - relative to specs/ directory
"path": "../fixtures/zig_test_fixture.zig"
```

### 2. Line Numbers are 1-Indexed

The first line of the file is line 1, not line 0.

### 3. Type Compatibility

These types are treated as equivalent:
- `class` ↔ `struct`
- `interface` ↔ `trait` ↔ `protocol`

### 4. Export Detection

Each language has different export rules:
- **Go:** Capitalized names are exported
- **Python:** No leading underscore = exported
- **Kotlin:** No `private`/`internal` = exported
- **Rust:** `pub` keyword

---

## Q&A: Anticipated Questions

### Q: Why JSON and not YAML for specs?

**A:** JSON has better tooling support (schema validation, jq parsing) and avoids YAML's whitespace pitfalls. The tradeoff is slightly more verbose syntax.

### Q: What if my parser finds more symbols than expected?

**A:** That's fine if `allow_extra_symbols: true` (default). The test only fails if **expected** symbols are missing.

### Q: How do I handle multi-line symbols?

**A:** Use `line_end` in the spec for symbols that span multiple lines. The parser should set `Symbol.EndLine`.

### Q: Can I skip certain symbols in validation?

**A:** Yes, mark them as `"optional": true` in the spec. Missing optional symbols don't cause failure.

### Q: How do I test signature extraction?

**A:** Use `signature_contains` for partial matches:

```json
{
  "name": "processData",
  "signature_contains": ["async", "Result", "error"]
}
```

### Q: What's the relationship between CodeElement and Symbol?

**A:**
- **CodeElement** = A parseable unit (file, class, module) - goes into the graph
- **Symbol** = Extracted metadata (function name, constant value) - searchable

### Q: How do I debug a failing test?

**A:** Run with verbose output:

```bash
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/kotlin -v 2>&1 | head -100
```

Look for:
- "MISSING" = expected symbol not found
- "LINE_MISMATCH" = wrong line number
- "PARENT_MISMATCH" = wrong containing class

---

## Summary

1. **UPTS = Single source of truth** for parser validation
2. **Three components:** Parser + Fixture + Spec
3. **Workflow:** Create parser → Create fixture → Create spec → Iterate until passing
4. **Run tests:** `go test -run TestUPTS`

---

**Next:** [UATS Deep Dive](./02-UATS-DEEP-DIVE.md)
