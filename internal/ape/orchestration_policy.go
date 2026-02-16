package ape

import (
	"fmt"
	"sync"
	"time"

	"mdemg/internal/config"
)

// ───────────── Trigger Decision ─────────────

// TriggerDecision is the result of evaluating a trigger request.
type TriggerDecision struct {
	Allowed bool            `json:"allowed"`
	Reason  string          `json:"reason,omitempty"`
	Meta    TriggerMetadata `json:"meta"`
}

// DedupeResult is returned when a trigger is deduplicated.
type DedupeResult struct {
	IsDuplicate     bool   `json:"is_duplicate"`
	WindowSec       int    `json:"window_sec"`
	OriginalCycleID string `json:"original_cycle_id,omitempty"`
}

// ───────────── Internal Records ─────────────

type triggerRecord struct {
	source    TriggerSource
	spaceID   string
	tier      CycleTier
	cycleID   string
	timestamp time.Time
}

type sessionCounter struct {
	Count       int       `json:"count"`
	LastTrigger time.Time `json:"last_trigger"`
}

// ───────────── Orchestration Policy ─────────────

// OrchestrationPolicy enforces cooldown, dedupe, overlap prevention, and
// source-tier validation for RSIC cycle triggers.
type OrchestrationPolicy struct {
	mu sync.Mutex

	cooldownSec int
	dedupeSec   int
	cfg         config.Config

	// activeCycles tracks one active cycle per {spaceID, tier}
	activeCycles map[string]triggerRecord // key: "space:tier"

	// lastTrigger tracks the most recent trigger per source+space for cooldown
	lastTrigger map[string]triggerRecord // key: "source:space"

	// dedupeWindow tracks idempotency keys within the dedupe window
	dedupeWindow map[string]triggerRecord // key: idempotency_key

	// sessionCounters tracks session counts per space for meso periodic
	sessionCounters map[string]*sessionCounter // key: spaceID
}

// NewOrchestrationPolicy creates a new policy from config.
func NewOrchestrationPolicy(cfg config.Config) *OrchestrationPolicy {
	return &OrchestrationPolicy{
		cooldownSec:     cfg.RSICTriggerCooldownSec,
		dedupeSec:       cfg.RSICTriggerDedupeSec,
		cfg:             cfg,
		activeCycles:    make(map[string]triggerRecord),
		lastTrigger:     make(map[string]triggerRecord),
		dedupeWindow:    make(map[string]triggerRecord),
		sessionCounters: make(map[string]*sessionCounter),
	}
}

// EvaluateTrigger checks whether a trigger should be allowed.
func (p *OrchestrationPolicy) EvaluateTrigger(source TriggerSource, spaceID string, tier CycleTier, idempotencyKey string) TriggerDecision {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	meta := TriggerMetadata{
		TriggerSource:  source,
		TriggerID:      fmt.Sprintf("%s:%s:%s", source, spaceID, now.Format("2006-01-02T15:04")),
		TriggeredAt:    now,
		PolicyVersion:  PolicyVersion,
		IdempotencyKey: idempotencyKey,
	}

	// 1. Validate source-tier pairing
	if !p.isAllowedTier(source, tier) {
		return TriggerDecision{
			Allowed: false,
			Reason:  fmt.Sprintf("source %s cannot trigger tier %s", source, tier),
			Meta:    meta,
		}
	}

	// 2. Dedupe check (if idempotency key provided)
	if idempotencyKey != "" {
		if rec, ok := p.dedupeWindow[idempotencyKey]; ok {
			if now.Sub(rec.timestamp).Seconds() < float64(p.dedupeSec) {
				return TriggerDecision{
					Allowed: false,
					Reason:  fmt.Sprintf("duplicate trigger (key=%s, original_cycle=%s)", idempotencyKey, rec.cycleID),
					Meta:    meta,
				}
			}
		}
	}

	// 3. Overlap check — one active cycle per {space, tier}
	activeKey := fmt.Sprintf("%s:%s", spaceID, tier)
	if rec, ok := p.activeCycles[activeKey]; ok {
		// Auto-cleanup stale entries (30 min)
		if now.Sub(rec.timestamp) > 30*time.Minute {
			delete(p.activeCycles, activeKey)
		} else {
			return TriggerDecision{
				Allowed: false,
				Reason:  fmt.Sprintf("active cycle exists for space=%s tier=%s (cycle=%s)", spaceID, tier, rec.cycleID),
				Meta:    meta,
			}
		}
	}

	// 4. Cooldown check per source+space
	cooldownKey := fmt.Sprintf("%s:%s", source, spaceID)
	if rec, ok := p.lastTrigger[cooldownKey]; ok {
		if now.Sub(rec.timestamp).Seconds() < float64(p.cooldownSec) {
			return TriggerDecision{
				Allowed: false,
				Reason:  fmt.Sprintf("cooldown active for source=%s space=%s (%.0fs remaining)", source, spaceID, float64(p.cooldownSec)-now.Sub(rec.timestamp).Seconds()),
				Meta:    meta,
			}
		}
	}

	// 5. Priority check — if a higher-priority cycle is active for this space
	for key, rec := range p.activeCycles {
		// Check same space, any tier
		if rec.spaceID == spaceID && now.Sub(rec.timestamp) < 30*time.Minute {
			if rec.source.Priority() < source.Priority() {
				return TriggerDecision{
					Allowed: false,
					Reason:  fmt.Sprintf("higher-priority source %s active (key=%s)", rec.source, key),
					Meta:    meta,
				}
			}
		}
	}

	return TriggerDecision{
		Allowed: true,
		Meta:    meta,
	}
}

