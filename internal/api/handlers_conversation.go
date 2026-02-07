package api

import (
	"net/http"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/conversation"
	"mdemg/internal/models"
)

// =============================================================================
// PHASE 5: RESUME AND RECALL ENDPOINTS
// =============================================================================

// handleObserve captures a conversation observation with surprise detection
// POST /v1/conversation/observe
func (s *Server) handleObserve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Check if conversation service is available
	if s.conversationSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "conversation service not available (embedder required)",
		})
		return
	}

	var req models.ObserveRequest
	if !readJSON(w, r, &req) {
		return
	}

	if !validateRequest(w, &req) {
		return
	}

	// Convert to internal type
	internalReq := conversation.ObserveRequest{
		SpaceID:    req.SpaceID,
		SessionID:  req.SessionID,
		Content:    req.Content,
		ObsType:    req.ObsType,
		Tags:       req.Tags,
		Metadata:   req.Metadata,
		UserID:     req.UserID,
		Visibility: req.Visibility,
		AgentID:    req.AgentID,
		RefersTo:   req.RefersTo,
		Pinned:     req.Pinned,
	}

	resp, err := s.conversationSvc.Observe(r.Context(), internalReq)
	if err != nil {
		writeInternalError(w, err, "observe")
		return
	}

	// Track observation in session tracker (Phase 3A)
	if s.sessionTracker != nil && req.SessionID != "" {
		s.sessionTracker.RecordObserve(req.SessionID)
	}

	// Convert to API response type
	apiResp := models.ObserveResponse{
		ObsID:           resp.ObsID,
		NodeID:          resp.NodeID,
		SurpriseScore:   resp.SurpriseScore,
		SurpriseFactors: resp.SurpriseFactors,
		Summary:         resp.Summary,
	}

	// Include detected constraints if any (Phase 45.5)
	if len(resp.DetectedConstraints) > 0 {
		apiResp.DetectedConstraints = make([]models.DetectedConstraintInfo, len(resp.DetectedConstraints))
		for i, dc := range resp.DetectedConstraints {
			apiResp.DetectedConstraints[i] = models.DetectedConstraintInfo{
				ConstraintType: dc.ConstraintType,
				Name:           dc.Name,
				Confidence:     dc.Confidence,
			}
		}
	}

	writeJSON(w, http.StatusOK, apiResp)
}

// handleCorrect captures an explicit user correction
// POST /v1/conversation/correct
func (s *Server) handleCorrect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Check if conversation service is available
	if s.conversationSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "conversation service not available (embedder required)",
		})
		return
	}

	var req models.CorrectRequest
	if !readJSON(w, r, &req) {
		return
	}

	if !validateRequest(w, &req) {
		return
	}

	// Convert to internal type
	internalReq := conversation.CorrectRequest{
		SpaceID:    req.SpaceID,
		SessionID:  req.SessionID,
		Incorrect:  req.Incorrect,
		Correct:    req.Correct,
		Context:    req.Context,
		UserID:     req.UserID,
		Visibility: req.Visibility,
		AgentID:    req.AgentID,
		RefersTo:   req.RefersTo,
	}

	resp, err := s.conversationSvc.Correct(r.Context(), internalReq)
	if err != nil {
		writeInternalError(w, err, "correct")
		return
	}

	// Convert to API response type
	apiResp := models.ObserveResponse{
		ObsID:           resp.ObsID,
		NodeID:          resp.NodeID,
		SurpriseScore:   resp.SurpriseScore,
		SurpriseFactors: resp.SurpriseFactors,
		Summary:         resp.Summary,
	}

	writeJSON(w, http.StatusOK, apiResp)
}

