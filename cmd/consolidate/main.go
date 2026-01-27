package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"mdemg/internal/config"
	"mdemg/internal/hidden"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// consolidateConfig holds CLI and environment configuration for the consolidation job
type consolidateConfig struct {
	// Neo4j connection
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string

	// Legacy consolidation parameters (CO_ACTIVATED_WITH based)
	MinClusterSize  int     // default: 3
	WeightThreshold float64 // default: 0.5
	MaxPromotions   int     // default: 50

	// Hidden layer parameters
	HiddenLayerEnabled    bool    // Run hidden layer operations
	MultiLayer            bool    // Run full multi-layer consolidation (L0-L5)
	HiddenClusterEps      float64 // DBSCAN epsilon
	HiddenMinSamples      int     // DBSCAN min samples
	HiddenMaxNodes        int     // Max hidden nodes to create
	HiddenForwardAlpha    float64 // Forward pass alpha
	HiddenForwardBeta     float64 // Forward pass beta
	HiddenBackwardSelf    float64 // Backward pass self weight
	HiddenBackwardBase    float64 // Backward pass base weight
	HiddenBackwardConcept float64 // Backward pass concept weight

	// Operation modes
	LegacyMode   bool // Run legacy CO_ACTIVATED_WITH consolidation
	ForwardOnly  bool // Only run forward pass (hidden layer)
	BackwardOnly bool // Only run backward pass (hidden layer)
	ClusterOnly  bool // Only run clustering (hidden layer)

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

	// Determine which operations to run
	if cfg.LegacyMode {
		// Run legacy CO_ACTIVATED_WITH based consolidation
		if err := runLegacyConsolidationJob(ctx, driver, cfg); err != nil {
			log.Fatalf("legacy consolidation job failed: %v", err)
		}
	}

	if cfg.HiddenLayerEnabled {
		// Run hidden layer operations
		if err := runHiddenLayerJob(ctx, driver, cfg); err != nil {
			log.Fatalf("hidden layer job failed: %v", err)
		}
	}

	if !cfg.LegacyMode && !cfg.HiddenLayerEnabled {
		fmt.Println("No operations selected. Use --hidden-layer and/or --legacy flags.")
		fmt.Println()
		flag.Usage()
		os.Exit(1)
	}
}

