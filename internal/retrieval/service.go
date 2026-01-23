package retrieval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/models"
)

type Service struct {
	cfg               config.Config
	driver            neo4j.DriverWithContext
	reasoningProvider ReasoningProvider
}

func NewService(cfg config.Config, driver neo4j.DriverWithContext) *Service {
	return &Service{
		cfg:               cfg,
		driver:            driver,
		reasoningProvider: &NoOpReasoningProvider{}, // Default: no reasoning modules
	}
}

// SetReasoningProvider sets the reasoning provider for the service.
// This allows reasoning modules to be wired in after service creation.
func (s *Service) SetReasoningProvider(provider ReasoningProvider) {
	if provider != nil {
		s.reasoningProvider = provider
	}
}

// Retrieve performs:
// 1) vector recall (top candidateK)
// 2) bounded neighborhood expansion (<= hopDepth, degree caps)
// 3) spreading activation in memory
// 4) scoring + topK selection
func (s *Service) Retrieve(ctx context.Context, req models.RetrieveRequest) (models.RetrieveResponse, error) {
	if req.SpaceID == "" {
		return models.RetrieveResponse{}, errors.New("space_id is required")
	}
	candK := req.CandidateK
	if candK <= 0 {
		candK = s.cfg.DefaultCandidateK
	}
	topK := req.TopK
	if topK <= 0 {
		topK = s.cfg.DefaultTopK
	}
	hopDepth := req.HopDepth
	if hopDepth <= 0 {
		hopDepth = s.cfg.DefaultHopDepth
	}
	if hopDepth > 3 {
		hopDepth = 3 // keep bounded by default
	}

	if len(req.QueryEmbedding) == 0 {
		// Intentionally not generating embeddings here; plug your embedder in upstream.
		return models.RetrieveResponse{}, errors.New("query_embedding is required (wire your embedder upstream)")
	}

	// 1) Vector recall
	vectorCands, err := s.vectorRecall(ctx, req.SpaceID, req.QueryEmbedding, candK)
	if err != nil {
		return models.RetrieveResponse{}, err
	}

	// 1b) Hybrid retrieval: BM25 search + RRF fusion (if enabled and query text provided)
	var cands []Candidate
	bm25Count := 0
	if s.cfg.HybridRetrievalEnabled && req.QueryText != "" {
		bm25Results, bm25Err := s.BM25Search(ctx, req.SpaceID, req.QueryText, s.cfg.BM25TopK)
		if bm25Err != nil {
			// Log warning but continue with vector-only results
			log.Printf("WARN: BM25 search failed, using vector-only: %v", bm25Err)
			cands = vectorCands
		} else {
			bm25Count = len(bm25Results)
			// Fuse vector and BM25 results using RRF
			fused := ReciprocalRankFusion(vectorCands, bm25Results, s.cfg.VectorWeight, s.cfg.BM25Weight)
			cands = ConvertFusedToCandidates(fused)
		}
	} else {
		cands = vectorCands
	}

	if len(cands) == 0 {
		return models.RetrieveResponse{SpaceID: req.SpaceID, Results: []models.RetrieveResult{}}, nil
	}

	// Seeds: use first N for expansion (keep bounded)
	seedN := minInt(50, len(cands))
	seedIDs := make([]string, 0, seedN)
	for i := 0; i < seedN; i++ {
		seedIDs = append(seedIDs, cands[i].NodeID)
	}

	// 2) Expansion: iterative 1-hop fetch up to hopDepth
	edges := make([]Edge, 0, 1024)
	seenEdge := map[string]struct{}{}
	frontier := append([]string{}, seedIDs...)
	seenNode := map[string]struct{}{}
	for _, id := range frontier {
		seenNode[id] = struct{}{}
	}

	for d := 0; d < hopDepth; d++ {
		if len(frontier) == 0 {
			break
		}
		batchEdges, nextNodes, err := s.fetchOutgoingEdges(ctx, req.SpaceID, frontier)
		if err != nil {
			return models.RetrieveResponse{}, err
		}
		frontier = frontier[:0]
		for _, e := range batchEdges {
			key := e.Src + "|" + e.RelType + "|" + e.Dst
			if _, ok := seenEdge[key]; ok {
				continue
			}
			seenEdge[key] = struct{}{}
			edges = append(edges, e)
		}
		for _, nid := range nextNodes {
			if _, ok := seenNode[nid]; ok {
				continue
			}
			seenNode[nid] = struct{}{}
			frontier = append(frontier, nid)
		}
	}

	// 3) Activation physics
	act := SpreadingActivation(cands, edges, 2, 0.15)

	// 4) Initial ranking (pass query text for path-based boosting)
	// Request more candidates for re-ranking if enabled
	initialTopK := topK
	if s.cfg.RerankEnabled && req.QueryText != "" {
		initialTopK = s.cfg.RerankTopN
	}

	// Use breakdown-enabled scoring if Jiminy is requested
	var scoredCands []ScoredCandidate
	var results []models.RetrieveResult
	if req.JiminyEnabled {
		scoredCands = ScoreAndRankWithBreakdown(cands, act, edges, initialTopK, s.cfg, req.QueryText)
		results = make([]models.RetrieveResult, len(scoredCands))
		for i, sc := range scoredCands {
			results[i] = sc.RetrieveResult
		}
	} else {
		results = ScoreAndRank(cands, act, edges, initialTopK, s.cfg, req.QueryText)
	}

	// 5) Reasoning Module Processing (if available and query text provided)
	var reasoningModuleID string
	var reasoningLatencyMs float64
	var reasoningTokens int
	if s.reasoningProvider != nil && s.reasoningProvider.Available() && req.QueryText != "" && len(results) > 0 {
		reasoningReq := ReasoningRequest{
			QueryText:  req.QueryText,
			Candidates: results,
			TopK:       initialTopK,
			Context: map[string]string{
				"space_id": req.SpaceID,
			},
		}

		reasoningResult, reasoningErr := s.reasoningProvider.Process(ctx, reasoningReq)
		if reasoningErr != nil {
			log.Printf("WARN: reasoning module processing failed, using initial results: %v", reasoningErr)
		} else if len(reasoningResult.Results) > 0 {
			results = reasoningResult.Results
			reasoningModuleID = reasoningResult.ModuleID
			reasoningLatencyMs = reasoningResult.LatencyMs
			reasoningTokens = reasoningResult.TokensUsed
		}
	}

	// 6) LLM Re-ranking (if enabled and query text provided)
	var rerankLatencyMs float64
	var rerankTokens int
	wasReranked := false
	if s.cfg.RerankEnabled && req.QueryText != "" && len(results) > 0 {
		// Store pre-rerank scores for delta calculation
		preRerankScores := make(map[string]float64)
		for _, r := range results {
			preRerankScores[r.NodeID] = r.Score
		}

		rerankResult, rerankErr := s.Rerank(ctx, RerankRequest{
			Query:      req.QueryText,
			Candidates: results,
			TopN:       s.cfg.RerankTopN,
			ReturnK:    topK,
		})
		if rerankErr != nil {
			// Log warning but continue with initial results
			log.Printf("WARN: LLM rerank failed, using initial results: %v", rerankErr)
		} else {
			wasReranked = true
			results = rerankResult.Results
			rerankLatencyMs = rerankResult.LatencyMs
			rerankTokens = rerankResult.TokensUsed

			// Update breakdowns with rerank delta if Jiminy is enabled
			if req.JiminyEnabled && scoredCands != nil {
				// Create a map for quick lookup
				breakdownMap := make(map[string]*ScoreBreakdown)
				for i := range scoredCands {
					breakdownMap[scoredCands[i].NodeID] = &scoredCands[i].Breakdown
				}

				// Calculate rerank delta for each result
				for i := range results {
					nodeID := results[i].NodeID
					if bd, ok := breakdownMap[nodeID]; ok {
						preScore := preRerankScores[nodeID]
						bd.RerankDelta = results[i].Score - preScore
						bd.FinalScore = results[i].Score
					}
				}
			}
		}
	}

	// Truncate to topK if needed
	if len(results) > topK {
		results = results[:topK]
	}

	// Generate Jiminy explanations if enabled
	if req.JiminyEnabled && scoredCands != nil {
		// Create a map for quick lookup
		breakdownMap := make(map[string]ScoreBreakdown)
		for _, sc := range scoredCands {
			breakdownMap[sc.NodeID] = sc.Breakdown
		}

		for i := range results {
			nodeID := results[i].NodeID
			if bd, ok := breakdownMap[nodeID]; ok {
				path := DetermineRetrievalPath(bd, wasReranked)
				jiminy := GenerateJiminyExplanation(bd, path)
				results[i].Jiminy = &models.JiminyExplanation{
					Rationale:           jiminy.Rationale,
					Confidence:          jiminy.Confidence,
					RetrievalPath:       jiminy.RetrievalPath,
					ContributingModules: jiminy.ContributingModules,
					ScoreBreakdown:      jiminy.ScoreBreakdown,
				}
			}
		}
	}

	resp := models.RetrieveResponse{
		SpaceID: req.SpaceID,
		Results: results,
		Debug: map[string]any{
			"candidate_k":            candK,
			"seed_n":                 seedN,
			"edges_fetched":          len(edges),
			"hop_depth":              hopDepth,
			"hybrid_enabled":         s.cfg.HybridRetrievalEnabled,
			"vector_count":           len(vectorCands),
			"bm25_count":             bm25Count,
			"fused_count":            len(cands),
			"reasoning_module":       reasoningModuleID,
			"reasoning_latency_ms":   reasoningLatencyMs,
			"reasoning_tokens":       reasoningTokens,
			"rerank_enabled":         s.cfg.RerankEnabled,
			"rerank_latency_ms":      rerankLatencyMs,
			"rerank_tokens":          rerankTokens,
			"jiminy_enabled":         req.JiminyEnabled,
		},
	}
	return resp, nil
}

