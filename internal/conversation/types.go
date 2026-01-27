package conversation

import "time"

// ObservationType represents what kind of observation this is
type ObservationType string

const (
	ObsTypeDecision   ObservationType = "decision"
	ObsTypeCorrection ObservationType = "correction"
	ObsTypeLearning   ObservationType = "learning"
	ObsTypePreference ObservationType = "preference"
	ObsTypeError      ObservationType = "error"
	ObsTypeTask       ObservationType = "task"
)

// Observation represents a significant conversational event
type Observation struct {
	ObsID         string
	SpaceID       string
	SessionID     string
	ObsType       ObservationType
	Content       string
	Summary       string // Auto-generated summary
	Embedding     []float32
	SurpriseScore float64 // 0.0-1.0
	Tags          []string
	Metadata      map[string]any
	CreatedAt     time.Time
}

// SurpriseFactors breaks down the surprise score
type SurpriseFactors struct {
	TermNovelty        float64 // Domain-specific terms
	ContradictionScore float64 // Conflicts with known facts
	CorrectionScore    float64 // User explicitly corrected
	EmbeddingNovelty   float64 // Distance from known concepts
}