// parseConfig parses CLI flags and environment variables
func parseConfig() (consolidateConfig, error) {
	var cfg consolidateConfig

	// Legacy consolidation flags
	flag.IntVar(&cfg.MinClusterSize, "min-cluster-size", 3, "Minimum number of nodes to form a cluster (legacy)")
	flag.Float64Var(&cfg.WeightThreshold, "weight-threshold", 0.5, "Minimum CO_ACTIVATED_WITH weight (legacy)")
	flag.IntVar(&cfg.MaxPromotions, "max-promotions", 50, "Maximum abstraction nodes to create (legacy)")

	// Hidden layer flags
	flag.BoolVar(&cfg.HiddenLayerEnabled, "hidden-layer", false, "Enable hidden layer operations")
	flag.BoolVar(&cfg.MultiLayer, "multi-layer", false, "Run full multi-layer consolidation (L0-L5)")
	flag.Float64Var(&cfg.HiddenClusterEps, "hidden-eps", 0.3, "DBSCAN epsilon (max distance)")
	flag.IntVar(&cfg.HiddenMinSamples, "hidden-min-samples", 3, "DBSCAN minimum samples per cluster")
	flag.IntVar(&cfg.HiddenMaxNodes, "hidden-max-nodes", 100, "Maximum hidden nodes to create")
	flag.Float64Var(&cfg.HiddenForwardAlpha, "hidden-fwd-alpha", 0.6, "Forward pass: weight of current embedding")
	flag.Float64Var(&cfg.HiddenForwardBeta, "hidden-fwd-beta", 0.4, "Forward pass: weight of aggregated embedding")
	flag.Float64Var(&cfg.HiddenBackwardSelf, "hidden-bwd-self", 0.2, "Backward pass: self weight")
	flag.Float64Var(&cfg.HiddenBackwardBase, "hidden-bwd-base", 0.5, "Backward pass: base signal weight")
	flag.Float64Var(&cfg.HiddenBackwardConcept, "hidden-bwd-concept", 0.3, "Backward pass: concept signal weight")

	// Operation mode flags
	flag.BoolVar(&cfg.LegacyMode, "legacy", false, "Run legacy CO_ACTIVATED_WITH consolidation")
	flag.BoolVar(&cfg.ForwardOnly, "forward-only", false, "Only run forward pass (hidden layer)")
	flag.BoolVar(&cfg.BackwardOnly, "backward-only", false, "Only run backward pass (hidden layer)")
	flag.BoolVar(&cfg.ClusterOnly, "cluster-only", false, "Only run clustering (hidden layer)")

	// Common flags
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

	// Validate legacy parameters
	if cfg.LegacyMode {
		if cfg.MinClusterSize < 2 {
			return consolidateConfig{}, errors.New("min-cluster-size must be at least 2")
		}
		if cfg.WeightThreshold < 0 || cfg.WeightThreshold > 1 {
			return consolidateConfig{}, errors.New("weight-threshold must be between 0 and 1")
		}
		if cfg.MaxPromotions <= 0 {
			return consolidateConfig{}, errors.New("max-promotions must be positive")
		}
	}

	// Validate hidden layer parameters
	if cfg.HiddenLayerEnabled {
		if cfg.HiddenClusterEps <= 0 || cfg.HiddenClusterEps > 1 {
			return consolidateConfig{}, errors.New("hidden-eps must be in range (0, 1]")
		}
		if cfg.HiddenMinSamples < 2 {
			return consolidateConfig{}, errors.New("hidden-min-samples must be at least 2")
		}
		if cfg.HiddenMaxNodes < 1 {
			return consolidateConfig{}, errors.New("hidden-max-nodes must be positive")
		}
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

// runHiddenLayerJob executes hidden layer operations using the hidden service
func runHiddenLayerJob(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) error {
	fmt.Println("\n========================================")
	fmt.Println("Hidden Layer Operations")
	fmt.Println("========================================")

	if cfg.DryRun {
		fmt.Println("Mode: DRY RUN (preview only)")
	} else {
		fmt.Println("Mode: LIVE (changes will be applied)")
	}
	fmt.Printf("Space: %s\n", cfg.SpaceID)
	fmt.Printf("Cluster Eps: %.2f\n", cfg.HiddenClusterEps)
	fmt.Printf("Min Samples: %d\n", cfg.HiddenMinSamples)
	fmt.Printf("Max Hidden Nodes: %d\n", cfg.HiddenMaxNodes)

	// Build config for hidden service
	svcCfg := config.Config{
		Neo4jURI:                cfg.Neo4jURI,
		Neo4jUser:               cfg.Neo4jUser,
		Neo4jPass:               cfg.Neo4jPass,
		HiddenLayerEnabled:      !cfg.DryRun, // Disable writes in dry-run
		HiddenLayerClusterEps:   cfg.HiddenClusterEps,
		HiddenLayerMinSamples:   cfg.HiddenMinSamples,
		HiddenLayerMaxHidden:    cfg.HiddenMaxNodes,
		HiddenLayerForwardAlpha: cfg.HiddenForwardAlpha,
		HiddenLayerForwardBeta:  cfg.HiddenForwardBeta,
		HiddenLayerBackwardSelf: cfg.HiddenBackwardSelf,
		HiddenLayerBackwardBase: cfg.HiddenBackwardBase,
		HiddenLayerBackwardConc: cfg.HiddenBackwardConcept,
	}

	svc := hidden.NewService(svcCfg, driver)

	// Determine operations
	runClustering := !cfg.ForwardOnly && !cfg.BackwardOnly
	runForward := !cfg.ClusterOnly && !cfg.BackwardOnly
	runBackward := !cfg.ClusterOnly && !cfg.ForwardOnly

	if cfg.DryRun {
		// Dry run: show what would happen
		fmt.Println("\nDry run - showing potential operations:")
		if runClustering {
			count, err := countOrphanBaseNodes(ctx, driver, cfg.SpaceID)
			if err != nil {
				return fmt.Errorf("count orphan nodes: %w", err)
			}
			fmt.Printf("  - Orphan base nodes available for clustering: %d\n", count)
			if count >= cfg.HiddenMinSamples {
				fmt.Printf("  - Would potentially create up to %d hidden nodes\n", min(count/cfg.HiddenMinSamples, cfg.HiddenMaxNodes))
			}
		}
		if runForward {
			hiddenCount, conceptCount, err := countLayerNodes(ctx, driver, cfg.SpaceID)
			if err != nil {
				return fmt.Errorf("count layer nodes: %w", err)
			}
			fmt.Printf("  - Hidden nodes to update (forward pass): %d\n", hiddenCount)
			fmt.Printf("  - Concept nodes to update (forward pass): %d\n", conceptCount)
		}
		if runBackward {
			hiddenCount, _, err := countLayerNodes(ctx, driver, cfg.SpaceID)
			if err != nil {
				return fmt.Errorf("count layer nodes: %w", err)
			}
			fmt.Printf("  - Hidden nodes to update (backward pass): %d\n", hiddenCount)
		}
		fmt.Println("\nRun with --dry-run=false to apply changes.")
		return nil
	}

	// Live run
	fmt.Println("\nExecuting operations...")

	if runClustering {
		if cfg.MultiLayer {
			fmt.Println("\nStep 1: Running full multi-layer consolidation (L0-L5)...")
			result, err := svc.RunConsolidation(ctx, cfg.SpaceID)
			if err != nil {
				return fmt.Errorf("run multi-layer consolidation: %w", err)
			}
			fmt.Printf("  Hidden nodes created: %d\n", result.HiddenNodesCreated)
			for layer, count := range result.ConceptNodesCreated {
				fmt.Printf("  Layer %d concept nodes created: %d\n", layer, count)
			}
			fmt.Printf("  Total duration: %v\n", result.TotalDuration)
		} else {
			fmt.Println("\nStep 1: Creating hidden nodes from orphan base data (L0-L1)...")
			created, err := svc.CreateHiddenNodes(ctx, cfg.SpaceID)
			if err != nil {
				return fmt.Errorf("create hidden nodes: %w", err)
			}
			fmt.Printf("  Hidden nodes created: %d\n", created)
		}
	}

	if runForward && !cfg.MultiLayer {
		fmt.Println("\nStep 2: Running forward pass...")
		result, err := svc.ForwardPass(ctx, cfg.SpaceID)
		if err != nil {
			return fmt.Errorf("forward pass: %w", err)
		}
		fmt.Printf("  Hidden nodes updated: %d\n", result.HiddenNodesUpdated)
		fmt.Printf("  Concept nodes updated: %d\n", result.ConceptNodesUpdated)
		fmt.Printf("  Duration: %v\n", result.Duration)
	}

	if runBackward && !cfg.MultiLayer {
		fmt.Println("\nStep 3: Running backward pass...")
		result, err := svc.BackwardPass(ctx, cfg.SpaceID)
		if err != nil {
			return fmt.Errorf("backward pass: %w", err)
		}
		fmt.Printf("  Hidden nodes updated: %d\n", result.HiddenNodesUpdated)
		fmt.Printf("  Duration: %v\n", result.Duration)
	}

	fmt.Println("\nHidden layer operations completed successfully.")
	return nil
}

// countOrphanBaseNodes counts base nodes without a GENERALIZES edge to hidden layer
func countOrphanBaseNodes(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})
WHERE NOT (b)-[:GENERALIZES]->(:MemoryNode {layer: 1})
  AND b.embedding IS NOT NULL
RETURN count(b) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			cnt, _ := res.Record().Get("cnt")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// countLayerNodes counts hidden (layer 1) and concept (layer >= 2) nodes
func countLayerNodes(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (int, int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer >= 1
RETURN n.layer AS layer, count(n) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		hiddenCount := 0
		conceptCount := 0
		for res.Next(ctx) {
			rec := res.Record()
			layer, _ := rec.Get("layer")
			cnt, _ := rec.Get("cnt")
			l := asInt(layer)
			c := asInt(cnt)
			if l == 1 {
				hiddenCount = c
			} else if l >= 2 {
				conceptCount += c
			}
		}
		return []int{hiddenCount, conceptCount}, res.Err()
	})
	if err != nil {
		return 0, 0, err
	}
	counts := result.([]int)
	return counts[0], counts[1], nil
}

