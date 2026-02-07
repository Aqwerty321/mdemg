package scraper

import (
	"strings"
	"testing"
)

// --- Small fixture (< 2000 words) ---
const smallPage = `# Small Page

This is a short page with very few words.

## Section A

Some content here.

` + "```go\nfunc main() {}\n```\n"

// --- Large fixture (> 2000 words, with multiple ## sections) ---
func largePage() string {
	var b strings.Builder
	b.WriteString("# Large Documentation Page\n\n")
	b.WriteString("Introductory paragraph for the large page.\n\n")

	sections := []struct {
		heading string
		lang    string
	}{
		{"Installation", "bash"},
		{"Configuration", "yaml"},
		{"API Reference", "go"},
	}

	for _, sec := range sections {
		b.WriteString("## " + sec.heading + "\n\n")
		// Write enough words to exceed MinSectionWords (100) and push total > 2000
		for i := 0; i < 700; i++ {
			b.WriteString("word ")
		}
		b.WriteString("\n\n")
		b.WriteString("```" + sec.lang + "\n")
		b.WriteString("example code here\n")
		b.WriteString("```\n\n")
		b.WriteString("[" + sec.heading + " Docs](https://example.com/" + strings.ToLower(sec.heading) + ")\n\n")
	}
	return b.String()
}

func TestParser_SmallPage_NoChunking(t *testing.T) {
	p := NewParser(DefaultParserConfig())
	result := p.Parse("Small Page", smallPage, "https://example.com/small", nil, 0.9)

	if result.WasChunked {
		t.Error("expected WasChunked=false for small page")
	}
	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(result.Sections))
	}
	if result.TotalWordCount >= 2000 {
		t.Errorf("expected < 2000 words, got %d", result.TotalWordCount)
	}
}

func TestParser_LargePage_Chunking(t *testing.T) {
	p := NewParser(DefaultParserConfig())
	content := largePage()
	result := p.Parse("Large Docs", content, "https://pkg.go.dev/net/http", nil, 0.85)

	if !result.WasChunked {
		t.Error("expected WasChunked=true for large page")
	}
	// Should have at least 3 sections (one per ## heading) + possibly intro
	if len(result.Sections) < 3 {
		t.Errorf("expected >= 3 sections, got %d", len(result.Sections))
	}
	// Each section should have content
	for i, sec := range result.Sections {
		if sec.WordCount == 0 {
			t.Errorf("section %d (%s) has 0 words", i, sec.Title)
		}
	}
}

func TestParser_SymbolExtraction_Headings(t *testing.T) {
	content := "# Top Level\n\n## Second Level\n\n### Third Level\n\nSome text.\n"
	p := NewParser(DefaultParserConfig())
	// Use small config to avoid chunking
	p.config.MinWordCount = 100000

	result := p.Parse("Test", content, "", nil, 1.0)
	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(result.Sections))
	}

	symbols := result.Sections[0].Symbols
	var headings []ParsedSymbol
	for _, s := range symbols {
		if s.Type == "heading" || s.Type == "section" {
			headings = append(headings, s)
		}
	}
	if len(headings) < 3 {
		t.Fatalf("expected >= 3 heading symbols, got %d", len(headings))
	}

	// Check parent tracking
	found := false
	for _, h := range headings {
		if h.Name == "Third Level" && h.Parent == "Second Level" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Third Level' to have parent 'Second Level'")
	}
}

