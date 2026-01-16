package learning

import (
	"testing"

	"mdemg/internal/models"
)

// TestClamp01 tests the clamp01 helper function
func TestClamp01(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"value within bounds", 0.5, 0.5},
		{"value at lower bound", 0.0, 0.0},
		{"value at upper bound", 1.0, 1.0},
		{"value below lower bound", -0.5, 0.0},
		{"value above upper bound", 1.5, 1.0},
		{"large negative value", -100.0, 0.0},
		{"large positive value", 100.0, 1.0},
		{"small positive value", 0.001, 0.001},
		{"value near upper bound", 0.999, 0.999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clamp01(tt.input)
			if result != tt.expected {
				t.Errorf("clamp01(%f) = %f, expected %f", tt.input, result, tt.expected)
			}
		})
	}
}

// TestPairsToMaps tests the conversion of pairs to maps for Cypher parameters
func TestPairsToMaps(t *testing.T) {
	tests := []struct {
		name   string
		pairs  []pair
		verify func(t *testing.T, result []map[string]any)
	}{
		{
			name:  "empty pairs",
			pairs: []pair{},
			verify: func(t *testing.T, result []map[string]any) {
				if len(result) != 0 {
					t.Errorf("expected empty result, got %d items", len(result))
				}
			},
		},
		{
			name: "single pair with normal values",
			pairs: []pair{
				{src: "node1", dst: "node2", ai: 0.5, aj: 0.7},
			},
			verify: func(t *testing.T, result []map[string]any) {
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				m := result[0]
				if m["src"] != "node1" {
					t.Errorf("expected src=node1, got %v", m["src"])
				}
				if m["dst"] != "node2" {
					t.Errorf("expected dst=node2, got %v", m["dst"])
				}
				if m["ai"] != 0.5 {
					t.Errorf("expected ai=0.5, got %v", m["ai"])
				}
				if m["aj"] != 0.7 {
					t.Errorf("expected aj=0.7, got %v", m["aj"])
				}
			},
		},
		{
			name: "pair with out-of-bounds activations gets clamped",
			pairs: []pair{
				{src: "a", dst: "b", ai: -0.5, aj: 1.5},
			},
			verify: func(t *testing.T, result []map[string]any) {
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				m := result[0]
				if m["ai"] != 0.0 {
					t.Errorf("expected ai=0.0 (clamped), got %v", m["ai"])
				}
				if m["aj"] != 1.0 {
					t.Errorf("expected aj=1.0 (clamped), got %v", m["aj"])
				}
			},
		},
		{
			name: "multiple pairs",
			pairs: []pair{
				{src: "n1", dst: "n2", ai: 0.3, aj: 0.4},
				{src: "n2", dst: "n3", ai: 0.5, aj: 0.6},
				{src: "n1", dst: "n3", ai: 0.7, aj: 0.8},
			},
			verify: func(t *testing.T, result []map[string]any) {
				if len(result) != 3 {
					t.Fatalf("expected 3 results, got %d", len(result))
				}
				// Check that all pairs are present in order
				expectedSrcs := []string{"n1", "n2", "n1"}
				for i, src := range expectedSrcs {
					if result[i]["src"] != src {
						t.Errorf("result[%d].src = %v, expected %v", i, result[i]["src"], src)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pairsToMaps(tt.pairs)
			tt.verify(t, result)
		})
	}
}

// TestFilterByActivationThreshold tests that nodes below the activation threshold are filtered out
func TestFilterByActivationThreshold(t *testing.T) {
	tests := []struct {
		name         string
		results      []models.RetrieveResult
		minAct       float64
		expectedLen  int
		expectedIDs  []string // expected node IDs after filtering
	}{
		{
			name:        "all nodes above threshold",
			minAct:      0.20,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.5},
				{NodeID: "n2", Activation: 0.6},
				{NodeID: "n3", Activation: 0.7},
			},
			expectedLen: 3,
			expectedIDs: []string{"n1", "n2", "n3"},
		},
		{
			name:        "all nodes below threshold",
			minAct:      0.50,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.1},
				{NodeID: "n2", Activation: 0.2},
				{NodeID: "n3", Activation: 0.3},
			},
			expectedLen: 0,
			expectedIDs: []string{},
		},
		{
			name:        "mixed nodes above and below threshold",
			minAct:      0.40,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.1},  // below
				{NodeID: "n2", Activation: 0.5},  // above
				{NodeID: "n3", Activation: 0.3},  // below
				{NodeID: "n4", Activation: 0.8},  // above
			},
			expectedLen: 2,
			expectedIDs: []string{"n2", "n4"},
		},
		{
			name:        "node exactly at threshold",
			minAct:      0.50,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.5},  // exactly at threshold
				{NodeID: "n2", Activation: 0.49}, // just below
			},
			expectedLen: 1,
			expectedIDs: []string{"n1"},
		},
		{
			name:        "empty results",
			minAct:      0.20,
			results:     []models.RetrieveResult{},
			expectedLen: 0,
			expectedIDs: []string{},
		},
		{
			name:        "zero threshold includes all",
			minAct:      0.0,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.0},
				{NodeID: "n2", Activation: 0.001},
			},
			expectedLen: 2,
			expectedIDs: []string{"n1", "n2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the filtering logic from ApplyCoactivation
			nodes := make([]models.RetrieveResult, 0, len(tt.results))
			for _, r := range tt.results {
				if r.Activation >= tt.minAct {
					nodes = append(nodes, r)
				}
			}

			if len(nodes) != tt.expectedLen {
				t.Errorf("expected %d nodes after filtering, got %d", tt.expectedLen, len(nodes))
			}

			// Verify expected IDs
			for i, expected := range tt.expectedIDs {
				if i >= len(nodes) {
					t.Errorf("expected node %s at index %d, but nodes slice too short", expected, i)
					continue
				}
				if nodes[i].NodeID != expected {
					t.Errorf("expected node %s at index %d, got %s", expected, i, nodes[i].NodeID)
				}
			}
		})
	}
}

