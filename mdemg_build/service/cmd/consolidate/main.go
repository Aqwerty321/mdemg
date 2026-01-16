package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// consolidateConfig holds CLI and environment configuration for the consolidation job
type consolidateConfig struct {
	// Neo4j connection
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string

	// Consolidation parameters
	MinClusterSize  int     // default: 3
	WeightThreshold float64 // default: 0.5
	MaxPromotions   int     // default: 50

	// Processing options
	DryRun  bool
	SpaceID string // REQUIRED
}

func main() {
	// Print banner first (always shown, even on error)
	fmt.Println("MDEMG Consolidation Job")
	fmt.Println("=======================")
	fmt.Println()

	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	driver, err := newDriver(cfg)
	if err != nil {
		log.Fatalf("failed to create neo4j driver: %v", err)
	}
	defer driver.Close(ctx)

	// Verify connectivity
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("failed to connect to neo4j: %v", err)
	}

	// Run the consolidation job
	if err := runConsolidationJob(ctx, driver, cfg); err != nil {
		log.Fatalf("consolidation job failed: %v", err)
	}
}

// parseConfig parses CLI flags and environment variables
func parseConfig() (consolidateConfig, error) {
	var cfg consolidateConfig

	// CLI flags with defaults
	flag.IntVar(&cfg.MinClusterSize, "min-cluster-size", 3, "Minimum number of nodes to form a cluster")
	flag.Float64Var(&cfg.WeightThreshold, "weight-threshold", 0.5, "Minimum CO_ACTIVATED_WITH weight to consider")
	flag.IntVar(&cfg.MaxPromotions, "max-promotions", 50, "Maximum number of abstraction nodes to create")
	flag.BoolVar(&cfg.DryRun, "dry-run", true, "Preview mode - no modifications (default: true)")
	flag.StringVar(&cfg.SpaceID, "space-id", "", "Space ID to process (REQUIRED)")

	flag.Parse()

	// Environment variables for Neo4j connection
	get := func(k, def string) string {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def
		}
		return v
	}

	cfg.Neo4jURI = get("NEO4J_URI", "")
	cfg.Neo4jUser = get("NEO4J_USER", "")
	cfg.Neo4jPass = get("NEO4J_PASS", "")

	if cfg.Neo4jURI == "" || cfg.Neo4jUser == "" || cfg.Neo4jPass == "" {
		return consolidateConfig{}, errors.New("NEO4J_URI/NEO4J_USER/NEO4J_PASS environment variables are required")
	}

	// Validate required flags
	if cfg.SpaceID == "" {
		return consolidateConfig{}, errors.New("--space-id is required")
	}

	// Validate flag values
	if cfg.MinClusterSize < 2 {
		return consolidateConfig{}, errors.New("min-cluster-size must be at least 2")
	}
	if cfg.WeightThreshold < 0 || cfg.WeightThreshold > 1 {
		return consolidateConfig{}, errors.New("weight-threshold must be between 0 and 1")
	}
	if cfg.MaxPromotions <= 0 {
		return consolidateConfig{}, errors.New("max-promotions must be positive")
	}

	return cfg, nil
}

// newDriver creates a new Neo4j driver with the given configuration
func newDriver(cfg consolidateConfig) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		return nil, err
	}
	return driver, nil
}

// consolidateStats tracks statistics for the consolidation job
type consolidateStats struct {
	clustersFound    int
	nodesPromoted    int
	edgesCreated     int
	skippedNoEmbed   int
	skippedTooSmall  int
}

// clusterCandidate represents a node with its high-weight neighbors at the same layer
type clusterCandidate struct {
	NodeID      string
	Layer       int
	Embedding   []float64
	NeighborIDs []string
}

// clusterMember represents a single node within a cluster
type clusterMember struct {
	NodeID    string
	Embedding []float64
}

// cluster represents a group of co-activated nodes at the same layer
type cluster struct {
	Members []clusterMember
	Layer   int
}

