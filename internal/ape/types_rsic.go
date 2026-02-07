package ape

import (
	"context"
	"time"
)

// ───────────── Tier ─────────────

// CycleTier represents the granularity of an RSIC cycle.
type CycleTier string

const (
	TierMicro CycleTier = "micro" // per-request opportunistic
	TierMeso  CycleTier = "meso"  // periodic (hours/sessions)
	TierMacro CycleTier = "macro" // cron-scheduled deep maintenance
)

// ───────────── Assessment ─────────────

// SelfAssessmentReport contains the output of the Assess stage.
type SelfAssessmentReport struct {
	SpaceID   string    `json:"space_id"`
	Tier      CycleTier `json:"tier"`
	Timestamp time.Time `json:"timestamp"`

	// Sub-scores (0–1, higher = healthier)
	RetrievalQuality float64 `json:"retrieval_quality"`
	TaskPerformance  float64 `json:"task_performance"`
	MemoryHealth     float64 `json:"memory_health"`
	EdgeHealth       float64 `json:"edge_health"`

	// Derived
	OverallHealth float64 `json:"overall_health"`
	Confidence    float64 `json:"confidence"`

	// Raw details exposed for reflection
	LearningPhase        string  `json:"learning_phase"`
	EdgeCount            int64   `json:"edge_count"`
	OrphanCount          int64   `json:"orphan_count"`
	TotalNodes           int64   `json:"total_nodes"`
	OrphanRatio          float64 `json:"orphan_ratio"`
	CorrectionRate       float64 `json:"correction_rate"`
	ConsolidationAgeSec  int64   `json:"consolidation_age_sec"`
	VolatileCount        int     `json:"volatile_count"`
	PermanentCount       int     `json:"permanent_count"`
	AvgEdgeWeight        float64 `json:"avg_edge_weight"`
	EdgesBelowThreshold  int64   `json:"edges_below_threshold"`
	EdgeWeightEntropy    float64 `json:"edge_weight_entropy"`
}

// ───────────── Reflection ─────────────

// InsightSeverity ranks the urgency of a reflection insight.
type InsightSeverity string

const (
	SeverityLow      InsightSeverity = "low"
	SeverityMedium   InsightSeverity = "medium"
	SeverityHigh     InsightSeverity = "high"
	SeverityCritical InsightSeverity = "critical"
)

// ReflectionInsight is a single observation produced by the Reflect stage.
type ReflectionInsight struct {
	PatternID         string          `json:"pattern_id"`
	Severity          InsightSeverity `json:"severity"`
	Description       string          `json:"description"`
	RecommendedAction string          `json:"recommended_action"` // action type key
	Metric            string          `json:"metric"`
	Value             float64         `json:"value"`
	Threshold         float64         `json:"threshold"`
}

// ───────────── Improvement Actions ─────────────

// ImprovementAction maps an insight to a concrete action.
type ImprovementAction struct {
	ActionType string    `json:"action_type"`
	TargetSpace string   `json:"target_space"`
	Scope       string   `json:"scope"` // "space" | "global"
	Priority    int      `json:"priority"`
	Rationale   string   `json:"rationale"`
}

// ───────────── Task Spec ─────────────

// EndpointSpec declares an API endpoint a task is allowed to call.
type EndpointSpec struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

// SafetyBounds constrains the blast radius of a task.
type SafetyBounds struct {
	MaxNodesAffected int      `json:"max_nodes_affected"`
	MaxEdgesAffected int      `json:"max_edges_affected"`
	ProtectedSpaces  []string `json:"protected_spaces"`
	DryRun           bool     `json:"dry_run"`
}

// Deliverable describes an expected output from a task.
type Deliverable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Format      string `json:"format"` // "json" | "text"
	Required    bool   `json:"required"`
}

// Criterion defines a success metric check.
type Criterion struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"` // "gte" | "lte" | "eq" | "gt" | "lt"
	Threshold float64 `json:"threshold"`
}

// ReportSchedule configures when progress reports are emitted.
type ReportSchedule struct {
	IntervalType string   `json:"interval_type"` // "time" | "progress" | "milestone"
	Values       []string `json:"values"`        // durations or milestone names
}

