package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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
