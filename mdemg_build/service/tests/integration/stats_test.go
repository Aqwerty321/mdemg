//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// StatsResponse mirrors the API response structure for tests.
type StatsResponse struct {
	SpaceID                string                `json:"space_id"`
	MemoryCount            int64                 `json:"memory_count"`
	ObservationCount       int64                 `json:"observation_count"`
	MemoriesByLayer        map[int]int64         `json:"memories_by_layer"`
	EmbeddingCoverage      float64               `json:"embedding_coverage"`
	AvgEmbeddingDimensions int                   `json:"avg_embedding_dimensions"`
	LearningActivity       *LearningActivity     `json:"learning_activity"`
	TemporalDistribution   *TemporalDistribution `json:"temporal_distribution"`
	Connectivity           *Connectivity         `json:"connectivity"`
	HealthScore            float64               `json:"health_score"`
	ComputedAt             string                `json:"computed_at"`
}

// LearningActivity mirrors the Hebbian learning metrics for tests.
type LearningActivity struct {
	CoActivatedEdges int64   `json:"co_activated_edges"`
	AvgWeight        float64 `json:"avg_weight"`
	MaxWeight        float64 `json:"max_weight"`
}

// TemporalDistribution mirrors the memory creation counts for tests.
type TemporalDistribution struct {
	Last24h int64 `json:"last_24h"`
	Last7d  int64 `json:"last_7d"`
	Last30d int64 `json:"last_30d"`
}

// Connectivity mirrors the graph connectivity stats for tests.
type Connectivity struct {
	AvgDegree   float64 `json:"avg_degree"`
	MaxDegree   int     `json:"max_degree"`
	OrphanCount int64   `json:"orphan_count"`
}

// TestStatsEndpoint validates the basic stats endpoint for an empty space.
// This test verifies that:
// 1. GET /v1/memory/stats?space_id=... returns HTTP 200
// 2. Response includes all expected fields
// 3. Response has valid structure even for empty space
// 4. Health score is 0.0 for empty space
func TestStatsEndpoint(t *testing.T) {
	// Setup: ensure service is ready
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Generate a unique space_id that won't have any data
	spaceID := GenerateTestSpaceID("stats-empty")

	// Note: No cleanup needed since we're not creating any data

	// --- Step 1: Call stats endpoint ---
	statsURL := cfg.MDEMGEndpoint + "/v1/memory/stats?space_id=" + spaceID
	httpReq, err := http.NewRequest(http.MethodGet, statsURL, nil)
	if err != nil {
		t.Fatalf("failed to create HTTP request: %v", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer resp.Body.Close()

	// --- Step 2: Verify HTTP 200 OK response ---
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Fatalf("stats returned status %d: %v", resp.StatusCode, errResp)
	}

	var statsResp StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&statsResp); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}

	// --- Step 3: Verify response structure ---

	// Space ID should be echoed back correctly
	if statsResp.SpaceID != spaceID {
		t.Errorf("response space_id mismatch: got %q, want %q", statsResp.SpaceID, spaceID)
	}

	// For empty space, counts should be 0
	if statsResp.MemoryCount != 0 {
		t.Errorf("expected memory_count=0 for empty space, got %d", statsResp.MemoryCount)
	}
	if statsResp.ObservationCount != 0 {
		t.Errorf("expected observation_count=0 for empty space, got %d", statsResp.ObservationCount)
	}

	// MemoriesByLayer should be an empty map (not nil)
	if statsResp.MemoriesByLayer == nil {
		t.Error("memories_by_layer should be an empty map, not nil")
	}

	// LearningActivity should be present (not nil)
	if statsResp.LearningActivity == nil {
		t.Error("learning_activity should be present, not nil")
	}

	// TemporalDistribution should be present (not nil)
	if statsResp.TemporalDistribution == nil {
		t.Error("temporal_distribution should be present, not nil")
	}

	// Connectivity should be present (not nil)
	if statsResp.Connectivity == nil {
		t.Error("connectivity should be present, not nil")
	}

	// Health score should be 0.0 for empty space
	if statsResp.HealthScore != 0.0 {
		t.Errorf("expected health_score=0.0 for empty space, got %f", statsResp.HealthScore)
	}

	// ComputedAt should be a valid timestamp
	if statsResp.ComputedAt == "" {
		t.Error("computed_at should not be empty")
	}

	t.Logf("Stats endpoint test passed: empty space %q returned valid response structure", spaceID)
}

