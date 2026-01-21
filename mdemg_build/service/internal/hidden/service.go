package hidden

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// Service handles hidden layer operations including clustering and message passing
type Service struct {
	cfg    config.Config
	driver neo4j.DriverWithContext
}

// NewService creates a new hidden layer service
func NewService(cfg config.Config, driver neo4j.DriverWithContext) *Service {
	return &Service{cfg: cfg, driver: driver}
}

// CreateHiddenNodes performs DBSCAN clustering on orphan base nodes and creates hidden nodes
func (s *Service) CreateHiddenNodes(ctx context.Context, spaceID string) (int, error) {
	if !s.cfg.HiddenLayerEnabled {
		return 0, nil
	}

	// Step 1: Fetch base nodes without hidden parent
	baseNodes, err := s.fetchOrphanBaseNodes(ctx, spaceID)
	if err != nil {
		return 0, fmt.Errorf("fetch orphan base nodes: %w", err)
	}

	if len(baseNodes) < s.cfg.HiddenLayerMinSamples {
		return 0, nil // Not enough data to cluster
	}

	// Step 2: Extract embeddings for clustering
	embeddings := make([][]float64, 0, len(baseNodes))
	validNodes := make([]BaseNode, 0, len(baseNodes))
	for _, node := range baseNodes {
		if len(node.Embedding) > 0 {
			embeddings = append(embeddings, node.Embedding)
			validNodes = append(validNodes, node)
		}
	}

	if len(embeddings) < s.cfg.HiddenLayerMinSamples {
		return 0, nil
	}

	// Step 3: Run DBSCAN clustering
	labels := DBSCAN(embeddings, s.cfg.HiddenLayerClusterEps, s.cfg.HiddenLayerMinSamples)
	clusters, _ := GroupByCluster(validNodes, labels)

	// Step 4: Create hidden nodes for each cluster
	created := 0
	for clusterID, members := range clusters {
		if created >= s.cfg.HiddenLayerMaxHidden {
			break
		}

		// Compute centroid embedding
		memberEmbeddings := make([][]float64, len(members))
		for i, m := range members {
			memberEmbeddings[i] = m.Embedding
		}
		centroid := ComputeCentroid(memberEmbeddings)
		if centroid == nil {
			continue
		}

		// Create hidden node and GENERALIZES edges
		name := fmt.Sprintf("Hidden-Pattern-%d", clusterID)
		err := s.createHiddenNodeWithEdges(ctx, spaceID, name, centroid, members)
		if err != nil {
			return created, fmt.Errorf("create hidden node %d: %w", clusterID, err)
		}
		created++
	}

	return created, nil
}

// fetchOrphanBaseNodes retrieves base layer nodes without a GENERALIZES edge to hidden layer
func (s *Service) fetchOrphanBaseNodes(ctx context.Context, spaceID string) ([]BaseNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})
WHERE NOT (b)-[:GENERALIZES]->(:MemoryNode {layer: 1})
  AND b.embedding IS NOT NULL
RETURN b.node_id AS nodeId, b.embedding AS embedding
LIMIT 1000`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var nodes []BaseNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			embedding, _ := rec.Get("embedding")

			nodes = append(nodes, BaseNode{
				NodeID:    asString(nodeID),
				SpaceID:   spaceID,
				Embedding: asFloat64Slice(embedding),
			})
		}
		return nodes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]BaseNode), nil
}

// createHiddenNodeWithEdges creates a hidden node and GENERALIZES edges from members
func (s *Service) createHiddenNodeWithEdges(ctx context.Context, spaceID, name string, centroid []float64, members []BaseNode) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.NodeID
	}

	cypher := `