// runConsolidationJob executes the cluster detection and abstraction promotion
func runConsolidationJob(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) error {
	// Print header
	printHeader(cfg)

	fmt.Println("\nProcessing...")

	stats := consolidateStats{}

	// Step 1: Query cluster candidates from Neo4j
	fmt.Println("\nStep 1: Detecting cluster candidates...")
	candidates, err := queryClusterCandidates(ctx, driver, cfg)
	if err != nil {
		return fmt.Errorf("query cluster candidates: %w", err)
	}
	fmt.Printf("Found %d nodes with sufficient high-weight neighbors\n", len(candidates))

	if len(candidates) == 0 {
		fmt.Println("\nNo cluster candidates found. Nothing to promote.")
		printStats(stats, cfg.DryRun)
		return nil
	}

	// Step 2: Build clusters from candidates using greedy first-come assignment
	fmt.Println("\nStep 2: Building clusters...")
	clusters := buildClusters(candidates, cfg.MinClusterSize)
	stats.clustersFound = len(clusters)
	fmt.Printf("Formed %d clusters (min size: %d)\n", len(clusters), cfg.MinClusterSize)

	if len(clusters) == 0 {
		fmt.Println("\nNo clusters met the minimum size requirement. Nothing to promote.")
		printStats(stats, cfg.DryRun)
		return nil
	}

	// Step 3: Process clusters and create abstractions
	fmt.Println("\nStep 3: Processing clusters for abstraction promotion...")
	promotionCount := 0

	for i, c := range clusters {
		// Respect max promotions cap
		if promotionCount >= cfg.MaxPromotions {
			fmt.Printf("\nReached max promotions cap (%d). Stopping.\n", cfg.MaxPromotions)
			break
		}

		// Collect embeddings from cluster members
		var embeddings [][]float64
		for _, member := range c.Members {
			if len(member.Embedding) > 0 {
				embeddings = append(embeddings, member.Embedding)
			} else {
				stats.skippedNoEmbed++
			}
		}

		// Calculate averaged embedding for the abstraction node
		avgEmbedding := averageEmbeddings(embeddings)
		if avgEmbedding == nil {
			fmt.Printf("  Cluster %d: Skipped (no valid embeddings)\n", i+1)
			stats.skippedTooSmall++
			continue
		}

		// Report cluster info
		memberIDs := make([]string, 0, len(c.Members))
		for _, m := range c.Members {
			memberIDs = append(memberIDs, m.NodeID)
		}
		fmt.Printf("  Cluster %d: %d members at layer %d -> layer %d\n",
			i+1, len(c.Members), c.Layer, c.Layer+1)

		if cfg.DryRun {
			// Dry-run mode: just count what would happen
			stats.nodesPromoted++
			stats.edgesCreated += len(c.Members)
		} else {
			// Live mode: create abstraction node and edges
			result, err := createAbstraction(ctx, driver, cfg, c, avgEmbedding)
			if err != nil {
				return fmt.Errorf("create abstraction for cluster %d: %w", i+1, err)
			}
			fmt.Printf("    Created abstraction node: %s (%d edges)\n", result.NodeID, result.MemberCount)
			stats.nodesPromoted++
			stats.edgesCreated += result.MemberCount
		}

		promotionCount++
	}

	// Print statistics
	printStats(stats, cfg.DryRun)

	return nil
}

// printHeader outputs the job configuration header
func printHeader(cfg consolidateConfig) {
	if cfg.DryRun {
		fmt.Println("Mode: DRY RUN (no changes will be made)")
	} else {
		fmt.Println("Mode: LIVE (changes will be applied)")
	}

	fmt.Printf("Space: %s\n", cfg.SpaceID)
	fmt.Printf("Min cluster size: %d\n", cfg.MinClusterSize)
	fmt.Printf("Weight threshold: %g\n", cfg.WeightThreshold)
	fmt.Printf("Max promotions: %d\n", cfg.MaxPromotions)
}

// printStats outputs the job statistics
func printStats(stats consolidateStats, dryRun bool) {
	fmt.Println("\nStatistics:")
	fmt.Printf("- Clusters found: %d\n", stats.clustersFound)
	if dryRun {
		fmt.Printf("- Nodes to promote: %d\n", stats.nodesPromoted)
		fmt.Printf("- Edges to create: %d\n", stats.edgesCreated)
	} else {
		fmt.Printf("- Nodes promoted: %d\n", stats.nodesPromoted)
		fmt.Printf("- Edges created: %d\n", stats.edgesCreated)
	}
	fmt.Printf("- Skipped (no embedding): %d\n", stats.skippedNoEmbed)
	fmt.Printf("- Skipped (too small): %d\n", stats.skippedTooSmall)

	if dryRun {
		fmt.Println("\nRun with --dry-run=false to apply changes.")
	} else {
		fmt.Println("\nChanges applied successfully.")
	}
}

