package api

import (
	"net/http"
)

// handleAPEStatus handles GET /v1/ape/status - returns APE scheduler status
func (s *Server) handleAPEStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.apeScheduler == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"enabled": false,
				"modules": []any{},
			},
		})
		return
	}

	status := s.apeScheduler.GetStatus()
	writeJSON(w, http.StatusOK, map[string]any{"data": status})
}

// handleAPETrigger handles POST /v1/ape/trigger - manually triggers an APE event
func (s *Server) handleAPETrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.apeScheduler == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "APE scheduler not enabled"})
		return
	}

	var req struct {
		Event string `json:"event"`
	}
	if r.ContentLength > 0 {
		if !readJSON(w, r, &req) {
			return
		}
	}

	if req.Event == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "event is required"})
		return
	}

	s.apeScheduler.TriggerEvent(req.Event)
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"triggered": req.Event,
			"message":   "event triggered, check logs for execution status",
		},
	})
}
