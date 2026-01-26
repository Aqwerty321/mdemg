package learning

import (
	"context"
	"sort"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/models"
)

type Service struct {
	cfg    config.Config
	driver neo4j.DriverWithContext
}

func NewService(cfg config.Config, driver neo4j.DriverWithContext) *Service {
	return &Service{cfg: cfg, driver: driver}
}

type pair struct {
	src string
	dst string
	ai  float64
	aj  float64
}

// ApplyCoactivation performs bounded, regularized learning updates.
// DB writes are intentionally limited to small deltas.
func (s *Service) ApplyCoactivation(ctx context.Context, spaceID string, resp models.RetrieveResponse) error {
	if spaceID == "" || len(resp.Results) < 2 {
		return nil
	}

	// Filter nodes by activation threshold (configurable, default 0.20)
	// This prevents clique spam by only considering significantly activated nodes
	minAct := s.cfg.LearningMinActivation
	if minAct <= 0 {
		minAct = 0.20 // fallback default
	}
	nodes := make([]models.RetrieveResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		if r.Activation >= minAct {
			nodes = append(nodes, r)
		}
	}
	if len(nodes) < 2 {
		return nil // need at least 2 nodes to form a pair
	}

	// Build pair updates (cap to config)
	// Generate all O(n²) candidate pairs with their activation products
	pairs := make([]pair, 0)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			pairs = append(pairs, pair{src: nodes[i].NodeID, dst: nodes[j].NodeID, ai: nodes[i].Activation, aj: nodes[j].Activation})
		}
	}

	capN := s.cfg.LearningEdgeCapPerRequest
	if capN <= 0 {
		capN = 200
	}

	// Select top-K pairs by activation product (ai * aj)
	// This prioritizes strengthening edges between the most strongly co-activated nodes
	// and prevents clique spam per 04_Activation_and_Learning.md guidelines
	if len(pairs) > capN {
		sort.Slice(pairs, func(i, j int) bool {
			// Sort descending by activation product
			return pairs[i].ai*pairs[i].aj > pairs[j].ai*pairs[j].aj
		})
		pairs = pairs[:capN]
	}

	// Hebbian learning parameters from config (with fallback defaults)
	// Formula: Δw_ij = η * a_i * a_j - μ * w_ij
	// Simplified: new_w = (1-μ)*w + η*prod, clamped to [wmin, wmax]
	eta := s.cfg.LearningEta
	if eta <= 0 {
		eta = 0.02 // fallback default learning rate
	}
	mu := s.cfg.LearningMu
	if mu <= 0 {
		mu = 0.01 // fallback default decay rate
	}
	wmin := s.cfg.LearningWMin
	wmax := s.cfg.LearningWMax
	if wmax <= wmin {
		wmin, wmax = 0.0, 1.0 // fallback default bounds
	}

	params := map[string]any{
		"spaceId": spaceID,
		"pairs":   pairsToMaps(pairs),
		"eta":     eta,
		"mu":      mu,
		"wmin":    wmin,
		"wmax":    wmax,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Cypher query implements MERGE pattern for CO_ACTIVATED_WITH edges:
		// - MERGE creates edge if missing, matches if exists
		// - ON CREATE SET initializes all required edge properties
		// - ON MATCH SET updates timestamps and evidence count
		// - Weight is calculated using Hebbian formula after MERGE
		// - Symmetric edges are created in both directions
		cypher := `UNWIND $pairs AS p
MATCH (a:MemoryNode {space_id:$spaceId, node_id:p.src})
MATCH (b:MemoryNode {space_id:$spaceId, node_id:p.dst})
WITH a,b, (p.ai * p.aj) AS prod
// forward edge: create or update
MERGE (a)-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(b)
ON CREATE SET r.edge_id=randomUUID(), r.created_at=datetime(), r.updated_at=datetime(),
              r.last_activated_at=datetime(), r.status='active', r.weight=0.10,
              r.evidence_count=1, r.version=1, r.dim_coactivation=1.0, r.decay_rate=0.001
ON MATCH SET r.updated_at=datetime(), r.last_activated_at=datetime(),
             r.evidence_count=coalesce(r.evidence_count,0)+1, r.version=coalesce(r.version,0)+1
WITH a,b,prod,r
WITH a,b,prod,r,
     coalesce(r.weight,0.10) AS w
// Apply Hebbian weight update: new_w = (1-μ)*w + η*prod, clamped to [wmin,wmax]
SET r.weight = CASE
  WHEN ((1-$mu)*w + $eta*prod) < $wmin THEN $wmin
  WHEN ((1-$mu)*w + $eta*prod) > $wmax THEN $wmax
  ELSE ((1-$mu)*w + $eta*prod)
END
// reverse edge (symmetry): mirrors forward edge weight
MERGE (b)-[rr:CO_ACTIVATED_WITH {space_id:$spaceId}]->(a)
ON CREATE SET rr.edge_id=randomUUID(), rr.created_at=datetime(), rr.updated_at=datetime(),
              rr.last_activated_at=datetime(), rr.status='active', rr.weight=r.weight,
              rr.evidence_count=1, rr.version=1, rr.dim_coactivation=1.0, rr.decay_rate=0.001
ON MATCH SET rr.updated_at=datetime(), rr.last_activated_at=datetime(),
             rr.evidence_count=coalesce(rr.evidence_count,0)+1, rr.version=coalesce(rr.version,0)+1, rr.weight=r.weight
RETURN count(*) AS updated;`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})
	return err
}

