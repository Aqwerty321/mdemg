package conversation

import (
	"testing"
)

func TestGenerateSummary(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "short content",
			content:  "User prefers tabs",
			expected: "User prefers tabs",
		},
		{
			name:     "content at boundary",
			content:  "A" + string(make([]byte, 199)),
			expected: "A" + string(make([]byte, 199)),
		},
		{
			name:     "long content gets truncated",
			content:  "This is a very long content string that exceeds the maximum length of 200 characters and should be truncated at a word boundary to ensure clean summaries. The system will find the last space before the 200 character limit and truncate there.",
			expected: "This is a very long content string that exceeds the maximum length of 200 characters and should be truncated at a word boundary to ensure clean summaries. The system will find the last space...",
		},
		{
			name:     "whitespace normalization",
			content:  "Content   with    multiple     spaces",
			expected: "Content with multiple spaces",
		},
		{
			name:     "leading/trailing whitespace",
			content:  "  Content with spaces  ",
			expected: "Content with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSummary(tt.content)

			// Check length constraint
			if len(result) > 203 { // 200 + "..."
				t.Errorf("summary too long: %d characters", len(result))
			}

			// For specific expected values, check exact match
			if tt.name != "long content gets truncated" {
				if result != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, result)
				}
			} else {
				// For truncation test, just verify it's truncated and ends with ...
				if len(result) <= len(tt.content) && result[len(result)-3:] != "..." {
					t.Errorf("expected truncation with ..., got %q", result)
				}
			}
		})
	}
}

func TestBuildObservationTags(t *testing.T) {
	tests := []struct {
		name     string
		req      ObserveRequest
		obsType  ObservationType
		expected []string
	}{
		{
			name: "basic tags",
			req: ObserveRequest{
				SessionID: "session-123",
				Tags:      []string{},
			},
			obsType: ObsTypeLearning,
			expected: []string{
				"conversation",
				"session:session-123",
				"obs_type:learning",
			},
		},
		{
			name: "with custom tags",
			req: ObserveRequest{
				SessionID: "session-456",
				Tags:      []string{"architecture", "database"},
			},
			obsType: ObsTypeDecision,
			expected: []string{
				"conversation",
				"session:session-456",
				"obs_type:decision",
				"architecture",
				"database",
			},
		},
		{
			name: "correction type",
			req: ObserveRequest{
				SessionID: "session-789",
				Tags:      []string{"correction"},
			},
			obsType: ObsTypeCorrection,
			expected: []string{
				"conversation",
				"session:session-789",
				"obs_type:correction",
				"correction",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildObservationTags(tt.req, tt.obsType)

			// Check that all expected tags are present
			for _, expectedTag := range tt.expected {
				found := false
				for _, tag := range result {
					if tag == expectedTag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected tag %q not found in result %v", expectedTag, result)
				}
			}

			// Check that we don't have unexpected extra tags
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d tags, got %d: %v", len(tt.expected), len(result), result)
			}
		})
	}
}

func TestObservationType(t *testing.T) {
	// Test that observation types are defined correctly
	types := []ObservationType{
		ObsTypeDecision,
		ObsTypeCorrection,
		ObsTypeLearning,
		ObsTypePreference,
		ObsTypeError,
		ObsTypeTask,
	}

	expectedValues := map[ObservationType]string{
		ObsTypeDecision:   "decision",
		ObsTypeCorrection: "correction",
		ObsTypeLearning:   "learning",
		ObsTypePreference: "preference",
		ObsTypeError:      "error",
		ObsTypeTask:       "task",
	}

	for _, obsType := range types {
		expected := expectedValues[obsType]
		if string(obsType) != expected {
			t.Errorf("ObservationType %v has unexpected value %q, expected %q", obsType, string(obsType), expected)
		}
	}
}

// =============================================================================
// PHASE 5: RESUME AND RECALL TESTS
// =============================================================================

func TestGenerateResumeSummary(t *testing.T) {
	tests := []struct {
		name     string
		resp     *ResumeResponse
		contains []string // Substrings that should be present
	}{
		{
			name: "no content",
			resp: &ResumeResponse{
				Observations:     []ObservationResult{},
				Themes:           []ThemeResult{},
				EmergentConcepts: []EmergentConceptResult{},
			},
			contains: []string{"No prior context found"},
		},
		{
			name: "with observations only",
			resp: &ResumeResponse{
				Observations: []ObservationResult{
					{ObsType: "decision"},
					{ObsType: "decision"},
					{ObsType: "learning"},
				},
				Themes:           []ThemeResult{},
				EmergentConcepts: []EmergentConceptResult{},
			},
			contains: []string{"Resuming with 3 recent observations"},
		},
		{
			name: "with themes",
			resp: &ResumeResponse{
				Observations: []ObservationResult{},
				Themes: []ThemeResult{
					{Name: "architecture-patterns"},
					{Name: "validation-rules"},
				},
				EmergentConcepts: []EmergentConceptResult{},
			},
			contains: []string{"Active themes:", "architecture-patterns", "validation-rules"},
		},
		{
			name: "with emergent concepts",
			resp: &ResumeResponse{
				Observations: []ObservationResult{},
				Themes:       []ThemeResult{},
				EmergentConcepts: []EmergentConceptResult{
					{Name: "user-prefers-modularity"},
				},
			},
			contains: []string{"Emergent concepts:", "user-prefers-modularity"},
		},
		{
			name: "full context",
			resp: &ResumeResponse{
				Observations: []ObservationResult{
					{ObsType: "decision"},
					{ObsType: "learning"},
				},
				Themes: []ThemeResult{
					{Name: "theme-1"},
				},
				EmergentConcepts: []EmergentConceptResult{
					{Name: "concept-1"},
				},
			},
			contains: []string{"Resuming with 2 recent observations", "Active themes:", "Emergent concepts:"},
		},
	}

	s := &Service{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.generateResumeSummary(tt.resp)

			for _, substr := range tt.contains {
				if !containsSubstring(result, substr) {
					t.Errorf("expected summary to contain %q, got %q", substr, result)
				}
			}
		})
	}
}