// TestStatsWithData validates stats after ingesting memories.
// This test verifies that:
// 1. After ingesting N memories, memory_count = N
// 2. Observation count matches number of ingests
// 3. Embedding coverage is calculated correctly
// 4. Health score is > 0 for non-empty space
func TestStatsWithData(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for cleanup
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("stats-with-data")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// --- Step 1: Ingest 3 memories with embeddings ---
	testMemories := []struct {
		name string
		path string
		seed float32
	}{
		{"memory-alpha", "/test/stats/alpha", 1.0},
		{"memory-beta", "/test/stats/beta", 2.0},
		{"memory-gamma", "/test/stats/gamma", 3.0},
	}

	for _, mem := range testMemories {
		embedding := CreateTestEmbedding(DefaultEmbeddingDims, mem.seed)
		confidence := 0.8

		ingestReq := IngestRequest{
			SpaceID:     spaceID,
			Timestamp:   time.Now().Format(time.RFC3339),
			Source:      "test-source-stats",
			Content:     "Content for " + mem.name,
			Tags:        []string{"stats", "test"},
			Name:        mem.name,
			Path:        mem.path,
			Sensitivity: "internal",
			Confidence:  &confidence,
			Embedding:   embedding,
		}

		ingestBody, err := json.Marshal(ingestReq)
		if err != nil {
			t.Fatalf("failed to marshal ingest request for %s: %v", mem.name, err)
		}

		ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
		httpReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(ingestBody))
		if err != nil {
			t.Fatalf("failed to create ingest HTTP request for %s: %v", mem.name, err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("ingest request failed for %s: %v", mem.name, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errResp map[string]any
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("ingest returned status %d for %s: %v", resp.StatusCode, mem.name, errResp)
		}

		t.Logf("Ingested %s", mem.name)
	}

	// Allow time for data to be committed
	time.Sleep(500 * time.Millisecond)

	// --- Step 2: Call stats endpoint ---
	statsURL := cfg.MDEMGEndpoint + "/v1/memory/stats?space_id=" + spaceID
	httpReq, err := http.NewRequest(http.MethodGet, statsURL, nil)
	if err != nil {
		t.Fatalf("failed to create stats HTTP request: %v", err)
	}

	statsResp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer statsResp.Body.Close()

	if statsResp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(statsResp.Body).Decode(&errResp)
		t.Fatalf("stats returned status %d: %v", statsResp.StatusCode, errResp)
	}

	var stats StatsResponse
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}

	// --- Step 3: Verify counts ---

	// Memory count should equal number of ingested memories
	expectedMemoryCount := int64(len(testMemories))
	if stats.MemoryCount != expectedMemoryCount {
		t.Errorf("memory_count mismatch: got %d, want %d", stats.MemoryCount, expectedMemoryCount)
	}

	// Observation count should equal number of ingests (1 per ingest)
	expectedObsCount := int64(len(testMemories))
	if stats.ObservationCount != expectedObsCount {
		t.Errorf("observation_count mismatch: got %d, want %d", stats.ObservationCount, expectedObsCount)
	}

	// All memories have embeddings, so coverage should be 1.0
	if stats.EmbeddingCoverage < 0.99 {
		t.Errorf("embedding_coverage should be ~1.0 for all embedded memories, got %f", stats.EmbeddingCoverage)
	}

	// Embedding dimensions should match what we provided
	if stats.AvgEmbeddingDimensions != DefaultEmbeddingDims {
		t.Errorf("avg_embedding_dimensions mismatch: got %d, want %d", stats.AvgEmbeddingDimensions, DefaultEmbeddingDims)
	}

	// Health score should be > 0 for non-empty space with good embedding coverage
	if stats.HealthScore <= 0 {
		t.Errorf("health_score should be > 0 for non-empty space, got %f", stats.HealthScore)
	}

	// Health score should be <= 1.0
	if stats.HealthScore > 1.0 {
		t.Errorf("health_score should be <= 1.0, got %f", stats.HealthScore)
	}

	// Temporal distribution: all memories should be in last_24h, last_7d, last_30d
	if stats.TemporalDistribution != nil {
		if stats.TemporalDistribution.Last24h != expectedMemoryCount {
			t.Errorf("temporal_distribution.last_24h mismatch: got %d, want %d",
				stats.TemporalDistribution.Last24h, expectedMemoryCount)
		}
		if stats.TemporalDistribution.Last7d < stats.TemporalDistribution.Last24h {
			t.Errorf("temporal_distribution.last_7d (%d) should be >= last_24h (%d)",
				stats.TemporalDistribution.Last7d, stats.TemporalDistribution.Last24h)
		}
	}

	// Connectivity: all nodes are orphans (no edges created)
	if stats.Connectivity != nil {
		if stats.Connectivity.OrphanCount != expectedMemoryCount {
			t.Errorf("connectivity.orphan_count mismatch: got %d, want %d",
				stats.Connectivity.OrphanCount, expectedMemoryCount)
		}
	}

	t.Logf("Stats with data test passed: memory_count=%d, observation_count=%d, embedding_coverage=%.2f, health_score=%.2f",
		stats.MemoryCount, stats.ObservationCount, stats.EmbeddingCoverage, stats.HealthScore)
}

