package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&RustParser{})
}

// RustParser implements LanguageParser for Rust source files
type RustParser struct{}

func (p *RustParser) Name() string {
	return "rust"
}

func (p *RustParser) Extensions() []string {
	return []string{".rs"}
}

func (p *RustParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".rs")
}

func (p *RustParser) IsTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.rs") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/test/")
}

func (p *RustParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract module name from path (crate/module structure)
	moduleName := strings.TrimSuffix(fileName, ".rs")
	if moduleName == "mod" || moduleName == "lib" || moduleName == "main" {
		dir := filepath.Dir(relPath)
		if dir != "." {
			parts := strings.Split(dir, string(filepath.Separator))
			if len(parts) > 0 {
				moduleName = parts[len(parts)-1]
			}
		}
	}

	// Find structs, enums, traits, and functions
	structs := FindAllMatches(content, `(?:pub\s+)?struct\s+(\w+)`)
	enums := FindAllMatches(content, `(?:pub\s+)?enum\s+(\w+)`)
	traits := FindAllMatches(content, `(?:pub\s+)?trait\s+(\w+)`)
	functions := FindAllMatches(content, `(?:pub\s+)?(?:async\s+)?fn\s+(\w+)`)
	macros := FindAllMatches(content, `macro_rules!\s+(\w+)`)
	uses := FindAllMatches(content, `^\s*use\s+([\w:]+)`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Rust file: %s\n", fileName))
	contentBuilder.WriteString(fmt.Sprintf("Module: %s\n", moduleName))

	if len(structs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Structs: %s\n", strings.Join(uniqueStrings(structs), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
	}
	if len(traits) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Traits: %s\n", strings.Join(uniqueStrings(traits), ", ")))
	}
	if len(functions) > 0 {
		fnList := uniqueStrings(functions)
		if len(fnList) > 15 {
			fnList = fnList[:15]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(fnList, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(fnList, ", ")))
		}
	}
	if len(macros) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Macros: %s\n", strings.Join(uniqueStrings(macros), ", ")))
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(uses)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"rust", "module"}
	tags = append(tags, concerns...)

	// Determine file kind
	rustKind := "rust-module"
	if fileName == "lib.rs" {
		rustKind = "rust-lib"
		tags = append(tags, "library")
	} else if fileName == "main.rs" {
		rustKind = "rust-main"
		tags = append(tags, "executable")
	} else if fileName == "mod.rs" {
		rustKind = "rust-mod"
		tags = append(tags, "submodule")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     rustKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  moduleName,
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
			Content:  fmt.Sprintf("Rust struct '%s' in module %s (file: %s)", st, moduleName, fileName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"rust", "struct"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add traits as separate elements
	for _, tr := range uniqueStrings(traits) {
		elements = append(elements, CodeElement{
			Name:     tr,
			Kind:     "trait",
			Path:     fmt.Sprintf("/%s#%s", relPath, tr),
			Content:  fmt.Sprintf("Rust trait '%s' in module %s (file: %s)", tr, moduleName, fileName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"rust", "trait"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *RustParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Pattern: const NAME: Type = value;
	constPattern := regexp.MustCompile(`^\s*(?:pub\s+)?const\s+([A-Z][A-Z0-9_]*)\s*:\s*([^=]+)\s*=\s*(.+?);`)
	// Pattern: pub fn name(...) -> Type
	fnPattern := regexp.MustCompile(`^\s*(?:pub\s+)?(?:async\s+)?fn\s+(\w+)\s*(?:<[^>]*>)?\s*\(([^)]*)\)(?:\s*->\s*([^\{]+))?`)

	for i, line := range lines {
		lineNum := i + 1

		// Extract constants
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			sym := Symbol{
				Name:           matches[1],
				Type:           "constant",
				TypeAnnotation: strings.TrimSpace(matches[2]),
				Value:          CleanValue(matches[3]),
				RawValue:       matches[3],
				Line:     lineNum,
				Exported:       strings.HasPrefix(strings.TrimSpace(line), "pub"),
				Language:       "rust",
			}
			symbols = append(symbols, sym)
		}

		// Extract function signatures
		if matches := fnPattern.FindStringSubmatch(line); matches != nil {
			returnType := ""
			if len(matches) > 3 && matches[3] != "" {
				returnType = strings.TrimSpace(matches[3])
			}

			sym := Symbol{
				Name:           matches[1],
				Type:           "function",
				Signature:      fmt.Sprintf("fn %s(%s)%s", matches[1], matches[2], formatReturn(returnType)),
				TypeAnnotation: returnType,
				Line:     lineNum,
				Exported:       strings.HasPrefix(strings.TrimSpace(line), "pub"),
				Language:       "rust",
			}
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// formatReturn formats a return type for display
func formatReturn(returnType string) string {
	if returnType == "" {
		return ""
	}
	return " -> " + returnType
}
