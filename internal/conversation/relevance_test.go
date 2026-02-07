package conversation

import (
	"math"
	"testing"
	"time"
)

func TestDefaultRelevanceWeights(t *testing.T) {
	weights := DefaultRelevanceWeights()

	// Weights should sum to approximately 1.0
	sum := weights.Recency + weights.Importance + weights.TaskRelevance
	if math.Abs(sum-1.0) > 0.001 {
		t.Errorf("Default weights should sum to 1.0, got %.3f", sum)
	}

	if weights.Recency != 0.3 {
		t.Errorf("Expected recency weight 0.3, got %.2f", weights.Recency)
	}
	if weights.Importance != 0.4 {
		t.Errorf("Expected importance weight 0.4, got %.2f", weights.Importance)
	}
	if weights.TaskRelevance != 0.3 {
		t.Errorf("Expected task_relevance weight 0.3, got %.2f", weights.TaskRelevance)
	}
}

func TestRecencyScoring(t *testing.T) {
	now := time.Now()
	scorer := NewRelevanceScorer().WithReferenceTime(now)

	tests := []struct {
		name     string
		age      time.Duration
		minScore float64
		maxScore float64
	}{
		{"Just now", 0, 0.99, 1.0},
		{"1 hour ago", 1 * time.Hour, 0.9, 0.99},
		{"12 hours ago", 12 * time.Hour, 0.6, 0.8},
		{"24 hours ago (half-life)", 24 * time.Hour, 0.45, 0.55},
		{"48 hours ago", 48 * time.Hour, 0.2, 0.3},
		{"1 week ago", 7 * 24 * time.Hour, 0.0, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &Observation{
				CreatedAt: now.Add(-tt.age),
			}
			score := scorer.scoreRecency(obs)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("For %s, expected score in [%.2f, %.2f], got %.4f",
					tt.name, tt.minScore, tt.maxScore, score)
			}
		})
	}
}

func TestImportanceScoring(t *testing.T) {
	scorer := NewRelevanceScorer()

	tests := []struct {
		name     string
		obs      *Observation
		minScore float64
		maxScore float64
	}{
		{
			name:     "Correction type",
			obs:      &Observation{ObsType: ObsTypeCorrection},
			minScore: 0.85,
			maxScore: 1.0,
		},
		{
			name:     "Decision type",
			obs:      &Observation{ObsType: ObsTypeDecision},
			minScore: 0.75,
			maxScore: 0.9,
		},
		{
			name:     "Learning type",
			obs:      &Observation{ObsType: ObsTypeLearning},
			minScore: 0.65,
			maxScore: 0.8,
		},
		{
			name:     "High surprise boost",
			obs:      &Observation{ObsType: ObsTypeLearning, SurpriseScore: 1.0},
			minScore: 0.85,
			maxScore: 1.0,
		},
		{
			name:     "Volatile penalty",
			obs:      &Observation{ObsType: ObsTypeDecision, Volatile: true},
			minScore: 0.55,
			maxScore: 0.7,
		},
		{
			name:     "Stored importance",
			obs:      &Observation{ImportanceScore: 0.95},
			minScore: 0.9,
			maxScore: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scorer.scoreImportance(tt.obs)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("Expected score in [%.2f, %.2f], got %.4f",
					tt.minScore, tt.maxScore, score)
			}
		})
	}
}

func TestTaskRelevanceHeuristic(t *testing.T) {
	scorer := NewRelevanceScorer()

	tests := []struct {
		name       string
		templateID string
		tier       string
		minScore   float64
		maxScore   float64
	}{
		{"task_handoff template", "task_handoff", "", 0.9, 1.0},
		{"decision template", "decision", "", 0.8, 0.9},
		{"correction template", "correction", "", 0.85, 0.95},
		{"critical tier", "", "critical", 0.85, 1.0},
		{"important tier", "", "important", 0.65, 0.8},
		{"background tier", "", "background", 0.0, 0.55},
		{"no template or tier", "", "", 0.45, 0.55},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &Observation{
				TemplateID: tt.templateID,
				Tier:       tt.tier,
			}
			score := scorer.scoreTaskRelevance(obs)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("Expected score in [%.2f, %.2f], got %.4f",
					tt.minScore, tt.maxScore, score)
			}
		})
	}
}

// Note: TestCosineSimilarity is defined in surprise_test.go

func TestOverallScoring(t *testing.T) {
	now := time.Now()
	scorer := NewRelevanceScorer().WithReferenceTime(now)

	// High scoring observation: recent, important, relevant
	highObs := &Observation{
		ObsType:         ObsTypeCorrection,
		CreatedAt:       now.Add(-1 * time.Hour),
		SurpriseScore:   0.8,
		StabilityScore:  0.9,
		TemplateID:      "correction",
		Tier:            "critical",
		ImportanceScore: 0.9,
	}

	// Low scoring observation: old, less important
	lowObs := &Observation{
		ObsType:   ObsTypeProgress,
		CreatedAt: now.Add(-7 * 24 * time.Hour),
		Volatile:  true,
		Tier:      "background",
	}

	highScore := scorer.Score(highObs)
	lowScore := scorer.Score(lowObs)

	if highScore <= lowScore {
		t.Errorf("High priority obs should score higher: high=%.3f, low=%.3f", highScore, lowScore)
	}

	if highScore < 0.7 {
		t.Errorf("High priority obs should have high score, got %.3f", highScore)
	}

	if lowScore > 0.4 {
		t.Errorf("Low priority obs should have low score, got %.3f", lowScore)
	}
}

