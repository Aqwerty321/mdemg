package hidden

import (
	"context"
	"fmt"
	"strings"
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

// CreateHiddenNodes performs hybrid DBSCAN clustering on orphan base nodes
// Strategy (hybrid - allows emergent patterns):
//  1. Run DBSCAN on ALL nodes first to find natural embedding-based clusters
//  2. Split oversized clusters using path-based grouping (secondary organization)
//  3. Name clusters based on most common path patterns (descriptive, not prescriptive)
//  4. Small clusters stay intact - these are emergent cross-cutting patterns
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

	// Step 2: Filter to nodes with embeddings
	validNodes := make([]BaseNode, 0, len(baseNodes))
	for _, node := range baseNodes {
		if len(node.Embedding) > 0 {
			validNodes = append(validNodes, node)
		}
	}

	if len(validNodes) < s.cfg.HiddenLayerMinSamples {
		return 0, nil
	}

	// Step 3: Extract embeddings and run DBSCAN on ALL nodes (embedding-first clustering)
	embeddings := make([][]float64, len(validNodes))
	for i, n := range validNodes {
		embeddings[i] = n.Embedding
	}
	labels := DBSCAN(embeddings, s.cfg.HiddenLayerClusterEps, s.cfg.HiddenLayerMinSamples)
	clusters, _ := GroupByCluster(validNodes, labels)

	// Step 4: Get existing hidden node count for unique naming
	existingCount, err := s.countHiddenNodes(ctx, spaceID)
	if err != nil {
		return 0, fmt.Errorf("count existing hidden nodes: %w", err)
	}

	// Step 5: Process each natural cluster
	created := 0
	clusterID := 0

	for _, members := range clusters {
		if created >= s.cfg.HiddenLayerMaxHidden {
			break
		}

		// For clusters within size limit, keep them intact (emergent patterns)
		if len(members) <= s.cfg.HiddenLayerMaxClusterSize {
			if len(members) < s.cfg.HiddenLayerMinSamples {
				continue
			}

			centroid := ComputeCentroid(extractEmbeddings(members))
			if centroid == nil {
				continue
			}

			// Name based on most common path pattern (descriptive)
			uniqueID := existingCount + clusterID
			name := fmt.Sprintf("Hidden-%s-%d", inferClusterName(members, s.cfg.HiddenLayerPathGroupDepth), uniqueID)
			err := s.createHiddenNodeWithEdges(ctx, spaceID, name, centroid, members)
			if err != nil {
				return created, fmt.Errorf("create hidden node %s: %w", name, err)
			}
			created++
			clusterID++
			continue
		}

		// For oversized clusters, use path-based splitting as secondary organization
		pathGroups := GroupByPathPrefix(members, s.cfg.HiddenLayerPathGroupDepth)

		for pathPrefix, groupMembers := range pathGroups {
			if created >= s.cfg.HiddenLayerMaxHidden {
				break
			}

			// Split path groups that are still too large
			subClusters := SplitLargeCluster(groupMembers, s.cfg.HiddenLayerMaxClusterSize)

			for _, subMembers := range subClusters {
				if created >= s.cfg.HiddenLayerMaxHidden {
					break
				}

				if len(subMembers) < s.cfg.HiddenLayerMinSamples {
					continue
				}

				centroid := ComputeCentroid(extractEmbeddings(subMembers))
				if centroid == nil {
					continue
				}

				uniqueID := existingCount + clusterID
				name := fmt.Sprintf("Hidden-%s-%d", sanitizePathPrefix(pathPrefix), uniqueID)
				err := s.createHiddenNodeWithEdges(ctx, spaceID, name, centroid, subMembers)
				if err != nil {
					return created, fmt.Errorf("create hidden node %s: %w", name, err)
				}
				created++
				clusterID++
			}
		}
	}

	return created, nil
}

// extractEmbeddings extracts embeddings from a slice of nodes
func extractEmbeddings(nodes []BaseNode) [][]float64 {
	embeddings := make([][]float64, len(nodes))
	for i, n := range nodes {
		embeddings[i] = n.Embedding
	}
	return embeddings
}

// inferClusterName determines a name for a cluster based on most common path patterns
func inferClusterName(members []BaseNode, depth int) string {
	if len(members) == 0 {
		return "misc"
	}

	// Count path prefixes
	prefixCounts := make(map[string]int)
	for _, m := range members {
		prefix := extractPathPrefix(m.Path, depth)
		prefixCounts[prefix]++
	}

	// Find most common prefix
	maxCount := 0
	mostCommon := "misc"
	for prefix, count := range prefixCounts {
		if count > maxCount {
			maxCount = count
			mostCommon = prefix
		}
	}

	// If dominant prefix covers >50% of members, use it; otherwise mark as "mixed"
	if float64(maxCount)/float64(len(members)) > 0.5 {
		return sanitizePathPrefix(mostCommon)
	}
	return "mixed"
}

// sanitizePathPrefix cleans a path prefix for use in node names
func sanitizePathPrefix(prefix string) string {
	if prefix == "" || prefix == "_unknown_" || prefix == "_root_" {
		return "misc"
	}
	// Replace slashes with dashes and limit length
	result := ""
	for _, ch := range prefix {
		if ch == '/' {
			result += "-"
		} else if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			result += string(ch)
		}
	}
	if len(result) > 30 {
		result = result[:30]
	}
	if result == "" {
		return "misc"
	}
	return result
}

