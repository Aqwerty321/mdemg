package languages

import (
	"path/filepath"
	"strings"
)

func init() {
	Register(&MarkdownParser{})
}

// MarkdownParser implements LanguageParser for Markdown documentation files
type MarkdownParser struct{}

func (p *MarkdownParser) Name() string {
	return "markdown"
}

func (p *MarkdownParser) Extensions() []string {
	return []string{".md", ".markdown"}
}

func (p *MarkdownParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".md") || strings.HasSuffix(pathLower, ".markdown")
}

func (p *MarkdownParser) IsTestFile(path string) bool {
	// Markdown files are never test files
	return false
}

func (p *MarkdownParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)

	// Truncate content if too long
	contentStr := TruncateContent(content, 4000)

	// Detect cross-cutting concerns from docs
	concerns := DetectConcerns(relPath, content)
	tags := []string{"documentation", "markdown"}
	tags = append(tags, concerns...)

	// Check if this is a configuration doc file
	mdKind := "documentation"
	if IsConfigFile(relPath) {
		tags = append(tags, "config")
		mdKind = "config"
	}

	element := CodeElement{
		Name:     name,
		Kind:     mdKind,
		Path:     "/" + relPath,
		Content:  contentStr,
		Package:  "docs",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
	}

	return []CodeElement{element}, nil
}
