package symbols

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Parser extracts symbols from source code files using tree-sitter.
type Parser struct {
	config    ParserConfig
	tsParser  *sitter.Parser
	languages map[Language]*sitter.Language
}

// NewParser creates a new symbol parser with the given configuration.
func NewParser(config ParserConfig) (*Parser, error) {
	p := &Parser{
		config:    config,
		tsParser:  sitter.NewParser(),
		languages: make(map[Language]*sitter.Language),
	}

	// Initialize language grammars
	p.languages[LangTypeScript] = typescript.GetLanguage()
	p.languages[LangJavaScript] = javascript.GetLanguage()
	p.languages[LangGo] = golang.GetLanguage()
	p.languages[LangPython] = python.GetLanguage()
	// Note: Rust grammar requires separate import if needed

	return p, nil
}

// ParseFile parses a source file and extracts symbols.
func (p *Parser) ParseFile(ctx context.Context, filePath string) (*FileSymbols, error) {
	// Determine language from extension
	ext := filepath.Ext(filePath)
	lang := LanguageFromExtension(ext)

	if lang == LangUnknown {
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}

	// Check if language is enabled
	if len(p.config.Languages) > 0 {
		enabled := false
		for _, l := range p.config.Languages {
			if l == lang {
				enabled = true
				break
			}
		}
		if !enabled {
			return nil, fmt.Errorf("language %s is not enabled", lang)
		}
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return p.ParseContent(ctx, filePath, lang, content)
}

// ParseContent parses source code content and extracts symbols.
func (p *Parser) ParseContent(ctx context.Context, filePath string, lang Language, content []byte) (*FileSymbols, error) {
	result := &FileSymbols{
		FilePath: filePath,
		Language: lang,
		Symbols:  make([]Symbol, 0),
		ParsedAt: time.Now(),
	}

	// Get the tree-sitter language
	tsLang, ok := p.languages[lang]
	if !ok {
		return nil, fmt.Errorf("no grammar loaded for language: %s", lang)
	}

	// Parse the content
	p.tsParser.SetLanguage(tsLang)
	tree, err := p.tsParser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: %w", err)
	}
	defer tree.Close()

	// Extract symbols based on language
	var symbols []Symbol
	switch lang {
	case LangTypeScript, LangJavaScript:
		symbols = p.extractTypeScriptSymbols(tree.RootNode(), content, filePath)
	case LangGo:
		symbols = p.extractGoSymbols(tree.RootNode(), content, filePath)
	case LangPython:
		symbols = p.extractPythonSymbols(tree.RootNode(), content, filePath)
	default:
		return nil, fmt.Errorf("extraction not implemented for: %s", lang)
	}

	// Apply filters
	for _, sym := range symbols {
		// Skip short names
		if len(sym.Name) < p.config.MinNameLength {
			continue
		}

		// Skip private symbols unless configured
		if !p.config.IncludePrivate && !sym.Exported {
			continue
		}

		// Add language
		sym.Language = lang

		result.Symbols = append(result.Symbols, sym)

		// Enforce max symbols limit
		if len(result.Symbols) >= p.config.MaxSymbolsPerFile {
			result.ParseErrors = append(result.ParseErrors,
				fmt.Sprintf("truncated at %d symbols (max limit)", p.config.MaxSymbolsPerFile))
			break
		}
	}

	return result, nil
}

// extractTypeScriptSymbols extracts symbols from TypeScript/JavaScript AST.
func (p *Parser) extractTypeScriptSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	// Walk the AST
	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "lexical_declaration", "variable_declaration":
			// const X = ... or let X = ... or var X = ...
			symbols = append(symbols, p.extractTSVariableDeclaration(node, content, lines, filePath)...)

		case "function_declaration", "function":
			// function X() { }
			if sym := p.extractTSFunction(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "class_declaration":
			// class X { }
			if sym := p.extractTSClass(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "interface_declaration":
			// interface X { }
			if sym := p.extractTSInterface(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "type_alias_declaration":
			// type X = ...
			if sym := p.extractTSTypeAlias(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "enum_declaration":
			// enum X { }
			symbols = append(symbols, p.extractTSEnum(node, content, lines, filePath)...)

		case "export_statement":
			// Check for export default or export { }
			// Child nodes will be processed recursively
			return true
		}

		return true // continue walking
	})

	return symbols
}

