package hidden

import (
	"math"
	"testing"
)

// =============================================================================
// PHASE 3: CONVERSATION HIDDEN LAYER TESTS
// =============================================================================

func TestConversationObservationStruct(t *testing.T) {
	obs := ConversationObservation{
		NodeID:        "obs-1",
		SpaceID:       "test-space",
		ObsType:       "learning",
		Content:       "User prefers plugin architecture",
		Summary:       "Prefers plugin architecture",
		Embedding:     []float64{0.1, 0.2, 0.3},
		SurpriseScore: 0.7,
		SessionID:     "session-123",
		Tags:          []string{"architecture", "preference"},
	}

	if obs.NodeID != "obs-1" {
		t.Errorf("ConversationObservation.NodeID = %s, want obs-1", obs.NodeID)
	}
	if obs.ObsType != "learning" {
		t.Errorf("ConversationObservation.ObsType = %s, want learning", obs.ObsType)
	}
	if obs.SurpriseScore != 0.7 {
		t.Errorf("ConversationObservation.SurpriseScore = %f, want 0.7", obs.SurpriseScore)
	}
}

func TestConversationThemeStruct(t *testing.T) {
	theme := ConversationTheme{
		NodeID:           "theme-1",
		SpaceID:          "test-space",
		Name:             "ConvTheme-architecture-0",
		Summary:          "Learning about plugin architecture (5 observations)",
		Embedding:        []float64{0.15, 0.25, 0.35},
		MemberCount:      5,
		DominantObsType:  "learning",
		AvgSurpriseScore: 0.65,
	}

	if theme.Name != "ConvTheme-architecture-0" {
		t.Errorf("ConversationTheme.Name = %s, want ConvTheme-architecture-0", theme.Name)
	}
	if theme.MemberCount != 5 {
		t.Errorf("ConversationTheme.MemberCount = %d, want 5", theme.MemberCount)
	}
	if theme.DominantObsType != "learning" {
		t.Errorf("ConversationTheme.DominantObsType = %s, want learning", theme.DominantObsType)
	}
}

func TestConversationThemeResultStruct(t *testing.T) {
	result := ConversationThemeResult{
		ThemesCreated:     3,
		EdgesCreated:      15,
		ThemeSummaries:    []string{"Theme 1", "Theme 2", "Theme 3"},
		ObservationsUsed:  20,
		NoiseObservations: 5,
	}

	if result.ThemesCreated != 3 {
		t.Errorf("ConversationThemeResult.ThemesCreated = %d, want 3", result.ThemesCreated)
	}
	if result.EdgesCreated != 15 {
		t.Errorf("ConversationThemeResult.EdgesCreated = %d, want 15", result.EdgesCreated)
	}
	if result.NoiseObservations != 5 {
		t.Errorf("ConversationThemeResult.NoiseObservations = %d, want 5", result.NoiseObservations)
	}
}

func TestGroupObservationsByCluster(t *testing.T) {
	observations := []ConversationObservation{
		{NodeID: "a", Embedding: []float64{1.0, 0.0}},
		{NodeID: "b", Embedding: []float64{0.0, 1.0}},
		{NodeID: "c", Embedding: []float64{1.0, 1.0}},
		{NodeID: "d", Embedding: []float64{0.0, 0.0}},
	}
	labels := []int{0, 0, 1, -1} // a,b in cluster 0; c in cluster 1; d is noise

	clusters, noise := groupObservationsByCluster(observations, labels)

	// Check cluster 0
	if len(clusters[0]) != 2 {
		t.Errorf("cluster 0 size = %d, want 2", len(clusters[0]))
	}

	// Check cluster 1
	if len(clusters[1]) != 1 {
		t.Errorf("cluster 1 size = %d, want 1", len(clusters[1]))
	}

	// Check noise
	if len(noise) != 1 {
		t.Errorf("noise size = %d, want 1", len(noise))
	}
	if noise[0].NodeID != "d" {
		t.Errorf("noise[0].NodeID = %s, want d", noise[0].NodeID)
	}
}

func TestExtractKeywordsFromObservations(t *testing.T) {
	observations := []ConversationObservation{
		{Content: "The user prefers plugin architecture for modularity", Summary: "Prefers plugin"},
		{Content: "Plugin-based systems allow better extensibility", Summary: "Plugin extensibility"},
		{Content: "Architecture should be modular with plugins", Summary: "Modular plugins"},
	}

	keywords := extractKeywordsFromObservations(observations)

	// Should find "plugin" as a top keyword (appears 3+ times)
	found := false
	for _, kw := range keywords {
		if kw == "plugin" || kw == "plugins" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'plugin' in keywords, got: %v", keywords)
	}

	// Should not include stop words
	for _, kw := range keywords {
		if kw == "the" || kw == "for" || kw == "with" {
			t.Errorf("Stop word '%s' should not be in keywords", kw)
		}
	}
}

