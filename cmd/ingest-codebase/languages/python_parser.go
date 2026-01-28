package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&PythonParser{})
}

// PythonParser implements LanguageParser for Python source files
type PythonParser struct{}

func (p *PythonParser) Name() string {
	return "python"
}

func (p *PythonParser) Extensions() []string {
	return []string{".py"}
}

func (p *PythonParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".py")
}

func (p *PythonParser) IsTestFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasPrefix(name, "test_") ||
		strings.HasSuffix(name, "_test.py") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/test/")
}

func (p *PythonParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract module name
	moduleName := strings.TrimSuffix(fileName, ".py")

	// Find classes, functions, and imports
	classes := FindAllMatches(content, `class\s+(\w+)`)
	functions := FindAllMatches(content, `def\s+(\w+)\s*\(`)
	imports := FindAllMatches(content, `^(?:from\s+[\w.]+\s+)?import\s+([\w.]+)`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Python module: %s\n", moduleName))
	contentBuilder.WriteString(fmt.Sprintf("File: %s\n", relPath))

	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(functions) > 0 {
		fnList := uniqueStrings(functions)
		// Filter out dunder methods for summary
		publicFuncs := filterDunderMethods(fnList)
		if len(publicFuncs) > 0 {
			if len(publicFuncs) > 15 {
				publicFuncs = publicFuncs[:15]
				contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(publicFuncs, ", ")))
			} else {
				contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(publicFuncs, ", ")))
			}
		}
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(imports)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"python", "module"}
	tags = append(tags, concerns...)

	// Determine file kind
	pyKind := "module"
	if fileName == "__init__.py" {
		pyKind = "package"
		tags = append(tags, "package")
	} else if fileName == "__main__.py" {
		pyKind = "entrypoint"
		tags = append(tags, "entrypoint")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     moduleName,
		Kind:     pyKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  moduleName,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add classes as separate elements
	for _, cls := range uniqueStrings(classes) {
		elements = append(elements, CodeElement{
			Name:     cls,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, cls),
			Content:  fmt.Sprintf("Python class '%s' in module %s", cls, moduleName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"python", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *PythonParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Pattern: CONSTANT = value (uppercase name at module level)
	constPattern := regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*=\s*(.+)$`)
	// Pattern: def function_name(args) -> Type:
	fnPattern := regexp.MustCompile(`^def\s+(\w+)\s*\(([^)]*)\)(?:\s*->\s*([^:]+))?:`)
	// Pattern: class ClassName(base):
	classPattern := regexp.MustCompile(`^class\s+(\w+)(?:\s*\(([^)]*)\))?:`)

	for i, line := range lines {
		lineNum := i + 1
		trimmedLine := strings.TrimSpace(line)

		// Extract module-level constants (must not be indented)
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if matches := constPattern.FindStringSubmatch(trimmedLine); matches != nil {
				// Skip if it looks like a class attribute
				if strings.Contains(matches[2], "class") || strings.Contains(matches[2], "def") {
					continue
				}
				sym := Symbol{
					Name:       matches[1],
					Type:       "constant",
					Value:      CleanValue(matches[2]),
					RawValue:   matches[2],
					LineNumber: lineNum,
					Exported:   !strings.HasPrefix(matches[1], "_"),
					Language:   "python",
				}
				symbols = append(symbols, sym)
			}
		}

		// Extract function definitions
		if matches := fnPattern.FindStringSubmatch(trimmedLine); matches != nil {
			returnType := ""
			if len(matches) > 3 && matches[3] != "" {
				returnType = strings.TrimSpace(matches[3])
			}

			sym := Symbol{
				Name:           matches[1],
				Type:           "function",
				Signature:      fmt.Sprintf("def %s(%s)%s", matches[1], matches[2], formatPythonReturn(returnType)),
				TypeAnnotation: returnType,
				LineNumber:     lineNum,
				Exported:       !strings.HasPrefix(matches[1], "_"),
				Language:       "python",
			}
			symbols = append(symbols, sym)
		}

		// Extract class definitions
		if matches := classPattern.FindStringSubmatch(trimmedLine); matches != nil {
			baseClasses := ""
			if len(matches) > 2 && matches[2] != "" {
				baseClasses = matches[2]
			}

			sym := Symbol{
				Name:       matches[1],
				Type:       "class",
				Parent:     baseClasses,
				LineNumber: lineNum,
				Exported:   !strings.HasPrefix(matches[1], "_"),
				Language:   "python",
			}
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// filterDunderMethods removes __dunder__ methods from a list
func filterDunderMethods(funcs []string) []string {
	var result []string
	for _, f := range funcs {
		if !strings.HasPrefix(f, "__") {
			result = append(result, f)
		}
	}
	return result
}

// formatPythonReturn formats a return type for display
func formatPythonReturn(returnType string) string {
	if returnType == "" {
		return ""
	}
	return " -> " + returnType
}
