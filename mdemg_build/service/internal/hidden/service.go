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

	if len(sourceNodes) < s.cfg.HiddenLayerMinSamples {
		return 0, nil // Not enough data to cluster
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

	if len(validNodes) < s.cfg.HiddenLayerMinSamples {
		return 0, nil
	}

	// Step 3: Run DBSCAN on ALL nodes (embedding-first clustering)
	embeddings := extractEmbeddings(validNodes)
	labels := DBSCAN(embeddings, s.cfg.HiddenLayerClusterEps, s.cfg.HiddenLayerMinSamples)
	clusters, _ := GroupByCluster(validNodes, labels)

	// Step 4: Get existing concept node count for unique naming
	existingCount, err := s.countLayerNodes(ctx, spaceID, targetLayer)
	if err != nil {
		return 0, fmt.Errorf("count existing layer %d nodes: %w", targetLayer, err)
	}

	// Calculate max concept cluster size (smaller for higher layers)
	maxConceptSize := s.cfg.HiddenLayerMaxClusterSize / (targetLayer * 2)
	if maxConceptSize < 10 {
		maxConceptSize = 10
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
			if len(members) < s.cfg.HiddenLayerMinSamples {
				continue
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

	// Step 2: Forward pass (update embeddings up the hierarchy)
	fwdResult, err := s.ForwardPass(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("forward pass: %w", err)
	}
	result.ForwardPass = fwdResult

	// Step 3: Multi-layer concept clustering (L1 → L2, L2 → L3, etc.)
	// Continue creating concept layers until no more clusters can be formed
	maxLayers := 5 // Limit hierarchy depth to prevent infinite loops
	for targetLayer := 2; targetLayer <= maxLayers; targetLayer++ {
		conceptCreated, err := s.CreateConceptNodes(ctx, spaceID, targetLayer)
		if err != nil {
			return nil, fmt.Errorf("create concept nodes layer %d: %w", targetLayer, err)
		}
		if conceptCreated == 0 {
			break // No more clusters to form at this level
		}
		result.ConceptNodesCreated[targetLayer] = conceptCreated

		// Run forward pass again to update new concept embeddings
		_, err = s.ForwardPass(ctx, spaceID)
		if err != nil {
			return nil, fmt.Errorf("forward pass after layer %d: %w", targetLayer, err)
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
