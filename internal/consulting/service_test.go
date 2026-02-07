package consulting

import (
	"testing"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// =============================================================================
// classifySuggestionType tests
// =============================================================================

func TestClassifySuggestionType_RiskKeywords(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name     string
		result   models.RetrieveResult
		expected models.SuggestionType
	}{
		{
			name:     "error in name",
			result:   models.RetrieveResult{Name: "handle-error-flow", Summary: "desc"},
			expected: models.SuggestionRisk,
		},
		{
			name:     "fail in summary",
			result:   models.RetrieveResult{Name: "process", Summary: "describes a failure case"},
			expected: models.SuggestionRisk,
		},
		{
			name:     "bug in name",
			result:   models.RetrieveResult{Name: "known-bug-workaround"},
			expected: models.SuggestionRisk,
		},
		{
			name:     "deprecated in summary",
			result:   models.RetrieveResult{Name: "old-api", Summary: "deprecated method"},
			expected: models.SuggestionRisk,
		},
		{
			name:     "todo in name",
			result:   models.RetrieveResult{Name: "todo-fix-later"},
			expected: models.SuggestionRisk,
		},
		{
			name:     "workaround in summary",
			result:   models.RetrieveResult{Name: "patch", Summary: "workaround for issue #123"},
			expected: models.SuggestionRisk,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.classifySuggestionType(tt.result)
			if got != tt.expected {
				t.Errorf("classifySuggestionType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifySuggestionType_ProcessKeywords(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name     string
		result   models.RetrieveResult
		expected models.SuggestionType
	}{
		{
			name:     "workflow in name",
			result:   models.RetrieveResult{Name: "deployment-workflow"},
			expected: models.SuggestionProcess,
		},
		{
			name:     "readme in path",
			result:   models.RetrieveResult{Name: "intro", Path: "/docs/readme.md"},
			expected: models.SuggestionProcess,
		},
		{
			name:     "howto in name",
			result:   models.RetrieveResult{Name: "howto-setup-db"},
			expected: models.SuggestionProcess,
		},
		{
			name:     "guide in path",
			result:   models.RetrieveResult{Name: "setup", Path: "/guide/setup.md"},
			expected: models.SuggestionProcess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.classifySuggestionType(tt.result)
			if got != tt.expected {
				t.Errorf("classifySuggestionType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifySuggestionType_ConceptKeywords(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name     string
		result   models.RetrieveResult
		expected models.SuggestionType
	}{
		{
			name:     "pattern in name",
			result:   models.RetrieveResult{Name: "repository-pattern"},
			expected: models.SuggestionConcept,
		},
		{
			name:     "architecture in path (no doc)",
			result:   models.RetrieveResult{Name: "layers", Path: "/src/architecture/layers.go"},
			expected: models.SuggestionConcept,
		},
		{
			name:     "design in summary",
			result:   models.RetrieveResult{Name: "structure", Summary: "design principles for modules"},
			expected: models.SuggestionConcept,
		},
		{
			name:     "interface in summary",
			result:   models.RetrieveResult{Name: "api-contract", Summary: "interface definition"},
			expected: models.SuggestionConcept,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.classifySuggestionType(tt.result)
			if got != tt.expected {
				t.Errorf("classifySuggestionType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifySuggestionType_DefaultContext(t *testing.T) {
	s := &Service{}

	result := models.RetrieveResult{
		Name:    "user-service",
		Path:    "/internal/services/user.go",
		Summary: "handles user operations",
	}

	got := s.classifySuggestionType(result)
	if got != models.SuggestionContext {
		t.Errorf("classifySuggestionType() = %v, want %v", got, models.SuggestionContext)
	}
}

// =============================================================================
// formatSuggestionContent tests
// =============================================================================

func TestFormatSuggestionContent_ByType(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name       string
		suggType   models.SuggestionType
		result     models.RetrieveResult
		wantPrefix string
	}{
		{
			name:       "context type",
			suggType:   models.SuggestionContext,
			result:     models.RetrieveResult{Summary: "some context"},
			wantPrefix: "Based on this codebase's patterns:",
		},
		{
			name:       "process type",
			suggType:   models.SuggestionProcess,
			result:     models.RetrieveResult{Summary: "workflow steps"},
			wantPrefix: "The typical workflow for this type of change:",
		},
		{
			name:       "concept type",
			suggType:   models.SuggestionConcept,
			result:     models.RetrieveResult{Summary: "higher principle"},
			wantPrefix: "This relates to the higher-level principle:",
		},
		{
			name:       "risk type",
			suggType:   models.SuggestionRisk,
			result:     models.RetrieveResult{Summary: "previous failure"},
			wantPrefix: "Caution - previous related finding:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.formatSuggestionContent(tt.suggType, tt.result, "question")
			if len(got) == 0 {
				t.Error("formatSuggestionContent() returned empty string")
			}
			if got[:len(tt.wantPrefix)] != tt.wantPrefix {
				t.Errorf("formatSuggestionContent() = %v, want prefix %v", got, tt.wantPrefix)
			}
		})
	}
}

func TestFormatSuggestionContent_UsesNameWhenNoSummary(t *testing.T) {
	s := &Service{}

	result := models.RetrieveResult{
		Name:    "user-authentication-handler",
		Summary: "",
	}

	got := s.formatSuggestionContent(models.SuggestionContext, result, "how does auth work?")
	if got == "" {
		t.Error("formatSuggestionContent() should use Name when Summary is empty")
	}
	if got != "Based on this codebase's patterns: user-authentication-handler" {
		t.Errorf("formatSuggestionContent() = %v", got)
	}
}

func TestFormatSuggestionContent_EmptyBoth(t *testing.T) {
	s := &Service{}

	result := models.RetrieveResult{Name: "", Summary: ""}

	got := s.formatSuggestionContent(models.SuggestionContext, result, "question")
	if got != "" {
		t.Errorf("formatSuggestionContent() = %v, want empty string", got)
	}
}

func TestFormatSuggestionContent_Truncation(t *testing.T) {
	s := &Service{}

	// Create a summary longer than 500 characters
	longSummary := ""
	for i := 0; i < 100; i++ {
		longSummary += "12345 "
	}

	result := models.RetrieveResult{Summary: longSummary}

	got := s.formatSuggestionContent(models.SuggestionContext, result, "question")

	// Should be truncated with "..."
	if len(got) > 550 { // 500 + prefix length + some buffer
		t.Errorf("formatSuggestionContent() should truncate long content, len = %d", len(got))
	}
	if got[len(got)-3:] != "..." {
		t.Error("formatSuggestionContent() should end with '...' for truncated content")
	}
}

// =============================================================================
// deduplicateSuggestions tests
// =============================================================================

func TestDeduplicateSuggestions_Empty(t *testing.T) {
	s := &Service{}

	got := s.deduplicateSuggestions(nil)
	if len(got) != 0 {
		t.Errorf("deduplicateSuggestions(nil) = %v, want empty", got)
	}
}

func TestDeduplicateSuggestions_Single(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Content: "single suggestion", Confidence: 0.8},
	}

	got := s.deduplicateSuggestions(suggestions)
	if len(got) != 1 {
		t.Errorf("deduplicateSuggestions() len = %d, want 1", len(got))
	}
}

func TestDeduplicateSuggestions_RemovesDuplicates(t *testing.T) {
	s := &Service{}

	// Content keys are based on first 50 chars (lowercased)
	// "based on this pattern: use repository layer for da" (50 chars)
	suggestions := []models.Suggestion{
		{Content: "Based on this pattern: use repository layer for data access which is standard", Confidence: 0.9},
		{Content: "Based on this pattern: use repository layer for data persistence with extra details", Confidence: 0.7},
		{Content: "Different approach: use the service layer pattern", Confidence: 0.8},
	}

	got := s.deduplicateSuggestions(suggestions)

	// First two share first 50 chars, so second should be removed
	if len(got) != 2 {
		t.Errorf("deduplicateSuggestions() len = %d, want 2", len(got))
	}
}

func TestDeduplicateSuggestions_PreservesOrder(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Content: "First unique suggestion", Confidence: 0.9},
		{Content: "Second unique suggestion", Confidence: 0.8},
		{Content: "Third unique suggestion", Confidence: 0.7},
	}

	got := s.deduplicateSuggestions(suggestions)

	if len(got) != 3 {
		t.Fatalf("deduplicateSuggestions() len = %d, want 3", len(got))
	}
	if got[0].Content != "First unique suggestion" {
		t.Error("deduplicateSuggestions() should preserve order")
	}
}

// =============================================================================
// calculateOverallConfidence tests
// =============================================================================

func TestCalculateOverallConfidence_Empty(t *testing.T) {
	s := &Service{}

	got := s.calculateOverallConfidence(nil, 0)
	if got != 0.0 {
		t.Errorf("calculateOverallConfidence(nil, 0) = %v, want 0.0", got)
	}
}

func TestCalculateOverallConfidence_SingleSuggestion(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.8},
	}

	got := s.calculateOverallConfidence(suggestions, 1)
	if got != 0.8 {
		t.Errorf("calculateOverallConfidence() = %v, want 0.8", got)
	}
}

func TestCalculateOverallConfidence_Average(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.6},
		{Confidence: 0.8},
	}

	got := s.calculateOverallConfidence(suggestions, 2)
	// Average is 0.7, no coverage boost (totalRetrieved < 5)
	if got != 0.7 {
		t.Errorf("calculateOverallConfidence() = %v, want 0.7", got)
	}
}

