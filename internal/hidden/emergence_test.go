package hidden

import (
	"testing"
)

// =============================================================================
// PHASE 4: EMERGENT CONCEPT FORMATION TESTS
// =============================================================================

func TestEmergentConceptStruct(t *testing.T) {
	concept := EmergentConcept{
		NodeID:           "concept-1",
		SpaceID:          "test-space",
		Layer:            2,
		Name:             "EmergentConcept-L2-plugin-architecture-0",
		Summary:          "Emerging pattern: plugin, architecture, modularity (3 themes, L2)",
		Embedding:        []float64{0.1, 0.2, 0.3},
		MemberCount:      3,
		Keywords:         []string{"plugin", "architecture", "modularity"},
		AvgSurpriseScore: 0.65,
		SessionCount:     5,
	}

	if concept.NodeID != "concept-1" {
		t.Errorf("EmergentConcept.NodeID = %s, want concept-1", concept.NodeID)
	}
	if concept.Layer != 2 {
		t.Errorf("EmergentConcept.Layer = %d, want 2", concept.Layer)
	}
	if concept.MemberCount != 3 {
		t.Errorf("EmergentConcept.MemberCount = %d, want 3", concept.MemberCount)
	}
	if len(concept.Keywords) != 3 {
		t.Errorf("EmergentConcept.Keywords length = %d, want 3", len(concept.Keywords))
	}
}

func TestEmergentConceptResultStruct(t *testing.T) {
	result := EmergentConceptResult{
		ConceptsCreated:  map[int]int{2: 3, 3: 1},
		EdgesCreated:     12,
		ThemesUsed:       10,
		NoiseThemes:      2,
		ConceptSummaries: []string{"Concept 1", "Concept 2", "Concept 3", "Concept 4"},
		MaxLayerReached:  3,
	}

	if result.ConceptsCreated[2] != 3 {
		t.Errorf("EmergentConceptResult.ConceptsCreated[2] = %d, want 3", result.ConceptsCreated[2])
	}
	if result.ConceptsCreated[3] != 1 {
		t.Errorf("EmergentConceptResult.ConceptsCreated[3] = %d, want 1", result.ConceptsCreated[3])
	}
	if result.EdgesCreated != 12 {
		t.Errorf("EmergentConceptResult.EdgesCreated = %d, want 12", result.EdgesCreated)
	}
	if result.MaxLayerReached != 3 {
		t.Errorf("EmergentConceptResult.MaxLayerReached = %d, want 3", result.MaxLayerReached)
	}
}

func TestConvertThemesToConceptNodes(t *testing.T) {
	themes := []ConversationThemeForClustering{
		{
			NodeID:     "theme-1",
			SpaceID:    "test-space",
			Name:       "ConvTheme-plugin-0",
			Summary:    "Learning about plugin architecture",
			Embedding:  []float64{1.0, 0.0, 0.0},
			SessionIDs: []string{"session-1", "session-2"},
			Keywords:   []string{"plugin", "architecture"},
		},
		{
			NodeID:     "theme-2",
			SpaceID:    "test-space",
			Name:       "ConvTheme-modular-1",
			Summary:    "Preferences for modularity",
			Embedding:  []float64{0.9, 0.1, 0.0},
			SessionIDs: []string{"session-2", "session-3"},
			Keywords:   []string{"modular", "loose-coupling"},
		},
	}

	nodes := convertThemesToConceptNodes(themes)

	if len(nodes) != 2 {
		t.Fatalf("convertThemesToConceptNodes returned %d nodes, want 2", len(nodes))
	}

	// Check first node
	if nodes[0].NodeID != "theme-1" {
		t.Errorf("nodes[0].NodeID = %s, want theme-1", nodes[0].NodeID)
	}
	if nodes[0].Layer != 1 {
		t.Errorf("nodes[0].Layer = %d, want 1", nodes[0].Layer)
	}
	if nodes[0].SessionCount != 2 {
		t.Errorf("nodes[0].SessionCount = %d, want 2", nodes[0].SessionCount)
	}
	if len(nodes[0].Keywords) != 2 {
		t.Errorf("nodes[0].Keywords length = %d, want 2", len(nodes[0].Keywords))
	}

	// Check second node
	if nodes[1].NodeID != "theme-2" {
		t.Errorf("nodes[1].NodeID = %s, want theme-2", nodes[1].NodeID)
	}
}

