# Language Parser Framework

This directory contains modular language parsers for the MDEMG codebase ingestion tool. Each programming language is implemented as a separate parser that auto-registers at program startup.

## Supported Languages

| Language | File | Extensions |
|----------|------|------------|
| Go | `go_parser.go` | `.go` |
| Rust | `rust_parser.go` | `.rs` |
| Python | `python_parser.go` | `.py` |
| TypeScript/JavaScript | `typescript_parser.go` | `.ts`, `.tsx`, `.js`, `.jsx` |
| Java | `java_parser.go` | `.java` |
| C++ | `cpp_parser.go` | `.cpp`, `.cxx`, `.cc`, `.hpp`, `.hxx`, `.h` (C++ headers) |
| C | `c_parser.go` | `.c`, `.h` (C headers) |
| CUDA | `cuda_parser.go` | `.cu`, `.cuh` |
| SQL | `sql_parser.go` | `.sql` |
| JSON | `json_parser.go` | `.json` |
| Markdown | `markdown_parser.go` | `.md`, `.markdown` |
| XML | `xml_parser.go` | `.xml`, `.xsd`, `.xsl`, etc. |

## Adding a New Language

### Step 1: Create the Parser File

Create a new file `<language>_parser.go` in this directory.

### Step 2: Implement the LanguageParser Interface

```go
package languages

func init() {
    Register(&MyLangParser{})
}

type MyLangParser struct{}

func (p *MyLangParser) Name() string {
    return "mylang"
}

func (p *MyLangParser) Extensions() []string {
    return []string{".ml", ".myl"}
}

func (p *MyLangParser) CanParse(path string) bool {
    return strings.HasSuffix(strings.ToLower(path), ".ml") ||
           strings.HasSuffix(strings.ToLower(path), ".myl")
}

func (p *MyLangParser) IsTestFile(path string) bool {
    // Return true if path is a test file
    return strings.Contains(path, "/test/") ||
           strings.HasSuffix(path, "_test.ml")
}

func (p *MyLangParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
    // Parse the file and return code elements
    var elements []CodeElement

    content, err := ReadFileContent(path)
    if err != nil {
        return nil, err
    }

    relPath, _ := filepath.Rel(root, path)
    fileName := filepath.Base(path)

    // Extract structures using regex or AST
    // ...

    // Build content for embedding
    var contentBuilder strings.Builder
    contentBuilder.WriteString(fmt.Sprintf("MyLang file: %s\n", fileName))
    // Add summary information
    contentBuilder.WriteString("\n--- Code ---\n")
    contentBuilder.WriteString(TruncateContent(content, 4000))

    // Detect cross-cutting concerns
    concerns := DetectConcerns(relPath, content)
    tags := []string{"mylang", "module"}

    // Extract symbols if requested
    var symbols []Symbol
    if extractSymbols {
        symbols = p.extractSymbols(content)
    }

    // Create the main file element
    elements = append(elements, CodeElement{
        Name:     fileName,
        Kind:     "module",
        Path:     "/" + relPath,
        Content:  contentBuilder.String(),
        Package:  "...",
        FilePath: relPath,
        Tags:     tags,
        Concerns: concerns,
        Symbols:  symbols,
    })

    return elements, nil
}

func (p *MyLangParser) extractSymbols(content string) []Symbol {
    // Extract constants, functions, classes, etc.
    // ...
    return nil
}
```

### Step 3: Key Components to Implement

#### Name() and Extensions()
Return the language name and supported file extensions.

#### CanParse(path)
Return `true` if this parser should handle the given file path. Usually checks file extension.

#### IsTestFile(path)
Return `true` if the file is a test file. Used to filter test files when `--include-tests=false`.

#### ParseFile(root, path, extractSymbols)
The main parsing function. Should:

1. **Read file content** using `ReadFileContent(path)`
2. **Extract code structures** (classes, functions, etc.) using regex patterns
3. **Build embedding content** - Summary info + truncated code
4. **Detect concerns** using `DetectConcerns(relPath, content)`
5. **Extract symbols** if requested (constants, function signatures)
6. **Return CodeElement slice** for each major structure

#### extractSymbols(content)
Optional but recommended. Extract:
- Constants with their values
- Function signatures
- Class/struct definitions
- Enum values

### Step 4: Use Common Utilities

The `common.go` file provides shared utilities:

