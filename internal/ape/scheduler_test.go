package ape

import (
	"testing"
	"time"

	"mdemg/internal/plugins"
)

func TestTriggerEventWithContext_ContextPassed(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Register a schedule that listens for "ingest_complete"
	s.schedules["test-module"] = &moduleSchedule{
		ModuleID:      "test-module",
		EventTriggers: []string{"ingest_complete", "source_changed"},
	}

	ctx := map[string]string{
		"space_id":    "test-space",
		"ingest_type": "batch-ingest",
	}

	// TriggerEventWithContext matches the event and spawns a goroutine.
	// The module won't be found (no real plugins loaded), so it logs and returns.
	s.TriggerEventWithContext("ingest_complete", ctx)

	// Give goroutine time to start and exit
	time.Sleep(50 * time.Millisecond)
}

func TestTriggerEventWithContext_NoMatch(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Register a schedule that listens for "session_end" only
	s.schedules["test-module"] = &moduleSchedule{
		ModuleID:      "test-module",
		EventTriggers: []string{"session_end"},
	}

	// Trigger a different event - should not match, no goroutine spawned
	s.TriggerEventWithContext("ingest_complete", map[string]string{
		"space_id": "test-space",
	})

	time.Sleep(50 * time.Millisecond)
}

func TestTriggerEvent_DelegatesToWithContext(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// TriggerEvent delegates to TriggerEventWithContext(nil) — should not panic
	s.TriggerEvent("test_event")
	time.Sleep(50 * time.Millisecond)
}

func TestExecuteModuleWithContext_NilContext(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Module won't be found, logs and returns without panic
	s.executeModuleWithContext("nonexistent-module", "test", nil)
}

func TestExecuteModuleWithContext_EmptyContext(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	s.executeModuleWithContext("nonexistent-module", "test", map[string]string{})
}