// handleResume restores context after context compaction
// POST /v1/conversation/resume
func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Check if conversation service is available
	if s.conversationSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "conversation service not available (embedder required)",
		})
		return
	}

	var req models.ResumeRequest
	if !readJSON(w, r, &req) {
		return
	}

	if !validateRequest(w, &req) {
		return
	}

	// Convert to internal type
	internalReq := conversation.ResumeRequest{
		SpaceID:          req.SpaceID,
		SessionID:        req.SessionID,
		IncludeTasks:     req.IncludeTasks,
		IncludeDecisions: req.IncludeDecisions,
		IncludeLearnings: req.IncludeLearnings,
		MaxObservations:  req.MaxObservations,
		RequestingUserID: req.RequestingUserID,
		AgentID:          req.AgentID,
	}

	resp, err := s.conversationSvc.Resume(r.Context(), internalReq)
	if err != nil {
		writeInternalError(w, err, "resume")
		return
	}

	// Track resume in session tracker (Phase 3A)
	if s.sessionTracker != nil {
		sessionID := req.SessionID
		if sessionID == "" {
			sessionID = req.SpaceID // Fall back to space ID if no session
		}
		s.sessionTracker.RecordResume(sessionID, req.SpaceID)
	}

	// Convert to API response type
	apiResp := models.ResumeResponse{
		SpaceID:          resp.SpaceID,
		SessionID:        resp.SessionID,
		Observations:     convertObservations(resp.Observations),
		Themes:           convertThemes(resp.Themes),
		EmergentConcepts: convertConcepts(resp.EmergentConcepts),
		Summary:          resp.Summary,
		Jiminy:           convertJiminy(resp.Jiminy),
		Debug:            resp.Debug,
	}

	writeJSON(w, http.StatusOK, apiResp)
}

// handleRecall retrieves relevant conversation knowledge via semantic query
// POST /v1/conversation/recall
func (s *Server) handleRecall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Check if conversation service is available
	if s.conversationSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "conversation service not available (embedder required)",
		})
		return
	}

	var req models.RecallRequest
	if !readJSON(w, r, &req) {
		return
	}

	if !validateRequest(w, &req) {
		return
	}

	// Convert to internal type
	internalReq := conversation.RecallRequest{
		SpaceID:          req.SpaceID,
		Query:            req.Query,
		QueryEmbedding:   req.QueryEmbedding,
		TopK:             req.TopK,
		IncludeThemes:    req.IncludeThemes,
		IncludeConcepts:  req.IncludeConcepts,
		RequestingUserID: req.RequestingUserID,
		AgentID:          req.AgentID,
		TemporalAfter:    req.TemporalAfter,
		TemporalBefore:   req.TemporalBefore,
		FilterTags:       req.FilterTags,
	}

	resp, err := s.conversationSvc.Recall(r.Context(), internalReq)
	if err != nil {
		writeInternalError(w, err, "recall")
		return
	}

	// Convert to API response type
	apiResp := models.RecallResponse{
		SpaceID: resp.SpaceID,
		Query:   resp.Query,
		Results: convertRecallResults(resp.Results),
		Debug:   resp.Debug,
	}

	writeJSON(w, http.StatusOK, apiResp)
}

// Conversion helpers for Phase 5 responses

func convertObservations(obs []conversation.ObservationResult) []models.ConversationObsResult {
	result := make([]models.ConversationObsResult, len(obs))
	for i, o := range obs {
		result[i] = models.ConversationObsResult{
			NodeID:        o.NodeID,
			ObsType:       o.ObsType,
			Content:       o.Content,
			Summary:       o.Summary,
			SessionID:     o.SessionID,
			SurpriseScore: o.SurpriseScore,
			Score:         o.Score,
			Tags:          o.Tags,
			CreatedAt:     o.CreatedAt,
		}
	}
	return result
}

func convertThemes(themes []conversation.ThemeResult) []models.ConversationThemeResult {
	result := make([]models.ConversationThemeResult, len(themes))
	for i, t := range themes {
		result[i] = models.ConversationThemeResult{
			NodeID:           t.NodeID,
			Name:             t.Name,
			Summary:          t.Summary,
			MemberCount:      t.MemberCount,
			DominantObsType:  t.DominantObsType,
			AvgSurpriseScore: t.AvgSurpriseScore,
			Score:            t.Score,
		}
	}
	return result
}

