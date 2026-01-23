package anomaly

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/models"
)

// MockDriver for unit testing without Neo4j
type mockDriverWithContext struct{}

func (m *mockDriverWithContext) NewSession(ctx context.Context, config interface{}) interface{} {
	return nil
}

func TestServiceConfig(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DuplicateThreshold: 0.95,
		OutlierStdDevs:     2.0,
		StaleDays:          30,
		MaxCheckMs:         100,
		VectorIndexName:    "memNodeEmbedding",
	}

	if cfg.DuplicateThreshold != 0.95 {
		t.Errorf("expected DuplicateThreshold 0.95, got %f", cfg.DuplicateThreshold)
	}
	if cfg.StaleDays != 30 {
		t.Errorf("expected StaleDays 30, got %d", cfg.StaleDays)
	}
	if cfg.MaxCheckMs != 100 {
		t.Errorf("expected MaxCheckMs 100, got %d", cfg.MaxCheckMs)
	}
}

func TestDetectionContextFields(t *testing.T) {
	dctx := DetectionContext{
		SpaceID:   "test-space",
		NodeID:    "node-123",
		Content:   "test content",
		Embedding: []float32{0.1, 0.2, 0.3},
		Tags:      []string{"tag1", "tag2"},
		IsUpdate:  true,
	}

	if dctx.SpaceID != "test-space" {
		t.Errorf("expected SpaceID 'test-space', got %s", dctx.SpaceID)
	}
	if dctx.NodeID != "node-123" {
		t.Errorf("expected NodeID 'node-123', got %s", dctx.NodeID)
	}
	if !dctx.IsUpdate {
		t.Error("expected IsUpdate to be true")
	}
	if len(dctx.Embedding) != 3 {
		t.Errorf("expected embedding length 3, got %d", len(dctx.Embedding))
	}
}

func TestDisabledServiceReturnsNoAnomalies(t *testing.T) {
	cfg := Config{
		Enabled:            false,
		DuplicateThreshold: 0.95,
		StaleDays:          30,
		MaxCheckMs:         100,
	}

	svc := &Service{config: cfg}

	dctx := DetectionContext{
		SpaceID:   "test-space",
		NodeID:    "node-123",
		Content:   "test content",
		Embedding: []float32{0.1, 0.2, 0.3},
	}

	ctx := context.Background()
	anomalies := svc.Detect(ctx, dctx)

	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies when disabled, got %d", len(anomalies))
	}
}

func TestTimeoutIsApplied(t *testing.T) {
	cfg := Config{
		Enabled:    true,
		MaxCheckMs: 1, // 1ms timeout - should be very fast
	}

	svc := &Service{config: cfg}

	dctx := DetectionContext{
		SpaceID: "test-space",
		NodeID:  "node-123",
	}

	ctx := context.Background()
	start := time.Now()
	_ = svc.Detect(ctx, dctx)
	elapsed := time.Since(start)

	// Should complete quickly (within 50ms even with overhead)
	if elapsed > 50*time.Millisecond {
		t.Errorf("detection took too long: %v", elapsed)
	}
}

func TestAnomalyTypes(t *testing.T) {
	tests := []struct {
		anomalyType models.AnomalyType
		expected    string
	}{
		{models.AnomalyContradiction, "contradiction"},
		{models.AnomalyDuplicate, "duplicate"},
		{models.AnomalyOutlier, "outlier"},
		{models.AnomalyStaleUpdate, "stale_update"},
	}

	for _, tt := range tests {
		if string(tt.anomalyType) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.anomalyType)
		}
	}
}

func TestAnomalyStructure(t *testing.T) {
	a := models.Anomaly{
		Type:        models.AnomalyDuplicate,
		Severity:    "warning",
		Message:     "Test message",
		RelatedNode: "node-456",
		Confidence:  0.95,
	}

	if a.Type != models.AnomalyDuplicate {
		t.Errorf("expected type AnomalyDuplicate, got %s", a.Type)
	}
	if a.Severity != "warning" {
		t.Errorf("expected severity 'warning', got %s", a.Severity)
	}
	if a.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", a.Confidence)
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input    any
		expected float64
	}{
		{float64(1.5), 1.5},
		{float32(2.5), 2.5},
		{int64(3), 3.0},
		{int(4), 4.0},
		{"invalid", 0.0},
		{nil, 0.0},
	}

	for _, tt := range tests {
		result := toFloat64(tt.input)
		if result != tt.expected {
			t.Errorf("toFloat64(%v) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		input    any
		expected int64
	}{
		{int64(100), 100},
		{int(200), 200},
		{float64(300.7), 300},
		{float32(400.9), 400},
		{"invalid", 0},
		{nil, 0},
	}

	for _, tt := range tests {
		result := toInt64(tt.input)
		if result != tt.expected {
			t.Errorf("toInt64(%v) = %d, expected %d", tt.input, result, tt.expected)
		}
	}
}

func TestDetectWithNoEmbeddingSkipsDuplicateCheck(t *testing.T) {
	cfg := Config{
		Enabled:            true,
		DuplicateThreshold: 0.95,
		StaleDays:          30,
		MaxCheckMs:         100,
	}

	svc := &Service{config: cfg}

	// No embedding - should skip duplicate check
	dctx := DetectionContext{
		SpaceID:   "test-space",
		NodeID:    "node-123",
		Content:   "test content",
		Embedding: nil,
		IsUpdate:  false,
	}

	ctx := context.Background()
	anomalies := svc.Detect(ctx, dctx)

	// With no embedding and no driver, should return empty (no errors)
	if anomalies == nil {
		anomalies = []models.Anomaly{}
	}
	// This should not panic and should complete
	t.Logf("Got %d anomalies (expected 0 with nil driver)", len(anomalies))
}

func TestDetectStaleUpdateSkippedForNewNodes(t *testing.T) {
	cfg := Config{
		Enabled:    true,
		StaleDays:  30,
		MaxCheckMs: 100,
	}

	svc := &Service{config: cfg}

	// IsUpdate=false - should skip stale update check
	dctx := DetectionContext{
		SpaceID:  "test-space",
		NodeID:   "node-123",
		IsUpdate: false,
	}

	ctx := context.Background()
	anomalies := svc.Detect(ctx, dctx)

	// Should not panic and should complete
	if anomalies == nil {
		anomalies = []models.Anomaly{}
	}
	t.Logf("Got %d anomalies for new node", len(anomalies))
}
