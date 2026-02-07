package ape

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/plugins"
)

// =============================================================================
// parseNextCronRun tests - covers all 9 cron patterns
// =============================================================================

func TestParseNextCronRun_EveryMinute(t *testing.T) {
	s := &Scheduler{}
	before := time.Now()
	next := s.parseNextCronRun("* * * * *")
	after := time.Now()

	// Should be about 1 minute from now, truncated to minute boundary
	expected := before.Add(1 * time.Minute).Truncate(time.Minute)
	if next.Before(expected.Add(-1*time.Second)) || next.After(after.Add(1*time.Minute).Add(1*time.Second)) {
		t.Errorf("parseNextCronRun('* * * * *') = %v, want ~%v", next, expected)
	}
}

func TestParseNextCronRun_Every5Minutes(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("*/5 * * * *")

	// Should be next 5-minute boundary
	expected := now.Truncate(5 * time.Minute).Add(5 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('*/5 * * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Every15Minutes(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("*/15 * * * *")

	expected := now.Truncate(15 * time.Minute).Add(15 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('*/15 * * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Every30Minutes(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("*/30 * * * *")

	expected := now.Truncate(30 * time.Minute).Add(30 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('*/30 * * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Hourly(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 * * * *")

	expected := now.Truncate(time.Hour).Add(time.Hour)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('0 * * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Every2Hours(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 */2 * * *")

	expected := now.Truncate(2 * time.Hour).Add(2 * time.Hour)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('0 */2 * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Every6Hours(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 */6 * * *")

	expected := now.Truncate(6 * time.Hour).Add(6 * time.Hour)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('0 */6 * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_DailyMidnight(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 0 * * *")

	expected := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('0 0 * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_UnknownPattern(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 3 * * 1") // Unknown pattern: 3am on Mondays

	// Default: 1 hour from now
	if next.Before(now.Add(59*time.Minute)) || next.After(now.Add(61*time.Minute)) {
		t.Errorf("parseNextCronRun('0 3 * * 1') = %v, want ~1 hour from %v", next, now)
	}
}

// =============================================================================
// NewScheduler tests
// =============================================================================

func TestNewScheduler_WithManager(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	if s.pluginMgr != mgr {
		t.Error("NewScheduler did not set pluginMgr")
	}
	if s.schedules == nil {
		t.Error("NewScheduler did not initialize schedules map")
	}
	if s.lastRun == nil {
		t.Error("NewScheduler did not initialize lastRun map")
	}
	if s.runningTask == nil {
		t.Error("NewScheduler did not initialize runningTask map")
	}
	if s.ctx == nil {
		t.Error("NewScheduler did not create context")
	}
	if s.cancel == nil {
		t.Error("NewScheduler did not create cancel func")
	}
}

func TestNewScheduler_NilManager(t *testing.T) {
	s := NewScheduler(nil)

	if s.pluginMgr != nil {
		t.Error("NewScheduler should allow nil pluginMgr")
	}
	// Maps should still be initialized
	if s.schedules == nil || s.lastRun == nil || s.runningTask == nil {
		t.Error("NewScheduler should initialize maps even with nil manager")
	}
}

// =============================================================================
// GetStatus tests
// =============================================================================

func TestGetStatus_EmptyScheduler(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	status := s.GetStatus()

	if status["enabled"] != true {
		t.Errorf("GetStatus enabled = %v, want true", status["enabled"])
	}
	modules, ok := status["modules"].([]map[string]any)
	if !ok {
		t.Error("GetStatus modules should be []map[string]any")
	}
	if len(modules) != 0 {
		t.Errorf("GetStatus modules len = %d, want 0", len(modules))
	}
}

func TestGetStatus_NilManager(t *testing.T) {
	s := NewScheduler(nil)

	status := s.GetStatus()

	if status["enabled"] != false {
		t.Errorf("GetStatus enabled = %v, want false for nil manager", status["enabled"])
	}
}

func TestGetStatus_WithSchedules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Add a test schedule
	now := time.Now()
	s.schedules["test-module"] = &moduleSchedule{
		ModuleID:       "test-module",
		CronExpression: "0 * * * *",
		EventTriggers:  []string{"ingest", "session_end"},
		MinInterval:    5 * time.Minute,
		nextRun:        now.Add(1 * time.Hour),
	}
	s.lastRun["test-module"] = now.Add(-30 * time.Minute)
	s.runningTask["test-module"] = false

	status := s.GetStatus()

	modules := status["modules"].([]map[string]any)
	if len(modules) != 1 {
		t.Fatalf("GetStatus modules len = %d, want 1", len(modules))
	}

	m := modules[0]
	if m["module_id"] != "test-module" {
		t.Errorf("module_id = %v, want test-module", m["module_id"])
	}
	if m["cron_expression"] != "0 * * * *" {
		t.Errorf("cron_expression = %v, want 0 * * * *", m["cron_expression"])
	}
	if m["running"] != false {
		t.Errorf("running = %v, want false", m["running"])
	}
	if _, ok := m["last_run"]; !ok {
		t.Error("last_run should be present")
	}
	if m["min_interval"] != "5m0s" {
		t.Errorf("min_interval = %v, want 5m0s", m["min_interval"])
	}
}

func TestGetStatus_RunningTask(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	s.schedules["running-module"] = &moduleSchedule{
		ModuleID:       "running-module",
		CronExpression: "*/5 * * * *",
		nextRun:        time.Now().Add(5 * time.Minute),
	}
	s.runningTask["running-module"] = true

	status := s.GetStatus()
	modules := status["modules"].([]map[string]any)

	if len(modules) != 1 {
		t.Fatalf("Expected 1 module, got %d", len(modules))
	}
	if modules[0]["running"] != true {
		t.Errorf("running = %v, want true", modules[0]["running"])
	}
}

// =============================================================================
// Start/Stop lifecycle tests
// =============================================================================

func TestStart_NilManager(t *testing.T) {
	s := NewScheduler(nil)

	err := s.Start()
	if err != nil {
		t.Errorf("Start with nil manager should not error, got: %v", err)
	}
}

func TestStart_EmptyManager(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	err := s.Start()
	if err != nil {
		t.Errorf("Start with empty manager should not error, got: %v", err)
	}

	// Clean up
	s.Stop()
}

func TestStop_GracefulShutdown(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give scheduler loop time to start
	time.Sleep(50 * time.Millisecond)

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop did not complete within 2 seconds")
	}
}

func TestStop_ContextCancellation(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	s.Stop()

	// Context should be cancelled
	select {
	case <-s.ctx.Done():
		// Expected
	default:
		t.Error("Context should be cancelled after Stop")
	}
}

// =============================================================================
// TriggerEvent tests (extending existing tests)
// =============================================================================

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

func TestTriggerEventWithContext_MultipleModules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Register multiple modules listening to the same event
	s.schedules["module-a"] = &moduleSchedule{
		ModuleID:      "module-a",
		EventTriggers: []string{"ingest"},
	}
	s.schedules["module-b"] = &moduleSchedule{
		ModuleID:      "module-b",
		EventTriggers: []string{"ingest", "other"},
	}
	s.schedules["module-c"] = &moduleSchedule{
		ModuleID:      "module-c",
		EventTriggers: []string{"other"},
	}

	// Trigger "ingest" - should match module-a and module-b
	s.TriggerEventWithContext("ingest", nil)
	time.Sleep(50 * time.Millisecond)
}

// =============================================================================
// executeModuleWithContext tests
// =============================================================================

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

func TestExecuteModuleWithContext_AlreadyRunning(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Mark as already running
	s.runningTask["test-module"] = true

	// Should return immediately without error
	s.executeModuleWithContext("test-module", "test", nil)

	// Should still be marked as running (not cleared)
	if !s.runningTask["test-module"] {
		t.Error("runningTask should still be true after skipping execution")
	}
}

func TestExecuteModuleWithContext_SetsAndClearsRunning(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Execute a nonexistent module - it will fail but should clean up state
	s.executeModuleWithContext("nonexistent-module", "test", nil)

	// After execution completes, runningTask should be cleared
	if s.runningTask["nonexistent-module"] {
		t.Error("runningTask should be false after execution completes")
	}

	// lastRun should be set
	if _, ok := s.lastRun["nonexistent-module"]; !ok {
		t.Error("lastRun should be set after execution")
	}
}

// =============================================================================
// checkScheduledTasks tests
// =============================================================================

func TestCheckScheduledTasks_NoCronSchedule(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Add module with no cron expression
	s.schedules["event-only"] = &moduleSchedule{
		ModuleID:      "event-only",
		EventTriggers: []string{"ingest"},
	}

	// Should not panic or trigger execution
	s.checkScheduledTasks()
}

func TestCheckScheduledTasks_NotYetTime(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Add module with nextRun in the future
	s.schedules["future-task"] = &moduleSchedule{
		ModuleID:       "future-task",
		CronExpression: "0 * * * *",
		nextRun:        time.Now().Add(1 * time.Hour),
	}

	// Should not trigger execution
	s.checkScheduledTasks()
	time.Sleep(50 * time.Millisecond)

	// No lastRun should be set
	if _, ok := s.lastRun["future-task"]; ok {
		t.Error("lastRun should not be set for future task")
	}
}

func TestCheckScheduledTasks_MinIntervalNotMet(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	now := time.Now()
	s.schedules["frequent-task"] = &moduleSchedule{
		ModuleID:       "frequent-task",
		CronExpression: "* * * * *",
		MinInterval:    10 * time.Minute,
		nextRun:        now.Add(-1 * time.Minute), // Due
	}
	// Last run was 5 minutes ago, but min interval is 10 minutes
	s.lastRun["frequent-task"] = now.Add(-5 * time.Minute)

	// Should not trigger execution due to min interval
	s.checkScheduledTasks()
	time.Sleep(50 * time.Millisecond)

	// lastRun should still be the old time
	if s.lastRun["frequent-task"].After(now.Add(-4 * time.Minute)) {
		t.Error("lastRun should not have been updated")
	}
}

// =============================================================================
// refreshSchedules tests
// =============================================================================

func TestRefreshSchedules_NoModules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	err := s.refreshSchedules()
	if err != nil {
		t.Errorf("refreshSchedules with no modules should not error: %v", err)
	}

	if len(s.schedules) != 0 {
		t.Errorf("schedules should be empty, got %d", len(s.schedules))
	}
}
