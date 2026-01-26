package gaps

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Store handles persistence of capability gaps in Neo4j
type Store struct {
	db neo4j.DriverWithContext
}

// NewStore creates a new gap store
func NewStore(db neo4j.DriverWithContext) *Store {
	return &Store{db: db}
}

// SaveGap creates or updates a capability gap in Neo4j
func (s *Store) SaveGap(ctx context.Context, gap CapabilityGap) error {
	sess := s.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MERGE (g:CapabilityGap {gap_id: $gapId})
ON CREATE SET
    g.type = $type,
    g.description = $description,
    g.evidence = $evidence,
    g.plugin_type = $pluginType,
    g.plugin_name = $pluginName,
    g.plugin_description = $pluginDescription,
    g.plugin_capabilities = $pluginCapabilities,
    g.priority = $priority,
    g.detected_at = datetime($detectedAt),
    g.updated_at = datetime($updatedAt),
    g.status = $status,
    g.occurrence_count = $occurrenceCount,
    g.space_id = $spaceId
ON MATCH SET
    g.description = $description,
    g.evidence = $evidence,
    g.priority = $priority,
    g.updated_at = datetime($updatedAt),
    g.occurrence_count = $occurrenceCount
RETURN g.gap_id`

		params := map[string]any{
			"gapId":              gap.ID,
			"type":               string(gap.Type),
			"description":        gap.Description,
			"evidence":           gap.Evidence,
			"pluginType":         string(gap.SuggestedPlugin.Type),
			"pluginName":         gap.SuggestedPlugin.Name,
			"pluginDescription":  gap.SuggestedPlugin.Description,
			"pluginCapabilities": gap.SuggestedPlugin.Capabilities,
			"priority":           gap.Priority,
			"detectedAt":         gap.DetectedAt.Format(time.RFC3339),
			"updatedAt":          gap.UpdatedAt.Format(time.RFC3339),
			"status":             string(gap.Status),
			"occurrenceCount":    gap.OccurrenceCount,
			"spaceId":            nilIfEmpty(gap.SpaceID),
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		_, err = res.Consume(ctx)
		return nil, err
	})

	return err
}

// GetGap retrieves a capability gap by ID
func (s *Store) GetGap(ctx context.Context, gapID string) (*CapabilityGap, error) {
	sess := s.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (g:CapabilityGap {gap_id: $gapId})
RETURN g.gap_id AS gap_id,
       g.type AS type,
       g.description AS description,
       g.evidence AS evidence,
       g.plugin_type AS plugin_type,
       g.plugin_name AS plugin_name,
       g.plugin_description AS plugin_description,
       g.plugin_capabilities AS plugin_capabilities,
       g.priority AS priority,
       g.detected_at AS detected_at,
       g.updated_at AS updated_at,
       g.status AS status,
       g.occurrence_count AS occurrence_count,
       g.space_id AS space_id`

		res, err := tx.Run(ctx, cypher, map[string]any{"gapId": gapID})
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			return recordToGap(res.Record()), nil
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	gap := result.(*CapabilityGap)
	return gap, nil
}

// ListGaps lists capability gaps with optional filtering
func (s *Store) ListGaps(ctx context.Context, status GapStatus, gapType string) ([]CapabilityGap, error) {
	sess := s.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (g:CapabilityGap)
WHERE ($status = '' OR g.status = $status)
  AND ($type = '' OR g.type = $type)
RETURN g.gap_id AS gap_id,
       g.type AS type,
       g.description AS description,
       g.evidence AS evidence,
       g.plugin_type AS plugin_type,
       g.plugin_name AS plugin_name,
       g.plugin_description AS plugin_description,
       g.plugin_capabilities AS plugin_capabilities,
       g.priority AS priority,
       g.detected_at AS detected_at,
       g.updated_at AS updated_at,
       g.status AS status,
       g.occurrence_count AS occurrence_count,
       g.space_id AS space_id
ORDER BY g.priority DESC, g.updated_at DESC`

		params := map[string]any{
			"status": string(status),
			"type":   gapType,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var gaps []CapabilityGap
		for res.Next(ctx) {
			gap := recordToGap(res.Record())
			if gap != nil {
				gaps = append(gaps, *gap)
			}
		}
		return gaps, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return []CapabilityGap{}, nil
	}
	return result.([]CapabilityGap), nil
}

// FindGapByPattern finds an existing gap that matches a pattern
func (s *Store) FindGapByPattern(ctx context.Context, pattern string) (*CapabilityGap, error) {
	sess := s.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Search for gaps where evidence contains the pattern
		cypher := `
MATCH (g:CapabilityGap)
WHERE any(e IN g.evidence WHERE e CONTAINS $pattern)
   OR g.description CONTAINS $pattern
RETURN g.gap_id AS gap_id,
       g.type AS type,
       g.description AS description,
       g.evidence AS evidence,
       g.plugin_type AS plugin_type,
       g.plugin_name AS plugin_name,
       g.plugin_description AS plugin_description,
       g.plugin_capabilities AS plugin_capabilities,
       g.priority AS priority,
       g.detected_at AS detected_at,
       g.updated_at AS updated_at,
       g.status AS status,
       g.occurrence_count AS occurrence_count,
       g.space_id AS space_id
ORDER BY g.priority DESC
LIMIT 1`

		res, err := tx.Run(ctx, cypher, map[string]any{"pattern": pattern})
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			return recordToGap(res.Record()), nil
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	gap := result.(*CapabilityGap)
	return gap, nil
}

// UpdateGapStatus updates the status of a capability gap
func (s *Store) UpdateGapStatus(ctx context.Context, gapID string, status GapStatus) error {
	sess := s.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (g:CapabilityGap {gap_id: $gapId})
SET g.status = $status,
    g.updated_at = datetime()
RETURN g.gap_id`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"gapId":  gapID,
			"status": string(status),
		})
		if err != nil {
			return nil, err
		}
		_, err = res.Consume(ctx)
		return nil, err
	})

	return err
}

