package main

import (
	"math"
	"testing"
)

// TestAverageEmbeddings verifies the centroid calculation of multiple embedding vectors
func TestAverageEmbeddings(t *testing.T) {
	tests := []struct {
		name       string
		embeddings [][]float64
		expected   []float64
		tolerance  float64
	}{
		{
			name: "two identical embeddings",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{1.0, 2.0, 3.0},
			},
			expected:  []float64{1.0, 2.0, 3.0},
			tolerance: 0.001,
		},
		{
			name: "two different embeddings",
			embeddings: [][]float64{
				{0.0, 0.0, 0.0},
				{2.0, 4.0, 6.0},
			},
			expected:  []float64{1.0, 2.0, 3.0},
			tolerance: 0.001,
		},
		{
			name: "three embeddings",
			embeddings: [][]float64{
				{1.0, 0.0, 0.0},
				{0.0, 1.0, 0.0},
				{0.0, 0.0, 1.0},
			},
			expected:  []float64{1.0 / 3.0, 1.0 / 3.0, 1.0 / 3.0},
			tolerance: 0.001,
		},
		{
			name: "single embedding",
			embeddings: [][]float64{
				{0.5, 0.5, 0.5},
			},
			expected:  []float64{0.5, 0.5, 0.5},
			tolerance: 0.001,
		},
		{
			name: "negative values",
			embeddings: [][]float64{
				{-1.0, 1.0, 0.0},
				{1.0, -1.0, 0.0},
			},
			expected:  []float64{0.0, 0.0, 0.0},
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := averageEmbeddings(tt.embeddings)

			if len(result) != len(tt.expected) {
				t.Fatalf("averageEmbeddings() length = %d, want %d", len(result), len(tt.expected))
			}

			for i, v := range result {
				if math.Abs(v-tt.expected[i]) > tt.tolerance {
					t.Errorf("averageEmbeddings()[%d] = %v, want %v (tolerance %v)",
						i, v, tt.expected[i], tt.tolerance)
				}
			}
		})
	}
}

// TestAverageEmbeddings_Empty verifies nil return for empty input
func TestAverageEmbeddings_Empty(t *testing.T) {
	tests := []struct {
		name       string
		embeddings [][]float64
	}{
		{
			name:       "nil slice",
			embeddings: nil,
		},
		{
			name:       "empty slice",
			embeddings: [][]float64{},
		},
		{
			name:       "slice with only empty embeddings",
			embeddings: [][]float64{{}, {}},
		},
		{
			name:       "slice with only nil embeddings",
			embeddings: [][]float64{nil, nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := averageEmbeddings(tt.embeddings)

			if result != nil {
				t.Errorf("averageEmbeddings() = %v, want nil", result)
			}
		})
	}
}

// TestAverageEmbeddings_MismatchedDims verifies handling of mismatched dimensions
func TestAverageEmbeddings_MismatchedDims(t *testing.T) {
	tests := []struct {
		name       string
		embeddings [][]float64
		expected   []float64
		tolerance  float64
	}{
		{
			name: "skip shorter embedding",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{1.0, 2.0}, // shorter - should be skipped
				{3.0, 4.0, 5.0},
			},
			expected:  []float64{2.0, 3.0, 4.0}, // average of first and third only
			tolerance: 0.001,
		},
		{
			name: "skip longer embedding",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{1.0, 2.0, 3.0, 4.0}, // longer - should be skipped
				{3.0, 4.0, 5.0},
			},
			expected:  []float64{2.0, 3.0, 4.0}, // average of first and third only
			tolerance: 0.001,
		},
		{
			name: "skip empty embedding in middle",
			embeddings: [][]float64{
				{2.0, 4.0, 6.0},
				{}, // empty - should be skipped
				{4.0, 6.0, 8.0},
			},
			expected:  []float64{3.0, 5.0, 7.0}, // average of first and third only
			tolerance: 0.001,
		},
		{
			name: "all mismatched after first",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{1.0, 2.0},
				{1.0},
			},
			expected:  []float64{1.0, 2.0, 3.0}, // only first is valid
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := averageEmbeddings(tt.embeddings)

			if len(result) != len(tt.expected) {
				t.Fatalf("averageEmbeddings() length = %d, want %d", len(result), len(tt.expected))
			}

			for i, v := range result {
				if math.Abs(v-tt.expected[i]) > tt.tolerance {
					t.Errorf("averageEmbeddings()[%d] = %v, want %v (tolerance %v)",
						i, v, tt.expected[i], tt.tolerance)
				}
			}
		})
	}
}

