package observations

import (
	"context"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/config"
	"mdemg/internal/domain"
)

type Service struct {
	cfg    config.Config
	driver neo4j.DriverWithContext
}

func NewService(cfg config.Config, driver neo4j.DriverWithContext) *Service {
	return &Service{cfg: cfg, driver: driver}
}

func (s *Service) UpsertNode(ctx context.Context, n domain.MemoryNode) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id": n.SpaceID,
			"node_id": n.NodeID,
			"path": n.Path,
			"name": n.Name,
			"layer": n.Layer,
			"role_type": n.RoleType,
			"description": n.Description,
			"summary": n.Summary,
			"confidence": n.Confidence,
			"sensitivity": n.Sensitivity,
			"tags": n.Tags,
			"embedding": n.Embedding,
		}
		// Keep writes minimal. Upsert only node properties.
		_, err := tx.Run(ctx, `
MERGE (m:MemoryNode {space_id:$space_id, node_id:$node_id})
ON CREATE SET
  m.path=$path,
  m.created_at=datetime(),
  m.updated_at=datetime(),
  m.update_count=1,
  m.version=1
ON MATCH SET
  m.updated_at=datetime(),
  m.update_count=coalesce(m.update_count,0)+1,
  m.version=coalesce(m.version,0)+1
SET
  m.name=$name,
  m.layer=$layer,
  m.role_type=$role_type,
  m.description=$description,
  m.summary=$summary,
  m.confidence=$confidence,
  m.sensitivity=$sensitivity,
  m.tags=$tags
WITH m
CALL {
  WITH m
  // Only set embedding if provided to avoid overwriting with empty.
  WITH m WHERE $embedding IS NOT NULL AND size($embedding) > 0
  SET m.embedding = $embedding
  RETURN 0 AS _
}
RETURN m.node_id
		`, params)
		return nil, err
	})
	return err
}

func (s *Service) AppendObservation(ctx context.Context, o domain.Observation, nodeIDs []string, nodePaths []string) error {
	if o.Timestamp.IsZero() {
		o.Timestamp = time.Now().UTC()
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id": o.SpaceID,
			"obs_id": o.ObsID,
			"ts": o.Timestamp,
			"source": o.Source,
			"content": o.Content,
			"embedding": o.Embedding,
			"node_ids": nodeIDs,
			"node_paths": nodePaths,
		}
		_, err := tx.Run(ctx, `
MERGE (obs:Observation {space_id:$space_id, obs_id:$obs_id})
ON CREATE SET
  obs.timestamp=$ts,
  obs.source=$source,
  obs.content=$content,
  obs.created_at=datetime()
ON MATCH SET
  obs.timestamp=$ts,
  obs.source=$source,
  obs.content=$content
WITH obs
CALL {
  WITH obs
  WITH obs WHERE $embedding IS NOT NULL AND size($embedding) > 0
  SET obs.embedding=$embedding
  RETURN 0 AS _
}
WITH obs
// Link by node_id
UNWIND coalesce($node_ids, []) AS nid
MATCH (n:MemoryNode {space_id:$space_id, node_id:nid})
MERGE (n)-[:HAS_OBSERVATION {space_id:$space_id, created_at=datetime(), status:'active', version:1}]->(obs)
WITH obs
// Link by path
UNWIND coalesce($node_paths, []) AS p
MATCH (n2:MemoryNode {space_id:$space_id, path:p})
MERGE (n2)-[:HAS_OBSERVATION {space_id:$space_id, created_at=datetime(), status:'active', version:1}]->(obs)
RETURN obs.obs_id
		`, params)
		return nil, err
	})
	return err
}
