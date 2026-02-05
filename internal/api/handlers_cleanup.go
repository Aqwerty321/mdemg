package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

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

	// Step 1: Detect orphans — L0 nodes with last_ingested_at < TapRoot.last_ingest_at
	detectCypher := `
MATCH (t:TapRoot {space_id: $spaceId})
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer = 0
  AND n.last_ingested_at IS NOT NULL
  AND n.last_ingested_at < t.last_ingest_at
  AND NOT coalesce(n.is_archived, false)
RETURN n.node_id AS node_id, n.path AS path, n.name AS name,
       toString(n.last_ingested_at) AS last_ingested_at
ORDER BY n.path
LIMIT $limit`

	params := map[string]any{
		"spaceId": req.SpaceID,
		"limit":   req.Limit,
	}

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
