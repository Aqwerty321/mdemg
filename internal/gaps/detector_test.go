package gaps

import (
	"testing"
	"time"
)

func TestExtractKeyTerms(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "basic query",
			query:    "how do I configure the database connection?",
			expected: []string{"configure", "database", "connection"},
		},
		{
			name:     "removes stop words",
			query:    "what is the best way to do this",
			expected: []string{"best", "way"},
		},
		{
			name:     "handles punctuation",
			query:    "what's the timeout? is it 30 seconds",
			expected: []string{"what's", "timeout", "seconds"}, // "30" filtered out (length < 3)
		},
		{
			name:     "empty query",
			query:    "",
			expected: nil,
		},
		{
			name:     "all stop words",
			query:    "the a an is are",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKeyTerms(tt.query)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d terms, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, term := range tt.expected {
				if i < len(result) && result[i] != term {
					t.Errorf("term %d: expected %q, got %q", i, term, result[i])
				}
			}
		})
	}
}

func TestExtractDataSourceReferences(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "slack mention",
			text:     "check the slack channel for updates",
			expected: []string{"slack"},
		},
		{
			name:     "jira reference",
			text:     "see the jira ticket PROJ-123 for details",
			expected: []string{"jira"},
		},
		{
			name:     "github url",
			text:     "see https://github.com/org/repo/issues/42",
			expected: []string{"github"},
		},
		{
			name:     "multiple references",
			text:     "check slack channel and the jira issue",
			expected: []string{"slack", "jira"},
		},
		{
			name:     "confluence page",
			text:     "documented in the confluence page",
			expected: []string{"confluence"},
		},
		{
			name:     "no references",
			text:     "this is just plain text with no external references",
			expected: nil,
		},
		{
			name:     "notion mention",
			text:     "see the notion page for architecture",
			expected: []string{"notion"},
		},
		{
			name:     "pull request mention",
			text:     "reviewed in pull request #42",
			expected: []string{"github"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDataSourceReferences(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d refs, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			// Check that expected refs are present (order may vary)
			resultMap := make(map[string]bool)
			for _, r := range result {
				resultMap[r] = true
			}
			for _, exp := range tt.expected {
				if !resultMap[exp] {
					t.Errorf("expected ref %q not found in result %v", exp, result)
				}
			}
		})
	}
}

func TestSuggestIngestionPlugin(t *testing.T) {
	tests := []struct {
		source       string
		expectedName string
		expectedType PluginType
	}{
		{"slack", "slack-ingestion", PluginTypeIngestion},
		{"jira", "jira-ingestion", PluginTypeIngestion},
		{"confluence", "confluence-ingestion", PluginTypeIngestion},
		{"github", "github-ingestion", PluginTypeIngestion},
		{"notion", "notion-ingestion", PluginTypeIngestion},
		{"unknown_source", "unknown-source-ingestion", PluginTypeIngestion},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			suggestion := suggestIngestionPlugin(tt.source)
			if suggestion.Name != tt.expectedName {
				t.Errorf("expected name %q, got %q", tt.expectedName, suggestion.Name)
			}
			if suggestion.Type != tt.expectedType {
				t.Errorf("expected type %v, got %v", tt.expectedType, suggestion.Type)
			}
		})
	}
}

func TestCalculatePriority(t *testing.T) {
	tests := []struct {
		name        string
		occurrences int
		total       int64
		minExpected float64
		maxExpected float64
	}{
		{"zero total", 5, 0, 0.5, 0.5},
		{"low ratio", 1, 1000, 0.1, 0.1},
		{"medium ratio", 50, 1000, 0.5, 0.5},
		{"high ratio", 200, 1000, 1.0, 1.0},
		{"very high ratio", 500, 100, 1.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePriority(tt.occurrences, tt.total)
			if result < tt.minExpected || result > tt.maxExpected {
				t.Errorf("expected priority in [%.2f, %.2f], got %.2f", tt.minExpected, tt.maxExpected, result)
			}
		})
	}
}

