package conversation

import (
	"testing"
	"time"
)

func TestTokenEstimator(t *testing.T) {
	est := NewTokenEstimator()

	tests := []struct {
		name     string
		text     string
		minToken int
		maxToken int
	}{
		{"Empty", "", 0, 0},
		{"Short word", "hello", 1, 3},
		{"Sentence", "Hello, this is a test sentence.", 6, 10},
		{"Long text", "This is a much longer piece of text that should result in more tokens being estimated for the content.", 20, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := est.EstimateTokens(tt.text)
			if tokens < tt.minToken || tokens > tt.maxToken {
				t.Errorf("Expected tokens in [%d, %d], got %d", tt.minToken, tt.maxToken, tokens)
			}
		})
	}
}

func TestTokenEstimatorObservation(t *testing.T) {
	est := NewTokenEstimator()

	obs := &Observation{
		Content: "This is a test observation with some content that needs token estimation.",
		Summary: "Test observation",
		StructuredData: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}

	tokens := est.EstimateObservationTokens(obs)

	// Should include content + summary + structured data + overhead
	if tokens < 30 {
		t.Errorf("Expected at least 30 tokens, got %d", tokens)
	}
	if tokens > 100 {
		t.Errorf("Expected at most 100 tokens, got %d", tokens)
	}
}

func TestSmartTruncator_EmptyInput(t *testing.T) {
	truncator := NewSmartTruncator(1000)
	result := truncator.Truncate(nil)

	if len(result.Observations) != 0 {
		t.Errorf("Expected 0 observations, got %d", len(result.Observations))
	}
	if result.TokenCount != 0 {
		t.Errorf("Expected 0 tokens, got %d", result.TokenCount)
	}
}

func TestSmartTruncator_AllCritical(t *testing.T) {
	truncator := NewSmartTruncator(2000)

	scored := []ScoredObservation{
		{
			Observation:    &Observation{ObsID: "1", Content: "Critical observation 1", ObsType: ObsTypeCorrection},
			RelevanceScore: 0.95,
			Tier:           TierCritical,
		},
		{
			Observation:    &Observation{ObsID: "2", Content: "Critical observation 2", ObsType: ObsTypeError},
			RelevanceScore: 0.90,
			Tier:           TierCritical,
		},
	}

	result := truncator.Truncate(scored)

	if len(result.Observations) != 2 {
		t.Errorf("Expected 2 observations, got %d", len(result.Observations))
	}
	if result.TierCounts[TierCritical] != 2 {
		t.Errorf("Expected 2 critical, got %d", result.TierCounts[TierCritical])
	}
}

func TestSmartTruncator_TieredOutput(t *testing.T) {
	truncator := NewSmartTruncator(2000)

	scored := []ScoredObservation{
		{
			Observation:    &Observation{ObsID: "crit", Content: "Critical content", ObsType: ObsTypeCorrection},
			RelevanceScore: 0.95,
			Tier:           TierCritical,
		},
		{
			Observation:    &Observation{ObsID: "imp", Content: "Important content", ObsType: ObsTypeDecision},
			RelevanceScore: 0.7,
			Tier:           TierImportant,
		},
		{
			Observation:    &Observation{ObsID: "bg1", Content: "Background content 1", ObsType: ObsTypeLearning},
			RelevanceScore: 0.3,
			Tier:           TierBackground,
		},
		{
			Observation:    &Observation{ObsID: "bg2", Content: "Background content 2", ObsType: ObsTypeProgress},
			RelevanceScore: 0.2,
			Tier:           TierBackground,
		},
	}

	result := truncator.Truncate(scored)

	// Should have critical and important in observations
	if len(result.Observations) < 2 {
		t.Errorf("Expected at least 2 observations, got %d", len(result.Observations))
	}

	// Should have background summary
	if len(result.Summaries) == 0 {
		t.Errorf("Expected at least 1 summary for background")
	}

	// Verify tier counts
	if result.TierCounts[TierCritical] != 1 {
		t.Errorf("Expected 1 critical, got %d", result.TierCounts[TierCritical])
	}
	if result.TierCounts[TierImportant] != 1 {
		t.Errorf("Expected 1 important, got %d", result.TierCounts[TierImportant])
	}
}

func TestSmartTruncator_BudgetEnforcement(t *testing.T) {
	// Very small budget
	truncator := NewSmartTruncator(100)

	longContent := "This is a very long observation that contains a lot of text and should require truncation to fit within the token budget. It goes on and on with more and more content."

	scored := []ScoredObservation{
		{
			Observation:    &Observation{ObsID: "1", Content: longContent, ObsType: ObsTypeDecision},
			RelevanceScore: 0.85,
			Tier:           TierCritical,
		},
	}

	result := truncator.Truncate(scored)

	// Should still include something
	if len(result.Observations) == 0 {
		t.Error("Expected at least 1 observation even with small budget")
	}

	// Token count should be within budget
	if result.TokenCount > result.TokenBudget {
		t.Errorf("Token count %d exceeds budget %d", result.TokenCount, result.TokenBudget)
	}
}

func TestSmartTruncator_ImportantLimit(t *testing.T) {
	truncator := NewSmartTruncator(10000).WithImportantLimit(3)

	scored := make([]ScoredObservation, 0)
	for i := 0; i < 10; i++ {
		scored = append(scored, ScoredObservation{
			Observation:    &Observation{ObsID: "imp-" + string(rune('0'+i)), Content: "Important", ObsType: ObsTypeDecision},
			RelevanceScore: 0.7,
			Tier:           TierImportant,
		})
	}

	result := truncator.Truncate(scored)

	if result.TierCounts[TierImportant] > 3 {
		t.Errorf("Expected at most 3 important observations, got %d", result.TierCounts[TierImportant])
	}
}

