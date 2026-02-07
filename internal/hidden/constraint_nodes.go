package hidden

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ConstraintNodeResult tracks what happened during constraint node creation.
type ConstraintNodeResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Linked  int `json:"linked"`
}

// CreateConstraintNodes promotes constraint-tagged observations to first-class
// constraint nodes (role_type='constraint') and links them via IMPLEMENTS_CONSTRAINT edges.
// Called as part of RunConsolidation.
func (s *Service) CreateConstraintNodes(ctx context.Context, spaceID string) (*ConstraintNodeResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res := &ConstraintNodeResult{}

		// Step 1: Find constraint-tagged observations not yet linked to a constraint node
		findCypher := `
			MATCH (obs:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
			WHERE any(tag IN coalesce(obs.tags, []) WHERE tag STARTS WITH 'constraint:')
			  AND NOT (obs)-[:IMPLEMENTS_CONSTRAINT]->(:MemoryNode {role_type: 'constraint'})
			RETURN obs.node_id AS nodeId,
			       obs.name AS name,
			       obs.content AS content,
			       obs.tags AS tags,
			       obs.embedding AS embedding
		`
		findRes, err := tx.Run(ctx, findCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, fmt.Errorf("find constraint observations: %w", err)
		}

		type constraintObs struct {
			nodeID    string
			name      string
			content   string
			tags      []string
			embedding []float64
			cTypes    []string // extracted constraint types
		}

		var observations []constraintObs
		for findRes.Next(ctx) {
			rec := findRes.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			content, _ := rec.Get("content")
			tagsRaw, _ := rec.Get("tags")
			embRaw, _ := rec.Get("embedding")

			obs := constraintObs{
				nodeID: fmt.Sprintf("%v", nodeID),
			}
			if name != nil {
				obs.name = fmt.Sprintf("%v", name)
			}
			if content != nil {
				obs.content = fmt.Sprintf("%v", content)
			}

			// Extract tags
			if tagSlice, ok := tagsRaw.([]any); ok {
				for _, t := range tagSlice {
					tag := fmt.Sprintf("%v", t)
					obs.tags = append(obs.tags, tag)
					if strings.HasPrefix(tag, "constraint:") {
						obs.cTypes = append(obs.cTypes, strings.TrimPrefix(tag, "constraint:"))
					}
				}
			}

			// Extract embedding
			if embSlice, ok := embRaw.([]any); ok {
				for _, e := range embSlice {
					if f, ok := e.(float64); ok {
						obs.embedding = append(obs.embedding, f)
					}
				}
			}

			observations = append(observations, obs)
		}
		if err := findRes.Err(); err != nil {
			return nil, fmt.Errorf("iterate constraint observations: %w", err)
		}

		if len(observations) == 0 {
			return res, nil
		}

		// Step 2: Group by constraint type, then create/update constraint nodes
		for _, obs := range observations {
			for _, cType := range obs.cTypes {
				// Extract constraint name
				cName := obs.name
				if cName == "" {
					cName = extractConstraintLabel(obs.content)
				}
				if cName == "" {
					cName = fmt.Sprintf("%s constraint", cType)
				}

				// Check if matching constraint node exists
				matchCypher := `
					MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'constraint'})
					WHERE c.constraint_type = $cType
					  AND c.name = $name
					RETURN c.node_id AS nodeId
					LIMIT 1
				`
				matchRes, err := tx.Run(ctx, matchCypher, map[string]any{
					"spaceId": spaceID,
					"cType":   cType,
					"name":    cName,
				})
				if err != nil {
					return nil, fmt.Errorf("match constraint node: %w", err)
				}

				var constraintNodeID string
				if matchRes.Next(ctx) {
					nid, _ := matchRes.Record().Get("nodeId")
					constraintNodeID = fmt.Sprintf("%v", nid)
					// Update existing node timestamp
					updateCypher := `
						MATCH (c:MemoryNode {space_id: $spaceId, node_id: $nodeId})
						SET c.updated_at = datetime(),
						    c.reinforcement_count = coalesce(c.reinforcement_count, 0) + 1
					`
					if _, err := tx.Run(ctx, updateCypher, map[string]any{
						"spaceId": spaceID,
						"nodeId":  constraintNodeID,
					}); err != nil {
						return nil, fmt.Errorf("update constraint node: %w", err)
					}
					res.Updated++
				} else {
					// Create new constraint node
					constraintNodeID = uuid.New().String()
					now := time.Now().UTC().Format(time.RFC3339)

					createCypher := `
						CREATE (c:MemoryNode {
							space_id: $spaceId,
							node_id: $nodeId,
							role_type: 'constraint',
							name: $name,
							constraint_type: $cType,
							content: $content,
							layer: 1,
							confidence: $confidence,
							tags: $tags,
							created_at: datetime($now),
							updated_at: datetime($now),
							volatile: false,
							is_archived: false
						})
					`

					// Use obs embedding if available
					embParam := []float64(nil)
					if len(obs.embedding) > 0 {
						embParam = obs.embedding
					}

					params := map[string]any{
						"spaceId":    spaceID,
						"nodeId":     constraintNodeID,
						"name":       cName,
						"cType":      cType,
						"content":    obs.content,
						"confidence": 0.8,
						"tags":       []string{"constraint", "constraint:" + cType},
						"now":        now,
					}

					if len(embParam) > 0 {
						createCypher = `
							CREATE (c:MemoryNode {
								space_id: $spaceId,
								node_id: $nodeId,
								role_type: 'constraint',
								name: $name,
								constraint_type: $cType,
								content: $content,
								layer: 1,
								confidence: $confidence,
								tags: $tags,
								embedding: $embedding,
								created_at: datetime($now),
								updated_at: datetime($now),
								volatile: false,
								is_archived: false
							})
						`
						params["embedding"] = embParam
					}

					if _, err := tx.Run(ctx, createCypher, params); err != nil {
						return nil, fmt.Errorf("create constraint node: %w", err)
					}
					res.Created++
					log.Printf("Created constraint node %s (type=%s, name=%s)", constraintNodeID, cType, cName)
				}

				// Step 3: Link observation → constraint via IMPLEMENTS_CONSTRAINT
				linkCypher := `
					MATCH (obs:MemoryNode {space_id: $spaceId, node_id: $obsNodeId})
					MATCH (c:MemoryNode {space_id: $spaceId, node_id: $constraintNodeId})
					MERGE (obs)-[r:IMPLEMENTS_CONSTRAINT]->(c)
					ON CREATE SET r.created_at = datetime(), r.weight = 1.0
					ON MATCH SET r.weight = r.weight + 0.1, r.updated_at = datetime()
				`
				if _, err := tx.Run(ctx, linkCypher, map[string]any{
					"spaceId":          spaceID,
					"obsNodeId":        obs.nodeID,
					"constraintNodeId": constraintNodeID,
				}); err != nil {
					return nil, fmt.Errorf("link constraint edge: %w", err)
				}
				res.Linked++
			}
		}

		return res, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*ConstraintNodeResult), nil
}

// extractConstraintLabel gets a short label from content (first sentence, max 120 chars).
func extractConstraintLabel(content string) string {
	if content == "" {
		return ""
	}
	name := content
	if idx := strings.IndexAny(name, ".\n"); idx > 0 {
		name = name[:idx]
	}
	if len(name) > 120 {
		name = name[:120]
	}
	return strings.TrimSpace(name)
}