// countHiddenNodes returns the current count of hidden nodes for unique naming
func (s *Service) countHiddenNodes(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1, role_type: 'hidden'})
RETURN count(h) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("cnt")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// fetchOrphanBaseNodes retrieves base layer nodes without a GENERALIZES edge to hidden layer
func (s *Service) fetchOrphanBaseNodes(ctx context.Context, spaceID string) ([]BaseNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build query with optional limit from config - now includes path for grouping
	var cypher string
	if s.cfg.HiddenLayerBatchSize > 0 {
		cypher = fmt.Sprintf(`
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})
WHERE NOT (b)-[:GENERALIZES]->(:MemoryNode {layer: 1})
  AND b.embedding IS NOT NULL
RETURN b.node_id AS nodeId, b.path AS path, b.embedding AS embedding
LIMIT %d`, s.cfg.HiddenLayerBatchSize)
	} else {
		// No limit - process all orphan nodes
		cypher = `
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})
WHERE NOT (b)-[:GENERALIZES]->(:MemoryNode {layer: 1})
  AND b.embedding IS NOT NULL
RETURN b.node_id AS nodeId, b.path AS path, b.embedding AS embedding`
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var nodes []BaseNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			path, _ := rec.Get("path")
			embedding, _ := rec.Get("embedding")

			nodes = append(nodes, BaseNode{
				NodeID:    asString(nodeID),
				SpaceID:   spaceID,
				Path:      asString(path),
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

// GenerateSummaries creates summaries for hidden and concept nodes based on their members
func (s *Service) GenerateSummaries(ctx context.Context, spaceID string) (int, error) {
	if !s.cfg.HiddenLayerEnabled {
		return 0, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Update hidden nodes (layer 1) with summaries from base node paths
	// Extract parent directories from paths and create a summary
	cypherHidden := `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WHERE h.summary IS NULL OR h.summary = ''
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[:GENERALIZES]->(h)
WITH h, count(b) AS memberCount,
     collect(DISTINCT CASE
       WHEN b.path IS NOT NULL AND size(split(b.path, '/')) > 2
       THEN split(b.path, '/')[-2]
       ELSE null
     END) AS dirs
WITH h, memberCount, [d IN dirs WHERE d IS NOT NULL][0..5] AS topDirs
SET h.summary = 'Pattern of ' + toString(memberCount) + ' code elements' +
    CASE WHEN size(topDirs) > 0
      THEN ' in: ' + reduce(s = '', d IN topDirs | s + CASE WHEN s = '' THEN '' ELSE ', ' END + d)
      ELSE ''
    END,
    h.updated_at = datetime()
RETURN count(h) AS updated`

	hiddenUpdated, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypherHidden, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("updated")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("update hidden summaries: %w", err)
	}

	// Update concept nodes (layer 2+) with summaries from hidden node names
	cypherConcept := `
MATCH (c:MemoryNode {space_id: $spaceId})
WHERE c.layer >= 2 AND (c.summary IS NULL OR c.summary = '')
MATCH (h:MemoryNode)-[:ABSTRACTS_TO]->(c)
WITH c, collect(DISTINCT coalesce(h.name, h.node_id)) AS hiddenNames,
     collect(DISTINCT h.summary) AS hiddenSummaries
WITH c,
     size(hiddenNames) AS hiddenCount,
     [s IN hiddenSummaries WHERE s IS NOT NULL AND s <> ''][0..3] AS topSummaries
SET c.summary = 'Meta-concept over ' + toString(hiddenCount) + ' clusters' +
    CASE WHEN size(topSummaries) > 0
      THEN '. Sub-concepts: ' + reduce(s = '', t IN topSummaries | s + CASE WHEN s = '' THEN '' ELSE '; ' END + t)
      ELSE ''
    END,
    c.updated_at = datetime()
RETURN count(c) AS updated`

	conceptUpdated, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypherConcept, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("updated")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return hiddenUpdated.(int), fmt.Errorf("update concept summaries: %w", err)
	}

	return hiddenUpdated.(int) + conceptUpdated.(int), nil
}

// CreateConceptNodes performs hybrid DBSCAN clustering on source layer nodes
// Strategy (hybrid - allows emergent patterns):
//  1. Run DBSCAN on ALL source nodes first (embedding-based clustering)
//  2. Split oversized clusters using name-prefix grouping (secondary organization)
//  3. Name clusters based on dominant patterns (descriptive, not prescriptive)
//  4. Small clusters stay intact - emergent cross-cutting patterns
//
// ADAPTIVE CONSTRAINTS (loosen as layers increase):
//  - Epsilon increases with layer (allows more distant concepts to cluster)
//  - MinSamples decreases with layer (smaller emergent groups allowed)
//  - MaxClusterSize stays generous (concepts can be broad)
func (s *Service) CreateConceptNodes(ctx context.Context, spaceID string, targetLayer int) (int, error) {
	if !s.cfg.HiddenLayerEnabled {
		return 0, nil
	}

	sourceLayer := targetLayer - 1
	if sourceLayer < 1 {
		return 0, fmt.Errorf("target layer must be >= 2 (source layer 1 = hidden)")
	}

	// Step 1: Fetch source layer nodes (includes name for secondary grouping)
	sourceNodes, err := s.fetchOrphanLayerNodesWithName(ctx, spaceID, sourceLayer, targetLayer)
	if err != nil {
		return 0, fmt.Errorf("fetch orphan layer %d nodes: %w", sourceLayer, err)
	}

	// Calculate ADAPTIVE parameters - constraints loosen as we go up
	// Higher layers represent more abstract concepts that should cluster more freely
	layerFactor := float64(targetLayer - 1) // 1 for L2, 2 for L3, etc.

	// Epsilon grows with layer: base * (1 + 0.4*layer) → L2: 1.4x, L3: 1.8x, L4: 2.2x, L5: 2.6x
	adaptiveEps := s.cfg.HiddenLayerClusterEps * (1.0 + 0.4*layerFactor)
	if adaptiveEps > 0.6 {
		adaptiveEps = 0.6 // Cap at 0.6 to maintain some semantic coherence
	}

	// MinSamples shrinks with layer: base - layer (min 2) → allows smaller emergent clusters at top
	adaptiveMinSamples := s.cfg.HiddenLayerMinSamples - int(layerFactor)
	if adaptiveMinSamples < 2 {
		adaptiveMinSamples = 2 // Minimum 2 nodes to form a cluster
	}

	if len(sourceNodes) < adaptiveMinSamples {
		return 0, nil // Not enough data to cluster at this layer
	}

	// Step 2: Filter to nodes with embeddings, use message_pass_embedding if available
	validNodes := make([]BaseNode, 0, len(sourceNodes))
	for _, node := range sourceNodes {
		emb := node.Embedding
		if len(node.MessagePassEmbedding) > 0 {
			emb = node.MessagePassEmbedding
		}
		if len(emb) > 0 {
			node.Embedding = emb // Store effective embedding
			validNodes = append(validNodes, node)
		}
	}

	if len(validNodes) < adaptiveMinSamples {
		return 0, nil
	}

	// Step 3: Run DBSCAN with ADAPTIVE parameters
	embeddings := extractEmbeddings(validNodes)
	labels := DBSCAN(embeddings, adaptiveEps, adaptiveMinSamples)
	clusters, _ := GroupByCluster(validNodes, labels)

	// Step 4: Get existing concept node count for unique naming
	existingCount, err := s.countLayerNodes(ctx, spaceID, targetLayer)
	if err != nil {
		return 0, fmt.Errorf("count existing layer %d nodes: %w", targetLayer, err)
	}

	// Max cluster size stays generous - don't artificially limit concept breadth
	// Higher layers can have broader concepts (gradual reduction only)
	maxConceptSize := s.cfg.HiddenLayerMaxClusterSize
	if targetLayer >= 4 {
		// Only slight reduction at very high layers to prevent mega-clusters
		maxConceptSize = maxConceptSize * 3 / 4
	}

	// Step 5: Process each natural cluster
	created := 0
	clusterID := 0

	for _, members := range clusters {
		if created >= s.cfg.HiddenLayerMaxHidden {
			break
		}

		// For clusters within size limit, keep them intact (emergent patterns)
		if len(members) <= maxConceptSize {
			if len(members) < adaptiveMinSamples {
				continue // Use adaptive threshold, not fixed
			}

			centroid := ComputeCentroid(extractEmbeddings(members))
			if centroid == nil {
				continue
			}

			// Name based on most common name prefix pattern
			uniqueID := existingCount + clusterID
			inferredName := inferConceptName(members)
			name := fmt.Sprintf("Concept-L%d-%s-%d", targetLayer, inferredName, uniqueID)
			err := s.createConceptNodeWithEdges(ctx, spaceID, name, centroid, members, targetLayer)
			if err != nil {
				return created, fmt.Errorf("create concept node %s: %w", name, err)
			}
			created++
			clusterID++
			continue
		}

		// For oversized clusters, use name-prefix splitting as secondary organization
		nameGroups := groupByNamePrefix(members)

		for namePrefix, groupMembers := range nameGroups {
			if created >= s.cfg.HiddenLayerMaxHidden {
				break
			}

			// Split groups that are still too large
			subClusters := SplitLargeCluster(groupMembers, maxConceptSize)

			for _, subMembers := range subClusters {
				if created >= s.cfg.HiddenLayerMaxHidden {
					break
				}

				if len(subMembers) < s.cfg.HiddenLayerMinSamples {
					continue
				}

				centroid := ComputeCentroid(extractEmbeddings(subMembers))
				if centroid == nil {
					continue
				}

				uniqueID := existingCount + clusterID
				name := fmt.Sprintf("Concept-L%d-%s-%d", targetLayer, sanitizePathPrefix(namePrefix), uniqueID)
				err := s.createConceptNodeWithEdges(ctx, spaceID, name, centroid, subMembers, targetLayer)
				if err != nil {
					return created, fmt.Errorf("create concept node %s: %w", name, err)
				}
				created++
				clusterID++
			}
		}
	}

	return created, nil
}

// groupByNamePrefix groups nodes by extracting the prefix from their Path field (which contains the name for layer 1+ nodes)
func groupByNamePrefix(nodes []BaseNode) map[string][]BaseNode {
	groups := make(map[string][]BaseNode)

	for _, node := range nodes {
		prefix := extractNamePrefix(node.Path)
		groups[prefix] = append(groups[prefix], node)
	}

	return groups
}

// extractNamePrefix extracts the prefix from a node name by removing the trailing number
// e.g., "Hidden-apps-whk-wms-39" -> "Hidden-apps-whk-wms"
func extractNamePrefix(name string) string {
	if name == "" {
		return "_unknown_"
	}

	// Find the last dash followed by only digits
	lastDash := -1
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '-' {
			suffix := name[i+1:]
			allDigits := len(suffix) > 0
			for _, ch := range suffix {
				if ch < '0' || ch > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				lastDash = i
				break
			}
		}
	}

	if lastDash > 0 {
		return name[:lastDash]
	}
	return name
}

// inferConceptName determines a name for a concept cluster based on member name patterns
func inferConceptName(members []BaseNode) string {
	if len(members) == 0 {
		return "misc"
	}

	// Count name prefixes
	prefixCounts := make(map[string]int)
	for _, m := range members {
		prefix := extractNamePrefix(m.Path)
		// Further simplify: extract the key part after "Hidden-" or "Concept-Ln-"
		simplified := simplifyPrefix(prefix)
		prefixCounts[simplified]++
	}

	// Find most common
	maxCount := 0
	mostCommon := "mixed"
	for prefix, count := range prefixCounts {
		if count > maxCount {
			maxCount = count
			mostCommon = prefix
		}
	}

	// If dominant prefix covers >50%, use it; otherwise "mixed"
	if float64(maxCount)/float64(len(members)) > 0.5 {
		return sanitizePathPrefix(mostCommon)
	}
	return "mixed"
}

// simplifyPrefix extracts the meaningful part from a hidden/concept node name prefix
// e.g., "Hidden-apps-whk-wms" -> "apps-whk-wms"
// e.g., "Concept-L2-frontend" -> "frontend"
func simplifyPrefix(prefix string) string {
	if len(prefix) > 7 && prefix[:7] == "Hidden-" {
		return prefix[7:]
	}
	// Handle "Concept-Ln-" pattern
	if len(prefix) > 10 && prefix[:8] == "Concept-" {
		// Find the second dash after "Concept-Ln"
		for i := 9; i < len(prefix); i++ {
			if prefix[i] == '-' {
				return prefix[i+1:]
			}
		}
	}
	return prefix
}

// fetchOrphanLayerNodes retrieves nodes from sourceLayer without ABSTRACTS_TO edge to targetLayer
func (s *Service) fetchOrphanLayerNodes(ctx context.Context, spaceID string, sourceLayer, targetLayer int) ([]BaseNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: $sourceLayer})
WHERE NOT (n)-[:ABSTRACTS_TO]->(:MemoryNode {layer: $targetLayer})
  AND (n.embedding IS NOT NULL OR n.message_pass_embedding IS NOT NULL)
RETURN n.node_id AS nodeId, n.embedding AS embedding, n.message_pass_embedding AS messagePassEmbedding`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"sourceLayer": sourceLayer,
			"targetLayer": targetLayer,
		})
		if err != nil {
			return nil, err
		}

		var nodes []BaseNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			embedding, _ := rec.Get("embedding")
			msgPassEmb, _ := rec.Get("messagePassEmbedding")

			nodes = append(nodes, BaseNode{
				NodeID:               asString(nodeID),
				SpaceID:              spaceID,
				Embedding:            asFloat64Slice(embedding),
				MessagePassEmbedding: asFloat64Slice(msgPassEmb),
			})
		}
		return nodes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]BaseNode), nil
}

// fetchOrphanLayerNodesWithName retrieves nodes from sourceLayer with name for grouping
func (s *Service) fetchOrphanLayerNodesWithName(ctx context.Context, spaceID string, sourceLayer, targetLayer int) ([]BaseNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: $sourceLayer})
WHERE NOT (n)-[:ABSTRACTS_TO]->(:MemoryNode {layer: $targetLayer})
  AND (n.embedding IS NOT NULL OR n.message_pass_embedding IS NOT NULL)
RETURN n.node_id AS nodeId, n.name AS name, n.embedding AS embedding, n.message_pass_embedding AS messagePassEmbedding`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"sourceLayer": sourceLayer,
			"targetLayer": targetLayer,
		})
		if err != nil {
			return nil, err
		}

		var nodes []BaseNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			embedding, _ := rec.Get("embedding")
			msgPassEmb, _ := rec.Get("messagePassEmbedding")

			nodes = append(nodes, BaseNode{
				NodeID:               asString(nodeID),
				SpaceID:              spaceID,
				Path:                 asString(name), // Store name in Path for grouping
				Embedding:            asFloat64Slice(embedding),
				MessagePassEmbedding: asFloat64Slice(msgPassEmb),
			})
		}
		return nodes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]BaseNode), nil
}

