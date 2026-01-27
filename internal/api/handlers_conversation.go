package api

import (
	"net/http"

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
		SpaceID:   req.SpaceID,
		SessionID: req.SessionID,
		Content:   req.Content,
		ObsType:   req.ObsType,
		Tags:      req.Tags,
		Metadata:  req.Metadata,
	}

	resp, err := s.conversationSvc.Observe(r.Context(), internalReq)
	if err != nil {
		writeInternalError(w, err, "observe")
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
		SpaceID:   req.SpaceID,
		SessionID: req.SessionID,
		Incorrect: req.Incorrect,
		Correct:   req.Correct,
		Context:   req.Context,
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
	}

	resp, err := s.conversationSvc.Resume(r.Context(), internalReq)
	if err != nil {
		writeInternalError(w, err, "resume")
		return
	}

	// Convert to API response type
	apiResp := models.ResumeResponse{
		SpaceID:          resp.SpaceID,
		SessionID:        resp.SessionID,
		Observations:     convertObservations(resp.Observations),
		Themes:           convertThemes(resp.Themes),
		EmergentConcepts: convertConcepts(resp.EmergentConcepts),
		Summary:          resp.Summary,
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
		SpaceID:         req.SpaceID,
		Query:           req.Query,
		QueryEmbedding:  req.QueryEmbedding,
		TopK:            req.TopK,
		IncludeThemes:   req.IncludeThemes,
		IncludeConcepts: req.IncludeConcepts,
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
