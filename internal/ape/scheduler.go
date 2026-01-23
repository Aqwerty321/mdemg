package ape

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	pb "mdemg/api/modulepb"
	"mdemg/internal/plugins"
)

// Scheduler manages execution of APE (Active Participant Engine) modules.
// It handles scheduled tasks and event-triggered executions.
type Scheduler struct {
	pluginMgr *plugins.Manager

	mu          sync.RWMutex
	schedules   map[string]*moduleSchedule // moduleID -> schedule
	lastRun     map[string]time.Time       // moduleID -> last execution time
	runningTask map[string]bool            // moduleID -> currently running

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type moduleSchedule struct {
	ModuleID       string
	CronExpression string   // e.g., "0 * * * *" for hourly
	EventTriggers  []string // e.g., ["ingest", "session_end"]
	MinInterval    time.Duration
	nextRun        time.Time
}

// NewScheduler creates a new APE scheduler
func NewScheduler(pluginMgr *plugins.Manager) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		pluginMgr:   pluginMgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start initializes schedules from APE modules and starts the scheduler loop
func (s *Scheduler) Start() error {
	if s.pluginMgr == nil {
		log.Printf("ape: no plugin manager, scheduler disabled")
		return nil
	}

	// Fetch schedules from all APE modules
	if err := s.refreshSchedules(); err != nil {
		log.Printf("ape: failed to refresh schedules: %v", err)
	}

	// Start scheduler loop
	s.wg.Add(1)
	go s.schedulerLoop()

	log.Printf("ape: scheduler started with %d modules", len(s.schedules))
	return nil
}

// Stop gracefully shuts down the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
	log.Printf("ape: scheduler stopped")
}

// TriggerEvent triggers all APE modules subscribed to the given event
func (s *Scheduler) TriggerEvent(event string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for moduleID, sched := range s.schedules {
		for _, trigger := range sched.EventTriggers {
			if trigger == event {
				go s.executeModule(moduleID, "event:"+event)
				break
			}
		}
	}
}

// refreshSchedules queries all APE modules for their schedules
func (s *Scheduler) refreshSchedules() error {
	modules := s.pluginMgr.GetAPEModules()
	if len(modules) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, mod := range modules {
		if mod.APEClient == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		resp, err := mod.APEClient.GetSchedule(ctx, &pb.GetScheduleRequest{})
		cancel()

		if err != nil {
			log.Printf("ape: failed to get schedule from %s: %v", mod.Manifest.ID, err)
			continue
		}

		sched := &moduleSchedule{
			ModuleID:       mod.Manifest.ID,
			CronExpression: resp.CronExpression,
			EventTriggers:  resp.EventTriggers,
			MinInterval:    time.Duration(resp.MinIntervalSeconds) * time.Second,
		}

		// Calculate next run time from cron
		if sched.CronExpression != "" {
			sched.nextRun = s.parseNextCronRun(sched.CronExpression)
		}

		s.schedules[mod.Manifest.ID] = sched
		log.Printf("ape: registered module %s (cron=%s, events=%v, minInterval=%v)",
			mod.Manifest.ID, sched.CronExpression, sched.EventTriggers, sched.MinInterval)
	}

	return nil
}

// schedulerLoop runs the main scheduling loop
func (s *Scheduler) schedulerLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkScheduledTasks()
		}
	}
}

// checkScheduledTasks checks if any scheduled tasks should run
func (s *Scheduler) checkScheduledTasks() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()

	for moduleID, sched := range s.schedules {
		// Skip if no cron schedule
		if sched.CronExpression == "" {
			continue
		}

		// Skip if not yet time
		if now.Before(sched.nextRun) {
			continue
		}

		// Check minimum interval
		if lastRun, ok := s.lastRun[moduleID]; ok {
			if now.Sub(lastRun) < sched.MinInterval {
				continue
			}
		}

		// Execute in goroutine
		go s.executeModule(moduleID, "schedule")

		// Update next run time
		sched.nextRun = s.parseNextCronRun(sched.CronExpression)
	}
}