// queryClusterCandidates fetches nodes with sufficient high-weight neighbors at the same layer.
// Each returned candidate has ≥ (minSize - 1) neighbors with CO_ACTIVATED_WITH weight ≥ threshold.
// This is the first step in cluster detection - nodes are later grouped into actual clusters.
func queryClusterCandidates(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) ([]clusterCandidate, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Find nodes with enough high-weight neighbors at same layer
	// The query returns nodes where each has at least (minSize-1) neighbors with weight >= threshold
	// This means when combined with the node itself, we have >= minSize nodes in potential cluster
	cypher := `
MATCH (a:MemoryNode)-[r:CO_ACTIVATED_WITH]-(b:MemoryNode)
WHERE a.space_id = $spaceId
  AND r.weight >= $threshold
  AND a.layer = b.layer
WITH a, collect(DISTINCT b) AS neighbors
WHERE size(neighbors) >= $minNeighbors
RETURN a.node_id AS nodeId,
       a.layer AS layer,
       a.embedding AS embedding,
       [n IN neighbors | n.node_id] AS neighborIds
ORDER BY size(neighbors) DESC`

	params := map[string]any{
		"spaceId":      cfg.SpaceID,
		"threshold":    cfg.WeightThreshold,
		"minNeighbors": cfg.MinClusterSize - 1, // need minSize-1 neighbors to form cluster of minSize
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		candidates := make([]clusterCandidate, 0)
		for res.Next(ctx) {
			rec := res.Record()

			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			embedding, _ := rec.Get("embedding")
			neighborIDs, _ := rec.Get("neighborIds")

			candidate := clusterCandidate{
				NodeID:      asString(nodeID),
				Layer:       asInt(layer),
				Embedding:   asFloat64Slice(embedding),
				NeighborIDs: asStringSlice(neighborIDs),
			}
			candidates = append(candidates, candidate)
		}

		if err := res.Err(); err != nil {
			return nil, err
		}
		return candidates, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]clusterCandidate), nil
}

// buildClusters groups cluster candidates into non-overlapping clusters using greedy first-come assignment.
// Each node can only belong to one cluster. Candidates are processed in order (highest neighbor count first,
// as returned by queryClusterCandidates). For each unassigned candidate, we form a cluster from the candidate
// plus any of its unassigned neighbors. Clusters smaller than minSize are discarded.
func buildClusters(candidates []clusterCandidate, minSize int) []cluster {
	// Build a map of candidate node IDs to their data for quick lookup
	candidateMap := make(map[string]clusterCandidate)
	for _, c := range candidates {
		candidateMap[c.NodeID] = c
	}

	// Track which nodes have been assigned to a cluster
	assigned := make(map[string]bool)

	// Result clusters
	var clusters []cluster

	// Process candidates in order (already sorted by neighbor count descending)
	for _, candidate := range candidates {
		// Skip if this node is already assigned to a cluster
		if assigned[candidate.NodeID] {
			continue
		}

		// Build cluster starting from this candidate
		var members []clusterMember

		// Add the candidate itself
		members = append(members, clusterMember{
			NodeID:    candidate.NodeID,
			Embedding: candidate.Embedding,
		})

		// Add unassigned neighbors
		for _, neighborID := range candidate.NeighborIDs {
			if assigned[neighborID] {
				continue
			}

			// Get neighbor's embedding from candidateMap if available
			var neighborEmbedding []float64
			if neighborCandidate, exists := candidateMap[neighborID]; exists {
				neighborEmbedding = neighborCandidate.Embedding
			}
			// Note: If neighbor is not in candidateMap, it means it didn't have
			// enough neighbors to be a candidate itself, but we still include it
			// in this cluster. Its embedding may be nil.

			members = append(members, clusterMember{
				NodeID:    neighborID,
				Embedding: neighborEmbedding,
			})
		}

		// Only keep clusters that meet minimum size requirement
		if len(members) < minSize {
			continue
		}

		// Mark all members as assigned
		for _, member := range members {
			assigned[member.NodeID] = true
		}

		// Add cluster to results
		clusters = append(clusters, cluster{
			Members: members,
			Layer:   candidate.Layer,
		})
	}

	return clusters
}

// asStringSlice safely converts interface{} to []string
func asStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	// Neo4j returns arrays as []interface{} or []any
	if arr, ok := v.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			result = append(result, asString(item))
		}
		return result
	}
	// Direct string slice (rare but possible)
	if arr, ok := v.([]string); ok {
		return arr
	}
	return nil
}

// asString safely converts interface{} to string
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// asFloat64 safely converts interface{} to float64
func asFloat64(v any) float64 {
	if v == nil {
		return 0.0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0.0
	}
}

// asInt safely converts interface{} to int
func asInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