func pairsToMaps(pairs []pair) []map[string]any {
	out := make([]map[string]any, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, map[string]any{
			"src": p.src,
			"dst": p.dst,
			"ai":  clamp01(p.ai),
			"aj":  clamp01(p.aj),
		})
	}
	return out
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// HebbianWeightUpdate computes the new weight using the Hebbian learning formula.
// The formula is: Δw = η * a_i * a_j - μ * w_ij
// Which simplifies to: new_w = (1-μ)*w + η*a_i*a_j
// The result is clamped to [wmin, wmax].
//
// Parameters:
//   - w: current weight
//   - ai: activation of node i (0 to 1)
//   - aj: activation of node j (0 to 1)
//   - eta: learning rate (η) - controls strengthening from co-activation
//   - mu: decay rate (μ) - controls regularization/forgetting
//   - wmin: minimum allowed weight
//   - wmax: maximum allowed weight
//
// This is a pure function exposed for unit testing.
func HebbianWeightUpdate(w, ai, aj, eta, mu, wmin, wmax float64) float64 {
	// Δw = η * a_i * a_j - μ * w
	// new_w = w + Δw = w + η*a_i*a_j - μ*w = (1-μ)*w + η*a_i*a_j
	prod := ai * aj
	newW := (1-mu)*w + eta*prod

	// Clamp to bounds
	if newW < wmin {
		return wmin
	}
	if newW > wmax {
		return wmax
	}
	return newW
}