type Candidate struct {
	NodeID     string
	Path       string
	Name       string
	Summary    string
	UpdatedAt  time.Time
	Confidence float64
	VectorSim  float64
	Layer      int      // 0=base, 1=hidden/concern, 2+=concept
	Tags       []string // Tags for scoring boosts (e.g., "config")
}

// SimilarNode represents a node returned from vector similarity search
type SimilarNode struct {
	NodeID string
	Score  float64
}

func (s *Service) vectorRecall(ctx context.Context, spaceID string, q []float32, k int) ([]Candidate, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"k": k,
		"q": q,
		"index": s.cfg.VectorIndexName,
	}
	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `WITH $q AS q
CALL db.index.vector.queryNodes($index, $k, q)
YIELD node, score
WHERE node.space_id = $spaceId AND NOT coalesce(node.is_archived, false)
RETURN node.node_id AS node_id,
       node.path AS path,
       node.name AS name,
       coalesce(node.summary,'') AS summary,
       coalesce(node.confidence,0.6) AS confidence,
       coalesce(node.updated_at, datetime()) AS updated_at,
       coalesce(node.layer, 0) AS layer,
       coalesce(node.tags, []) AS tags,
       score AS score
ORDER BY score DESC`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		cands := make([]Candidate, 0, k)
		for res.Next(ctx) {
			rec := res.Record()
			nid, _ := rec.Get("node_id")
			path, _ := rec.Get("path")
			name, _ := rec.Get("name")
			sum, _ := rec.Get("summary")
			conf, _ := rec.Get("confidence")
			upd, _ := rec.Get("updated_at")
			layer, _ := rec.Get("layer")
			tagsAny, _ := rec.Get("tags")
			sc, _ := rec.Get("score")

			ct := Candidate{
				NodeID:     fmt.Sprint(nid),
				Path:       fmt.Sprint(path),
				Name:       fmt.Sprint(name),
				Summary:    fmt.Sprint(sum),
				Confidence: toFloat64(conf, 0.6),
				VectorSim:  toFloat64(sc, 0),
				Layer:      toInt(layer, 0),
				Tags:       toStringSlice(tagsAny),
			}
			// neo4j returns time as neo4j.LocalDateTime or time.Time depending on driver
			switch v := upd.(type) {
			case time.Time:
				ct.UpdatedAt = v
			default:
				ct.UpdatedAt = time.Now()
			}
			cands = append(cands, ct)
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return cands, nil
	})
	if err != nil {
		return nil, err
	}
	return outAny.([]Candidate), nil
}

