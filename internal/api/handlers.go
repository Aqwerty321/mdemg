package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/anomaly"
	"mdemg/internal/db"
	"mdemg/internal/models"
	"mdemg/internal/symbols"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := db.AssertSchemaVersion(ctx, s.driver, s.cfg.RequiredSchemaVersion); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "error": err.Error()})
		return
	}

	resp := map[string]any{"status": "ready"}
	if s.embedder != nil {
		resp["embedding_provider"] = s.embedder.Name()
		resp["embedding_dimensions"] = s.embedder.Dimensions()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRetrieve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.RetrieveRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Generate embedding from query_text if provided and no embedding given
	if len(req.QueryEmbedding) == 0 && req.QueryText != "" {
		if s.embedder == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "query_text provided but no embedding provider configured (set EMBEDDING_PROVIDER env var)",
			})
			return
		}
		emb, err := s.embedder.Embed(r.Context(), req.QueryText)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": fmt.Sprintf("failed to generate embedding: %v", err),
			})
			return
		}
		req.QueryEmbedding = emb
	}

	resp, err := s.retriever.Retrieve(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// Add embedding info to debug
	if s.embedder != nil && resp.Debug != nil {
		resp.Debug["embedding_provider"] = s.embedder.Name()
	}

	// Fetch symbol evidence for each result (when requested)
	if req.IncludeEvidence && s.symbolStore != nil && len(resp.Results) > 0 {
		metrics := &models.EvidenceMetrics{
			TotalResults: len(resp.Results),
		}
		for i := range resp.Results {
			symbols, err := s.symbolStore.GetSymbolsForMemoryNode(r.Context(), req.SpaceID, resp.Results[i].NodeID)
			if err != nil {
				// Log but don't fail - evidence is optional enrichment
				log.Printf("warning: failed to fetch symbols for node %s: %v", resp.Results[i].NodeID, err)
				continue
			}
			if len(symbols) > 0 {
				evidence := make([]models.SymbolEvidence, 0, len(symbols))
				for _, sym := range symbols {
					evidence = append(evidence, models.SymbolEvidence{
						SymbolName: sym.Name,
						SymbolType: sym.SymbolType,
						FilePath:   sym.FilePath,
						LineNumber: sym.LineNumber,
						EndLine:    sym.EndLine,
						Value:      sym.Value,
						RawValue:   sym.RawValue,
						Signature:  sym.Signature,
						DocComment: sym.DocComment,
					})
				}
				resp.Results[i].Evidence = evidence
				metrics.ResultsWithEvidence++
				metrics.TotalSymbols += len(symbols)
			}
		}
		// Calculate compliance metrics
		if metrics.TotalResults > 0 {
			metrics.ComplianceRate = float64(metrics.ResultsWithEvidence) / float64(metrics.TotalResults)
		}
		if metrics.ResultsWithEvidence > 0 {
			metrics.AvgSymbolsPerResult = float64(metrics.TotalSymbols) / float64(metrics.ResultsWithEvidence)
		}
		resp.EvidenceMetrics = metrics
	}

	// Learning deltas: bounded writeback
	_ = s.learner.ApplyCoactivation(r.Context(), req.SpaceID, resp)

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.IngestRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Generate embedding if not provided and embedder is available
	if len(req.Embedding) == 0 && s.embedder != nil {
		// Build text for embedding from content
		textForEmbedding := contentToText(req.Content)
		if req.Name != "" {
			textForEmbedding = req.Name + ": " + textForEmbedding
		}

		if textForEmbedding != "" {
			emb, err := s.embedder.Embed(r.Context(), textForEmbedding)
			if err != nil {
				// Log but don't fail - embedding is optional
				// log.Printf("WARNING: failed to generate embedding: %v", err)
			} else {
				req.Embedding = emb
			}
		}
	}

	out, err := s.retriever.IngestObservation(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// Include embedding dimensions in response if generated
	if len(req.Embedding) > 0 {
		out.EmbeddingDims = len(req.Embedding)
	}

	// Run anomaly detection (non-blocking - errors are logged, not returned)
	if s.anomalyDetector != nil {
		dctx := anomaly.DetectionContext{
			SpaceID:   req.SpaceID,
			NodeID:    out.NodeID,
			Content:   contentToText(req.Content),
			Embedding: req.Embedding,
			Tags:      req.Tags,
			IsUpdate:  req.NodeID != "", // If node_id was provided, this is an update
		}
		out.Anomalies = s.anomalyDetector.Detect(r.Context(), dctx)
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBatchIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.BatchIngestRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Validate batch size limit
	if len(req.Observations) > s.cfg.BatchIngestMaxItems {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("batch size %d exceeds maximum allowed %d items", len(req.Observations), s.cfg.BatchIngestMaxItems),
		})
		return
	}

	// Generate embeddings for items that don't have them
	if s.embedder != nil {
		for i := range req.Observations {
			if len(req.Observations[i].Embedding) == 0 {
				textForEmbedding := contentToText(req.Observations[i].Content)
				if req.Observations[i].Name != "" {
					textForEmbedding = req.Observations[i].Name + ": " + textForEmbedding
				}
				if textForEmbedding != "" {
					emb, err := s.embedder.Embed(r.Context(), textForEmbedding)
					if err == nil {
						req.Observations[i].Embedding = emb
					}
					// Embedding errors are non-fatal for batch ingest
				}
			}
		}
	}

	resp, err := s.retriever.BatchIngestObservations(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// Store symbols if any were provided
	// Build path->nodeID map from response to link symbols directly
	// (avoids transaction isolation issues with MATCH by path)
	if s.symbolStore != nil {
		pathToNodeID := make(map[string]string)
		for i, result := range resp.Results {
			if result.Status == "success" && result.NodeID != "" && i < len(req.Observations) {
				pathToNodeID[req.Observations[i].Path] = result.NodeID
			}
		}

		var allSymbols []symbols.SymbolRecord
		for _, obs := range req.Observations {
			if len(obs.Symbols) > 0 {
				nodeID := pathToNodeID[obs.Path]
				for _, sym := range obs.Symbols {
					rec := symbols.SymbolRecord{
						SpaceID:        req.SpaceID,
						SymbolID:       symbols.GenerateSymbolID(req.SpaceID, obs.Path, sym.Name, sym.LineNumber),
						Name:           sym.Name,
						SymbolType:     sym.Type,
						Value:          sym.Value,
						RawValue:       sym.RawValue,
						FilePath:       obs.Path,
						ParentNodeID:   nodeID, // Direct link to MemoryNode
						LineNumber:     sym.LineNumber,
						EndLine:        sym.EndLine,
						Exported:       sym.Exported,
						DocComment:     sym.DocComment,
						Signature:      sym.Signature,
						Parent:         sym.Parent,
						Language:       sym.Language,
						TypeAnnotation: sym.TypeAnnotation,
					}
					allSymbols = append(allSymbols, rec)
				}
			}
		}
		if len(allSymbols) > 0 {
			if err := s.symbolStore.SaveSymbols(r.Context(), req.SpaceID, allSymbols); err != nil {
				log.Printf("WARNING: failed to save symbols: %v", err)
				// Non-fatal - continue with response
			} else {
				log.Printf("Stored %d symbols for space %s", len(allSymbols), req.SpaceID)
			}
		}
	}

	// Set appropriate status code based on results
	statusCode := http.StatusOK
	if resp.ErrorCount > 0 && resp.SuccessCount > 0 {
		statusCode = http.StatusMultiStatus // 207 for partial success
	} else if resp.ErrorCount > 0 && resp.SuccessCount == 0 {
		statusCode = http.StatusBadRequest // All failed
	}

	writeJSON(w, statusCode, resp)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse optional space_id query parameter
	spaceID := r.URL.Query().Get("space_id")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Execute metrics queries
	resp, err := s.queryMetrics(ctx, spaceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// queryMetrics executes Neo4j queries to gather graph metrics
func (s *Server) queryMetrics(ctx context.Context, spaceID string) (models.MetricsResponse, error) {
	// Initialize response with empty maps and slices (avoid nil)
	resp := models.MetricsResponse{
		NodesByLayer:   make(map[int]int64),
		EdgesByType:    make(map[string]int64),
		HubNodes:       []models.HubNode{},
		RecentActivity: &models.ActivityStats{},
	}

	// Convert empty string to nil for Cypher NULL handling
	var spaceParam any
	if spaceID != "" {
		spaceParam = spaceID
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceParam,
	}

	// Query 1: Total nodes and nodes by layer
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE $spaceId IS NULL OR n.space_id = $spaceId
WITH count(n) AS total, collect(coalesce(n.layer, 0)) AS layers
RETURN total, layers`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if total, ok := rec.Get("total"); ok {
				resp.TotalNodes = toInt64(total)
			}
			if layers, ok := rec.Get("layers"); ok {
				if layerList, ok := layers.([]any); ok {
					for _, l := range layerList {
						layer := int(toInt64(l))
						resp.NodesByLayer[layer]++
					}
				}
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query node metrics: %w", err)
	}

	// Query 2: Total edges and edges by type
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE $spaceId IS NULL OR (a.space_id = $spaceId AND b.space_id = $spaceId)
RETURN type(r) AS rel_type, count(r) AS cnt`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			relType, _ := rec.Get("rel_type")
			cnt, _ := rec.Get("cnt")
			if relType != nil {
				resp.EdgesByType[fmt.Sprint(relType)] = toInt64(cnt)
				resp.TotalEdges += toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query edge metrics: %w", err)
	}

	// Query 3: Hub nodes (top 10 by degree)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE $spaceId IS NULL OR n.space_id = $spaceId
OPTIONAL MATCH (n)-[r]-()
WITH n, count(r) AS degree
ORDER BY degree DESC
LIMIT 10
RETURN n.node_id AS node_id, coalesce(n.name, n.node_id) AS name, degree`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			degree, _ := rec.Get("degree")
			if nodeID != nil {
				resp.HubNodes = append(resp.HubNodes, models.HubNode{
					NodeID: fmt.Sprint(nodeID),
					Name:   fmt.Sprint(name),
					Degree: int(toInt64(degree)),
				})
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query hub nodes: %w", err)
	}

	// Query 4: Orphan nodes (nodes with no edges)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE ($spaceId IS NULL OR n.space_id = $spaceId)
  AND NOT (n)-[]-()
RETURN count(n) AS orphan_count`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("orphan_count"); ok {
				resp.OrphanNodes = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query orphan nodes: %w", err)
	}

	// Query 5: Average edge weight
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE $spaceId IS NULL OR (a.space_id = $spaceId AND b.space_id = $spaceId)
RETURN avg(coalesce(r.weight, 0.0)) AS avg_weight`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if avgW, ok := rec.Get("avg_weight"); ok {
				resp.AvgEdgeWeight = toFloat64Val(avgW, 0.0)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query avg edge weight: %w", err)
	}

	// Query 6: Recent activity (24h window) - nodes created
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE ($spaceId IS NULL OR n.space_id = $spaceId)
  AND n.created_at >= datetime() - duration('P1D')
RETURN count(n) AS nodes_created`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("nodes_created"); ok {
				resp.RecentActivity.NodesCreated = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query recent nodes: %w", err)
	}

	// Query 7: Recent activity (24h window) - edges created
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE ($spaceId IS NULL OR (a.space_id = $spaceId AND b.space_id = $spaceId))
  AND r.created_at >= datetime() - duration('P1D')
RETURN count(r) AS edges_created`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("edges_created"); ok {
				resp.RecentActivity.EdgesCreated = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query recent edges: %w", err)
	}

	// Note: Retrievals cannot be tracked from Neo4j as retrieval counts
	// are not persisted to the database (per design: no per-request writes)

	return resp, nil
}

// toInt64 converts various Neo4j numeric types to int64
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	default:
		return 0
	}
}

// toFloat64Val converts various Neo4j numeric types to float64 with default
func toFloat64Val(v any, def float64) float64 {
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

// handleStats returns comprehensive per-space memory statistics
// GET /v1/memory/stats?space_id={space_id}
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse required space_id query parameter
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.queryStats(ctx, spaceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// queryStats executes Neo4j queries to gather per-space memory statistics
func (s *Server) queryStats(ctx context.Context, spaceID string) (models.StatsResponse, error) {
	// Initialize response with empty maps and slices (avoid nil)
	resp := models.StatsResponse{
		SpaceID:              spaceID,
		MemoriesByLayer:      make(map[int]int64),
		LearningActivity:     &models.LearningActivity{},
		TemporalDistribution: &models.TemporalDistribution{},
		Connectivity:         &models.Connectivity{},
		ComputedAt:           time.Now().UTC().Format(time.RFC3339),
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
	}

	// Query 1: Memory count and memories by layer
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
WITH count(n) AS total, collect(coalesce(n.layer, 0)) AS layers
RETURN total, layers`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if total, ok := rec.Get("total"); ok {
				resp.MemoryCount = toInt64(total)
			}
			if layers, ok := rec.Get("layers"); ok {
				if layerList, ok := layers.([]any); ok {
					for _, l := range layerList {
						layer := int(toInt64(l))
						resp.MemoriesByLayer[layer]++
					}
				}
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query memory count: %w", err)
	}

	// Query 2: Observation count
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)-[:HAS_OBSERVATION]->(o:Observation)
WHERE n.space_id = $spaceId
RETURN count(o) AS total`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if total, ok := rec.Get("total"); ok {
				resp.ObservationCount = toInt64(total)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query observation count: %w", err)
	}

	// Query 3: Embedding coverage
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
WITH count(n) AS total,
     sum(CASE WHEN n.embedding IS NOT NULL AND size(n.embedding) > 0 THEN 1 ELSE 0 END) AS with_embedding,
     avg(CASE WHEN n.embedding IS NOT NULL AND size(n.embedding) > 0 THEN size(n.embedding) ELSE NULL END) AS avg_dims
RETURN total, with_embedding, avg_dims`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			total, _ := rec.Get("total")
			withEmb, _ := rec.Get("with_embedding")
			avgDims, _ := rec.Get("avg_dims")

			totalCount := toInt64(total)
			withEmbCount := toInt64(withEmb)
			if totalCount > 0 {
				resp.EmbeddingCoverage = float64(withEmbCount) / float64(totalCount)
			}
			resp.AvgEmbeddingDimensions = int(toFloat64Val(avgDims, 0.0))
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query embedding coverage: %w", err)
	}

	// Query 4: Learning activity (CO_ACTIVATED_WITH edges)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode)-[r:CO_ACTIVATED_WITH]->(b:MemoryNode)
WHERE a.space_id = $spaceId AND b.space_id = $spaceId
RETURN count(r) AS edge_count,
       avg(coalesce(r.weight, 0.0)) AS avg_weight,
       max(coalesce(r.weight, 0.0)) AS max_weight`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("edge_count"); ok {
				resp.LearningActivity.CoActivatedEdges = toInt64(cnt)
			}
			if avgW, ok := rec.Get("avg_weight"); ok {
				resp.LearningActivity.AvgWeight = toFloat64Val(avgW, 0.0)
			}
			if maxW, ok := rec.Get("max_weight"); ok {
				resp.LearningActivity.MaxWeight = toFloat64Val(maxW, 0.0)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query learning activity: %w", err)
	}

	// Query 5: Temporal distribution - last 24h
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND n.created_at >= datetime() - duration('P1D')
RETURN count(n) AS count_24h`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("count_24h"); ok {
				resp.TemporalDistribution.Last24h = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query temporal distribution (24h): %w", err)
	}

	// Query 6: Temporal distribution - last 7d
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND n.created_at >= datetime() - duration('P7D')
RETURN count(n) AS count_7d`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("count_7d"); ok {
				resp.TemporalDistribution.Last7d = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query temporal distribution (7d): %w", err)
	}

	// Query 7: Temporal distribution - last 30d
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND n.created_at >= datetime() - duration('P30D')
RETURN count(n) AS count_30d`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("count_30d"); ok {
				resp.TemporalDistribution.Last30d = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query temporal distribution (30d): %w", err)
	}

	// Query 8: Connectivity stats
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
OPTIONAL MATCH (n)-[r]-()
WITH n, count(r) AS degree
RETURN avg(degree) AS avg_degree, max(degree) AS max_degree`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if avgD, ok := rec.Get("avg_degree"); ok {
				resp.Connectivity.AvgDegree = toFloat64Val(avgD, 0.0)
			}
			if maxD, ok := rec.Get("max_degree"); ok {
				resp.Connectivity.MaxDegree = int(toInt64(maxD))
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query connectivity stats: %w", err)
	}

	// Query 9: Orphan count (nodes with no edges)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND NOT (n)-[]-()
RETURN count(n) AS orphan_count`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("orphan_count"); ok {
				resp.Connectivity.OrphanCount = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query orphan count: %w", err)
	}

	// Compute health score
	resp.HealthScore = computeHealthScore(resp)

	return resp, nil
}

// computeHealthScore calculates a 0.0-1.0 health score based on:
// - Embedding coverage (40% weight)
// - Connectivity (30% weight) - penalized for too many orphans
// - Recency (30% weight) - based on recent activity
func computeHealthScore(resp models.StatsResponse) float64 {
	if resp.MemoryCount == 0 {
		return 0.0
	}

	// Embedding coverage component (40%)
	embeddingScore := resp.EmbeddingCoverage * 0.4

	// Connectivity component (30%)
	// Score based on orphan ratio - lower is better
	orphanRatio := float64(resp.Connectivity.OrphanCount) / float64(resp.MemoryCount)
	connectivityScore := (1.0 - orphanRatio) * 0.3

	// Recency component (30%)
	// Score based on ratio of 7d activity to total
	var recencyScore float64
	if resp.MemoryCount > 0 {
		recentRatio := float64(resp.TemporalDistribution.Last7d) / float64(resp.MemoryCount)
		// Cap at 1.0 (if all memories are recent)
		if recentRatio > 1.0 {
			recentRatio = 1.0
		}
		recencyScore = recentRatio * 0.3
	}

	// Combine scores
	healthScore := embeddingScore + connectivityScore + recencyScore

	// Ensure result is in 0.0-1.0 range
	if healthScore < 0.0 {
		healthScore = 0.0
	}
	if healthScore > 1.0 {
		healthScore = 1.0
	}

	return healthScore
}

func (s *Server) handleReflect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.ReflectRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Generate embedding from topic if provided and no embedding given
	if len(req.TopicEmbedding) == 0 && req.Topic != "" {
		if s.embedder == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "topic provided but no embedding provider configured (set EMBEDDING_PROVIDER env var)",
			})
			return
		}
		emb, err := s.embedder.Embed(r.Context(), req.Topic)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": fmt.Sprintf("failed to generate embedding: %v", err),
			})
			return
		}
		req.TopicEmbedding = emb
	}

	resp, err := s.retriever.Reflect(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleConsolidate handles POST /v1/memory/consolidate
// Triggers hidden layer creation via DBSCAN clustering and runs message passing (forward + backward passes)
func (s *Server) handleConsolidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.ConsolidateRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Check if hidden layer is enabled
	if !s.cfg.HiddenLayerEnabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": models.ConsolidateResponse{
				SpaceID: req.SpaceID,
				Enabled: false,
			},
		})
		return
	}

	// Run full consolidation (or partial based on skip flags)
	var resp models.ConsolidateResponse
	resp.SpaceID = req.SpaceID
	resp.Enabled = true

	start := time.Now()

	// Step 1: Create hidden nodes from orphan base data (unless skipped)
	if !req.SkipClustering {
		created, err := s.hiddenLayer.CreateHiddenNodes(r.Context(), req.SpaceID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": fmt.Sprintf("failed to create hidden nodes: %v", err),
			})
			return
		}
		resp.HiddenNodesCreated = created
	}

	// Step 1b: Create concern nodes for cross-cutting patterns (P1 improvement)
	if !req.SkipClustering {
		concernResult, err := s.hiddenLayer.CreateConcernNodes(r.Context(), req.SpaceID)
		if err != nil {
			// Log but don't fail - concern nodes are an enhancement
			log.Printf("warning: failed to create concern nodes: %v", err)
		} else if concernResult != nil {
			resp.ConcernNodesCreated = concernResult.ConcernNodesCreated
			resp.ConcernEdgesCreated = concernResult.EdgesCreated
		}
	}

	// Step 1c: Create config summary node (P2 Track 4.3)
	if !req.SkipClustering {
		configResult, err := s.hiddenLayer.CreateConfigNodes(r.Context(), req.SpaceID)
		if err != nil {
			// Log but don't fail - config nodes are an enhancement
			log.Printf("warning: failed to create config nodes: %v", err)
		} else if configResult != nil && configResult.ConfigNodeCreated {
			resp.ConfigNodeCreated = true
			resp.ConfigEdgesCreated = configResult.EdgesCreated
		}
	}

	// Step 1d: Create comparison nodes for similar modules (P2 Track 3)
	if !req.SkipClustering {
		compResult, err := s.hiddenLayer.CreateComparisonNodes(r.Context(), req.SpaceID)
		if err != nil {
			// Log but don't fail - comparison nodes are an enhancement
			log.Printf("warning: failed to create comparison nodes: %v", err)
		} else if compResult != nil && compResult.ComparisonNodesCreated > 0 {
			resp.ComparisonNodesCreated = compResult.ComparisonNodesCreated
			resp.ComparisonEdgesCreated = compResult.EdgesCreated
		}
	}

	// Step 1e: Create temporal pattern nodes (P3 Track 5)
	if !req.SkipClustering {
		tempResult, err := s.hiddenLayer.CreateTemporalNodes(r.Context(), req.SpaceID)
		if err != nil {
			// Log but don't fail - temporal nodes are an enhancement
			log.Printf("warning: failed to create temporal nodes: %v", err)
		} else if tempResult != nil && tempResult.TemporalNodeCreated {
			resp.TemporalNodeCreated = true
			resp.TemporalEdgesCreated = tempResult.EdgesCreated
		}
	}

	// Step 1f: Create UI pattern nodes (P4 Track 6)
	if !req.SkipClustering {
		uiResult, err := s.hiddenLayer.CreateUINodes(r.Context(), req.SpaceID)
		if err != nil {
			// Log but don't fail - UI nodes are an enhancement
			log.Printf("warning: failed to create UI nodes: %v", err)
		} else if uiResult != nil && uiResult.UINodesCreated > 0 {
			resp.UINodesCreated = uiResult.UINodesCreated
			resp.UIEdgesCreated = uiResult.EdgesCreated
		}
	}

	// Step 2: Forward pass (unless skipped)
	if !req.SkipForward {
		fwdResult, err := s.hiddenLayer.ForwardPass(r.Context(), req.SpaceID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": fmt.Sprintf("failed in forward pass: %v", err),
			})
			return
		}
		resp.HiddenNodesUpdated = fwdResult.HiddenNodesUpdated
		resp.ConceptNodesUpdated = fwdResult.ConceptNodesUpdated
	}

	// Step 3: Multi-layer concept clustering (unless skipped)
	// Build concept layers: hidden (L1) → concepts (L2, L3, etc.)
	// Try ALL layers - upper layers have adaptive (looser) constraints for emergence
	if !req.SkipClustering {
		maxLayers := 5
		for targetLayer := 2; targetLayer <= maxLayers; targetLayer++ {
			conceptCreated, err := s.hiddenLayer.CreateConceptNodes(r.Context(), req.SpaceID, targetLayer)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"error": fmt.Sprintf("failed to create concept nodes layer %d: %v", targetLayer, err),
				})
				return
			}
			// Don't break on zero - upper layers may still form clusters
			if conceptCreated > 0 {
				resp.ConceptNodesCreated += conceptCreated

				// Run forward pass to update new concept embeddings
				fwdResult, err := s.hiddenLayer.ForwardPass(r.Context(), req.SpaceID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]any{
						"error": fmt.Sprintf("failed in forward pass after layer %d: %v", targetLayer, err),
					})
					return
				}
				resp.ConceptNodesUpdated += fwdResult.ConceptNodesUpdated
			}
		}
	}

	// Step 4: Backward pass (unless skipped)
	if !req.SkipBackward {
		bwdResult, err := s.hiddenLayer.BackwardPass(r.Context(), req.SpaceID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": fmt.Sprintf("failed in backward pass: %v", err),
			})
			return
		}
		// Add to existing count if forward pass was also run
		resp.HiddenNodesUpdated += bwdResult.HiddenNodesUpdated
		resp.EdgesStrengthened = bwdResult.EdgesStrengthened
	}

	// Step 5: Generate summaries for hidden and concept nodes
	summariesUpdated, err := s.hiddenLayer.GenerateSummaries(r.Context(), req.SpaceID)
	if err != nil {
		// Log but don't fail - summaries are nice-to-have
		log.Printf("warning: failed to generate summaries: %v", err)
	}
	resp.SummariesGenerated = summariesUpdated

	resp.DurationMs = float64(time.Since(start).Milliseconds())

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// contentToText converts the content field to a string for embedding
func contentToText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case map[string]any:
		// Try common text fields
		if text, ok := v["text"].(string); ok {
			return text
		}
		if text, ok := v["content"].(string); ok {
			return text
		}
		if text, ok := v["message"].(string); ok {
			return text
		}
		// Fallback: marshal to JSON
		// This is intentionally simple - caller should pass string content
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// handleArchiveNode handles POST /v1/memory/nodes/{node_id}/archive
// Soft-deletes a memory node by setting is_archived=true and archived_at timestamp
func (s *Server) handleArchiveNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract node_id from URL path: /v1/memory/nodes/{node_id}/archive
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/nodes/")
	nodeID := strings.TrimSuffix(path, "/archive")
	if nodeID == "" || nodeID == path {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid node_id in path"})
		return
	}

	// Parse optional ArchiveRequest (body may be empty)
	var req models.ArchiveRequest
	if r.ContentLength > 0 {
		if !readJSON(w, r, &req) {
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"nodeId": nodeID,
		"reason": req.Reason,
	}

	// Execute archive operation
	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
SET n.is_archived = true,
    n.archived_at = datetime(),
    n.archive_reason = $reason
RETURN n.node_id AS node_id,
       coalesce(n.name, n.node_id) AS name,
       n.archived_at AS archived_at`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			nodeIDVal, _ := rec.Get("node_id")
			nameVal, _ := rec.Get("name")
			archivedAtVal, _ := rec.Get("archived_at")

			// Convert archived_at to ISO string
			var archivedAtStr string
			if at, ok := archivedAtVal.(neo4j.LocalDateTime); ok {
				archivedAtStr = at.Time().Format(time.RFC3339)
			} else if at, ok := archivedAtVal.(time.Time); ok {
				archivedAtStr = at.Format(time.RFC3339)
			} else {
				archivedAtStr = fmt.Sprint(archivedAtVal)
			}

			return &models.ArchiveResponse{
				NodeID:     fmt.Sprint(nodeIDVal),
				Name:       fmt.Sprint(nameVal),
				ArchivedAt: archivedAtStr,
				Reason:     req.Reason,
			}, nil
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		// Node not found
		return nil, nil
	})

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("node not found: %s", nodeID)})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleUnarchiveNode handles POST /v1/memory/nodes/{node_id}/unarchive
// Restores an archived memory node by clearing is_archived, archived_at, and archive_reason
func (s *Server) handleUnarchiveNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract node_id from URL path: /v1/memory/nodes/{node_id}/unarchive
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/nodes/")
	nodeID := strings.TrimSuffix(path, "/unarchive")
	if nodeID == "" || nodeID == path {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid node_id in path"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"nodeId": nodeID,
	}

	// Execute unarchive operation
	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
REMOVE n.is_archived, n.archived_at, n.archive_reason
WITH n, datetime() AS unarchiveTime
RETURN n.node_id AS node_id,
       coalesce(n.name, n.node_id) AS name,
       unarchiveTime AS unarchived_at`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			nodeIDVal, _ := rec.Get("node_id")
			nameVal, _ := rec.Get("name")
			unarchivedAtVal, _ := rec.Get("unarchived_at")

			// Convert unarchived_at to ISO string
			var unarchivedAtStr string
			if at, ok := unarchivedAtVal.(neo4j.LocalDateTime); ok {
				unarchivedAtStr = at.Time().Format(time.RFC3339)
			} else if at, ok := unarchivedAtVal.(time.Time); ok {
				unarchivedAtStr = at.Format(time.RFC3339)
			} else {
				unarchivedAtStr = fmt.Sprint(unarchivedAtVal)
			}

			return &models.UnarchiveResponse{
				NodeID:       fmt.Sprint(nodeIDVal),
				Name:         fmt.Sprint(nameVal),
				UnarchivedAt: unarchivedAtStr,
			}, nil
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		// Node not found
		return nil, nil
	})

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("node not found: %s", nodeID)})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleBulkArchive handles POST /v1/memory/archive/bulk
// Archives multiple memory nodes in a single request with partial success support
func (s *Server) handleBulkArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.BulkArchiveRequest
	if !readJSON(w, r, &req) {
		return
	}

	// Validate non-empty node_ids array
	if len(req.NodeIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "node_ids array cannot be empty",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Process each node
	results := make([]models.BulkArchiveResult, 0, len(req.NodeIDs))
	var successCount, errorCount int

	for _, nodeID := range req.NodeIDs {
		params := map[string]any{
			"nodeId": nodeID,
			"reason": req.Reason,
		}

		// Execute archive operation for this node
		result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
SET n.is_archived = true,
    n.archived_at = datetime(),
    n.archive_reason = $reason
RETURN n.node_id AS node_id, n.archived_at AS archived_at`
			res, err := tx.Run(ctx, cypher, params)
			if err != nil {
				return nil, err
			}
			if res.Next(ctx) {
				rec := res.Record()
				archivedAtVal, _ := rec.Get("archived_at")

				// Convert archived_at to ISO string
				var archivedAtStr string
				if at, ok := archivedAtVal.(neo4j.LocalDateTime); ok {
					archivedAtStr = at.Time().Format(time.RFC3339)
				} else if at, ok := archivedAtVal.(time.Time); ok {
					archivedAtStr = at.Format(time.RFC3339)
				} else {
					archivedAtStr = fmt.Sprint(archivedAtVal)
				}

				return archivedAtStr, nil
			}
			if err := res.Err(); err != nil {
				return nil, err
			}
			// Node not found
			return nil, nil
		})

		if err != nil {
			results = append(results, models.BulkArchiveResult{
				NodeID: nodeID,
				Status: "error",
				Error:  err.Error(),
			})
			errorCount++
		} else if result == nil {
			results = append(results, models.BulkArchiveResult{
				NodeID: nodeID,
				Status: "error",
				Error:  "node not found",
			})
			errorCount++
		} else {
			results = append(results, models.BulkArchiveResult{
				NodeID:     nodeID,
				Status:     "success",
				ArchivedAt: result.(string),
			})
			successCount++
		}
	}

	resp := models.BulkArchiveResponse{
		SpaceID:      req.SpaceID,
		TotalItems:   len(req.NodeIDs),
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		Results:      results,
	}

	// Set appropriate status code based on results
	statusCode := http.StatusOK
	if errorCount > 0 && successCount > 0 {
		statusCode = http.StatusMultiStatus // 207 for partial success
	} else if errorCount > 0 && successCount == 0 {
		statusCode = http.StatusBadRequest // All failed
	}

	writeJSON(w, statusCode, resp)
}

// handleDeleteNode handles DELETE /v1/memory/nodes/{node_id}
// Hard-deletes a memory node (DETACH DELETE) with safety checks
func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract node_id from URL path: /v1/memory/nodes/{node_id}
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/nodes/")
	nodeID := path
	if nodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid node_id in path"})
		return
	}

	// Require ?confirm=true query parameter for safety
	if r.URL.Query().Get("confirm") != "true" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "deletion requires ?confirm=true query parameter",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"nodeId": nodeID,
	}

	// Check for outgoing ABSTRACTS_TO edges (block deletion if present)
	hasAbstractions, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})-[:ABSTRACTS_TO]->()
RETURN count(*) > 0 AS has_abstractions`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if val, ok := rec.Get("has_abstractions"); ok {
				if b, ok := val.(bool); ok {
					return b, nil
				}
			}
		}
		return false, res.Err()
	})

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if hasAbstractions.(bool) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "cannot delete node with outgoing ABSTRACTS_TO edges; delete child abstractions first or use archive instead",
		})
		return
	}

	// Execute delete operation with DETACH DELETE
	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// First check if node exists and count edges
		checkCypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
