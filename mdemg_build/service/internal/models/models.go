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
	SpaceID  string `json:"space_id"`
	Topic    string `json:"topic"`              // natural language topic (required)
	MaxDepth int    `json:"max_depth,omitempty"` // hop depth (default: 3)
	MaxNodes int    `json:"max_nodes,omitempty"` // cap results (default: 50)
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
