package main

import (
	"testing"
	"time"
)

// TestShouldPruneEdge verifies the edge pruning decision logic.
// From docs/07_Consolidation_and_Pruning.md Section 5.1:
// Prune edges if ALL are true:
// - weight < weight_threshold
// - evidence_count < min_evidence
// - updated_at older than olderThanDays
// - edge not pinned
// Special case: weight <= 0 always marks for pruning
func TestShouldPruneEdge(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		edge            edge
		weightThreshold float64
		minEvidence     int
		olderThanDays   int
		expectPrune     bool
		expectProtected bool
		expectReason    string
	}{
		// Low weight prune cases
		{
			name: "low weight, low evidence, old edge -> prune",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 2,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour), // 60 days old
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "zero weight -> always prune regardless of other factors",
			edge: edge{
				Weight:        0.0,
				EvidenceCount: 10, // high evidence should not protect
				Pinned:        true, // even pinned should not protect
				UpdatedAt:     now, // even recent should not protect
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "negative weight -> always prune",
			edge: edge{
				Weight:        -0.1,
				EvidenceCount: 10,
				Pinned:        true,
				UpdatedAt:     now,
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},

		// Pinned protection cases
		{
			name: "low weight, pinned -> protected",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        true,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned",
		},
		{
			name: "pinned edge with very low weight -> protected",
			edge: edge{
				Weight:        0.001,
				EvidenceCount: 0,
				Pinned:        true,
				UpdatedAt:     now.Add(-365 * 24 * time.Hour), // 1 year old
			},
			weightThreshold: 0.01,
			minEvidence:     5,
			olderThanDays:   7,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned",
		},

		// High evidence protection cases
		{
			name: "low weight, high evidence -> protected",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 5,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
		{
			name: "evidence exactly at threshold -> protected",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 3,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
		{
			name: "high evidence count protects old edge",
			edge: edge{
				Weight:        0.001,
				EvidenceCount: 10,
				Pinned:        false,
				UpdatedAt:     now.Add(-365 * 24 * time.Hour), // 1 year old
			},
			weightThreshold: 0.01,
			minEvidence:     5,
			olderThanDays:   7,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},

		// Weight above threshold cases
		{
			name: "weight above threshold -> not pruned",
			edge: edge{
				Weight:        0.5,
				EvidenceCount: 1,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "weight exactly at threshold -> not pruned",
			edge: edge{
				Weight:        0.01,
				EvidenceCount: 0,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "weight just above threshold -> not pruned",
			edge: edge{
				Weight:        0.011,
				EvidenceCount: 0,
				Pinned:        false,
				UpdatedAt:     now.Add(-365 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     5,
			olderThanDays:   7,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},

		// Age criterion cases (edge must be old enough to prune)
		{
			name: "low weight but too recent -> not pruned",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        false,
				UpdatedAt:     now.Add(-10 * 24 * time.Hour), // 10 days old, threshold is 30
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "edge exactly at age threshold -> not pruned",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        false,
				UpdatedAt:     now.Add(-30 * 24 * time.Hour), // exactly 30 days old
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "edge with zero timestamp -> prune (treated as very old)",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        false,
				UpdatedAt:     time.Time{}, // zero time
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},

		// Combined protection - pinned takes precedence over high_evidence
		{
			name: "pinned with high evidence -> protected by pinned",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 10,
				Pinned:        true,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned", // pinned is checked first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prune, protected, reason := shouldPruneEdge(
				tt.edge, tt.weightThreshold, tt.minEvidence, tt.olderThanDays, now,
			)

			if prune != tt.expectPrune {
				t.Errorf("shouldPruneEdge() prune = %v, want %v", prune, tt.expectPrune)
			}
			if protected != tt.expectProtected {
				t.Errorf("shouldPruneEdge() protected = %v, want %v", protected, tt.expectProtected)
			}
			if reason != tt.expectReason {
				t.Errorf("shouldPruneEdge() reason = %q, want %q", reason, tt.expectReason)
			}
		})
	}
}

// TestIsOlderThan verifies the age calculation for pruning
func TestIsOlderThan(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		updatedAt     time.Time
		olderThanDays int
		expectOlder   bool
	}{
		{
			name:          "60 days old, threshold 30 -> older",
			updatedAt:     now.Add(-60 * 24 * time.Hour),
			olderThanDays: 30,
			expectOlder:   true,
		},
		{
			name:          "10 days old, threshold 30 -> not older",
			updatedAt:     now.Add(-10 * 24 * time.Hour),
			olderThanDays: 30,
			expectOlder:   false,
		},
		{
			name:          "exactly at threshold -> not older",
			updatedAt:     now.Add(-30 * 24 * time.Hour),
			olderThanDays: 30,
			expectOlder:   false,
		},
		{
			name:          "zero time -> treated as very old",
			updatedAt:     time.Time{},
			olderThanDays: 30,
			expectOlder:   true,
		},
		{
			name:          "just updated -> not older",
			updatedAt:     now,
			olderThanDays: 1,
			expectOlder:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			older := isOlderThan(tt.updatedAt, tt.olderThanDays, now)

			if older != tt.expectOlder {
				t.Errorf("isOlderThan() = %v, want %v", older, tt.expectOlder)
			}
		})
	}
}

// TestProcessEdgeForPruning verifies the combined edge processing logic
func TestProcessEdgeForPruning(t *testing.T) {
	now := time.Now()
	cfg := pruneConfig{
		WeightThreshold: 0.01,
		MinEvidence:     3,
		OlderThanDays:   30,
	}

	tests := []struct {
		name            string
		edge            edge
		expectPrune     bool
		expectProtected bool
		expectReason    string
	}{
		{
			name: "low weight, low evidence, old -> prune",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 2,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "high evidence -> protected",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 5,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
		{
			name: "pinned -> protected",
			edge: edge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        true,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processEdgeForPruning(tt.edge, cfg, now)

			if result.ShouldPrune != tt.expectPrune {
				t.Errorf("processEdgeForPruning() ShouldPrune = %v, want %v",
					result.ShouldPrune, tt.expectPrune)
			}
			if result.Protected != tt.expectProtected {
				t.Errorf("processEdgeForPruning() Protected = %v, want %v",
					result.Protected, tt.expectProtected)
			}
			if result.ProtectReason != tt.expectReason {
				t.Errorf("processEdgeForPruning() ProtectReason = %q, want %q",
					result.ProtectReason, tt.expectReason)
			}

			// Verify edge is preserved in result
			if result.Edge.Weight != tt.edge.Weight {
				t.Errorf("processEdgeForPruning() Edge.Weight = %v, want %v",
					result.Edge.Weight, tt.edge.Weight)
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

	t.Run("asTime", func(t *testing.T) {
		if !asTime(nil).IsZero() {
			t.Error("asTime(nil) should return zero time")
		}
		now := time.Now()
		if !asTime(now).Equal(now) {
			t.Error("asTime(time.Time) should return the time")
		}
	})
}

// TestAsFloat32Slice verifies the embedding conversion helper
func TestAsFloat32Slice(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		if asFloat32Slice(nil) != nil {
			t.Error("asFloat32Slice(nil) should return nil")
		}
	})

	t.Run("float32 slice passthrough", func(t *testing.T) {
		input := []float32{0.1, 0.2, 0.3}
		result := asFloat32Slice(input)
		if len(result) != 3 || result[0] != 0.1 {
			t.Error("asFloat32Slice should passthrough []float32")
		}
	})

	t.Run("float64 slice conversion", func(t *testing.T) {
		input := []float64{0.1, 0.2, 0.3}
		result := asFloat32Slice(input)
		if len(result) != 3 {
			t.Errorf("asFloat32Slice should convert []float64, got len %d", len(result))
		}
	})

	t.Run("any slice conversion", func(t *testing.T) {
		input := []any{float64(0.1), float64(0.2)}
		result := asFloat32Slice(input)
		if len(result) != 2 {
			t.Errorf("asFloat32Slice should convert []any, got len %d", len(result))
		}
	})
}