// ============================================================================
// Legacy consolidation code (CO_ACTIVATED_WITH based)
// ============================================================================

// consolidateStats tracks statistics for the consolidation job
type consolidateStats struct {
	clustersFound   int
	nodesPromoted   int
	edgesCreated    int
	skippedNoEmbed  int
	skippedTooSmall int
	samples         []clusterSample // first few clusters for sample output
}

// clusterSample holds information about a processed cluster for sample output
type clusterSample struct {
	ClusterNum    int
	MemberCount   int
	SourceLayer   int
	TargetLayer   int
	MemberIDs     []string
	AbstractionID string // empty in dry-run
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

// runLegacyConsolidationJob executes the cluster detection and abstraction promotion
func runLegacyConsolidationJob(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) error {
	fmt.Println("\n========================================")
	fmt.Println("Legacy Consolidation (CO_ACTIVATED_WITH)")
	fmt.Println("========================================")

	// Print header
	printHeader(cfg)

	fmt.Println("\nProcessing...")

	stats := consolidateStats{
		samples: make([]clusterSample, 0, 5),
	}

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

			// Collect sample cluster data (first 5)
			if len(stats.samples) < 5 {
				stats.samples = append(stats.samples, clusterSample{
					ClusterNum:    i + 1,
					MemberCount:   len(c.Members),
					SourceLayer:   c.Layer,
					TargetLayer:   c.Layer + 1,
					MemberIDs:     memberIDs,
					AbstractionID: "", // empty in dry-run
				})
			}
		} else {
			// Live mode: create abstraction node and edges
			result, err := createAbstraction(ctx, driver, cfg, c, avgEmbedding)
			if err != nil {
				return fmt.Errorf("create abstraction for cluster %d: %w", i+1, err)
			}
			fmt.Printf("    Created abstraction node: %s (%d edges)\n", result.NodeID, result.MemberCount)
			stats.nodesPromoted++
			stats.edgesCreated += result.MemberCount

			// Collect sample cluster data (first 5)
			if len(stats.samples) < 5 {
				stats.samples = append(stats.samples, clusterSample{
					ClusterNum:    i + 1,
					MemberCount:   len(c.Members),
					SourceLayer:   c.Layer,
					TargetLayer:   c.Layer + 1,
					MemberIDs:     memberIDs,
					AbstractionID: result.NodeID,
				})
			}
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
	fmt.Println("\n----------------------------------------")
	fmt.Println("Legacy Consolidation Statistics")
	fmt.Println("----------------------------------------")

	// Main counts
	fmt.Printf("Clusters found:          %d\n", stats.clustersFound)
	if dryRun {
		fmt.Printf("Nodes to promote:        %d\n", stats.nodesPromoted)
		fmt.Printf("Edges to create:         %d\n", stats.edgesCreated)
	} else {
		fmt.Printf("Nodes promoted:          %d\n", stats.nodesPromoted)
		fmt.Printf("Edges created:           %d\n", stats.edgesCreated)
	}

	// Skip reasons
	fmt.Println("\nSkipped clusters:")
	fmt.Printf("- No valid embeddings:   %d\n", stats.skippedNoEmbed)
	fmt.Printf("- Too small:             %d\n", stats.skippedTooSmall)

	// Print sample clusters (up to 5)
	if len(stats.samples) > 0 {
		fmt.Println("\nSample clusters:")
		for _, s := range stats.samples {
			// Truncate member IDs display if too many
			displayIDs := s.MemberIDs
			suffix := ""
			if len(displayIDs) > 4 {
				displayIDs = displayIDs[:4]
				suffix = fmt.Sprintf("... +%d more", len(s.MemberIDs)-4)
			}

			memberList := strings.Join(displayIDs, ", ")
			if suffix != "" {
				memberList += " " + suffix
			}

			if s.AbstractionID != "" {
				// Live run - show created abstraction ID
				fmt.Printf("  Cluster %d: %d members (layer %d->%d) -> %s\n",
					s.ClusterNum, s.MemberCount, s.SourceLayer, s.TargetLayer, truncateID(s.AbstractionID))
			} else {
				// Dry run - show member IDs
				fmt.Printf("  Cluster %d: %d members (layer %d->%d) [%s]\n",
					s.ClusterNum, s.MemberCount, s.SourceLayer, s.TargetLayer, memberList)
			}
		}
	}

	fmt.Println()
	if dryRun {
		fmt.Println("Run with --dry-run=false to apply changes.")
	} else {
		fmt.Println("Changes applied successfully.")
	}
}

// truncateID shortens a UUID for display (first 8 chars + "...")
func truncateID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "..."
}