// DeleteGap permanently removes a capability gap
func (s *Store) DeleteGap(ctx context.Context, gapID string) error {
	sess := s.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (g:CapabilityGap {gap_id: $gapId})
DELETE g`

		res, err := tx.Run(ctx, cypher, map[string]any{"gapId": gapID})
		if err != nil {
			return nil, err
		}
		_, err = res.Consume(ctx)
		return nil, err
	})

	return err
}

// GetGapStats returns statistics about capability gaps
func (s *Store) GetGapStats(ctx context.Context) (map[string]any, error) {
	sess := s.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (g:CapabilityGap)
WITH count(g) AS total,
     sum(CASE WHEN g.status = 'open' THEN 1 ELSE 0 END) AS open_count,
     sum(CASE WHEN g.status = 'addressed' THEN 1 ELSE 0 END) AS addressed_count,
     sum(CASE WHEN g.status = 'dismissed' THEN 1 ELSE 0 END) AS dismissed_count,
     sum(CASE WHEN g.priority > 0.7 THEN 1 ELSE 0 END) AS high_priority_count,
     avg(g.priority) AS avg_priority
RETURN total, open_count, addressed_count, dismissed_count, high_priority_count, avg_priority`

		res, err := tx.Run(ctx, cypher, nil)
		if err != nil {
			return nil, err
		}

		stats := map[string]any{
			"total":               int64(0),
			"open_count":          int64(0),
			"addressed_count":     int64(0),
			"dismissed_count":     int64(0),
			"high_priority_count": int64(0),
			"avg_priority":        float64(0),
		}

		if res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("total"); ok && v != nil {
				stats["total"] = toInt64(v)
			}
			if v, ok := rec.Get("open_count"); ok && v != nil {
				stats["open_count"] = toInt64(v)
			}
			if v, ok := rec.Get("addressed_count"); ok && v != nil {
				stats["addressed_count"] = toInt64(v)
			}
			if v, ok := rec.Get("dismissed_count"); ok && v != nil {
				stats["dismissed_count"] = toInt64(v)
			}
			if v, ok := rec.Get("high_priority_count"); ok && v != nil {
				stats["high_priority_count"] = toInt64(v)
			}
			if v, ok := rec.Get("avg_priority"); ok && v != nil {
				stats["avg_priority"] = toFloat64(v)
			}
		}
		return stats, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}

// Helper functions

func recordToGap(rec *neo4j.Record) *CapabilityGap {
	if rec == nil {
		return nil
	}

	gap := &CapabilityGap{}

	if v, ok := rec.Get("gap_id"); ok && v != nil {
		gap.ID = fmt.Sprint(v)
	}
	if v, ok := rec.Get("type"); ok && v != nil {
		gap.Type = GapType(fmt.Sprint(v))
	}
	if v, ok := rec.Get("description"); ok && v != nil {
		gap.Description = fmt.Sprint(v)
	}
	if v, ok := rec.Get("evidence"); ok && v != nil {
		gap.Evidence = toStringSlice(v)
	}
	if v, ok := rec.Get("status"); ok && v != nil {
		gap.Status = GapStatus(fmt.Sprint(v))
	}
	if v, ok := rec.Get("priority"); ok && v != nil {
		gap.Priority = toFloat64(v)
	}
	if v, ok := rec.Get("occurrence_count"); ok && v != nil {
		gap.OccurrenceCount = int(toInt64(v))
	}
	if v, ok := rec.Get("space_id"); ok && v != nil {
		gap.SpaceID = fmt.Sprint(v)
	}
	if v, ok := rec.Get("detected_at"); ok && v != nil {
		gap.DetectedAt = toTime(v)
	}
	if v, ok := rec.Get("updated_at"); ok && v != nil {
		gap.UpdatedAt = toTime(v)
	}

	// Plugin suggestion
	if v, ok := rec.Get("plugin_type"); ok && v != nil {
		gap.SuggestedPlugin.Type = PluginType(fmt.Sprint(v))
	}
	if v, ok := rec.Get("plugin_name"); ok && v != nil {
		gap.SuggestedPlugin.Name = fmt.Sprint(v)
	}
	if v, ok := rec.Get("plugin_description"); ok && v != nil {
		gap.SuggestedPlugin.Description = fmt.Sprint(v)
	}
	if v, ok := rec.Get("plugin_capabilities"); ok && v != nil {
		gap.SuggestedPlugin.Capabilities = toStringSlice(v)
	}

	return gap
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	default:
		return 0
	}
}

func toFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return 0
	}
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		result := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func toTime(v any) time.Time {
	switch x := v.(type) {
	case time.Time:
		return x
	case neo4j.LocalDateTime:
		return x.Time()
	case string:
		t, _ := time.Parse(time.RFC3339, x)
		return t
	default:
		return time.Time{}
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