// countLayerNodes returns the count of nodes at a specific layer
func (s *Service) countLayerNodes(ctx context.Context, spaceID string, layer int) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: $layer})
RETURN count(n) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID, "layer": layer})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("cnt")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// createConceptNodeWithEdges creates a concept node and ABSTRACTS_TO edges from members
func (s *Service) createConceptNodeWithEdges(ctx context.Context, spaceID, name string, centroid []float64, members []BaseNode, layer int) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.NodeID
	}

	cypher := `
CREATE (c:MemoryNode {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $name,
  layer: $layer,
  role_type: 'concept',
  embedding: $centroid,
  message_pass_embedding: $centroid,
  aggregation_count: $memberCount,
  stability_score: 1.0,
  last_forward_pass: datetime(),
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c
UNWIND $memberIds AS memberId
MATCH (m:MemoryNode {space_id: $spaceId, node_id: memberId})
CREATE (m)-[:ABSTRACTS_TO {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS conceptId, count(m) AS edgeCount`

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"name":        name,
			"layer":       layer,
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
// Multi-layer hierarchy: base (L0) → hidden (L1) → concepts (L2, L3, ...)
func (s *Service) RunConsolidation(ctx context.Context, spaceID string) (*ConsolidationResult, error) {
	start := time.Now()
	result := &ConsolidationResult{
		ConceptNodesCreated: make(map[int]int),
	}

	// Step 1: Create hidden nodes from orphan base data (L0 → L1)
	hiddenCreated, err := s.CreateHiddenNodes(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("create hidden nodes: %w", err)
	}
	result.HiddenNodesCreated = hiddenCreated

	// Step 1b: Create concern nodes for cross-cutting patterns (P1 improvement)
	_, err = s.CreateConcernNodes(ctx, spaceID)
	if err != nil {
		// Log but don't fail - concern nodes are an enhancement
		fmt.Printf("warning: failed to create concern nodes: %v\n", err)
	}

	// Step 1c: Create config summary node (P2 Track 4.3)
	_, err = s.CreateConfigNodes(ctx, spaceID)
	if err != nil {
		// Log but don't fail - config nodes are an enhancement
		fmt.Printf("warning: failed to create config nodes: %v\n", err)
	}

	// Step 1d: Create comparison nodes for similar modules (P2 Track 3)
	_, err = s.CreateComparisonNodes(ctx, spaceID)
	if err != nil {
		// Log but don't fail - comparison nodes are an enhancement
		fmt.Printf("warning: failed to create comparison nodes: %v\n", err)
	}

	// Step 2: Forward pass (update embeddings up the hierarchy)
	fwdResult, err := s.ForwardPass(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("forward pass: %w", err)
	}
	result.ForwardPass = fwdResult

	// Step 3: Multi-layer concept clustering (L1 → L2, L2 → L3, etc.)
	// Try ALL layers - don't break early. Upper layers have looser constraints
	// and may form clusters even if intermediate layers don't.
	// This allows emergent concepts to form at any level of abstraction.
	maxLayers := 5
	for targetLayer := 2; targetLayer <= maxLayers; targetLayer++ {
		conceptCreated, err := s.CreateConceptNodes(ctx, spaceID, targetLayer)
		if err != nil {
			return nil, fmt.Errorf("create concept nodes layer %d: %w", targetLayer, err)
		}
		// Don't break on zero - upper layers may still form clusters
		// due to adaptive (looser) constraints
		if conceptCreated > 0 {
			result.ConceptNodesCreated[targetLayer] = conceptCreated

			// Run forward pass to update new concept embeddings
			_, err = s.ForwardPass(ctx, spaceID)
			if err != nil {
				return nil, fmt.Errorf("forward pass after layer %d: %w", targetLayer, err)
			}
		}
	}

	// Step 4: Backward pass (propagate signals down)
	bwdResult, err := s.BackwardPass(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("backward pass: %w", err)
	}
	result.BackwardPass = bwdResult

	result.TotalDuration = time.Since(start)
	return result, nil
}