```go
// Read file content
content, err := ReadFileContent(path)

// Truncate long content
truncated := TruncateContent(content, 4000)

// Detect cross-cutting concerns (auth, validation, logging, etc.)
concerns := DetectConcerns(relPath, content)

// Check if file is a config file
isConfig := IsConfigFile(path)

// Clean up values (remove quotes, trailing comments)
clean := CleanValue(rawValue)

// Regex helper - returns capture group 1 from all matches
names := FindAllMatches(content, `class\s+(\w+)`)
```

### Step 5: Rebuild

After adding your parser:

```bash
go build ./cmd/ingest-codebase/
```

The parser auto-registers via `init()`, so no other changes needed.

## Architecture

### Compile-Time Registration

Unlike plugin-based systems, parsers are compiled into the binary. This avoids:
- Go plugin version compatibility issues
- Runtime loading errors
- Platform limitations (plugins don't work on Windows)

### Interface-Based Design

All parsers implement `LanguageParser`:

```go
type LanguageParser interface {
    Name() string
    Extensions() []string
    CanParse(path string) bool
    ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error)
    IsTestFile(path string) bool
}
```

### Registry

Parsers register themselves via `Register()` called from `init()`:

```go
func init() {
    Register(&GoParser{})
}
```

The registry provides:
- `GetParser(name)` - Get parser by language name
- `GetParserForFile(path)` - Find parser for a file path
- `AllParsers()` - List all registered parsers
- `SupportedExtensions()` - List all supported extensions

## Data Structures

### CodeElement

Represents a parsed code unit (file, class, function, etc.):

```go
type CodeElement struct {
    // v1 fields (original)
    Name     string    // Element name (class name, function name, etc.)
    Kind     string    // Code construct: "class", "function", "struct", "module", "kernel", etc.
    Path     string    // Virtual path for linking (e.g., "/src/main.go#Handler")
    Content  string    // Text content for embedding generation
    Summary  string    // Brief summary (from docstrings)
    Package  string    // Package/module name
    FilePath string    // Relative file path
    Tags     []string  // Labels: language, kind, concerns
    Concerns []string  // Cross-cutting concerns detected
    Symbols  []Symbol  // Extracted code symbols

    // v2 fields (evidence and stability)
    ElementKind string // Ingestion unit type: "file", "symbol", "section", "keypath_fact", "unit", "snippet", "migration", "kernel", "other"
    StartLine   int    // First line of element in source file (1-indexed, 0 = not set)
    EndLine     int    // Last line of element in source file
    StableID    string // Deterministic ID for evidence tracking
    Signature   string // Human-readable signature
}
```

### Kind vs ElementKind

| Kind (code construct) | ElementKind (ingestion unit) | Notes |
|-----------------------|------------------------------|-------|
| function | symbol | Standard code symbol |
| class | symbol | Standard code symbol |
| struct | symbol | Standard code symbol |
| kernel | kernel | CUDA GPU kernel |
| module | unit | Represents a compilation unit |
| config | keypath_fact | Config file key-value |
| doc | section | Documentation section |

### Symbol

Represents an extracted code symbol (constant, function signature, etc.):

```go
type Symbol struct {
    Name           string // Symbol name
    Type           string // "constant", "function", "class", "variable", "struct", "enum", "method", "macro", "kernel", "device_function", "typedef"
    Value          string // Value for constants
    RawValue       string // Original value string
    LineNumber     int    // Line number in source
    EndLine        int    // End line (for multi-line)
    Exported       bool   // Whether publicly visible
    DocComment     string // Documentation comment
    Signature      string // Function/method signature
    Parent         string // Parent class/struct
    TypeAnnotation string // Type annotation
    Language       string // Source language
}
```

### CUDA-Specific Symbol Types

The CUDA parser extracts these specialized symbol types:
- `kernel` - CUDA `__global__` kernel functions
- `device_function` - CUDA `__device__` functions (GPU-only)

Example:
```go
Symbol{
    Name: "matmul_kernel",
    Type: "kernel",
    Signature: "__global__ void matmul_kernel(float* A, float* B, float* C, int N)",
    Language: "cuda",
}
```

## Best Practices

1. **Extract meaningful structures** - Classes, functions, interfaces, not individual lines
2. **Include code in embedding** - Raw code improves semantic search quality
3. **Detect concerns** - Use `DetectConcerns()` for cross-cutting concepts
4. **Extract symbols** - Constants and function signatures help evidence-based retrieval
5. **Handle test files** - Implement `IsTestFile()` correctly
6. **Truncate long content** - Use `TruncateContent(content, 4000)` to prevent oversized embeddings