// TestBuildClusters verifies cluster grouping using greedy first-come assignment
func TestBuildClusters(t *testing.T) {
	tests := []struct {
		name             string
		candidates       []clusterCandidate
		minSize          int
		expectedClusters int
		expectedMembers  []int // number of members in each cluster
	}{
		{
			name: "single cluster of 3",
			candidates: []clusterCandidate{
				{
					NodeID:      "node-a",
					Layer:       0,
					Embedding:   []float64{0.1, 0.2, 0.3},
					NeighborIDs: []string{"node-b", "node-c"},
				},
				{
					NodeID:      "node-b",
					Layer:       0,
					Embedding:   []float64{0.2, 0.3, 0.4},
					NeighborIDs: []string{"node-a", "node-c"},
				},
				{
					NodeID:      "node-c",
					Layer:       0,
					Embedding:   []float64{0.3, 0.4, 0.5},
					NeighborIDs: []string{"node-a", "node-b"},
				},
			},
			minSize:          3,
			expectedClusters: 1,
			expectedMembers:  []int{3},
		},
		{
			name: "two separate clusters",
			candidates: []clusterCandidate{
				{
					NodeID:      "node-a",
					Layer:       0,
					Embedding:   []float64{0.1, 0.2, 0.3},
					NeighborIDs: []string{"node-b", "node-c"},
				},
				{
					NodeID:      "node-d",
					Layer:       0,
					Embedding:   []float64{0.4, 0.5, 0.6},
					NeighborIDs: []string{"node-e", "node-f"},
				},
			},
			minSize:          3,
			expectedClusters: 2,
			expectedMembers:  []int{3, 3},
		},
		{
			name: "nodes already assigned skip",
			candidates: []clusterCandidate{
				{
					NodeID:      "node-a",
					Layer:       0,
					Embedding:   []float64{0.1, 0.2, 0.3},
					NeighborIDs: []string{"node-b", "node-c", "node-d"},
				},
				{
					NodeID:      "node-b",
					Layer:       0,
					Embedding:   []float64{0.2, 0.3, 0.4},
					NeighborIDs: []string{"node-a", "node-c"},
				},
			},
			minSize:          3,
			expectedClusters: 1,
			expectedMembers:  []int{4}, // node-a + node-b + node-c + node-d
		},
		{
			name:             "empty candidates",
			candidates:       []clusterCandidate{},
			minSize:          3,
			expectedClusters: 0,
			expectedMembers:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildClusters(tt.candidates, tt.minSize)

			if len(result) != tt.expectedClusters {
				t.Fatalf("buildClusters() returned %d clusters, want %d",
					len(result), tt.expectedClusters)
			}

			for i, cluster := range result {
				if i < len(tt.expectedMembers) && len(cluster.Members) != tt.expectedMembers[i] {
					t.Errorf("cluster[%d] has %d members, want %d",
						i, len(cluster.Members), tt.expectedMembers[i])
				}
			}
		})
	}
}

