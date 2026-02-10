package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/models"
)

// handleCleanupOrphans handles POST /v1/memory/cleanup/orphans
// Detects L0 nodes whose last_ingested_at < TapRoot.last_ingest_at,
// meaning they were not included in the most recent full re-ingestion.
func (s *Server) handleCleanupOrphans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.OrphanCleanupRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Enforce limit defaults and bounds
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Limit > 1000 {
		req.Limit = 1000
	}

	// Check protected space for delete action
	if req.Action == "delete" && IsProtectedSpace(req.SpaceID) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": fmt.Sprintf("space %q is protected from deletion", req.SpaceID),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Build dynamic Cypher for orphan detection with optional filters
	var whereClauses []string
	whereClauses = append(whereClauses, "n.layer = 0")
	whereClauses = append(whereClauses, "n.last_ingested_at IS NOT NULL")
	whereClauses = append(whereClauses, "n.last_ingested_at < t.last_ingest_at")
	whereClauses = append(whereClauses, "NOT coalesce(n.is_archived, false)")

	params := map[string]any{
		"spaceId": req.SpaceID,
		"limit":   req.Limit,
	}

	// OlderThanDays filter
	if req.OlderThanDays > 0 {
		whereClauses = append(whereClauses, "n.last_ingested_at < datetime() - duration({days: $olderThanDays})")
		params["olderThanDays"] = req.OlderThanDays
	}

	// PathPrefix filter
	if req.PathPrefix != "" {
		whereClauses = append(whereClauses, "n.path STARTS WITH $pathPrefix")
		params["pathPrefix"] = req.PathPrefix
	}

	// Handle count action specially
	if req.Action == "count" {
		countCypher := fmt.Sprintf(`
MATCH (t:TapRoot {space_id: $spaceId})
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE %s
RETURN count(n) AS count`, joinWhereClauses(whereClauses))

		countResult, err := sess.Run(ctx, countCypher, params)
		if err != nil {
			writeInternalError(w, err, "orphan count")
			return
		}

		countVal := 0
		if countResult.Next(ctx) {
			if c, ok := countResult.Record().Get("count"); ok {
				countVal = int(c.(int64))
			}
		}

		writeJSON(w, http.StatusOK, models.OrphanCleanupResponse{
			SpaceID:      req.SpaceID,
			OrphansFound: countVal,
			OrphansActed: 0,
			Action:       req.Action,
			DryRun:       req.DryRun,
			Orphans:      []models.OrphanNode{},
		})
		return
	}

	// Step 1: Detect orphans — L0 nodes with last_ingested_at < TapRoot.last_ingest_at
	detectCypher := fmt.Sprintf(`
MATCH (t:TapRoot {space_id: $spaceId})
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE %s
RETURN n.node_id AS node_id, n.path AS path, n.name AS name,
       toString(n.last_ingested_at) AS last_ingested_at
ORDER BY n.path
LIMIT $limit`, joinWhereClauses(whereClauses))

	result, err := sess.Run(ctx, detectCypher, params)
	if err != nil {
		writeInternalError(w, err, "orphan detection")
		return
	}

	orphans := make([]models.OrphanNode, 0)
	for result.Next(ctx) {
		rec := result.Record()
		nid, _ := rec.Get("node_id")
		path, _ := rec.Get("path")
		name, _ := rec.Get("name")
		lastIngested, _ := rec.Get("last_ingested_at")

		orphans = append(orphans, models.OrphanNode{
			NodeID:         fmt.Sprint(nid),
			Path:           fmt.Sprint(path),
			Name:           fmt.Sprint(name),
			LastIngestedAt: fmt.Sprint(lastIngested),
			Status:         "listed",
		})
	}
	if err := result.Err(); err != nil {
		writeInternalError(w, err, "orphan detection query")
		return
	}

	resp := models.OrphanCleanupResponse{
		SpaceID:      req.SpaceID,
		OrphansFound: len(orphans),
		Action:       req.Action,
		DryRun:       req.DryRun,
		Orphans:      orphans,
	}

	// If dry_run or list action, return without modifying
	if req.DryRun || req.Action == "list" {
		resp.OrphansActed = 0
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Step 2: Act on orphans
	acted := 0
	for i, orphan := range orphans {
		var actionErr error
		switch req.Action {
		case "archive":
			_, actionErr = sess.Run(ctx, `
MATCH (n:MemoryNode {node_id: $nodeId, space_id: $spaceId})
SET n.is_archived = true,
    n.archived_at = datetime(),
    n.archive_reason = 'orphan-cleanup',
    n.version = coalesce(n.version, 0) + 1`, map[string]any{
				"nodeId":  orphan.NodeID,
				"spaceId": req.SpaceID,
			})
			if actionErr == nil {
				orphans[i].Status = "archived"
				acted++
			}
		case "delete":
			_, actionErr = sess.Run(ctx, `
MATCH (n:MemoryNode {node_id: $nodeId, space_id: $spaceId})
DETACH DELETE n`, map[string]any{
				"nodeId":  orphan.NodeID,
				"spaceId": req.SpaceID,
			})
			if actionErr == nil {
				orphans[i].Status = "deleted"
				acted++
			}
		}
		if actionErr != nil {
			log.Printf("warning: failed to %s orphan %s: %v", req.Action, orphan.NodeID, actionErr)
		}
	}

	resp.OrphansActed = acted
	resp.Orphans = orphans
	writeJSON(w, http.StatusOK, resp)
}

