package retrieval

import (
	"context"
	"errors"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/models"
)

// Default values for reflection parameters
const (
	DefaultReflectMaxDepth = 3
	DefaultReflectMaxNodes = 50
	DefaultReflectSeedK    = 20 // number of vector search results for core memories
)

// Reflect performs deep context exploration on a topic:
// 1) SEED: vector recall to find core memories matching the topic
// 2) EXPAND: lateral traversal via CO_ACTIVATED_WITH and ASSOCIATED_WITH edges
// 3) ABSTRACT: upward traversal via ABSTRACTS_TO edges to find abstractions
// 4) INSIGHTS: detect patterns in the traversed subgraph
func (s *Service) Reflect(ctx context.Context, req models.ReflectRequest) (models.ReflectResponse, error) {
	// Validate required fields
	if req.SpaceID == "" {
		return models.ReflectResponse{}, errors.New("space_id is required")
	}
	if req.Topic == "" {
		return models.ReflectResponse{}, errors.New("topic is required")
	}

	// Apply defaults for optional parameters
	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultReflectMaxDepth
	}
	if maxDepth > 5 {
		maxDepth = 5 // cap to prevent runaway traversal
	}

	maxNodes := req.MaxNodes
	if maxNodes <= 0 {
		maxNodes = DefaultReflectMaxNodes
	}
	if maxNodes > 200 {
		maxNodes = 200 // cap to prevent memory issues
	}

	// Validate embedding is provided (embeddings are generated upstream, per project design)
	if len(req.TopicEmbedding) == 0 {
		return models.ReflectResponse{}, errors.New("topic_embedding is required (wire your embedder upstream)")
	}

	// Initialize response with empty slices (not nil)
	resp := models.ReflectResponse{
		Topic:           req.Topic,
		CoreMemories:    []models.ScoredNode{},
		RelatedConcepts: []models.ScoredNode{},
		Abstractions:    []models.ScoredNode{},
		Insights:        []models.Insight{},
		GraphContext: &models.GraphContext{
			NodesExplored:   0,
			EdgesTraversed:  0,
			MaxLayerReached: 0,
		},
	}

	// Stage 1: SEED - Vector search for topic using embedding
	// Use vectorRecall to find core memories matching the topic
	cands, err := s.vectorRecall(ctx, req.SpaceID, req.TopicEmbedding, DefaultReflectSeedK)
	if err != nil {
		return models.ReflectResponse{}, err
	}

	// Initialize visited set with core memory node IDs
	visited := make(map[string]struct{}, len(cands))

	// Convert Candidate to ScoredNode with Distance=0 for core memories
	for _, c := range cands {
		visited[c.NodeID] = struct{}{}
		resp.CoreMemories = append(resp.CoreMemories, models.ScoredNode{
			NodeID:   c.NodeID,
			Name:     c.Name,
			Path:     c.Path,
			Summary:  c.Summary,
			Layer:    0, // Will be updated when we fetch node layer info
			Score:    c.VectorSim,
			Distance: 0, // Core memories are at distance 0
		})
	}

	// Update graph context with nodes explored
	resp.GraphContext.NodesExplored = len(cands)

	// If no core memories found, return early with empty response
	if len(cands) == 0 {
		return resp, nil
	}

	// Stage 2: EXPAND - Lateral traversal via CO_ACTIVATED_WITH and ASSOCIATED_WITH
	// BFS traversal from core memories to find related concepts
	frontier := make([]string, 0, len(cands))
	for _, c := range cands {
		frontier = append(frontier, c.NodeID)
	}

	// Track distance from seed for each node
	nodeDistance := make(map[string]int, len(visited))
	for id := range visited {
		nodeDistance[id] = 0 // core memories are at distance 0
	}

	// Track total edges traversed
	totalEdgesTraversed := 0

	// BFS expansion up to maxDepth
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		// Check if we've reached maxNodes limit
		if len(visited) >= maxNodes {
			break
		}

		// Fetch lateral edges for frontier nodes
		lateralEdges, err := s.fetchLateralEdges(ctx, req.SpaceID, frontier)
		if err != nil {
			return models.ReflectResponse{}, err
		}
		totalEdgesTraversed += len(lateralEdges)

		// Collect new nodes discovered at this depth
		nextFrontier := make([]string, 0)
		for _, e := range lateralEdges {
			// Check both directions - the edge might point to or from a frontier node
			targetID := ""
			if _, ok := visited[e.Src]; ok {
				targetID = e.Dst
			} else if _, ok := visited[e.Dst]; ok {
				targetID = e.Src
			}

			if targetID == "" {
				continue // neither end is in visited set
			}

			// Skip if already visited
			if _, ok := visited[targetID]; ok {
				continue
			}

			// Check maxNodes limit
			if len(visited) >= maxNodes {
				break
			}

			// Mark as visited and record distance
			visited[targetID] = struct{}{}
			nodeDistance[targetID] = depth
			nextFrontier = append(nextFrontier, targetID)
		}

		frontier = nextFrontier
	}

	// Fetch node metadata for all related concepts (nodes found during EXPAND, excluding core memories)
	relatedIDs := make([]string, 0, len(visited)-len(cands))
	for id := range visited {
		isCoreMemory := false
		for _, c := range cands {
			if c.NodeID == id {
				isCoreMemory = true
				break
			}
		}
		if !isCoreMemory {
			relatedIDs = append(relatedIDs, id)
		}
	}

	if len(relatedIDs) > 0 {
		nodeMetadata, err := s.fetchNodeMetadata(ctx, req.SpaceID, relatedIDs)
		if err != nil {
			return models.ReflectResponse{}, err
		}

		// Convert to ScoredNode and add to RelatedConcepts
		for _, meta := range nodeMetadata {
			dist := nodeDistance[meta.NodeID]
			// Score decays with distance (closer = higher score)
			score := 1.0 / float64(1+dist)
			resp.RelatedConcepts = append(resp.RelatedConcepts, models.ScoredNode{
				NodeID:   meta.NodeID,
				Name:     meta.Name,
				Path:     meta.Path,
				Summary:  meta.Summary,
				Layer:    meta.Layer,
				Score:    score,
				Distance: dist,
			})
		}
	}

	// Update graph context
	resp.GraphContext.NodesExplored = len(visited)
	resp.GraphContext.EdgesTraversed = totalEdgesTraversed

	// Stage 3: ABSTRACT - Upward traversal via ABSTRACTS_TO edges
	// Collect all explored node IDs for abstraction traversal
	exploredNodeIDs := make([]string, 0, len(visited))
	for id := range visited {
		exploredNodeIDs = append(exploredNodeIDs, id)
	}

	// Track abstractions separately from visited (abstractions can be re-discovered via different paths)
	abstractionIDs := make(map[string]struct{})
	abstractionDistance := make(map[string]int)
	maxLayerReached := 0

	// BFS upward traversal via ABSTRACTS_TO edges
	abstractionFrontier := exploredNodeIDs
	for depth := 1; depth <= maxDepth && len(abstractionFrontier) > 0; depth++ {
		// Check if we've hit maxNodes limit (counting visited + abstractions)
		if len(visited)+len(abstractionIDs) >= maxNodes {
			break
		}

		// Fetch upward ABSTRACTS_TO edges for frontier nodes
		abstractionEdges, err := s.fetchAbstractionEdges(ctx, req.SpaceID, abstractionFrontier)
		if err != nil {
			return models.ReflectResponse{}, err
		}
		totalEdgesTraversed += len(abstractionEdges)

		// Collect new abstraction nodes discovered at this depth
		nextAbstractionFrontier := make([]string, 0)
		for _, e := range abstractionEdges {
			// ABSTRACTS_TO edges point from concrete (src) to abstract (dst)
			// We're traversing upward, so we want the destination
			targetID := e.Dst

			// Skip if already found as an abstraction
			if _, ok := abstractionIDs[targetID]; ok {
				continue
			}

			// Skip if already in visited set (core memories or related concepts)
			if _, ok := visited[targetID]; ok {
				continue
			}

			// Check maxNodes limit
			if len(visited)+len(abstractionIDs) >= maxNodes {
				break
			}

			// Mark as discovered abstraction and record distance
			abstractionIDs[targetID] = struct{}{}
			abstractionDistance[targetID] = depth
			nextAbstractionFrontier = append(nextAbstractionFrontier, targetID)
		}

		abstractionFrontier = nextAbstractionFrontier
	}

	// Fetch metadata for all abstraction nodes
	if len(abstractionIDs) > 0 {
		absIDs := make([]string, 0, len(abstractionIDs))
		for id := range abstractionIDs {
			absIDs = append(absIDs, id)
		}

		absMetadata, err := s.fetchNodeMetadata(ctx, req.SpaceID, absIDs)
		if err != nil {
			return models.ReflectResponse{}, err
		}

		// Convert to ScoredNode and add to Abstractions
		for _, meta := range absMetadata {
			dist := abstractionDistance[meta.NodeID]
			// Score decays with distance, but abstractions get a boost based on layer
			// Higher layer = more abstract = potentially more valuable for context
			layerBoost := 1.0 + float64(meta.Layer)*0.1
			score := layerBoost / float64(1+dist)
			resp.Abstractions = append(resp.Abstractions, models.ScoredNode{
				NodeID:   meta.NodeID,
				Name:     meta.Name,
				Path:     meta.Path,
				Summary:  meta.Summary,
				Layer:    meta.Layer,
				Score:    score,
				Distance: dist,
			})

			// Track max layer reached
			if meta.Layer > maxLayerReached {
				maxLayerReached = meta.Layer
			}
		}
	}

	// Also check layer of related concepts for max layer
	for _, rc := range resp.RelatedConcepts {
		if rc.Layer > maxLayerReached {
			maxLayerReached = rc.Layer
		}
	}

	// Update final graph context
	resp.GraphContext.NodesExplored = len(visited) + len(abstractionIDs)
	resp.GraphContext.EdgesTraversed = totalEdgesTraversed
	resp.GraphContext.MaxLayerReached = maxLayerReached

	// TODO (subtask-2-5): Stage 4 - INSIGHT GENERATION
	// - Cluster detection: find groups of 3+ nodes with mutual edges
	// - Pattern detection: count edge types, flag if one type > 50%
	// - Gap detection: check for expected edges missing

	return resp, nil
}