// TestStatsMissingSpaceID verifies that missing space_id returns 400 Bad Request.
func TestStatsMissingSpaceID(t *testing.T) {
	// Setup: ensure service is ready
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// --- Call stats endpoint without space_id ---
	statsURL := cfg.MDEMGEndpoint + "/v1/memory/stats"
	httpReq, err := http.NewRequest(http.MethodGet, statsURL, nil)
	if err != nil {
		t.Fatalf("failed to create HTTP request: %v", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer resp.Body.Close()

	// --- Verify HTTP 400 Bad Request ---
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing space_id, got %d", resp.StatusCode)
	}

	// Verify error message
	var errResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errMsg, ok := errResp["error"].(string); ok {
		if errMsg == "" {
			t.Error("error message should not be empty")
		}
		t.Logf("Got expected error: %s", errMsg)
	} else {
		t.Error("error response should contain 'error' field")
	}
}

// TestStatsMethodNotAllowed verifies that non-GET methods return 405 Method Not Allowed.
func TestStatsMethodNotAllowed(t *testing.T) {
	// Setup: ensure service is ready
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	spaceID := GenerateTestSpaceID("stats-method")
	statsURL := cfg.MDEMGEndpoint + "/v1/memory/stats?space_id=" + spaceID

	// Test POST, PUT, DELETE methods
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			httpReq, err := http.NewRequest(method, statsURL, nil)
			if err != nil {
				t.Fatalf("failed to create HTTP request: %v", err)
			}

			resp, err := client.Do(httpReq)
			if err != nil {
				t.Fatalf("%s request failed: %v", method, err)
			}
			defer resp.Body.Close()

			// Verify HTTP 405 Method Not Allowed
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s should return 405, got %d", method, resp.StatusCode)
			}
		})
	}
}

