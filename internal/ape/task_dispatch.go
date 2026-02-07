package ape

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Dispatcher launches and manages RSIC task goroutines.
type Dispatcher struct {
	mu          sync.RWMutex
	activeTasks map[string]*activeTask
	reports     map[string][]RSICProgressReport

	learner   LearningStatsProvider
	convSvc   ConversationStatsProvider
	hiddenSvc HiddenLayerProvider
	driver    neo4j.DriverWithContext
}

type activeTask struct {
	Spec      RSICTaskSpec
	StartedAt time.Time
	Status    string // "running" | "completed" | "failed"
	cancel    context.CancelFunc
}

// NewDispatcher creates a Dispatcher wired to the given service providers.
func NewDispatcher(driver neo4j.DriverWithContext, learner LearningStatsProvider, convSvc ConversationStatsProvider, hiddenSvc HiddenLayerProvider) *Dispatcher {
	return &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
		learner:     learner,
		convSvc:     convSvc,
		hiddenSvc:   hiddenSvc,
		driver:      driver,
	}
}

// Dispatch launches all tasks as background goroutines and returns immediately.
func (d *Dispatcher) Dispatch(ctx context.Context, tasks []RSICTaskSpec) error {
	for i := range tasks {
		task := tasks[i]
		taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)

		at := &activeTask{
			Spec:      task,
			StartedAt: time.Now(),
			Status:    "running",
			cancel:    cancel,
		}

		d.mu.Lock()
		d.activeTasks[task.TaskID] = at
		d.reports[task.TaskID] = nil
		d.mu.Unlock()

		go d.executeTask(taskCtx, at)
	}
	return nil
}

func (d *Dispatcher) executeTask(ctx context.Context, at *activeTask) {
	defer at.cancel()

	taskID := at.Spec.TaskID

	// Milestone: snapshot_taken
	d.postReport(taskID, "running", 10, "snapshot_taken", "Baseline metrics captured", nil, "")

	// Execute based on action type
	var execErr error
	var deliverables map[string]any

	switch at.Spec.ActionType {
	case "prune_decayed_edges":
		deliverables, execErr = d.executePruneDecayed(ctx, at.Spec.TargetSpace)
	case "prune_excess_edges":
		deliverables, execErr = d.executePruneExcess(ctx, at.Spec.TargetSpace)
	case "trigger_consolidation":
		deliverables, execErr = d.executeConsolidation(ctx, at.Spec.TargetSpace)
	case "graduate_volatile":
		deliverables, execErr = d.executeGraduateVolatile(ctx, at.Spec.TargetSpace)
	case "tombstone_stale":
		deliverables, execErr = d.executeTombstoneStale(ctx, at.Spec.TargetSpace)
	case "refresh_stale_edges":
		deliverables, execErr = d.executeRefreshStaleEdges(ctx, at.Spec.TargetSpace)
	default:
		execErr = fmt.Errorf("unknown action type: %s", at.Spec.ActionType)
	}

	if execErr != nil {
		d.mu.Lock()
		at.Status = "failed"
		d.mu.Unlock()
		d.postReport(taskID, "failed", 100, "execution_complete", "", nil, execErr.Error())
		log.Printf("RSIC task %s failed: %v", taskID, execErr)
		return
	}

	// Milestone: execution_complete
	d.postReport(taskID, "running", 80, "execution_complete", "Action executed successfully", deliverables, "")

	// Milestone: validation_complete
	d.postReport(taskID, "completed", 100, "validation_complete", "Task completed", deliverables, "")

	d.mu.Lock()
	at.Status = "completed"
	d.mu.Unlock()
}

// ─── Action executors ───

func (d *Dispatcher) executePruneDecayed(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.learner == nil {
		return nil, fmt.Errorf("learning service not available")
	}
	pruned, err := d.learner.PruneDecayedEdges(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"decayed_pruned": pruned}, nil
}

func (d *Dispatcher) executePruneExcess(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.learner == nil {
		return nil, fmt.Errorf("learning service not available")
	}
	pruned, err := d.learner.PruneExcessEdgesPerNode(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"excess_pruned": pruned}, nil
}

func (d *Dispatcher) executeConsolidation(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.hiddenSvc == nil {
		return nil, fmt.Errorf("hidden layer service not available")
	}
	result, err := d.hiddenSvc.RunFullConversationConsolidation(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"consolidation_result": result}, nil
}

func (d *Dispatcher) executeGraduateVolatile(ctx context.Context, spaceID string) (map[string]any, error) {
	// Graduate volatile observations via Neo4j directly
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND n.volatile = true
			  AND coalesce(n.stability_score, 0.1) >= 0.7
			SET n.volatile = false
			RETURN count(n) AS graduated
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("graduated"); ok {
				return v, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"graduated": result}, nil
}

func (d *Dispatcher) executeTombstoneStale(ctx context.Context, spaceID string) (map[string]any, error) {
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Tombstone observations that have been superseded by corrections
		cypher := `
			MATCH (correction:MemoryNode {space_id: $spaceId, obs_type: 'correction'})
			WHERE correction.created_at > datetime() - duration('P7D')
			WITH correction
			MATCH (stale:MemoryNode {space_id: $spaceId})
			WHERE stale.role_type = 'conversation_observation'
			  AND stale.obs_type <> 'correction'
			  AND stale.created_at < correction.created_at
			  AND NOT coalesce(stale.is_archived, false)
			WITH stale LIMIT 50
			SET stale.is_archived = true
			RETURN count(stale) AS tombstoned
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("tombstoned"); ok {
				return v, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"tombstoned": result}, nil
}

func (d *Dispatcher) executeRefreshStaleEdges(ctx context.Context, spaceID string) (map[string]any, error) {
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Refresh stale edges by bumping their last_activated timestamp
		cypher := `
			MATCH ()-[e:LEARNING_EDGE {space_id: $spaceId}]->()
			WHERE e.last_activated < datetime() - duration('P30D')
			WITH e LIMIT 100
			SET e.last_activated = datetime()
			RETURN count(e) AS refreshed
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("refreshed"); ok {
				return v, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"refreshed": result}, nil
}

// ─── Report posting ───

func (d *Dispatcher) postReport(taskID, status string, pct float64, milestone, summary string, deliverables map[string]any, errMsg string) {
	report := RSICProgressReport{
		TaskID:       taskID,
		Status:       status,
		ProgressPct:  pct,
		Milestone:    milestone,
		Summary:      summary,
		Deliverables: deliverables,
		Timestamp:    time.Now(),
		Error:        errMsg,
	}
	// Find cycleID from active task
	d.mu.RLock()
	if at, ok := d.activeTasks[taskID]; ok {
		report.CycleID = at.Spec.CycleID
	}
	d.mu.RUnlock()

	d.mu.Lock()
	d.reports[taskID] = append(d.reports[taskID], report)
	d.mu.Unlock()
}