func TestExtractKeywordsFromObservations_Empty(t *testing.T) {
	observations := []ConversationObservation{}

	keywords := extractKeywordsFromObservations(observations)

	if len(keywords) != 0 {
		t.Errorf("Expected empty keywords for empty observations, got: %v", keywords)
	}
}

func TestAnalyzeClusterMetadata(t *testing.T) {
	tests := []struct {
		name              string
		members           []ConversationObservation
		expectedType      string
		expectedSurprise  float64
		surpriseTolerance float64
	}{
		{
			name: "single type",
			members: []ConversationObservation{
				{ObsType: "learning", SurpriseScore: 0.5},
				{ObsType: "learning", SurpriseScore: 0.7},
				{ObsType: "learning", SurpriseScore: 0.6},
			},
			expectedType:      "learning",
			expectedSurprise:  0.6,
			surpriseTolerance: 0.001,
		},
		{
			name: "mixed types with dominant",
			members: []ConversationObservation{
				{ObsType: "decision", SurpriseScore: 0.8},
				{ObsType: "decision", SurpriseScore: 0.9},
				{ObsType: "learning", SurpriseScore: 0.3},
			},
			expectedType:      "decision",
			expectedSurprise:  2.0 / 3.0,
			surpriseTolerance: 0.001,
		},
		{
			name:              "empty members",
			members:           []ConversationObservation{},
			expectedType:      "unknown",
			expectedSurprise:  0.0,
			surpriseTolerance: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dominantType, avgSurprise := analyzeClusterMetadata(tt.members)

			if dominantType != tt.expectedType {
				t.Errorf("analyzeClusterMetadata() dominantType = %s, want %s", dominantType, tt.expectedType)
			}
			if math.Abs(avgSurprise-tt.expectedSurprise) > tt.surpriseTolerance {
				t.Errorf("analyzeClusterMetadata() avgSurprise = %f, want %f", avgSurprise, tt.expectedSurprise)
			}
		})
	}
}

func TestGenerateConversationThemeSummary(t *testing.T) {
	tests := []struct {
		name     string
		members  []ConversationObservation
		contains []string // Strings that should be present in summary
	}{
		{
			name: "learning observations about plugins",
			members: []ConversationObservation{
				{ObsType: "learning", Content: "User prefers plugin architecture", Summary: "Prefers plugins"},
				{ObsType: "learning", Content: "Plugin system is extensible", Summary: "Plugin extensibility"},
			},
			contains: []string{"Learning about", "(2 observations)"},
		},
		{
			name: "decision observations",
			members: []ConversationObservation{
				{ObsType: "decision", Content: "We decided to use React", Summary: "Use React"},
				{ObsType: "decision", Content: "React with TypeScript chosen", Summary: "TypeScript chosen"},
			},
			contains: []string{"Decisions about", "(2 observations)"},
		},
		{
			name:     "empty members",
			members:  []ConversationObservation{},
			contains: []string{"Empty conversation theme"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := generateConversationThemeSummary(tt.members)

			for _, expected := range tt.contains {
				if !containsString(summary, expected) {
					t.Errorf("generateConversationThemeSummary() summary = %q, should contain %q", summary, expected)
				}
			}
		})
	}
}

func TestSanitizeThemeName(t *testing.T) {
	tests := []struct {
		name     string
		summary  string
		expected string
	}{
		{
			name:     "simple words",
			summary:  "Learning about plugin architecture (5 observations)",
			expected: "plugin-architecture-5",
		},
		{
			name:     "skip type prefixes",
			summary:  "Decisions about database schema",
			expected: "database-schema",
		},
		{
			name:     "long summary truncated",
			summary:  "Learning about very long topic name that exceeds the maximum character limit allowed",
			expected: "very-long-topic", // Takes first 3 words after skip words, then truncates
		},
		{
			name:     "empty summary",
			summary:  "",
			expected: "misc",
		},
		{
			name:     "only skip words but treated as content",
			summary:  "about regarding for",
			expected: "about-regarding-for", // Skip word matching is case-sensitive and partial
		},
		{
			name:     "special characters removed",
			summary:  "Learning about API: endpoint/design",
			expected: "api-endpointdesign",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeThemeName(tt.summary)
			if result != tt.expected {
				t.Errorf("sanitizeThemeName(%q) = %q, want %q", tt.summary, result, tt.expected)
			}
		})
	}
}

func TestAsStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{"nil", nil, nil},
		{
			"any slice with strings",
			[]any{"a", "b", "c"},
			[]string{"a", "b", "c"},
		},
		{
			"string slice",
			[]string{"x", "y", "z"},
			[]string{"x", "y", "z"},
		},
		{
			"any slice with mixed (only strings extracted)",
			[]any{"str", 123, "another"},
			[]string{"str", "another"},
		},
		{
			"empty any slice",
			[]any{},
			[]string{},
		},
		{"unsupported type", "not a slice", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asStringSlice(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("asStringSlice(%v) = %v, want nil", tt.input, result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("asStringSlice(%v) length = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i := range tt.expected {
				if result[i] != tt.expected[i] {
					t.Errorf("asStringSlice(%v)[%d] = %v, want %v", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// TestObservationClusteringSeparation verifies that conversation observations
// cluster separately from code files (they use different queries)
func TestObservationClusteringSeparation(t *testing.T) {
	// This is a structural test - verify the types are distinct
	// Code files use BaseNode with role_type not set
	// Conversations use ConversationObservation with role_type='conversation_observation'

	codeNode := BaseNode{
		NodeID:    "code-1",
		SpaceID:   "test",
		Path:      "/src/main.go",
		Embedding: []float64{1.0, 0.0},
	}

	convObs := ConversationObservation{
		NodeID:    "obs-1",
		SpaceID:   "test",
		ObsType:   "learning",
		Content:   "Test content",
		Embedding: []float64{0.0, 1.0},
	}

	// Verify distinct types
	if codeNode.NodeID == convObs.NodeID {
		t.Error("Code and conversation nodes should have distinct identities")
	}

	// Verify observation has ObsType field
	if convObs.ObsType == "" {
		t.Error("ConversationObservation should have ObsType field")
	}

	// Verify base node has Path field (used for code grouping)
	if codeNode.Path == "" {
		t.Error("BaseNode should have Path field for code grouping")
	}
}

// TestConversationDBSCAN verifies DBSCAN works correctly with conversation embeddings
func TestConversationDBSCAN(t *testing.T) {
	// Create observations with distinct clusters
	// Cluster 1: Similar embeddings (plugin-related)
	// Cluster 2: Similar embeddings (database-related)
	// Noise: Isolated point

	observations := []ConversationObservation{
		// Cluster 1 - similar embeddings
		{NodeID: "obs1", Embedding: []float64{1.0, 0.0, 0.0}},
		{NodeID: "obs2", Embedding: []float64{0.99, 0.1, 0.0}},
		{NodeID: "obs3", Embedding: []float64{0.98, 0.15, 0.0}},

		// Cluster 2 - similar embeddings but different direction
		{NodeID: "obs4", Embedding: []float64{0.0, 1.0, 0.0}},
		{NodeID: "obs5", Embedding: []float64{0.1, 0.99, 0.0}},
		{NodeID: "obs6", Embedding: []float64{0.15, 0.98, 0.0}},

		// Noise - isolated
		{NodeID: "obs7", Embedding: []float64{0.0, 0.0, 1.0}},
	}

	// Extract embeddings
	embeddings := make([][]float64, len(observations))
	for i, obs := range observations {
		embeddings[i] = obs.Embedding
	}

	// Run DBSCAN with reasonable parameters
	labels := DBSCAN(embeddings, 0.2, 2)

	// Group by cluster
	clusters, noise := groupObservationsByCluster(observations, labels)

	// Should have exactly 2 clusters
	if len(clusters) != 2 {
		t.Errorf("Expected 2 clusters, got %d", len(clusters))
	}

	// Should have 1 noise point
	if len(noise) != 1 {
		t.Errorf("Expected 1 noise point, got %d", len(noise))
	}

	// Verify cluster sizes
	totalClustered := 0
	for _, members := range clusters {
		totalClustered += len(members)
		// Each cluster should have 3 members
		if len(members) != 3 {
			t.Errorf("Expected cluster size 3, got %d", len(members))
		}
	}

	if totalClustered != 6 {
		t.Errorf("Expected 6 clustered observations, got %d", totalClustered)
	}
}

// Helper function for string containment check
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
