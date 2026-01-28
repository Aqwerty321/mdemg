package languages

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&GoParser{})
}

// GoParser implements LanguageParser for Go source files
type GoParser struct {
	fset *token.FileSet
}

func (p *GoParser) Name() string {
	return "go"
}

func (p *GoParser) Extensions() []string {
	return []string{".go"}
}

func (p *GoParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".go")
}

func (p *GoParser) IsTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go") ||
		strings.Contains(path, "/testdata/")
}

func (p *GoParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	if p.fset == nil {
		p.fset = token.NewFileSet()
	}

	file, err := parser.ParseFile(p.fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	pkgName := file.Name.Name

	// Read file content for concern detection
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	concerns := DetectConcerns(relPath, content)
	tags := []string{"go", "package", pkgName}
	tags = append(tags, concerns...)

	// Check if this is a configuration file
	kind := "package"
	if IsConfigFile(relPath) {
		tags = append(tags, "config")
		kind = "config"
	}

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Go package %s in file %s\n", pkgName, relPath))
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add package-level element
	elements = append(elements, CodeElement{
		Name:     pkgName,
		Kind:     kind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  pkgName,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Extract declarations
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if elem := p.extractFunction(d, pkgName, relPath); elem != nil {
				elem.Tags = append(elem.Tags, concerns...)
				elem.Concerns = concerns
				elements = append(elements, *elem)
			}
		case *ast.GenDecl:
			for _, elem := range p.extractGenDecl(d, pkgName, relPath) {
				elem.Tags = append(elem.Tags, concerns...)
				elem.Concerns = concerns
				elements = append(elements, elem)
			}
		}
	}

	return elements, nil
}

func (p *GoParser) extractFunction(decl *ast.FuncDecl, pkgName, relPath string) *CodeElement {
	funcName := decl.Name.Name

	// Skip unexported functions
	if funcName[0] < 'A' || funcName[0] > 'Z' {
		return nil
	}

	// Build description from doc comment
	var doc string
	if decl.Doc != nil {
		doc = decl.Doc.Text()
	}

	// Build tags
	tags := []string{"go", "function", pkgName}

	// Check if it's a method
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		tags = append(tags, "method")
	}

	content := fmt.Sprintf("Go function %s.%s\n%s", pkgName, funcName, doc)

	return &CodeElement{
		Name:     funcName,
		Kind:     "function",
		Path:     "/" + relPath + "#" + funcName,
		Content:  content,
		Package:  pkgName,
		FilePath: relPath,
		Tags:     tags,
	}
}

func (p *GoParser) extractGenDecl(decl *ast.GenDecl, pkgName, relPath string) []CodeElement {
	var elements []CodeElement

	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			// Skip unexported types
			if s.Name.Name[0] < 'A' || s.Name.Name[0] > 'Z' {
				continue
			}

			kind := "type"
			tags := []string{"go", pkgName}

			switch s.Type.(type) {
			case *ast.StructType:
				kind = "struct"
				tags = append(tags, "struct")
			case *ast.InterfaceType:
				kind = "interface"
				tags = append(tags, "interface")
			}

			var doc string
			if decl.Doc != nil {
				doc = decl.Doc.Text()
			}

			content := fmt.Sprintf("Go %s %s.%s\n%s", kind, pkgName, s.Name.Name, doc)

			elements = append(elements, CodeElement{
				Name:     s.Name.Name,
				Kind:     kind,
				Path:     "/" + relPath + "#" + s.Name.Name,
				Content:  content,
				Package:  pkgName,
				FilePath: relPath,
				Tags:     tags,
			})
		}
	}

	return elements
}

func (p *GoParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Pattern: const NAME = value or const NAME Type = value
	constPattern := regexp.MustCompile(`^\s*const\s+(\w+)\s*(?:\w+)?\s*=\s*(.+)$`)
	// Pattern: const ( block start
	constBlockStart := regexp.MustCompile(`^\s*const\s*\(\s*$`)
	// Pattern: NAME = value inside const block
	constBlockItem := regexp.MustCompile(`^\s*(\w+)\s*(?:\w+)?\s*=\s*(.+)$`)

	inConstBlock := false

	for i, line := range lines {
		lineNum := i + 1

		// Check for const block start
		if constBlockStart.MatchString(line) {
			inConstBlock = true
			continue
		}

		// Check for const block end
		if inConstBlock && strings.TrimSpace(line) == ")" {
			inConstBlock = false
			continue
		}

		// Extract const from block
		if inConstBlock {
			if matches := constBlockItem.FindStringSubmatch(line); matches != nil {
				// Only extract exported constants (capitalized names)
				if matches[1][0] >= 'A' && matches[1][0] <= 'Z' {
					sym := Symbol{
						Name:       matches[1],
						Type:       "constant",
						Value:      CleanValue(matches[2]),
						RawValue:   matches[2],
						LineNumber: lineNum,
						Exported:   true,
						Language:   "go",
					}
					symbols = append(symbols, sym)
				}
			}
			continue
		}

		// Single const declaration
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			exported := matches[1][0] >= 'A' && matches[1][0] <= 'Z'
			sym := Symbol{
				Name:       matches[1],
				Type:       "constant",
				Value:      CleanValue(matches[2]),
				RawValue:   matches[2],
				LineNumber: lineNum,
				Exported:   exported,
				Language:   "go",
			}
			symbols = append(symbols, sym)
		}
	}

	return symbols
}