// queryClusterCandidates fetches nodes with sufficient high-weight neighbors at the same layer.
func queryClusterCandidates(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) ([]clusterCandidate, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

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
		"minNeighbors": cfg.MinClusterSize - 1,
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

// buildClusters groups cluster candidates into non-overlapping clusters
func buildClusters(candidates []clusterCandidate, minSize int) []cluster {
	candidateMap := make(map[string]clusterCandidate)
	for _, c := range candidates {
		candidateMap[c.NodeID] = c
	}

	assigned := make(map[string]bool)
	var clusters []cluster

	for _, candidate := range candidates {
		if assigned[candidate.NodeID] {
			continue
		}

		var members []clusterMember
		members = append(members, clusterMember{
			NodeID:    candidate.NodeID,
			Embedding: candidate.Embedding,
		})

		for _, neighborID := range candidate.NeighborIDs {
			if assigned[neighborID] {
				continue
			}

			var neighborEmbedding []float64
			if neighborCandidate, exists := candidateMap[neighborID]; exists {
				neighborEmbedding = neighborCandidate.Embedding
			}

			members = append(members, clusterMember{
				NodeID:    neighborID,
				Embedding: neighborEmbedding,
			})
		}

		if len(members) < minSize {
			continue
		}

		for _, member := range members {
			assigned[member.NodeID] = true
		}

		clusters = append(clusters, cluster{
			Members: members,
			Layer:   candidate.Layer,
		})
	}

	return clusters
}

// Helper type conversion functions

func asStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			result = append(result, asString(item))
		}
		return result
	}
	if arr, ok := v.([]string); ok {
		return arr
	}
	return nil
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

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

