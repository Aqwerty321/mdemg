// Package hidden implements hierarchical graph convolution with hidden layers
// for generalized concept representations in MDEMG.
package hidden

import "time"

// HiddenNode represents a node in the hidden layer (layer 1)
type HiddenNode struct {
	NodeID               string
	SpaceID              string
	Name                 string
	Embedding            []float64
	MessagePassEmbedding []float64
	AggregationCount     int
	StabilityScore       float64
	LastForwardPass      *time.Time
	LastBackwardPass     *time.Time
}

// BaseNode represents a node in the base data layer (layer 0)
type BaseNode struct {
	NodeID    string
	SpaceID   string
	Embedding []float64
}

// ConceptNode represents a node in the concept layer (layer 2+)
type ConceptNode struct {
	NodeID               string
	SpaceID              string
	Layer                int
	Embedding            []float64
	MessagePassEmbedding []float64
}

// Cluster represents a group of base nodes that form a natural cluster
type Cluster struct {
	Members  []BaseNode
	Centroid []float64
}

// ClusteringResult holds the output of DBSCAN clustering
type ClusteringResult struct {
	Clusters      []Cluster
	NoisePoints   []BaseNode
	TotalPoints   int
	ClusterCount  int
	NoiseCount    int
}

// ForwardPassResult holds statistics from a forward pass operation
type ForwardPassResult struct {
	HiddenNodesUpdated  int
	ConceptNodesUpdated int
	Duration            time.Duration
}

// BackwardPassResult holds statistics from a backward pass operation
type BackwardPassResult struct {
	HiddenNodesUpdated int
	EdgesStrengthened  int
	Duration           time.Duration
}

// ConsolidationResult holds the combined results of a full consolidation run
type ConsolidationResult struct {
	HiddenNodesCreated  int
	ForwardPass         *ForwardPassResult
	BackwardPass        *BackwardPassResult
	TotalDuration       time.Duration
}

// Edge represents a relationship between nodes with weight
type Edge struct {
	SourceID string
	TargetID string
	Type     string
	Weight   float64
}