func TestParser_SymbolExtraction_CodeBlocks(t *testing.T) {
	content := "# Doc\n\n```python\nprint('hi')\n```\n\n```go\nfmt.Println()\n```\n\n```python\nmore code\n```\n"
	p := NewParser(DefaultParserConfig())
	p.config.MinWordCount = 100000

	result := p.Parse("Test", content, "", nil, 1.0)
	symbols := result.Sections[0].Symbols

	var codeBlocks []ParsedSymbol
	for _, s := range symbols {
		if s.Type == "code_block" {
			codeBlocks = append(codeBlocks, s)
		}
	}
	// UPTS parser deduplicates by line, but each occurrence is separate
	if len(codeBlocks) < 2 {
		t.Errorf("expected >= 2 code_block symbols, got %d", len(codeBlocks))
	}

	// Check that python and go are both found
	langs := make(map[string]bool)
	for _, cb := range codeBlocks {
		langs[cb.Name] = true
	}
	if !langs["python"] {
		t.Error("expected python code_block symbol")
	}
	if !langs["go"] {
		t.Error("expected go code_block symbol")
	}
}

func TestParser_SymbolExtraction_Links(t *testing.T) {
	content := "# Links\n\n[GitHub](https://github.com)\n[Local](/api/reference)\n\nInline text with [embedded](https://inline.com) link.\n"
	p := NewParser(DefaultParserConfig())
	p.config.MinWordCount = 100000

	result := p.Parse("Test", content, "", nil, 1.0)
	symbols := result.Sections[0].Symbols

	var links []ParsedSymbol
	for _, s := range symbols {
		if s.Type == "link" {
			links = append(links, s)
		}
	}
	// Only standalone links (lines starting with [) are extracted
	if len(links) < 2 {
		t.Errorf("expected >= 2 link symbols, got %d", len(links))
	}

	foundGH := false
	for _, l := range links {
		if l.Name == "GitHub" && l.Value == "https://github.com" {
			foundGH = true
		}
	}
	if !foundGH {
		t.Error("expected GitHub link symbol")
	}
}

func TestParser_MergeTinySections(t *testing.T) {
	p := NewParser(DefaultParserConfig())
	p.config.MinSectionWords = 50

	// Create content with one tiny section followed by a big one
	var b strings.Builder
	b.WriteString("## Tiny\n\nJust a few words.\n\n")
	b.WriteString("## Big Section\n\n")
	for i := 0; i < 200; i++ {
		b.WriteString("word ")
	}
	b.WriteString("\n\n## Another Big\n\n")
	for i := 0; i < 200; i++ {
		b.WriteString("word ")
	}

	chunks := p.chunkByHeadings(b.String())
	merged := p.mergeTinySections(chunks)

	// Tiny should be merged into Big Section
	if len(merged) >= len(chunks) {
		t.Errorf("expected fewer sections after merge: before=%d, after=%d", len(chunks), len(merged))
	}
	// First merged section should contain content from both tiny and big
	if merged[0].heading == "" {
		t.Error("merged section should have a heading")
	}
}

func TestParser_TagGeneration(t *testing.T) {
	content := "## Installation\n\n```go\nfunc main() {}\n```\n\n```python\nprint('hi')\n```\n"
	p := NewParser(DefaultParserConfig())
	p.config.MinWordCount = 100000

	result := p.Parse("Test", content, "https://pkg.go.dev/net/http", []string{"source:web-scraper"}, 0.9)
	tags := result.Sections[0].Tags

	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}

	// Check base tags preserved
	if !tagSet["source:web-scraper"] {
		t.Error("expected base tag 'source:web-scraper'")
	}

	// Check lang tags
	if !tagSet["lang:go"] {
		t.Error("expected 'lang:go' tag")
	}
	if !tagSet["lang:python"] {
		t.Error("expected 'lang:python' tag")
	}

	// Check domain tag
	if !tagSet["domain:pkg.go.dev"] {
		t.Error("expected 'domain:pkg.go.dev' tag")
	}

	// Check topic tag from heading
	if !tagSet["topic:installation"] {
		t.Error("expected 'topic:installation' tag")
	}

	// Check no duplicate tags
	seen := make(map[string]int)
	for _, tag := range tags {
		seen[tag]++
		if seen[tag] > 1 {
			t.Errorf("duplicate tag: %s", tag)
		}
	}
}