// FindSimilarNodes queries the vector index for nodes similar to the provided embedding,
// excluding the specified node (self-match). Used for semantic edge creation on ingest.
func (s *Service) FindSimilarNodes(ctx context.Context, spaceID string, embedding []float32, excludeNodeID string, topN int) ([]SimilarNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use a larger k value to account for cross-space filtering.
	// The vector index returns top-k across ALL spaces, then we filter by space_id.
	// Using DefaultCandidateK ensures we have enough candidates after filtering.
	queryK := s.cfg.DefaultCandidateK
	if queryK < topN+1 {
		queryK = topN + 1
	}

	params := map[string]any{
		"spaceId":       spaceID,
		"k":             queryK,
		"q":             embedding,
		"index":         s.cfg.VectorIndexName,
		"excludeNodeId": excludeNodeID,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `WITH $q AS q
CALL db.index.vector.queryNodes($index, $k, q)
YIELD node, score
WHERE node.space_id = $spaceId AND node.node_id <> $excludeNodeId AND NOT coalesce(node.is_archived, false)
RETURN node.node_id AS node_id, score
ORDER BY score DESC
LIMIT $k`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		results := make([]SimilarNode, 0, topN)
		for res.Next(ctx) {
			rec := res.Record()
			nid, _ := rec.Get("node_id")
			sc, _ := rec.Get("score")

			sn := SimilarNode{
				NodeID: fmt.Sprint(nid),
				Score:  toFloat64(sc, 0),
			}
			results = append(results, sn)
			// Stop after topN results
			if len(results) >= topN {
				break
			}
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return results, nil
	})
	if err != nil {
		return nil, err
	}
	return outAny.([]SimilarNode), nil
}

