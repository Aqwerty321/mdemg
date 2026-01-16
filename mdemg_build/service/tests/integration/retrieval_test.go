//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// RetrieveRequest mirrors the API request structure for tests.
type RetrieveRequest struct {
	SpaceID        string    `json:"space_id"`
	QueryText      string    `json:"query_text,omitempty"`
	QueryEmbedding []float32 `json:"query_embedding,omitempty"`
	CandidateK     int       `json:"candidate_k,omitempty"`
	TopK           int       `json:"top_k,omitempty"`
	HopDepth       int       `json:"hop_depth,omitempty"`
}

// RetrieveResult mirrors the API result structure for tests.
type RetrieveResult struct {
	NodeID     string  `json:"node_id"`
	Path       string  `json:"path"`
	Name       string  `json:"name"`
	Summary    string  `json:"summary"`
	Score      float64 `json:"score"`
	VectorSim  float64 `json:"vector_sim,omitempty"`
	Activation float64 `json:"activation,omitempty"`
}

// RetrieveResponse mirrors the API response structure for tests.
type RetrieveResponse struct {
	SpaceID string           `json:"space_id"`
	Results []RetrieveResult `json:"results"`
	Debug   map[string]any   `json:"debug,omitempty"`
}

// TestIngestAndRetrieve validates the end-to-end flow: ingest a node, then retrieve it.
// This test verifies that:
// 1. A node ingested with an embedding can be found via vector search
// 2. The retrieve API returns the ingested node with correct properties
// 3. Vector similarity scoring works correctly (same embedding should return high similarity)
func TestIngestAndRetrieve(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for verification
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("retrieve")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Create a test embedding that we'll use for both ingest and retrieve
	testEmbedding := CreateTestEmbedding(DefaultEmbeddingDims, 1.0)

	// --- Step 1: Ingest a node with a known embedding ---
	confidence := 0.9
	ingestReq := IngestRequest{
		SpaceID:     spaceID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "test-source-retrieve",
		Content:     "This is test content for retrieval testing",
		Tags:        []string{"retrieve", "integration", "test"},
		Name:        "retrieve-test-node",
		Path:        "/test/retrieve/node1",
		Sensitivity: "internal",
		Confidence:  &confidence,
		Embedding:   testEmbedding,
	}

	ingestBody, err := json.Marshal(ingestReq)
	if err != nil {
		t.Fatalf("failed to marshal ingest request: %v", err)
	}

	ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
	httpIngestReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(ingestBody))
	if err != nil {
		t.Fatalf("failed to create ingest HTTP request: %v", err)
	}
	httpIngestReq.Header.Set("Content-Type", "application/json")

	ingestResp, err := client.Do(httpIngestReq)
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer ingestResp.Body.Close()

	if ingestResp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(ingestResp.Body).Decode(&errResp)
		t.Fatalf("ingest returned status %d: %v", ingestResp.StatusCode, errResp)
	}

	var ingestResponse IngestResponse
	if err := json.NewDecoder(ingestResp.Body).Decode(&ingestResponse); err != nil {
		t.Fatalf("failed to decode ingest response: %v", err)
	}

	if ingestResponse.NodeID == "" {
		t.Fatal("ingest response node_id is empty")
	}

	ingestedNodeID := ingestResponse.NodeID
	t.Logf("Ingested node with ID: %s", ingestedNodeID)

	// Small delay to ensure data is committed and indexed
	time.Sleep(500 * time.Millisecond)

	// --- Step 2: Retrieve using the same embedding ---
	retrieveReq := RetrieveRequest{
		SpaceID:        spaceID,
		QueryEmbedding: testEmbedding,
		CandidateK:     50,
		TopK:           10,
		HopDepth:       2,
	}

	retrieveBody, err := json.Marshal(retrieveReq)
	if err != nil {
		t.Fatalf("failed to marshal retrieve request: %v", err)
	}

	retrieveURL := cfg.MDEMGEndpoint + "/v1/memory/retrieve"
	httpRetrieveReq, err := http.NewRequest(http.MethodPost, retrieveURL, bytes.NewReader(retrieveBody))
	if err != nil {
		t.Fatalf("failed to create retrieve HTTP request: %v", err)
	}
	httpRetrieveReq.Header.Set("Content-Type", "application/json")

	retrieveResp, err := client.Do(httpRetrieveReq)
	if err != nil {
		t.Fatalf("retrieve request failed: %v", err)
	}
	defer retrieveResp.Body.Close()

	if retrieveResp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(retrieveResp.Body).Decode(&errResp)
		t.Fatalf("retrieve returned status %d: %v", retrieveResp.StatusCode, errResp)
	}

	var retrieveResponse RetrieveResponse
	if err := json.NewDecoder(retrieveResp.Body).Decode(&retrieveResponse); err != nil {
		t.Fatalf("failed to decode retrieve response: %v", err)
	}

	// --- Step 3: Verify the response ---

	// Check space_id matches
	if retrieveResponse.SpaceID != spaceID {
		t.Errorf("retrieve response space_id mismatch: got %q, want %q", retrieveResponse.SpaceID, spaceID)
	}

	// Check that results are not empty
	if len(retrieveResponse.Results) == 0 {
		t.Fatal("retrieve returned no results - expected at least the ingested node")
	}

	// Check that the ingested node is in the results
	var foundNode *RetrieveResult
	for i, result := range retrieveResponse.Results {
		if result.NodeID == ingestedNodeID {
			foundNode = &retrieveResponse.Results[i]
			break
		}
	}

	if foundNode == nil {
		t.Fatalf("ingested node %q not found in retrieve results. Got results: %+v", ingestedNodeID, retrieveResponse.Results)
	}

	// Verify node properties match
	if foundNode.Path != ingestReq.Path {
		t.Errorf("result path mismatch: got %q, want %q", foundNode.Path, ingestReq.Path)
	}
	if foundNode.Name != ingestReq.Name {
		t.Errorf("result name mismatch: got %q, want %q", foundNode.Name, ingestReq.Name)
	}

	// Score should be > 0 for a matching node
	if foundNode.Score <= 0 {
		t.Errorf("result score should be positive, got %f", foundNode.Score)
	}

	// Since we used the exact same embedding for ingest and retrieve,
	// the vector similarity should be high (close to 1.0 for cosine similarity)
	if foundNode.VectorSim <= 0.5 {
		t.Errorf("vector_sim should be high for identical embedding, got %f", foundNode.VectorSim)
	}

	// Verify debug information is present
	if retrieveResponse.Debug == nil {
		t.Error("retrieve response debug is nil - expected debug information")
	} else {
		// Check for expected debug fields
		if _, ok := retrieveResponse.Debug["candidate_k"]; !ok {
			t.Error("debug missing candidate_k field")
		}
		if _, ok := retrieveResponse.Debug["hop_depth"]; !ok {
			t.Error("debug missing hop_depth field")
		}
	}

	t.Logf("Successfully retrieved ingested node with score=%f, vector_sim=%f", foundNode.Score, foundNode.VectorSim)
}