// extractTSVariableDeclaration extracts const/let/var declarations.
func (p *Parser) extractTSVariableDeclaration(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	// Determine if const, let, or var
	declKind := SymbolTypeVar
	if node.Type() == "lexical_declaration" {
		kindNode := node.ChildByFieldName("kind")
		if kindNode != nil && kindNode.Content(content) == "const" {
			declKind = SymbolTypeConst
		}
	}

	// Check if exported (parent is export_statement)
	exported := p.isExported(node)

	// Find variable declarators
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			sym := Symbol{
				Name:       name,
				Type:       declKind,
				FilePath:   filePath,
				LineNumber: int(nameNode.StartPoint().Row) + 1,
				EndLine:    int(node.EndPoint().Row) + 1,
				Column:     int(nameNode.StartPoint().Column),
				Exported:   exported,
			}

			if valueNode != nil {
				sym.RawValue = valueNode.Content(content)
				sym.Value = p.evaluateValue(sym.RawValue)
			}

			// Extract type annotation
			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				sym.TypeAnnotation = typeNode.Content(content)
			}

			// Extract doc comment
			if p.config.IncludeDocComments {
				sym.DocComment = p.extractPrecedingComment(node, content)
			}

			// Add snippet
			sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.EndLine)

			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractTSFunction extracts function declarations.
func (p *Parser) extractTSFunction(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := p.isExported(node)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeFunction,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Extract parameters for signature
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
	}

	// Extract return type
	returnTypeNode := node.ChildByFieldName("return_type")
	if returnTypeNode != nil {
		sym.TypeAnnotation = returnTypeNode.Content(content)
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.EndLine)

	return &sym
}

// extractTSClass extracts class declarations.
func (p *Parser) extractTSClass(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := p.isExported(node)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeClass,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.LineNumber+5) // First few lines

	return &sym
}

// extractTSInterface extracts interface declarations.
func (p *Parser) extractTSInterface(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := p.isExported(node)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeInterface,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.LineNumber+5)

	return &sym
}

// extractTSTypeAlias extracts type alias declarations.
func (p *Parser) extractTSTypeAlias(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := p.isExported(node)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeType,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Get the type value
	valueNode := node.ChildByFieldName("value")
	if valueNode != nil {
		sym.RawValue = valueNode.Content(content)
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.EndLine)

	return &sym
}

// extractTSEnum extracts enum declarations and their values.
func (p *Parser) extractTSEnum(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return symbols
	}

	enumName := nameNode.Content(content)
	exported := p.isExported(node)

	// Add the enum itself
	enumSym := Symbol{
		Name:       enumName,
		Type:       SymbolTypeEnum,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	if p.config.IncludeDocComments {
		enumSym.DocComment = p.extractPrecedingComment(node, content)
	}

	enumSym.Snippet = p.extractSnippet(lines, enumSym.LineNumber, enumSym.EndLine)
	symbols = append(symbols, enumSym)

	// Extract enum members
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		for i := 0; i < int(bodyNode.NamedChildCount()); i++ {
			member := bodyNode.NamedChild(i)
			if member.Type() == "enum_assignment" || member.Type() == "property_identifier" {
				var memberName, memberValue string

				if member.Type() == "enum_assignment" {
					memberNameNode := member.ChildByFieldName("name")
					memberValueNode := member.ChildByFieldName("value")
					if memberNameNode != nil {
						memberName = memberNameNode.Content(content)
					}
					if memberValueNode != nil {
						memberValue = memberValueNode.Content(content)
					}
				} else {
					memberName = member.Content(content)
				}

				if memberName != "" {
					memberSym := Symbol{
						Name:       enumName + "." + memberName,
						Type:       SymbolTypeEnumValue,
						Value:      memberValue,
						FilePath:   filePath,
						LineNumber: int(member.StartPoint().Row) + 1,
						EndLine:    int(member.EndPoint().Row) + 1,
						Column:     int(member.StartPoint().Column),
						Exported:   exported,
						Parent:     enumName,
					}
					symbols = append(symbols, memberSym)
				}
			}
		}
	}

	return symbols
}

