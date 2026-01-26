package retrieval

import (
	"testing"
	"time"

	"mdemg/internal/models"
)

func TestCacheKey(t *testing.T) {
	req1 := models.RetrieveRequest{
		SpaceID:         "test-space",
		QueryText:       "test query",
		CandidateK:      100,
		TopK:            10,
		HopDepth:        2,
		IncludeEvidence: true,
	}

	req2 := models.RetrieveRequest{
		SpaceID:         "test-space",
		QueryText:       "test query",
		CandidateK:      100,
		TopK:            10,
		HopDepth:        2,
		IncludeEvidence: true,
	}

	req3 := models.RetrieveRequest{
		SpaceID:         "test-space",
		QueryText:       "different query",
		CandidateK:      100,
		TopK:            10,
		HopDepth:        2,
		IncludeEvidence: true,
	}

	key1 := CacheKey(req1)
	key2 := CacheKey(req2)
	key3 := CacheKey(req3)

	// Same request should produce same key
	if key1 != key2 {
		t.Errorf("identical requests produced different keys: %s vs %s", key1, key2)
	}

	// Different query should produce different key
	if key1 == key3 {
		t.Errorf("different queries produced same key: %s", key1)
	}

	// Key should be 32 hex chars (16 bytes)
	if len(key1) != 32 {
		t.Errorf("expected key length 32, got %d", len(key1))
	}
}

func TestQueryCache_PutGet(t *testing.T) {
	cache := NewQueryCache(10, time.Minute)

	req := models.RetrieveRequest{
		SpaceID:    "test-space",
		QueryText:  "test query",
		CandidateK: 100,
		TopK:       10,
		HopDepth:   2,
	}

	resp := models.RetrieveResponse{
		Results: []models.RetrieveResult{
			{NodeID: "node1", Activation: 0.9},
			{NodeID: "node2", Activation: 0.8},
		},
	}

	// Initially cache miss
	if _, ok := cache.Get(req); ok {
		t.Error("expected cache miss for empty cache")
	}

	// Put value
	cache.Put(req, resp)

	// Now should hit
	cached, ok := cache.Get(req)
	if !ok {
		t.Error("expected cache hit after Put")
	}

	if len(cached.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(cached.Results))
	}

	if cached.Results[0].NodeID != "node1" {
		t.Errorf("expected node1, got %s", cached.Results[0].NodeID)
	}

	// Check stats
	stats := cache.Stats()
	if stats["hits"].(int64) != 1 {
		t.Errorf("expected 1 hit, got %v", stats["hits"])
	}
	if stats["misses"].(int64) != 1 {
		t.Errorf("expected 1 miss, got %v", stats["misses"])
	}
}

func TestQueryCache_TTLExpiration(t *testing.T) {
	// Use very short TTL for testing
	cache := NewQueryCache(10, 50*time.Millisecond)

	req := models.RetrieveRequest{
		SpaceID:   "test-space",
		QueryText: "test query",
	}

	resp := models.RetrieveResponse{
		Results: []models.RetrieveResult{
			{NodeID: "node1", Activation: 0.9},
		},
	}

	cache.Put(req, resp)

	// Should hit immediately
	if _, ok := cache.Get(req); !ok {
		t.Error("expected cache hit immediately after Put")
	}

	// Wait for TTL expiration
	time.Sleep(60 * time.Millisecond)

	// Should miss after TTL
	if _, ok := cache.Get(req); ok {
		t.Error("expected cache miss after TTL expiration")
	}
}

func TestQueryCache_LRUEviction(t *testing.T) {
	// Small capacity for testing eviction
	cache := NewQueryCache(3, time.Minute)

	// Add 4 items, oldest should be evicted
	for i := 0; i < 4; i++ {
		req := models.RetrieveRequest{
			SpaceID:   "test-space",
			QueryText: string(rune('a' + i)),
		}
		resp := models.RetrieveResponse{
			Results: []models.RetrieveResult{
				{NodeID: string(rune('a' + i))},
			},
		}
		cache.Put(req, resp)
	}

	// Capacity should not exceed 3
	if cache.Len() != 3 {
		t.Errorf("expected cache len 3, got %d", cache.Len())
	}

	// First item (query="a") should have been evicted
	req0 := models.RetrieveRequest{SpaceID: "test-space", QueryText: "a"}
	if _, ok := cache.Get(req0); ok {
		t.Error("expected first item to be evicted")
	}

	// Last three items should still be present
	for i := 1; i < 4; i++ {
		req := models.RetrieveRequest{SpaceID: "test-space", QueryText: string(rune('a' + i))}
		if _, ok := cache.Get(req); !ok {
			t.Errorf("expected item %d to still be in cache", i)
		}
	}
}

func TestQueryCache_InvalidateSpace(t *testing.T) {
	cache := NewQueryCache(10, time.Minute)

	// Add items from different spaces
	req1 := models.RetrieveRequest{SpaceID: "space-a", QueryText: "query1"}
	req2 := models.RetrieveRequest{SpaceID: "space-a", QueryText: "query2"}
	req3 := models.RetrieveRequest{SpaceID: "space-b", QueryText: "query1"}

	cache.Put(req1, models.RetrieveResponse{})
	cache.Put(req2, models.RetrieveResponse{})
	cache.Put(req3, models.RetrieveResponse{})

	if cache.Len() != 3 {
		t.Errorf("expected 3 items, got %d", cache.Len())
	}

	// Invalidate space-a
	removed := cache.InvalidateSpace("space-a")
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	// space-a items should be gone
	if _, ok := cache.Get(req1); ok {
		t.Error("space-a item should have been invalidated")
	}
	if _, ok := cache.Get(req2); ok {
		t.Error("space-a item should have been invalidated")
	}

	// space-b item should remain
	if _, ok := cache.Get(req3); !ok {
		t.Error("space-b item should still be in cache")
	}
}

func TestQueryCache_Clear(t *testing.T) {
	cache := NewQueryCache(10, time.Minute)

	for i := 0; i < 5; i++ {
		req := models.RetrieveRequest{SpaceID: "test", QueryText: string(rune('a' + i))}
		cache.Put(req, models.RetrieveResponse{})
	}

	if cache.Len() != 5 {
		t.Errorf("expected 5 items, got %d", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected 0 items after clear, got %d", cache.Len())
	}
}

func TestQueryCache_HitRate(t *testing.T) {
	cache := NewQueryCache(10, time.Minute)

	req := models.RetrieveRequest{SpaceID: "test", QueryText: "query"}
	cache.Put(req, models.RetrieveResponse{})

	// 1 miss (initial check), 3 hits
	cache.Get(models.RetrieveRequest{SpaceID: "test", QueryText: "miss"}) // miss
	cache.Get(req) // hit
	cache.Get(req) // hit
	cache.Get(req) // hit

	stats := cache.Stats()
	hitRate := stats["hit_rate"].(float64)

	// 3 hits out of 4 total = 0.75
	expected := 0.75
	if hitRate != expected {
		t.Errorf("expected hit rate %.2f, got %.2f", expected, hitRate)
	}
}