func convertConcepts(concepts []conversation.EmergentConceptResult) []models.EmergentConceptResult {
	result := make([]models.EmergentConceptResult, len(concepts))
	for i, c := range concepts {
		result[i] = models.EmergentConceptResult{
			NodeID:       c.NodeID,
			Name:         c.Name,
			Summary:      c.Summary,
			Layer:        c.Layer,
			Keywords:     c.Keywords,
			SessionCount: c.SessionCount,
			Score:        c.Score,
		}
	}
	return result
}

func convertRecallResults(results []conversation.RecallResult) []models.RecallResult {
	apiResults := make([]models.RecallResult, len(results))
	for i, r := range results {
		apiResults[i] = models.RecallResult{
			Type:     r.Type,
			NodeID:   r.NodeID,
			Content:  r.Content,
			Score:    r.Score,
			Layer:    r.Layer,
			Metadata: r.Metadata,
		}
	}
	return apiResults
}

func convertJiminy(j *conversation.JiminyRationale) *models.JiminyRationale {
	if j == nil {
		return nil
	}
	return &models.JiminyRationale{
		Rationale:      j.Rationale,
		Confidence:     j.Confidence,
		ScoreBreakdown: j.ScoreBreakdown,
		Highlights:     j.Highlights,
	}
}

// handleConversationConsolidate runs conversation-specific consolidation
// POST /v1/conversation/consolidate
func (s *Server) handleConversationConsolidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.hiddenSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "hidden layer service not available",
		})
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}

	result, err := s.hiddenSvc.RunFullConversationConsolidation(r.Context(), req.SpaceID)
	if err != nil {
		writeInternalError(w, err, "conversation consolidation")
		return
	}

	themesCreated := 0
	conceptsCreated := 0
	if result.ThemeResult != nil {
		themesCreated = result.ThemeResult.ThemesCreated
	}
	if result.ConceptResult != nil {
		for _, count := range result.ConceptResult.ConceptsCreated {
			conceptsCreated += count
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id":         req.SpaceID,
		"themes_created":   themesCreated,
		"concepts_created": conceptsCreated,
		"duration_ms":      result.TotalDuration.Milliseconds(),
	})
}

// handleVolatileStats returns statistics about volatile conversation observations
// GET /v1/conversation/volatile/stats
func (s *Server) handleVolatileStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.contextCooler == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "context cooler not available",
		})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	stats, err := s.contextCooler.GetVolatileStats(r.Context(), spaceID)
	if err != nil {
		writeInternalError(w, err, "volatile stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleProcessGraduations manually triggers graduation processing
// POST /v1/conversation/graduate
func (s *Server) handleProcessGraduations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.contextCooler == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "context cooler not available",
		})
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}

	// Apply decay first
	decayed, err := s.contextCooler.ApplyDecay(r.Context(), req.SpaceID)
	if err != nil {
		writeInternalError(w, err, "apply decay")
		return
	}

	// Then process graduations
	summary, err := s.contextCooler.ProcessGraduations(r.Context(), req.SpaceID)
	if err != nil {
		writeInternalError(w, err, "process graduations")
		return
	}

	summary.DecayApplied = decayed
	writeJSON(w, http.StatusOK, summary)
}

