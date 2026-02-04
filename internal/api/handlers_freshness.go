package api

import (
	"math"
	"net/http"
	"strings"
	"time"

	"mdemg/internal/models"
)

// handleSpaceFreshness handles GET /v1/memory/spaces/{space_id}/freshness
// Returns freshness/staleness information for a space's TapRoot node.
func (s *Server) handleSpaceFreshness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract space_id from URL path: /v1/memory/spaces/{space_id}/freshness
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/spaces/")
	spaceID := strings.TrimSuffix(path, "/freshness")
	spaceID = strings.TrimSuffix(spaceID, "/")

	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required in path"})
		return
	}

	// Query TapRoot for freshness properties
	props, err := s.retriever.GetTapRootFreshness(r.Context(), spaceID)
	if err != nil {
		writeInternalError(w, err, "get freshness")
		return
	}

	thresholdHours := s.cfg.SyncStaleThresholdHours

	if props == nil {
		// No TapRoot found - space has never been ingested
		writeJSON(w, http.StatusOK, models.FreshnessResponse{
			SpaceID:        spaceID,
			IngestCount:    0,
			IsStale:        true,
			ThresholdHours: thresholdHours,
		})
		return
	}

	// Build response from TapRoot properties
	resp := models.FreshnessResponse{
		SpaceID:        spaceID,
		ThresholdHours: thresholdHours,
	}

	// Extract ingest_count
	if count, ok := props["ingest_count"]; ok {
		switch v := count.(type) {
		case int64:
			resp.IngestCount = int(v)
		case float64:
			resp.IngestCount = int(v)
		}
	}

	// Extract last_ingest_type
	if t, ok := props["last_ingest_type"].(string); ok {
		resp.LastIngestType = t
	}

	// Extract last_ingest_at and compute staleness
	if lastIngest, ok := props["last_ingest_at"]; ok {
		var lastTime time.Time
		switch v := lastIngest.(type) {
		case time.Time:
			lastTime = v
		case string:
			if parsed, parseErr := time.Parse(time.RFC3339, v); parseErr == nil {
				lastTime = parsed
			}
		}

		if !lastTime.IsZero() {
			resp.LastIngestAt = lastTime.UTC().Format(time.RFC3339)
			hoursSince := time.Since(lastTime).Hours()
			resp.StaleHours = int(math.Floor(hoursSince))
			resp.IsStale = hoursSince >= float64(thresholdHours)
		} else {
			resp.IsStale = true
		}
	} else {
		resp.IsStale = true
	}

	writeJSON(w, http.StatusOK, resp)
}