func TestParser_ContentPrefix(t *testing.T) {
	p := NewParser(DefaultParserConfig())
	p.config.MinWordCount = 100000

	content := "Some content here."
	result := p.Parse("My Page", content, "", nil, 1.0)

	sec := result.Sections[0]
	if !strings.HasPrefix(sec.Content, "# My Page\n\n") {
		t.Errorf("expected content prefix '# My Page\\n\\n', got: %q", sec.Content[:min(50, len(sec.Content))])
	}
}

func TestParser_ContentPrefix_WithSection(t *testing.T) {
	p := NewParser(DefaultParserConfig())
	// Force chunking with low threshold
	p.config.MinWordCount = 5

	content := "## Setup\n\nSome setup instructions with enough words to pass.\n\n## Usage\n\nSome usage instructions with enough words to pass.\n"
	result := p.Parse("My Library", content, "", nil, 1.0)

	if !result.WasChunked {
		t.Fatal("expected chunking")
	}
	for _, sec := range result.Sections {
		if sec.Title != "" {
			expected := "# My Library > " + sec.Title
			if !strings.HasPrefix(sec.Content, expected) {
				t.Errorf("expected prefix %q in section %q, got: %q", expected, sec.Title, sec.Content[:min(80, len(sec.Content))])
			}
		}
	}
}

func TestParser_NoContextPrefix(t *testing.T) {
	cfg := DefaultParserConfig()
	cfg.IncludePageContext = false
	p := NewParser(cfg)

	content := "Some plain content."
	result := p.Parse("Title", content, "", nil, 1.0)

	if strings.Contains(result.Sections[0].Content, "# Title") {
		t.Error("expected no context prefix when IncludePageContext=false")
	}
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  spaces   everywhere  ", 2},
		{"line\nbreaks\ncount", 3},
	}
	for _, tt := range tests {
		got := countWords(tt.input)
		if got != tt.want {
			t.Errorf("countWords(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://pkg.go.dev/net/http", "pkg.go.dev"},
		{"http://example.com:8080/path", "example.com"},
		{"", ""},
		{"not-a-url", ""},
		{"https://docs.python.org/3/library/", "docs.python.org"},
	}
	for _, tt := range tests {
		got := extractDomain(tt.input)
		if got != tt.want {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractKeyTerms(t *testing.T) {
	content := "## Installation Guide\n\n## API Reference\n\n## The Quick Overview\n"
	terms := extractKeyTerms(content)

	termSet := make(map[string]bool)
	for _, term := range terms {
		termSet[term] = true
	}

	if !termSet["installation"] {
		t.Error("expected 'installation' term")
	}
	if !termSet["guide"] {
		t.Error("expected 'guide' term")
	}
	if !termSet["api"] {
		t.Error("expected 'api' term")
	}
	if !termSet["reference"] {
		t.Error("expected 'reference' term")
	}
	// "the" should be filtered as stop word
	if termSet["the"] {
		t.Error("stop word 'the' should be filtered")
	}
}

func TestExtractCodeLanguages(t *testing.T) {
	content := "```go\ncode\n```\n\n```python\ncode\n```\n\n```go\nmore\n```\n"
	langs := extractCodeLanguages(content)

	if len(langs) != 2 {
		t.Errorf("expected 2 unique languages, got %d: %v", len(langs), langs)
	}
	langSet := make(map[string]bool)
	for _, l := range langs {
		langSet[l] = true
	}
	if !langSet["go"] || !langSet["python"] {
		t.Errorf("expected go and python, got %v", langs)
	}
}

func TestParser_SectionIndex(t *testing.T) {
	p := NewParser(DefaultParserConfig())
	p.config.MinWordCount = 5

	content := "## A\n\nContent A.\n\n## B\n\nContent B.\n\n## C\n\nContent C.\n"
	result := p.Parse("Doc", content, "", nil, 1.0)

	for i, sec := range result.Sections {
		if sec.SectionIndex != i {
			t.Errorf("section %d has SectionIndex=%d", i, sec.SectionIndex)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