// CreateAssociatedWithEdge creates or updates an ASSOCIATED_WITH edge between two nodes.
// Uses MERGE to avoid duplicates. On create, sets initial weight from config.
// On match, increments weight and evidence_count.
func (s *Service) CreateAssociatedWithEdge(ctx context.Context, spaceID, fromNodeID, toNodeID string, similarityScore float64) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId":         spaceID,
		"fromNodeId":      fromNodeID,
		"toNodeId":        toNodeID,
		"initialWeight":   s.cfg.SemanticEdgeInitialWeight,
		"similarityScore": similarityScore,
	}

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (a:MemoryNode {space_id:$spaceId, node_id:$fromNodeId})
MATCH (b:MemoryNode {space_id:$spaceId, node_id:$toNodeId})
MERGE (a)-[r:ASSOCIATED_WITH]->(b)
ON CREATE SET
    r.edge_id = randomUUID(),
    r.space_id = $spaceId,
    r.weight = $initialWeight,
    r.dim_semantic = $similarityScore,
    r.evidence_count = 1,
    r.status = 'active',
    r.created_at = datetime(),
    r.updated_at = datetime()
ON MATCH SET
    r.weight = CASE WHEN r.weight + ($similarityScore * 0.1) > 1.0 THEN 1.0 ELSE r.weight + ($similarityScore * 0.1) END,
    r.evidence_count = r.evidence_count + 1,
    r.updated_at = datetime()`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		// Consume the result
		_, err = res.Consume(ctx)
		return nil, err
	})
	return err
}

type Edge struct {
	Src string
	Dst string
	RelType string
	Weight float64
	DimSemantic float64
	DimTemporal float64
	DimCoactivation float64
	UpdatedAt time.Time
}

func (s *Service) fetchOutgoingEdges(ctx context.Context, spaceID string, nodeIDs []string) ([]Edge, []string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
		"allowed": s.cfg.AllowedRelationshipTypes,
		"maxNbr": s.cfg.MaxNeighborsPerNode,
		"maxTotal": s.cfg.MaxTotalEdgesFetched,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `UNWIND $nodeIds AS sid