// executeModule runs an APE module
func (s *Scheduler) executeModule(moduleID, trigger string) {
	// Check if already running
	s.mu.Lock()
	if s.runningTask[moduleID] {
		s.mu.Unlock()
		log.Printf("ape: skipping %s, already running", moduleID)
		return
	}
	s.runningTask[moduleID] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.runningTask[moduleID] = false
		s.lastRun[moduleID] = time.Now()
		s.mu.Unlock()
	}()

	// Get the module
	modInfo, ok := s.pluginMgr.GetModule(moduleID)
	if !ok || modInfo.APEClient == nil {
		log.Printf("ape: module %s not found or not APE type", moduleID)
		return
	}

	taskID := uuid.New().String()
	log.Printf("ape: executing %s (task=%s, trigger=%s)", moduleID, taskID[:8], trigger)

	start := time.Now()
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	resp, err := modInfo.APEClient.Execute(ctx, &pb.ExecuteRequest{
		TaskId:  taskID,
		Trigger: trigger,
		Context: map[string]string{},
	})

	elapsed := time.Since(start)

	if err != nil {
		log.Printf("ape: %s failed after %v: %v", moduleID, elapsed, err)
		return
	}

	if resp.Error != "" {
		log.Printf("ape: %s completed with error after %v: %s", moduleID, elapsed, resp.Error)
		return
	}

	if resp.Stats != nil {
		log.Printf("ape: %s completed in %v (nodes: +%d/~%d, edges: +%d/~%d)",
			moduleID, elapsed,
			resp.Stats.NodesCreated, resp.Stats.NodesUpdated,
			resp.Stats.EdgesCreated, resp.Stats.EdgesUpdated)
	} else {
		log.Printf("ape: %s completed in %v: %s", moduleID, elapsed, resp.Message)
	}
}

// parseNextCronRun parses a cron expression and returns the next run time.
// Simplified implementation supporting: minute hour day month weekday
// Examples: "0 * * * *" (hourly), "*/15 * * * *" (every 15 min), "0 0 * * *" (daily)
func (s *Scheduler) parseNextCronRun(cron string) time.Time {
	// Simplified: parse common patterns
	now := time.Now()

	switch {
	case cron == "* * * * *": // Every minute
		return now.Add(1 * time.Minute).Truncate(time.Minute)

	case cron == "*/5 * * * *": // Every 5 minutes
		next := now.Truncate(5 * time.Minute).Add(5 * time.Minute)
		return next

	case cron == "*/15 * * * *": // Every 15 minutes
		next := now.Truncate(15 * time.Minute).Add(15 * time.Minute)
		return next

	case cron == "*/30 * * * *": // Every 30 minutes
		next := now.Truncate(30 * time.Minute).Add(30 * time.Minute)
		return next

	case cron == "0 * * * *": // Every hour
		next := now.Truncate(time.Hour).Add(time.Hour)
		return next

	case cron == "0 */2 * * *": // Every 2 hours
		next := now.Truncate(2 * time.Hour).Add(2 * time.Hour)
		return next

	case cron == "0 */6 * * *": // Every 6 hours
		next := now.Truncate(6 * time.Hour).Add(6 * time.Hour)
		return next

	case cron == "0 0 * * *": // Daily at midnight
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		return next

	default:
		// Default: run in 1 hour
		return now.Add(1 * time.Hour)
	}
}

// GetStatus returns the current scheduler status
func (s *Scheduler) GetStatus() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	modules := make([]map[string]any, 0, len(s.schedules))
	for moduleID, sched := range s.schedules {
		m := map[string]any{
			"module_id":       moduleID,
			"cron_expression": sched.CronExpression,
			"event_triggers":  sched.EventTriggers,
			"min_interval":    sched.MinInterval.String(),
			"next_run":        sched.nextRun.Format(time.RFC3339),
			"running":         s.runningTask[moduleID],
		}
		if lastRun, ok := s.lastRun[moduleID]; ok {
			m["last_run"] = lastRun.Format(time.RFC3339)
		}
		modules = append(modules, m)
	}

	return map[string]any{
		"enabled": s.pluginMgr != nil,
		"modules": modules,
	}
}