// asBool safely converts interface{} to bool
func asBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// asFloat64Slice safely converts interface{} to []float64 for embeddings
func asFloat64Slice(v any) []float64 {
	if v == nil {
		return nil
	}
	// Neo4j returns arrays as []interface{} or []any
	if arr, ok := v.([]any); ok {
		result := make([]float64, 0, len(arr))
		for _, item := range arr {
			result = append(result, asFloat64(item))
		}
		return result
	}
	// Direct float64 slice (rare but possible)
	if arr, ok := v.([]float64); ok {
		return arr
	}
	return nil
}

// abstractionResult holds the result of creating an abstraction node
type abstractionResult struct {
	NodeID      string
	MemberCount int
}

// createAbstraction creates a new MemoryNode at layer+1 and ABSTRACTS_TO edges from cluster members.
// Returns the new abstraction node's ID and the count of edges created.
// This function performs actual database writes - only call when dryRun is false.
func createAbstraction(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig, c cluster, embedding []float64) (*abstractionResult, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Generate name and summary from cluster members
	name := generateAbstractionName(c.Members)
	summary := fmt.Sprintf("Cluster abstraction of %d nodes at layer %d", len(c.Members), c.Layer)
	newLayer := c.Layer + 1

	// Collect member node IDs
	memberIDs := make([]string, 0, len(c.Members))
	for _, m := range c.Members {
		memberIDs = append(memberIDs, m.NodeID)
	}

	params := map[string]any{
		"spaceId":   cfg.SpaceID,
		"name":      name,
		"summary":   summary,
		"layer":     newLayer,
		"embedding": embedding,
		"memberIds": memberIDs,
	}

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Cypher query creates abstraction node and ABSTRACTS_TO edges:
		// - CREATE for the new abstraction node with all required properties
		// - UNWIND + MATCH to find each cluster member
		// - CREATE for ABSTRACTS_TO edges with required properties
		cypher := `
CREATE (abs:MemoryNode {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $name,
  summary: $summary,
  layer: $layer,
  embedding: $embedding,
  created_at: datetime(),
  updated_at: datetime(),
  role_type: 'abstraction',
  version: 1
})
WITH abs
UNWIND $memberIds AS memberId
MATCH (m:MemoryNode {space_id: $spaceId, node_id: memberId})
CREATE (m)-[:ABSTRACTS_TO {
  space_id: $spaceId,
  edge_id: randomUUID(),
  created_at: datetime(),
  updated_at: datetime()
}]->(abs)
RETURN abs.node_id AS absNodeId, count(m) AS memberCount`

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		// Expect exactly one result record
		if res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("absNodeId")
			memberCount, _ := rec.Get("memberCount")
			return &abstractionResult{
				NodeID:      asString(nodeID),
				MemberCount: asInt(memberCount),
			}, nil
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		// No result returned - shouldn't happen if query is correct
		return nil, errors.New("no result returned from abstraction creation query")
	})

	if err != nil {
		return nil, err
	}
	return result.(*abstractionResult), nil
}

// generateAbstractionName creates a descriptive name for the abstraction node
// based on cluster member node IDs, truncating if too long
func generateAbstractionName(members []clusterMember) string {
	if len(members) == 0 {
		return "Abstraction: (empty)"
	}

	// Collect node IDs
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.NodeID)
	}

	// Join IDs with commas
	joined := strings.Join(ids, ", ")

	// Truncate if too long (max ~60 chars for readability)
	const maxLen = 60
	if len(joined) > maxLen {
		joined = joined[:maxLen-3] + "..."
	}

	return fmt.Sprintf("Abstraction: [%s]", joined)
}

// averageEmbeddings computes the centroid (element-wise average) of multiple embedding vectors.
// Returns nil if embeddings is empty. Skips embeddings with mismatched dimensions.
func averageEmbeddings(embeddings [][]float64) []float64 {
	if len(embeddings) == 0 {
		return nil
	}

	// Find the first non-nil, non-empty embedding to determine dimensions
	var dim int
	for _, emb := range embeddings {
		if len(emb) > 0 {
			dim = len(emb)
			break
		}
	}
	if dim == 0 {
		return nil
	}

	result := make([]float64, dim)
	validCount := 0

	for _, emb := range embeddings {
		// Skip nil, empty, or mismatched dimension embeddings
		if len(emb) != dim {
			continue
		}
		for i, v := range emb {
			result[i] += v
		}
		validCount++
	}

	// If no valid embeddings were found, return nil
	if validCount == 0 {
		return nil
	}

	// Divide by count to get average
	count := float64(validCount)
	for i := range result {
		result[i] /= count
	}

	return result
}