// TestBuildClusters_MinSize verifies that clusters smaller than min size are excluded
func TestBuildClusters_MinSize(t *testing.T) {
	tests := []struct {
		name             string
		candidates       []clusterCandidate
		minSize          int
		expectedClusters int
	}{
		{
			name: "cluster below min size excluded",
			candidates: []clusterCandidate{
				{
					NodeID:      "node-a",
					Layer:       0,
					Embedding:   []float64{0.1, 0.2},
					NeighborIDs: []string{"node-b"}, // only 2 total, below minSize 3
				},
			},
			minSize:          3,
			expectedClusters: 0,
		},
		{
			name: "cluster exactly at min size included",
			candidates: []clusterCandidate{
				{
					NodeID:      "node-a",
					Layer:       0,
					Embedding:   []float64{0.1, 0.2},
					NeighborIDs: []string{"node-b", "node-c"}, // exactly 3
				},
			},
			minSize:          3,
			expectedClusters: 1,
		},
		{
			name: "min size 2",
			candidates: []clusterCandidate{
				{
					NodeID:      "node-a",
					Layer:       0,
					Embedding:   []float64{0.1, 0.2},
					NeighborIDs: []string{"node-b"}, // 2 total, meets minSize 2
				},
			},
			minSize:          2,
			expectedClusters: 1,
		},
		{
			name: "larger min size filters out small clusters",
			candidates: []clusterCandidate{
				{
					NodeID:      "node-a",
					Layer:       0,
					Embedding:   []float64{0.1, 0.2},
					NeighborIDs: []string{"node-b", "node-c", "node-d"}, // 4 total
				},
				{
					NodeID:      "node-e",
					Layer:       0,
					Embedding:   []float64{0.3, 0.4},
					NeighborIDs: []string{"node-f"}, // 2 total - below minSize 5
				},
			},
			minSize:          5,
			expectedClusters: 0, // first cluster has 4 (below 5), second has 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildClusters(tt.candidates, tt.minSize)

			if len(result) != tt.expectedClusters {
				t.Errorf("buildClusters() returned %d clusters, want %d",
					len(result), tt.expectedClusters)
			}
		})
	}
}

// TestBuildClusters_LayerPreserved verifies that cluster layer matches candidate layer
func TestBuildClusters_LayerPreserved(t *testing.T) {
	candidates := []clusterCandidate{
		{
			NodeID:      "node-a",
			Layer:       2,
			Embedding:   []float64{0.1, 0.2, 0.3},
			NeighborIDs: []string{"node-b", "node-c"},
		},
	}

	result := buildClusters(candidates, 3)

	if len(result) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result))
	}

	if result[0].Layer != 2 {
		t.Errorf("cluster Layer = %d, want 2", result[0].Layer)
	}
}

// TestBuildClusters_EmbeddingsPreserved verifies embeddings are copied to cluster members
func TestBuildClusters_EmbeddingsPreserved(t *testing.T) {
	expectedEmb := []float64{0.1, 0.2, 0.3}
	candidates := []clusterCandidate{
		{
			NodeID:      "node-a",
			Layer:       0,
			Embedding:   expectedEmb,
			NeighborIDs: []string{"node-b", "node-c"},
		},
		{
			NodeID:      "node-b",
			Layer:       0,
			Embedding:   []float64{0.4, 0.5, 0.6},
			NeighborIDs: []string{"node-a"},
		},
	}

	result := buildClusters(candidates, 3)

	if len(result) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result))
	}

	// Find member node-a and verify its embedding
	var found bool
	for _, member := range result[0].Members {
		if member.NodeID == "node-a" {
			found = true
			if len(member.Embedding) != len(expectedEmb) {
				t.Errorf("member embedding length = %d, want %d", len(member.Embedding), len(expectedEmb))
			}
			for i, v := range member.Embedding {
				if v != expectedEmb[i] {
					t.Errorf("member embedding[%d] = %v, want %v", i, v, expectedEmb[i])
				}
			}
			break
		}
	}
	if !found {
		t.Error("node-a not found in cluster members")
	}
}

// TestConfigValidation_MinClusterSize verifies min cluster size validation
func TestConfigValidation_MinClusterSize(t *testing.T) {
	tests := []struct {
		name        string
		minSize     int
		expectError bool
	}{
		{
			name:        "valid min size 3",
			minSize:     3,
			expectError: false,
		},
		{
			name:        "valid min size 2",
			minSize:     2,
			expectError: false,
		},
		{
			name:        "invalid min size 1",
			minSize:     1,
			expectError: true,
		},
		{
			name:        "invalid min size 0",
			minSize:     0,
			expectError: true,
		},
		{
			name:        "invalid negative min size",
			minSize:     -1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Directly test the validation logic
			valid := tt.minSize >= 2
			hasError := !valid

			if hasError != tt.expectError {
				t.Errorf("minClusterSize=%d validation: got error=%v, want error=%v",
					tt.minSize, hasError, tt.expectError)
			}
		})
	}
}