// extractGoSymbols extracts symbols from Go AST.
func (p *Parser) extractGoSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "const_declaration":
			symbols = append(symbols, p.extractGoConst(node, content, lines, filePath)...)

		case "var_declaration":
			symbols = append(symbols, p.extractGoVar(node, content, lines, filePath)...)

		case "function_declaration":
			if sym := p.extractGoFunction(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "method_declaration":
			if sym := p.extractGoMethod(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "type_declaration":
			symbols = append(symbols, p.extractGoType(node, content, lines, filePath)...)
		}

		return true
	})

	return symbols
}

// extractGoConst extracts Go const declarations.
func (p *Parser) extractGoConst(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "const_spec" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			exported := isGoExported(name)

			sym := Symbol{
				Name:       name,
				Type:       SymbolTypeConst,
				FilePath:   filePath,
				LineNumber: int(nameNode.StartPoint().Row) + 1,
				EndLine:    int(child.EndPoint().Row) + 1,
				Column:     int(nameNode.StartPoint().Column),
				Exported:   exported,
			}

			if valueNode != nil {
				sym.RawValue = valueNode.Content(content)
				sym.Value = p.evaluateValue(sym.RawValue)
			}

			// Type annotation
			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				sym.TypeAnnotation = typeNode.Content(content)
			}

			if p.config.IncludeDocComments {
				sym.DocComment = p.extractPrecedingComment(node, content)
			}

			sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.EndLine)
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractGoVar extracts Go var declarations.
func (p *Parser) extractGoVar(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "var_spec" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			exported := isGoExported(name)

			sym := Symbol{
				Name:       name,
				Type:       SymbolTypeVar,
				FilePath:   filePath,
				LineNumber: int(nameNode.StartPoint().Row) + 1,
				EndLine:    int(child.EndPoint().Row) + 1,
				Column:     int(nameNode.StartPoint().Column),
				Exported:   exported,
			}

			if valueNode != nil {
				sym.RawValue = valueNode.Content(content)
				sym.Value = p.evaluateValue(sym.RawValue)
			}

			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				sym.TypeAnnotation = typeNode.Content(content)
			}

			if p.config.IncludeDocComments {
				sym.DocComment = p.extractPrecedingComment(node, content)
			}

			sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.EndLine)
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractGoFunction extracts Go function declarations.
func (p *Parser) extractGoFunction(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := isGoExported(name)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeFunction,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Extract signature (parameters + return type)
	paramsNode := node.ChildByFieldName("parameters")
	resultNode := node.ChildByFieldName("result")

	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
		if resultNode != nil {
			sym.Signature += " " + resultNode.Content(content)
		}
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.LineNumber+5)

	return &sym
}

// extractGoMethod extracts Go method declarations.
func (p *Parser) extractGoMethod(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := isGoExported(name)

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeMethod,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Extract receiver type as parent
	receiverNode := node.ChildByFieldName("receiver")
	if receiverNode != nil {
		// Find the type identifier in the receiver
		for i := 0; i < int(receiverNode.NamedChildCount()); i++ {
			param := receiverNode.NamedChild(i)
			typeNode := param.ChildByFieldName("type")
			if typeNode != nil {
				typeName := typeNode.Content(content)
				// Remove pointer prefix if present
				typeName = strings.TrimPrefix(typeName, "*")
				sym.Parent = typeName
				break
			}
		}
	}

	paramsNode := node.ChildByFieldName("parameters")
	resultNode := node.ChildByFieldName("result")

	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
		if resultNode != nil {
			sym.Signature += " " + resultNode.Content(content)
		}
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.LineNumber+5)

	return &sym
}