func TestSanitizePluginName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"With Spaces", "with-spaces"},
		{"With-Dashes", "with-dashes"},
		{"With_Underscores", "with-underscores"},
		{"UPPERCASE", "uppercase"},
		{"Special!@#$%Chars", "special-chars"},
		{"", ""},
		{"a", "a"},
		{"ab", "ab"},
		{"abc", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizePluginName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestQueryMetrics_RecordQueryResult(t *testing.T) {
	cfg := DetectorConfig{
		LowScoreThreshold: 0.5,
		MetricsWindowSize: 10,
	}
	detector := NewGapDetector(nil, cfg)

	// Record some queries
	detector.RecordQueryResult("space1", "how to configure database", 0.8, 5)
	detector.RecordQueryResult("space1", "configure slack integration", 0.3, 2)
	detector.RecordQueryResult("space1", "what is slack", 0.2, 1)

	metrics := detector.GetMetrics()

	if metrics["total_queries"].(int64) != 3 {
		t.Errorf("expected 3 total queries, got %v", metrics["total_queries"])
	}

	if metrics["low_score_queries"].(int64) != 2 {
		t.Errorf("expected 2 low score queries, got %v", metrics["low_score_queries"])
	}
}

func TestQueryMetrics_WindowSize(t *testing.T) {
	cfg := DetectorConfig{
		LowScoreThreshold: 0.5,
		MetricsWindowSize: 3,
	}
	detector := NewGapDetector(nil, cfg)

	// Record more queries than window size
	for i := 0; i < 5; i++ {
		detector.RecordQueryResult("space1", "query", 0.8, 5)
	}

	metrics := detector.GetMetrics()

	historySize := metrics["history_size"].(int)
	if historySize != 3 {
		t.Errorf("expected history size 3, got %d", historySize)
	}
}

func TestCapabilityGap_Fields(t *testing.T) {
	now := time.Now()
	gap := CapabilityGap{
		ID:          "gap-12345",
		Type:        GapTypeDataSource,
		Description: "Missing Slack integration",
		Evidence:    []string{"slack channel", "slack message"},
		SuggestedPlugin: PluginSuggestion{
			Type:         PluginTypeIngestion,
			Name:         "slack-ingestion",
			Description:  "Ingest Slack messages",
			Capabilities: []string{"slack_messages"},
		},
		Priority:        0.85,
		DetectedAt:      now,
		UpdatedAt:       now,
		Status:          GapStatusOpen,
		OccurrenceCount: 10,
		SpaceID:         "test-space",
	}

	if gap.ID != "gap-12345" {
		t.Error("ID not set correctly")
	}
	if gap.Type != GapTypeDataSource {
		t.Error("Type not set correctly")
	}
	if gap.Status != GapStatusOpen {
		t.Error("Status not set correctly")
	}
	if gap.SuggestedPlugin.Type != PluginTypeIngestion {
		t.Error("SuggestedPlugin.Type not set correctly")
	}
	if len(gap.Evidence) != 2 {
		t.Error("Evidence not set correctly")
	}
}

func TestGapType_Values(t *testing.T) {
	if GapTypeDataSource != "data_source" {
		t.Errorf("GapTypeDataSource = %q, want %q", GapTypeDataSource, "data_source")
	}
	if GapTypeReasoning != "reasoning" {
		t.Errorf("GapTypeReasoning = %q, want %q", GapTypeReasoning, "reasoning")
	}
	if GapTypeQueryPattern != "query_pattern" {
		t.Errorf("GapTypeQueryPattern = %q, want %q", GapTypeQueryPattern, "query_pattern")
	}
}

func TestGapStatus_Values(t *testing.T) {
	if GapStatusOpen != "open" {
		t.Errorf("GapStatusOpen = %q, want %q", GapStatusOpen, "open")
	}
	if GapStatusAddressed != "addressed" {
		t.Errorf("GapStatusAddressed = %q, want %q", GapStatusAddressed, "addressed")
	}
	if GapStatusDismissed != "dismissed" {
		t.Errorf("GapStatusDismissed = %q, want %q", GapStatusDismissed, "dismissed")
	}
}

func TestPluginType_Values(t *testing.T) {
	if PluginTypeIngestion != "INGESTION" {
		t.Errorf("PluginTypeIngestion = %q, want %q", PluginTypeIngestion, "INGESTION")
	}
	if PluginTypeReasoning != "REASONING" {
		t.Errorf("PluginTypeReasoning = %q, want %q", PluginTypeReasoning, "REASONING")
	}
	if PluginTypeAPE != "APE" {
		t.Errorf("PluginTypeAPE = %q, want %q", PluginTypeAPE, "APE")
	}
}

func TestSortGapsByPriority(t *testing.T) {
	gaps := []CapabilityGap{
		{ID: "1", Priority: 0.3},
		{ID: "2", Priority: 0.9},
		{ID: "3", Priority: 0.5},
		{ID: "4", Priority: 0.1},
	}

	SortGapsByPriority(gaps)

	expectedOrder := []string{"2", "3", "1", "4"}
	for i, g := range gaps {
		if g.ID != expectedOrder[i] {
			t.Errorf("position %d: expected ID %s, got %s", i, expectedOrder[i], g.ID)
		}
	}
}

func TestDetectorConfig_Defaults(t *testing.T) {
	cfg := DetectorConfig{}
	detector := NewGapDetector(nil, cfg)

	if detector.lowScoreThreshold != 0.5 {
		t.Errorf("expected default lowScoreThreshold 0.5, got %f", detector.lowScoreThreshold)
	}
	if detector.minOccurrences != 3 {
		t.Errorf("expected default minOccurrences 3, got %d", detector.minOccurrences)
	}
	if detector.analysisWindow != 24*time.Hour {
		t.Errorf("expected default analysisWindow 24h, got %v", detector.analysisWindow)
	}
}

func TestRecordContentIngest(t *testing.T) {
	cfg := DetectorConfig{
		LowScoreThreshold: 0.5,
		MetricsWindowSize: 100,
	}
	detector := NewGapDetector(nil, cfg)

	// Ingest content with references
	detector.RecordContentIngest("space1", "Check the Slack channel for updates")
	detector.RecordContentIngest("space1", "See Jira ticket PROJ-123")
	detector.RecordContentIngest("space1", "No external references here")

	metrics := detector.GetMetrics()
	trackedSources := metrics["tracked_sources"].(int)

	// Should have tracked slack and jira
	if trackedSources < 2 {
		t.Errorf("expected at least 2 tracked sources, got %d", trackedSources)
	}
}

func TestIsRegisteredSource(t *testing.T) {
	cfg := DetectorConfig{
		RegisteredSources: []string{"slack", "github"},
	}
	detector := NewGapDetector(nil, cfg)

	if !detector.isRegisteredSource("slack") {
		t.Error("slack should be registered")
	}
	if !detector.isRegisteredSource("github") {
		t.Error("github should be registered")
	}
	if detector.isRegisteredSource("jira") {
		t.Error("jira should not be registered")
	}
	if detector.isRegisteredSource("confluence") {
		t.Error("confluence should not be registered")
	}
}