// RSICTaskSpec is the standardised specification for a single improvement task.
type RSICTaskSpec struct {
	// Identity
	TaskID  string `json:"task_id"`
	CycleID string `json:"cycle_id"`

	// Purpose
	ActionType  string `json:"action_type"`
	Description string `json:"description"`
	Rationale   string `json:"rationale"`

	// Scope
	TargetSpace string `json:"target_space"`

	// Permissions
	AllowedEndpoints []EndpointSpec `json:"allowed_endpoints"`
	Safety           SafetyBounds   `json:"safety"`

	// Deliverables
	Deliverables    []Deliverable  `json:"deliverables"`
	SuccessCriteria []Criterion    `json:"success_criteria"`

	// Reporting
	ReportSchedule ReportSchedule `json:"report_schedule"`

	// Execution limits
	Timeout         time.Duration  `json:"timeout"`
	BaselineMetrics map[string]float64 `json:"baseline_metrics"`
	Priority        int            `json:"priority"`
}

// ───────────── Progress & Outcome ─────────────

// RSICProgressReport is emitted by a running task at milestones.
type RSICProgressReport struct {
	TaskID       string             `json:"task_id"`
	CycleID      string             `json:"cycle_id"`
	Status       string             `json:"status"` // "running" | "completed" | "failed"
	ProgressPct  float64            `json:"progress_pct"`
	Milestone    string             `json:"milestone"`
	Summary      string             `json:"summary"`
	MetricsDelta map[string]float64 `json:"metrics_delta,omitempty"`
	Deliverables map[string]any     `json:"deliverables,omitempty"`
	Timestamp    time.Time          `json:"timestamp"`
	Error        string             `json:"error,omitempty"`
}

// CycleOutcome summarises a completed RSIC cycle.
type CycleOutcome struct {
	CycleID          string             `json:"cycle_id"`
	Tier             CycleTier          `json:"tier"`
	SpaceID          string             `json:"space_id"`
	StartedAt        time.Time          `json:"started_at"`
	CompletedAt      time.Time          `json:"completed_at"`
	ActionsExecuted  int                `json:"actions_executed"`
	SuccessCount     int                `json:"success_count"`
	FailedCount      int                `json:"failed_count"`
	MetricsBefore    map[string]float64 `json:"metrics_before"`
	MetricsAfter     map[string]float64 `json:"metrics_after"`
	CalibrationDelta map[string]float64 `json:"calibration_delta,omitempty"`
	Insights         []ReflectionInsight `json:"insights,omitempty"`
	Error            string             `json:"error,omitempty"`
}

// ───────────── Watchdog ─────────────

// EscalationLevel represents the watchdog's urgency state.
type EscalationLevel int

const (
	EscalationNominal EscalationLevel = 0 // all good
	EscalationNudge   EscalationLevel = 1 // gentle log
	EscalationWarn    EscalationLevel = 2 // warning log
	EscalationForce   EscalationLevel = 3 // auto-trigger
)

// WatchdogState tracks the decay watchdog's current state.
type WatchdogState struct {
	DecayScore      float64         `json:"decay_score"`
	EscalationLevel EscalationLevel `json:"escalation_level"`
	LastCycleTime   time.Time       `json:"last_cycle_time"`
	NextDue         time.Time       `json:"next_due"`
	SpaceID         string          `json:"space_id"`
}

// ───────────── Provider Interfaces ─────────────
// These interfaces decouple RSIC from concrete service implementations.

// LearningStatsProvider exposes learning-edge operations needed by RSIC.
type LearningStatsProvider interface {
	GetLearningEdgeStats(ctx context.Context, spaceID string) (map[string]any, error)
	PruneDecayedEdges(ctx context.Context, spaceID string) (int64, error)
	PruneExcessEdgesPerNode(ctx context.Context, spaceID string) (int64, error)
}

// ConversationStatsProvider exposes CMS stats and observation recording.
type ConversationStatsProvider interface {
	GetVolatileStats(ctx context.Context, spaceID string) (VolatileStatsResult, error)
}

// VolatileStatsResult mirrors the subset of conversation.VolatileStats we need.
type VolatileStatsResult struct {
	VolatileCount        int     `json:"volatile_count"`
	PermanentCount       int     `json:"permanent_count"`
	AvgVolatileStability float64 `json:"avg_volatile_stability"`
}

// HiddenLayerProvider exposes consolidation triggers.
type HiddenLayerProvider interface {
	RunFullConversationConsolidation(ctx context.Context, spaceID string) (any, error)
}
