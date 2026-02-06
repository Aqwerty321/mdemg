package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestLimiter_Allow(t *testing.T) {
	// Create a limiter with 10 tokens/sec, burst of 5
	l := NewLimiter(10, 5)

	// Should allow burst of 5
	for i := 0; i < 5; i++ {
		if !l.Allow() {
			t.Errorf("request %d should have been allowed", i+1)
		}
	}

	// 6th request should be denied
	if l.Allow() {
		t.Error("6th request should have been denied")
	}
}

func TestLimiter_Refill(t *testing.T) {
	// Create a limiter with 100 tokens/sec, burst of 10
	l := NewLimiter(100, 10)

	// Drain all tokens
	for i := 0; i < 10; i++ {
		l.Allow()
	}

	// Should be empty
	if l.Allow() {
		t.Error("should be empty after draining")
	}

	// Wait for refill (100 tokens/sec = 1 token per 10ms)
	time.Sleep(50 * time.Millisecond)

	// Should have refilled at least some tokens
	if !l.Allow() {
		t.Error("should have refilled after 50ms")
	}
}

func TestLimiter_Concurrent(t *testing.T) {
	l := NewLimiter(1000, 100)

	var wg sync.WaitGroup
	allowed := int64(0)
	var mu sync.Mutex

	// Launch 200 concurrent requests
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should have allowed exactly burst size (100)
	if allowed != 100 {
		t.Errorf("expected 100 allowed, got %d", allowed)
	}
}

func TestLimiterStore_GetLimiter(t *testing.T) {
	store := NewLimiterStore(100, 10)

	// Same key should return same limiter
	l1 := store.GetLimiter("192.168.1.1")
	l2 := store.GetLimiter("192.168.1.1")

	if l1 != l2 {
		t.Error("same key should return same limiter")
	}

	// Different key should return different limiter
	l3 := store.GetLimiter("192.168.1.2")
	if l1 == l3 {
		t.Error("different keys should return different limiters")
	}

	// Store should have 2 entries
	if store.Size() != 2 {
		t.Errorf("expected 2 limiters, got %d", store.Size())
	}
}

func TestMiddleware_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// All requests should pass through
	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d should have been allowed when disabled", i+1)
		}
	}
}

func TestMiddleware_RateLimits(t *testing.T) {
	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 10,
		BurstSize:         5,
		ByIP:              false, // Global limiter
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 5 should pass (burst)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d should have been allowed", i+1)
		}
	}

	// 6th should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}

	// Should have Retry-After header
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestMiddleware_SkipEndpoints(t *testing.T) {
	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		BurstSize:         1,
		ByIP:              false,
		SkipEndpoints: map[string]bool{
			"/healthz": true,
		},
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the limiter
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Health check should still pass
	req = httptest.NewRequest("GET", "/healthz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("healthz should bypass rate limiting, got %d", rec.Code)
	}
}

func TestMiddleware_ByIP(t *testing.T) {
	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 10,
		BurstSize:         2,
		ByIP:              true,
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP 1 - exhaust its burst
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if i < 2 && rec.Code != http.StatusOK {
			t.Errorf("IP1 request %d should have been allowed", i+1)
		}
		if i >= 2 && rec.Code != http.StatusTooManyRequests {
			t.Errorf("IP1 request %d should have been rate limited", i+1)
		}
	}

	// IP 2 should still have its own burst allowance
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.2:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("IP2 request %d should have been allowed", i+1)
		}
	}
}

func TestExtractIP_DirectConnection(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:12345"

	ip := extractIP(r, nil)
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

func TestExtractIP_TrustedProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:12345"
	r.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.2")

	// Without trusted proxy, should use RemoteAddr
	ip := extractIP(r, nil)
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}

	// With trusted proxy, should use X-Forwarded-For
	ip = extractIP(r, []string{"10.0.0.1"})
	if ip != "203.0.113.1" {
		t.Errorf("expected 203.0.113.1, got %s", ip)
	}
}

func TestExtractIP_TrustedProxyCIDR(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.50:12345"
	r.Header.Set("X-Forwarded-For", "203.0.113.1")

	// Trust entire 10.0.0.0/24 subnet
	ip := extractIP(r, []string{"10.0.0.0/24"})
	if ip != "203.0.113.1" {
		t.Errorf("expected 203.0.113.1, got %s", ip)
	}
}