OPTIONAL MATCH (n)-[r]-()
RETURN n.node_id AS node_id, count(DISTINCT r) AS edge_count`
		checkRes, err := tx.Run(ctx, checkCypher, params)
		if err != nil {
			return nil, err
		}

		var edgeCount int64
		var foundNodeID string
		if checkRes.Next(ctx) {
			rec := checkRes.Record()
			if nid, ok := rec.Get("node_id"); ok && nid != nil {
				foundNodeID = fmt.Sprint(nid)
			}
			if ec, ok := rec.Get("edge_count"); ok {
				edgeCount = toInt64(ec)
			}
		}
		if err := checkRes.Err(); err != nil {
			return nil, err
		}

		if foundNodeID == "" {
			// Node not found
			return nil, nil
		}

		// Execute DETACH DELETE
		deleteCypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
DETACH DELETE n`
		_, err = tx.Run(ctx, deleteCypher, params)
		if err != nil {
			return nil, err
		}

		return &models.DeleteResponse{
			NodeID:       nodeID,
			DeletedNodes: 1,
			DeletedEdges: int(edgeCount),
		}, nil
	})

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("node not found: %s", nodeID)})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleLearningPrune handles POST /v1/learning/prune
// Prunes decayed and excess learning edges (CO_ACTIVATED_WITH) for a space.
func (s *Server) handleLearningPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	ctx := r.Context()

	// Prune decayed edges (below threshold after time decay)
	decayedDeleted, err := s.learner.PruneDecayedEdges(ctx, spaceID)
	if err != nil {
		log.Printf("ERROR: PruneDecayedEdges failed for space %s: %v", spaceID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// Prune excess edges per node (above cap)
	excessDeleted, err := s.learner.PruneExcessEdgesPerNode(ctx, spaceID)
	if err != nil {
		log.Printf("ERROR: PruneExcessEdgesPerNode failed for space %s: %v", spaceID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	totalDeleted := decayedDeleted + excessDeleted
	log.Printf("Learning edge pruning for %s: decayed=%d, excess=%d, total=%d", spaceID, decayedDeleted, excessDeleted, totalDeleted)

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id":        spaceID,
		"decayed_deleted": decayedDeleted,
		"excess_deleted":  excessDeleted,
		"total_deleted":   totalDeleted,
	})
}

// handleConsult handles POST /v1/memory/consult
// The Agent Consulting Service acts as an SME for coding agents.
func (s *Server) handleConsult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.ConsultRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	resp, err := s.consultant.Consult(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleSuggest handles POST /v1/memory/suggest
// Context-triggered suggestions - proactively surfaces relevant information
// without requiring an explicit question. This is MDEMG's "Active Participation" mode.
func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.SuggestRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	resp, err := s.consultant.Suggest(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleLearningStats handles GET /v1/learning/stats
// Returns statistics about learning edges for a space.
func (s *Server) handleLearningStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	ctx := r.Context()

	stats, err := s.learner.GetLearningEdgeStats(ctx, spaceID)
	if err != nil {
		log.Printf("ERROR: GetLearningEdgeStats failed for space %s: %v", spaceID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	stats["space_id"] = spaceID
	stats["decay_per_day"] = s.cfg.LearningDecayPerDay
	stats["prune_threshold"] = s.cfg.LearningPruneThreshold
	stats["max_edges_per_node"] = s.cfg.LearningMaxEdgesPerNode

	writeJSON(w, http.StatusOK, stats)
}
