package models

type RetrieveRequest struct {
	SpaceID        string    `json:"space_id"`
	QueryText      string    `json:"query_text,omitempty"`
	QueryEmbedding []float32 `json:"query_embedding,omitempty"`

	CandidateK int `json:"candidate_k,omitempty"`
	TopK       int `json:"top_k,omitempty"`
	HopDepth   int `json:"hop_depth,omitempty"`

	PolicyContext map[string]any `json:"policy_context,omitempty"`
}

type RetrieveResult struct {
	NodeID  string  `json:"node_id"`
	Path    string  `json:"path"`
	Name    string  `json:"name"`
	Summary string  `json:"summary"`
	Score   float64 `json:"score"`

	VectorSim float64 `json:"vector_sim,omitempty"`
	Activation float64 `json:"activation,omitempty"`
}

type RetrieveResponse struct {
	SpaceID string           `json:"space_id"`
	Results []RetrieveResult `json:"results"`
	Debug   map[string]any   `json:"debug,omitempty"`
}

type IngestRequest struct {
	SpaceID     string    `json:"space_id"`
	Timestamp   string    `json:"timestamp"`
	Source      string    `json:"source"`
	Content     any       `json:"content"`
	Tags        []string  `json:"tags,omitempty"`
	NodeID      string    `json:"node_id,omitempty"`
	Path        string    `json:"path,omitempty"`
	Name        string    `json:"name,omitempty"`
	Sensitivity string    `json:"sensitivity,omitempty"`
	Confidence  *float64  `json:"confidence,omitempty"`
	Embedding   []float32 `json:"embedding,omitempty"` // Optional: pre-computed embedding
}

type IngestResponse struct {
	SpaceID       string `json:"space_id"`
	NodeID        string `json:"node_id"`
	ObsID         string `json:"obs_id"`
	EmbeddingDims int    `json:"embedding_dims,omitempty"` // Dimensions of generated embedding
}
