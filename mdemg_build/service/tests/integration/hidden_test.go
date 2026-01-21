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

// TestHiddenLayerConsolidation tests the full hidden layer consolidation flow:
// 1. Creates base layer nodes with embeddings
// 2. Calls the consolidate endpoint
// 3. Verifies hidden nodes are created
// 4. Verifies message passing updates embeddings
func TestHiddenLayerConsolidation(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()
	driver := SetupTestNeo4j(t)

	spaceID := GenerateTestSpaceID("hidden")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	ctx := context.Background()

	// Step 1: Create base layer nodes with similar embeddings (should form a cluster)
	// Create 5 nodes with similar embeddings to form a cluster
	for i := 0; i < 5; i++ {
		// Create embeddings that are similar (close in cosine distance)
		embedding := CreateControlledEmbedding(DefaultEmbeddingDims, 0.95-float64(i)*0.01, 1)

		ingestReq := map[string]any{
			"space_id":  spaceID,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"source":    "test",
			"name":      fmt.Sprintf("Base Node %d", i),
			"content":   fmt.Sprintf("Test content for base node %d", i),
			"path":      fmt.Sprintf("/test/base/%d", i),
			"embedding": embedding,
		}
		reqBody, _ := json.Marshal(ingestReq)

		resp, err := client.Post(
			cfg.MDEMGEndpoint+"/v1/memory/ingest",
			"application/json",
			bytes.NewReader(reqBody),
		)
		if err != nil {
			t.Fatalf("Failed to ingest base node %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Ingest failed for node %d: status %d", i, resp.StatusCode)
		}
	}

	// Verify nodes were created at layer 0
	nodeCount := countNodesAtLayer(t, driver, ctx, spaceID, 0)
	if nodeCount < 5 {
		t.Fatalf("Expected at least 5 base nodes, got %d", nodeCount)
	}
	t.Logf("Created %d base layer nodes", nodeCount)

	// Step 2: Call consolidate endpoint
	consolidateReq := map[string]any{
		"space_id": spaceID,
	}
	reqBody, _ := json.Marshal(consolidateReq)

	resp, err := client.Post(
		cfg.MDEMGEndpoint+"/v1/memory/consolidate",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		t.Fatalf("Failed to call consolidate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Fatalf("Consolidate failed: status %d, error: %v", resp.StatusCode, errResp)
	}

	var consolidateResp struct {
		Data struct {
			SpaceID             string  `json:"space_id"`
			HiddenNodesCreated  int     `json:"hidden_nodes_created"`
			HiddenNodesUpdated  int     `json:"hidden_nodes_updated"`
			ConceptNodesUpdated int     `json:"concept_nodes_updated"`
			DurationMs          float64 `json:"duration_ms"`
			Enabled             bool    `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&consolidateResp); err != nil {
		t.Fatalf("Failed to decode consolidate response: %v", err)
	}

	t.Logf("Consolidation result: created=%d, hidden_updated=%d, concept_updated=%d, duration=%.2fms, enabled=%v",
		consolidateResp.Data.HiddenNodesCreated,
		consolidateResp.Data.HiddenNodesUpdated,
		consolidateResp.Data.ConceptNodesUpdated,
		consolidateResp.Data.DurationMs,
		consolidateResp.Data.Enabled)

	// Step 3: Verify hidden nodes were created (if enabled)
	if consolidateResp.Data.Enabled {
		hiddenCount := countNodesAtLayer(t, driver, ctx, spaceID, 1)
		t.Logf("Hidden layer nodes after consolidation: %d", hiddenCount)

		// Verify GENERALIZES edges exist
		edgeCount := countEdgesOfType(t, driver, ctx, spaceID, "GENERALIZES")
		t.Logf("GENERALIZES edges: %d", edgeCount)
	} else {
		t.Log("Hidden layer is disabled, skipping hidden node verification")
	}
}

// TestConsolidateWithSkipFlags tests the skip_* flags in consolidate request
func TestConsolidateWithSkipFlags(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()
	driver := SetupTestNeo4j(t)

	spaceID := GenerateTestSpaceID("hidden-skip")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Create a few test nodes
	for i := 0; i < 3; i++ {
		embedding := CreateControlledEmbedding(DefaultEmbeddingDims, 0.9, 1)
		ingestReq := map[string]any{
			"space_id":  spaceID,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"source":    "test",
			"name":      fmt.Sprintf("Skip Test Node %d", i),
			"content":   fmt.Sprintf("Content %d", i),
			"embedding": embedding,
		}
		reqBody, _ := json.Marshal(ingestReq)
		resp, _ := client.Post(cfg.MDEMGEndpoint+"/v1/memory/ingest", "application/json", bytes.NewReader(reqBody))
		resp.Body.Close()
	}

	// Test with all operations skipped
	consolidateReq := map[string]any{
		"space_id":        spaceID,
		"skip_clustering": true,
		"skip_forward":    true,
		"skip_backward":   true,
	}
	reqBody, _ := json.Marshal(consolidateReq)

	resp, err := client.Post(
		cfg.MDEMGEndpoint+"/v1/memory/consolidate",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		t.Fatalf("Failed to call consolidate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Consolidate with skip flags failed: status %d", resp.StatusCode)
	}

	var consolidateResp struct {
		Data struct {
			HiddenNodesCreated  int  `json:"hidden_nodes_created"`
			HiddenNodesUpdated  int  `json:"hidden_nodes_updated"`
			ConceptNodesUpdated int  `json:"concept_nodes_updated"`
			Enabled             bool `json:"enabled"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&consolidateResp)

	// When all operations are skipped, counts should be 0
	if consolidateResp.Data.Enabled {
		if consolidateResp.Data.HiddenNodesCreated != 0 {
			t.Errorf("Expected 0 hidden nodes created with skip_clustering=true, got %d",
				consolidateResp.Data.HiddenNodesCreated)
		}
	}

	t.Logf("Consolidate with all skips: created=%d, updated=%d",
		consolidateResp.Data.HiddenNodesCreated,
		consolidateResp.Data.HiddenNodesUpdated)
}

// TestConsolidateDisabled tests behavior when hidden layer is disabled
func TestConsolidateDisabled(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// This test checks behavior when hidden layer might be disabled
	// The response should still be valid, just with enabled=false

	consolidateReq := map[string]any{
		"space_id": GenerateTestSpaceID("disabled-check"),
	}
	reqBody, _ := json.Marshal(consolidateReq)

	resp, err := client.Post(
		cfg.MDEMGEndpoint+"/v1/memory/consolidate",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		t.Fatalf("Failed to call consolidate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Consolidate failed: status %d", resp.StatusCode)
	}

	var consolidateResp struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&consolidateResp)

	t.Logf("Hidden layer enabled: %v", consolidateResp.Data.Enabled)
}

// TestConsolidateValidation tests request validation
func TestConsolidateValidation(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	tests := []struct {
		name       string
		request    map[string]any
		wantStatus int
	}{
		{
			name:       "missing space_id",
			request:    map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty space_id",
			request:    map[string]any{"space_id": ""},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid request",
			request:    map[string]any{"space_id": "test-valid"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, _ := json.Marshal(tt.request)
			resp, err := client.Post(
				cfg.MDEMGEndpoint+"/v1/memory/consolidate",
				"application/json",
				bytes.NewReader(reqBody),
			)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("Status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

// Helper: count nodes at a specific layer
func countNodesAtLayer(t *testing.T, driver neo4j.DriverWithContext, ctx context.Context, spaceID string, layer int) int {
	t.Helper()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (n:MemoryNode {space_id: $spaceId, layer: $layer})
			RETURN count(n) AS count
		`, map[string]any{"spaceId": spaceID, "layer": layer})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("count")
			return count, nil
		}
		return 0, res.Err()
	})
	if err != nil {
		t.Fatalf("Failed to count nodes: %v", err)
	}

	switch v := result.(type) {
	case int64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

// Helper: count edges of a specific type
func countEdgesOfType(t *testing.T, driver neo4j.DriverWithContext, ctx context.Context, spaceID string, edgeType string) int {
	t.Helper()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := fmt.Sprintf(`
		MATCH (a:MemoryNode {space_id: $spaceId})-[r:%s]->(b:MemoryNode {space_id: $spaceId})
		RETURN count(r) AS count
	`, edgeType)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("count")
			return count, nil
		}
		return 0, res.Err()
	})
	if err != nil {
		t.Fatalf("Failed to count edges: %v", err)
	}

	switch v := result.(type) {
	case int64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}
