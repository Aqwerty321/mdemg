package conversation

import (
	"testing"
)

func TestScoreObservationQuality_HighQuality(t *testing.T) {
	// A specific, actionable correction with context
	content := `User corrected: don't use sync.Mutex for the session tracker.
Use sync.Map instead because it's optimized for read-heavy workloads
with concurrent access. See internal/conversation/session_tracker.go.`
	tags := []string{"concurrency", "go", "session-tracker"}
	metadata := map[string]any{"file": "session_tracker.go", "decision": "sync.Map"}

	score := ScoreObservationQuality(content, "correction", tags, metadata)

	if score.Overall < QualityHighThreshold {
		t.Errorf("expected high quality (>= %.1f), got overall=%.3f (spec=%.3f act=%.3f ctx=%.3f)",
			QualityHighThreshold, score.Overall, score.Specificity, score.Actionability, score.ContextRich)
	}
}

func TestScoreObservationQuality_LowQuality(t *testing.T) {
	// Vague, unspecific, no tags or metadata
	content := "something happened"

	score := ScoreObservationQuality(content, "context", nil, nil)

	if !score.IsLowQuality() {
		t.Errorf("expected low quality (< %.1f), got overall=%.3f",
			QualityLowThreshold, score.Overall)
	}
}

func TestScoreObservationQuality_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		obsType  string
		tags     []string
		metadata map[string]any
		wantMin  float64
		wantMax  float64
	}{
		{
			name:    "empty content",
			content: "",
			obsType: "context",
			wantMin: 0.0,
			wantMax: 0.15,
		},
		{
			name:    "short vague progress",
			content: "made some progress on things",
			obsType: "progress",
			wantMin: 0.0,
			wantMax: 0.35,
		},
		{
			name:    "specific decision with code references",
			content: "Use Neo4j vector index memNodeEmbedding for similarity search. Set dimensions=1536 for OpenAI ada-002.",
			obsType: "decision",
			tags:    []string{"neo4j", "vectors"},
			wantMin: 0.45,
			wantMax: 1.0,
		},
		{
			name:    "correction with action",
			content: "Use -space-id flag, not -space flag for the ingest-codebase CLI tool",
			obsType: "correction",
			wantMin: 0.28,
			wantMax: 1.0,
		},
		{
			name:    "error with file path and details",
			content: "Build failed in internal/api/middleware.go:42 - undefined: SessionResumeWarningMiddleware. Need to implement the function before it can be referenced in server.go Routes().",
			obsType: "error",
			tags:    []string{"build-error", "middleware"},
			metadata: map[string]any{
				"file": "middleware.go",
				"line": 42,
			},
			wantMin: 0.55,
			wantMax: 1.0,
		},
		{
			name:    "learning with structured content",
			content: "MDEMG learning phases: cold(0) → learning(1-10k) → warm(10k-50k) → saturated(50k+). Edge count determines phase. Saturated phase needs pruning.",
			obsType: "learning",
			tags:    []string{"learning-system"},
			wantMin: 0.25,
			wantMax: 1.0,
		},
		{
			name:    "task with clear action",
			content: "Add SessionResumeWarningMiddleware to internal/api/middleware.go. Must check /v1/memory/retrieve and /v1/conversation/recall paths.",
			obsType: "task",
			wantMin: 0.45,
			wantMax: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ScoreObservationQuality(tt.content, tt.obsType, tt.tags, tt.metadata)
			if score.Overall < tt.wantMin || score.Overall > tt.wantMax {
				t.Errorf("Overall = %.3f, want [%.2f, %.2f] (spec=%.3f act=%.3f ctx=%.3f)",
					score.Overall, tt.wantMin, tt.wantMax,
					score.Specificity, score.Actionability, score.ContextRich)
			}
		})
	}
}

func TestScoreSpecificity(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantMin float64
		wantMax float64
	}{
		{"empty", "", 0.0, 0.01},
		{"very short", "ok", 0.0, 0.2},
		{"vague", "something stuff things maybe probably", 0.0, 0.15},
		{
			"code identifiers",
			"The SessionTracker uses sync.Map to store SessionState objects with TTL-based cleanup",
			0.3, 1.0,
		},
		{
			"file paths and numbers",
			"Found 1561 elements in internal/conversation/service.go with 15 functions",
			0.4, 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreSpecificity(tt.content)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("scoreSpecificity() = %.3f, want [%.2f, %.2f]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestScoreActionability(t *testing.T) {
	tests := []struct {
		name    string
		content string
		obsType string
		wantMin float64
	}{
		{"correction type", "Actually use sync.Map not Mutex", "correction", 0.5},
		{"decision type", "Decided to use gRPC for module communication", "decision", 0.4},
		{"error type", "Build failed: undefined variable", "error", 0.4},
		{"context type, no action", "The system runs on port 9999", "context", 0.1},
		{"imperative start", "Use the -space-id flag for ingest", "preference", 0.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreActionability(tt.content, tt.obsType)
			if score < tt.wantMin {
				t.Errorf("scoreActionability() = %.3f, want >= %.2f", score, tt.wantMin)
			}
		})
	}
}

func TestScoreContextRichness(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		tags     []string
		metadata map[string]any
		wantMin  float64
	}{
		{"no context", "basic observation", nil, nil, 0.0},
		{"tags only", "observation", []string{"tag1", "tag2"}, nil, 0.1},
		{"tags and metadata", "observation", []string{"a", "b", "c"}, map[string]any{"k": "v"}, 0.3},
		{
			"structured content with refs",
			"See internal/api/server.go for details.\nRelated to the middleware chain: compression -> warning -> logging",
			[]string{"api"},
			map[string]any{"file": "server.go", "phase": "3a", "task": "middleware"},
			0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreContextRichness(tt.content, tt.tags, tt.metadata)
			if score < tt.wantMin {
				t.Errorf("scoreContextRichness() = %.3f, want >= %.2f", score, tt.wantMin)
			}
		})
	}
}

func TestIsCodeIdentifier(t *testing.T) {
	tests := []struct {
		word string
		want bool
	}{
		{"SessionTracker", true},
		{"camelCase", true},
		{"sync.Map", true},        // dotted identifier (Go package.Type)
		{"snake_case", true},
		{"SCREAMING_CASE", true},
		{"internal.api.server", true},
		{"the", false},
		{"hello", false},
		{"A", false},
		{"session_tracker.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := isCodeIdentifier(tt.word)
			if got != tt.want {
				t.Errorf("isCodeIdentifier(%q) = %v, want %v", tt.word, got, tt.want)
			}
		})
	}
}

func TestQualityScore_IsLowQuality(t *testing.T) {
	low := QualityScore{Overall: 0.2}
	high := QualityScore{Overall: 0.8}

	if !low.IsLowQuality() {
		t.Error("expected 0.2 to be low quality")
	}
	if high.IsLowQuality() {
		t.Error("expected 0.8 to not be low quality")
	}
}
