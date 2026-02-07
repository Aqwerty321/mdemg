package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// SnapshotTrigger represents what caused the snapshot
type SnapshotTrigger string

const (
	TriggerManual     SnapshotTrigger = "manual"
	TriggerCompaction SnapshotTrigger = "compaction"
	TriggerSessionEnd SnapshotTrigger = "session_end"
	TriggerError      SnapshotTrigger = "error"
)

// TaskSnapshot captures task state for continuity across sessions
type TaskSnapshot struct {
	SnapshotID string                 `json:"snapshot_id"`
	SpaceID    string                 `json:"space_id"`
	SessionID  string                 `json:"session_id"`
	Trigger    SnapshotTrigger        `json:"trigger"`
	Context    map[string]interface{} `json:"context"`
	CreatedAt  time.Time              `json:"created_at"`
}

// SnapshotContext is the expected structure for task context
type SnapshotContext struct {
	TaskName         string   `json:"task_name,omitempty"`
	ActiveFiles      []string `json:"active_files,omitempty"`
	CurrentGoal      string   `json:"current_goal,omitempty"`
	RecentToolCalls  []string `json:"recent_tool_calls,omitempty"`
	PendingItems     []string `json:"pending_items,omitempty"`
	Blockers         []string `json:"blockers,omitempty"`
	RecentDecisions  []string `json:"recent_decisions,omitempty"`
	SessionNotes     string   `json:"session_notes,omitempty"`
}

// SnapshotService handles task context snapshot operations
type SnapshotService struct {
	driver neo4j.DriverWithContext
}

// NewSnapshotService creates a new snapshot service
func NewSnapshotService(driver neo4j.DriverWithContext) *SnapshotService {
	return &SnapshotService{driver: driver}
}

// CreateSnapshot creates a new task context snapshot
func (s *SnapshotService) CreateSnapshot(ctx context.Context, snapshot *TaskSnapshot) error {
	if snapshot.SnapshotID == "" {
		snapshot.SnapshotID = uuid.New().String()
	}
	if snapshot.SpaceID == "" {
		return fmt.Errorf("space_id is required")
	}
	if snapshot.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if snapshot.Trigger == "" {
		snapshot.Trigger = TriggerManual
	}
	if snapshot.Context == nil {
		snapshot.Context = make(map[string]interface{})
	}

	snapshot.CreatedAt = time.Now()

	// Serialize context to JSON
	contextJSON, err := json.Marshal(snapshot.Context)
	if err != nil {
		return fmt.Errorf("failed to serialize context: %w", err)
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			CREATE (snap:TaskSnapshot {
				snapshot_id: $snapshotId,
				space_id: $spaceId,
				session_id: $sessionId,
				trigger: $trigger,
				context: $context,
				created_at: datetime($createdAt)
			})
			RETURN snap.snapshot_id AS id
		`
		params := map[string]interface{}{
			"snapshotId": snapshot.SnapshotID,
			"spaceId":    snapshot.SpaceID,
			"sessionId":  snapshot.SessionID,
			"trigger":    string(snapshot.Trigger),
			"context":    string(contextJSON),
			"createdAt":  snapshot.CreatedAt.Format(time.RFC3339),
		}
		_, err := tx.Run(ctx, query, params)
		return nil, err
	})

	return err
}

// GetSnapshot retrieves a snapshot by ID
func (s *SnapshotService) GetSnapshot(ctx context.Context, snapshotID string) (*TaskSnapshot, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (snap:TaskSnapshot {snapshot_id: $snapshotId})
			RETURN snap
		`
		params := map[string]interface{}{
			"snapshotId": snapshotID,
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			return res.Record().Values[0], nil
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return parseSnapshotNode(result.(neo4j.Node))
}

// GetLatestSnapshot retrieves the most recent snapshot for a space/session
func (s *SnapshotService) GetLatestSnapshot(ctx context.Context, spaceID, sessionID string) (*TaskSnapshot, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (snap:TaskSnapshot {space_id: $spaceId})
			WHERE snap.session_id = $sessionId OR $sessionId = ""
			RETURN snap
			ORDER BY snap.created_at DESC
			LIMIT 1
		`
		params := map[string]interface{}{
			"spaceId":   spaceID,
			"sessionId": sessionID,
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			return res.Record().Values[0], nil
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return parseSnapshotNode(result.(neo4j.Node))
}

// ListSnapshots retrieves snapshots for a space with optional filtering
func (s *SnapshotService) ListSnapshots(ctx context.Context, spaceID, sessionID string, limit int) ([]*TaskSnapshot, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (snap:TaskSnapshot {space_id: $spaceId})
			WHERE snap.session_id = $sessionId OR $sessionId = ""
			RETURN snap
			ORDER BY snap.created_at DESC
			LIMIT $limit
		`
		params := map[string]interface{}{
			"spaceId":   spaceID,
			"sessionId": sessionID,
			"limit":     int64(limit),
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		var snapshots []*TaskSnapshot
		for res.Next(ctx) {
			node := res.Record().Values[0].(neo4j.Node)
			snap, err := parseSnapshotNode(node)
			if err != nil {
				continue
			}
			snapshots = append(snapshots, snap)
		}
		return snapshots, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]*TaskSnapshot), nil
}

// DeleteSnapshot deletes a snapshot
func (s *SnapshotService) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (snap:TaskSnapshot {snapshot_id: $snapshotId})
			DELETE snap
			RETURN count(snap) AS deleted
		`
		params := map[string]interface{}{
			"snapshotId": snapshotID,
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			deleted, _ := res.Record().Get("deleted")
			if deleted.(int64) == 0 {
				return nil, fmt.Errorf("snapshot not found")
			}
		}
		return nil, nil
	})

	return err
}

// CleanupOldSnapshots removes snapshots older than the retention period
func (s *SnapshotService) CleanupOldSnapshots(ctx context.Context, spaceID string, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30 // Default 30 days
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (snap:TaskSnapshot {space_id: $spaceId})
			WHERE snap.created_at < datetime() - duration({days: $retentionDays})
			WITH snap
			DELETE snap
			RETURN count(*) AS deleted
		`
		params := map[string]interface{}{
			"spaceId":       spaceID,
			"retentionDays": int64(retentionDays),
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			deleted, _ := res.Record().Get("deleted")
			return deleted.(int64), nil
		}
		return int64(0), nil
	})

	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// CaptureSnapshotOnTrigger creates a snapshot if auto-capture is enabled for this trigger