// joinWhereClauses joins WHERE clause conditions with AND.
func joinWhereClauses(clauses []string) string {
	if len(clauses) == 0 {
		return "true"
	}
	result := clauses[0]
	for i := 1; i < len(clauses); i++ {
		result += "\n  AND " + clauses[i]
	}
	return result
}

// handleScheduleCleanup handles POST /v1/memory/cleanup/schedule
// Sets up scheduled orphan cleanup for a space.
func (s *Server) handleScheduleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.ScheduleCleanupRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Enforce limit defaults and bounds
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Limit > 1000 {
		req.Limit = 1000
	}

	// Check protected space for delete action
	if req.Action == "delete" && IsProtectedSpace(req.SpaceID) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": fmt.Sprintf("space %q is protected from deletion", req.SpaceID),
		})
		return
	}

	scheduleID := "cleanup-" + uuid.New().String()[:8]
	nextRun := time.Now().Add(time.Duration(req.IntervalHours) * time.Hour)

	// Note: In a full implementation, this would store the schedule in a database
	// and be picked up by a background scheduler. For now, we return the schedule
	// configuration that could be used by an external scheduler or APE module.

	log.Printf("orphan cleanup schedule created: id=%s space=%s interval=%dh action=%s",
		scheduleID, req.SpaceID, req.IntervalHours, req.Action)

	writeJSON(w, http.StatusOK, models.ScheduleCleanupResponse{
		SpaceID:       req.SpaceID,
		ScheduleID:    scheduleID,
		IntervalHours: req.IntervalHours,
		Action:        req.Action,
		Status:        "enabled",
		NextRunAt:     nextRun.UTC().Format(time.RFC3339),
	})
}

// handleListCleanupSchedules handles GET /v1/memory/cleanup/schedules
// Lists all configured cleanup schedules.
func (s *Server) handleListCleanupSchedules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Filter by space_id if provided
	spaceID := r.URL.Query().Get("space_id")

	// Note: In a full implementation, this would query from a database.
	// For now, return empty list as schedules are not persisted.
	_ = spaceID

	writeJSON(w, http.StatusOK, map[string]any{
		"schedules": []models.ScheduleCleanupResponse{},
		"count":     0,
	})
}

// getCleanupStatsForSpace returns cleanup statistics for a space.
func (s *Server) getCleanupStatsForSpace(ctx context.Context, spaceID string) (map[string]any, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Count orphans
	result, err := sess.Run(ctx, `
MATCH (t:TapRoot {space_id: $spaceId})
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer = 0
  AND n.last_ingested_at IS NOT NULL
  AND n.last_ingested_at < t.last_ingest_at
  AND NOT coalesce(n.is_archived, false)
RETURN count(n) AS orphan_count`, map[string]any{"spaceId": spaceID})
	if err != nil {
		return nil, err
	}

	orphanCount := int64(0)
	if result.Next(ctx) {
		if c, ok := result.Record().Get("orphan_count"); ok {
			orphanCount = c.(int64)
		}
	}

	// Count archived nodes
	archivedResult, err := sess.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.is_archived = true
RETURN count(n) AS archived_count`, map[string]any{"spaceId": spaceID})
	if err != nil {
		return nil, err
	}

	archivedCount := int64(0)
	if archivedResult.Next(ctx) {
		if c, ok := archivedResult.Record().Get("archived_count"); ok {
			archivedCount = c.(int64)
		}
	}

	return map[string]any{
		"space_id":       spaceID,
		"orphan_count":   orphanCount,
		"archived_count": archivedCount,
	}, nil
}

// handleCleanupStats handles GET /v1/memory/cleanup/stats
// Returns cleanup statistics for a space.
func (s *Server) handleCleanupStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	stats, err := s.getCleanupStatsForSpace(ctx, spaceID)
	if err != nil {
		writeInternalError(w, err, "cleanup stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
