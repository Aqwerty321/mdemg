package main

import (
	"regexp"
	"strings"
)

// QualityScorer evaluates the quality of extracted content.
type QualityScorer struct{}

// NewQualityScorer creates a new quality scorer.
func NewQualityScorer() *QualityScorer {
	return &QualityScorer{}
}

// Score returns a quality score between 0.0 and 1.0 based on content heuristics.
func (q *QualityScorer) Score(content string, wordCount int) float64 {
	if wordCount == 0 {
		return 0.0
	}

	score := 0.0
	maxScore := 0.0

	// 1. Content length (25% weight)
	maxScore += 0.25
	switch {
	case wordCount >= 200:
		score += 0.25
	case wordCount >= 100:
		score += 0.20
	case wordCount >= 50:
		score += 0.15
	case wordCount >= 20:
		score += 0.10
	default:
		score += 0.05
	}

	// 2. Heading structure (20% weight)
	maxScore += 0.20
	headingCount := strings.Count(content, "# ")
	switch {
	case headingCount >= 3:
		score += 0.20
	case headingCount >= 1:
		score += 0.15
	default:
		score += 0.05
	}

	// 3. Code blocks (15% weight)
	maxScore += 0.15
	codeBlockCount := strings.Count(content, "```")
	if codeBlockCount >= 2 {
		score += 0.15
	} else if codeBlockCount >= 1 {
		score += 0.10
	} else {
		score += 0.05
	}

	// 4. Link density penalty (15% weight — high link density = low quality)
	maxScore += 0.15
	linkRe := regexp.MustCompile(`\[.*?\]\(.*?\)`)
	linkCount := len(linkRe.FindAllString(content, -1))
	linkRatio := float64(linkCount) / float64(wordCount)
	switch {
	case linkRatio < 0.05:
		score += 0.15
	case linkRatio < 0.15:
		score += 0.10
	case linkRatio < 0.30:
		score += 0.05
	default:
		score += 0.0 // Heavy link pages are low quality
	}

	// 5. Text coherence — sentences per paragraph proxy (15% weight)
	maxScore += 0.15
	paragraphs := strings.Split(content, "\n\n")
	meaningfulParas := 0
	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if len(trimmed) > 50 {
			meaningfulParas++
		}
	}
	switch {
	case meaningfulParas >= 5:
		score += 0.15
	case meaningfulParas >= 2:
		score += 0.10
	case meaningfulParas >= 1:
		score += 0.05
	}

	// 6. Lists present (10% weight)
	maxScore += 0.10
	listItems := strings.Count(content, "\n- ")
	if listItems >= 3 {
		score += 0.10
	} else if listItems >= 1 {
		score += 0.05
	}

	if maxScore == 0 {
		return 0.5
	}
	return score / maxScore
}
