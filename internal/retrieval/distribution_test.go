package retrieval

import (
	"testing"
)

func TestComputeDistribution(t *testing.T) {
	tests := []struct {
		name   string
		scores []float64
		wantN  int
	}{
		{
			name:   "empty scores",
			scores: []float64{},
			wantN:  0,
		},
		{
			name:   "single score",
			scores: []float64{0.5},
			wantN:  1,
		},
		{
			name:   "uniform scores",
			scores: []float64{0.5, 0.5, 0.5, 0.5, 0.5},
			wantN:  5,
		},
		{
			name:   "varied scores",
			scores: []float64{0.1, 0.3, 0.5, 0.7, 0.9},
			wantN:  5,
		},
		{
			name:   "typical retrieval scores",
			scores: []float64{0.85, 0.72, 0.68, 0.55, 0.42, 0.38, 0.35, 0.30, 0.28, 0.25},
			wantN:  10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dist := ComputeDistribution(tt.scores)
			if dist.Count != tt.wantN {
				t.Errorf("Count = %d, want %d", dist.Count, tt.wantN)
			}

			if tt.wantN > 0 {
				// Verify min/max
				if dist.Min > dist.Max {
					t.Errorf("Min (%f) > Max (%f)", dist.Min, dist.Max)
				}

				// Verify range
				if dist.Range != dist.Max-dist.Min {
					t.Errorf("Range = %f, want %f", dist.Range, dist.Max-dist.Min)
				}

				// Verify percentiles are ordered
				if dist.P10 > dist.P25 || dist.P25 > dist.P50 || dist.P50 > dist.P75 || dist.P75 > dist.P90 {
					t.Errorf("Percentiles not ordered: P10=%f P25=%f P50=%f P75=%f P90=%f",
						dist.P10, dist.P25, dist.P50, dist.P75, dist.P90)
				}

				// Verify std dev is non-negative
				if dist.StdDev < 0 {
					t.Errorf("StdDev = %f, want >= 0", dist.StdDev)
				}
			}
		})
	}
}

func TestComputeDistribution_Stats(t *testing.T) {
	// Test with known values
	scores := []float64{0.1, 0.3, 0.5, 0.7, 0.9}
	dist := ComputeDistribution(scores)

	// Mean should be 0.5
	if dist.Mean < 0.49 || dist.Mean > 0.51 {
		t.Errorf("Mean = %f, want ~0.5", dist.Mean)
	}

	// Min/Max
	if dist.Min != 0.1 {
		t.Errorf("Min = %f, want 0.1", dist.Min)
	}
	if dist.Max != 0.9 {
		t.Errorf("Max = %f, want 0.9", dist.Max)
	}

	// Median (P50) should be 0.5
	if dist.P50 != 0.5 {
		t.Errorf("P50 = %f, want 0.5", dist.P50)
	}
}

func TestDistributionMonitor_RecordAndAlert(t *testing.T) {
	monitor := NewDistributionMonitor()

	// Record a distribution with low std dev (should trigger alert)
	lowVarianceScores := []float64{0.5, 0.51, 0.49, 0.50, 0.51, 0.50}
	dist := ComputeDistribution(lowVarianceScores)
	alerts := monitor.RecordDistribution("test-space", dist)

	// Should have distribution_compressed alert
	hasCompressedAlert := false
	for _, a := range alerts {
		if a.Type == "distribution_compressed" {
			hasCompressedAlert = true
		}
	}
	if !hasCompressedAlert {
		t.Error("Expected distribution_compressed alert for low variance scores")
	}
}

func TestDistributionMonitor_LearningPhase(t *testing.T) {
	monitor := NewDistributionMonitor()

	tests := []struct {
		edgeCount int64
		wantPhase LearningPhase
	}{
		{0, PhaseCold},
		{100, PhaseLearning},
		{5000, PhaseLearning},
		{10000, PhaseWarm},
		{30000, PhaseWarm},
		{50000, PhaseSaturated},
		{100000, PhaseSaturated},
	}

	for _, tt := range tests {
		phase := monitor.UpdateEdgeCount("test-space", tt.edgeCount)
		if phase != tt.wantPhase {
			t.Errorf("UpdateEdgeCount(%d) = %s, want %s", tt.edgeCount, phase, tt.wantPhase)
		}
	}
}

func TestDistributionMonitor_GetStats(t *testing.T) {
	monitor := NewDistributionMonitor()
	spaceID := "test-space"

	// Initially empty
	stats := monitor.GetStats(spaceID)
	if stats.QueryCount != 0 {
		t.Errorf("Initial QueryCount = %d, want 0", stats.QueryCount)
	}
	if stats.Phase != PhaseCold {
		t.Errorf("Initial Phase = %s, want cold", stats.Phase)
	}

	// Record some distributions
	for i := 0; i < 15; i++ {
		scores := []float64{0.8, 0.7, 0.6, 0.5, 0.4}
		dist := ComputeDistribution(scores)
		monitor.RecordDistribution(spaceID, dist)
	}

	// Update edge count
	monitor.UpdateEdgeCount(spaceID, 15000)

	stats = monitor.GetStats(spaceID)
	if stats.QueryCount != 15 {
		t.Errorf("QueryCount = %d, want 15", stats.QueryCount)
	}
	if stats.Phase != PhaseWarm {
		t.Errorf("Phase = %s, want warm", stats.Phase)
	}
	if stats.Latest == nil {
		t.Error("Latest should not be nil")
	}
	if stats.AggregatedStats == nil {
		t.Error("AggregatedStats should not be nil")
	}
	if stats.Trend == nil {
		t.Error("Trend should not be nil with 15 samples")
	}
}

func TestPercentile(t *testing.T) {
	sorted := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}

	tests := []struct {
		p    float64
		want float64
	}{
		{0.0, 0.1},
		{0.5, 0.55}, // Interpolated
		{1.0, 1.0},
		{0.25, 0.325}, // Interpolated
	}

	for _, tt := range tests {
		got := percentile(sorted, tt.p)
		if got < tt.want-0.01 || got > tt.want+0.01 {
			t.Errorf("percentile(sorted, %f) = %f, want ~%f", tt.p, got, tt.want)
		}
	}
}

func TestTrendDirection(t *testing.T) {
	tests := []struct {
		delta float64
		want  string
	}{
		{0.15, "improving"},
		{0.06, "improving"},
		{0.02, "stable"},
		{-0.02, "stable"},
		{-0.06, "degrading"},
		{-0.15, "degrading"},
	}

	for _, tt := range tests {
		got := trendDirection(tt.delta)
		if got != tt.want {
			t.Errorf("trendDirection(%f) = %s, want %s", tt.delta, got, tt.want)
		}
	}
}
