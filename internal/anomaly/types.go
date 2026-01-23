package anomaly

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/models"
)

// DetectionContext holds context for anomaly detection during ingest
type DetectionContext struct {
	SpaceID   string
	NodeID    string
	Content   string
	Embedding []float32
	Tags      []string
	IsUpdate  bool // true if updating existing node
}

// Detector defines the interface for anomaly detection
type Detector interface {
	// Detect checks for anomalies and returns any detected issues.
	// Detection is non-blocking - errors are logged but don't fail ingest.
	Detect(ctx context.Context, dctx DetectionContext) []models.Anomaly
}

// Config holds configuration for anomaly detection
type Config struct {
	Enabled            bool
	DuplicateThreshold float64 // Vector similarity threshold (default: 0.95)
	OutlierStdDevs     float64 // Standard deviations for outlier (default: 2.0)
	StaleDays          int     // Days after which update is stale (default: 30)
	MaxCheckMs         int     // Maximum time for checks in ms (default: 100)
	VectorIndexName    string  // Name of the vector index
}

// Service implements the Detector interface
type Service struct {
	driver neo4j.DriverWithContext
	config Config
}