func (s *SnapshotService) CaptureSnapshotOnTrigger(ctx context.Context, spaceID, sessionID string, trigger SnapshotTrigger, context map[string]interface{}) (*TaskSnapshot, error) {
	snapshot := &TaskSnapshot{
		SpaceID:   spaceID,
		SessionID: sessionID,
		Trigger:   trigger,
		Context:   context,
	}

	if err := s.CreateSnapshot(ctx, snapshot); err != nil {
		return nil, err
	}

	return snapshot, nil
}

// parseSnapshotNode converts a Neo4j node to TaskSnapshot
func parseSnapshotNode(node neo4j.Node) (*TaskSnapshot, error) {
	props := node.Props

	snapshot := &TaskSnapshot{
		SnapshotID: props["snapshot_id"].(string),
		SpaceID:    props["space_id"].(string),
		SessionID:  props["session_id"].(string),
		Trigger:    SnapshotTrigger(props["trigger"].(string)),
	}

	// Parse context JSON
	if contextStr, ok := props["context"].(string); ok && contextStr != "" {
		var context map[string]interface{}
		if err := json.Unmarshal([]byte(contextStr), &context); err == nil {
			snapshot.Context = context
		}
	}

	// Parse timestamp
	if createdAt, ok := props["created_at"].(time.Time); ok {
		snapshot.CreatedAt = createdAt
	}

	return snapshot, nil
}

// =============================================================================
// Snapshot Request/Response Types for API
// =============================================================================

// CreateSnapshotRequest is the API request for creating a snapshot
type CreateSnapshotRequest struct {
	SpaceID   string                 `json:"space_id"`
	SessionID string                 `json:"session_id"`
	Trigger   string                 `json:"trigger,omitempty"`
	Context   map[string]interface{} `json:"context"`
}

// SnapshotResponse is the API response for a snapshot
type SnapshotResponse struct {
	SnapshotID string                 `json:"snapshot_id"`
	SpaceID    string                 `json:"space_id"`
	SessionID  string                 `json:"session_id"`
	Trigger    string                 `json:"trigger"`
	Context    map[string]interface{} `json:"context"`
	CreatedAt  string                 `json:"created_at"`
}

// ListSnapshotsResponse is the API response for listing snapshots
type ListSnapshotsResponse struct {
	Snapshots []SnapshotResponse `json:"snapshots"`
	Count     int                `json:"count"`
}

// ToResponse converts a TaskSnapshot to SnapshotResponse
func (snap *TaskSnapshot) ToResponse() SnapshotResponse {
	return SnapshotResponse{
		SnapshotID: snap.SnapshotID,
		SpaceID:    snap.SpaceID,
		SessionID:  snap.SessionID,
		Trigger:    string(snap.Trigger),
		Context:    snap.Context,
		CreatedAt:  snap.CreatedAt.Format(time.RFC3339),
	}
}
