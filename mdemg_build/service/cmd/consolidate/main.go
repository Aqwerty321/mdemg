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

// runConsolidationJob executes the cluster detection and abstraction promotion
func runConsolidationJob(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) error {
	// Print header
	printHeader(cfg)

	fmt.Println("\nProcessing...")

	// TODO: Implement cluster detection and abstraction promotion in subsequent subtasks
	// This subtask focuses on CLI setup only

	stats := consolidateStats{}

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