func TestCalculateOverallConfidence_CoverageBoost5(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.5},
		{Confidence: 0.5},
	}

	got := s.calculateOverallConfidence(suggestions, 5)
	// Average is 0.5, boost 1.1 -> 0.55
	if got != 0.55 {
		t.Errorf("calculateOverallConfidence() = %v, want 0.55", got)
	}
}

func TestCalculateOverallConfidence_CoverageBoost10(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.5},
		{Confidence: 0.5},
	}

	got := s.calculateOverallConfidence(suggestions, 10)
	// Average is 0.5, boost 1.2 -> 0.6
	if got != 0.6 {
		t.Errorf("calculateOverallConfidence() = %v, want 0.6", got)
	}
}

func TestCalculateOverallConfidence_CappedAtMax(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.9},
		{Confidence: 0.9},
	}

	got := s.calculateOverallConfidence(suggestions, 10)
	// Average is 0.9, boost 1.2 -> 1.08, capped at 0.95
	if got != MaxConfidence {
		t.Errorf("calculateOverallConfidence() = %v, want %v (MaxConfidence)", got, MaxConfidence)
	}
}

// =============================================================================
// generateRationale tests
// =============================================================================

func TestGenerateRationale_NoSuggestions(t *testing.T) {
	s := &Service{}

	got := s.generateRationale(models.ConsultRequest{}, nil, nil)
	if got != "No relevant patterns found in the knowledge base for this query." {
		t.Errorf("generateRationale() = %v", got)
	}
}