// TestPairGeneration tests that pairs are generated correctly from nodes
func TestPairGeneration(t *testing.T) {
	tests := []struct {
		name          string
		nodes         []models.RetrieveResult
		expectedPairs int
		verifyPairs   func(t *testing.T, pairs []pair)
	}{
		{
			name:          "two nodes generate one pair",
			nodes:         makeNodes(2, 0.5),
			expectedPairs: 1,
			verifyPairs: func(t *testing.T, pairs []pair) {
				if pairs[0].src != "n0" || pairs[0].dst != "n1" {
					t.Errorf("expected pair (n0, n1), got (%s, %s)", pairs[0].src, pairs[0].dst)
				}
			},
		},
		{
			name:          "three nodes generate three pairs",
			nodes:         makeNodes(3, 0.5),
			expectedPairs: 3, // C(3,2) = 3
			verifyPairs: func(t *testing.T, pairs []pair) {
				expected := []struct{ src, dst string }{
					{"n0", "n1"},
					{"n0", "n2"},
					{"n1", "n2"},
				}
				for i, e := range expected {
					if pairs[i].src != e.src || pairs[i].dst != e.dst {
						t.Errorf("pair[%d]: expected (%s, %s), got (%s, %s)",
							i, e.src, e.dst, pairs[i].src, pairs[i].dst)
					}
				}
			},
		},
		{
			name:          "four nodes generate six pairs",
			nodes:         makeNodes(4, 0.5),
			expectedPairs: 6, // C(4,2) = 6
			verifyPairs:   nil,
		},
		{
			name:          "five nodes generate ten pairs",
			nodes:         makeNodes(5, 0.5),
			expectedPairs: 10, // C(5,2) = 10
			verifyPairs:   nil,
		},
		{
			name:          "ten nodes generate 45 pairs",
			nodes:         makeNodes(10, 0.5),
			expectedPairs: 45, // C(10,2) = 45
			verifyPairs:   nil,
		},
		{
			name:          "one node generates zero pairs",
			nodes:         makeNodes(1, 0.5),
			expectedPairs: 0,
			verifyPairs:   nil,
		},
		{
			name:          "zero nodes generates zero pairs",
			nodes:         makeNodes(0, 0.5),
			expectedPairs: 0,
			verifyPairs:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the pair generation logic from ApplyCoactivation
			pairs := make([]pair, 0)
			for i := 0; i < len(tt.nodes); i++ {
				for j := i + 1; j < len(tt.nodes); j++ {
					pairs = append(pairs, pair{
						src: tt.nodes[i].NodeID,
						dst: tt.nodes[j].NodeID,
						ai:  tt.nodes[i].Activation,
						aj:  tt.nodes[j].Activation,
					})
				}
			}

			if len(pairs) != tt.expectedPairs {
				t.Errorf("expected %d pairs, got %d", tt.expectedPairs, len(pairs))
			}

			if tt.verifyPairs != nil {
				tt.verifyPairs(t, pairs)
			}
		})
	}
}

