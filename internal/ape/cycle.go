package ape

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"mdemg/internal/config"
)

// CycleOrchestrator runs the full Assess → Reflect → Plan → Execute → Validate cycle.
type CycleOrchestrator struct {
	assessor   *Assessor
	reflector  *Reflector
	planner    *Planner
	dispatcher *Dispatcher
	monitor    *Monitor
	calibrator *Calibrator
	watchdog   *Watchdog
	cfg        config.Config
}

// NewCycleOrchestrator wires together all RSIC components.
func NewCycleOrchestrator(
	cfg config.Config,
	assessor *Assessor,
	reflector *Reflector,
	planner *Planner,
	dispatcher *Dispatcher,
	monitor *Monitor,
	calibrator *Calibrator,
	watchdog *Watchdog,
) *CycleOrchestrator {
	return &CycleOrchestrator{
		assessor:   assessor,
		reflector:  reflector,
		planner:    planner,
		dispatcher: dispatcher,
		monitor:    monitor,
		calibrator: calibrator,
		watchdog:   watchdog,
		cfg:        cfg,
	}
}

// RunCycle executes the full 5-stage RSIC cycle for the given space and tier.
func (c *CycleOrchestrator) RunCycle(ctx context.Context, spaceID string, tier CycleTier) (*CycleOutcome, error) {
	cycleID := fmt.Sprintf("rsic-%s-%s", tier, uuid.New().String()[:8])
	startedAt := time.Now()

	log.Printf("RSIC cycle %s started (tier=%s, space=%s)", cycleID, tier, spaceID)

	// Stage 1: Assess
	report, err := c.assessor.Assess(ctx, spaceID, tier)
	if err != nil {
		return nil, fmt.Errorf("assess failed: %w", err)
	}
	log.Printf("RSIC %s: assess complete (health=%.2f, confidence=%.2f)", cycleID, report.OverallHealth, report.Confidence)

	// Bail early if confidence is too low
	if report.Confidence < c.cfg.RSICMinConfidence {
		return &CycleOutcome{
			CycleID:     cycleID,
			Tier:        tier,
			SpaceID:     spaceID,
			StartedAt:   startedAt,
			CompletedAt: time.Now(),
			Error:       fmt.Sprintf("confidence %.2f below threshold %.2f", report.Confidence, c.cfg.RSICMinConfidence),
		}, nil
	}

	// Stage 2: Reflect
	insights, err := c.reflector.Reflect(ctx, report)
	if err != nil {
		return nil, fmt.Errorf("reflect failed: %w", err)
	}
	log.Printf("RSIC %s: reflect complete (%d insights)", cycleID, len(insights))

	if len(insights) == 0 {
		outcome := &CycleOutcome{
			CycleID:     cycleID,
			Tier:        tier,
			SpaceID:     spaceID,
			StartedAt:   startedAt,
			CompletedAt: time.Now(),
			MetricsBefore: map[string]float64{
				"overall_health": report.OverallHealth,
			},
		}
		log.Printf("RSIC %s: no insights — system is healthy", cycleID)
		if c.watchdog != nil {
			c.watchdog.RecordCycle()
		}
		return outcome, nil
	}

	// Stage 3: Plan
	baseline := map[string]float64{
		"overall_health":   report.OverallHealth,
		"edge_count":       float64(report.EdgeCount),
		"orphan_ratio":     report.OrphanRatio,
		"volatile_count":   float64(report.VolatileCount),
		"correction_rate":  report.CorrectionRate,
		"edge_entropy":     report.EdgeWeightEntropy,
	}

	tasks, err := c.planner.Plan(ctx, insights, spaceID, baseline)
	if err != nil {
		return nil, fmt.Errorf("plan failed: %w", err)
	}
	log.Printf("RSIC %s: plan complete (%d tasks)", cycleID, len(tasks))

	// Stamp cycle ID into each task
	for i := range tasks {
		tasks[i].CycleID = cycleID
		tasks[i].TaskID = fmt.Sprintf("%s-task-%d", cycleID, i)
	}

	// Stage 4: Execute (dispatch + wait)
	if err := c.dispatcher.Dispatch(ctx, tasks); err != nil {
		return nil, fmt.Errorf("dispatch failed: %w", err)
	}

	// Wait for completion with tier-dependent timeout
	timeout := c.tierTimeout(tier)
	completed := c.monitor.WaitForCycle(cycleID, timeout)
	if !completed {
		log.Printf("RSIC %s: timed out after %s", cycleID, timeout)
	}

	// Stage 5: Validate + Calibrate
	reports := c.monitor.CollectReportsForCycle(cycleID)
	outcome := c.calibrator.Validate(ctx, cycleID, tier, spaceID, tasks, reports, baseline)
	outcome.StartedAt = startedAt
	outcome.Insights = insights

	c.calibrator.UpdateCalibration(outcome, tasks, reports)

	log.Printf("RSIC %s: cycle complete (executed=%d, success=%d, failed=%d)",
		cycleID, outcome.ActionsExecuted, outcome.SuccessCount, outcome.FailedCount)

	// Reset watchdog
	if c.watchdog != nil {
		c.watchdog.RecordCycle()
	}

	return outcome, nil
}

// Assess exposes just the assessment stage for the API.
func (c *CycleOrchestrator) Assess(ctx context.Context, spaceID string, tier CycleTier) (*SelfAssessmentReport, error) {
	return c.assessor.Assess(ctx, spaceID, tier)
}

// GetCalibration returns current per-action confidence scores.
func (c *CycleOrchestrator) GetCalibration() map[string]float64 {
	return c.calibrator.GetCalibration()
}

// GetHistory returns recent cycle outcomes.
func (c *CycleOrchestrator) GetHistory(limit int) []CycleOutcome {
	return c.calibrator.GetHistory(limit)
}

// GetWatchdogState returns the current watchdog state.
func (c *CycleOrchestrator) GetWatchdogState() *WatchdogState {
	if c.watchdog == nil {
		return nil
	}
	st := c.watchdog.GetState()
	return &st
}

// GetActiveTasks returns currently active task statuses.
func (c *CycleOrchestrator) GetActiveTasks() map[string]string {
	return c.monitor.GetAllActiveTasks()
}

// GetTaskReports returns progress reports for a specific task.
func (c *CycleOrchestrator) GetTaskReports(taskID string) []RSICProgressReport {
	return c.monitor.GetTaskReports(taskID)
}

func (c *CycleOrchestrator) tierTimeout(tier CycleTier) time.Duration {
	switch tier {
	case TierMicro:
		return 30 * time.Second
	case TierMeso:
		return 10 * time.Minute
	case TierMacro:
		return 30 * time.Minute
	default:
		return 10 * time.Minute
	}
}