// extractGoType extracts Go type declarations (struct, interface, type alias).
func (p *Parser) extractGoType(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_spec" {
			nameNode := child.ChildByFieldName("name")
			typeNode := child.ChildByFieldName("type")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			exported := isGoExported(name)

			sym := Symbol{
				Name:       name,
				FilePath:   filePath,
				LineNumber: int(nameNode.StartPoint().Row) + 1,
				EndLine:    int(child.EndPoint().Row) + 1,
				Column:     int(nameNode.StartPoint().Column),
				Exported:   exported,
			}

			// Determine type kind
			if typeNode != nil {
				switch typeNode.Type() {
				case "struct_type":
					sym.Type = SymbolTypeStruct
				case "interface_type":
					sym.Type = SymbolTypeInterface
				default:
					sym.Type = SymbolTypeType
					sym.RawValue = typeNode.Content(content)
				}
			}

			if p.config.IncludeDocComments {
				sym.DocComment = p.extractPrecedingComment(node, content)
			}

			sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.LineNumber+10)
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractPythonSymbols extracts symbols from Python AST.
func (p *Parser) extractPythonSymbols(root *sitter.Node, content []byte, filePath string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	p.walkTree(root, func(node *sitter.Node) bool {
		nodeType := node.Type()

		switch nodeType {
		case "assignment":
			// CONSTANT = value (module-level with UPPER_CASE)
			symbols = append(symbols, p.extractPythonAssignment(node, content, lines, filePath)...)

		case "function_definition":
			if sym := p.extractPythonFunction(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}

		case "class_definition":
			if sym := p.extractPythonClass(node, content, lines, filePath); sym != nil {
				symbols = append(symbols, *sym)
			}
		}

		return true
	})

	return symbols
}

// extractPythonAssignment extracts Python assignments (constants use UPPER_CASE).
func (p *Parser) extractPythonAssignment(node *sitter.Node, content []byte, lines []string, filePath string) []Symbol {
	var symbols []Symbol

	// Get the left side (name)
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")

	if leftNode == nil || leftNode.Type() != "identifier" {
		return symbols
	}

	name := leftNode.Content(content)

	// Check if it's a constant (UPPER_CASE convention)
	isConstant := isUpperCase(name)
	exported := !strings.HasPrefix(name, "_")

	symType := SymbolTypeVar
	if isConstant {
		symType = SymbolTypeConst
	}

	sym := Symbol{
		Name:       name,
		Type:       symType,
		FilePath:   filePath,
		LineNumber: int(leftNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(leftNode.StartPoint().Column),
		Exported:   exported,
	}

	if rightNode != nil {
		sym.RawValue = rightNode.Content(content)
		sym.Value = p.evaluateValue(sym.RawValue)
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPrecedingComment(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.EndLine)
	symbols = append(symbols, sym)

	return symbols
}

// extractPythonFunction extracts Python function definitions.
func (p *Parser) extractPythonFunction(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := !strings.HasPrefix(name, "_")

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeFunction,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	// Extract parameters
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		sym.Signature = paramsNode.Content(content)
	}

	// Extract return type annotation
	returnTypeNode := node.ChildByFieldName("return_type")
	if returnTypeNode != nil {
		sym.TypeAnnotation = returnTypeNode.Content(content)
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPythonDocstring(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.LineNumber+5)

	return &sym
}

// extractPythonClass extracts Python class definitions.
func (p *Parser) extractPythonClass(node *sitter.Node, content []byte, lines []string, filePath string) *Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	exported := !strings.HasPrefix(name, "_")

	sym := Symbol{
		Name:       name,
		Type:       SymbolTypeClass,
		FilePath:   filePath,
		LineNumber: int(nameNode.StartPoint().Row) + 1,
		EndLine:    int(node.EndPoint().Row) + 1,
		Column:     int(nameNode.StartPoint().Column),
		Exported:   exported,
	}

	if p.config.IncludeDocComments {
		sym.DocComment = p.extractPythonDocstring(node, content)
	}

	sym.Snippet = p.extractSnippet(lines, sym.LineNumber, sym.LineNumber+10)

	return &sym
}

// Helper functions

// walkTree walks the AST calling fn for each node.
func (p *Parser) walkTree(node *sitter.Node, fn func(*sitter.Node) bool) {
	if !fn(node) {
		return
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		p.walkTree(node.Child(i), fn)
	}
}

// isExported checks if a TypeScript/JavaScript node is exported.
func (p *Parser) isExported(node *sitter.Node) bool {
	// Check if parent is export_statement
	parent := node.Parent()
	if parent != nil && parent.Type() == "export_statement" {
		return true
	}

	// Check for export keyword in declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "export" {
			return true
		}
	}

	return false
}

// isGoExported checks if a Go identifier is exported (starts with uppercase).
func isGoExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	first := rune(name[0])
	return first >= 'A' && first <= 'Z'
}

