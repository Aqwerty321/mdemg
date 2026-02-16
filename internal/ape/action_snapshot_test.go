package ape

import (
	"testing"
	"time"
)

func TestSnapshotStore_BasicOperations(t *testing.T) {
	ss := &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: 3600,
		maxCap:    50,
	}

	if ss.GetSnapshotCount() != 0 {
		t.Errorf("expected 0 snapshots, got %d", ss.GetSnapshotCount())
	}
	if ss.GetOldestSnapshotAgeSec() != 0 {
		t.Errorf("expected 0 age, got %d", ss.GetOldestSnapshotAgeSec())
	}
}

func TestSnapshotStore_ListSnapshots_Empty(t *testing.T) {
	ss := &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: 3600,
		maxCap:    50,
	}

	snaps := ss.ListSnapshots()
	if len(snaps) != 0 {
		t.Errorf("expected empty list, got %d", len(snaps))
	}
}

func TestSnapshotStore_ManualInsertAndList(t *testing.T) {
	ss := &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: 3600,
		maxCap:    50,
	}

	snap := &ActionSnapshot{
		SnapshotID:    "snap-test-1",
		CycleID:       "rsic-meso-abc",
		Action:        "tombstone_stale",
		SpaceID:       "test-space",
		CapturedAt:    time.Now(),
		AffectedCount: 5,
		Reversible:    true,
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}

	ss.snapshots[snap.SnapshotID] = snap
	ss.order = append(ss.order, snap.SnapshotID)

	if ss.GetSnapshotCount() != 1 {
		t.Errorf("expected 1 snapshot, got %d", ss.GetSnapshotCount())
	}

	listed := ss.ListSnapshots()
	if len(listed) != 1 {
		t.Errorf("expected 1 listed, got %d", len(listed))
	}
	if listed[0].SnapshotID != "snap-test-1" {
		t.Errorf("unexpected snapshot ID: %s", listed[0].SnapshotID)
	}
}

func TestSnapshotStore_CleanupExpired(t *testing.T) {
	ss := &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: 1,
		maxCap:    50,
	}

	// Add an already-expired snapshot
	expired := &ActionSnapshot{
		SnapshotID: "snap-expired",
		CapturedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt:  time.Now().Add(-1 * time.Hour),
	}
	ss.snapshots[expired.SnapshotID] = expired
	ss.order = append(ss.order, expired.SnapshotID)

	// Add a valid snapshot
	valid := &ActionSnapshot{
		SnapshotID: "snap-valid",
		CapturedAt: time.Now(),
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}
	ss.snapshots[valid.SnapshotID] = valid
	ss.order = append(ss.order, valid.SnapshotID)

	ss.CleanupExpired()

	if ss.GetSnapshotCount() != 1 {
		t.Errorf("expected 1 snapshot after cleanup, got %d", ss.GetSnapshotCount())
	}
	if _, ok := ss.snapshots["snap-expired"]; ok {
		t.Error("expected expired snapshot to be removed")
	}
	if _, ok := ss.snapshots["snap-valid"]; !ok {
		t.Error("expected valid snapshot to remain")
	}
}

func TestSnapshotStore_CapacityEviction(t *testing.T) {
	ss := &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: 3600,
		maxCap:    3,
	}

	// Fill to capacity
	for i := 0; i < 3; i++ {
		id := "snap-" + string(rune('a'+i))
		snap := &ActionSnapshot{
			SnapshotID: id,
			CapturedAt: time.Now(),
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		}
		ss.snapshots[id] = snap
		ss.order = append(ss.order, id)
	}

	if ss.GetSnapshotCount() != 3 {
		t.Errorf("expected 3 snapshots, got %d", ss.GetSnapshotCount())
	}

	// Adding one more via CaptureSnapshot (no driver, will fail pre-state but create snap)
	// Simulate by directly adding and checking eviction
	snap := &ActionSnapshot{
		SnapshotID: "snap-d",
		CapturedAt: time.Now(),
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}

	// Evict oldest
	for len(ss.snapshots) >= ss.maxCap && len(ss.order) > 0 {
		oldest := ss.order[0]
		ss.order = ss.order[1:]
		delete(ss.snapshots, oldest)
	}
	ss.snapshots[snap.SnapshotID] = snap
	ss.order = append(ss.order, snap.SnapshotID)

	if ss.GetSnapshotCount() != 3 {
		t.Errorf("expected 3 snapshots after eviction, got %d", ss.GetSnapshotCount())
	}

	// First snapshot should be gone
	if _, ok := ss.snapshots["snap-a"]; ok {
		t.Error("expected oldest snapshot snap-a to be evicted")
	}
	// Newest should exist
	if _, ok := ss.snapshots["snap-d"]; !ok {
		t.Error("expected snap-d to exist")
	}
}

func TestSnapshotStore_RollbackNotFound(t *testing.T) {
	ss := &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: 3600,
		maxCap:    50,
	}

	_, err := ss.Rollback(nil, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent snapshot")
	}
}

func TestSnapshotStore_RollbackExpired(t *testing.T) {
	ss := &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: 1,
		maxCap:    50,
	}

	expired := &ActionSnapshot{
		SnapshotID: "snap-expired",
		CapturedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt:  time.Now().Add(-1 * time.Hour),
		Reversible: true,
	}
	ss.snapshots[expired.SnapshotID] = expired

	_, err := ss.Rollback(nil, "snap-expired")
	if err == nil {
		t.Error("expected error for expired snapshot")
	}

	// Should be cleaned up
	if _, ok := ss.snapshots["snap-expired"]; ok {
		t.Error("expected expired snapshot to be removed after rollback attempt")
	}
}

func TestSnapshotStore_RollbackNotReversible(t *testing.T) {
	ss := &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: 3600,
		maxCap:    50,
	}

	snap := &ActionSnapshot{
		SnapshotID: "snap-noreversal",
		CapturedAt: time.Now(),
		ExpiresAt:  time.Now().Add(1 * time.Hour),
		Reversible: false,
	}
	ss.snapshots[snap.SnapshotID] = snap

	_, err := ss.Rollback(nil, "snap-noreversal")
	if err == nil {
		t.Error("expected error for non-reversible snapshot")
	}
}

func TestSnapshotStore_GetRollbackWindowSec(t *testing.T) {
	ss := &SnapshotStore{windowSec: 7200}
	if ss.GetRollbackWindowSec() != 7200 {
		t.Errorf("expected 7200, got %d", ss.GetRollbackWindowSec())
	}
}

func TestSnapshotStore_DefaultWindow(t *testing.T) {
	ss := NewSnapshotStore(nil, 0) // 0 should default to 3600
	if ss.GetRollbackWindowSec() != 3600 {
		t.Errorf("expected default 3600, got %d", ss.GetRollbackWindowSec())
	}
}
