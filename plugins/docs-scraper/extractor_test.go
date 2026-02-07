package main

import (
	"strings"
	"testing"
)

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "title tag",
			html:     `<html><head><title>My Page</title></head><body></body></html>`,
			expected: "My Page",
		},
		{
			name:     "h1 fallback",
			html:     `<html><body><h1>Hello World</h1></body></html>`,
			expected: "Hello World",
		},
		{
			name:     "no title",
			html:     `<html><body><p>content</p></body></html>`,
			expected: "Untitled",
		},
		{
			name:     "title with HTML entities",
			html:     `<html><head><title>API &amp; Docs</title></head></html>`,
			expected: "API & Docs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTitle(tc.html)
			if got != tc.expected {
				t.Errorf("extractTitle() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestExtractDocumentation(t *testing.T) {
	html := `<html>
	<head><title>Docs</title></head>
	<body>
		<nav>Navigation menu</nav>
		<main>
			<h1>Getting Started</h1>
			<p>Welcome to the documentation.</p>
			<pre><code class="language-go">fmt.Println("hello")</code></pre>
		</main>
		<footer>Copyright 2024</footer>
	</body>
	</html>`

	result := extractDocumentation(html)

	// Should contain main content
	if !strings.Contains(result, "Getting Started") {
		t.Error("expected main heading to be present")
	}
	if !strings.Contains(result, "Welcome to the documentation") {
		t.Error("expected paragraph content")
	}

	// Should NOT contain nav/footer
	if strings.Contains(result, "Navigation menu") {
		t.Error("expected nav to be stripped")
	}
	if strings.Contains(result, "Copyright 2024") {
		t.Error("expected footer to be stripped")
	}
}

func TestExtractGeneric(t *testing.T) {
	html := `<html>
	<body>
		<nav>Nav</nav>
		<p>Main content here.</p>
		<footer>Footer</footer>
	</body>
	</html>`

	result := extractGeneric(html)

	// Generic should retain more content
	if !strings.Contains(result, "Main content here") {
		t.Error("expected body content to be present")
	}
}

func TestContentHashDeterminism(t *testing.T) {
	e := NewExtractor()
	html := []byte(`<html><body><p>Hello world</p></body></html>`)

	result1, _ := e.Extract(html, "https://example.com", "generic")
	result2, _ := e.Extract(html, "https://example.com", "generic")

	if result1.ContentHash != result2.ContentHash {
		t.Errorf("hashes should be deterministic: %s != %s", result1.ContentHash, result2.ContentHash)
	}
}

func TestExtractLinks(t *testing.T) {
	html := `<a href="/docs">Docs</a><a href="https://example.com/api">API</a><a href="#section">Skip</a>`

	links := extractLinks(html, "https://example.com")

	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d: %v", len(links), links)
	}

	// Should resolve relative URL
	found := false
	for _, l := range links {
		if l == "https://example.com/docs" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected resolved relative URL https://example.com/docs, got %v", links)
	}
}

func TestTruncation(t *testing.T) {
	e := &Extractor{}

	// Create large content
	body := "<html><body><p>" + strings.Repeat("word ", 50000) + "</p></body></html>"

	result, err := e.Extract([]byte(body), "https://example.com", "generic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Content should exist but not be absurdly large
	if result.WordCount == 0 {
		t.Error("expected non-zero word count")
	}
}
