package models

import (
	"crypto/rand"
	"fmt"
	"time"
)

// BaseEdgeProperties returns standard properties for new edges.
// Ensures all edges have consistent metadata.
func BaseEdgeProperties(spaceID string) map[string]any {
	now := time.Now()
	return map[string]any{
		"edge_id":        generateEdgeID(),
		"space_id":       spaceID,
		"created_at":     now,
		"updated_at":     now,
		"version":        1,
		"status":         "active",
		"weight":         1.0,
		"evidence_count": 1,
	}
}

func generateEdgeID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
