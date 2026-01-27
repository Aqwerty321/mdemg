package conversation

import (
	"testing"
)

func TestComputeTermNovelty(t *testing.T) {
	detector := &SurpriseDetector{}

	tests := []struct {
		name     string
		content  string
		expected float64 // approximate expected range
		minScore float64
		maxScore float64
	}{
		{
			name:     "simple text with no technical terms",
			content:  "The user prefers using tabs for indentation",
			minScore: 0.0,
			maxScore: 0.1,
		},
		{
			name:     "text with PascalCase technical term",
			content:  "The codebase uses BlueSeerValidator for validation",
			minScore: 0.15,
			maxScore: 0.5,
		},
		{
			name:     "text with multiple technical terms",
			content:  "BlueSeerData and BlueSeerValidator are custom frameworks used in this ORM",
			minScore: 0.05,
			maxScore: 0.4,
		},
		{
			name:     "text with acronyms",
			content:  "The API uses REST endpoints and JSON for data transfer",
			minScore: 0.2,
			maxScore: 0.6,
		},
		{
			name:     "text with snake_case",
			content:  "Use the custom_validation_framework for input checking",
			minScore: 0.1,
			maxScore: 0.4,
		},
		{
			name:     "empty content",
			content:  "",
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := detector.computeTermNovelty(tt.content)

			if tt.expected > 0 {
				if score != tt.expected {
					t.Errorf("expected exactly %.2f, got %.2f", tt.expected, score)
				}
			} else {
				if score < tt.minScore || score > tt.maxScore {
					t.Errorf("expected score in range [%.2f, %.2f], got %.2f", tt.minScore, tt.maxScore, score)
				}
			}

			// Score should always be in valid range
			if score < 0.0 || score > 1.0 {
				t.Errorf("score %.2f is out of valid range [0.0, 1.0]", score)
			}
		})
	}
}

func TestDetectCorrection(t *testing.T) {
	detector := &SurpriseDetector{}

	tests := []struct {
		name     string
		content  string
		expected float64
	}{
		{
			name:     "explicit no that's wrong",
			content:  "No, that's wrong. The API uses GraphQL.",
			expected: 0.9,
		},
		{
			name:     "actually correction",
			content:  "Actually, it's PostgreSQL, not MySQL.",
			expected: 0.9,
		},
		{
			name:     "you're mistaken",
			content:  "You're mistaken about the architecture.",
			expected: 0.9,
		},
		{
			name:     "correction label",
			content:  "Correction: The ORM is called BlueSeerData.",
			expected: 0.9,
		},
		{
			name:     "not X but Y pattern",
			content:  "Not REST, but GraphQL.",
			expected: 0.9,
		},
		{
			name:     "incorrect statement",
			content:  "That's incorrect. We use tabs.",
			expected: 0.9,
		},
		{
			name:     "let me correct",
			content:  "Let me correct that - we use Neo4j.",
			expected: 0.9,
		},
		{
			name:     "I meant correction",
			content:  "I meant to say we use TypeScript, not JavaScript.",
			expected: 0.9,
		},
		{
			name:     "no correction patterns",
			content:  "The user prefers using tabs for indentation.",
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := detector.detectCorrection(tt.content)
			if score != tt.expected {
				t.Errorf("expected %.2f, got %.2f", tt.expected, score)
			}
		})
	}
}

func TestContainsMixedCase(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"BlueSeer", true},
		{"camelCase", true},
		{"PascalCase", true},
		{"ALLCAPS", false},
		{"lowercase", false},
		{"snake_case", false}, // all lowercase with underscore
		{"API", false},
		{"myCustomClass", true},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := containsMixedCase(tt.word)
			if result != tt.expected {
				t.Errorf("word %q: expected %v, got %v", tt.word, tt.expected, result)
			}
		})
	}
}

func TestIsAcronym(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"API", true},
		{"REST", true},
		{"SDK", true},
		{"HTTP", true},
		{"BlueSeer", false},
		{"lowercase", false},
		{"A", false}, // too short
		{"AB", true},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := isAcronym(tt.word)
			if result != tt.expected {
				t.Errorf("word %q: expected %v, got %v", tt.word, tt.expected, result)
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		tolerance float64
	}{
		{
			name:      "identical vectors",
			a:         []float32{1.0, 0.0, 0.0},
			b:         []float32{1.0, 0.0, 0.0},
			expected:  1.0,
			tolerance: 0.001,
		},
		{
			name:      "orthogonal vectors",
			a:         []float32{1.0, 0.0},
			b:         []float32{0.0, 1.0},
			expected:  0.0,
			tolerance: 0.001,
		},
		{
			name:      "opposite vectors",
			a:         []float32{1.0, 0.0},
			b:         []float32{-1.0, 0.0},
			expected:  -1.0,
			tolerance: 0.001,
		},
		{
			name:      "different lengths",
			a:         []float32{1.0, 0.0},
			b:         []float32{1.0, 0.0, 0.0},
			expected:  0.0, // returns 0 for mismatched dimensions
			tolerance: 0.001,
		},
		{
			name:      "zero vector",
			a:         []float32{0.0, 0.0},
			b:         []float32{1.0, 0.0},
			expected:  0.0,
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.tolerance {
				t.Errorf("expected %.4f, got %.4f (diff: %.4f)", tt.expected, result, diff)
			}
		})
	}
}