// CreateConcernNodes creates dedicated nodes for cross-cutting concerns
// based on "concern:*" tags in the base data layer.
// This addresses P1 in the development roadmap - improving retrieval for
// ACL, RBAC, authentication, error-handling, and other cross-cutting patterns.
func (s *Service) CreateConcernNodes(ctx context.Context, spaceID string) (*ConcernNodeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ConcernNodeResult{}, nil
	}

	result := &ConcernNodeResult{
		Concerns: make([]string, 0),
	}

	// Step 1: Find all unique concerns from tags
	concerns, err := s.fetchUniqueConcerns(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("fetch unique concerns: %w", err)
	}

	if len(concerns) == 0 {
		return result, nil
	}

	result.Concerns = concerns

	// Step 2: For each concern, create a ConcernNode and edges
	for _, concern := range concerns {
		created, edges, err := s.createConcernNodeWithEdges(ctx, spaceID, concern)
		if err != nil {
			return result, fmt.Errorf("create concern node %s: %w", concern, err)
		}
		if created {
			result.ConcernNodesCreated++
			result.EdgesCreated += edges
		}
	}

	return result, nil
}

// fetchUniqueConcerns finds all unique "concern:*" tags in the space
func (s *Service) fetchUniqueConcerns(ctx context.Context, spaceID string) ([]string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE n.tags IS NOT NULL
UNWIND n.tags AS tag
WITH tag WHERE tag STARTS WITH 'concern:'
RETURN DISTINCT tag AS concern
ORDER BY concern`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var concerns []string
		for res.Next(ctx) {
			rec := res.Record()
			concern, _ := rec.Get("concern")
			if concern != nil {
				concerns = append(concerns, asString(concern))
			}
		}
		return concerns, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// createConcernNodeWithEdges creates a ConcernNode and IMPLEMENTS_CONCERN edges
// Returns (created, edgeCount, error)
func (s *Service) createConcernNodeWithEdges(ctx context.Context, spaceID, concern string) (bool, int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Extract concern type from "concern:authentication" -> "authentication"
	concernType := concern
	if len(concern) > 8 && concern[:8] == "concern:" {
		concernType = concern[8:]
	}

	// Check if concern node already exists
	checkCypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'concern', name: $concernName})
RETURN c.node_id AS nodeId`

	existing, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, checkCypher, map[string]any{
			"spaceId":     spaceID,
			"concernName": concern,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			return true, nil
		}
		return false, res.Err()
	})
	if err != nil {
		return false, 0, err
	}
	if existing.(bool) {
		// Already exists, skip
		return false, 0, nil
	}

	// Create the concern node and edges in a single transaction
	// Works with or without embeddings - computes centroid only if embeddings exist
	createCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE $concern IN n.tags
WITH collect(n) AS members,
     [m IN collect(n) WHERE m.embedding IS NOT NULL | m.embedding] AS embeddings
WHERE size(members) > 0
WITH members, embeddings,
     CASE WHEN size(embeddings) > 0 THEN
       [i IN range(0, size(embeddings[0])-1) |
         reduce(sum = 0.0, emb IN embeddings | sum + emb[i]) / size(embeddings)
       ]
     ELSE null END AS centroid
CREATE (c:MemoryNode {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $concernName,
  concern_type: $concernType,
  layer: 1,
  role_type: 'concern',
  embedding: centroid,
  message_pass_embedding: centroid,
  aggregation_count: size(members),
  stability_score: 1.0,
  summary: 'Cross-cutting concern: ' + $concernType + ' (' + toString(size(members)) + ' implementations)',
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c, members
UNWIND members AS m
CREATE (m)-[:IMPLEMENTS_CONCERN {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  concern_type: $concernType,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS nodeId, size(members) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, createCypher, map[string]any{
			"spaceId":     spaceID,
			"concern":     concern,
			"concernName": concern,
			"concernType": concernType,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), nil
		}
		return 0, res.Err()
	})

	if err != nil {
		return false, 0, err
	}
	return true, result.(int), nil
}

// CreateConfigNodes creates a dedicated summary node for configuration files
// based on "config" tag in the base data layer.
// This addresses P2 Track 4.3 - improving retrieval for configuration-related queries.
func (s *Service) CreateConfigNodes(ctx context.Context, spaceID string) (*ConfigNodeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ConfigNodeResult{}, nil
	}

	result := &ConfigNodeResult{}

	// Check if config node already exists
	exists, err := s.configNodeExists(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("check config node exists: %w", err)
	}
	if exists {
		return result, nil // Already exists
	}

	// Create the config summary node and edges
	created, edges, err := s.createConfigNodeWithEdges(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("create config node: %w", err)
	}

	if created {
		result.ConfigNodeCreated = true
		result.EdgesCreated = edges
	}

	return result, nil
}