// PruneDecayedEdges removes CO_ACTIVATED_WITH edges that have decayed below the threshold.
// This is a maintenance operation that can be run periodically.
// Returns the number of edges deleted.
func (s *Service) PruneDecayedEdges(ctx context.Context, spaceID string) (int64, error) {
	if spaceID == "" {
		return 0, nil
	}

	// Get decay parameters with defaults
	decayPerDay := s.cfg.LearningDecayPerDay
	if decayPerDay <= 0 {
		decayPerDay = 0.05
	}
	pruneThreshold := s.cfg.LearningPruneThreshold
	if pruneThreshold <= 0 {
		pruneThreshold = 0.05
	}

	params := map[string]any{
		"spaceId":        spaceID,
		"decayPerDay":    decayPerDay,
		"pruneThreshold": pruneThreshold,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Find and delete CO_ACTIVATED_WITH edges where decayed weight < threshold
		cypher := `MATCH (a:MemoryNode {space_id:$spaceId})-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(b:MemoryNode {space_id:$spaceId})
WITH r,
     duration.between(coalesce(r.last_activated_at, r.created_at, datetime()), datetime()).days AS daysSinceActive,
     coalesce(r.weight, 0.0) AS rawWeight
WITH r, daysSinceActive, rawWeight,
     CASE WHEN daysSinceActive > 0 THEN
       rawWeight * ((1.0 - $decayPerDay) ^ daysSinceActive)
     ELSE rawWeight END AS decayedWeight
WHERE decayedWeight < $pruneThreshold
DELETE r
RETURN count(*) AS deleted`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			rec := res.Record()
			deleted, _ := rec.Get("deleted")
			if d, ok := deleted.(int64); ok {
				return d, nil
			}
		}
		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// PruneExcessEdgesPerNode removes excess CO_ACTIVATED_WITH edges per node beyond the cap.
// Keeps only the top N edges by weight for each node.
// Returns the number of edges deleted.
func (s *Service) PruneExcessEdgesPerNode(ctx context.Context, spaceID string) (int64, error) {
	if spaceID == "" {
		return 0, nil
	}

	maxEdgesPerNode := s.cfg.LearningMaxEdgesPerNode
	if maxEdgesPerNode <= 0 {
		maxEdgesPerNode = 50
	}

	params := map[string]any{
		"spaceId":         spaceID,
		"maxEdgesPerNode": maxEdgesPerNode,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Find nodes with more than maxEdgesPerNode CO_ACTIVATED_WITH edges
		// and delete the lowest-weight excess edges
		cypher := `MATCH (n:MemoryNode {space_id:$spaceId})-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(m:MemoryNode {space_id:$spaceId})
WITH n, r, m
ORDER BY n.node_id, r.weight DESC
WITH n, collect({rel: r, weight: r.weight}) AS edges
WHERE size(edges) > $maxEdgesPerNode
UNWIND range($maxEdgesPerNode, size(edges)-1) AS idx
WITH (edges[idx]).rel AS toDelete
DELETE toDelete
RETURN count(*) AS deleted`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			rec := res.Record()
			deleted, _ := rec.Get("deleted")
			if d, ok := deleted.(int64); ok {
				return d, nil
			}
		}
		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// GetLearningEdgeStats returns statistics about CO_ACTIVATED_WITH edges for a space.
func (s *Service) GetLearningEdgeStats(ctx context.Context, spaceID string) (map[string]any, error) {
	if spaceID == "" {
		return nil, nil
	}

	decayPerDay := s.cfg.LearningDecayPerDay
	if decayPerDay <= 0 {
		decayPerDay = 0.05
	}
	pruneThreshold := s.cfg.LearningPruneThreshold
	if pruneThreshold <= 0 {
		pruneThreshold = 0.05
	}

	params := map[string]any{
		"spaceId":        spaceID,
		"decayPerDay":    decayPerDay,
		"pruneThreshold": pruneThreshold,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (a:MemoryNode {space_id:$spaceId})-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(b:MemoryNode {space_id:$spaceId})
WITH r,
     duration.between(coalesce(r.last_activated_at, r.created_at, datetime()), datetime()).days AS daysSinceActive,
     coalesce(r.weight, 0.0) AS rawWeight
WITH r, daysSinceActive, rawWeight,
     CASE WHEN daysSinceActive > 0 THEN
       rawWeight * ((1.0 - $decayPerDay) ^ daysSinceActive)
     ELSE rawWeight END AS decayedWeight
RETURN count(r) AS total_edges,
       avg(rawWeight) AS avg_raw_weight,
       avg(decayedWeight) AS avg_decayed_weight,
       sum(CASE WHEN decayedWeight < $pruneThreshold THEN 1 ELSE 0 END) AS edges_below_threshold,
       avg(daysSinceActive) AS avg_days_since_active,
       max(daysSinceActive) AS max_days_since_active`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			stats := make(map[string]any)
			for _, key := range rec.Keys {
				val, _ := rec.Get(key)
				stats[key] = val
			}
			return stats, nil
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result.(map[string]any), nil
}