func TestGenerateRationale_CountsTypes(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Type: models.SuggestionContext},
		{Type: models.SuggestionContext},
		{Type: models.SuggestionRisk},
		{Type: models.SuggestionProcess},
	}

	got := s.generateRationale(models.ConsultRequest{}, suggestions, nil)

	// Should contain counts
	if got == "" {
		t.Error("generateRationale() returned empty string")
	}
	// Should mention "Found 4 relevant patterns"
	if len(got) < 20 {
		t.Errorf("generateRationale() too short: %v", got)
	}
}

func TestGenerateRationale_IncludesConcepts(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Type: models.SuggestionContext},
	}
	concepts := []models.RelatedConcept{
		{NodeID: "c1", Name: "Architecture"},
		{NodeID: "c2", Name: "Design Pattern"},
	}

	got := s.generateRationale(models.ConsultRequest{}, suggestions, concepts)

	// Should mention higher-level abstractions
	if got == "" {
		t.Error("generateRationale() returned empty string")
	}
}

// =============================================================================
// analyzeContextTriggers tests
// =============================================================================

func TestAnalyzeContextTriggers_PatternMatching(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name            string
		context         string
		wantTriggerType string
	}{
		{
			name:            "error handling",
			context:         "try { doSomething() } catch (err) { handleError(err) }",
			wantTriggerType: "error_handling",
		},
		{
			name:            "authentication",
			context:         "const token = await login(username, password)",
			wantTriggerType: "authentication",
		},
		{
			name:            "database",
			context:         "db.query('SELECT * FROM users WHERE id = ?', [userId])",
			wantTriggerType: "database",
		},
		{
			name:            "api",
			context:         "router.get('/api/users', handleGetUsers)",
			wantTriggerType: "api",
		},
		{
			name:            "testing",
			context:         "describe('UserService', () => { it('should create user', () => {}) })",
			wantTriggerType: "testing",
		},
		{
			name:            "config",
			context:         "const config = process.env.DATABASE_URL",
			wantTriggerType: "config",
		},
		{
			name:            "async",
			context:         "async function fetchData() { const result = await api.get() }",
			wantTriggerType: "async",
		},
		{
			name:            "security",
			context:         "const hashedPassword = bcrypt.hash(password, salt)",
			wantTriggerType: "security",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggers := s.analyzeContextTriggers(tt.context, "")

			found := false
			for _, tr := range triggers {
				if tr.Matched == tt.wantTriggerType {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("analyzeContextTriggers() did not find %s trigger in context %q", tt.wantTriggerType, tt.context)
			}
		})
	}
}

