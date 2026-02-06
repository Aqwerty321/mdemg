package metrics

import (
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/ratelimit"
)

// StandardMetrics holds pre-registered standard metrics.
type StandardMetrics struct {
	// HTTP metrics
	HTTPRequestsTotal   func(method, path, status string) *Counter
	HTTPRequestDuration func(method, path string) *Histogram

	// Retrieval metrics
	RetrievalLatency    *Histogram
	RetrievalCacheHits  *Counter
	RetrievalCacheMiss  *Counter

	// Rate limiting metrics
	RateLimitRejected *Counter

	// Circuit breaker metrics (by service)
	CircuitBreakerState func(service string) *Gauge

	// Cache metrics
	CacheHitRatio func(cache string) *Gauge

	// Embedding metrics
	EmbeddingLatency *Histogram
	EmbeddingBatches *Counter
}

// NewStandardMetrics creates and registers all standard MDEMG metrics.
func NewStandardMetrics(r *Registry) *StandardMetrics {
	m := &StandardMetrics{}

	// HTTP metrics - use factory functions for labeled metrics
	httpReqCounters := make(map[string]*Counter)
	m.HTTPRequestsTotal = func(method, path, status string) *Counter {
		labels := map[string]string{"method": method, "path": normalizePath(path), "status": status}
		return r.NewCounter("http_requests_total", "Total HTTP requests", labels)
	}

	httpReqHistograms := make(map[string]*Histogram)
	_ = httpReqHistograms // silence unused warning
	m.HTTPRequestDuration = func(method, path string) *Histogram {
		labels := map[string]string{"method": method, "path": normalizePath(path)}
		return r.NewHistogram("http_request_duration_seconds", "HTTP request latency in seconds", labels)
	}
	_ = httpReqCounters // silence unused warning

	// Retrieval metrics
	m.RetrievalLatency = r.NewHistogram("retrieval_latency_seconds", "Retrieval operation latency", nil)
	m.RetrievalCacheHits = r.NewCounter("retrieval_cache_hits_total", "Retrieval cache hits", nil)
	m.RetrievalCacheMiss = r.NewCounter("retrieval_cache_misses_total", "Retrieval cache misses", nil)

	// Rate limiting
	m.RateLimitRejected = r.NewCounter("rate_limit_rejected_total", "Requests rejected by rate limiting", nil)

	// Circuit breaker
	m.CircuitBreakerState = func(service string) *Gauge {
		labels := map[string]string{"service": service}
		return r.NewGauge("circuit_breaker_state", "Circuit breaker state (0=closed, 1=open, 2=half-open)", labels)
	}

	// Cache hit ratio
	m.CacheHitRatio = func(cache string) *Gauge {
		labels := map[string]string{"cache": cache}
		return r.NewGauge("cache_hit_ratio", "Cache hit ratio (0-1)", labels)
	}

	// Embedding metrics
	m.EmbeddingLatency = r.NewHistogram("embedding_latency_seconds", "Embedding operation latency", nil)
	m.EmbeddingBatches = r.NewCounter("embedding_batches_total", "Total embedding batch operations", nil)

	return m
}

// normalizePath normalizes an HTTP path for metric labels.
// Replaces dynamic path segments (UUIDs, IDs) with placeholders.
func normalizePath(path string) string {
	// Common patterns to normalize
	// /v1/memory/nodes/{uuid} -> /v1/memory/nodes/:id
	// /v1/plugins/{name} -> /v1/plugins/:name

	// Simple normalization: truncate at known dynamic segments
	if len(path) > 50 {
		return path[:50] + "..."
	}
	return path
}

// CollectRateLimitMetrics updates rate limit metrics from the ratelimit package.
func (m *StandardMetrics) CollectRateLimitMetrics() {
	// Get current rejected count from ratelimit package
	m.RateLimitRejected.Add(ratelimit.RejectedTotal())
}

// CollectCircuitBreakerMetrics updates circuit breaker metrics from a registry.
func (m *StandardMetrics) CollectCircuitBreakerMetrics(cbRegistry *circuitbreaker.Registry) {
	if cbRegistry == nil {
		return
	}

	for name, state := range cbRegistry.States() {
		gauge := m.CircuitBreakerState(name)
		switch state {
		case circuitbreaker.StateClosed:
			gauge.Set(0)
		case circuitbreaker.StateOpen:
			gauge.Set(1)
		case circuitbreaker.StateHalfOpen:
			gauge.Set(2)
		}
	}
}

// CollectCacheMetrics updates cache hit ratio metrics from cache stats.
func (m *StandardMetrics) CollectCacheMetrics(cacheStats map[string]map[string]any) {
	for cacheName, stats := range cacheStats {
		if hitRate, ok := stats["hit_rate"].(float64); ok {
			gauge := m.CacheHitRatio(cacheName)
			gauge.Set(hitRate)
		}
	}
}

// global standard metrics instance
var globalMetrics *StandardMetrics

// InitStandardMetrics initializes the global standard metrics.
func InitStandardMetrics() *StandardMetrics {
	globalMetrics = NewStandardMetrics(globalRegistry)
	return globalMetrics
}

// Metrics returns the global standard metrics.
func Metrics() *StandardMetrics {
	if globalMetrics == nil {
		return InitStandardMetrics()
	}
	return globalMetrics
}