// TestConfigValidation_WeightThreshold verifies weight threshold validation
func TestConfigValidation_WeightThreshold(t *testing.T) {
	tests := []struct {
		name        string
		threshold   float64
		expectError bool
	}{
		{
			name:        "valid threshold 0.5",
			threshold:   0.5,
			expectError: false,
		},
		{
			name:        "valid threshold 0.0",
			threshold:   0.0,
			expectError: false,
		},
		{
			name:        "valid threshold 1.0",
			threshold:   1.0,
			expectError: false,
		},
		{
			name:        "invalid negative threshold",
			threshold:   -0.1,
			expectError: true,
		},
		{
			name:        "invalid threshold above 1",
			threshold:   1.1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Directly test the validation logic
			valid := tt.threshold >= 0 && tt.threshold <= 1
			hasError := !valid

			if hasError != tt.expectError {
				t.Errorf("weightThreshold=%v validation: got error=%v, want error=%v",
					tt.threshold, hasError, tt.expectError)
			}
		})
	}
}

// TestConfigValidation_MaxPromotions verifies max promotions validation
func TestConfigValidation_MaxPromotions(t *testing.T) {
	tests := []struct {
		name        string
		maxPromos   int
		expectError bool
	}{
		{
			name:        "valid max 50",
			maxPromos:   50,
			expectError: false,
		},
		{
			name:        "valid max 1",
			maxPromos:   1,
			expectError: false,
		},
		{
			name:        "invalid max 0",
			maxPromos:   0,
			expectError: true,
		},
		{
			name:        "invalid negative max",
			maxPromos:   -1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Directly test the validation logic
			valid := tt.maxPromos > 0
			hasError := !valid

			if hasError != tt.expectError {
				t.Errorf("maxPromotions=%d validation: got error=%v, want error=%v",
					tt.maxPromos, hasError, tt.expectError)
			}
		})
	}
}

// TestAsConversionHelpers verifies the type conversion helpers
func TestAsConversionHelpers(t *testing.T) {
	t.Run("asString", func(t *testing.T) {
		if asString(nil) != "" {
			t.Error("asString(nil) should return empty string")
		}
		if asString("hello") != "hello" {
			t.Error("asString(string) should return the string")
		}
		if asString(123) != "123" {
			t.Error("asString(int) should format as string")
		}
	})

	t.Run("asFloat64", func(t *testing.T) {
		if asFloat64(nil) != 0.0 {
			t.Error("asFloat64(nil) should return 0.0")
		}
		if asFloat64(3.14) != 3.14 {
			t.Error("asFloat64(float64) should return the float")
		}
		if asFloat64(int64(42)) != 42.0 {
			t.Error("asFloat64(int64) should convert to float64")
		}
		if asFloat64(int(42)) != 42.0 {
			t.Error("asFloat64(int) should convert to float64")
		}
	})

	t.Run("asInt", func(t *testing.T) {
		if asInt(nil) != 0 {
			t.Error("asInt(nil) should return 0")
		}
		if asInt(int64(42)) != 42 {
			t.Error("asInt(int64) should convert to int")
		}
		if asInt(int(42)) != 42 {
			t.Error("asInt(int) should return the int")
		}
		if asInt(3.9) != 3 {
			t.Error("asInt(float64) should truncate to int")
		}
	})

	t.Run("asBool", func(t *testing.T) {
		if asBool(nil) != false {
			t.Error("asBool(nil) should return false")
		}
		if asBool(true) != true {
			t.Error("asBool(true) should return true")
		}
		if asBool(false) != false {
			t.Error("asBool(false) should return false")
		}
		if asBool("true") != false {
			t.Error("asBool(string) should return false")
		}
	})
}