// configNodeExists checks if a config summary node already exists
func (s *Service) configNodeExists(ctx context.Context, spaceID string) (bool, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'config'})
RETURN count(c) > 0 AS exists`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			exists, _ := rec.Get("exists")
			return exists.(bool), nil
		}
		return false, res.Err()
	})

	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// createConfigNodeWithEdges creates a ConfigNode and IMPLEMENTS_CONFIG edges
func (s *Service) createConfigNodeWithEdges(ctx context.Context, spaceID string) (bool, int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Create the config node and edges in a single transaction
	// Also extracts config file categories for the summary
	createCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE 'config' IN n.tags
WITH collect(n) AS members,
     [m IN collect(n) WHERE m.embedding IS NOT NULL | m.embedding] AS embeddings,
     collect(DISTINCT
       CASE
         WHEN n.path CONTAINS 'docker' THEN 'docker'
         WHEN n.path CONTAINS '.env' THEN 'environment'
         WHEN n.path CONTAINS 'package.json' OR n.path CONTAINS 'tsconfig' THEN 'package'
         WHEN n.path CONTAINS 'config' THEN 'app-config'
         ELSE 'other'
       END
     ) AS categories
WHERE size(members) > 0
WITH members, embeddings, categories,
     CASE WHEN size(embeddings) > 0 THEN
       [i IN range(0, size(embeddings[0])-1) |
         reduce(sum = 0.0, emb IN embeddings | sum + emb[i]) / size(embeddings)
       ]
     ELSE null END AS centroid
CREATE (c:MemoryNode {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: 'configuration',
  layer: 1,
  role_type: 'config',
  embedding: centroid,
  message_pass_embedding: centroid,
  aggregation_count: size(members),
  stability_score: 1.0,
  summary: 'Configuration summary: ' + toString(size(members)) + ' config files. Categories: ' +
           reduce(s = '', cat IN categories | s + CASE WHEN s = '' THEN '' ELSE ', ' END + cat),
  tags: ['config', 'configuration-summary'],
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c, members
UNWIND members AS m
CREATE (m)-[:IMPLEMENTS_CONFIG {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS nodeId, size(members) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, createCypher, map[string]any{
			"spaceId": spaceID,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), nil
		}
		return 0, res.Err()
	})

	if err != nil {
		return false, 0, err
	}
	if result.(int) == 0 {
		return false, 0, nil // No config files found
	}
	return true, result.(int), nil
}

// ConfigNodeResult tracks config node creation
type ConfigNodeResult struct {
	ConfigNodeCreated bool
	EdgesCreated      int
}

// ComparisonNodeResult tracks comparison node creation for similar modules
type ComparisonNodeResult struct {
	ComparisonNodesCreated int
	EdgesCreated           int
	ModulesCompared        int
}

// CreateComparisonNodes creates comparison nodes for similar modules (P2 Track 3)
// This helps answer questions like "What is the purpose of having both X and Y?"
func (s *Service) CreateComparisonNodes(ctx context.Context, spaceID string) (*ComparisonNodeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ComparisonNodeResult{}, nil
	}

	result := &ComparisonNodeResult{}

	// Step 1: Find module-like base nodes (files with Module, Service, Controller patterns)
	modules, err := s.fetchModuleNodes(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("fetch module nodes: %w", err)
	}

	if len(modules) < 2 {
		return result, nil // Need at least 2 modules to compare
	}

	// Step 2: Group modules by similarity (name pattern and embedding)
	groups := s.groupSimilarModules(modules)

	// Step 3: Create comparison nodes for each group
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		created, edges, err := s.createComparisonNodeWithEdges(ctx, spaceID, group)
		if err != nil {
			return result, fmt.Errorf("create comparison node: %w", err)
		}
		if created {
			result.ComparisonNodesCreated++
			result.EdgesCreated += edges
			result.ModulesCompared += len(group)
		}
	}

	return result, nil
}

// ModuleNode represents a module-like base node for comparison detection
type ModuleNode struct {
	NodeID    string
	Name      string
	Path      string
	Embedding []float64
}

// fetchModuleNodes retrieves base layer nodes that look like modules/services
func (s *Service) fetchModuleNodes(ctx context.Context, spaceID string) ([]ModuleNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Find nodes whose names suggest they are modules, services, or controllers
	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE n.name IS NOT NULL
  AND (n.name CONTAINS 'Module' OR n.name CONTAINS 'Service' OR n.name CONTAINS 'Controller'
       OR n.name CONTAINS 'Provider' OR n.name CONTAINS 'Handler' OR n.name CONTAINS 'Manager')
  AND n.embedding IS NOT NULL
RETURN n.node_id AS nodeId, n.name AS name, n.path AS path, n.embedding AS embedding
ORDER BY n.name`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var modules []ModuleNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			path, _ := rec.Get("path")
			embedding, _ := rec.Get("embedding")

			modules = append(modules, ModuleNode{
				NodeID:    asString(nodeID),
				Name:      asString(name),
				Path:      asString(path),
				Embedding: asFloat64Slice(embedding),
			})
		}
		return modules, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]ModuleNode), nil
}

// groupSimilarModules groups modules by naming patterns
// Looks for pairs/groups of modules with names differing by a prefix (e.g., SyncModule vs DeltaSyncModule)
func (s *Service) groupSimilarModules(modules []ModuleNode) [][]ModuleNode {
	// Build a map of base names -> modules
	baseToModules := make(map[string][]ModuleNode)
	for _, m := range modules {
		base := extractModuleBaseName(m.Name)
		if len(base) >= 4 {
			baseToModules[base] = append(baseToModules[base], m)
		}
	}

	// Find modules where one name is a suffix of another (e.g., "sync" contained in "deltasync")
	groups := make(map[string][]ModuleNode)
	baseNames := make([]string, 0, len(baseToModules))
	for base := range baseToModules {
		baseNames = append(baseNames, base)
	}

	for i, base1 := range baseNames {
		for j := i + 1; j < len(baseNames); j++ {
			base2 := baseNames[j]
			// Check if one is a suffix of the other (with at least 4 char overlap)
			if len(base1) >= 4 && len(base2) >= 4 {
				if strings.HasSuffix(base2, base1) || strings.HasSuffix(base1, base2) {
					// Create a group key from the shorter base
					key := base1
					if len(base2) < len(base1) {
						key = base2
					}
					// Add modules from both bases
					for _, m := range baseToModules[base1] {
						found := false
						for _, existing := range groups[key] {
							if existing.NodeID == m.NodeID {
								found = true
								break
							}
						}
						if !found {
							groups[key] = append(groups[key], m)
						}
					}
					for _, m := range baseToModules[base2] {
						found := false
						for _, existing := range groups[key] {
							if existing.NodeID == m.NodeID {
								found = true
								break
							}
						}
						if !found {
							groups[key] = append(groups[key], m)
						}
					}
				}
			}
		}
	}

	// Convert to slice and filter to groups with 2-6 members (focused comparisons)
	result := make([][]ModuleNode, 0, len(groups))
	for _, group := range groups {
		if len(group) >= 2 && len(group) <= 6 {
			result = append(result, group)
		}
	}

	return result
}

// extractParentDir extracts the parent directory from a file path
func extractParentDir(path string) string {
	if path == "" {
		return ""
	}
	// Find last slash
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash <= 0 {
		return ""
	}
	return path[:lastSlash]
}

// extractModuleBaseName extracts the base name from a module name
// e.g., "DeltaSyncModule" -> "sync", "UserService" -> "user"
func extractModuleBaseName(name string) string {
	// Remove common suffixes
	suffixes := []string{"Module", "Service", "Controller", "Provider", "Handler", "Manager"}
	base := name
	for _, suffix := range suffixes {
		if len(base) > len(suffix) && base[len(base)-len(suffix):] == suffix {
			base = base[:len(base)-len(suffix)]
			break
		}
	}
	// Convert to lowercase for comparison
	return strings.ToLower(base)
}

// sharesSimilarBase checks if two base names share a significant common substring
func sharesSimilarBase(base1, base2 string) bool {
	if base1 == "" || base2 == "" {
		return false
	}
	// Check if one contains the other
	if strings.Contains(base1, base2) || strings.Contains(base2, base1) {
		return true
	}
	// Check for common prefix of at least 3 chars
	minLen := len(base1)
	if len(base2) < minLen {
		minLen = len(base2)
	}
	if minLen >= 3 {
		for i := 3; i <= minLen; i++ {
			if base1[:i] == base2[:i] {
				return true
			}
		}
	}
	return false
}

