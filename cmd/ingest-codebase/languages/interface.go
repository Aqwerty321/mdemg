// Package languages provides a modular framework for parsing different programming languages.
// Each language is implemented as a separate parser that implements the LanguageParser interface.
package languages

// CodeElement represents a parsed code element (function, class, struct, etc.)
type CodeElement struct {
	Name     string
	Kind     string   // "function", "class", "struct", "interface", "enum", "trait", "module"
	Path     string   // Relative path to file
	Content  string   // Source code content
	Summary  string   // Brief summary from docstrings/comments
	Package  string   // Package/module name
	FilePath string   // Full file path
	Tags     []string // Language, file type, etc.
	Concerns []string // Cross-cutting concerns detected
	Symbols  []Symbol // Extracted code symbols
}

// Symbol represents an extracted code symbol (constant, function signature, etc.)
type Symbol struct {
	Name           string `json:"name"`
	Type           string `json:"type"` // "constant", "function", "class", "interface", "variable"
	Value          string `json:"value,omitempty"`
	RawValue       string `json:"raw_value,omitempty"`
	LineNumber     int    `json:"line_number"`
	EndLine        int    `json:"end_line,omitempty"`
	Exported       bool   `json:"exported"`
	DocComment     string `json:"doc_comment,omitempty"`
	Signature      string `json:"signature,omitempty"`
	Parent         string `json:"parent,omitempty"` // Parent class/struct/module
	TypeAnnotation string `json:"type_annotation,omitempty"`
	Language       string `json:"language,omitempty"`
}

// LanguageParser defines the interface that all language parsers must implement.
type LanguageParser interface {
	// Name returns the language name (e.g., "go", "rust", "python")
	Name() string

	// Extensions returns file extensions this parser handles (e.g., [".go"], [".rs"])
	Extensions() []string

	// CanParse returns true if this parser can handle the given file path
	CanParse(path string) bool

	// ParseFile parses a source file and returns code elements
	ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error)

	// IsTestFile returns true if the file is a test file
	IsTestFile(path string) bool
}

// ParserConfig holds configuration options for language parsers
type ParserConfig struct {
	ExtractSymbols bool     // Whether to extract detailed symbols
	IncludeTests   bool     // Whether to include test files
	ExcludeDirs    []string // Directories to exclude
}

// registry holds all registered language parsers
var registry = make(map[string]LanguageParser)

// Register adds a language parser to the registry
func Register(parser LanguageParser) {
	registry[parser.Name()] = parser
}

// GetParser returns a parser by language name
func GetParser(name string) (LanguageParser, bool) {
	p, ok := registry[name]
	return p, ok
}

// GetParserForFile returns the appropriate parser for a file based on extension
func GetParserForFile(path string) (LanguageParser, bool) {
	for _, parser := range registry {
		if parser.CanParse(path) {
			return parser, true
		}
	}
	return nil, false
}

// AllParsers returns all registered parsers
func AllParsers() []LanguageParser {
	parsers := make([]LanguageParser, 0, len(registry))
	for _, p := range registry {
		parsers = append(parsers, p)
	}
	return parsers
}

// SupportedExtensions returns all file extensions supported by registered parsers
func SupportedExtensions() []string {
	var extensions []string
	for _, parser := range registry {
		extensions = append(extensions, parser.Extensions()...)
	}
	return extensions
}
