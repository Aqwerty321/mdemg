package retrieval

import (
	"testing"
	"time"
)

func TestQueryTimer(t *testing.T) {
	timer := StartQueryTimer("test_query", "MATCH (n) RETURN n", map[string]any{"param": "value"})

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	duration := timer.End()

	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}
}

func TestQueryMetrics(t *testing.T) {
	// Reset metrics before test
	ResetQueryMetrics()

	// Run a few timed queries
	for i := 0; i < 5; i++ {
		timer := StartQueryTimer("test", "MATCH (n) RETURN n", nil)
		time.Sleep(5 * time.Millisecond)
		timer.End()
	}

	metrics := GetQueryMetrics()

	totalQueries := metrics["total_queries"].(int64)
	if totalQueries != 5 {
		t.Errorf("expected 5 total queries, got %d", totalQueries)
	}

	slowQueries := metrics["slow_queries"].(int64)
	if slowQueries != 0 {
		t.Errorf("expected 0 slow queries (all under 100ms), got %d", slowQueries)
	}

	avgDuration := metrics["avg_duration_ms"].(int64)
	if avgDuration < 5 {
		t.Errorf("expected avg duration >= 5ms, got %d", avgDuration)
	}
}

func TestSlowQueryDetection(t *testing.T) {
	ResetQueryMetrics()

	// Simulate a slow query (>100ms)
	timer := StartQueryTimer("slow_query", "MATCH (n)-[*..10]-(m) RETURN n,m", nil)
	time.Sleep(110 * time.Millisecond)
	timer.End()

	metrics := GetQueryMetrics()

	slowQueries := metrics["slow_queries"].(int64)
	if slowQueries != 1 {
		t.Errorf("expected 1 slow query, got %d", slowQueries)
	}

	slowPct := metrics["slow_query_pct"].(float64)
	if slowPct != 100.0 {
		t.Errorf("expected 100%% slow queries, got %.1f%%", slowPct)
	}
}

func TestResetQueryMetrics(t *testing.T) {
	// Add some metrics
	timer := StartQueryTimer("test", "MATCH (n) RETURN n", nil)
	timer.End()

	// Verify non-zero
	metrics := GetQueryMetrics()
	if metrics["total_queries"].(int64) == 0 {
		t.Error("expected non-zero total queries before reset")
	}

	// Reset
	ResetQueryMetrics()

	// Verify zero
	metrics = GetQueryMetrics()
	if metrics["total_queries"].(int64) != 0 {
		t.Error("expected zero total queries after reset")
	}
}

func TestSanitizeParams(t *testing.T) {
	params := map[string]any{
		"spaceId": "test-space",
		"k":       100,
		"q":       make([]float32, 1536), // Embedding vector
	}

	sanitized := sanitizeParams(params)

	// spaceId and k should be unchanged
	if sanitized["spaceId"] != "test-space" {
		t.Error("spaceId should be unchanged")
	}
	if sanitized["k"] != 100 {
		t.Error("k should be unchanged")
	}

	// Embedding should be sanitized to a description
	qVal, ok := sanitized["q"].(string)
	if !ok {
		t.Errorf("expected q to be sanitized to string, got %T", sanitized["q"])
	}
	if qVal == "" {
		t.Error("sanitized q should not be empty")
	}
}

func TestCompactCypher(t *testing.T) {
	cypher := `MATCH (n:MemoryNode {space_id:$spaceId})
WHERE n.node_id IN $nodeIds
  AND n.status = 'active'
RETURN n.node_id, n.name`

	compact := compactCypher(cypher)

	// Should not contain newlines
	for _, c := range compact {
		if c == '\n' {
			t.Error("compact cypher should not contain newlines")
		}
	}

	// Should contain the key parts
	if len(compact) == 0 {
		t.Error("compact cypher should not be empty")
	}
}

func TestCompactCypherTruncation(t *testing.T) {
	// Create a very long cypher query
	longCypher := "MATCH (n) WHERE "
	for i := 0; i < 100; i++ {
		longCypher += "n.property" + string(rune('a'+i%26)) + " = $param AND "
	}
	longCypher += "RETURN n"

	compact := compactCypher(longCypher)

	// Should be truncated to ~200 chars + "..."
	if len(compact) > 210 {
		t.Errorf("expected truncated cypher <= 210 chars, got %d", len(compact))
	}

	// Should end with "..."
	if len(compact) > 200 && compact[len(compact)-3:] != "..." {
		t.Error("truncated cypher should end with ...")
	}
}