MATCH (src:MemoryNode {space_id:$spaceId, node_id:sid})
CALL {
  WITH src
  MATCH (src)-[r]->(dst:MemoryNode {space_id:$spaceId})
  WHERE type(r) IN $allowed AND coalesce(r.status,'active')='active'
  RETURN src.node_id AS s, dst.node_id AS d, type(r) AS t,
         coalesce(r.weight,0.0) AS w,
         coalesce(r.dim_semantic,0.0) AS ds,
         coalesce(r.dim_temporal,0.0) AS dt,
         coalesce(r.dim_coactivation,0.0) AS dc,
         coalesce(r.updated_at, datetime()) AS upd
  ORDER BY w DESC
  LIMIT $maxNbr
}
RETURN s, d, t, w, ds, dt, dc, upd
LIMIT $maxTotal`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		edges := make([]Edge, 0, 1024)
		next := make([]string, 0, 1024)
		seenNext := map[string]struct{}{}
		for res.Next(ctx) {
			rec := res.Record()
			s, _ := rec.Get("s")
			d, _ := rec.Get("d")
			t, _ := rec.Get("t")
			w, _ := rec.Get("w")
			ds, _ := rec.Get("ds")
			dt, _ := rec.Get("dt")
			dc, _ := rec.Get("dc")
			upd, _ := rec.Get("upd")

			e := Edge{
				Src: fmt.Sprint(s),
				Dst: fmt.Sprint(d),
				RelType: fmt.Sprint(t),
				Weight: toFloat64(w, 0),
				DimSemantic: toFloat64(ds, 0),
				DimTemporal: toFloat64(dt, 0),
				DimCoactivation: toFloat64(dc, 0),
				UpdatedAt: time.Now(),
			}
			edges = append(edges, e)
			if _, ok := seenNext[e.Dst]; !ok {
				seenNext[e.Dst] = struct{}{}
				next = append(next, e.Dst)
			}
			_ = upd
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return struct {
			Edges []Edge
			Next []string
		}{Edges: edges, Next: next}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	pack := outAny.(struct {
		Edges []Edge
		Next []string
	})
	return pack.Edges, pack.Next, nil
}

// IngestObservation is intentionally minimal: append-only Observation + basic node upsert.
func (s *Service) IngestObservation(ctx context.Context, req models.IngestRequest) (models.IngestResponse, error) {
	if req.SpaceID == "" {
		return models.IngestResponse{}, errors.New("space_id is required")
	}
	if req.Source == "" {
		return models.IngestResponse{}, errors.New("source is required")
	}
	if req.Timestamp == "" {
		return models.IngestResponse{}, errors.New("timestamp is required")
	}

	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = newID("n")
	}
	obsID := newID("o")

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId":     req.SpaceID,
		"nodeId":      nodeID,
		"path":        req.Path,
		"name":        req.Name,
		"obsId":       obsID,
		"timestamp":   req.Timestamp,
		"source":      req.Source,
		"content":     req.Content,
		"tags":        req.Tags,
		"sensitivity": req.Sensitivity,
		"confidence":  req.Confidence,
		"embedding":   req.Embedding, // May be nil/empty
	}

	// Determine merge key: prefer path if provided, else use node_id
	mergeKey := "node_id"
	mergeValue := nodeID
	if req.Path != "" {
		mergeKey = "path"
		mergeValue = req.Path
	}
	params["mergeValue"] = mergeValue

	// Build embedding SET clause (only if embedding provided)
	embeddingClause := ""
	if len(req.Embedding) > 0 {
		embeddingClause = ", n.embedding = $embedding"
	}

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Build cypher dynamically based on merge key
		var cypher string
		if mergeKey == "path" {
			cypher = `
MERGE (t:TapRoot {space_id:$spaceId})
ON CREATE SET t.name='tap_root', t.created_at=datetime()
WITH t
MERGE (n:MemoryNode {space_id:$spaceId, path:$mergeValue})
ON CREATE SET n.node_id=$nodeId,
              n.name=coalesce($name, $mergeValue),
              n.layer=0,
              n.role_type='leaf',
              n.version=1,
              n.status='active',
              n.created_at=datetime(),
              n.updated_at=datetime(),
              n.update_count=0,
              n.summary='',
              n.description='',
              n.confidence=coalesce($confidence, 0.6),
              n.sensitivity=coalesce($sensitivity,'internal'),
              n.tags=coalesce($tags,[])
WITH n
CREATE (o:Observation {
  space_id:$spaceId,
  obs_id:$obsId,
  timestamp: datetime($timestamp),
  source:$source,
  content:$content,
  created_at: datetime()
})
MERGE (n)-[:HAS_OBSERVATION {space_id:$spaceId, created_at:datetime()}]->(o)
SET n.updated_at=datetime(),
    n.update_count = coalesce(n.update_count,0) + 1` + embeddingClause + `
RETURN n.node_id AS node_id`
		} else {
			cypher = `
MERGE (t:TapRoot {space_id:$spaceId})
ON CREATE SET t.name='tap_root', t.created_at=datetime()
WITH t
MERGE (n:MemoryNode {space_id:$spaceId, node_id:$mergeValue})
ON CREATE SET n.path=coalesce($path, $mergeValue),
              n.name=coalesce($name, $mergeValue),
              n.layer=0,
              n.role_type='leaf',
              n.version=1,
              n.status='active',
              n.created_at=datetime(),
              n.updated_at=datetime(),
              n.update_count=0,
              n.summary='',
              n.description='',
              n.confidence=coalesce($confidence, 0.6),
              n.sensitivity=coalesce($sensitivity,'internal'),
              n.tags=coalesce($tags,[])
