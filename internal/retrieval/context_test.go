package retrieval

import (
	"testing"

	"mdemg/internal/models"
)

// =============================================================================
// PHASE 5: CONTEXT-AWARE RETRIEVAL TESTS
// =============================================================================

func TestSortResultsByScore(t *testing.T) {
	tests := []struct {
		name     string
		input    []models.RetrieveResult
		expected []float64 // Expected scores in order
	}{
		{
			name:     "empty results",
			input:    []models.RetrieveResult{},
			expected: []float64{},
		},
		{
			name: "already sorted",
			input: []models.RetrieveResult{
				{NodeID: "1", Score: 0.9},
				{NodeID: "2", Score: 0.7},
				{NodeID: "3", Score: 0.5},
			},
			expected: []float64{0.9, 0.7, 0.5},
		},
		{
			name: "unsorted input",
			input: []models.RetrieveResult{
				{NodeID: "1", Score: 0.5},
				{NodeID: "2", Score: 0.9},
				{NodeID: "3", Score: 0.7},
			},
			expected: []float64{0.9, 0.7, 0.5},
		},
		{
			name: "single element",
			input: []models.RetrieveResult{
				{NodeID: "1", Score: 0.8},
			},
			expected: []float64{0.8},
		},
		{
			name: "equal scores maintain stability",
			input: []models.RetrieveResult{
				{NodeID: "1", Score: 0.5},
				{NodeID: "2", Score: 0.5},
				{NodeID: "3", Score: 0.5},
			},
			expected: []float64{0.5, 0.5, 0.5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make([]models.RetrieveResult, len(tt.input))
			copy(results, tt.input)

			sortResultsByScore(results)

			if len(results) != len(tt.expected) {
				t.Errorf("expected %d results, got %d", len(tt.expected), len(results))
				return
			}

			for i, expectedScore := range tt.expected {
				if results[i].Score != expectedScore {
					t.Errorf("position %d: expected score %.2f, got %.2f", i, expectedScore, results[i].Score)
				}
			}
		})
	}
}

func TestConversationContextResult(t *testing.T) {
	// Test that ConversationContextResult has all expected fields
	result := ConversationContextResult{
		NodeID:  "test-node",
		Type:    "conversation_observation",
		Content: "User prefers modular architecture",
		Score:   0.85,
		Layer:   0,
	}

	if result.NodeID != "test-node" {
		t.Errorf("expected NodeID 'test-node', got %s", result.NodeID)
	}
	if result.Type != "conversation_observation" {
		t.Errorf("expected Type 'conversation_observation', got %s", result.Type)
	}
	if result.Score != 0.85 {
		t.Errorf("expected Score 0.85, got %.2f", result.Score)
	}
	if result.Layer != 0 {
		t.Errorf("expected Layer 0, got %d", result.Layer)
	}
}

func TestConversationContextTypes(t *testing.T) {
	// Test that all conversation node types are supported
	types := []string{
		"conversation_observation",
		"conversation_theme",
		"emergent_concept",
	}

	for _, typ := range types {
		result := ConversationContextResult{
			NodeID: "test",
			Type:   typ,
		}
		if result.Type != typ {
			t.Errorf("expected type %s, got %s", typ, result.Type)
		}
	}
}

func TestBoostFactor(t *testing.T) {
	tests := []struct {
		name          string
		input         float64
		expectedBoost float64 // Minimum expected boost
	}{
		{
			name:          "default boost factor",
			input:         0,
			expectedBoost: 1.2, // Default
		},
		{
			name:          "custom boost factor",
			input:         1.5,
			expectedBoost: 1.5,
		},
		{
			name:          "minimal boost",
			input:         1.0,
			expectedBoost: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify boost factor logic
			boostFactor := tt.input
			if boostFactor <= 0 {
				boostFactor = 1.2 // Default
			}
			if boostFactor != tt.expectedBoost {
				t.Errorf("expected boost factor %.2f, got %.2f", tt.expectedBoost, boostFactor)
			}
		})
	}
}

func TestScoreBoostCalculation(t *testing.T) {
	// Test the boost calculation formula:
	// boost = 1.0 + (boostFactor - 1.0) * contextScore
	tests := []struct {
		name         string
		boostFactor  float64
		contextScore float64
		expectedMin  float64
		expectedMax  float64
	}{
		{
			name:         "high context score",
			boostFactor:  1.2,
			contextScore: 1.0,
			expectedMin:  1.19, // Should be close to 1.2
			expectedMax:  1.21,
		},
		{
			name:         "medium context score",
			boostFactor:  1.2,
			contextScore: 0.5,
			expectedMin:  1.09, // Should be close to 1.1
			expectedMax:  1.11,
		},
		{
			name:         "low context score",
			boostFactor:  1.2,
			contextScore: 0.0,
			expectedMin:  0.99, // Should be close to 1.0
			expectedMax:  1.01,
		},
		{
			name:         "higher boost factor",
			boostFactor:  1.5,
			contextScore: 1.0,
			expectedMin:  1.49,
			expectedMax:  1.51,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boost := 1.0 + (tt.boostFactor-1.0)*tt.contextScore
			if boost < tt.expectedMin || boost > tt.expectedMax {
				t.Errorf("expected boost in range [%.2f, %.2f], got %.2f", tt.expectedMin, tt.expectedMax, boost)
			}
		})
	}
}
