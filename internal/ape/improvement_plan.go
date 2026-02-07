package ape

import (
	"context"
	"sort"

	"mdemg/internal/config"
)

// Planner maps reflection insights to concrete task specs.
type Planner struct {
	cfg config.Config
}

// NewPlanner creates a Planner.
func NewPlanner(cfg config.Config) *Planner {
	return &Planner{cfg: cfg}
}

// Plan converts insights into prioritised RSICTaskSpecs, filtering by confidence.
func (p *Planner) Plan(_ context.Context, insights []ReflectionInsight, spaceID string, baseline map[string]float64) ([]RSICTaskSpec, error) {
	if len(insights) == 0 {
		return nil, nil
	}

	// Deduplicate actions by type (keep highest-severity version)
	actionMap := make(map[string]ImprovementAction)
	for _, insight := range insights {
		action := ImprovementAction{
			ActionType:  insight.RecommendedAction,
			TargetSpace: spaceID,
			Scope:       "space",
			Priority:    severityRank(insight.Severity),
			Rationale:   insight.Description,
		}
		if existing, ok := actionMap[action.ActionType]; !ok || action.Priority > existing.Priority {
			actionMap[action.ActionType] = action
		}
	}

	// Build task specs
	var specs []RSICTaskSpec
	cycleID := "" // will be set by CycleOrchestrator before dispatch
	for _, action := range actionMap {
		spec := BuildTaskSpec(p.cfg, action, cycleID, baseline)
		specs = append(specs, *spec)
	}

	// Sort by priority DESC
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Priority > specs[j].Priority
	})

	return specs, nil
}