WITH n
CREATE (o:Observation {
  space_id:$spaceId,
  obs_id:$obsId,
  timestamp: datetime($timestamp),
  source:$source,
  content:$content,
  created_at: datetime()
})
MERGE (n)-[:HAS_OBSERVATION {space_id:$spaceId, created_at:datetime()}]->(o)
SET n.updated_at=datetime(),
    n.update_count = coalesce(n.update_count,0) + 1` + embeddingClause + `
RETURN n.node_id AS node_id`
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})
	if err != nil {
		return models.IngestResponse{}, err
	}

	// Semantic edge creation: link to similar existing nodes
	if s.cfg.SemanticEdgeOnIngest && len(req.Embedding) > 0 {
		similarNodes, findErr := s.FindSimilarNodes(ctx, req.SpaceID, req.Embedding, nodeID, s.cfg.SemanticEdgeTopN)
		if findErr != nil {
			// Log warning but don't fail the ingest
			log.Printf("WARN: FindSimilarNodes failed for node %s: %v", nodeID, findErr)
		} else {
			for _, sn := range similarNodes {
				if sn.Score >= s.cfg.SemanticEdgeMinSimilarity {
					edgeErr := s.CreateAssociatedWithEdge(ctx, req.SpaceID, nodeID, sn.NodeID, sn.Score)
					if edgeErr != nil {
						// Log warning but don't fail the ingest
						log.Printf("WARN: CreateAssociatedWithEdge failed from %s to %s: %v", nodeID, sn.NodeID, edgeErr)
					}
				}
			}
		}
	}

	return models.IngestResponse{SpaceID: req.SpaceID, NodeID: nodeID, ObsID: obsID}, nil
}

// BatchIngestObservations processes multiple observations in a single batch.
// Returns results for each item, supporting partial success (some items may fail while others succeed).
func (s *Service) BatchIngestObservations(ctx context.Context, req models.BatchIngestRequest) (models.BatchIngestResponse, error) {
	if req.SpaceID == "" {
		return models.BatchIngestResponse{}, errors.New("space_id is required")
	}
	if len(req.Observations) == 0 {
		return models.BatchIngestResponse{}, errors.New("observations array is required and must not be empty")
	}

	results := make([]models.BatchIngestResult, 0, len(req.Observations))
	successCount := 0
	errorCount := 0

	for i, obs := range req.Observations {
		// Convert BatchIngestItem to IngestRequest
		ingestReq := models.IngestRequest{
			SpaceID:     req.SpaceID,
			Timestamp:   obs.Timestamp,
			Source:      obs.Source,
			Content:     obs.Content,
			Tags:        obs.Tags,
			NodeID:      obs.NodeID,
			Path:        obs.Path,
			Name:        obs.Name,
			Sensitivity: obs.Sensitivity,
			Confidence:  obs.Confidence,
			Embedding:   obs.Embedding,
		}

		// Validate required fields
		if ingestReq.Source == "" {
			results = append(results, models.BatchIngestResult{
				Index:  i,
				Status: "error",
				Error:  "source is required",
			})
			errorCount++
			continue
		}
		if ingestReq.Timestamp == "" {
			results = append(results, models.BatchIngestResult{
				Index:  i,
				Status: "error",
				Error:  "timestamp is required",
			})
			errorCount++
			continue
		}

		// Use existing IngestObservation logic
		resp, err := s.IngestObservation(ctx, ingestReq)
		if err != nil {
			results = append(results, models.BatchIngestResult{
				Index:  i,
				Status: "error",
				Error:  err.Error(),
			})
			errorCount++
			continue
		}

		result := models.BatchIngestResult{
			Index:  i,
			Status: "success",
			NodeID: resp.NodeID,
			ObsID:  resp.ObsID,
		}
		if resp.EmbeddingDims > 0 {
			result.EmbeddingDims = resp.EmbeddingDims
		}
		results = append(results, result)
		successCount++
	}

	return models.BatchIngestResponse{
		SpaceID:      req.SpaceID,
		TotalItems:   len(req.Observations),
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		Results:      results,
	}, nil
}

func newID(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func toFloat64(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return def
	}
}

func toInt(v any, def int) int {
	switch x := v.(type) {
	case int64:
		return int(x)
	case int:
		return x
	case float64:
		return int(x)
	default:
		return def
	}
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		result := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// helpers used by scoring
func expRecency(updatedAt time.Time, rho float64) float64 {
	age := time.Since(updatedAt).Hours() / 24.0
	return math.Exp(-rho * age)
}