CREATE (h:MemoryNode {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $name,
  layer: 1,
  role_type: 'hidden',
  embedding: $centroid,
  message_pass_embedding: $centroid,
  aggregation_count: $memberCount,
  stability_score: 1.0,
  last_forward_pass: datetime(),
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH h
UNWIND $memberIds AS memberId
MATCH (b:MemoryNode {space_id: $spaceId, node_id: memberId})
CREATE (b)-[:GENERALIZES {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0 - point.distance(b.embedding, h.embedding) / 2.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(h)
RETURN h.node_id AS hiddenId, count(b) AS edgeCount`

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"name":        name,
			"centroid":    centroid,
			"memberCount": len(members),
			"memberIds":   memberIDs,
		})
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

// ForwardPass updates hidden and concept layer embeddings by aggregating from lower layers
func (s *Service) ForwardPass(ctx context.Context, spaceID string) (*ForwardPassResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ForwardPassResult{}, nil
	}

	start := time.Now()
	result := &ForwardPassResult{}

	// Phase 1: Update hidden layer from base data
	hiddenUpdated, err := s.forwardPassHiddenLayer(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("forward pass hidden layer: %w", err)
	}
	result.HiddenNodesUpdated = hiddenUpdated

	// Phase 2: Update concept layer from hidden (using ABSTRACTS_TO)
	conceptUpdated, err := s.forwardPassConceptLayer(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("forward pass concept layer: %w", err)
	}
	result.ConceptNodesUpdated = conceptUpdated

	result.Duration = time.Since(start)
	return result, nil
}

// forwardPassHiddenLayer aggregates base node embeddings into hidden nodes
func (s *Service) forwardPassHiddenLayer(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// GraphSAGE-style weighted mean aggregation with embedding combination
	cypher := `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[r:GENERALIZES]->(h)
WHERE b.embedding IS NOT NULL
WITH h, collect({emb: b.embedding, weight: coalesce(r.weight, 1.0)}) AS neighbors
WHERE size(neighbors) > 0
WITH h, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH h, neighbors, totalWeight,
     [i IN range(0, size(h.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $alpha * coalesce(h.embedding[i], 0) + $beta * aggregated[i]
    ],
    h.last_forward_pass = datetime(),
    h.aggregation_count = size(neighbors)
RETURN count(h) AS updated`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"alpha":   s.cfg.HiddenLayerForwardAlpha,
			"beta":    s.cfg.HiddenLayerForwardBeta,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			updated, _ := rec.Get("updated")
			return asInt(updated), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// forwardPassConceptLayer aggregates hidden node embeddings into concept nodes
func (s *Service) forwardPassConceptLayer(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Update concept nodes (layer >= 2) from hidden layer via ABSTRACTS_TO
	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId})
WHERE c.layer >= 2
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})-[r:ABSTRACTS_TO]->(c)
WHERE h.message_pass_embedding IS NOT NULL OR h.embedding IS NOT NULL
WITH c, collect({
  emb: coalesce(h.message_pass_embedding, h.embedding),
  weight: coalesce(r.weight, 1.0)
}) AS neighbors
WHERE size(neighbors) > 0
WITH c, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH c, neighbors, totalWeight,
     [i IN range(0, size(c.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET c.message_pass_embedding = [i IN range(0, size(c.embedding)-1) |
      $alpha * coalesce(c.embedding[i], 0) + $beta * aggregated[i]
    ],
    c.last_forward_pass = datetime(),
    c.aggregation_count = size(neighbors)
RETURN count(c) AS updated`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"alpha":   s.cfg.HiddenLayerForwardAlpha,
			"beta":    s.cfg.HiddenLayerForwardBeta,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			updated, _ := rec.Get("updated")
			return asInt(updated), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// BackwardPass propagates feedback from concepts to hidden layers
func (s *Service) BackwardPass(ctx context.Context, spaceID string) (*BackwardPassResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &BackwardPassResult{}, nil
	}

	start := time.Now()
	result := &BackwardPassResult{}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Update hidden nodes with signals from both concepts (above) and base data (below)
	cypher := `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
OPTIONAL MATCH (h)-[rUp:ABSTRACTS_TO]->(c:MemoryNode)
WHERE c.layer >= 2 AND (c.message_pass_embedding IS NOT NULL OR c.embedding IS NOT NULL)
WITH h, collect(coalesce(c.message_pass_embedding, c.embedding)) AS conceptEmbs
OPTIONAL MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[rDown:GENERALIZES]->(h)
WHERE b.embedding IS NOT NULL
WITH h, conceptEmbs, collect(b.embedding) AS baseEmbs
WHERE size(conceptEmbs) > 0 OR size(baseEmbs) > 0
WITH h, conceptEmbs, baseEmbs,
     CASE WHEN size(conceptEmbs) > 0 THEN
       [i IN range(0, size(h.embedding)-1) |
         reduce(sum = 0.0, e IN conceptEmbs | sum + e[i]) / size(conceptEmbs)
       ]
     ELSE null END AS conceptSignal,
     CASE WHEN size(baseEmbs) > 0 THEN
       [i IN range(0, size(h.embedding)-1) |
         reduce(sum = 0.0, e IN baseEmbs | sum + e[i]) / size(baseEmbs)
       ]
     ELSE null END AS baseSignal
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $selfW * coalesce(h.embedding[i], 0) +
      $baseW * coalesce(baseSignal[i], h.embedding[i]) +
      $concW * coalesce(conceptSignal[i], h.embedding[i])
    ],
    h.last_backward_pass = datetime()
RETURN count(h) AS updated`

	updated, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"selfW":   s.cfg.HiddenLayerBackwardSelf,
			"baseW":   s.cfg.HiddenLayerBackwardBase,
			"concW":   s.cfg.HiddenLayerBackwardConc,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			u, _ := rec.Get("updated")
			return asInt(u), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return nil, err
	}

	result.HiddenNodesUpdated = updated.(int)
	result.Duration = time.Since(start)
	return result, nil
}

// RunConsolidation performs a full consolidation: clustering + forward + backward pass
func (s *Service) RunConsolidation(ctx context.Context, spaceID string) (*ConsolidationResult, error) {
	start := time.Now()
	result := &ConsolidationResult{}

	// Step 1: Create hidden nodes from orphan base data
	created, err := s.CreateHiddenNodes(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("create hidden nodes: %w", err)
	}
	result.HiddenNodesCreated = created

	// Step 2: Forward pass
	fwdResult, err := s.ForwardPass(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("forward pass: %w", err)
	}
	result.ForwardPass = fwdResult

	// Step 3: Backward pass
	bwdResult, err := s.BackwardPass(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("backward pass: %w", err)
	}
	result.BackwardPass = bwdResult

	result.TotalDuration = time.Since(start)
	return result, nil
}

// Helper functions for type conversion

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
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
			switch n := item.(type) {
			case float64:
				result = append(result, n)
			case int64:
				result = append(result, float64(n))
			}
		}
		return result
	}
	if arr, ok := v.([]float64); ok {
		return arr
	}
	return nil
}
