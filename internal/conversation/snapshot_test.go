package conversation

import (
	"testing"
	"time"
)

func TestSnapshotTriggerConstants(t *testing.T) {
	tests := []struct {
		trigger  SnapshotTrigger
		expected string
	}{
		{TriggerManual, "manual"},
		{TriggerCompaction, "compaction"},
		{TriggerSessionEnd, "session_end"},
		{TriggerError, "error"},
	}

	for _, tt := range tests {
		if string(tt.trigger) != tt.expected {
			t.Errorf("Expected trigger %q, got %q", tt.expected, string(tt.trigger))
		}
	}
}

func TestTaskSnapshotToResponse(t *testing.T) {
	now := time.Now()
	snapshot := &TaskSnapshot{
		SnapshotID: "snap-123",
		SpaceID:    "test-space",
		SessionID:  "session-456",
		Trigger:    TriggerManual,
		Context: map[string]interface{}{
			"task_name":   "Test Task",
			"active_files": []string{"file1.go", "file2.go"},
		},
		CreatedAt: now,
	}

	response := snapshot.ToResponse()

	if response.SnapshotID != "snap-123" {
		t.Errorf("Expected snapshot_id 'snap-123', got %q", response.SnapshotID)
	}
	if response.SpaceID != "test-space" {
		t.Errorf("Expected space_id 'test-space', got %q", response.SpaceID)
	}
	if response.SessionID != "session-456" {
		t.Errorf("Expected session_id 'session-456', got %q", response.SessionID)
	}
	if response.Trigger != "manual" {
		t.Errorf("Expected trigger 'manual', got %q", response.Trigger)
	}
	if response.Context == nil {
		t.Error("Expected context to be non-nil")
	}
	if response.Context["task_name"] != "Test Task" {
		t.Errorf("Expected task_name 'Test Task', got %v", response.Context["task_name"])
	}
}

func TestSnapshotContext(t *testing.T) {
	ctx := SnapshotContext{
		TaskName:        "Phase 60 Implementation",
		ActiveFiles:     []string{"snapshot.go", "service.go"},
		CurrentGoal:     "Add snapshot support",
		RecentToolCalls: []string{"Read", "Write", "Edit"},
		PendingItems:    []string{"Add tests", "Update docs"},
		Blockers:        []string{},
		RecentDecisions: []string{"Use Neo4j for storage"},
		SessionNotes:    "Making good progress",
	}

	if ctx.TaskName != "Phase 60 Implementation" {
		t.Errorf("Expected task_name 'Phase 60 Implementation', got %q", ctx.TaskName)
	}
	if len(ctx.ActiveFiles) != 2 {
		t.Errorf("Expected 2 active files, got %d", len(ctx.ActiveFiles))
	}
	if len(ctx.RecentToolCalls) != 3 {
		t.Errorf("Expected 3 recent tool calls, got %d", len(ctx.RecentToolCalls))
	}
	if len(ctx.PendingItems) != 2 {
		t.Errorf("Expected 2 pending items, got %d", len(ctx.PendingItems))
	}
	if len(ctx.Blockers) != 0 {
		t.Errorf("Expected 0 blockers, got %d", len(ctx.Blockers))
	}
}

func TestCreateSnapshotRequest(t *testing.T) {
	req := CreateSnapshotRequest{
		SpaceID:   "mdemg-dev",
		SessionID: "claude-core",
		Trigger:   "compaction",
		Context: map[string]interface{}{
			"task_name": "Test",
			"status":    "in_progress",
		},
	}

	if req.SpaceID != "mdemg-dev" {
		t.Errorf("Expected space_id 'mdemg-dev', got %q", req.SpaceID)
	}
	if req.Trigger != "compaction" {
		t.Errorf("Expected trigger 'compaction', got %q", req.Trigger)
	}
}

func TestListSnapshotsResponse(t *testing.T) {
	now := time.Now()
	response := ListSnapshotsResponse{
		Snapshots: []SnapshotResponse{
			{
				SnapshotID: "snap-1",
				SpaceID:    "test-space",
				SessionID:  "session-1",
				Trigger:    "manual",
				Context:    map[string]interface{}{"task": "Task 1"},
				CreatedAt:  now.Format(time.RFC3339),
			},
			{
				SnapshotID: "snap-2",
				SpaceID:    "test-space",
				SessionID:  "session-1",
				Trigger:    "compaction",
				Context:    map[string]interface{}{"task": "Task 2"},
				CreatedAt:  now.Add(-1 * time.Hour).Format(time.RFC3339),
			},
		},
		Count: 2,
	}

	if response.Count != 2 {
		t.Errorf("Expected count 2, got %d", response.Count)
	}
	if len(response.Snapshots) != 2 {
		t.Errorf("Expected 2 snapshots, got %d", len(response.Snapshots))
	}
	if response.Snapshots[0].SnapshotID != "snap-1" {
		t.Errorf("Expected first snapshot 'snap-1', got %q", response.Snapshots[0].SnapshotID)
	}
}
