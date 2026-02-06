package retrieval

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/optimistic"
)

// VersionedUpdateResult contains the result of a versioned update operation.
type VersionedUpdateResult struct {
	// NodeID is the ID of the node that was updated.
	NodeID string

	// OldVersion is the version before the update.
	OldVersion int64

	// NewVersion is the version after the update.
	NewVersion int64

	// Matched indicates whether a node was found and updated.
	Matched bool
}

// UpdateNodeWithVersion updates a node only if its current version matches expectedVersion.
// Returns ErrVersionMismatch if the version doesn't match, allowing retry logic to handle conflicts.
//
// The update function should return a map of properties to SET on the node.
// The version will be automatically incremented on success.
func (s *Service) UpdateNodeWithVersion(
	ctx context.Context,
	spaceID, nodeID string,
	expectedVersion int64,
	updateFn func() map[string]any,
) (VersionedUpdateResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	updates := updateFn()
	if updates == nil {
		updates = make(map[string]any)
	}

	// Build dynamic SET clause from updates map
	setClause := "n.version = n.version + 1, n.updated_at = datetime()"
	params := map[string]any{
		"spaceId":         spaceID,
		"nodeId":          nodeID,
		"expectedVersion": expectedVersion,
	}

	// Add each update property to the SET clause
	for key, value := range updates {
		setClause += fmt.Sprintf(", n.%s = $update_%s", key, key)
		params["update_"+key] = value
	}

	cypher := fmt.Sprintf(`
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
WHERE n.version = $expectedVersion
SET %s
RETURN n.node_id AS node_id, n.version AS new_version, $expectedVersion AS old_version, 1 AS matched
`, setClause)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			nid, _ := rec.Get("node_id")
			newVer, _ := rec.Get("new_version")
			oldVer, _ := rec.Get("old_version")
			matched, _ := rec.Get("matched")

			return VersionedUpdateResult{
				NodeID:     fmt.Sprint(nid),
				OldVersion: toInt64(oldVer, 0),
				NewVersion: toInt64(newVer, 0),
				Matched:    toInt(matched, 0) == 1,
			}, nil
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		// No rows returned means version mismatch or node not found
		return VersionedUpdateResult{Matched: false}, nil
	})

	if err != nil {
		return VersionedUpdateResult{}, err
	}

	vur := result.(VersionedUpdateResult)

	if !vur.Matched {
		// Query actual version for error reporting
		actualVersion, _ := s.getNodeVersion(ctx, spaceID, nodeID)

		return vur, &optimistic.VersionMismatchError{
			EntityType: "node",
			EntityID:   nodeID,
			SpaceID:    spaceID,
			Expected:   expectedVersion,
			Actual:     actualVersion,
			Operation:  "update",
		}
	}

	return vur, nil
}

// getNodeVersion retrieves the current version of a node.
func (s *Service) getNodeVersion(ctx context.Context, spaceID, nodeID string) (int64, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
RETURN n.version AS version
`, map[string]any{"spaceId": spaceID, "nodeId": nodeID})
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			ver, _ := res.Record().Get("version")
			return toInt64(ver, 0), nil
		}
		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// EdgeVersionedUpdateResult contains the result of an edge update with version checking.
type EdgeVersionedUpdateResult struct {
	// EdgeID is the ID of the edge that was updated.
	EdgeID string

	// SrcNodeID is the source node of the edge.
	SrcNodeID string

	// DstNodeID is the destination node of the edge.
	DstNodeID string

	// OldVersion is the version before the update.
	OldVersion int64

	// NewVersion is the version after the update.
	NewVersion int64

	// Matched indicates whether an edge was found and updated.
	Matched bool
}

// UpdateEdgeWithVersion updates a CO_ACTIVATED_WITH edge only if its current version matches expectedVersion.
// Returns ErrVersionMismatch if the version doesn't match.
func (s *Service) UpdateEdgeWithVersion(
	ctx context.Context,
	spaceID, srcNodeID, dstNodeID string,
	expectedVersion int64,
	updateFn func() map[string]any,
) (EdgeVersionedUpdateResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	updates := updateFn()
	if updates == nil {
		updates = make(map[string]any)
	}

	// Build dynamic SET clause from updates map
	setClause := "r.version = r.version + 1, r.updated_at = datetime()"
	params := map[string]any{
		"spaceId":         spaceID,
		"srcNodeId":       srcNodeID,
		"dstNodeId":       dstNodeID,
		"expectedVersion": expectedVersion,
	}

	// Add each update property to the SET clause
	for key, value := range updates {
		setClause += fmt.Sprintf(", r.%s = $update_%s", key, key)
		params["update_"+key] = value
	}

	cypher := fmt.Sprintf(`
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $srcNodeId})
      -[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->
      (b:MemoryNode {space_id: $spaceId, node_id: $dstNodeId})
WHERE r.version = $expectedVersion
SET %s
RETURN r.edge_id AS edge_id, r.version AS new_version, $expectedVersion AS old_version, 1 AS matched
`, setClause)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			eid, _ := rec.Get("edge_id")
			newVer, _ := rec.Get("new_version")
			oldVer, _ := rec.Get("old_version")
			matched, _ := rec.Get("matched")

			return EdgeVersionedUpdateResult{
				EdgeID:     fmt.Sprint(eid),
				SrcNodeID:  srcNodeID,
				DstNodeID:  dstNodeID,
				OldVersion: toInt64(oldVer, 0),
				NewVersion: toInt64(newVer, 0),
				Matched:    toInt(matched, 0) == 1,
			}, nil
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		// No rows returned means version mismatch or edge not found
		return EdgeVersionedUpdateResult{Matched: false}, nil
	})

	if err != nil {
		return EdgeVersionedUpdateResult{}, err
	}

	eur := result.(EdgeVersionedUpdateResult)

	if !eur.Matched {
		// Query actual version for error reporting
		actualVersion, _ := s.getEdgeVersion(ctx, spaceID, srcNodeID, dstNodeID)

		return eur, &optimistic.VersionMismatchError{
			EntityType: "edge",
			EntityID:   fmt.Sprintf("%s->%s", srcNodeID, dstNodeID),
			SpaceID:    spaceID,
			Expected:   expectedVersion,
			Actual:     actualVersion,
			Operation:  "update",
		}
	}

	return eur, nil
}

// getEdgeVersion retrieves the current version of a CO_ACTIVATED_WITH edge.
func (s *Service) getEdgeVersion(ctx context.Context, spaceID, srcNodeID, dstNodeID string) (int64, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $srcNodeId})
      -[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->
      (b:MemoryNode {space_id: $spaceId, node_id: $dstNodeId})
RETURN r.version AS version
`, map[string]any{"spaceId": spaceID, "srcNodeId": srcNodeID, "dstNodeId": dstNodeID})
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			ver, _ := res.Record().Get("version")
			return toInt64(ver, 0), nil
		}
		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// UpdateAssociatedWithEdgeWithVersion updates an ASSOCIATED_WITH edge only if its version matches.