// isUpperCase checks if a string is all uppercase (Python constant convention).
func isUpperCase(s string) bool {
	hasLetter := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
	}
	return hasLetter
}

// extractPrecedingComment extracts comment above a node.
func (p *Parser) extractPrecedingComment(node *sitter.Node, content []byte) string {
	// Look for comment node before this one
	if node.PrevSibling() != nil {
		prev := node.PrevSibling()
		if prev.Type() == "comment" {
			return strings.TrimSpace(prev.Content(content))
		}
	}
	return ""
}

// extractPythonDocstring extracts docstring from Python function/class.
func (p *Parser) extractPythonDocstring(node *sitter.Node, content []byte) string {
	// Look for expression_statement with string as first child in body
	bodyNode := node.ChildByFieldName("body")
	if bodyNode == nil {
		return ""
	}

	if bodyNode.NamedChildCount() > 0 {
		first := bodyNode.NamedChild(0)
		if first.Type() == "expression_statement" && first.NamedChildCount() > 0 {
			expr := first.NamedChild(0)
			if expr.Type() == "string" {
				docstring := expr.Content(content)
				// Remove quotes
				docstring = strings.Trim(docstring, "\"'")
				docstring = strings.TrimPrefix(docstring, "\"\"")
				docstring = strings.TrimSuffix(docstring, "\"\"")
				return strings.TrimSpace(docstring)
			}
		}
	}

	return ""
}

// extractSnippet extracts source lines with context.
func (p *Parser) extractSnippet(lines []string, startLine, endLine int) string {
	// Convert to 0-indexed
	start := startLine - 1 - p.config.ContextLines
	end := endLine - 1 + p.config.ContextLines

	if start < 0 {
		start = 0
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}

	// Limit snippet size
	maxLines := 15
	if end-start > maxLines {
		end = start + maxLines
	}

	return strings.Join(lines[start:end+1], "\n")
}

// evaluateValue attempts to evaluate simple constant expressions.
func (p *Parser) evaluateValue(raw string) string {
	if !p.config.EvaluateConstants {
		return raw
	}

	// Try to evaluate simple arithmetic
	raw = strings.TrimSpace(raw)

	// Handle simple multiplication like "60 * 1000"
	if strings.Contains(raw, "*") {
		parts := strings.Split(raw, "*")
		if len(parts) == 2 {
			a := parseNumber(strings.TrimSpace(parts[0]))
			b := parseNumber(strings.TrimSpace(parts[1]))
			if a != 0 && b != 0 {
				return fmt.Sprintf("%d", a*b)
			}
		}
	}

	// Handle simple addition
	if strings.Contains(raw, "+") && !strings.Contains(raw, "\"") {
		parts := strings.Split(raw, "+")
		if len(parts) == 2 {
			a := parseNumber(strings.TrimSpace(parts[0]))
			b := parseNumber(strings.TrimSpace(parts[1]))
			if a != 0 || b != 0 {
				return fmt.Sprintf("%d", a+b)
			}
		}
	}

	return raw
}

// parseNumber parses a simple integer from string.
func parseNumber(s string) int64 {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0
	}
	return n
}

// Close releases parser resources.
func (p *Parser) Close() {
	if p.tsParser != nil {
		p.tsParser.Close()
	}
}

// ParseDirectory parses all supported files in a directory.
func (p *Parser) ParseDirectory(ctx context.Context, dir string) ([]*FileSymbols, error) {
	var results []*FileSymbols
	var parseErrors []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip common non-code directories
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file is supported
		ext := filepath.Ext(path)
		lang := LanguageFromExtension(ext)
		if !lang.IsSupported() {
			return nil
		}

		// Parse file
		relPath, _ := filepath.Rel(dir, path)
		fileSymbols, err := p.ParseFile(ctx, path)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("%s: %v", relPath, err))
			log.Printf("Warning: failed to parse %s: %v", relPath, err)
			return nil // Continue with other files
		}

		// Update path to relative
		fileSymbols.FilePath = relPath
		for i := range fileSymbols.Symbols {
			fileSymbols.Symbols[i].FilePath = relPath
		}

		results = append(results, fileSymbols)
		return nil
	})

	if err != nil {
		return results, fmt.Errorf("directory walk failed: %w", err)
	}

	return results, nil
}
