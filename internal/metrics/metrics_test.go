package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCounter(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	c := r.NewCounter("test_counter", "Test counter", nil)

	if c.Value() != 0 {
		t.Errorf("initial value = %d, want 0", c.Value())
	}

	c.Inc()
	if c.Value() != 1 {
		t.Errorf("after Inc() value = %d, want 1", c.Value())
	}

	c.Add(10)
	if c.Value() != 11 {
		t.Errorf("after Add(10) value = %d, want 11", c.Value())
	}
}

func TestCounterWithLabels(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	c1 := r.NewCounter("http_requests", "HTTP requests", map[string]string{"method": "GET", "path": "/"})
	c2 := r.NewCounter("http_requests", "HTTP requests", map[string]string{"method": "POST", "path": "/"})
	c3 := r.NewCounter("http_requests", "HTTP requests", map[string]string{"method": "GET", "path": "/"})

	c1.Inc()
	c2.Add(5)

	if c1.Value() != 1 {
		t.Errorf("c1 value = %d, want 1", c1.Value())
	}
	if c2.Value() != 5 {
		t.Errorf("c2 value = %d, want 5", c2.Value())
	}
	if c3.Value() != 1 {
		t.Errorf("c3 should be same as c1, value = %d, want 1", c3.Value())
	}
}

func TestGauge(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	g := r.NewGauge("test_gauge", "Test gauge", nil)

	if g.Value() != 0 {
		t.Errorf("initial value = %.3f, want 0", g.Value())
	}

	g.Set(42.5)
	if g.Value() != 42.5 {
		t.Errorf("after Set(42.5) value = %.3f, want 42.5", g.Value())
	}

	g.Inc()
	if g.Value() != 43.5 {
		t.Errorf("after Inc() value = %.3f, want 43.5", g.Value())
	}

	g.Dec()
	if g.Value() != 42.5 {
		t.Errorf("after Dec() value = %.3f, want 42.5", g.Value())
	}
}

func TestHistogram(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	h := r.NewHistogram("test_latency", "Test latency", nil)

	h.Observe(0.005) // 5ms - falls in first bucket
	h.Observe(0.015) // 15ms - falls in 25ms bucket
	h.Observe(0.1)   // 100ms
	h.Observe(1.0)   // 1s

	buckets, sum, count := h.Snapshot()

	if count != 4 {
		t.Errorf("count = %d, want 4", count)
	}

	expectedSum := 0.005 + 0.015 + 0.1 + 1.0
	if sum != expectedSum {
		t.Errorf("sum = %.3f, want %.3f", sum, expectedSum)
	}

	// Check bucket counts (cumulative)
	if buckets[0.005] != 1 { // 5ms bucket
		t.Errorf("bucket[0.005] = %d, want 1", buckets[0.005])
	}
	if buckets[0.1] != 3 { // 100ms bucket (cumulative: 5ms + 15ms + 100ms)
		t.Errorf("bucket[0.1] = %d, want 3", buckets[0.1])
	}
}

func TestHistogram_ObserveDuration(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	h := r.NewHistogram("test_duration", "Test duration", nil)

	start := time.Now()
	time.Sleep(10 * time.Millisecond)
	h.ObserveDuration(start)

	_, sum, count := h.Snapshot()

	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if sum < 0.01 || sum > 0.1 {
		t.Errorf("sum = %.3f, expected ~0.01", sum)
	}
}

func TestRegistry_Render(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	r.NewCounter("http_requests_total", "Total HTTP requests", map[string]string{"method": "GET"}).Add(100)
	r.NewGauge("active_connections", "Active connections", nil).Set(42)
	r.NewHistogram("request_latency_seconds", "Request latency", nil).Observe(0.1)

	output := r.Render()

	// Check counter output
	if !strings.Contains(output, "# TYPE mdemg_http_requests_total counter") {
		t.Error("missing counter TYPE")
	}
	if !strings.Contains(output, `mdemg_http_requests_total{method="GET"} 100`) {
		t.Error("missing counter value")
	}

	// Check gauge output
	if !strings.Contains(output, "# TYPE mdemg_active_connections gauge") {
		t.Error("missing gauge TYPE")
	}
	if !strings.Contains(output, "mdemg_active_connections 42.") {
		t.Error("missing gauge value")
	}

	// Check histogram output
	if !strings.Contains(output, "# TYPE mdemg_request_latency_seconds histogram") {
		t.Error("missing histogram TYPE")
	}
	if !strings.Contains(output, "mdemg_request_latency_seconds_bucket") {
		t.Error("missing histogram buckets")
	}
	if !strings.Contains(output, "mdemg_request_latency_seconds_count 1") {
		t.Error("missing histogram count")
	}
}

func TestHTTPMiddleware(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	handler := HTTPMiddleware(r)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	// Check that metrics were recorded
	output := r.Render()
	if !strings.Contains(output, "mdemg_http_requests_total") {
		t.Error("HTTP request counter not recorded")
	}
	if !strings.Contains(output, "mdemg_http_request_duration_seconds") {
		t.Error("HTTP request duration not recorded")
	}
}

func TestMetricsHandler(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	r.NewCounter("test_metric", "Test metric", nil).Inc()

	handler := MetricsHandler(r)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Content-Type = %s, want text/plain", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "mdemg_test_metric 1") {
		t.Errorf("body doesn't contain expected metric: %s", body)
	}
}

func TestStandardMetrics(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	m := NewStandardMetrics(r)

	// Test HTTP metrics factory functions
	c1 := m.HTTPRequestsTotal("GET", "/test", "200")
	c2 := m.HTTPRequestsTotal("GET", "/test", "200")

	c1.Inc()
	if c2.Value() != 1 {
		t.Error("same labels should return same counter")
	}

	// Test histogram factory
	h := m.HTTPRequestDuration("GET", "/test")
	h.Observe(0.1)
	_, _, count := h.Snapshot()
	if count != 1 {
		t.Errorf("histogram count = %d, want 1", count)
	}

	// Test fixed metrics
	m.RetrievalLatency.Observe(0.05)
	m.RetrievalCacheHits.Inc()
	m.RateLimitRejected.Inc()

	// Test gauge factory
	g := m.CircuitBreakerState("openai")
	g.Set(1)
	if g.Value() != 1 {
		t.Errorf("circuit breaker state = %.1f, want 1", g.Value())
	}
}
