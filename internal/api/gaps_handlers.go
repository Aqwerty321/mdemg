package api

import (
	"net/http"
	"strings"

	"mdemg/internal/gaps"
)

// handleCapabilityGaps handles GET /v1/system/capability-gaps
// Returns all capability gaps with optional filtering
func (s *Server) handleCapabilityGaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	status := r.URL.Query().Get("status")  // "open", "addressed", "dismissed"
	gapType := r.URL.Query().Get("type")   // "data_source", "reasoning", "query_pattern"
	spaceID := r.URL.Query().Get("space_id")

	ctx := r.Context()

	// Get gaps from store
	gapsList, err := s.gapDetector.GetStore().ListGaps(ctx, gaps.GapStatus(status), gapType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// Filter by space_id if provided
	if spaceID != "" {
		var filtered []gaps.CapabilityGap
		for _, gap := range gapsList {
			if gap.SpaceID == spaceID || gap.SpaceID == "" {
				filtered = append(filtered, gap)
			}
		}
		gapsList = filtered
	}

	// Sort by priority
	gaps.SortGapsByPriority(gapsList)

	// Build summary
	summary, err := s.gapDetector.GetGapsSummary(ctx)
	if err != nil {
		summary = &gaps.GapsSummary{
			Total:  len(gapsList),
			ByType: make(map[string]int),
		}
	}

	// Convert to response format
	gapsResponse := make([]map[string]any, 0, len(gapsList))
	for _, gap := range gapsList {
		gapsResponse = append(gapsResponse, map[string]any{
			"id":          gap.ID,
			"type":        gap.Type,
			"description": gap.Description,
			"evidence":    gap.Evidence,
			"suggested_plugin": map[string]any{
				"type":         gap.SuggestedPlugin.Type,
				"name":         gap.SuggestedPlugin.Name,
				"description":  gap.SuggestedPlugin.Description,
				"capabilities": gap.SuggestedPlugin.Capabilities,
			},
			"priority":         gap.Priority,
			"detected_at":      gap.DetectedAt,
			"updated_at":       gap.UpdatedAt,
			"status":           gap.Status,
			"occurrence_count": gap.OccurrenceCount,
			"space_id":         gap.SpaceID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"gaps": gapsResponse,
			"summary": map[string]any{
				"total":         summary.Total,
				"by_type":       summary.ByType,
				"high_priority": summary.HighPriority,
			},
		},
	})
}

// handleCapabilityGapDismiss handles POST /v1/system/capability-gaps/{id}/dismiss
func (s *Server) handleCapabilityGapDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract gap ID from path
	gapID := extractGapID(r.URL.Path, "/dismiss")
	if gapID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid gap_id in path"})
		return
	}

	ctx := r.Context()

	// Check if gap exists
	gap, err := s.gapDetector.GetStore().GetGap(ctx, gapID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if gap == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "gap not found"})
		return
	}

	// Update status
	if err := s.gapDetector.GetStore().UpdateGapStatus(ctx, gapID, gaps.GapStatusDismissed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":     gapID,
		"status": "dismissed",
	})
}

// handleCapabilityGapAddress handles POST /v1/system/capability-gaps/{id}/address
func (s *Server) handleCapabilityGapAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract gap ID from path
	gapID := extractGapID(r.URL.Path, "/address")
	if gapID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid gap_id in path"})
		return
	}

	ctx := r.Context()

	// Check if gap exists
	gap, err := s.gapDetector.GetStore().GetGap(ctx, gapID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if gap == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "gap not found"})
		return
	}

	// Update status
	if err := s.gapDetector.GetStore().UpdateGapStatus(ctx, gapID, gaps.GapStatusAddressed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":     gapID,
		"status": "addressed",
	})
}

// handleFeedback handles POST /v1/feedback
// Accepts user feedback on retrieval results for gap detection
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req gaps.Feedback
	if !readJSON(w, r, &req) {
		return
	}

	// Validate required fields
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}
	if req.QueryText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "query_text is required"})
		return
	}

	ctx := r.Context()

	if err := s.gapDetector.ProcessFeedback(ctx, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "recorded",
		"message": "Feedback recorded for capability gap analysis",
	})
}

// handleGapAnalyze handles POST /v1/system/capability-gaps/analyze
// Triggers a full gap analysis
func (s *Server) handleGapAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	ctx := r.Context()

	gapsList, err := s.gapDetector.RunFullAnalysis(ctx, spaceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// Sort by priority
	gaps.SortGapsByPriority(gapsList)

	// Get metrics
	metrics := s.gapDetector.GetMetrics()

	writeJSON(w, http.StatusOK, map[string]any{
		"gaps_found": len(gapsList),
		"metrics":    metrics,
	})
}

// handleGapMetrics handles GET /v1/system/capability-gaps/metrics
// Returns gap detection metrics
func (s *Server) handleGapMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get in-memory metrics
	metrics := s.gapDetector.GetMetrics()

	// Get stored stats
	stats, err := s.gapDetector.GetStore().GetGapStats(ctx)
	if err != nil {
		stats = make(map[string]any)
	}

	// Merge
	for k, v := range stats {
		metrics[k] = v
	}

	writeJSON(w, http.StatusOK, metrics)
}

// handleCapabilityGapOperation routes requests to the appropriate handler
func (s *Server) handleCapabilityGapOperation(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/dismiss"):
		s.handleCapabilityGapDismiss(w, r)
	case strings.HasSuffix(path, "/address"):
		s.handleCapabilityGapAddress(w, r)
	case strings.HasSuffix(path, "/analyze"):
		s.handleGapAnalyze(w, r)
	case strings.HasSuffix(path, "/metrics"):
		s.handleGapMetrics(w, r)
	default:
		// Single gap by ID
		s.handleCapabilityGapGet(w, r)
	}
}

// handleCapabilityGapGet handles GET /v1/system/capability-gaps/{id}
func (s *Server) handleCapabilityGapGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract gap ID from path
	path := strings.TrimPrefix(r.URL.Path, "/v1/system/capability-gaps/")
	gapID := path
	if gapID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid gap_id in path"})
		return
	}

	ctx := r.Context()

	gap, err := s.gapDetector.GetStore().GetGap(ctx, gapID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if gap == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "gap not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          gap.ID,
		"type":        gap.Type,
		"description": gap.Description,
		"evidence":    gap.Evidence,
		"suggested_plugin": map[string]any{
			"type":         gap.SuggestedPlugin.Type,
			"name":         gap.SuggestedPlugin.Name,
			"description":  gap.SuggestedPlugin.Description,
			"capabilities": gap.SuggestedPlugin.Capabilities,
		},
		"priority":         gap.Priority,
		"detected_at":      gap.DetectedAt,
		"updated_at":       gap.UpdatedAt,
		"status":           gap.Status,
		"occurrence_count": gap.OccurrenceCount,
		"space_id":         gap.SpaceID,
	})
}

// Helper function to extract gap ID from path
func extractGapID(path, suffix string) string {
	// Path format: /v1/system/capability-gaps/{id}/suffix
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimPrefix(path, "/v1/system/capability-gaps/")
	return path
}
