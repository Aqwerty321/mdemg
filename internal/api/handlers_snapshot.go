package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mdemg/internal/conversation"
)

// handleSnapshots handles /v1/conversation/snapshot
func (s *Server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListSnapshots(w, r)
	case http.MethodPost:
		s.handleCreateSnapshot(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSnapshotByID handles /v1/conversation/snapshot/{id}
func (s *Server) handleSnapshotByID(w http.ResponseWriter, r *http.Request) {
	// Extract snapshot ID from path
	path := strings.TrimPrefix(r.URL.Path, "/v1/conversation/snapshot/")
	snapshotID := strings.TrimSuffix(path, "/")

	if snapshotID == "" {
		http.Error(w, "snapshot_id is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetSnapshot(w, r, snapshotID)
	case http.MethodDelete:
		s.handleDeleteSnapshot(w, r, snapshotID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListSnapshots lists snapshots for a space
func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		http.Error(w, "space_id query parameter is required", http.StatusBadRequest)
		return
	}

	sessionID := r.URL.Query().Get("session_id") // Optional filter

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	snapshots, err := s.snapshotService.ListSnapshots(r.Context(), spaceID, sessionID, limit)
	if err != nil {
		http.Error(w, "Failed to list snapshots: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := conversation.ListSnapshotsResponse{
		Snapshots: make([]conversation.SnapshotResponse, 0, len(snapshots)),
		Count:     len(snapshots),
	}

	for _, snap := range snapshots {
		response.Snapshots = append(response.Snapshots, snap.ToResponse())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateSnapshot creates a new snapshot
func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req conversation.CreateSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	trigger := conversation.TriggerManual
	if req.Trigger != "" {
		switch req.Trigger {
		case "manual":
			trigger = conversation.TriggerManual
		case "compaction":
			trigger = conversation.TriggerCompaction
		case "session_end":
			trigger = conversation.TriggerSessionEnd
		case "error":
			trigger = conversation.TriggerError
		default:
			http.Error(w, "Invalid trigger: must be manual, compaction, session_end, or error", http.StatusBadRequest)
			return
		}
	}

	snapshot := &conversation.TaskSnapshot{
		SpaceID:   req.SpaceID,
		SessionID: req.SessionID,
		Trigger:   trigger,
		Context:   req.Context,
	}

	if err := s.snapshotService.CreateSnapshot(r.Context(), snapshot); err != nil {
		http.Error(w, "Failed to create snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(snapshot.ToResponse())
}

// handleGetSnapshot retrieves a snapshot by ID
func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request, snapshotID string) {
	snapshot, err := s.snapshotService.GetSnapshot(r.Context(), snapshotID)
	if err != nil {
		http.Error(w, "Failed to get snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if snapshot == nil {
		http.Error(w, "Snapshot not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot.ToResponse())
}

// handleDeleteSnapshot deletes a snapshot
func (s *Server) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request, snapshotID string) {
	if err := s.snapshotService.DeleteSnapshot(r.Context(), snapshotID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Snapshot not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to delete snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleLatestSnapshot handles GET /v1/conversation/snapshot/latest
func (s *Server) handleLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		http.Error(w, "space_id query parameter is required", http.StatusBadRequest)
		return
	}

	sessionID := r.URL.Query().Get("session_id") // Optional

	snapshot, err := s.snapshotService.GetLatestSnapshot(r.Context(), spaceID, sessionID)
	if err != nil {
		http.Error(w, "Failed to get latest snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if snapshot == nil {
		http.Error(w, "No snapshots found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot.ToResponse())
}

// handleCleanupSnapshots handles POST /v1/conversation/snapshot/cleanup
func (s *Server) handleCleanupSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SpaceID       string `json:"space_id"`
		RetentionDays int    `json:"retention_days,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}

	deleted, err := s.snapshotService.CleanupOldSnapshots(r.Context(), req.SpaceID, req.RetentionDays)
	if err != nil {
		http.Error(w, "Failed to cleanup snapshots: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deleted":        deleted,
		"retention_days": req.RetentionDays,
	})
}