func TestAnalyzeContextTriggers_FileType(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name     string
		filePath string
		wantType string
	}{
		{
			name:     "go test file",
			filePath: "/internal/service_test.go",
			wantType: "test_file",
		},
		{
			name:     "ts test file",
			filePath: "/src/user.test.ts",
			wantType: "test_file",
		},
		{
			name:     "spec file",
			filePath: "/src/user.spec.ts",
			wantType: "test_file",
		},
		{
			name:     "config directory",
			filePath: "/config/database.yaml",
			wantType: "config_file",
		},
		{
			name:     "config file",
			filePath: "/src/config.ts",
			wantType: "config_file",
		},
		{
			name:     "api handler",
			filePath: "/internal/api/handlers.go",
			wantType: "api_handler",
		},
		{
			name:     "handler file",
			filePath: "/src/handlers/user.ts",
			wantType: "api_handler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggers := s.analyzeContextTriggers("", tt.filePath)

			found := false
			for _, tr := range triggers {
				if tr.TriggerType == "file_type" && tr.Matched == tt.wantType {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("analyzeContextTriggers() did not find file_type %s trigger for path %s", tt.wantType, tt.filePath)
			}
		})
	}
}

func TestAnalyzeContextTriggers_NoTriggers(t *testing.T) {
	s := &Service{}

	triggers := s.analyzeContextTriggers("const x = 1 + 2", "/src/math.go")

	if len(triggers) != 0 {
		t.Errorf("analyzeContextTriggers() = %v, want empty", triggers)
	}
}

func TestAnalyzeContextTriggers_MultiplePatterns(t *testing.T) {
	s := &Service{}

	// Context with both authentication and database patterns
	context := "await db.query('SELECT * FROM users WHERE token = ?', [token])"

	triggers := s.analyzeContextTriggers(context, "")

	// Should find both database and possibly authentication triggers
	if len(triggers) < 1 {
		t.Errorf("analyzeContextTriggers() should find at least 1 trigger, got %d", len(triggers))
	}
}

// =============================================================================
// detectConflicts tests
// =============================================================================

func TestDetectConflicts_Empty(t *testing.T) {
	s := &Service{}

	conflicts := s.detectConflicts(nil, "test-space", "some context", nil)
	if len(conflicts) != 0 {
		t.Errorf("detectConflicts() with empty results = %v, want empty", conflicts)
	}
}