// TestStatsEmptySpace verifies that querying a non-existent space returns valid zeroed stats.
// This is similar to TestStatsEndpoint but explicitly tests the "non-existent space" scenario.
func TestStatsEmptySpace(t *testing.T) {
	// Setup: ensure service is ready
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Generate a unique space_id that definitely doesn't exist
	nonExistentSpaceID := GenerateTestSpaceID("nonexistent-stats-space")

	// --- Call stats endpoint ---
	statsURL := cfg.MDEMGEndpoint + "/v1/memory/stats?space_id=" + nonExistentSpaceID
	httpReq, err := http.NewRequest(http.MethodGet, statsURL, nil)
	if err != nil {
		t.Fatalf("failed to create HTTP request: %v", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer resp.Body.Close()

	// --- Verify HTTP 200 OK (not an error for non-existent space) ---
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Fatalf("expected HTTP 200 for non-existent space, got status %d: %v", resp.StatusCode, errResp)
	}

	var stats StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}

	// --- Verify zeroed statistics ---
	if stats.SpaceID != nonExistentSpaceID {
		t.Errorf("space_id mismatch: got %q, want %q", stats.SpaceID, nonExistentSpaceID)
	}

	if stats.MemoryCount != 0 {
		t.Errorf("expected memory_count=0, got %d", stats.MemoryCount)
	}

	if stats.ObservationCount != 0 {
		t.Errorf("expected observation_count=0, got %d", stats.ObservationCount)
	}

	if stats.EmbeddingCoverage != 0.0 {
		t.Errorf("expected embedding_coverage=0.0, got %f", stats.EmbeddingCoverage)
	}

	if stats.HealthScore != 0.0 {
		t.Errorf("expected health_score=0.0 for empty space, got %f", stats.HealthScore)
	}

	// Learning activity should have zero values
	if stats.LearningActivity != nil {
		if stats.LearningActivity.CoActivatedEdges != 0 {
			t.Errorf("expected co_activated_edges=0, got %d", stats.LearningActivity.CoActivatedEdges)
		}
	}

	// Temporal distribution should have zero values
	if stats.TemporalDistribution != nil {
		if stats.TemporalDistribution.Last24h != 0 {
			t.Errorf("expected last_24h=0, got %d", stats.TemporalDistribution.Last24h)
		}
	}

	// Connectivity should have zero values
	if stats.Connectivity != nil {
		if stats.Connectivity.OrphanCount != 0 {
			t.Errorf("expected orphan_count=0, got %d", stats.Connectivity.OrphanCount)
		}
	}

	t.Logf("Empty space stats test passed: non-existent space %q returned zeroed stats", nonExistentSpaceID)
}

// TestStatsEmbeddingCoverage validates embedding coverage calculation.
// This test verifies that:
// 1. Nodes without embeddings are counted correctly
// 2. Coverage = nodes_with_embedding / total_nodes
func TestStatsEmbeddingCoverage(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for cleanup
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("stats-embed-coverage")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// --- Step 1: Ingest 2 memories WITH embeddings ---
	for i := 1; i <= 2; i++ {
		embedding := CreateTestEmbedding(DefaultEmbeddingDims, float32(i))
		confidence := 0.8

		ingestReq := IngestRequest{
			SpaceID:    spaceID,
			Timestamp:  time.Now().Format(time.RFC3339),
			Source:     "test-source-coverage",
			Content:    "Content with embedding",
			Name:       "node-with-embedding",
			Path:       "/test/coverage/with-" + time.Now().Format("150405.000") + "-" + string(rune('a'+i-1)),
			Confidence: &confidence,
			Embedding:  embedding,
		}

		ingestBody, err := json.Marshal(ingestReq)
		if err != nil {
			t.Fatalf("failed to marshal ingest request: %v", err)
		}

		ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
		httpReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(ingestBody))
		if err != nil {
			t.Fatalf("failed to create HTTP request: %v", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("ingest request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("ingest returned status %d", resp.StatusCode)
		}
	}

	// Allow time for data to be committed
	time.Sleep(500 * time.Millisecond)

	// --- Step 2: Get stats ---
	statsURL := cfg.MDEMGEndpoint + "/v1/memory/stats?space_id=" + spaceID
	httpReq, err := http.NewRequest(http.MethodGet, statsURL, nil)
	if err != nil {
		t.Fatalf("failed to create stats HTTP request: %v", err)
	}

	statsResp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer statsResp.Body.Close()

	if statsResp.StatusCode != http.StatusOK {
		t.Fatalf("stats returned status %d", statsResp.StatusCode)
	}

	var stats StatsResponse
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}

	// --- Step 3: Verify embedding coverage ---

	// All 2 nodes have embeddings, so coverage should be 1.0 (100%)
	expectedCoverage := 1.0
	if stats.EmbeddingCoverage < expectedCoverage-0.01 || stats.EmbeddingCoverage > expectedCoverage+0.01 {
		t.Errorf("embedding_coverage mismatch: got %.2f, want %.2f", stats.EmbeddingCoverage, expectedCoverage)
	}

	// Verify memory count
	if stats.MemoryCount != 2 {
		t.Errorf("memory_count mismatch: got %d, want 2", stats.MemoryCount)
	}

	t.Logf("Embedding coverage test passed: coverage=%.2f with %d memories",
		stats.EmbeddingCoverage, stats.MemoryCount)
}