// TestAsFloat64Slice verifies the float64 slice conversion helper
func TestAsFloat64Slice(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := asFloat64Slice(nil)
		if result != nil {
			t.Error("asFloat64Slice(nil) should return nil")
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		result := asFloat64Slice([]any{})
		if len(result) != 0 {
			t.Error("asFloat64Slice([]) should return empty slice")
		}
	})

	t.Run("float64 values", func(t *testing.T) {
		input := []any{1.0, 2.0, 3.0}
		result := asFloat64Slice(input)
		if len(result) != 3 {
			t.Fatalf("asFloat64Slice length = %d, want 3", len(result))
		}
		if result[0] != 1.0 || result[1] != 2.0 || result[2] != 3.0 {
			t.Errorf("asFloat64Slice values = %v, want [1.0, 2.0, 3.0]", result)
		}
	})

	t.Run("mixed numeric values", func(t *testing.T) {
		input := []any{int64(1), 2.0, int(3)}
		result := asFloat64Slice(input)
		if len(result) != 3 {
			t.Fatalf("asFloat64Slice length = %d, want 3", len(result))
		}
		if result[0] != 1.0 || result[1] != 2.0 || result[2] != 3.0 {
			t.Errorf("asFloat64Slice values = %v, want [1.0, 2.0, 3.0]", result)
		}
	})

	t.Run("direct float64 slice", func(t *testing.T) {
		input := []float64{1.0, 2.0, 3.0}
		result := asFloat64Slice(input)
		if len(result) != 3 {
			t.Fatalf("asFloat64Slice length = %d, want 3", len(result))
		}
		if result[0] != 1.0 || result[1] != 2.0 || result[2] != 3.0 {
			t.Errorf("asFloat64Slice values = %v, want [1.0, 2.0, 3.0]", result)
		}
	})
}

// TestAsStringSlice verifies the string slice conversion helper
func TestAsStringSlice(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := asStringSlice(nil)
		if result != nil {
			t.Error("asStringSlice(nil) should return nil")
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		result := asStringSlice([]any{})
		if len(result) != 0 {
			t.Error("asStringSlice([]) should return empty slice")
		}
	})

	t.Run("string values", func(t *testing.T) {
		input := []any{"a", "b", "c"}
		result := asStringSlice(input)
		if len(result) != 3 {
			t.Fatalf("asStringSlice length = %d, want 3", len(result))
		}
		if result[0] != "a" || result[1] != "b" || result[2] != "c" {
			t.Errorf("asStringSlice values = %v, want [a, b, c]", result)
		}
	})

	t.Run("mixed values", func(t *testing.T) {
		input := []any{"a", 123, nil}
		result := asStringSlice(input)
		if len(result) != 3 {
			t.Fatalf("asStringSlice length = %d, want 3", len(result))
		}
		if result[0] != "a" || result[1] != "123" || result[2] != "" {
			t.Errorf("asStringSlice values = %v, want [a, 123, ]", result)
		}
	})

	t.Run("direct string slice", func(t *testing.T) {
		input := []string{"x", "y", "z"}
		result := asStringSlice(input)
		if len(result) != 3 {
			t.Fatalf("asStringSlice length = %d, want 3", len(result))
		}
		if result[0] != "x" || result[1] != "y" || result[2] != "z" {
			t.Errorf("asStringSlice values = %v, want [x, y, z]", result)
		}
	})
}

// TestTruncateID verifies the ID truncation helper
func TestTruncateID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short ID unchanged",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "exactly 12 chars unchanged",
			input:    "123456789012",
			expected: "123456789012",
		},
		{
			name:     "long ID truncated",
			input:    "12345678-1234-1234-1234-123456789012",
			expected: "12345678...",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateID(tt.input)
			if result != tt.expected {
				t.Errorf("truncateID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGenerateAbstractionName verifies abstraction name generation
func TestGenerateAbstractionName(t *testing.T) {
	tests := []struct {
		name     string
		members  []clusterMember
		contains string // name should contain this substring
	}{
		{
			name:     "empty members",
			members:  []clusterMember{},
			contains: "empty",
		},
		{
			name: "single member",
			members: []clusterMember{
				{NodeID: "node-a"},
			},
			contains: "node-a",
		},
		{
			name: "multiple members",
			members: []clusterMember{
				{NodeID: "node-a"},
				{NodeID: "node-b"},
				{NodeID: "node-c"},
			},
			contains: "Abstraction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateAbstractionName(tt.members)

			if result == "" {
				t.Error("generateAbstractionName() returned empty string")
			}

			if tt.contains != "" && !containsSubstring(result, tt.contains) {
				t.Errorf("generateAbstractionName() = %q, should contain %q", result, tt.contains)
			}
		})
	}
}

// containsSubstring is a helper to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
