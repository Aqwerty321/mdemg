package ape

import (
	"context"
	"log"
	"sync"
	"time"

	"mdemg/internal/config"
)

// Watchdog monitors the time since the last RSIC cycle and escalates if overdue.
type Watchdog struct {
	cfg     config.Config
	spaceID string

	mu            sync.RWMutex
	state         WatchdogState
	cycleTrigger  func(ctx context.Context, spaceID string) // callback to auto-trigger meso cycle

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWatchdog creates a Watchdog. cycleTrigger is called at EscalationForce level.
func NewWatchdog(cfg config.Config, spaceID string, cycleTrigger func(ctx context.Context, spaceID string)) *Watchdog {
	ctx, cancel := context.WithCancel(context.Background())
	return &Watchdog{
		cfg:          cfg,
		spaceID:      spaceID,
		cycleTrigger: cycleTrigger,
		ctx:          ctx,
		cancel:       cancel,
		state: WatchdogState{
			SpaceID:       spaceID,
			LastCycleTime: time.Now(),
		},
	}
}

// Start begins the watchdog ticker loop.
func (w *Watchdog) Start() {
	if !w.cfg.RSICWatchdogEnabled {
		log.Printf("RSIC watchdog disabled")
		return
	}

	interval := time.Duration(w.cfg.RSICWatchdogCheckSec) * time.Second
	if interval < time.Second {
		interval = 5 * time.Minute
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("RSIC watchdog started (check every %s, decay rate %.2f/hr)", interval, w.cfg.RSICWatchdogDecayRate)

		for {
			select {
			case <-w.ctx.Done():
				return
			case <-ticker.C:
				w.check()
			}
		}
	}()
}

// Stop gracefully stops the watchdog.
func (w *Watchdog) Stop() {
	w.cancel()
	w.wg.Wait()
}

// RecordCycle resets the watchdog after a successful cycle.
func (w *Watchdog) RecordCycle() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state.LastCycleTime = time.Now()
	w.state.DecayScore = 0
	w.state.EscalationLevel = EscalationNominal
}

// GetState returns the current watchdog state.
func (w *Watchdog) GetState() WatchdogState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state
}

func (w *Watchdog) check() {
	w.mu.Lock()
	defer w.mu.Unlock()

	hoursSinceCycle := time.Since(w.state.LastCycleTime).Hours()
	w.state.DecayScore = hoursSinceCycle * w.cfg.RSICWatchdogDecayRate
	w.state.NextDue = w.state.LastCycleTime.Add(time.Duration(w.cfg.RSICMesoPeriodHours) * time.Hour)

	prevLevel := w.state.EscalationLevel

	switch {
	case w.state.DecayScore >= w.cfg.RSICForceThreshold:
		w.state.EscalationLevel = EscalationForce
	case w.state.DecayScore >= w.cfg.RSICWarnThreshold:
		w.state.EscalationLevel = EscalationWarn
	case w.state.DecayScore >= w.cfg.RSICNudgeThreshold:
		w.state.EscalationLevel = EscalationNudge
	default:
		w.state.EscalationLevel = EscalationNominal
	}

	// Log on escalation level changes
	if w.state.EscalationLevel != prevLevel {
		switch w.state.EscalationLevel {
		case EscalationNudge:
			log.Printf("RSIC watchdog: nudge — decay score %.2f (%.1f hours since last cycle)", w.state.DecayScore, hoursSinceCycle)
		case EscalationWarn:
			log.Printf("RSIC watchdog: WARNING — decay score %.2f (%.1f hours since last cycle)", w.state.DecayScore, hoursSinceCycle)
		case EscalationForce:
			log.Printf("RSIC watchdog: FORCE — auto-triggering meso cycle (decay score %.2f)", w.state.DecayScore)
		}
	}

	// Auto-trigger at force level
	if w.state.EscalationLevel == EscalationForce && w.cycleTrigger != nil {
		// Reset before triggering to avoid re-triggering
		w.state.LastCycleTime = time.Now()
		w.state.DecayScore = 0
		w.state.EscalationLevel = EscalationNominal

		go w.cycleTrigger(context.Background(), w.spaceID)
	}
}