// RecordTrigger marks a cycle as active and records cooldown/dedupe state.
func (p *OrchestrationPolicy) RecordTrigger(meta TriggerMetadata, spaceID string, tier CycleTier, cycleID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	rec := triggerRecord{
		source:    meta.TriggerSource,
		spaceID:   spaceID,
		tier:      tier,
		cycleID:   cycleID,
		timestamp: meta.TriggeredAt,
	}

	// Mark active
	activeKey := fmt.Sprintf("%s:%s", spaceID, tier)
	p.activeCycles[activeKey] = rec

	// Record cooldown
	cooldownKey := fmt.Sprintf("%s:%s", meta.TriggerSource, spaceID)
	p.lastTrigger[cooldownKey] = rec

	// Record dedupe
	if meta.IdempotencyKey != "" {
		p.dedupeWindow[meta.IdempotencyKey] = rec
	}
}

// CompleteCycle removes the active marker for a cycle.
func (p *OrchestrationPolicy) CompleteCycle(spaceID string, tier CycleTier) {
	p.mu.Lock()
	defer p.mu.Unlock()

	activeKey := fmt.Sprintf("%s:%s", spaceID, tier)
	delete(p.activeCycles, activeKey)
}

// CheckDedupe returns a DedupeResult for a given idempotency key (fast path).
func (p *OrchestrationPolicy) CheckDedupe(idempotencyKey string) *DedupeResult {
	if idempotencyKey == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	rec, ok := p.dedupeWindow[idempotencyKey]
	if !ok {
		return nil
	}

	if time.Since(rec.timestamp).Seconds() >= float64(p.dedupeSec) {
		delete(p.dedupeWindow, idempotencyKey)
		return nil
	}

	return &DedupeResult{
		IsDuplicate:     true,
		WindowSec:       p.dedupeSec,
		OriginalCycleID: rec.cycleID,
	}
}

// IncrementSession increments the session counter for a space and returns
// whether a meso trigger should fire.
func (p *OrchestrationPolicy) IncrementSession(spaceID string) (count int, shouldTrigger bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sc, ok := p.sessionCounters[spaceID]
	if !ok {
		sc = &sessionCounter{}
		p.sessionCounters[spaceID] = sc
	}

	sc.Count++
	count = sc.Count

	period := p.cfg.RSICMesoPeriodSessions
	if period <= 0 {
		return count, false
	}

	if sc.Count%period == 0 {
		sc.LastTrigger = time.Now()
		return count, true
	}

	return count, false
}

// GetSessionCounters returns session counter state for the health payload.
func (p *OrchestrationPolicy) GetSessionCounters() []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()

	var result []map[string]any
	for spaceID, sc := range p.sessionCounters {
		entry := map[string]any{
			"space_id": spaceID,
			"count":    sc.Count,
		}
		period := p.cfg.RSICMesoPeriodSessions
		if period > 0 {
			nextAt := ((sc.Count / period) + 1) * period
			entry["next_meso_at"] = nextAt
		}
		result = append(result, entry)
	}
	return result
}

// GetLastTriggers returns the most recent triggers for the health payload.
func (p *OrchestrationPolicy) GetLastTriggers(limit int) []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()

	var result []map[string]any
	for _, rec := range p.lastTrigger {
		result = append(result, map[string]any{
			"space_id":       rec.spaceID,
			"tier":           rec.tier,
			"trigger_source": rec.source,
			"triggered_at":   rec.timestamp.UTC().Format(time.RFC3339),
		})
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// GetOrchestrationStatus returns the full orchestration block for the health endpoint.
func (p *OrchestrationPolicy) GetOrchestrationStatus(macroNextRun time.Time) map[string]any {
	status := map[string]any{
		"micro_enabled":        p.cfg.RSICMicroEnabled,
		"meso_period_sessions": p.cfg.RSICMesoPeriodSessions,
		"macro_cron":           p.cfg.RSICMacroCron,
		"cooldown_sec":         p.cooldownSec,
		"dedupe_sec":           p.dedupeSec,
		"last_triggers":        p.GetLastTriggers(10),
		"session_counters":     p.GetSessionCounters(),
	}

	scheduler := map[string]any{
		"enabled": p.cfg.RSICMacroCron != "",
	}
	if !macroNextRun.IsZero() {
		scheduler["macro_next_run"] = macroNextRun.UTC().Format(time.RFC3339)
	} else if p.cfg.RSICMacroCron != "" {
		scheduler["macro_next_run"] = "pending"
	} else {
		scheduler["macro_next_run"] = "disabled"
	}
	status["scheduler"] = scheduler

	return status
}

// CleanupExpired removes stale entries from all maps.
func (p *OrchestrationPolicy) CleanupExpired() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	// Clean stale active cycles (30 min timeout)
	for key, rec := range p.activeCycles {
		if now.Sub(rec.timestamp) > 30*time.Minute {
			delete(p.activeCycles, key)
		}
	}

	// Clean expired dedupe entries
	for key, rec := range p.dedupeWindow {
		if now.Sub(rec.timestamp).Seconds() >= float64(p.dedupeSec) {
			delete(p.dedupeWindow, key)
		}
	}

	// Clean old cooldown entries (keep only within 2x cooldown window)
	maxAge := time.Duration(p.cooldownSec*2) * time.Second
	for key, rec := range p.lastTrigger {
		if now.Sub(rec.timestamp) > maxAge {
			delete(p.lastTrigger, key)
		}
	}
}

// isAllowedTier checks if a source is allowed to trigger the given tier.
func (p *OrchestrationPolicy) isAllowedTier(source TriggerSource, tier CycleTier) bool {
	for _, t := range source.AllowedTiers() {
		if t == tier {
			return true
		}
	}
	return false
}