// commonBase returns the longest common substring between two base names
func commonBase(base1, base2 string) string {
	if strings.Contains(base1, base2) {
		return base2
	}
	if strings.Contains(base2, base1) {
		return base1
	}
	// Find longest common prefix
	minLen := len(base1)
	if len(base2) < minLen {
		minLen = len(base2)
	}
	for i := minLen; i >= 3; i-- {
		if base1[:i] == base2[:i] {
			return base1[:i]
		}
	}
	return ""
}

// comparisonNodeExists checks if a comparison node already exists for a group
func (s *Service) comparisonNodeExists(ctx context.Context, spaceID string, moduleIDs []string) (bool, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Check if any comparison node links to ALL modules in this group
	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'comparison'})
WHERE ALL(moduleId IN $moduleIds WHERE
      (c)<-[:COMPARED_IN]-(:MemoryNode {node_id: moduleId}))
RETURN count(c) > 0 AS exists`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"moduleIds": moduleIDs,
		})
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			exists, _ := rec.Get("exists")
			return exists.(bool), nil
		}
		return false, res.Err()
	})

	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// createComparisonNodeWithEdges creates a comparison node and COMPARED_IN edges
func (s *Service) createComparisonNodeWithEdges(ctx context.Context, spaceID string, modules []ModuleNode) (bool, int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Extract module IDs and names for the query
	moduleIDs := make([]string, len(modules))
	moduleNames := make([]string, len(modules))
	for i, m := range modules {
		moduleIDs[i] = m.NodeID
		moduleNames[i] = m.Name
	}

	// Check if comparison node already exists
	exists, err := s.comparisonNodeExists(ctx, spaceID, moduleIDs)
	if err != nil {
		return false, 0, err
	}
	if exists {
		return false, 0, nil
	}

	// Compute centroid embedding for the comparison node
	var centroid []float64
	count := 0
	for _, m := range modules {
		if len(m.Embedding) > 0 {
			if centroid == nil {
				centroid = make([]float64, len(m.Embedding))
			}
			for i, v := range m.Embedding {
				centroid[i] += v
			}
			count++
		}
	}
	if count > 0 {
		for i := range centroid {
			centroid[i] /= float64(count)
		}
	}

	// Generate comparison name from module names
	compName := "comparison:" + strings.Join(moduleNames[:min(len(moduleNames), 3)], "-vs-")
	if len(moduleNames) > 3 {
		compName += fmt.Sprintf("-and-%d-more", len(moduleNames)-3)
	}

	// Generate rich summary using pattern analysis (Track 3.2)
	summary := generateComparisonSummary(modules)

	// Create comparison node and COMPARED_IN edges
	createCypher := `
