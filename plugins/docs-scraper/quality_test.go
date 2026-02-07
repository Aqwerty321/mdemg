package main

import (
	"strings"
	"testing"
)

func TestHighQualityDocs(t *testing.T) {
	content := `# Getting Started Guide

## Prerequisites

Before you begin, ensure you have the following installed:

- Go 1.21 or later
- Docker
- Git

## Installation

Follow these steps to set up the project:

` + "```bash\ngit clone https://github.com/example/project.git\ncd project\ngo build ./...\n```" + `

## Configuration

Create a configuration file with the following settings. The default configuration
should work for most users, but you may need to adjust the database settings.

## Running Tests

Run the test suite to verify your installation is working correctly:

` + "```go\nfunc TestExample(t *testing.T) {\n    // Test code here\n}\n```" + `

This guide covers the basic setup. For more advanced configuration options,
see the API Reference documentation.`

	scorer := NewQualityScorer()
	score := scorer.Score(content, len(strings.Fields(content)))

	if score < 0.7 {
		t.Errorf("expected high-quality docs to score >= 0.7, got %.2f", score)
	}
}

func TestLowQualityContent(t *testing.T) {
	content := "click here buy now"

	scorer := NewQualityScorer()
	score := scorer.Score(content, len(strings.Fields(content)))

	if score > 0.5 {
		t.Errorf("expected low-quality content to score < 0.5, got %.2f", score)
	}
}

func TestLinkHeavyPenalty(t *testing.T) {
	// Content that's mostly links
	content := `[Link 1](http://a.com) [Link 2](http://b.com) [Link 3](http://c.com) [Link 4](http://d.com) [Link 5](http://e.com) word`

	scorer := NewQualityScorer()
	score := scorer.Score(content, len(strings.Fields(content)))

	if score > 0.6 {
		t.Errorf("expected link-heavy content to be penalized (score < 0.6), got %.2f", score)
	}
}

func TestEmptyContent(t *testing.T) {
	scorer := NewQualityScorer()
	score := scorer.Score("", 0)

	if score != 0.0 {
		t.Errorf("expected empty content to score 0.0, got %.2f", score)
	}
}

func TestModerateContent(t *testing.T) {
	content := `# API Reference

The API provides the following endpoints for managing resources.

Users can authenticate using API keys or OAuth tokens.`

	scorer := NewQualityScorer()
	score := scorer.Score(content, len(strings.Fields(content)))

	if score < 0.3 || score > 0.9 {
		t.Errorf("expected moderate content score in [0.3, 0.9], got %.2f", score)
	}
}