func TestTruncateContent(t *testing.T) {
	truncator := NewSmartTruncator(1000)

	tests := []struct {
		name      string
		content   string
		maxTokens int
		wantEmpty bool
		wantShort bool
	}{
		{"Zero budget", "Hello world", 0, true, false},
		{"Small budget", "Hello world. This is a test.", 5, false, true},
		{"Large budget", "Hello", 100, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &Observation{Content: tt.content}
			result := truncator.truncateContent(obs, tt.maxTokens)

			if tt.wantEmpty && result != "" {
				t.Errorf("Expected empty, got %q", result)
			}
			if !tt.wantEmpty && result == "" {
				t.Error("Expected non-empty result")
			}
			if tt.wantShort && len(result) > len(tt.content) {
				t.Errorf("Expected shorter result, got %d chars (original: %d)", len(result), len(tt.content))
			}
		})
	}
}

func TestSummarizeBackground(t *testing.T) {
	truncator := NewSmartTruncator(1000)

	background := []ScoredObservation{
		{
			Observation:    &Observation{ObsID: "1", Content: "Learning about Go patterns", ObsType: ObsTypeLearning},
			RelevanceScore: 0.3,
			Tier:           TierBackground,
		},
		{
			Observation:    &Observation{ObsID: "2", Content: "Progress on feature X", ObsType: ObsTypeProgress},
			RelevanceScore: 0.2,
			Tier:           TierBackground,
		},
		{
			Observation:    &Observation{ObsID: "3", Content: "Another learning item", ObsType: ObsTypeLearning},
			RelevanceScore: 0.25,
			Tier:           TierBackground,
		},
	}

	summary := truncator.summarizeBackground(background, 200)

	if summary == nil {
		t.Fatal("Expected non-nil summary")
	}

	if summary.ObsID != "summary-background" {
		t.Errorf("Expected obs_id 'summary-background', got %q", summary.ObsID)
	}

	if !summary.Truncated {
		t.Error("Expected truncated=true for summary")
	}

	if len(summary.Summarizes) != 3 {
		t.Errorf("Expected 3 summarized IDs, got %d", len(summary.Summarizes))
	}

	// Content should mention count
	if summary.Content == "" {
		t.Error("Expected non-empty summary content")
	}
}

func TestExtractTopic(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{"Short topic", "Short topic"},
		{"This is a sentence. And another.", "This is a sentence"},
		{"One two three four five six seven eight nine", "One two three four five six..."},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractTopic(tt.content)
		if result != tt.expected {
			t.Errorf("extractTopic(%q) = %q, want %q", tt.content, result, tt.expected)
		}
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		count    int
		singular string
		expected string
	}{
		{0, "item", "0 items"},
		{1, "item", "1 item"},
		{2, "item", "2 items"},
		{10, "observation", "10 observations"},
	}

	for _, tt := range tests {
		result := pluralize(tt.count, tt.singular)
		if result != tt.expected {
			t.Errorf("pluralize(%d, %q) = %q, want %q", tt.count, tt.singular, result, tt.expected)
		}
	}
}

func TestTruncatedResumeBuilder(t *testing.T) {
	now := time.Now()
	builder := NewTruncatedResumeBuilder(2000)

	observations := []*Observation{
		{
			ObsID:     "1",
			ObsType:   ObsTypeCorrection,
			Content:   "Critical correction",
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ObsID:     "2",
			ObsType:   ObsTypeDecision,
			Content:   "Important decision",
			CreatedAt: now.Add(-2 * time.Hour),
		},
		{
			ObsID:     "3",
			ObsType:   ObsTypeLearning,
			Content:   "Background learning",
			CreatedAt: now.Add(-24 * time.Hour),
			Volatile:  true,
		},
	}

	result := builder.Build(observations)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if len(result.Observations) == 0 {
		t.Error("Expected at least 1 observation")
	}

	if result.TokenBudget != 2000 {
		t.Errorf("Expected budget 2000, got %d", result.TokenBudget)
	}

	if result.TokenCount > result.TokenBudget {
		t.Errorf("Token count %d exceeds budget %d", result.TokenCount, result.TokenBudget)
	}
}

func TestToEnhanced(t *testing.T) {
	now := time.Now()
	scored := ScoredObservation{
		Observation: &Observation{
			ObsID:      "test-123",
			ObsType:    ObsTypeDecision,
			Content:    "Test content",
			TemplateID: "decision",
			CreatedAt:  now,
			StructuredData: map[string]any{
				"key": "value",
			},
		},
		RelevanceScore: 0.85,
		Tier:           TierImportant,
	}

	enhanced := toEnhanced(scored, false)

	if enhanced.ObsID != "test-123" {
		t.Errorf("Expected obs_id 'test-123', got %q", enhanced.ObsID)
	}
	if enhanced.Tier != "important" {
		t.Errorf("Expected tier 'important', got %q", enhanced.Tier)
	}
	if enhanced.ObsType != "decision" {
		t.Errorf("Expected obs_type 'decision', got %q", enhanced.ObsType)
	}
	if enhanced.TemplateID != "decision" {
		t.Errorf("Expected template_id 'decision', got %q", enhanced.TemplateID)
	}
	if enhanced.RelevanceScore != 0.85 {
		t.Errorf("Expected relevance_score 0.85, got %.2f", enhanced.RelevanceScore)
	}
	if enhanced.Truncated {
		t.Error("Expected truncated=false")
	}

	// Test with truncated=true
	enhancedTrunc := toEnhanced(scored, true)
	if !enhancedTrunc.Truncated {
		t.Error("Expected truncated=true")
	}
}

// Note: min helper is tested in service_test.go
