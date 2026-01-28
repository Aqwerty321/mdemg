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
	// Use (?m)^ to match at line start, require colon or parenthesis after name
	// This avoids matching "class that" in docstrings like "A dummy class that..."
	classes := FindAllMatches(content, `(?m)^\s*class\s+(\w+)\s*[:\(]`)
	functions := FindAllMatches(content, `(?m)^\s*def\s+(\w+)\s*\(`)
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

	// Add classes as separate elements with enhanced content for better retrieval
	for _, cls := range uniqueStrings(classes) {
		// Try to extract class definition and docstring for better embedding
		classContent := extractClassContent(content, cls, moduleName)
		elements = append(elements, CodeElement{
			Name:     cls,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, cls),
			Content:  classContent,
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

// extractClassContent extracts a class definition with docstring for better embedding.
// Returns enhanced content that includes class signature, docstring, and key attributes.
func extractClassContent(content, className, moduleName string) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Python class %s in module %s\n", className, moduleName))

	// Find class definition
	classPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^class\s+%s\s*(?:\(([^)]*)\))?\s*:`, regexp.QuoteMeta(className)))
	match := classPattern.FindStringSubmatchIndex(content)
	if match == nil {
		return result.String()
	}

	// Extract base classes if present
	if match[2] != -1 && match[3] != -1 {
		bases := content[match[2]:match[3]]
		if bases != "" {
			result.WriteString(fmt.Sprintf("Inherits from: %s\n", bases))
		}
	}

	// Find class body start
	classStart := match[0]
	lines := strings.Split(content[classStart:], "\n")

	// Extract docstring if present (first string after class:)
	docLines := []string{}
	inDocstring := false
	docstringDelim := ""
	for i := 1; i < len(lines) && i < 30; i++ {
		line := strings.TrimSpace(lines[i])
		if !inDocstring {
			if strings.HasPrefix(line, `"""`) || strings.HasPrefix(line, `'''`) {
				inDocstring = true
				docstringDelim = line[:3]
				docLine := strings.TrimPrefix(line, docstringDelim)
				if strings.HasSuffix(docLine, docstringDelim) {
					// Single-line docstring
					docLines = append(docLines, strings.TrimSuffix(docLine, docstringDelim))
					break
				}
				docLines = append(docLines, docLine)
			} else if line != "" && !strings.HasPrefix(line, "#") {
				// Not a docstring, move on
				break
			}
		} else {
			if strings.HasSuffix(line, docstringDelim) {
				docLines = append(docLines, strings.TrimSuffix(line, docstringDelim))
				break
			}
			docLines = append(docLines, line)
		}
	}

	if len(docLines) > 0 {
		docstring := strings.Join(docLines, " ")
		if len(docstring) > 500 {
			docstring = docstring[:500] + "..."
		}
		result.WriteString(fmt.Sprintf("Description: %s\n", docstring))
	}

	// Extract class attributes (lines with : type annotation)
	attrPattern := regexp.MustCompile(`^\s{4}(\w+)\s*:\s*(.+?)(?:\s*=.*)?$`)
	attrs := []string{}
	for i := 1; i < len(lines) && i < 50 && len(attrs) < 10; i++ {
		if attrMatch := attrPattern.FindStringSubmatch(lines[i]); attrMatch != nil {
			attrs = append(attrs, fmt.Sprintf("%s: %s", attrMatch[1], attrMatch[2]))
		}
	}
	if len(attrs) > 0 {
		result.WriteString(fmt.Sprintf("Key attributes: %s\n", strings.Join(attrs, ", ")))
	}

	return result.String()
}
