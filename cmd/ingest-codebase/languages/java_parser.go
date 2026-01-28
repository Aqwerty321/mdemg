package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&JavaParser{})
}

// JavaParser implements LanguageParser for Java source files
type JavaParser struct{}

func (p *JavaParser) Name() string {
	return "java"
}

func (p *JavaParser) Extensions() []string {
	return []string{".java"}
}

func (p *JavaParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".java")
}

func (p *JavaParser) IsTestFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, "Test.java") ||
		strings.HasSuffix(name, "Tests.java") ||
		strings.HasSuffix(name, "IT.java") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/")
}

func (p *JavaParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract package name
	packageName := ""
	packageMatch := regexp.MustCompile(`^\s*package\s+([\w.]+)\s*;`).FindStringSubmatch(content)
	if packageMatch != nil {
		packageName = packageMatch[1]
	}

	// Find classes and interfaces
	classes := FindAllMatches(content, `(?:public\s+)?(?:abstract\s+)?(?:final\s+)?class\s+(\w+)`)
	interfaces := FindAllMatches(content, `(?:public\s+)?interface\s+(\w+)`)
	enums := FindAllMatches(content, `(?:public\s+)?enum\s+(\w+)`)

	// Find methods
	methods := FindAllMatches(content, `(?:public|private|protected)?\s*(?:static\s+)?(?:final\s+)?(?:synchronized\s+)?(?:\w+(?:<[^>]+>)?)\s+(\w+)\s*\([^)]*\)\s*(?:throws\s+[\w,\s]+)?\s*\{`)

	// Find imports
	imports := FindAllMatches(content, `^\s*import\s+([\w.*]+)\s*;`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Java file: %s\n", fileName))

	if packageName != "" {
		contentBuilder.WriteString(fmt.Sprintf("Package: %s\n", packageName))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(interfaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Interfaces: %s\n", strings.Join(uniqueStrings(interfaces), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
	}
	if len(methods) > 0 {
		methodList := uniqueStrings(methods)
		if len(methodList) > 15 {
			methodList = methodList[:15]
			contentBuilder.WriteString(fmt.Sprintf("Methods: %s (and more)\n", strings.Join(methodList, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Methods: %s\n", strings.Join(methodList, ", ")))
		}
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(imports)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"java", "module"}
	tags = append(tags, concerns...)

	// Determine file kind based on content
	javaKind := "java-class"
	if len(interfaces) > 0 && len(classes) == 0 {
		javaKind = "java-interface"
		tags = append(tags, "interface")
	} else if len(enums) > 0 && len(classes) == 0 {
		javaKind = "java-enum"
		tags = append(tags, "enum")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     javaKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  packageName,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add classes as separate elements
	for _, class := range uniqueStrings(classes) {
		elements = append(elements, CodeElement{
			Name:     class,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, class),
			Content:  fmt.Sprintf("Java class '%s' in package %s (file: %s)", class, packageName, fileName),
			Package:  packageName,
			FilePath: relPath,
			Tags:     append([]string{"java", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add interfaces as separate elements
	for _, iface := range uniqueStrings(interfaces) {
		elements = append(elements, CodeElement{
			Name:     iface,
			Kind:     "interface",
			Path:     fmt.Sprintf("/%s#%s", relPath, iface),
			Content:  fmt.Sprintf("Java interface '%s' in package %s (file: %s)", iface, packageName, fileName),
			Package:  packageName,
			FilePath: relPath,
			Tags:     append([]string{"java", "interface"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *JavaParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Pattern: public static final TYPE NAME = value;
	constPattern := regexp.MustCompile(`^\s*(?:public|private|protected)?\s*static\s+final\s+(\w+)\s+([A-Z][A-Z0-9_]*)\s*=\s*(.+?);`)
	// Pattern: public static TYPE NAME = value;
	staticFieldPattern := regexp.MustCompile(`^\s*(?:public|private|protected)?\s*static\s+(\w+)\s+([A-Z][A-Z0-9_]*)\s*=`)
	// Pattern: method declarations
	methodPattern := regexp.MustCompile(`^\s*(?:public|private|protected)\s+(?:static\s+)?(?:final\s+)?(?:synchronized\s+)?(\w+(?:<[^>]+>)?)\s+(\w+)\s*\(([^)]*)\)`)
	// Pattern: enum values
	enumValuePattern := regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*(?:\([^)]*\))?\s*[,;]`)

	inEnum := false

	for i, line := range lines {
		lineNum := i + 1

		// Check for enum declaration
		if strings.Contains(line, "enum ") && strings.Contains(line, "{") {
			inEnum = true
			continue
		}

		// Check for enum end
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
					LineNumber: lineNum,
					Exported:   true,
					Language:   "java",
				})
			}
			continue
		}

		// Check for constants (public static final)
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			valueStr := strings.TrimSpace(matches[3])
			if len(valueStr) > 100 {
				valueStr = valueStr[:100] + "..."
			}
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "constant",
				TypeAnnotation: matches[1],
				Value:          valueStr,
				LineNumber:     lineNum,
				Exported:       strings.Contains(line, "public"),
				Language:       "java",
			})
			continue
		}

		// Check for static fields
		if matches := staticFieldPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "field",
				TypeAnnotation: matches[1],
				LineNumber:     lineNum,
				Exported:       strings.Contains(line, "public"),
				Language:       "java",
			})
			continue
		}

		// Check for methods
		if matches := methodPattern.FindStringSubmatch(line); matches != nil {
			returnType := matches[1]
			methodName := matches[2]
			params := matches[3]
			signature := fmt.Sprintf("%s %s(%s)", returnType, methodName, params)
			if len(signature) > 150 {
				signature = signature[:150] + "..."
			}
			symbols = append(symbols, Symbol{
				Name:           methodName,
				Type:           "method",
				Signature:      signature,
				TypeAnnotation: returnType,
				LineNumber:     lineNum,
				Exported:       strings.Contains(line, "public"),
				Language:       "java",
			})
		}
	}

	return symbols
}
