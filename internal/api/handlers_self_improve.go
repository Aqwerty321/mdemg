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
		SpaceID        string `json:"space_id"`
		Tier           string `json:"tier"`
		TriggerSource  string `json:"trigger_source,omitempty"`
		IdempotencyKey string `json:"idempotency_key,omitempty"`
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

	// Resolve trigger source (default: manual_api)
	triggerSource := ape.TriggerManualAPI
	if req.TriggerSource != "" {
		ts, ok := ape.ValidTriggerSources[req.TriggerSource]
		if !ok {
			http.Error(w, "invalid trigger_source: "+req.TriggerSource, http.StatusBadRequest)
			return
		}
		triggerSource = ts
	}

	// Phase 87: Orchestration policy evaluation
	if s.orchestrationPolicy != nil {
		// Fast-path dedupe check
		if dedupeResult := s.orchestrationPolicy.CheckDedupe(req.IdempotencyKey); dedupeResult != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"cycle_id":       dedupeResult.OriginalCycleID,
				"dedupe":         dedupeResult,
				"policy_version": ape.PolicyVersion,
			})
			return
		}

		decision := s.orchestrationPolicy.EvaluateTrigger(triggerSource, req.SpaceID, tier, req.IdempotencyKey)
		if !decision.Allowed {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":          "trigger rejected",
				"reason":         decision.Reason,
				"policy_version": ape.PolicyVersion,
			})
			return
		}

		opts := &ape.RunCycleOpts{
			TriggerMeta:    &decision.Meta,
			IdempotencyKey: req.IdempotencyKey,
		}

		outcome, err := s.rsicCycle.RunCycle(r.Context(), req.SpaceID, tier, opts)
		if err != nil {
			s.orchestrationPolicy.CompleteCycle(req.SpaceID, tier)
			http.Error(w, sanitizeError(err, "self-improve cycle"), http.StatusInternalServerError)
			return
		}

		s.orchestrationPolicy.RecordTrigger(decision.Meta, req.SpaceID, tier, outcome.CycleID)
		s.orchestrationPolicy.CompleteCycle(req.SpaceID, tier)

		// Phase 80: Record RSIC cycle for signal tracking
		if s.sessionTracker != nil {
			s.sessionTracker.RecordRSICCall("claude-core")
		}
		if s.signalLearner != nil {
			s.signalLearner.RecordResponse("rsic-cycle-called")
		}

		writeJSON(w, http.StatusOK, outcome)
		return
	}

	// Fallback path when orchestration policy is nil (backward compat)
	outcome, err := s.rsicCycle.RunCycle(r.Context(), req.SpaceID, tier, nil)
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

	// Phase 87: Optional filters
	qTriggerSource := r.URL.Query().Get("trigger_source")
	qTier := r.URL.Query().Get("tier")
	qSpaceID := r.URL.Query().Get("space_id")

	var history []ape.CycleOutcome
	hasFilter := qTriggerSource != "" || qTier != "" || qSpaceID != ""

	if hasFilter {
		filter := &ape.HistoryFilter{
			TriggerSource: ape.TriggerSource(qTriggerSource),
			Tier:          ape.CycleTier(qTier),
			SpaceID:       qSpaceID,
		}
		history = s.rsicCycle.GetHistoryFiltered(limit, filter)
	} else {
		history = s.rsicCycle.GetHistory(limit)
	}

	if history == nil {
		history = []ape.CycleOutcome{}
	}

	resp := map[string]any{
		"history": history,
		"count":   len(history),
	}

	if hasFilter {
		filters := map[string]string{}
		if qTriggerSource != "" {
			filters["trigger_source"] = qTriggerSource
		}
		if qTier != "" {
			filters["tier"] = qTier
		}
		if qSpaceID != "" {
			filters["space_id"] = qSpaceID
		}
		resp["filters"] = filters
	}

	writeJSON(w, http.StatusOK, resp)
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

	// Phase 87: Orchestration status block
	if s.orchestrationPolicy != nil {
		resp["orchestration"] = s.orchestrationPolicy.GetOrchestrationStatus(s.macroNextRun)
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

