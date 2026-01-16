//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// IngestRequest mirrors the API request structure for tests.
type IngestRequest struct {
	SpaceID     string    `json:"space_id"`
	Timestamp   string    `json:"timestamp"`
	Source      string    `json:"source"`
	Content     any       `json:"content"`
	Tags        []string  `json:"tags,omitempty"`
	NodeID      string    `json:"node_id,omitempty"`
	Path        string    `json:"path,omitempty"`
	Name        string    `json:"name,omitempty"`
	Sensitivity string    `json:"sensitivity,omitempty"`
	Confidence  *float64  `json:"confidence,omitempty"`
	Embedding   []float32 `json:"embedding,omitempty"`
}

// IngestResponse mirrors the API response structure for tests.
type IngestResponse struct {
	SpaceID       string `json:"space_id"`
	NodeID        string `json:"node_id"`
	ObsID         string `json:"obs_id"`
	EmbeddingDims int    `json:"embedding_dims,omitempty"`
}

// TestIngestCreatesNode verifies that POST to /v1/memory/ingest creates a Neo4j node with correct properties.
func TestIngestCreatesNode(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for verification
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("ingest")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Prepare ingest request with all fields
	confidence := 0.85
	embedding := CreateTestEmbedding(DefaultEmbeddingDims, 1.0)

	req := IngestRequest{
		SpaceID:     spaceID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "test-source",
		Content:     "This is test content for ingestion",
		Tags:        []string{"test", "integration"},
		Name:        "test-node-name",
		Path:        "/test/path/node",
		Sensitivity: "internal",
		Confidence:  &confidence,
		Embedding:   embedding,
	}

	// Make ingest request
	reqBody, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
	httpReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify HTTP response status
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Fatalf("ingest returned status %d: %v", resp.StatusCode, errResp)
	}

	// Parse response
	var ingestResp IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&ingestResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify response fields
	if ingestResp.SpaceID != spaceID {
		t.Errorf("response space_id mismatch: got %q, want %q", ingestResp.SpaceID, spaceID)
	}
	if ingestResp.NodeID == "" {
		t.Error("response node_id is empty")
	}
	if ingestResp.ObsID == "" {
		t.Error("response obs_id is empty")
	}
	if ingestResp.EmbeddingDims != DefaultEmbeddingDims {
		t.Errorf("response embedding_dims mismatch: got %d, want %d", ingestResp.EmbeddingDims, DefaultEmbeddingDims)
	}

	// Verify node was created in Neo4j with correct properties
	verifyNodeInNeo4j(t, driver, spaceID, ingestResp.NodeID, req)

	// Verify observation was created and linked
	verifyObservationInNeo4j(t, driver, spaceID, ingestResp.NodeID, ingestResp.ObsID, req)
}