// LateralEdge represents an edge used for lateral traversal in reflection
type LateralEdge struct {
	Src     string
	Dst     string
	RelType string
	Weight  float64
}

// NodeMetadata holds basic metadata for a memory node
type NodeMetadata struct {
	NodeID  string
	Name    string
	Path    string
	Summary string
	Layer   int
}

// fetchLateralEdges fetches CO_ACTIVATED_WITH and ASSOCIATED_WITH edges for the given node IDs
func (s *Service) fetchLateralEdges(ctx context.Context, spaceID string, nodeIDs []string) ([]LateralEdge, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
		"maxNbr":  s.cfg.MaxNeighborsPerNode,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Query for lateral edges (CO_ACTIVATED_WITH and ASSOCIATED_WITH)
		// We query both outgoing and incoming edges to ensure bidirectional traversal
		cypher := `UNWIND $nodeIds AS sid
MATCH (src:MemoryNode {space_id:$spaceId, node_id:sid})
CALL {
  WITH src
  MATCH (src)-[r:CO_ACTIVATED_WITH|ASSOCIATED_WITH]-(dst:MemoryNode {space_id:$spaceId})
  WHERE coalesce(r.status,'active')='active'
  RETURN src.node_id AS s, dst.node_id AS d, type(r) AS t,
         coalesce(r.weight,0.0) AS w
  ORDER BY w DESC
  LIMIT $maxNbr
}
RETURN DISTINCT s, d, t, w`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		edges := make([]LateralEdge, 0, 256)
		for res.Next(ctx) {
			rec := res.Record()
			s, _ := rec.Get("s")
			d, _ := rec.Get("d")
			t, _ := rec.Get("t")
			w, _ := rec.Get("w")

			e := LateralEdge{
				Src:     fmt.Sprint(s),
				Dst:     fmt.Sprint(d),
				RelType: fmt.Sprint(t),
				Weight:  toFloat64(w, 0),
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
	return outAny.([]LateralEdge), nil
}

// fetchNodeMetadata fetches basic metadata for a list of node IDs
func (s *Service) fetchNodeMetadata(ctx context.Context, spaceID string, nodeIDs []string) ([]NodeMetadata, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `UNWIND $nodeIds AS nid
MATCH (n:MemoryNode {space_id:$spaceId, node_id:nid})
RETURN n.node_id AS node_id,
       coalesce(n.name,'') AS name,
       coalesce(n.path,'') AS path,
       coalesce(n.summary,'') AS summary,
       coalesce(n.layer,0) AS layer`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		nodes := make([]NodeMetadata, 0, len(nodeIDs))
		for res.Next(ctx) {
			rec := res.Record()
			nid, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			path, _ := rec.Get("path")
			summary, _ := rec.Get("summary")
			layer, _ := rec.Get("layer")

			meta := NodeMetadata{
				NodeID:  fmt.Sprint(nid),
				Name:    fmt.Sprint(name),
				Path:    fmt.Sprint(path),
				Summary: fmt.Sprint(summary),
				Layer:   int(toFloat64(layer, 0)),
			}
			nodes = append(nodes, meta)
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return nodes, nil
	})
	if err != nil {
		return nil, err
	}
	return outAny.([]NodeMetadata), nil
}

// AbstractionEdge represents an ABSTRACTS_TO edge for upward traversal
type AbstractionEdge struct {
	Src     string
	Dst     string
	Weight  float64
}

// fetchAbstractionEdges fetches ABSTRACTS_TO edges for upward traversal from the given node IDs
// ABSTRACTS_TO edges point from concrete nodes (src) to abstract nodes (dst)
func (s *Service) fetchAbstractionEdges(ctx context.Context, spaceID string, nodeIDs []string) ([]AbstractionEdge, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
		"maxNbr":  s.cfg.MaxNeighborsPerNode,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Query for ABSTRACTS_TO edges (upward traversal to higher layers)
		// We follow outgoing ABSTRACTS_TO edges from concrete to abstract
		cypher := `UNWIND $nodeIds AS sid
MATCH (src:MemoryNode {space_id:$spaceId, node_id:sid})
CALL {
  WITH src
  MATCH (src)-[r:ABSTRACTS_TO]->(dst:MemoryNode {space_id:$spaceId})
  WHERE coalesce(r.status,'active')='active'
  RETURN src.node_id AS s, dst.node_id AS d,
         coalesce(r.weight,0.0) AS w
  ORDER BY w DESC
  LIMIT $maxNbr
}
RETURN DISTINCT s, d, w`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		edges := make([]AbstractionEdge, 0, 128)
		for res.Next(ctx) {
			rec := res.Record()
			s, _ := rec.Get("s")
			d, _ := rec.Get("d")
			w, _ := rec.Get("w")

			e := AbstractionEdge{
				Src:    fmt.Sprint(s),
				Dst:    fmt.Sprint(d),
				Weight: toFloat64(w, 0),
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
	return outAny.([]AbstractionEdge), nil
}
