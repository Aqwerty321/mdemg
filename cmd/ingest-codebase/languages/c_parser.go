package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&CParser{})
}

// CParser implements LanguageParser for C source files
type CParser struct{}

func (p *CParser) Name() string {
	return "c"
}

func (p *CParser) Extensions() []string {
	return []string{".c", ".h"}
}

func (p *CParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	if strings.HasSuffix(pathLower, ".c") {
		return true
	}
	// For .h files, only claim if it's NOT C++ (no classes/namespaces)
	if strings.HasSuffix(pathLower, ".h") {
		return !p.isCppHeader(path)
	}
	return false
}

// isCppHeader detects if a .h file contains C++ code
func (p *CParser) isCppHeader(path string) bool {
	content, err := ReadFileContent(path)
	if err != nil {
		return false
	}
	return strings.Contains(content, "class ") ||
		strings.Contains(content, "namespace ") ||
		strings.Contains(content, "template<") ||
		strings.Contains(content, "template <") ||
		strings.Contains(content, "public:") ||
		strings.Contains(content, "private:")
}

func (p *CParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "_test.") ||
		strings.Contains(pathLower, "/test/") ||
		strings.Contains(pathLower, "/tests/")
}

func (p *CParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	isHeader := strings.HasSuffix(path, ".h")

	// Extract structs and typedefs
	structs := FindAllMatches(content, `struct\s+(\w+)\s*\{`)
	typedefs := FindAllMatches(content, `typedef\s+(?:struct\s+)?[\w\s*]+\s+(\w+)\s*;`)
	enums := FindAllMatches(content, `enum\s+(\w+)\s*\{`)

	// Extract functions
	functions := FindAllMatches(content, `(?:static\s+)?(?:inline\s+)?[\w*]+\s+(\w+)\s*\([^)]*\)\s*[;{]`)

	// Extract includes
	includes := FindAllMatches(content, `#include\s*[<"]([^>"]+)[>"]`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("C file: %s\n", fileName))

	if isHeader {
		contentBuilder.WriteString("Type: Header\n")
	} else {
		contentBuilder.WriteString("Type: Source\n")
	}

	if len(structs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Structs: %s\n", strings.Join(uniqueStrings(structs), ", ")))
	}
	if len(typedefs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Typedefs: %s\n", strings.Join(uniqueStrings(typedefs), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
	}
	if len(functions) > 0 {
		fnList := uniqueStrings(functions)
		fnList = filterCFunctions(fnList)
		if len(fnList) > 15 {
			fnList = fnList[:15]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(fnList, ", ")))
		} else if len(fnList) > 0 {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(fnList, ", ")))
		}
	}
	contentBuilder.WriteString(fmt.Sprintf("Includes: %d\n", len(includes)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"c", "module"}
	if isHeader {
		tags = append(tags, "header")
	} else {
		tags = append(tags, "source")
	}
	tags = append(tags, concerns...)

	// Determine file kind
	cKind := "c-source"
	if isHeader {
		cKind = "c-header"
	}

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     cKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  "",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add structs as separate elements
	for _, st := range uniqueStrings(structs) {
		elements = append(elements, CodeElement{
			Name:     st,
			Kind:     "struct",
			Path:     fmt.Sprintf("/%s#%s", relPath, st),
			Content:  fmt.Sprintf("C struct '%s' in file %s", st, fileName),
			Package:  "",
			FilePath: relPath,
			Tags:     append([]string{"c", "struct"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *CParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Pattern: #define NAME value
	definePattern := regexp.MustCompile(`^\s*#define\s+([A-Z][A-Z0-9_]*)\s+(.+)$`)
	// Pattern: const TYPE NAME = value;
	constPattern := regexp.MustCompile(`^\s*(?:static\s+)?const\s+[\w*]+\s+([A-Z][A-Z0-9_]*)\s*=\s*(.+?);`)
	// Pattern: enum { VALUE, ... }
	enumValuePattern := regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*(?:=\s*[^,]+)?\s*[,}]`)

	inEnum := false

	for i, line := range lines {
		lineNum := i + 1

		// Check for enum block
		if strings.Contains(line, "enum") && strings.Contains(line, "{") {
			inEnum = true
			continue
		}
		if inEnum && strings.Contains(line, "}") {
			inEnum = false
			continue
		}

		// Extract enum values
		if inEnum {
			if matches := enumValuePattern.FindStringSubmatch(line); matches != nil {
				symbols = append(symbols, Symbol{
					Name:       matches[1],
					Type:       "enum_value",
					Line: lineNum,
					Exported:   true,
					Language:   "c",
				})
			}
			continue
		}

		// Check for #define
		if matches := definePattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "macro",
				Value:      CleanValue(matches[2]),
				RawValue:   matches[2],
				Line: lineNum,
				Exported:   true,
				Language:   "c",
			})
			continue
		}

		// Check for const
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "constant",
				Value:      CleanValue(matches[2]),
				RawValue:   matches[2],
				Line: lineNum,
				Exported:   !strings.Contains(line, "static"),
				Language:   "c",
			})
		}
	}

	return symbols
}

// filterCFunctions removes common false positives from function names
func filterCFunctions(funcs []string) []string {
	skip := map[string]bool{
		"if": true, "for": true, "while": true, "switch": true,
		"return": true, "sizeof": true, "typeof": true,
	}
	var result []string
	for _, f := range funcs {
		if !skip[f] {
			result = append(result, f)
		}
	}
	return result
}
