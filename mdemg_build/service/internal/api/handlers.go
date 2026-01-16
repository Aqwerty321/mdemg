package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

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

	writeJSON(w, http.StatusOK, out)
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
	_ = spaceParam // Will be used in subsequent subtasks for Neo4j queries

	// TODO: Neo4j queries will be added in subtasks 3-2, 3-3, 3-4

	return resp, nil
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