// handleSessionHealth returns the CMS usage health for a session.
// GET /v1/conversation/session/health?session_id=X
func (s *Server) handleSessionHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session_id query parameter is required"})
		return
	}

	if s.sessionTracker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "session tracker not available"})
		return
	}

	state := s.sessionTracker.GetState(sessionID)
	if state == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id":               sessionID,
			"resumed":                  false,
			"observations_since_resume": 0,
			"health_score":             0.0,
			"tracked":                  false,
		})
		return
	}

	resp := map[string]any{
		"session_id":               state.SessionID,
		"space_id":                 state.SpaceID,
		"resumed":                  state.Resumed,
		"observations_since_resume": state.ObservationsSinceResume,
		"health_score":             state.HealthScore(),
		"tracked":                  true,
	}

	if !state.LastResumeAt.IsZero() {
		resp["last_resume_at"] = state.LastResumeAt.Format("2006-01-02T15:04:05Z")
	}
	if !state.LastObserveAt.IsZero() {
		resp["last_observe_at"] = state.LastObserveAt.Format("2006-01-02T15:04:05Z")
	}
	if !state.LastActivityAt.IsZero() {
		resp["last_activity_at"] = state.LastActivityAt.Format("2006-01-02T15:04:05Z")
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleConstraintsList lists constraint nodes for a space
// GET /v1/constraints?space_id=...
func (s *Server) handleConstraintsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	sess := s.driver.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(r.Context())

	result, err := sess.ExecuteRead(r.Context(), func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'constraint'})
			WHERE NOT coalesce(c.is_archived, false)
			OPTIONAL MATCH (c)<-[:IMPLEMENTS_CONSTRAINT]-(obs)
			RETURN c.node_id AS node_id,
			       c.name AS name,
			       c.constraint_type AS constraint_type,
			       c.content AS content,
			       c.confidence AS confidence,
			       c.created_at AS created_at,
			       c.updated_at AS updated_at,
			       count(obs) AS source_count
			ORDER BY c.confidence DESC
		`
		res, err := tx.Run(r.Context(), cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var constraints []map[string]any
		for res.Next(r.Context()) {
			rec := res.Record()
			entry := make(map[string]any)
			for _, key := range rec.Keys {
				val, _ := rec.Get(key)
				entry[key] = val
			}
			constraints = append(constraints, entry)
		}
		if constraints == nil {
			constraints = []map[string]any{}
		}
		return constraints, res.Err()
	})

	if err != nil {
		writeInternalError(w, err, "list constraints")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id":    spaceID,
		"constraints": result,
	})
}

// handleConstraintStats returns summary statistics about constraint nodes
// GET /v1/constraints/stats?space_id=...
func (s *Server) handleConstraintStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	sess := s.driver.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(r.Context())

	result, err := sess.ExecuteRead(r.Context(), func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'constraint'})
			WHERE NOT coalesce(c.is_archived, false)
			RETURN c.constraint_type AS constraint_type,
			       count(c) AS count,
			       avg(c.confidence) AS avg_confidence
			ORDER BY count DESC
		`
		res, err := tx.Run(r.Context(), cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var byType []map[string]any
		total := 0
		for res.Next(r.Context()) {
			rec := res.Record()
			entry := make(map[string]any)
			for _, key := range rec.Keys {
				val, _ := rec.Get(key)
				entry[key] = val
			}
			byType = append(byType, entry)
			if cnt, ok := entry["count"]; ok {
				if n, ok := cnt.(int64); ok {
					total += int(n)
				}
			}
		}
		if byType == nil {
			byType = []map[string]any{}
		}

		// Count constraint-tagged observations
		obsCypher := `
			MATCH (obs:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
			WHERE any(tag IN coalesce(obs.tags, []) WHERE tag STARTS WITH 'constraint:')
			RETURN count(obs) AS tagged_observation_count
		`
		obsRes, err := tx.Run(r.Context(), obsCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		taggedCount := 0
		if obsRes.Next(r.Context()) {
			if v, ok := obsRes.Record().Get("tagged_observation_count"); ok && v != nil {
				taggedCount = int(v.(int64))
			}
		}

		return map[string]any{
			"space_id":                 spaceID,
			"total_constraint_nodes":   total,
			"by_type":                  byType,
			"tagged_observation_count": taggedCount,
		}, res.Err()
	})

	if err != nil {
		writeInternalError(w, err, "constraint stats")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
