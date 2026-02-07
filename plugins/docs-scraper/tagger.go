package main

import (
	"net/url"
	"regexp"
	"strings"
)

// Tagger suggests tags for scraped content.
type Tagger struct{}

// NewTagger creates a new tagger.
func NewTagger() *Tagger {
	return &Tagger{}
}

// SuggestTags returns a list of suggested tags based on content analysis.
func (t *Tagger) SuggestTags(title, content, rawURL string) []string {
	tags := []string{"source:web-scraper"}

	// Domain tag
	if u, err := url.Parse(rawURL); err == nil {
		tags = append(tags, "domain:"+u.Hostname())

		// URL path patterns
		pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for _, part := range pathParts {
			part = strings.ToLower(part)
			switch part {
			case "docs", "documentation", "doc":
				tags = addUnique(tags, "type:documentation")
			case "api", "reference":
				tags = addUnique(tags, "type:api-reference")
			case "guide", "guides", "tutorial", "tutorials":
				tags = addUnique(tags, "type:guide")
			case "blog", "posts":
				tags = addUnique(tags, "type:blog")
			case "faq", "help":
				tags = addUnique(tags, "type:help")
			}
		}
	}

	// Title-based tags
	titleLower := strings.ToLower(title)
	if strings.Contains(titleLower, "api") {
		tags = addUnique(tags, "type:api-reference")
	}
	if strings.Contains(titleLower, "getting started") || strings.Contains(titleLower, "quickstart") {
		tags = addUnique(tags, "type:getting-started")
	}
	if strings.Contains(titleLower, "install") || strings.Contains(titleLower, "setup") {
		tags = addUnique(tags, "type:installation")
	}
	if strings.Contains(titleLower, "changelog") || strings.Contains(titleLower, "release") {
		tags = addUnique(tags, "type:changelog")
	}

	// Code language detection
	codeBlockRe := regexp.MustCompile("```(\\w+)")
	matches := codeBlockRe.FindAllStringSubmatch(content, -1)
	langSeen := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			lang := strings.ToLower(m[1])
			if !langSeen[lang] && isKnownLanguage(lang) {
				langSeen[lang] = true
				tags = append(tags, "lang:"+lang)
			}
		}
	}

	return tags
}

func addUnique(tags []string, tag string) []string {
	for _, t := range tags {
		if t == tag {
			return tags
		}
	}
	return append(tags, tag)
}

func isKnownLanguage(lang string) bool {
	known := map[string]bool{
		"go": true, "python": true, "javascript": true, "typescript": true,
		"java": true, "rust": true, "c": true, "cpp": true, "csharp": true,
		"ruby": true, "php": true, "swift": true, "kotlin": true,
		"bash": true, "shell": true, "sql": true, "yaml": true, "json": true,
		"html": true, "css": true, "xml": true, "toml": true, "markdown": true,
	}
	return known[lang]
}
