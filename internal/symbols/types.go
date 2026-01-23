// Package symbols provides AST-based symbol extraction for code files.
// It uses tree-sitter for multi-language parsing to extract constants,
// functions, classes, interfaces, and other code symbols with their values.
package symbols

import "time"

// SymbolType represents the kind of code symbol extracted.
type SymbolType string

const (
	SymbolTypeConst     SymbolType = "const"
	SymbolTypeVar       SymbolType = "var"
	SymbolTypeFunction  SymbolType = "function"
	SymbolTypeMethod    SymbolType = "method"
	SymbolTypeClass     SymbolType = "class"
	SymbolTypeInterface SymbolType = "interface"
	SymbolTypeType      SymbolType = "type"
	SymbolTypeEnum      SymbolType = "enum"
	SymbolTypeEnumValue SymbolType = "enum_value"
	SymbolTypeStruct    SymbolType = "struct"
	SymbolTypeTrait     SymbolType = "trait"
	SymbolTypeProperty  SymbolType = "property"
)

// Language represents a programming language for parsing.
type Language string

const (
	LangTypeScript Language = "typescript"
	LangJavaScript Language = "javascript"
	LangGo         Language = "go"
	LangPython     Language = "python"
	LangRust       Language = "rust"
	LangUnknown    Language = "unknown"
)

// Symbol represents an extracted code symbol with its metadata.
type Symbol struct {
	// Name is the symbol identifier (e.g., "DEFAULT_FLUSH_INTERVAL").
	Name string `json:"name"`

	// Type is the kind of symbol (const, function, class, etc.).
	Type SymbolType `json:"type"`

	// Value is the literal value for constants, empty for functions/classes.
	// For numeric constants, this is the evaluated value (e.g., "60000" not "60 * 1000").
	Value string `json:"value,omitempty"`

	// RawValue is the original source text of the value (e.g., "60 * 1000").
	RawValue string `json:"raw_value,omitempty"`

	// FilePath is the relative path from the repository root.
	FilePath string `json:"file_path"`

	// LineNumber is the 1-indexed start line of the symbol definition.
	LineNumber int `json:"line_number"`

	// EndLine is the 1-indexed end line of the symbol definition.
	EndLine int `json:"end_line"`

	// Column is the 0-indexed start column of the symbol name.
	Column int `json:"column,omitempty"`

	// Exported indicates whether the symbol is public/exported.
	Exported bool `json:"exported"`

	// DocComment is the documentation comment above the symbol (JSDoc, GoDoc, etc.).
	DocComment string `json:"doc_comment,omitempty"`

	// Signature is the function signature for functions/methods.
	// Example: "(ctx context.Context, id string) (error)"
	Signature string `json:"signature,omitempty"`

	// Parent is the name of the containing class/struct for members.
	Parent string `json:"parent,omitempty"`

	// Snippet is the source code of the definition with context (2 lines above/below).
	Snippet string `json:"snippet,omitempty"`

	// Language is the programming language of the source file.
	Language Language `json:"language"`

	// TypeAnnotation is the explicit type annotation if present.
	// Example: "number", "string", "StorageScope"
	TypeAnnotation string `json:"type_annotation,omitempty"`
}

// FileSymbols represents all symbols extracted from a single file.
type FileSymbols struct {
	// FilePath is the relative path of the source file.
	FilePath string `json:"file_path"`

	// Language is the detected programming language.
	Language Language `json:"language"`

	// Symbols is the list of extracted symbols.
	Symbols []Symbol `json:"symbols"`

	// ParseErrors contains any non-fatal parsing errors encountered.
	ParseErrors []string `json:"parse_errors,omitempty"`

	// ParsedAt is when the file was parsed.
	ParsedAt time.Time `json:"parsed_at"`
}

// ParserConfig holds configuration for symbol extraction.
type ParserConfig struct {
	// Languages is the list of languages to parse.
	// If empty, all supported languages are enabled.
	Languages []Language

	// MaxSymbolsPerFile limits symbols extracted per file to prevent bloat.
	// Default: 500
	MaxSymbolsPerFile int

	// MinNameLength skips symbols with names shorter than this.
	// Default: 2 (skip single-char like 'i', 'x')
	MinNameLength int

	// IncludePrivate includes non-exported/private symbols.
	// Default: false (only exported symbols)
	IncludePrivate bool

	// IncludeDocComments extracts documentation comments.
	// Default: true
	IncludeDocComments bool

	// ContextLines is the number of lines above/below to include in Snippet.
	// Default: 2
	ContextLines int

	// EvaluateConstants attempts to evaluate constant expressions.
	// Example: "60 * 1000" -> "60000"
	// Default: true
	EvaluateConstants bool
}

// DefaultParserConfig returns the default parser configuration.
func DefaultParserConfig() ParserConfig {
	return ParserConfig{
		Languages:          nil, // all supported
		MaxSymbolsPerFile:  500,
		MinNameLength:      2,
		IncludePrivate:     false,
		IncludeDocComments: true,
		ContextLines:       2,
		EvaluateConstants:  true,
	}
}

// LanguageFromExtension returns the Language for a file extension.
func LanguageFromExtension(ext string) Language {
	switch ext {
	case ".ts", ".tsx", ".mts", ".cts":
		return LangTypeScript
	case ".js", ".jsx", ".mjs", ".cjs":
		return LangJavaScript
	case ".go":
		return LangGo
	case ".py", ".pyi":
		return LangPython
	case ".rs":
		return LangRust
	default:
		return LangUnknown
	}
}

// SupportedLanguages returns all languages supported by the parser.
func SupportedLanguages() []Language {
	return []Language{
		LangTypeScript,
		LangJavaScript,
		LangGo,
		LangPython,
		LangRust,
	}
}

// IsSupported checks if a language is supported for parsing.
func (l Language) IsSupported() bool {
	switch l {
	case LangTypeScript, LangJavaScript, LangGo, LangPython, LangRust:
		return true
	default:
		return false
	}
}
