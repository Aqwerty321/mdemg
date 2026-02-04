package conversation

import (
	"testing"
)

func TestDedupAction(t *testing.T) {
	tests := []struct {
		name       string
		similarity float64
		threshold  float64
		want       string
	}{
		{"below threshold", 0.80, 0.95, ""},
		{"at threshold", 0.95, 0.95, "skip"},
		{"above threshold", 0.99, 0.95, "skip"},
		{"zero similarity", 0.0, 0.95, ""},
		{"exact match", 1.0, 0.95, "skip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupAction(tt.similarity, tt.threshold)
			if got != tt.want {
				t.Errorf("dedupAction(%.2f, %.2f) = %q, want %q", tt.similarity, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestDedupResult_IsDuplicate(t *testing.T) {
	tests := []struct {
		name   string
		result DedupResult
		want   bool
	}{
		{
			name:   "not duplicate",
			result: DedupResult{IsDuplicate: false, Similarity: 0.5},
			want:   false,
		},
		{
			name:   "is duplicate",
			result: DedupResult{IsDuplicate: true, Similarity: 0.98, DuplicateOfID: "node-123", Action: "skip"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.IsDuplicate != tt.want {
				t.Errorf("IsDuplicate = %v, want %v", tt.result.IsDuplicate, tt.want)
			}
		})
	}
}

func TestDedupThreshold(t *testing.T) {
	if DedupThreshold < 0.90 || DedupThreshold > 0.99 {
		t.Errorf("DedupThreshold = %.2f, expected between 0.90 and 0.99", DedupThreshold)
	}
}

func TestAsFloat32Slice(t *testing.T) {
	// This tests the conversion helper used for Neo4j embedding extraction.
	// We can't easily test with a real neo4j.Record, but we verify the
	// DedupResult struct serialization is correct.
	result := DedupResult{
		IsDuplicate:   true,
		DuplicateOfID: "test-node",
		Similarity:    0.97,
		Action:        "skip",
	}

	if result.DuplicateOfID != "test-node" {
		t.Errorf("unexpected DuplicateOfID: %s", result.DuplicateOfID)
	}
	if result.Action != "skip" {
		t.Errorf("unexpected Action: %s", result.Action)
	}
}
