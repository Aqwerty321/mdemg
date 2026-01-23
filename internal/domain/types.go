package domain

import "time"

type MemoryNode struct {
	SpaceID     string    `json:"space_id"`
	NodeID      string    `json:"node_id"`
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	Layer       int       `json:"layer"`
	RoleType    string    `json:"role_type"`
	Version     int       `json:"version,omitempty"`
	Description string    `json:"description,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	Confidence  float64   `json:"confidence,omitempty"`
	Sensitivity string    `json:"sensitivity,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Embedding   []float64 `json:"embedding,omitempty"`

	// Hidden layer support (V0005)
	MessagePassEmbedding []float64  `json:"message_pass_embedding,omitempty"`
	LastForwardPass      *time.Time `json:"last_forward_pass,omitempty"`
	LastBackwardPass     *time.Time `json:"last_backward_pass,omitempty"`
	AggregationCount     int        `json:"aggregation_count,omitempty"`
	StabilityScore       float64    `json:"stability_score,omitempty"`
}

type Observation struct {
	SpaceID    string    `json:"space_id"`
	ObsID      string    `json:"obs_id"`
	Timestamp  time.Time `json:"timestamp"`
	Source     string    `json:"source,omitempty"`
	Content    string    `json:"content"`
	Embedding  []float64 `json:"embedding,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
}

type RetrievalRequest struct {
	SpaceID     string `json:"space_id"`
	QueryText   string `json:"query_text,omitempty"`
	QueryEmbedding []float64 `json:"query_embedding,omitempty"`
	TopK        int    `json:"top_k,omitempty"`
	CandidateK  int    `json:"candidate_k,omitempty"`
	HopDepth    int    `json:"hop_depth,omitempty"`
	DegreeCap   int    `json:"degree_cap,omitempty"`
	LearnTopK   int    `json:"learn_top_k,omitempty"`
	Explain     bool   `json:"explain,omitempty"`
}

type RetrievalResult struct {
	NodeID     string                 `json:"node_id"`
	Path       string                 `json:"path"`
	Name       string                 `json:"name"`
	Summary    string                 `json:"summary"`
	Score      float64                `json:"score"`
	Components map[string]float64     `json:"components,omitempty"`
	Explain    []map[string]any       `json:"explain,omitempty"`
}
