package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mdemg/internal/ape"
)

// ─── POST /v1/self-improve/assess ───

func (s *Server) handleSelfImproveAssess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
		Tier    string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}
	tier := ape.CycleTier(req.Tier)
	if tier == "" {
		tier = ape.TierMeso
	}

	report, err := s.rsicCycle.Assess(r.Context(), req.SpaceID, tier)
	if err != nil {
		http.Error(w, sanitizeError(err, "self-improve assess"), http.StatusInternalServerError)
		return
	}

	// Phase 80: Record RSIC call for signal tracking
	if s.sessionTracker != nil {
		s.sessionTracker.RecordRSICCall("claude-core")
	}
	if s.signalLearner != nil {
		s.signalLearner.RecordResponse("rsic-assess-called")
	}

	writeJSON(w, http.StatusOK, report)
}

// ─── GET /v1/self-improve/report ───

func (s *Server) handleSelfImproveReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	tasks := s.rsicCycle.GetActiveTasks()
	writeJSON(w, http.StatusOK, map[string]any{
		"active_tasks": tasks,
	})
}

// ─── GET /v1/self-improve/report/{taskID} ───

func (s *Server) handleSelfImproveReportByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	taskID := strings.TrimPrefix(r.URL.Path, "/v1/self-improve/report/")
	taskID = strings.TrimSuffix(taskID, "/")
	if taskID == "" {
		http.Error(w, "task_id is required in path", http.StatusBadRequest)
		return
	}

	reports := s.rsicCycle.GetTaskReports(taskID)
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id": taskID,
		"reports": reports,
	})
}

// ─── POST /v1/self-improve/cycle ───

func (s *Server) handleSelfImproveCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
		Tier    string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}
	tier := ape.CycleTier(req.Tier)
	if tier == "" {
		tier = ape.TierMeso
	}

	outcome, err := s.rsicCycle.RunCycle(r.Context(), req.SpaceID, tier)
	if err != nil {
		http.Error(w, sanitizeError(err, "self-improve cycle"), http.StatusInternalServerError)
		return
	}

	// Phase 80: Record RSIC cycle for signal tracking
	if s.sessionTracker != nil {
		s.sessionTracker.RecordRSICCall("claude-core")
	}
	if s.signalLearner != nil {
		s.signalLearner.RecordResponse("rsic-cycle-called")
	}

	writeJSON(w, http.StatusOK, outcome)
}

// ─── GET /v1/self-improve/history ───

func (s *Server) handleSelfImproveHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	history := s.rsicCycle.GetHistory(limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"history": history,
		"count":   len(history),
	})
}

// ─── GET /v1/self-improve/calibration ───

func (s *Server) handleSelfImproveCalibration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	calibration := s.rsicCycle.GetCalibration()
	writeJSON(w, http.StatusOK, map[string]any{
		"calibration": calibration,
	})
}

// ─── GET /v1/self-improve/health ───

func (s *Server) handleSelfImproveHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	watchdogState := s.rsicCycle.GetWatchdogState()
	activeTasks := s.rsicCycle.GetActiveTasks()

	resp := map[string]any{
		"status":       "ok",
		"active_tasks": len(activeTasks),
	}
	if watchdogState != nil {
		resp["watchdog"] = watchdogState
	}

	writeJSON(w, http.StatusOK, resp)
}

// ─── GET /v1/self-improve/signals ───

func (s *Server) handleSelfImproveSignals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.signalLearner == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"signals": []any{},
			"enabled": false,
		})
		return
	}

	signals := s.signalLearner.GetAllEffectiveness()
	writeJSON(w, http.StatusOK, map[string]any{
		"signals": signals,
		"enabled": true,
		"count":   len(signals),
	})
}