func asFloat64Slice(v any) []float64 {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		result := make([]float64, 0, len(arr))
		for _, item := range arr {
			result = append(result, asFloat64(item))
		}
		return result
	}
	if arr, ok := v.([]float64); ok {
		return arr
	}
	return nil
}

func asBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// abstractionResult holds the result of creating an abstraction node
type abstractionResult struct {
	NodeID      string
	MemberCount int
}

// createAbstraction creates a new MemoryNode at layer+1 and ABSTRACTS_TO edges
func createAbstraction(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig, c cluster, embedding []float64) (*abstractionResult, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	name := generateAbstractionName(c.Members)
	summary := fmt.Sprintf("Cluster abstraction of %d nodes at layer %d", len(c.Members), c.Layer)
	newLayer := c.Layer + 1

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

		return nil, errors.New("no result returned from abstraction creation query")
	})

	if err != nil {
		return nil, err
	}
	return result.(*abstractionResult), nil
}

// generateAbstractionName creates a descriptive name for the abstraction node
func generateAbstractionName(members []clusterMember) string {
	if len(members) == 0 {
		return "Abstraction: (empty)"
	}

	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.NodeID)
	}

	joined := strings.Join(ids, ", ")

	const maxLen = 60
	if len(joined) > maxLen {
		joined = joined[:maxLen-3] + "..."
	}

	return fmt.Sprintf("Abstraction: [%s]", joined)
}

// averageEmbeddings computes the centroid of multiple embedding vectors
func averageEmbeddings(embeddings [][]float64) []float64 {
	if len(embeddings) == 0 {
		return nil
	}

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
		if len(emb) != dim {
			continue
		}
		for i, v := range emb {
			result[i] += v
		}
		validCount++
	}

	if validCount == 0 {
		return nil
	}

	count := float64(validCount)
	for i := range result {
		result[i] /= count
	}

	return result
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getEnvInt gets an integer from environment variable with default
func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return i
}
