package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&TypeScriptParser{})
}

// TypeScriptParser implements LanguageParser for TypeScript/JavaScript files
type TypeScriptParser struct{}

func (p *TypeScriptParser) Name() string {
	return "typescript"
}

func (p *TypeScriptParser) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx"}
}

func (p *TypeScriptParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".ts") ||
		strings.HasSuffix(pathLower, ".tsx") ||
		strings.HasSuffix(pathLower, ".js") ||
		strings.HasSuffix(pathLower, ".jsx")
}

func (p *TypeScriptParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".test.ts") ||
		strings.HasSuffix(pathLower, ".test.tsx") ||
		strings.HasSuffix(pathLower, ".test.js") ||
		strings.HasSuffix(pathLower, ".test.jsx") ||
		strings.HasSuffix(pathLower, ".spec.ts") ||
		strings.HasSuffix(pathLower, ".spec.tsx") ||
		strings.HasSuffix(pathLower, ".spec.js") ||
		strings.HasSuffix(pathLower, ".spec.jsx") ||
		strings.Contains(path, "/__tests__/")
}

func (p *TypeScriptParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract module name
	moduleName := strings.TrimSuffix(fileName, filepath.Ext(fileName))

	// Find classes, interfaces, functions
	classes := FindAllMatches(content, `(?:export\s+)?class\s+(\w+)`)
	interfaces := FindAllMatches(content, `(?:export\s+)?interface\s+(\w+)`)
	functions := FindAllMatches(content, `(?:export\s+)?(?:async\s+)?function\s+(\w+)`)
	arrowFuncs := FindAllMatches(content, `(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\([^)]*\)\s*(?::\s*[^=]+)?\s*=>`)
	imports := FindAllMatches(content, `import\s+.*?\s+from\s+['"]([^'"]+)['"]`)

	// Check for NestJS decorators
	decorators := FindAllMatches(content, `@(\w+)\(`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("TypeScript/JavaScript module: %s\n", moduleName))
	contentBuilder.WriteString(fmt.Sprintf("File: %s\n", relPath))

	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(interfaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Interfaces: %s\n", strings.Join(uniqueStrings(interfaces), ", ")))
	}

	allFuncs := append(uniqueStrings(functions), uniqueStrings(arrowFuncs)...)
	if len(allFuncs) > 0 {
		if len(allFuncs) > 15 {
			allFuncs = allFuncs[:15]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(allFuncs, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(allFuncs, ", ")))
		}
	}

	if len(decorators) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Decorators: %s\n", strings.Join(uniqueStrings(decorators), ", ")))
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(imports)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"typescript", "module"}
	tags = append(tags, concerns...)

	// Add framework-specific tags
	if containsAny(decorators, []string{"Controller", "Injectable", "Module", "Guard"}) {
		tags = append(tags, "nestjs")
	}
	if containsAny(content, []string{"React", "useState", "useEffect"}) {
		tags = append(tags, "react")
	}
	if containsAny(content, []string{"@angular", "NgModule"}) {
		tags = append(tags, "angular")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     moduleName,
		Kind:     "module",
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
			Content:  fmt.Sprintf("TypeScript class '%s' in module %s", cls, moduleName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"typescript", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add interfaces as separate elements
	for _, iface := range uniqueStrings(interfaces) {
		elements = append(elements, CodeElement{
			Name:     iface,
			Kind:     "interface",
			Path:     fmt.Sprintf("/%s#%s", relPath, iface),
			Content:  fmt.Sprintf("TypeScript interface '%s' in module %s", iface, moduleName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"typescript", "interface"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *TypeScriptParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Pattern: const NAME = value (exported constants)
	constPattern := regexp.MustCompile(`^(?:export\s+)?const\s+([A-Z][A-Z0-9_]*)\s*(?::\s*\w+)?\s*=\s*(.+)$`)
	// Pattern: export function name(args): Type
	fnPattern := regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(([^)]*)\)(?:\s*:\s*([^\{]+))?`)
	// Pattern: export const name = (args): Type =>
	arrowPattern := regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\(([^)]*)\)(?:\s*:\s*([^=]+))?\s*=>`)

	for i, line := range lines {
		lineNum := i + 1
		trimmedLine := strings.TrimSpace(line)

		// Extract constants
		if matches := constPattern.FindStringSubmatch(trimmedLine); matches != nil {
			sym := Symbol{
				Name:       matches[1],
				Type:       "constant",
				Value:      CleanValue(matches[2]),
				RawValue:   matches[2],
				LineNumber: lineNum,
				Exported:   strings.HasPrefix(trimmedLine, "export"),
				Language:   "typescript",
			}
			symbols = append(symbols, sym)
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
				Signature:      fmt.Sprintf("function %s(%s)%s", matches[1], matches[2], formatTSReturn(returnType)),
				TypeAnnotation: returnType,
				LineNumber:     lineNum,
				Exported:       strings.HasPrefix(trimmedLine, "export"),
				Language:       "typescript",
			}
			symbols = append(symbols, sym)
		}

		// Extract arrow functions
		if matches := arrowPattern.FindStringSubmatch(trimmedLine); matches != nil {
			returnType := ""
			if len(matches) > 3 && matches[3] != "" {
				returnType = strings.TrimSpace(matches[3])
			}

			sym := Symbol{
				Name:           matches[1],
				Type:           "function",
				Signature:      fmt.Sprintf("const %s = (%s)%s =>", matches[1], matches[2], formatTSReturn(returnType)),
				TypeAnnotation: returnType,
				LineNumber:     lineNum,
				Exported:       strings.HasPrefix(trimmedLine, "export"),
				Language:       "typescript",
			}
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// containsAny checks if any of the patterns appear in the string
func containsAny(s interface{}, patterns []string) bool {
	var str string
	switch v := s.(type) {
	case string:
		str = v
	case []string:
		str = strings.Join(v, " ")
	default:
		return false
	}
	for _, pattern := range patterns {
		if strings.Contains(str, pattern) {
			return true
		}
	}
	return false
}

// formatTSReturn formats a return type for display
func formatTSReturn(returnType string) string {
	if returnType == "" {
		return ""
	}
	return ": " + returnType
}
