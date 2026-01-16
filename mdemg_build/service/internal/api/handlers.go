package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/anomaly"
	"mdemg/internal/db"
	"mdemg/internal/models"
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