// TestPairActivationProducts tests that pairs correctly store activation products
func TestPairActivationProducts(t *testing.T) {
	nodes := []models.RetrieveResult{
		{NodeID: "n0", Activation: 0.3},
		{NodeID: "n1", Activation: 0.5},
		{NodeID: "n2", Activation: 0.8},
	}

	// Generate pairs
	pairs := make([]pair, 0)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			pairs = append(pairs, pair{
				src: nodes[i].NodeID,
				dst: nodes[j].NodeID,
				ai:  nodes[i].Activation,
				aj:  nodes[j].Activation,
			})
		}
	}

	// Verify activation products
	expectedProducts := []float64{
		0.3 * 0.5, // n0-n1: 0.15
		0.3 * 0.8, // n0-n2: 0.24
		0.5 * 0.8, // n1-n2: 0.40
	}

	for i, p := range pairs {
		product := p.ai * p.aj
		if product != expectedProducts[i] {
			t.Errorf("pair[%d] product = %f, expected %f", i, product, expectedProducts[i])
		}
	}
}

// TestTopKPairSelection tests that top-K pairs are selected by activation product
func TestTopKPairSelection(t *testing.T) {
	tests := []struct {
		name         string
		pairs        []pair
		cap          int
		expectedLen  int
		verifyFirst  func(t *testing.T, pairs []pair) // verify first pair is highest product
	}{
		{
			name: "cap larger than pairs keeps all",
			pairs: []pair{
				{src: "a", dst: "b", ai: 0.3, aj: 0.4}, // product: 0.12
				{src: "c", dst: "d", ai: 0.5, aj: 0.6}, // product: 0.30
			},
			cap:         100,
			expectedLen: 2,
			verifyFirst: nil,
		},
		{
			name: "cap smaller than pairs truncates and sorts",
			pairs: []pair{
				{src: "a", dst: "b", ai: 0.3, aj: 0.4}, // product: 0.12
				{src: "c", dst: "d", ai: 0.5, aj: 0.6}, // product: 0.30
				{src: "e", dst: "f", ai: 0.7, aj: 0.8}, // product: 0.56
			},
			cap:         2,
			expectedLen: 2,
			verifyFirst: func(t *testing.T, pairs []pair) {
				// First pair should have highest product (0.56)
				if pairs[0].src != "e" || pairs[0].dst != "f" {
					t.Errorf("expected first pair (e, f), got (%s, %s)", pairs[0].src, pairs[0].dst)
				}
				// Second pair should have second highest product (0.30)
				if pairs[1].src != "c" || pairs[1].dst != "d" {
					t.Errorf("expected second pair (c, d), got (%s, %s)", pairs[1].src, pairs[1].dst)
				}
			},
		},
		{
			name: "cap equal to pairs keeps all without sorting",
			pairs: []pair{
				{src: "low", dst: "low", ai: 0.1, aj: 0.1},   // product: 0.01
				{src: "mid", dst: "mid", ai: 0.5, aj: 0.5},   // product: 0.25
				{src: "high", dst: "high", ai: 0.9, aj: 0.9}, // product: 0.81
			},
			cap:         3,
			expectedLen: 3,
			verifyFirst: func(t *testing.T, pairs []pair) {
				// When cap >= len(pairs), no sorting happens - order is preserved
				// First pair should be "low" since that was the original order
				if pairs[0].src != "low" {
					t.Errorf("expected first pair src=low (original order), got %s", pairs[0].src)
				}
			},
		},
		{
			name:        "empty pairs",
			pairs:       []pair{},
			cap:         10,
			expectedLen: 0,
			verifyFirst: nil,
		},
		{
			name: "cap of 1 returns single best pair",
			pairs: []pair{
				{src: "a", dst: "b", ai: 0.2, aj: 0.2}, // product: 0.04
				{src: "c", dst: "d", ai: 0.9, aj: 0.9}, // product: 0.81
				{src: "e", dst: "f", ai: 0.5, aj: 0.5}, // product: 0.25
			},
			cap:         1,
			expectedLen: 1,
			verifyFirst: func(t *testing.T, pairs []pair) {
				if pairs[0].src != "c" || pairs[0].dst != "d" {
					t.Errorf("expected best pair (c, d), got (%s, %s)", pairs[0].src, pairs[0].dst)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the test case
			pairs := make([]pair, len(tt.pairs))
			copy(pairs, tt.pairs)

			// Apply the top-K selection logic from ApplyCoactivation
			if len(pairs) > tt.cap {
				// Sort descending by activation product
				sortPairsByProduct(pairs)
				pairs = pairs[:tt.cap]
			}

			if len(pairs) != tt.expectedLen {
				t.Errorf("expected %d pairs, got %d", tt.expectedLen, len(pairs))
			}

			if tt.verifyFirst != nil && len(pairs) > 0 {
				tt.verifyFirst(t, pairs)
			}
		})
	}
}

// TestTopKSelectionPreservesHighestProducts verifies that top-K always keeps highest products
func TestTopKSelectionPreservesHighestProducts(t *testing.T) {
	// Create pairs with known products
	pairs := []pair{
		{src: "p1", dst: "p2", ai: 0.1, aj: 0.1}, // product: 0.01
		{src: "p3", dst: "p4", ai: 0.2, aj: 0.3}, // product: 0.06
		{src: "p5", dst: "p6", ai: 0.5, aj: 0.4}, // product: 0.20
		{src: "p7", dst: "p8", ai: 0.6, aj: 0.6}, // product: 0.36
		{src: "p9", dst: "p10", ai: 0.9, aj: 0.8}, // product: 0.72
	}

	cap := 3
	sortPairsByProduct(pairs)
	selected := pairs[:cap]

	// The three highest products should be: 0.72, 0.36, 0.20
	expectedMinProduct := 0.20

	for i, p := range selected {
		product := p.ai * p.aj
		if product < expectedMinProduct {
			t.Errorf("selected[%d] has product %f, expected >= %f", i, product, expectedMinProduct)
		}
	}

	// Verify the lowest product pair (0.01) is not selected
	for _, p := range selected {
		if p.src == "p1" && p.dst == "p2" {
			t.Error("pair with lowest product should not be selected")
		}
	}
}

// TestEdgeCapEnforcement tests that the edge cap is properly enforced
func TestEdgeCapEnforcement(t *testing.T) {
	tests := []struct {
		name        string
		numNodes    int
		cap         int
		expectedMax int // max pairs after cap
	}{
		{
			name:        "small number of nodes well under cap",
			numNodes:    5,
			cap:         200,
			expectedMax: 10, // C(5,2) = 10
		},
		{
			name:        "many nodes capped at limit",
			numNodes:    50,
			cap:         200,
			expectedMax: 200, // C(50,2) = 1225, but capped at 200
		},
		{
			name:        "exact cap boundary",
			numNodes:    20,
			cap:         190,
			expectedMax: 190, // C(20,2) = 190, equals cap
		},
		{
			name:        "very small cap",
			numNodes:    100,
			cap:         10,
			expectedMax: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := makeNodes(tt.numNodes, 0.5)

			// Generate all pairs
			pairs := make([]pair, 0)
			for i := 0; i < len(nodes); i++ {
				for j := i + 1; j < len(nodes); j++ {
					pairs = append(pairs, pair{
						src: nodes[i].NodeID,
						dst: nodes[j].NodeID,
						ai:  nodes[i].Activation,
						aj:  nodes[j].Activation,
					})
				}
			}

			// Apply cap
			if len(pairs) > tt.cap {
				sortPairsByProduct(pairs)
				pairs = pairs[:tt.cap]
			}

			if len(pairs) > tt.expectedMax {
				t.Errorf("expected at most %d pairs, got %d", tt.expectedMax, len(pairs))
			}
		})
	}
}