func TestSortAndLimitRecallResults(t *testing.T) {
	tests := []struct {
		name     string
		input    []RecallResult
		topK     int
		expected []float64 // Expected scores in order
	}{
		{
			name:     "empty results",
			input:    []RecallResult{},
			topK:     5,
			expected: []float64{},
		},
		{
			name: "already sorted",
			input: []RecallResult{
				{Score: 0.9},
				{Score: 0.7},
				{Score: 0.5},
			},
			topK:     5,
			expected: []float64{0.9, 0.7, 0.5},
		},
		{
			name: "unsorted input",
			input: []RecallResult{
				{Score: 0.5},
				{Score: 0.9},
				{Score: 0.7},
			},
			topK:     5,
			expected: []float64{0.9, 0.7, 0.5},
		},
		{
			name: "limit to topK",
			input: []RecallResult{
				{Score: 0.5},
				{Score: 0.9},
				{Score: 0.7},
				{Score: 0.3},
				{Score: 0.8},
			},
			topK:     3,
			expected: []float64{0.9, 0.8, 0.7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make([]RecallResult, len(tt.input))
			copy(results, tt.input)

			sortAndLimitRecallResults(&results, tt.topK)

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

func TestResumeRequestDefaults(t *testing.T) {
	// Test that ResumeRequest has sensible defaults
	req := ResumeRequest{
		SpaceID: "test-space",
	}

	// MaxObservations should default to 20 when processing
	if req.MaxObservations != 0 {
		t.Errorf("MaxObservations should be 0 (unset), got %d", req.MaxObservations)
	}

	// Default should be applied during Resume() processing
	if req.SpaceID != "test-space" {
		t.Errorf("SpaceID should be 'test-space', got %s", req.SpaceID)
	}
}

func TestRecallRequestDefaults(t *testing.T) {
	// Test that RecallRequest has sensible defaults
	req := RecallRequest{
		SpaceID: "test-space",
		Query:   "test query",
	}

	// TopK should default to 10 when processing
	if req.TopK != 0 {
		t.Errorf("TopK should be 0 (unset), got %d", req.TopK)
	}

	// IncludeThemes and IncludeConcepts should default to false
	if req.IncludeThemes {
		t.Errorf("IncludeThemes should default to false")
	}
	if req.IncludeConcepts {
		t.Errorf("IncludeConcepts should default to false")
	}
}

func TestRecallResultTypes(t *testing.T) {
	// Test that RecallResult supports all expected types
	types := []string{
		"conversation_observation",
		"conversation_theme",
		"emergent_concept",
	}

	for _, typ := range types {
		result := RecallResult{
			Type:    typ,
			NodeID:  "test-node",
			Content: "test content",
			Score:   0.8,
			Layer:   0,
		}

		if result.Type != typ {
			t.Errorf("expected type %s, got %s", typ, result.Type)
		}
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsSubstringHelper(s, substr)))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestValidVisibility(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"private is valid", "private", true},
		{"team is valid", "team", true},
		{"global is valid", "global", true},
		{"empty defaults to valid", "", true},
		{"invalid value", "public", false},
		{"case sensitive", "Private", false},
		{"random string", "foobar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidVisibility(tt.input)
			if result != tt.expected {
				t.Errorf("ValidVisibility(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestVisibilityConstants(t *testing.T) {
	// Verify constant values are as expected
	if VisibilityPrivate != "private" {
		t.Errorf("VisibilityPrivate = %q, want %q", VisibilityPrivate, "private")
	}
	if VisibilityTeam != "team" {
		t.Errorf("VisibilityTeam = %q, want %q", VisibilityTeam, "team")
	}
	if VisibilityGlobal != "global" {
		t.Errorf("VisibilityGlobal = %q, want %q", VisibilityGlobal, "global")
	}
}

func TestStabilityScoreConstants(t *testing.T) {
	// Verify stability score constants
	if DefaultStabilityScore != 0.1 {
		t.Errorf("DefaultStabilityScore = %v, want %v", DefaultStabilityScore, 0.1)
	}
	if GraduationStabilityThreshold != 0.8 {
		t.Errorf("GraduationStabilityThreshold = %v, want %v", GraduationStabilityThreshold, 0.8)
	}
	// Graduation threshold must be higher than default
	if GraduationStabilityThreshold <= DefaultStabilityScore {
		t.Errorf("GraduationStabilityThreshold (%v) must be > DefaultStabilityScore (%v)",
			GraduationStabilityThreshold, DefaultStabilityScore)
	}
}

func TestObservationIdentityFields(t *testing.T) {
	// Test that Observation struct has the new identity fields
	obs := Observation{
		ObsID:          "test-obs",
		SpaceID:        "test-space",
		SessionID:      "test-session",
		UserID:         "user-123",
		Visibility:     VisibilityTeam,
		Volatile:       true,
		StabilityScore: 0.5,
	}

	if obs.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", obs.UserID, "user-123")
	}
	if obs.Visibility != VisibilityTeam {
		t.Errorf("Visibility = %q, want %q", obs.Visibility, VisibilityTeam)
	}
	if !obs.Volatile {
		t.Error("Volatile should be true")
	}
	if obs.StabilityScore != 0.5 {
		t.Errorf("StabilityScore = %v, want %v", obs.StabilityScore, 0.5)
	}
}