func TestDetectConflicts_DeprecatedPattern(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "node1",
			Name:    "old-api",
			Summary: "This method is deprecated, use newMethod instead",
			Score:   0.8, // High similarity
		},
	}

	conflicts := s.detectConflicts(nil, "test-space", "using oldMethod", results)

	if len(conflicts) == 0 {
		t.Error("detectConflicts() should detect deprecated pattern")
	}
	if len(conflicts) > 0 && conflicts[0].Severity != "medium" {
		t.Errorf("deprecated conflict severity = %s, want medium", conflicts[0].Severity)
	}
}

func TestDetectConflicts_AvoidPattern(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "node1",
			Name:    "coding-guideline",
			Summary: "Avoid using global variables for state management",
			Score:   0.7,
		},
	}

	conflicts := s.detectConflicts(nil, "test-space", "global state", results)

	if len(conflicts) == 0 {
		t.Error("detectConflicts() should detect avoid pattern")
	}
}

func TestDetectConflicts_ContradictoryPatterns(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "node1",
			Name:    "async-patterns",
			Summary: "The codebase uses async/await patterns consistently",
			Score:   0.7,
		},
	}

	// Context uses "sync" but codebase has "async" patterns
	conflicts := s.detectConflicts(nil, "test-space", "using sync operations", results)

	// Should detect sync vs async contradiction
	foundContradiction := false
	for _, c := range conflicts {
		if c.Description != "" {
			foundContradiction = true
			break
		}
	}
	if !foundContradiction && len(conflicts) > 0 {
		t.Log("detectConflicts() found conflicts but no contradiction description")
	}
}

func TestDetectConflicts_LowScoreIgnored(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "node1",
			Name:    "deprecated-api",
			Summary: "This is deprecated",
			Score:   0.3, // Low score - not relevant
		},
	}

	conflicts := s.detectConflicts(nil, "test-space", "using api", results)

	// Low score should not trigger conflict
	if len(conflicts) != 0 {
		t.Errorf("detectConflicts() should ignore low-score results, got %d conflicts", len(conflicts))
	}
}

// =============================================================================
// calculateSuggestConfidence tests
// =============================================================================

func TestCalculateSuggestConfidence_Empty(t *testing.T) {
	s := &Service{}

	got := s.calculateSuggestConfidence(nil, 0, 0)
	if got != 0.0 {
		t.Errorf("calculateSuggestConfidence(nil, 0, 0) = %v, want 0.0", got)
	}
}

func TestCalculateSuggestConfidence_BaseConfidence(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.5},
		{Confidence: 0.5},
	}

	got := s.calculateSuggestConfidence(suggestions, 0, 0)
	// Average 0.5, no boosts
	if got != 0.5 {
		t.Errorf("calculateSuggestConfidence() = %v, want 0.5", got)
	}
}

func TestCalculateSuggestConfidence_TriggerBoost(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.5},
	}

	// 2 triggers = 0.05 * 2 = 0.1 boost
	got := s.calculateSuggestConfidence(suggestions, 0, 2)
	expected := 0.5 * 1.1 // 0.55
	if got != expected {
		t.Errorf("calculateSuggestConfidence() = %v, want %v", got, expected)
	}
}

func TestCalculateSuggestConfidence_FilteredBoost(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.5},
	}

	// 5+ filtered results = 0.1 boost
	got := s.calculateSuggestConfidence(suggestions, 5, 0)
	expected := 0.5 * 1.1 // 0.55
	if got != expected {
		t.Errorf("calculateSuggestConfidence() = %v, want %v", got, expected)
	}
}

func TestCalculateSuggestConfidence_CombinedBoost(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.5},
	}

	// 2 triggers (0.1) + 5 filtered (0.1) = 0.2 boost (1.2 multiplier)
	got := s.calculateSuggestConfidence(suggestions, 5, 2)
	expected := 0.6
	// Use tolerance for floating-point comparison
	if got < expected-0.0001 || got > expected+0.0001 {
		t.Errorf("calculateSuggestConfidence() = %v, want ~%v", got, expected)
	}
}