func TestGroupEmergentConceptsByCluster(t *testing.T) {
	nodes := []EmergentConceptNode{
		{NodeID: "a", Embedding: []float64{1.0, 0.0}},
		{NodeID: "b", Embedding: []float64{0.0, 1.0}},
		{NodeID: "c", Embedding: []float64{1.0, 1.0}},
		{NodeID: "d", Embedding: []float64{0.0, 0.0}},
	}
	labels := []int{0, 0, 1, -1} // a,b in cluster 0; c in cluster 1; d is noise

	clusters, noise := groupEmergentConceptsByCluster(nodes, labels)

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

func TestGenerateEmergentConceptSummary(t *testing.T) {
	tests := []struct {
		name     string
		members  []EmergentConceptNode
		layer    int
		contains []string // Strings that should be present in summary
	}{
		{
			name: "layer 2 with keywords",
			members: []EmergentConceptNode{
				{Summary: "Learning about plugin architecture", Keywords: []string{"plugin", "architecture"}},
				{Summary: "Preferences for modularity", Keywords: []string{"modular", "extensibility"}},
				{Summary: "Decisions about loose coupling", Keywords: []string{"coupling", "design"}},
			},
			layer:    2,
			contains: []string{"Emerging pattern:", "(3 themes, L2)"},
		},
		{
			name: "layer 3",
			members: []EmergentConceptNode{
				{Summary: "Emerging pattern: plugin, architecture", Keywords: []string{"plugin"}},
				{Summary: "Emerging pattern: modular, design", Keywords: []string{"modular"}},
				{Summary: "Emerging pattern: extensibility", Keywords: []string{"extensible"}},
			},
			layer:    3,
			contains: []string{"Core understanding:", "(3 themes, L3)"},
		},
		{
			name: "layer 4",
			members: []EmergentConceptNode{
				{Summary: "Core understanding: patterns", Keywords: []string{"patterns"}},
				{Summary: "Core understanding: architecture", Keywords: []string{"architecture"}},
			},
			layer:    4,
			contains: []string{"Foundational principle:", "(2 themes, L4)"},
		},
		{
			name:     "empty members",
			members:  []EmergentConceptNode{},
			layer:    2,
			contains: []string{"Empty emergent concept"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := generateEmergentConceptSummary(tt.members, tt.layer)

			for _, expected := range tt.contains {
				if !containsString(summary, expected) {
					t.Errorf("generateEmergentConceptSummary() summary = %q, should contain %q", summary, expected)
				}
			}
		})
	}
}

func TestAggregateKeywordsFromConcepts(t *testing.T) {
	members := []EmergentConceptNode{
		{Keywords: []string{"plugin", "architecture", "modular"}},
		{Keywords: []string{"plugin", "extensibility", "design"}},
		{Keywords: []string{"plugin", "modularity", "architecture"}},
	}

	keywords := aggregateKeywordsFromConcepts(members)

	// "plugin" appears 3 times, should be first
	if len(keywords) == 0 || keywords[0] != "plugin" {
		t.Errorf("aggregateKeywordsFromConcepts() first keyword = %v, want 'plugin'", keywords)
	}

	// "architecture" appears 2 times, should be present
	found := false
	for _, kw := range keywords {
		if kw == "architecture" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("aggregateKeywordsFromConcepts() should contain 'architecture', got: %v", keywords)
	}
}

func TestAggregateKeywordsFromConcepts_FromSummary(t *testing.T) {
	// When keywords are empty, should extract from summary
	members := []EmergentConceptNode{
		{Summary: "Learning about plugin architecture and design patterns", Keywords: nil},
		{Summary: "Preferences for plugin-based modularity", Keywords: nil},
	}

	keywords := aggregateKeywordsFromConcepts(members)

	// Should extract meaningful words from summaries
	if len(keywords) == 0 {
		t.Errorf("aggregateKeywordsFromConcepts() returned empty keywords, expected extraction from summary")
	}
}

func TestCalculateEmergentConceptMetadata(t *testing.T) {
	tests := []struct {
		name                 string
		members              []EmergentConceptNode
		expectedSessionCount int
	}{
		{
			name: "multiple sessions",
			members: []EmergentConceptNode{
				{SessionCount: 2},
				{SessionCount: 3},
				{SessionCount: 1},
			},
			expectedSessionCount: 6,
		},
		{
			name: "capped session count",
			members: []EmergentConceptNode{
				{SessionCount: 10},
				{SessionCount: 10},
				{SessionCount: 10},
			},
			expectedSessionCount: 6, // Capped at len(members) * 2
		},
		{
			name:                 "empty members",
			members:              []EmergentConceptNode{},
			expectedSessionCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, sessionCount := calculateEmergentConceptMetadata(tt.members)
			if sessionCount != tt.expectedSessionCount {
				t.Errorf("calculateEmergentConceptMetadata() sessionCount = %d, want %d", sessionCount, tt.expectedSessionCount)
			}
		})
	}
}

