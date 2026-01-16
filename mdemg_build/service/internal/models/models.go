package models

type RetrieveRequest struct {
	SpaceID        string    `json:"space_id" validate:"required,min=1,max=256"`
	QueryText      string    `json:"query_text,omitempty" validate:"required_without=QueryEmbedding,omitempty,min=1"`
	QueryEmbedding []float32 `json:"query_embedding,omitempty" validate:"required_without=QueryText,omitempty,embedding_dims"`

	CandidateK int `json:"candidate_k,omitempty" validate:"omitempty,min=1,max=1000"`
	TopK       int `json:"top_k,omitempty" validate:"omitempty,min=1,max=100"`
	HopDepth   int `json:"hop_depth,omitempty" validate:"omitempty,min=0,max=5"`

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
	SpaceID       string    `json:"space_id"`
	NodeID        string    `json:"node_id"`
	ObsID         string    `json:"obs_id"`
	EmbeddingDims int       `json:"embedding_dims,omitempty"` // Dimensions of generated embedding
	Anomalies     []Anomaly `json:"anomalies,omitempty"`      // Detected anomalies (non-blocking)
}

// AnomalyType represents the type of anomaly detected during ingest
type AnomalyType string

const (
	AnomalyContradiction AnomalyType = "contradiction" // Conflicts with existing node
	AnomalyDuplicate     AnomalyType = "duplicate"     // Very similar node exists
	AnomalyOutlier       AnomalyType = "outlier"       // Unusual for this space
	AnomalyStaleUpdate   AnomalyType = "stale_update"  // Updating old node
)

// Anomaly represents a detected anomaly during ingest
type Anomaly struct {
	Type        AnomalyType `json:"type"`
	Severity    string      `json:"severity"`               // "info", "warning", "critical"
	Message     string      `json:"message"`
	RelatedNode string      `json:"related_node,omitempty"` // Node ID of related node
	Confidence  float64     `json:"confidence"`             // 0.0 - 1.0
}

// MetricsResponse - main response structure for GET /v1/metrics
type MetricsResponse struct {
	TotalNodes     int64            `json:"total_nodes"`
	TotalEdges     int64            `json:"total_edges"`
	NodesByLayer   map[int]int64    `json:"nodes_by_layer"`
	EdgesByType    map[string]int64 `json:"edges_by_type"`
	AvgEdgeWeight  float64          `json:"avg_edge_weight"`
	HubNodes       []HubNode        `json:"hub_nodes"`       // top 10 by degree
	OrphanNodes    int64            `json:"orphan_nodes"`    // nodes with no edges
	RecentActivity *ActivityStats   `json:"recent_activity"` // last 24h
}

// HubNode - represents a high-connectivity node
type HubNode struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
	Degree int    `json:"degree"`
}

// ActivityStats - recent activity within 24h window
type ActivityStats struct {
	NodesCreated int64 `json:"nodes_created"`
	EdgesCreated int64 `json:"edges_created"`
	Retrievals   int64 `json:"retrievals"`
}

// ReflectRequest - request for deep context exploration via /v1/memory/reflect
type ReflectRequest struct {
	SpaceID        string    `json:"space_id"`
	Topic          string    `json:"topic"`                     // natural language topic (required)
	TopicEmbedding []float32 `json:"topic_embedding,omitempty"` // pre-computed embedding for topic
	MaxDepth       int       `json:"max_depth,omitempty"`       // hop depth (default: 3)
	MaxNodes       int       `json:"max_nodes,omitempty"`       // cap results (default: 50)
}

// ReflectResponse - response from deep context exploration
type ReflectResponse struct {
	Topic           string        `json:"topic"`
	CoreMemories    []ScoredNode  `json:"core_memories"`
	RelatedConcepts []ScoredNode  `json:"related_concepts"`
	Abstractions    []ScoredNode  `json:"abstractions"`
	Insights        []Insight     `json:"insights"`
	GraphContext    *GraphContext `json:"graph_context"`
}

// ScoredNode - a node with relevance scoring for reflection results
type ScoredNode struct {
	NodeID   string  `json:"node_id"`
	Name     string  `json:"name"`
	Path     string  `json:"path,omitempty"`
	Summary  string  `json:"summary,omitempty"`
	Layer    int     `json:"layer"`
	Score    float64 `json:"score"`    // relevance score
	Distance int     `json:"distance"` // hops from seed
}

// Insight - a detected pattern or observation from reflection
type Insight struct {
	Type        string   `json:"type"`        // "cluster", "pattern", "gap"
	Description string   `json:"description"`
	NodeIDs     []string `json:"node_ids"`
}

// GraphContext - traversal statistics from reflection
type GraphContext struct {
	NodesExplored   int `json:"nodes_explored"`
	EdgesTraversed  int `json:"edges_traversed"`
	MaxLayerReached int `json:"max_layer_reached"`
}

// BatchIngestRequest - request for batch ingest endpoint
type BatchIngestRequest struct {
	SpaceID      string            `json:"space_id"`
	Observations []BatchIngestItem `json:"observations"`
}

// BatchIngestItem - single observation in a batch ingest request
type BatchIngestItem struct {
	Timestamp   string    `json:"timestamp"`
	Source      string    `json:"source"`
	Content     any       `json:"content"`
	Tags        []string  `json:"tags,omitempty"`
	NodeID      string    `json:"node_id,omitempty"`
	Path        string    `json:"path,omitempty"`
	Name        string    `json:"name,omitempty"`
	Sensitivity string    `json:"sensitivity,omitempty"`
	Confidence  *float64  `json:"confidence,omitempty"`
	Embedding   []float32 `json:"embedding,omitempty"`
}

// BatchIngestResult - result for a single item in batch ingest
type BatchIngestResult struct {
	Index         int    `json:"index"`
	Status        string `json:"status"` // "success" or "error"
	NodeID        string `json:"node_id,omitempty"`
	ObsID         string `json:"obs_id,omitempty"`
	EmbeddingDims int    `json:"embedding_dims,omitempty"`
	Error         string `json:"error,omitempty"`
}

// BatchIngestResponse - response for batch ingest endpoint
type BatchIngestResponse struct {
	SpaceID      string              `json:"space_id"`
	TotalItems   int                 `json:"total_items"`
	SuccessCount int                 `json:"success_count"`
	ErrorCount   int                 `json:"error_count"`
	Results      []BatchIngestResult `json:"results"`
}