func TestTierClassification(t *testing.T) {
	tests := []struct {
		score    float64
		expected Tier
	}{
		{0.95, TierCritical},
		{0.80, TierCritical},
		{0.79, TierImportant},
		{0.50, TierImportant},
		{0.49, TierBackground},
		{0.10, TierBackground},
	}

	for _, tt := range tests {
		tier := ClassifyTier(tt.score)
		if tier != tt.expected {
			t.Errorf("Score %.2f: expected tier %q, got %q", tt.score, tt.expected, tier)
		}
	}
}

func TestTierFromType(t *testing.T) {
	tests := []struct {
		obsType  ObservationType
		expected Tier
	}{
		{ObsTypeCorrection, TierCritical},
		{ObsTypeError, TierCritical},
		{ObsTypeBlocker, TierCritical},
		{ObsTypeDecision, TierImportant},
		{ObsTypeTask, TierImportant},
		{ObsTypeInsight, TierImportant},
		{ObsTypeLearning, TierBackground},
		{ObsTypeProgress, TierBackground},
	}

	for _, tt := range tests {
		tier := TierFromType(tt.obsType)
		if tier != tt.expected {
			t.Errorf("Type %q: expected tier %q, got %q", tt.obsType, tt.expected, tier)
		}
	}
}

func TestScoreObservationsBatch(t *testing.T) {
	now := time.Now()
	scorer := NewRelevanceScorer().WithReferenceTime(now)

	observations := []*Observation{
		{ObsID: "low", ObsType: ObsTypeProgress, CreatedAt: now.Add(-48 * time.Hour), Volatile: true},
		{ObsID: "high", ObsType: ObsTypeCorrection, CreatedAt: now.Add(-1 * time.Hour), SurpriseScore: 0.9},
		{ObsID: "mid", ObsType: ObsTypeDecision, CreatedAt: now.Add(-12 * time.Hour)},
	}

	scored := scorer.ScoreObservations(observations)

	if len(scored) != 3 {
		t.Fatalf("Expected 3 scored observations, got %d", len(scored))
	}

	// Should be sorted by score descending
	if scored[0].Observation.ObsID != "high" {
		t.Errorf("Expected 'high' to be first, got %q", scored[0].Observation.ObsID)
	}
	if scored[2].Observation.ObsID != "low" {
		t.Errorf("Expected 'low' to be last, got %q", scored[2].Observation.ObsID)
	}

	// Scores should be in descending order
	for i := 0; i < len(scored)-1; i++ {
		if scored[i].RelevanceScore < scored[i+1].RelevanceScore {
			t.Errorf("Scores not in descending order at index %d", i)
		}
	}
}

func TestCustomWeights(t *testing.T) {
	now := time.Now()

	// Create obs that would score high on recency but low on importance
	obs := &Observation{
		ObsType:   ObsTypeProgress,
		CreatedAt: now.Add(-1 * time.Minute),
		Volatile:  true,
	}

	// Weight recency heavily
	recencyScorer := NewRelevanceScorer().
		WithReferenceTime(now).
		WithWeights(&RelevanceWeights{Recency: 0.9, Importance: 0.05, TaskRelevance: 0.05})

	// Weight importance heavily
	importanceScorer := NewRelevanceScorer().
		WithReferenceTime(now).
		WithWeights(&RelevanceWeights{Recency: 0.05, Importance: 0.9, TaskRelevance: 0.05})

	recencyScore := recencyScorer.Score(obs)
	importanceScore := importanceScorer.Score(obs)

	// With recency weight high, very recent obs should score higher
	if recencyScore <= importanceScore {
		t.Errorf("With high recency weight, recent obs should score higher: recency=%.3f, importance=%.3f",
			recencyScore, importanceScore)
	}
}

func TestResumeStrategy(t *testing.T) {
	if ResumeTaskFocused != "task_focused" {
		t.Errorf("Expected task_focused, got %q", ResumeTaskFocused)
	}
	if ResumeComprehensive != "comprehensive" {
		t.Errorf("Expected comprehensive, got %q", ResumeComprehensive)
	}
	if ResumeMinimal != "minimal" {
		t.Errorf("Expected minimal, got %q", ResumeMinimal)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		value    float64
		min      float64
		max      float64
		expected float64
	}{
		{0.5, 0.0, 1.0, 0.5},
		{-0.5, 0.0, 1.0, 0.0},
		{1.5, 0.0, 1.0, 1.0},
		{0.0, 0.0, 1.0, 0.0},
		{1.0, 0.0, 1.0, 1.0},
	}

	for _, tt := range tests {
		result := clamp(tt.value, tt.min, tt.max)
		if result != tt.expected {
			t.Errorf("clamp(%.1f, %.1f, %.1f) = %.1f, want %.1f",
				tt.value, tt.min, tt.max, result, tt.expected)
		}
	}
}