CREATE (c:MemoryNode {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $compName,
  layer: 1,
  role_type: 'comparison',
  embedding: $centroid,
  message_pass_embedding: $centroid,
  aggregation_count: $moduleCount,
  stability_score: 1.0,
  summary: $summary,
  tags: ['comparison', 'architecture'],
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c
UNWIND $moduleIds AS moduleId
MATCH (m:MemoryNode {space_id: $spaceId, node_id: moduleId})
CREATE (m)-[:COMPARED_IN {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS nodeId, count(m) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, createCypher, map[string]any{
			"spaceId":     spaceID,
			"compName":    compName,
			"centroid":    centroid,
			"moduleCount": len(modules),
			"moduleIds":   moduleIDs,
			"summary":     summary,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), nil
		}
		return 0, res.Err()
	})

	if err != nil {
		return false, 0, err
	}
	if result.(int) == 0 {
		return false, 0, nil
	}
	return true, result.(int), nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// generateComparisonSummary creates a rich, pattern-based summary for comparison nodes
// Analyzes module names to identify architectural patterns and relationships
func generateComparisonSummary(modules []ModuleNode) string {
	if len(modules) == 0 {
		return "Empty comparison group"
	}

	// Extract names for analysis
	names := make([]string, len(modules))
	for i, m := range modules {
		names[i] = m.Name
	}

	// Find the common base pattern
	baseName := findCommonBase(names)

	// Identify variant prefixes/suffixes
	variants := identifyVariants(names, baseName)

	// Find common path (domain/subsystem)
	domain := extractDomain(modules)

	// Build the summary
	var summary strings.Builder

	// Lead with the relationship type
	summary.WriteString(fmt.Sprintf("Architectural variants of %s", baseName))

	// Add domain context if found
	if domain != "" {
		summary.WriteString(fmt.Sprintf(" in %s", domain))
	}

	summary.WriteString(". ")

	// Describe the variants
	if len(variants) > 0 {
		summary.WriteString("Includes ")
		for i, v := range variants {
			if i > 0 {
				if i == len(variants)-1 {
					summary.WriteString(" and ")
				} else {
					summary.WriteString(", ")
				}
			}
			summary.WriteString(describeVariant(v))
		}
		summary.WriteString(" implementations.")
	}

	// Add pattern insight
	pattern := detectArchitecturalPattern(variants)
	if pattern != "" {
		summary.WriteString(" ")
		summary.WriteString(pattern)
	}

	return summary.String()
}

// findCommonBase extracts the common base name from a list of module names
func findCommonBase(names []string) string {
	if len(names) == 0 {
		return "module"
	}

	// Find the shortest name as likely base
	shortest := names[0]
	for _, n := range names[1:] {
		clean := cleanModuleName(n)
		if len(clean) < len(cleanModuleName(shortest)) {
			shortest = n
		}
	}

	// Clean and use as base
	base := cleanModuleName(shortest)
	if base == "" {
		return "module"
	}
	return base
}

// cleanModuleName removes common suffixes like Module, Service, Controller
func cleanModuleName(name string) string {
	suffixes := []string{"Module", "Service", "Controller", "Provider", "Handler", "Manager"}
	result := name
	for _, suffix := range suffixes {
		if strings.HasSuffix(result, suffix) {
			result = result[:len(result)-len(suffix)]
			break
		}
	}
	return result
}

// identifyVariants extracts the variant prefixes from module names
func identifyVariants(names []string, baseName string) []string {
	variants := make([]string, 0, len(names))
	baseLower := strings.ToLower(baseName)

	for _, name := range names {
		clean := cleanModuleName(name)
		cleanLower := strings.ToLower(clean)

		// Extract the prefix/variant part
		if cleanLower == baseLower {
			variants = append(variants, "base")
		} else if strings.HasSuffix(cleanLower, baseLower) {
			prefix := clean[:len(clean)-len(baseName)]
			if prefix != "" {
				variants = append(variants, prefix)
			}
		} else {
			// Use the whole name as variant
			variants = append(variants, clean)
		}
	}
	return variants
}

// extractDomain extracts the common domain/subsystem from module paths
func extractDomain(modules []ModuleNode) string {
	if len(modules) == 0 {
		return ""
	}

	// Extract path components
	pathCounts := make(map[string]int)
	for _, m := range modules {
		if m.Path == "" {
			continue
		}
		parts := strings.Split(m.Path, "/")
		for _, part := range parts {
			if part != "" && !strings.HasPrefix(part, ".") {
				pathCounts[part]++
			}
		}
	}

	// Find most common meaningful path component
	var domain string
	maxCount := 0
	skipWords := map[string]bool{
		"src": true, "lib": true, "pkg": true, "internal": true,
		"modules": true, "services": true, "components": true,
	}

	for part, count := range pathCounts {
		if count > maxCount && !skipWords[part] && len(part) > 2 {
			domain = part
			maxCount = count
		}
	}
	return domain
}

// describeVariant returns a human-readable description of a variant type
func describeVariant(variant string) string {
	lowerVariant := strings.ToLower(variant)

	descriptions := map[string]string{
		"base":     "base/standard",
		"delta":    "incremental/delta",
		"full":     "full/complete",
		"partial":  "partial/subset",
		"async":    "asynchronous",
		"sync":     "synchronous",
		"batch":    "batch processing",
		"stream":   "streaming",
		"mock":     "mock/test",
		"stub":     "stub/placeholder",
		"default":  "default",
		"custom":   "custom/specialized",
		"legacy":   "legacy/deprecated",
		"v2":       "version 2",
		"new":      "new/updated",
		"extended": "extended",
		"simple":   "simplified",
		"advanced": "advanced",
		"core":     "core/essential",
		"enhanced": "enhanced",
	}

	if desc, ok := descriptions[lowerVariant]; ok {
		return desc
	}
	return variant
}

// detectArchitecturalPattern identifies common architectural patterns from variants
func detectArchitecturalPattern(variants []string) string {
	variantSet := make(map[string]bool)
	for _, v := range variants {
		variantSet[strings.ToLower(v)] = true
	}

	// Check for specific patterns
	if variantSet["delta"] && (variantSet["full"] || variantSet["base"]) {
		return "Pattern: Delta synchronization with full/incremental modes for efficient data transfer."
	}
	if variantSet["async"] && variantSet["sync"] {
		return "Pattern: Dual sync/async implementations for flexibility in blocking/non-blocking contexts."
	}
	if variantSet["batch"] && (variantSet["stream"] || variantSet["single"]) {
		return "Pattern: Batch and individual processing modes for throughput optimization."
	}
	if variantSet["mock"] || variantSet["stub"] {
		return "Pattern: Includes test doubles for unit testing support."
	}
	if variantSet["legacy"] && (variantSet["base"] || variantSet["new"]) {
		return "Pattern: Migration path with legacy support alongside current implementation."
	}
	if variantSet["simple"] && variantSet["advanced"] {
		return "Pattern: Tiered complexity with simple and advanced implementations."
	}

	return ""
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

// =============================================================================
// DYNAMIC EDGE AND NODE TYPE INFERENCE FOR UPPER LAYERS (L4-H4-L5)
// =============================================================================

// InferEdgeType determines the dynamic edge type between two upper-layer nodes
// based on embedding geometry, structural position, and co-activation patterns
func (s *Service) InferEdgeType(source, target UpperLayerNode, coActivation float64) *EdgeInference {
	thresholds := DefaultInferenceThresholds()

	// Calculate metrics
	metrics := EdgeMetrics{
		CosineSimilarity: cosineSimilarity(source.Embedding, target.Embedding),
		CoActivation:     coActivation,
		LayerDistance:    abs(source.Layer - target.Layer),
	}

	// Infer type based on metrics
	var inferredType DynamicEdgeType
	var confidence float64
	var evidence string

	switch {
	// High similarity + same layer = ANALOGOUS_TO
	case metrics.CosineSimilarity >= thresholds.AnalogousMinSim && metrics.LayerDistance == 0:
		inferredType = EdgeAnalogous
		confidence = metrics.CosineSimilarity
		evidence = fmt.Sprintf("High embedding similarity (%.2f) at same layer suggests analogous concepts", metrics.CosineSimilarity)

	// Low similarity + high co-activation = CONTRASTS_WITH (often accessed together but different)
	case metrics.CosineSimilarity <= thresholds.ContrastsMaxSim && metrics.CoActivation >= thresholds.ComposesMinCoact:
		inferredType = EdgeContrasts
		confidence = metrics.CoActivation * (1 - metrics.CosineSimilarity)
		evidence = fmt.Sprintf("Low similarity (%.2f) but high co-activation (%.2f) suggests contrasting approaches", metrics.CosineSimilarity, metrics.CoActivation)

	// High co-activation + moderate similarity = COMPOSES_WITH
	case metrics.CoActivation >= thresholds.ComposesMinCoact && metrics.CosineSimilarity >= 0.4 && metrics.CosineSimilarity < thresholds.AnalogousMinSim:
		inferredType = EdgeComposes
		confidence = metrics.CoActivation
		evidence = fmt.Sprintf("High co-activation (%.2f) with moderate similarity suggests composition", metrics.CoActivation)

	// Layer difference with similarity = SPECIALIZES or GENERALIZES_TO
	case metrics.LayerDistance > 0 && metrics.CosineSimilarity >= 0.5:
		if source.Layer > target.Layer {
			inferredType = EdgeGeneralizes
			evidence = fmt.Sprintf("Higher layer node generalizes lower layer concept (layer %d → %d)", source.Layer, target.Layer)
		} else {
			inferredType = EdgeSpecializes
			evidence = fmt.Sprintf("Lower layer node specializes higher layer concept (layer %d → %d)", source.Layer, target.Layer)
		}
		confidence = metrics.CosineSimilarity

	// Moderate similarity = INFLUENCES (default soft relationship)
	default:
		inferredType = EdgeInfluences
		confidence = 0.5
		evidence = "Default relationship - moderate structural connection"
	}

	return &EdgeInference{
		SourceID:     source.NodeID,
		TargetID:     target.NodeID,
		InferredType: inferredType,
		Confidence:   confidence,
		Evidence:     evidence,
		Metrics:      metrics,
	}
}

// InferNodeType determines the dynamic node type for an upper-layer concept
// based on structural position, connectivity, and embedding stability
func (s *Service) InferNodeType(ctx context.Context, spaceID, nodeID string, layer int) (*NodeInference, error) {
	thresholds := DefaultInferenceThresholds()

	// Fetch node metrics from graph
	metrics, err := s.fetchNodeMetrics(ctx, spaceID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("fetch node metrics: %w", err)
	}

	// Infer type based on metrics
	var inferredType DynamicNodeType
	var confidence float64
	var evidence string

	totalDegree := metrics.InDegree + metrics.OutDegree

	switch {
	// High cross-domain links = BRIDGE
	case metrics.CrossDomainLinks >= thresholds.BridgeMinDomains:
		inferredType = NodeBridge
		confidence = float64(metrics.CrossDomainLinks) / float64(maxInt(totalDegree, 1))
		evidence = fmt.Sprintf("Connects %d distinct domains - acts as a bridge concept", metrics.CrossDomainLinks)

	// High degree = HUB
	case totalDegree >= thresholds.HubMinDegree:
		inferredType = NodeHub
		confidence = minFloat(1.0, float64(totalDegree)/float64(thresholds.HubMinDegree*2))
		evidence = fmt.Sprintf("High connectivity (degree %d) - central hub concept", totalDegree)

	// High stability + high aggregation = PRINCIPLE
	case metrics.StabilityScore >= thresholds.EstablishedMinStab && metrics.AggregationDepth >= 2:
		inferredType = NodePrinciple
		confidence = metrics.StabilityScore
		evidence = fmt.Sprintf("Stable (%.2f) with deep aggregation (%d layers) - guiding principle", metrics.StabilityScore, metrics.AggregationDepth)

	// High stability = ESTABLISHED
	case metrics.StabilityScore >= thresholds.EstablishedMinStab:
		inferredType = NodeEstablished
		confidence = metrics.StabilityScore
		evidence = fmt.Sprintf("High embedding stability (%.2f) - established concept", metrics.StabilityScore)

	// Multiple children with diversity = PATTERN
	case metrics.InDegree >= 3 && metrics.ChildDiversity >= 0.5:
		inferredType = NodePattern
		confidence = metrics.ChildDiversity
		evidence = fmt.Sprintf("Diverse children (%.2f diversity) suggest recurring pattern", metrics.ChildDiversity)

	// Low stability = EMERGENT
	case metrics.StabilityScore < 0.5:
		inferredType = NodeEmergent
		confidence = 1.0 - metrics.StabilityScore
		evidence = fmt.Sprintf("Low stability (%.2f) - newly emergent concept", metrics.StabilityScore)

	// Default = PATTERN
	default:
		inferredType = NodePattern
		confidence = 0.5
		evidence = "Default classification as architectural pattern"
	}

	return &NodeInference{
		NodeID:       nodeID,
		InferredType: inferredType,
		Confidence:   confidence,
		Evidence:     evidence,
		Metrics:      *metrics,
	}, nil
}

// fetchNodeMetrics retrieves structural metrics for a node from the graph
func (s *Service) fetchNodeMetrics(ctx context.Context, spaceID, nodeID string) (*NodeMetrics, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
OPTIONAL MATCH (n)<-[inEdge]-()
OPTIONAL MATCH (n)-[outEdge]->()
OPTIONAL MATCH (n)<-[:ABSTRACTS_TO*1..3]-(child:MemoryNode)
WITH n,
     count(DISTINCT inEdge) AS inDegree,
     count(DISTINCT outEdge) AS outDegree,
     count(DISTINCT child) AS childCount,
     collect(DISTINCT split(child.path, '/')[0]) AS childDomains
RETURN inDegree, outDegree, childCount,
       size(childDomains) AS crossDomainLinks,
       COALESCE(n.stability_score, 0.5) AS stabilityScore,
       n.layer AS layer`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			inDegree, _ := rec.Get("inDegree")
			outDegree, _ := rec.Get("outDegree")
			childCount, _ := rec.Get("childCount")
			crossDomainLinks, _ := rec.Get("crossDomainLinks")
			stabilityScore, _ := rec.Get("stabilityScore")
			layer, _ := rec.Get("layer")

			// Calculate child diversity (unique types / total children)
			childDiversity := 0.0
			if asInt(childCount) > 0 {
				childDiversity = float64(asInt(crossDomainLinks)) / float64(asInt(childCount))
				if childDiversity > 1.0 {
					childDiversity = 1.0
				}
			}

			return &NodeMetrics{
				InDegree:         asInt(inDegree),
				OutDegree:        asInt(outDegree),
				CrossDomainLinks: asInt(crossDomainLinks),
				StabilityScore:   asFloat64(stabilityScore),
				AggregationDepth: asInt(layer),
				ChildDiversity:   childDiversity,
			}, nil
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return &NodeMetrics{StabilityScore: 0.5}, nil
	}
	return result.(*NodeMetrics), nil
}

// ClassifyUpperLayerNodes classifies all nodes at L4+ with dynamic types
func (s *Service) ClassifyUpperLayerNodes(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Fetch all L4+ nodes without classification
	fetchCypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer >= 4 AND (n.node_type IS NULL OR n.node_type = '')
RETURN n.node_id AS nodeId, n.layer AS layer
LIMIT 100`

	nodes, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, fetchCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var nodes []struct {
			NodeID string
			Layer  int
		}
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			nodes = append(nodes, struct {
				NodeID string
				Layer  int
			}{asString(nodeID), asInt(layer)})
		}
		return nodes, res.Err()
	})

	if err != nil {
		return 0, err
	}

	nodeList := nodes.([]struct {
		NodeID string
		Layer  int
	})

	classified := 0
	for _, node := range nodeList {
		inference, err := s.InferNodeType(ctx, spaceID, node.NodeID, node.Layer)
		if err != nil {
			continue // Skip on error, don't fail entire operation
		}

		// Update node with inferred type
		updateCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
SET n.node_type = $nodeType,
    n.type_confidence = $confidence,
    n.type_evidence = $evidence,
    n.type_inferred_at = datetime()
RETURN n.node_id`

		_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, updateCypher, map[string]any{
				"spaceId":    spaceID,
				"nodeId":     node.NodeID,
				"nodeType":   string(inference.InferredType),
				"confidence": inference.Confidence,
				"evidence":   inference.Evidence,
			})
			return nil, err
		})

		if err == nil {
			classified++
		}
	}

	return classified, nil
}

// CreateDynamicEdges creates edges with inferred types for upper layer relationships
func (s *Service) CreateDynamicEdges(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Find pairs of L4+ nodes that should be connected but aren't
	findPairsCypher := `
MATCH (a:MemoryNode {space_id: $spaceId}), (b:MemoryNode {space_id: $spaceId})
WHERE a.layer >= 4 AND b.layer >= 4
  AND a.node_id < b.node_id
  AND NOT (a)-[:DYNAMIC_EDGE]-(b)
  AND a.embedding IS NOT NULL AND b.embedding IS NOT NULL
WITH a, b,
     gds.similarity.cosine(a.embedding, b.embedding) AS sim
WHERE sim > 0.3
RETURN a.node_id AS sourceId, b.node_id AS targetId,
       a.embedding AS sourceEmb, b.embedding AS targetEmb,
       a.layer AS sourceLayer, b.layer AS targetLayer,
       sim
ORDER BY sim DESC
LIMIT 50`

	pairs, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, findPairsCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var pairs []struct {
			Source UpperLayerNode
			Target UpperLayerNode
			Sim    float64
		}
		for res.Next(ctx) {
			rec := res.Record()
			sourceId, _ := rec.Get("sourceId")
			targetId, _ := rec.Get("targetId")
			sourceEmb, _ := rec.Get("sourceEmb")
			targetEmb, _ := rec.Get("targetEmb")
			sourceLayer, _ := rec.Get("sourceLayer")
			targetLayer, _ := rec.Get("targetLayer")
			sim, _ := rec.Get("sim")

			pairs = append(pairs, struct {
				Source UpperLayerNode
				Target UpperLayerNode
				Sim    float64
			}{
				Source: UpperLayerNode{
					NodeID:    asString(sourceId),
					Layer:     asInt(sourceLayer),
					Embedding: asFloat64Slice(sourceEmb),
				},
				Target: UpperLayerNode{
					NodeID:    asString(targetId),
					Layer:     asInt(targetLayer),
					Embedding: asFloat64Slice(targetEmb),
				},
				Sim: asFloat64(sim),
			})
		}
		return pairs, res.Err()
	})

	if err != nil {
		return 0, err
	}

	pairList := pairs.([]struct {
		Source UpperLayerNode
		Target UpperLayerNode
		Sim    float64
	})

	created := 0
	for _, pair := range pairList {
		inference := s.InferEdgeType(pair.Source, pair.Target, pair.Sim)

		createCypher := `
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $sourceId})
MATCH (b:MemoryNode {space_id: $spaceId, node_id: $targetId})
CREATE (a)-[r:DYNAMIC_EDGE {
  space_id: $spaceId,
  edge_id: randomUUID(),
  edge_type: $edgeType,
  weight: $confidence,
  confidence: $confidence,
  evidence: $evidence,
  created_at: datetime(),
  inferred_at: datetime()
}]->(b)
RETURN r.edge_id`

		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, createCypher, map[string]any{
				"spaceId":    spaceID,
				"sourceId":   inference.SourceID,
				"targetId":   inference.TargetID,
				"edgeType":   string(inference.InferredType),
				"confidence": inference.Confidence,
				"evidence":   inference.Evidence,
			})
			return nil, err
		})

		if err == nil {
			created++
		}
	}

	return created, nil
}

// Helper functions for inference

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func asFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}