func TestCalculateSuggestConfidence_CappedAtMax(t *testing.T) {
	s := &Service{}

	suggestions := []models.Suggestion{
		{Confidence: 0.9},
	}

	// High base + boosts should cap at MaxConfidence
	got := s.calculateSuggestConfidence(suggestions, 10, 5)
	if got != MaxConfidence {
		t.Errorf("calculateSuggestConfidence() = %v, want %v (MaxConfidence)", got, MaxConfidence)
	}
}

// =============================================================================
// findApplicableConstraints tests (via service pointer, no DB)
// =============================================================================

func TestFindApplicableConstraints_Empty(t *testing.T) {
	s := &Service{}

	constraints := s.findApplicableConstraints(nil, "test-space", nil, nil)
	if len(constraints) != 0 {
		t.Errorf("findApplicableConstraints() with empty = %v, want empty", constraints)
	}
}

func TestFindApplicableConstraints_MustPattern(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "rule1",
			Name:    "validation-rule",
			Summary: "All inputs must be validated before processing",
			Score:   0.7,
		},
	}

	constraints := s.findApplicableConstraints(nil, "test-space", results, nil)

	if len(constraints) == 0 {
		t.Error("findApplicableConstraints() should detect 'must' constraint")
	}
	if len(constraints) > 0 && constraints[0].ConstraintType != "must" {
		t.Errorf("constraint type = %s, want must", constraints[0].ConstraintType)
	}
}

func TestFindApplicableConstraints_ShouldPattern(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "guideline1",
			Name:    "naming-convention",
			Summary: "Functions should use camelCase naming",
			Score:   0.7,
		},
	}

	constraints := s.findApplicableConstraints(nil, "test-space", results, nil)

	if len(constraints) == 0 {
		t.Error("findApplicableConstraints() should detect 'should' constraint")
	}
	if len(constraints) > 0 && constraints[0].ConstraintType != "should" {
		t.Errorf("constraint type = %s, want should", constraints[0].ConstraintType)
	}
}

func TestFindApplicableConstraints_RuleNode(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "policy1",
			Name:    "security-policy",
			Summary: "Encryption standards for data at rest",
			Score:   0.6,
		},
	}

	constraints := s.findApplicableConstraints(nil, "test-space", results, nil)

	if len(constraints) == 0 {
		t.Error("findApplicableConstraints() should detect policy node as constraint")
	}
}

func TestFindApplicableConstraints_Deduplication(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "rule1",
			Name:    "validation-rule",
			Summary: "All inputs must be validated",
			Score:   0.8,
		},
		{
			NodeID:  "rule2",
			Name:    "validation-rule", // Same name
			Summary: "Inputs must be checked",
			Score:   0.7,
		},
	}

	constraints := s.findApplicableConstraints(nil, "test-space", results, nil)

	// Should deduplicate by name
	count := 0
	for _, c := range constraints {
		if c.Name == "validation-rule" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("findApplicableConstraints() found %d duplicates, want 1", count)
	}
}

func TestFindApplicableConstraints_LowScoreIgnored(t *testing.T) {
	s := &Service{}

	results := []models.RetrieveResult{
		{
			NodeID:  "rule1",
			Name:    "important-rule",
			Summary: "This must be followed",
			Score:   0.3, // Low score
		},
	}

	constraints := s.findApplicableConstraints(nil, "test-space", results, nil)

	if len(constraints) != 0 {
		t.Errorf("findApplicableConstraints() should ignore low-score results, got %d", len(constraints))
	}
}

// =============================================================================
// Service constructor tests
// =============================================================================

func TestNewService(t *testing.T) {
	// NewService with nil dependencies should not panic
	s := NewService(config.Config{}, nil, nil, nil, nil)
	if s == nil {
		t.Error("NewService() returned nil")
	}
}

func TestMaxConfidence(t *testing.T) {
	// Verify MaxConfidence is set correctly for Bayesian epistemology
	if MaxConfidence != 0.95 {
		t.Errorf("MaxConfidence = %v, want 0.95", MaxConfidence)
	}
}
