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
	SpaceID     string    `json:"space_id" validate:"required,min=1,max=256"`
	Timestamp   string    `json:"timestamp" validate:"required,min=1"`
	Source      string    `json:"source" validate:"required,min=1,max=64"`
	Content     any       `json:"content" validate:"required"`
	Tags        []string  `json:"tags,omitempty" validate:"omitempty,dive,min=1"`
	NodeID      string    `json:"node_id,omitempty"`
	Path        string    `json:"path,omitempty" validate:"omitempty,max=512"`
	Name        string    `json:"name,omitempty"`
	Sensitivity string    `json:"sensitivity,omitempty" validate:"omitempty,oneof=public internal confidential"`
	Confidence  *float64  `json:"confidence,omitempty" validate:"omitempty,min=0,max=1"`
	Embedding   []float32 `json:"embedding,omitempty" validate:"omitempty,embedding_dims"` // Optional: pre-computed embedding
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
	SpaceID        string    `json:"space_id" validate:"required,min=1"`
	Topic          string    `json:"topic" validate:"required_without=TopicEmbedding,omitempty,min=1,max=500"`                     // natural language topic (required)
	TopicEmbedding []float32 `json:"topic_embedding,omitempty" validate:"required_without=Topic,omitempty,embedding_dims"` // pre-computed embedding for topic
	MaxDepth       int       `json:"max_depth,omitempty" validate:"omitempty,min=1,max=10"`       // hop depth (default: 3)
	MaxNodes       int       `json:"max_nodes,omitempty" validate:"omitempty,min=1,max=500"`       // cap results (default: 50)
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
	SpaceID      string            `json:"space_id" validate:"required,min=1"`
	Observations []BatchIngestItem `json:"observations" validate:"required,min=1,max=100,dive"`
}

// BatchIngestItem - single observation in a batch ingest request
type BatchIngestItem struct {
	Timestamp   string    `json:"timestamp" validate:"required,min=1"`
	Source      string    `json:"source" validate:"required,min=1,max=64"`
	Content     any       `json:"content" validate:"required"`
	Tags        []string  `json:"tags,omitempty" validate:"omitempty,dive,min=1"`
	NodeID      string    `json:"node_id,omitempty"`
	Path        string    `json:"path,omitempty" validate:"omitempty,max=512"`
	Name        string    `json:"name,omitempty"`
	Sensitivity string    `json:"sensitivity,omitempty" validate:"omitempty,oneof=public internal confidential"`
	Confidence  *float64  `json:"confidence,omitempty" validate:"omitempty,min=0,max=1"`
	Embedding   []float32 `json:"embedding,omitempty" validate:"omitempty,embedding_dims"`
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

// StatsResponse - response for GET /v1/memory/stats endpoint
// Provides comprehensive per-space memory statistics including counts,
// embedding coverage, learning metrics, and health indicators.
type StatsResponse struct {
	SpaceID               string               `json:"space_id"`
	MemoryCount           int64                `json:"memory_count"`
	ObservationCount      int64                `json:"observation_count"`
	MemoriesByLayer       map[int]int64        `json:"memories_by_layer"`
	EmbeddingCoverage     float64              `json:"embedding_coverage"`      // 0.0 - 1.0
	AvgEmbeddingDimensions int                 `json:"avg_embedding_dimensions"`
	LearningActivity      *LearningActivity    `json:"learning_activity"`
	TemporalDistribution  *TemporalDistribution `json:"temporal_distribution"`
	Connectivity          *Connectivity        `json:"connectivity"`
	HealthScore           float64              `json:"health_score"` // 0.0 - 1.0
	ComputedAt            string               `json:"computed_at"`  // ISO8601 timestamp
}

// LearningActivity - Hebbian learning metrics from CO_ACTIVATED_WITH edges
type LearningActivity struct {
	CoActivatedEdges int64   `json:"co_activated_edges"`
	AvgWeight        float64 `json:"avg_weight"`
	MaxWeight        float64 `json:"max_weight"`
}

// TemporalDistribution - memory creation counts over time periods
type TemporalDistribution struct {
	Last24h int64 `json:"last_24h"`
	Last7d  int64 `json:"last_7d"`
	Last30d int64 `json:"last_30d"`
}

// Connectivity - graph connectivity statistics for a space
type Connectivity struct {
	AvgDegree   float64 `json:"avg_degree"`
	MaxDegree   int     `json:"max_degree"`
	OrphanCount int64   `json:"orphan_count"`
}

// ArchiveRequest - request for archiving a memory node
type ArchiveRequest struct {
	Reason string `json:"reason,omitempty"` // optional reason for archiving
}

// ArchiveResponse - response from archive endpoint
type ArchiveResponse struct {
	NodeID     string `json:"node_id"`
	Name       string `json:"name"`
	ArchivedAt string `json:"archived_at"`
	Reason     string `json:"reason,omitempty"`
}

// UnarchiveResponse - response from unarchive endpoint
type UnarchiveResponse struct {
	NodeID       string `json:"node_id"`
	Name         string `json:"name"`
	UnarchivedAt string `json:"unarchived_at"`
}

// DeleteResponse - response from delete endpoint
type DeleteResponse struct {
	NodeID       string `json:"node_id"`
	DeletedNodes int    `json:"deleted_nodes"`
	DeletedEdges int    `json:"deleted_edges"`
}

// BulkArchiveRequest - request for bulk archiving memory nodes
type BulkArchiveRequest struct {
	SpaceID string   `json:"space_id"`
	NodeIDs []string `json:"node_ids"`
	Reason  string   `json:"reason,omitempty"` // optional reason for archiving
}

// BulkArchiveResult - result for a single item in bulk archive
type BulkArchiveResult struct {
	NodeID     string `json:"node_id"`
	Status     string `json:"status"` // "success" or "error"
	ArchivedAt string `json:"archived_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BulkArchiveResponse - response for bulk archive endpoint
type BulkArchiveResponse struct {
	SpaceID      string              `json:"space_id"`
	TotalItems   int                 `json:"total_items"`
	SuccessCount int                 `json:"success_count"`
	ErrorCount   int                 `json:"error_count"`
	Results      []BulkArchiveResult `json:"results"`
}

// ConsolidateRequest - request for POST /v1/memory/consolidate
// Triggers hidden layer creation and message passing operations.
type ConsolidateRequest struct {
	SpaceID       string `json:"space_id" validate:"required,min=1,max=256"`
	SkipClustering bool   `json:"skip_clustering,omitempty"` // Skip DBSCAN clustering, only run message passing
	SkipForward   bool   `json:"skip_forward,omitempty"`    // Skip forward pass
	SkipBackward  bool   `json:"skip_backward,omitempty"`   // Skip backward pass
}

// ConsolidateResponse - response from consolidate endpoint
type ConsolidateResponse struct {
	SpaceID             string  `json:"space_id"`
	HiddenNodesCreated  int     `json:"hidden_nodes_created"`
	HiddenNodesUpdated  int     `json:"hidden_nodes_updated"`
	ConceptNodesCreated int     `json:"concept_nodes_created"` // Number of concept layer nodes created (L2+)
	ConceptNodesUpdated int     `json:"concept_nodes_updated"`
	SummariesGenerated  int     `json:"summaries_generated"` // Number of summaries generated
	EdgesStrengthened   int     `json:"edges_strengthened"`
	DurationMs          float64 `json:"duration_ms"`
	Enabled             bool    `json:"enabled"` // Whether hidden layer is enabled
}
