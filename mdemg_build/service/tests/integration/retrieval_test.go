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

// TestScoringDeterminism verifies that identical queries produce identical scores across multiple runs.
// This test ensures the retrieval pipeline (vector recall, spreading activation, scoring) is deterministic.
// According to doc 12_Retrieval_Scoring_Worked_Examples.md, scoring is computed as:
//   S_i = α*v_i + β*a_i + γ*r_i + δ*c_i - φ*h_i - κ*d_i
// All these factors should produce identical results for identical inputs.
func TestScoringDeterminism(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for verification
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("scoring-determinism")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// --- Step 1: Ingest multiple nodes with known embeddings ---
	// Create several nodes to ensure scoring involves non-trivial computation
	// (vector similarity, activation spread, hub penalties)
	testNodes := []struct {
		name       string
		path       string
		seed       float32
		confidence float64
	}{
		{"node-alpha", "/test/scoring/alpha", 1.0, 0.9},
		{"node-beta", "/test/scoring/beta", 1.5, 0.8},
		{"node-gamma", "/test/scoring/gamma", 2.0, 0.7},
		{"node-delta", "/test/scoring/delta", 2.5, 0.6},
		{"node-epsilon", "/test/scoring/epsilon", 3.0, 0.5},
	}

	for _, node := range testNodes {
		embedding := CreateTestEmbedding(DefaultEmbeddingDims, node.seed)
		confidence := node.confidence

		ingestReq := IngestRequest{
			SpaceID:     spaceID,
			Timestamp:   time.Now().Format(time.RFC3339),
			Source:      "test-source-determinism",
			Content:     "Content for " + node.name + " - scoring determinism test",
			Tags:        []string{"determinism", "test"},
			Name:        node.name,
			Path:        node.path,
			Sensitivity: "internal",
			Confidence:  &confidence,
			Embedding:   embedding,
		}

		ingestBody, err := json.Marshal(ingestReq)
		if err != nil {
			t.Fatalf("failed to marshal ingest request for %s: %v", node.name, err)
		}

		ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
		httpReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(ingestBody))
		if err != nil {
			t.Fatalf("failed to create ingest HTTP request for %s: %v", node.name, err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("ingest request failed for %s: %v", node.name, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errResp map[string]any
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("ingest returned status %d for %s: %v", resp.StatusCode, node.name, errResp)
		}
	}

	// Allow time for data to be committed and indexed
	time.Sleep(500 * time.Millisecond)

	// --- Step 2: Build the query that will be repeated ---
	// Use a known embedding that will match our test nodes at varying similarities
	queryEmbedding := CreateTestEmbedding(DefaultEmbeddingDims, 1.25) // Between alpha and beta

	retrieveReq := RetrieveRequest{
		SpaceID:        spaceID,
		QueryEmbedding: queryEmbedding,
		CandidateK:     50,
		TopK:           10,
		HopDepth:       2,
	}

	// --- Step 3: Run the same query multiple times and collect results ---
	numRuns := 5
	allResponses := make([]RetrieveResponse, numRuns)

	retrieveURL := cfg.MDEMGEndpoint + "/v1/memory/retrieve"

	for i := 0; i < numRuns; i++ {
		retrieveBody, err := json.Marshal(retrieveReq)
		if err != nil {
			t.Fatalf("run %d: failed to marshal retrieve request: %v", i+1, err)
		}

		httpReq, err := http.NewRequest(http.MethodPost, retrieveURL, bytes.NewReader(retrieveBody))
		if err != nil {
			t.Fatalf("run %d: failed to create retrieve HTTP request: %v", i+1, err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("run %d: retrieve request failed: %v", i+1, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errResp map[string]any
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("run %d: retrieve returned status %d: %v", i+1, resp.StatusCode, errResp)
		}

		if err := json.NewDecoder(resp.Body).Decode(&allResponses[i]); err != nil {
			t.Fatalf("run %d: failed to decode retrieve response: %v", i+1, err)
		}
	}

	// --- Step 4: Verify all runs produced identical results ---
	baseline := allResponses[0]

	// First, check that we got results
	if len(baseline.Results) == 0 {
		t.Fatal("baseline query returned no results - cannot verify determinism")
	}

	t.Logf("Baseline query returned %d results", len(baseline.Results))

	for runIdx := 1; runIdx < numRuns; runIdx++ {
		current := allResponses[runIdx]

		// Check result count matches
		if len(current.Results) != len(baseline.Results) {
			t.Errorf("run %d: result count differs from baseline: got %d, want %d",
				runIdx+1, len(current.Results), len(baseline.Results))
			continue
		}

		// Check each result matches exactly
		for i, baseResult := range baseline.Results {
			currResult := current.Results[i]

			// Verify node ordering is identical
			if currResult.NodeID != baseResult.NodeID {
				t.Errorf("run %d, position %d: node_id differs: got %q, want %q",
					runIdx+1, i, currResult.NodeID, baseResult.NodeID)
			}

			// Verify scores are identical (exact match - no floating point tolerance)
			// The scoring algorithm should be fully deterministic
			if currResult.Score != baseResult.Score {
				t.Errorf("run %d, position %d (node %s): score differs: got %f, want %f",
					runIdx+1, i, baseResult.NodeID, currResult.Score, baseResult.Score)
			}

			// Verify vector similarity is identical
			if currResult.VectorSim != baseResult.VectorSim {
				t.Errorf("run %d, position %d (node %s): vector_sim differs: got %f, want %f",
					runIdx+1, i, baseResult.NodeID, currResult.VectorSim, baseResult.VectorSim)
			}

			// Verify activation is identical
			if currResult.Activation != baseResult.Activation {
				t.Errorf("run %d, position %d (node %s): activation differs: got %f, want %f",
					runIdx+1, i, baseResult.NodeID, currResult.Activation, baseResult.Activation)
			}
		}
	}

	// Log summary of baseline results for debugging
	t.Logf("Scoring determinism verified across %d runs with %d results each", numRuns, len(baseline.Results))
	for i, result := range baseline.Results {
		t.Logf("  Position %d: node=%s, score=%.6f, vector_sim=%.6f, activation=%.6f",
			i, result.NodeID, result.Score, result.VectorSim, result.Activation)
	}
}
