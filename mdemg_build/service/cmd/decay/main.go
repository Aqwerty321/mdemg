package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// edge represents a relationship in the graph for decay processing
type edge struct {
	ID            int64
	RelType       string
	SourceID      string
	TargetID      string
	Weight        float64
	EvidenceCount int
	Pinned        bool
	LastActivated time.Time
}

// decayResult holds the result of decay calculation for a single edge
type decayResult struct {
	Edge         edge
	OldWeight    float64
	NewWeight    float64
	DecayPercent float64
	ShouldPrune  bool
	Protected    bool
	ProtectReason string // "high_evidence", "pinned", or ""
}

// calculateDecay applies exponential decay formula to an edge weight.
// Formula: w_new = w_old * exp(-decay_rate * days_since_activation)
// From docs/04_Activation_and_Learning.md Section 5
func calculateDecay(e edge, decayRate float64, now time.Time) (newWeight float64, decayPercent float64) {
	// Calculate days since last activation
	daysSince := daysSinceActivation(e.LastActivated, now)

	// Apply exponential decay: w_new = w_old * exp(-decay_rate * days)
	decayFactor := math.Exp(-decayRate * daysSince)
	newWeight = e.Weight * decayFactor

	// Calculate percentage decay for reporting
	if e.Weight > 0 {
		decayPercent = (1 - newWeight/e.Weight) * 100
	} else {
		decayPercent = 0
	}

	return newWeight, decayPercent
}

// daysSinceActivation calculates the number of days between lastActivated and now.
// Returns 0 if lastActivated is zero or in the future.
func daysSinceActivation(lastActivated time.Time, now time.Time) float64 {
	if lastActivated.IsZero() {
		return 0
	}
	duration := now.Sub(lastActivated)
	if duration < 0 {
		return 0
	}
	return duration.Hours() / 24.0
}

// shouldPrune determines if an edge should be pruned based on protection rules.
// From docs/07_Consolidation_and_Pruning.md Section 5.1:
// Prune edges if ALL are true:
// - weight < prune_threshold
// - evidence_count < min_evidence
// - edge not pinned
// Special case: weight <= 0 always marks for pruning (regardless of other factors)
func shouldPrune(weight float64, evidenceCount int, pinned bool, pruneThreshold float64, minEvidence int) (prune bool, protected bool, reason string) {
	// Special case: zero or negative weight always pruned
	if weight <= 0 {
		return true, false, ""
	}

	// Check if weight is below threshold
	if weight >= pruneThreshold {
		// Weight is high enough, no need to prune
		return false, false, ""
	}

	// Weight is below threshold - check protection rules

	// Protection: pinned edges are never pruned
	if pinned {
		return false, true, "pinned"
	}

	// Protection: high evidence count protects the edge
	if evidenceCount >= minEvidence {
		return false, true, "high_evidence"
	}

	// All prune conditions met: weight < threshold AND evidence < min AND !pinned
	return true, false, ""
}

// processEdge calculates decay and determines pruning status for a single edge
func processEdge(e edge, cfg decayConfig, now time.Time) decayResult {
	// Calculate decayed weight
	newWeight, decayPercent := calculateDecay(e, cfg.DecayRate, now)

	// Determine if edge should be pruned
	prune, protected, reason := shouldPrune(newWeight, e.EvidenceCount, e.Pinned, cfg.PruneThreshold, cfg.MinEvidence)

	return decayResult{
		Edge:          e,
		OldWeight:     e.Weight,
		NewWeight:     newWeight,
		DecayPercent:  decayPercent,
		ShouldPrune:   prune,
		Protected:     protected,
		ProtectReason: reason,
	}
}

// decayConfig holds CLI and environment configuration for the decay job
type decayConfig struct {
	// Neo4j connection
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string

	// Decay parameters
	DecayRate      float64
	PruneThreshold float64
	MinEvidence    int
	OlderThanDays  int

	// Processing options
	DryRun    bool
	SpaceID   string
	BatchSize int
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
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

	// Run the decay job
	if err := runDecayJob(ctx, driver, cfg); err != nil {
		log.Fatalf("decay job failed: %v", err)
	}
}

// parseConfig parses CLI flags and environment variables
func parseConfig() (decayConfig, error) {
	var cfg decayConfig

	// CLI flags with defaults
	flag.Float64Var(&cfg.DecayRate, "decay-rate", 0.1, "Exponential decay rate constant")
	flag.Float64Var(&cfg.PruneThreshold, "prune-threshold", 0.01, "Minimum weight to keep (below = prune candidate)")
	flag.IntVar(&cfg.MinEvidence, "min-evidence", 3, "Minimum evidence_count to protect from pruning")
	flag.IntVar(&cfg.OlderThanDays, "older-than", 7, "Only process edges older than N days")
	flag.BoolVar(&cfg.DryRun, "dry-run", true, "Preview mode - no modifications (default: true)")
	flag.StringVar(&cfg.SpaceID, "space-id", "", "Limit to specific space (empty = all)")
	flag.IntVar(&cfg.BatchSize, "batch-size", 1000, "Process edges in batches of this size")

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
		return decayConfig{}, errors.New("NEO4J_URI/NEO4J_USER/NEO4J_PASS environment variables are required")
	}

	// Validate flag values
	if cfg.DecayRate < 0 {
		return decayConfig{}, errors.New("decay-rate must be non-negative")
	}
	if cfg.PruneThreshold < 0 {
		return decayConfig{}, errors.New("prune-threshold must be non-negative")
	}
	if cfg.MinEvidence < 0 {
		return decayConfig{}, errors.New("min-evidence must be non-negative")
	}
	if cfg.OlderThanDays < 0 {
		return decayConfig{}, errors.New("older-than must be non-negative")
	}
	if cfg.BatchSize <= 0 {
		return decayConfig{}, errors.New("batch-size must be positive")
	}

	return cfg, nil
}