// TestCombinedFilteringAndSelection tests the full pipeline of filtering and selection
func TestCombinedFilteringAndSelection(t *testing.T) {
	// Create a mix of nodes with varying activation levels
	results := []models.RetrieveResult{
		{NodeID: "low1", Activation: 0.05},
		{NodeID: "low2", Activation: 0.10},
		{NodeID: "med1", Activation: 0.30},
		{NodeID: "med2", Activation: 0.40},
		{NodeID: "high1", Activation: 0.70},
		{NodeID: "high2", Activation: 0.85},
		{NodeID: "high3", Activation: 0.95},
	}

	minAct := 0.25
	cap := 3

	// Step 1: Filter by activation threshold
	nodes := make([]models.RetrieveResult, 0)
	for _, r := range results {
		if r.Activation >= minAct {
			nodes = append(nodes, r)
		}
	}

	// Should have 5 nodes (med1, med2, high1, high2, high3)
	if len(nodes) != 5 {
		t.Fatalf("expected 5 nodes after filtering, got %d", len(nodes))
	}

	// Step 2: Generate pairs
	pairs := make([]pair, 0)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			pairs = append(pairs, pair{
				src: nodes[i].NodeID,
				dst: nodes[j].NodeID,
				ai:  nodes[i].Activation,
				aj:  nodes[j].Activation,
			})
		}
	}

	// C(5,2) = 10 pairs
	if len(pairs) != 10 {
		t.Fatalf("expected 10 pairs, got %d", len(pairs))
	}

	// Step 3: Select top-K by activation product
	sortPairsByProduct(pairs)
	if len(pairs) > cap {
		pairs = pairs[:cap]
	}

	if len(pairs) != cap {
		t.Fatalf("expected %d pairs after cap, got %d", cap, len(pairs))
	}

	// Verify the top pairs include the highest activation nodes
	// high2 (0.85) * high3 (0.95) = 0.8075 should be first
	// high1 (0.70) * high3 (0.95) = 0.665 should be second or third
	// high1 (0.70) * high2 (0.85) = 0.595 should be second or third

	topPair := pairs[0]
	// Both nodes in top pair should be from high activation nodes
	isHighNode := func(id string) bool {
		return id == "high1" || id == "high2" || id == "high3"
	}

	if !isHighNode(topPair.src) || !isHighNode(topPair.dst) {
		t.Errorf("expected top pair to contain high activation nodes, got (%s, %s)",
			topPair.src, topPair.dst)
	}
}

// Helper function to create test nodes
func makeNodes(n int, activation float64) []models.RetrieveResult {
	nodes := make([]models.RetrieveResult, n)
	for i := 0; i < n; i++ {
		nodes[i] = models.RetrieveResult{
			NodeID:     "n" + itoa(i),
			Activation: activation,
		}
	}
	return nodes
}

// Simple int to string conversion for test helper
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	return result
}

// Helper function to sort pairs by activation product (descending)
// This mirrors the logic in ApplyCoactivation
func sortPairsByProduct(pairs []pair) {
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[i].ai*pairs[i].aj < pairs[j].ai*pairs[j].aj {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
}