func TestSanitizeConceptName(t *testing.T) {
	tests := []struct {
		name     string
		summary  string
		expected string
	}{
		{
			name:     "emerging pattern prefix",
			summary:  "Emerging pattern: plugin, architecture",
			expected: "plugin-architecture",
		},
		{
			name:     "core understanding prefix",
			summary:  "Core understanding: design patterns",
			expected: "design-patterns",
		},
		{
			name:     "foundational principle prefix",
			summary:  "Foundational principle: modularity",
			expected: "modularity",
		},
		{
			name:     "long name truncated",
			summary:  "Emerging pattern: very-long-concept-name-that-exceeds-limit",
			expected: "very-long-concept-name-th",
		},
		{
			name:     "empty summary",
			summary:  "",
			expected: "misc",
		},
		{
			name:     "only skip words",
			summary:  "Emerging pattern: ",
			expected: "misc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeConceptName(tt.summary)
			if result != tt.expected {
				t.Errorf("sanitizeConceptName(%q) = %q, want %q", tt.summary, result, tt.expected)
			}
		})
	}
}

func TestExtractConceptsFromNames(t *testing.T) {
	members := []EmergentConceptNode{
		{Name: "ConvTheme-plugin-0", Summary: "Learning about plugin architecture"},
		{Name: "ConvTheme-plugin-1", Summary: "Plugin extensibility patterns"},
		{Name: "ConvTheme-modular-2", Summary: "Modular design principles"},
	}

	concepts := extractConceptsFromNames(members)

	// Should extract meaningful words
	if len(concepts) == 0 {
		t.Errorf("extractConceptsFromNames() returned empty, expected meaningful concepts")
	}

	// Should not contain skip words
	for _, c := range concepts {
		skipWords := []string{"convtheme", "about", "the"}
		for _, skip := range skipWords {
			if c == skip {
				t.Errorf("extractConceptsFromNames() returned skip word %q", c)
			}
		}
	}
}

func TestConceptHierarchyNodeStruct(t *testing.T) {
	node := ConceptHierarchyNode{
		NodeID:    "node-1",
		SpaceID:   "test-space",
		Layer:     2,
		RoleType:  "emergent_concept",
		Name:      "EmergentConcept-L2-test-0",
		Summary:   "Test concept",
		Embedding: []float64{0.1, 0.2},
	}

	if node.NodeID != "node-1" {
		t.Errorf("ConceptHierarchyNode.NodeID = %s, want node-1", node.NodeID)
	}
	if node.RoleType != "emergent_concept" {
		t.Errorf("ConceptHierarchyNode.RoleType = %s, want emergent_concept", node.RoleType)
	}
}

// TestEmergentConceptDBSCAN verifies DBSCAN clustering works for emergent concepts
func TestEmergentConceptDBSCAN(t *testing.T) {
	// Create concept nodes with distinct clusters
	// Cluster 1: Similar embeddings (design-related themes)
	// Cluster 2: Similar embeddings (testing-related themes)
	// Noise: Isolated point

	nodes := []EmergentConceptNode{
		// Cluster 1 - similar embeddings
		{NodeID: "n1", Embedding: []float64{1.0, 0.0, 0.0}},
		{NodeID: "n2", Embedding: []float64{0.99, 0.1, 0.0}},
		{NodeID: "n3", Embedding: []float64{0.98, 0.15, 0.0}},

		// Cluster 2 - similar embeddings but different direction
		{NodeID: "n4", Embedding: []float64{0.0, 1.0, 0.0}},
		{NodeID: "n5", Embedding: []float64{0.1, 0.99, 0.0}},
		{NodeID: "n6", Embedding: []float64{0.15, 0.98, 0.0}},

		// Noise - isolated
		{NodeID: "n7", Embedding: []float64{0.0, 0.0, 1.0}},
	}

	// Extract embeddings
	embeddings := make([][]float64, len(nodes))
	for i, n := range nodes {
		embeddings[i] = n.Embedding
	}

	// Run DBSCAN with reasonable parameters
	labels := DBSCAN(embeddings, 0.2, 2)

	// Group by cluster
	clusters, noise := groupEmergentConceptsByCluster(nodes, labels)

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
		t.Errorf("Expected 6 clustered nodes, got %d", totalClustered)
	}
}