// newDriver creates a new Neo4j driver with the given configuration
func newDriver(cfg decayConfig) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		return nil, err
	}
	return driver, nil
}

// runDecayJob executes the decay and pruning operations
func runDecayJob(ctx context.Context, driver neo4j.DriverWithContext, cfg decayConfig) error {
	// Print header
	printHeader(cfg)

	// TODO: Implement edge querying, decay calculation, and pruning (subtasks 1-2 through 1-5)
	fmt.Println("\nProcessing...")
	fmt.Println("\nStatistics:")
	fmt.Println("- Edges scanned: 0")
	fmt.Println("- Edges decayed: 0")
	fmt.Println("- Edges to prune: 0")
	fmt.Println("- Edges protected (high evidence): 0")
	fmt.Println("- Edges protected (pinned): 0")

	if cfg.DryRun {
		fmt.Println("\nRun with --dry-run=false to apply changes.")
	}

	return nil
}

// printHeader outputs the job configuration header
func printHeader(cfg decayConfig) {
	fmt.Println("MDEMG Decay Job")
	fmt.Println("===============")
	fmt.Println()

	if cfg.DryRun {
		fmt.Println("Mode: DRY RUN (no changes will be made)")
	} else {
		fmt.Println("Mode: LIVE (changes will be applied)")
	}

	if cfg.SpaceID == "" {
		fmt.Println("Space: all")
	} else {
		fmt.Printf("Space: %s\n", cfg.SpaceID)
	}

	fmt.Printf("Edges older than: %d days\n", cfg.OlderThanDays)
	fmt.Printf("Decay rate: %g\n", cfg.DecayRate)
	fmt.Printf("Prune threshold: %g\n", cfg.PruneThreshold)
	fmt.Printf("Min evidence to keep: %d\n", cfg.MinEvidence)
}

// queryEdgeBatch fetches a batch of edges from Neo4j for decay processing.
// It filters by space_id (optional) and age threshold (older than N days).
func queryEdgeBatch(ctx context.Context, driver neo4j.DriverWithContext, cfg decayConfig, offset int) ([]edge, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build the Cypher query with optional space filtering
	// Note: We use updated_at as fallback if last_activated_at is not present
	cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE (r.last_activated_at IS NOT NULL AND r.last_activated_at < datetime() - duration({days: $olderThan}))
   OR (r.last_activated_at IS NULL AND r.updated_at IS NOT NULL AND r.updated_at < datetime() - duration({days: $olderThan}))
WITH a, b, r
WHERE $spaceId = '' OR r.space_id = $spaceId
RETURN id(r) AS edgeId,
       type(r) AS relType,
       a.node_id AS sourceId,
       b.node_id AS targetId,
       coalesce(r.weight, 0.0) AS weight,
       coalesce(r.evidence_count, 0) AS evidence,
       coalesce(r.pinned, false) AS pinned,
       coalesce(r.last_activated_at, r.updated_at, r.created_at) AS lastActivated
ORDER BY lastActivated ASC
SKIP $offset
LIMIT $batchSize`

	params := map[string]any{
		"olderThan": cfg.OlderThanDays,
		"spaceId":   cfg.SpaceID,
		"offset":    offset,
		"batchSize": cfg.BatchSize,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		edges := make([]edge, 0)
		for res.Next(ctx) {
			rec := res.Record()

			edgeID, _ := rec.Get("edgeId")
			relType, _ := rec.Get("relType")
			sourceID, _ := rec.Get("sourceId")
			targetID, _ := rec.Get("targetId")
			weight, _ := rec.Get("weight")
			evidence, _ := rec.Get("evidence")
			pinned, _ := rec.Get("pinned")
			lastActivated, _ := rec.Get("lastActivated")

			e := edge{
				ID:            edgeID.(int64),
				RelType:       relType.(string),
				SourceID:      asString(sourceID),
				TargetID:      asString(targetID),
				Weight:        asFloat64(weight),
				EvidenceCount: asInt(evidence),
				Pinned:        asBool(pinned),
				LastActivated: asTime(lastActivated),
			}
			edges = append(edges, e)
		}

		if err := res.Err(); err != nil {
			return nil, err
		}
		return edges, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]edge), nil
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

// asTime safely converts interface{} to time.Time
func asTime(v any) time.Time {
	if v == nil {
		return time.Time{}
	}
	// Neo4j returns datetime as neo4j.Time or time.Time depending on driver version
	if t, ok := v.(time.Time); ok {
		return t
	}
	// Handle neo4j.LocalDateTime and similar types that have Time() method
	if dt, ok := v.(interface{ Time() time.Time }); ok {
		return dt.Time()
	}
	return time.Time{}
}

// atoi is a helper for parsing environment variables as integers
// (kept for potential future use in environment-based configuration)
func atoi(k string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be int: %w", k, err)
	}
	return n, nil
}