// verifyNodeInNeo4j checks that the MemoryNode was created with expected properties.
func verifyNodeInNeo4j(t *testing.T, driver neo4j.DriverWithContext, spaceID, nodeID string, req IngestRequest) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
			RETURN n.space_id AS space_id,
			       n.node_id AS node_id,
			       n.path AS path,
			       n.name AS name,
			       n.layer AS layer,
			       n.role_type AS role_type,
			       n.status AS status,
			       n.confidence AS confidence,
			       n.sensitivity AS sensitivity,
			       n.tags AS tags,
			       n.embedding AS embedding,
			       n.created_at AS created_at,
			       n.updated_at AS updated_at,
			       n.update_count AS update_count
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if err != nil {
			return nil, err
		}

		if !res.Next(ctx) {
			return nil, fmt.Errorf("node not found: space_id=%s, node_id=%s", spaceID, nodeID)
		}

		return res.Record().AsMap(), res.Err()
	})
	if err != nil {
		t.Fatalf("failed to query node from Neo4j: %v", err)
	}

	nodeProps := result.(map[string]any)

	// Verify all expected properties
	assertEqual(t, "space_id", spaceID, nodeProps["space_id"])
	assertEqual(t, "node_id", nodeID, nodeProps["node_id"])
	assertEqual(t, "path", req.Path, nodeProps["path"])
	assertEqual(t, "name", req.Name, nodeProps["name"])
	assertEqual(t, "layer", int64(0), nodeProps["layer"])
	assertEqual(t, "role_type", "leaf", nodeProps["role_type"])
	assertEqual(t, "status", "active", nodeProps["status"])
	assertEqual(t, "sensitivity", req.Sensitivity, nodeProps["sensitivity"])

	// Verify confidence (with float tolerance)
	if conf, ok := nodeProps["confidence"].(float64); ok {
		if conf < *req.Confidence-0.01 || conf > *req.Confidence+0.01 {
			t.Errorf("confidence mismatch: got %f, want %f", conf, *req.Confidence)
		}
	} else {
		t.Errorf("confidence has unexpected type: %T", nodeProps["confidence"])
	}

	// Verify tags
	if tags, ok := nodeProps["tags"].([]any); ok {
		if len(tags) != len(req.Tags) {
			t.Errorf("tags length mismatch: got %d, want %d", len(tags), len(req.Tags))
		} else {
			for i, tag := range tags {
				if tagStr, ok := tag.(string); ok {
					if tagStr != req.Tags[i] {
						t.Errorf("tag[%d] mismatch: got %q, want %q", i, tagStr, req.Tags[i])
					}
				}
			}
		}
	} else if nodeProps["tags"] != nil {
		t.Errorf("tags has unexpected type: %T", nodeProps["tags"])
	}

	// Verify embedding was stored
	if embedding, ok := nodeProps["embedding"].([]any); ok {
		if len(embedding) != len(req.Embedding) {
			t.Errorf("embedding length mismatch: got %d, want %d", len(embedding), len(req.Embedding))
		}
	} else if nodeProps["embedding"] != nil {
		t.Errorf("embedding has unexpected type: %T", nodeProps["embedding"])
	}

	// Verify timestamps exist
	if nodeProps["created_at"] == nil {
		t.Error("created_at is nil")
	}
	if nodeProps["updated_at"] == nil {
		t.Error("updated_at is nil")
	}

	// Verify update_count is set
	if updateCount, ok := nodeProps["update_count"].(int64); ok {
		if updateCount < 1 {
			t.Errorf("update_count should be >= 1, got %d", updateCount)
		}
	} else {
		t.Errorf("update_count has unexpected type: %T", nodeProps["update_count"])
	}
}

// verifyObservationInNeo4j checks that the Observation was created and linked to the MemoryNode.
func verifyObservationInNeo4j(t *testing.T, driver neo4j.DriverWithContext, spaceID, nodeID, obsID string, req IngestRequest) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})-[:HAS_OBSERVATION]->(o:Observation {obs_id: $obsId})
			RETURN o.space_id AS space_id,
			       o.obs_id AS obs_id,
			       o.source AS source,
			       o.content AS content,
			       o.timestamp AS timestamp,
			       o.created_at AS created_at
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
			"obsId":   obsID,
		})
		if err != nil {
			return nil, err
		}

		if !res.Next(ctx) {
			return nil, fmt.Errorf("observation not found or not linked: node_id=%s, obs_id=%s", nodeID, obsID)
		}

		return res.Record().AsMap(), res.Err()
	})
	if err != nil {
		t.Fatalf("failed to query observation from Neo4j: %v", err)
	}

	obsProps := result.(map[string]any)

	// Verify observation properties
	assertEqual(t, "obs space_id", spaceID, obsProps["space_id"])
	assertEqual(t, "obs obs_id", obsID, obsProps["obs_id"])
	assertEqual(t, "obs source", req.Source, obsProps["source"])
	assertEqual(t, "obs content", req.Content, obsProps["content"])

	if obsProps["timestamp"] == nil {
		t.Error("observation timestamp is nil")
	}
	if obsProps["created_at"] == nil {
		t.Error("observation created_at is nil")
	}
}

// assertEqual is a helper for comparing values with helpful error messages.
func assertEqual(t *testing.T, field string, expected, actual any) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s mismatch: got %v (%T), want %v (%T)", field, actual, actual, expected, expected)
	}
}
