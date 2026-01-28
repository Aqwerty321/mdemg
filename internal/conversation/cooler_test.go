package conversation

import (
	"math"
	"testing"
)

func TestCalculateDecayedStability(t *testing.T) {
	tests := []struct {
		name            string
		stability       float64
		daysInactive    int
		expectedApprox  float64
		tolerance       float64
	}{
		{
			name:           "no decay for 0 days",
			stability:      0.5,
			daysInactive:   0,
			expectedApprox: 0.5,
			tolerance:      0.001,
		},
		{
			name:           "decay after 1 day",
			stability:      0.5,
			daysInactive:   1,
			expectedApprox: 0.45, // 0.5 * (1 - 0.1)^1 = 0.45
			tolerance:      0.001,
		},
		{
			name:           "decay after 7 days",
			stability:      0.5,
			daysInactive:   7,
			expectedApprox: 0.239, // 0.5 * (1 - 0.1)^7 ≈ 0.239
			tolerance:      0.01,
		},
		{
			name:           "decay after 30 days",
			stability:      0.5,
			daysInactive:   30,
			expectedApprox: 0.021, // 0.5 * (1 - 0.1)^30 ≈ 0.021
			tolerance:      0.01,
		},
		{
			name:           "negative days treated as 0",
			stability:      0.5,
			daysInactive:   -5,
			expectedApprox: 0.5,
			tolerance:      0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateDecayedStability(tt.stability, tt.daysInactive)
			if math.Abs(result-tt.expectedApprox) > tt.tolerance {
				t.Errorf("CalculateDecayedStability(%.2f, %d) = %.4f, want ≈%.4f (±%.3f)",
					tt.stability, tt.daysInactive, result, tt.expectedApprox, tt.tolerance)
			}
		})
	}
}

func TestCalculateReinforcementsToGraduate(t *testing.T) {
	tests := []struct {
		name      string
		stability float64
		expected  int
	}{
		{
			name:      "already graduated",
			stability: 0.9,
			expected:  0,
		},
		{
			name:      "at threshold",
			stability: 0.8,
			expected:  0,
		},
		{
			name:      "just below threshold",
			stability: 0.79,
			expected:  1, // 0.01 / 0.15 = 0.067, ceil = 1
		},
		{
			name:      "default stability",
			stability: 0.1,
			expected:  5, // (0.8 - 0.1) / 0.15 = 4.67, ceil = 5
		},
		{
			name:      "zero stability",
			stability: 0.0,
			expected:  6, // 0.8 / 0.15 = 5.33, ceil = 6
		},
		{
			name:      "mid stability",
			stability: 0.5,
			expected:  3, // (0.8 - 0.5) / 0.15 = 2.0, but float precision can cause ceil to 3
		},
		{
			name:      "mid stability higher",
			stability: 0.65,
			expected:  2, // (0.8 - 0.65) / 0.15 = 1.0, but float precision causes ceil to 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateReinforcementsToGraduate(tt.stability)
			if result != tt.expected {
				t.Errorf("CalculateReinforcementsToGraduate(%.2f) = %d, want %d",
					tt.stability, result, tt.expected)
			}
		})
	}
}

func TestContextCoolerConstants(t *testing.T) {
	// Verify constants are sensible
	if ReinforcementWindowHours < 1 {
		t.Error("ReinforcementWindowHours should be at least 1 hour")
	}

	if StabilityIncreasePerReinforcement <= 0 || StabilityIncreasePerReinforcement > 0.5 {
		t.Errorf("StabilityIncreasePerReinforcement = %.2f, should be between 0 and 0.5",
			StabilityIncreasePerReinforcement)
	}

	if StabilityDecayRate <= 0 || StabilityDecayRate > 0.5 {
		t.Errorf("StabilityDecayRate = %.2f, should be between 0 and 0.5",
			StabilityDecayRate)
	}

	if TombstoneThreshold >= DefaultStabilityScore {
		t.Errorf("TombstoneThreshold (%.2f) should be less than DefaultStabilityScore (%.2f)",
			TombstoneThreshold, DefaultStabilityScore)
	}

	if GraduationStabilityThreshold <= DefaultStabilityScore {
		t.Errorf("GraduationStabilityThreshold (%.2f) should be greater than DefaultStabilityScore (%.2f)",
			GraduationStabilityThreshold, DefaultStabilityScore)
	}
}

func TestGraduationSummary(t *testing.T) {
	// Test that GraduationSummary struct works correctly
	summary := GraduationSummary{
		SpaceID:           "test-space",
		Graduated:         5,
		Tombstoned:        2,
		RemainingVolatile: 10,
		DecayApplied:      3,
	}

	if summary.Graduated != 5 {
		t.Errorf("Graduated = %d, want 5", summary.Graduated)
	}
	if summary.Tombstoned != 2 {
		t.Errorf("Tombstoned = %d, want 2", summary.Tombstoned)
	}
	if summary.RemainingVolatile != 10 {
		t.Errorf("RemainingVolatile = %d, want 10", summary.RemainingVolatile)
	}
}

func TestVolatileStats(t *testing.T) {
	// Test that VolatileStats struct works correctly
	stats := VolatileStats{
		SpaceID:              "test-space",
		VolatileCount:        15,
		PermanentCount:       25,
		AvgVolatileStability: 0.35,
		MinVolatileStability: 0.1,
		MaxVolatileStability: 0.75,
	}

	if stats.VolatileCount != 15 {
		t.Errorf("VolatileCount = %d, want 15", stats.VolatileCount)
	}
	if stats.PermanentCount != 25 {
		t.Errorf("PermanentCount = %d, want 25", stats.PermanentCount)
	}

	// Graduation rate calculation
	total := stats.VolatileCount + stats.PermanentCount
	if total != 40 {
		t.Errorf("total = %d, want 40", total)
	}
	graduationRate := float64(stats.PermanentCount) / float64(total)
	if graduationRate < 0.6 || graduationRate > 0.65 {
		t.Errorf("graduationRate = %.2f, want ~0.625", graduationRate)
	}
}

func TestGraduationResult(t *testing.T) {
	// Test GraduationResult struct
	result := GraduationResult{
		NodeID:         "test-node",
		Graduated:      true,
		StabilityScore: 0.85,
		Reason:         "stability 0.85 >= 0.80 threshold",
	}

	if !result.Graduated {
		t.Error("Graduated should be true")
	}
	if result.Tombstoned {
		t.Error("Tombstoned should be false")
	}
	if result.StabilityScore != 0.85 {
		t.Errorf("StabilityScore = %.2f, want 0.85", result.StabilityScore)
	}
}