func (s *Service) UpdateAssociatedWithEdgeWithVersion(
	ctx context.Context,
	spaceID, srcNodeID, dstNodeID string,
	expectedVersion int64,
	updateFn func() map[string]any,
) (EdgeVersionedUpdateResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	updates := updateFn()
	if updates == nil {
		updates = make(map[string]any)
	}

	// Build dynamic SET clause from updates map
	setClause := "r.version = coalesce(r.version, 0) + 1, r.updated_at = datetime()"
	params := map[string]any{
		"spaceId":         spaceID,
		"srcNodeId":       srcNodeID,
		"dstNodeId":       dstNodeID,
		"expectedVersion": expectedVersion,
	}

	// Add each update property to the SET clause
	for key, value := range updates {
		setClause += fmt.Sprintf(", r.%s = $update_%s", key, key)
		params["update_"+key] = value
	}

	cypher := fmt.Sprintf(`
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $srcNodeId})
      -[r:ASSOCIATED_WITH {space_id: $spaceId}]->
      (b:MemoryNode {space_id: $spaceId, node_id: $dstNodeId})
WHERE coalesce(r.version, 0) = $expectedVersion
SET %s
RETURN coalesce(r.edge_id, '') AS edge_id, r.version AS new_version, $expectedVersion AS old_version, 1 AS matched
`, setClause)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			eid, _ := rec.Get("edge_id")
			newVer, _ := rec.Get("new_version")
			oldVer, _ := rec.Get("old_version")
			matched, _ := rec.Get("matched")

			return EdgeVersionedUpdateResult{
				EdgeID:     fmt.Sprint(eid),
				SrcNodeID:  srcNodeID,
				DstNodeID:  dstNodeID,
				OldVersion: toInt64(oldVer, 0),
				NewVersion: toInt64(newVer, 0),
				Matched:    toInt(matched, 0) == 1,
			}, nil
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		return EdgeVersionedUpdateResult{Matched: false}, nil
	})

	if err != nil {
		return EdgeVersionedUpdateResult{}, err
	}

	eur := result.(EdgeVersionedUpdateResult)

	if !eur.Matched {
		actualVersion, _ := s.getAssociatedWithEdgeVersion(ctx, spaceID, srcNodeID, dstNodeID)

		return eur, &optimistic.VersionMismatchError{
			EntityType: "edge",
			EntityID:   fmt.Sprintf("%s-ASSOCIATED_WITH->%s", srcNodeID, dstNodeID),
			SpaceID:    spaceID,
			Expected:   expectedVersion,
			Actual:     actualVersion,
			Operation:  "update",
		}
	}

	return eur, nil
}

// getAssociatedWithEdgeVersion retrieves the current version of an ASSOCIATED_WITH edge.
func (s *Service) getAssociatedWithEdgeVersion(ctx context.Context, spaceID, srcNodeID, dstNodeID string) (int64, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $srcNodeId})
      -[r:ASSOCIATED_WITH {space_id: $spaceId}]->
      (b:MemoryNode {space_id: $spaceId, node_id: $dstNodeId})
RETURN coalesce(r.version, 0) AS version
`, map[string]any{"spaceId": spaceID, "srcNodeId": srcNodeID, "dstNodeId": dstNodeID})
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			ver, _ := res.Record().Get("version")
			return toInt64(ver, 0), nil
		}
		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// toInt64 converts an interface{} to int64 with a default value.
func toInt64(v any, def int64) int64 {
	if v == nil {
		return def
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	default:
		return def
	}
}