// TestMultiLayerHierarchy tests that L0->L1->L2->L3 hierarchy can be built
func TestMultiLayerHierarchy(t *testing.T) {
	// This is a structural test - verify the types support the hierarchy

	// L0: Observations
	obs := ConversationObservation{
		NodeID:    "obs-1",
		SpaceID:   "test",
		ObsType:   "learning",
		Embedding: []float64{1.0, 0.0},
	}

	// L1: Themes
	theme := ConversationThemeForClustering{
		NodeID:     "theme-1",
		SpaceID:    "test",
		Embedding:  []float64{0.9, 0.1},
		SessionIDs: []string{"session-1"},
	}

	// L2: Emergent Concept
	concept := EmergentConcept{
		NodeID:      "concept-1",
		SpaceID:     "test",
		Layer:       2,
		Embedding:   []float64{0.8, 0.2},
		MemberCount: 3,
	}

	// L3: Higher Emergent Concept
	higherConcept := EmergentConcept{
		NodeID:      "concept-2",
		SpaceID:     "test",
		Layer:       3,
		Embedding:   []float64{0.7, 0.3},
		MemberCount: 2,
	}

	// Verify distinct layers
	if obs.SpaceID != theme.SpaceID || theme.SpaceID != concept.SpaceID {
		t.Error("All nodes should be in the same space")
	}

	// Verify layer progression
	if concept.Layer != 2 {
		t.Errorf("L2 concept Layer = %d, want 2", concept.Layer)
	}
	if higherConcept.Layer != 3 {
		t.Errorf("L3 concept Layer = %d, want 3", higherConcept.Layer)
	}

	// Verify member counts
	if concept.MemberCount < 2 {
		t.Error("Emergent concept should have at least 2 members")
	}
}

// TestCrossSessionConceptFormation verifies concepts can form from multiple sessions
func TestCrossSessionConceptFormation(t *testing.T) {
	// Create themes from different sessions
	themes := []ConversationThemeForClustering{
		{
			NodeID:     "theme-1",
			SessionIDs: []string{"session-1", "session-2"},
			Embedding:  []float64{1.0, 0.0, 0.0},
			Keywords:   []string{"plugin"},
		},
		{
			NodeID:     "theme-2",
			SessionIDs: []string{"session-3"},
			Embedding:  []float64{0.95, 0.1, 0.0},
			Keywords:   []string{"modular"},
		},
		{
			NodeID:     "theme-3",
			SessionIDs: []string{"session-4", "session-5"},
			Embedding:  []float64{0.9, 0.2, 0.0},
			Keywords:   []string{"extensible"},
		},
	}

	// Convert to concept nodes
	nodes := convertThemesToConceptNodes(themes)

	// Calculate total sessions
	totalSessions := 0
	for _, n := range nodes {
		totalSessions += n.SessionCount
	}

	// Should represent multiple sessions
	if totalSessions < 5 {
		t.Errorf("Expected at least 5 sessions represented, got %d", totalSessions)
	}

	// Embeddings should be similar (can cluster together)
	embeddings := make([][]float64, len(nodes))
	for i, n := range nodes {
		embeddings[i] = n.Embedding
	}

	// Run DBSCAN - these should cluster together
	labels := DBSCAN(embeddings, 0.3, 2)

	// Count non-noise
	clustered := 0
	for _, l := range labels {
		if l != -1 {
			clustered++
		}
	}

	if clustered < 3 {
		t.Errorf("Cross-session themes should cluster together, only %d clustered", clustered)
	}
}
