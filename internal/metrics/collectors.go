package metrics

import (
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/db"
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

	// Neo4j pool metrics (Phase 48.4.1)
	// Note: Using gauges for all metrics since we read absolute values from driver
	Neo4jPoolActive   *Gauge // Current active connections
	Neo4jPoolIdle     *Gauge // Current idle connections
	Neo4jPoolWaiting  *Gauge // Waiting requests
	Neo4jPoolAcquired *Gauge // Total connections acquired (absolute value from driver)
	Neo4jPoolCreated  *Gauge // Total connections created (absolute value from driver)
	Neo4jPoolClosed   *Gauge // Total connections closed (absolute value from driver)
	Neo4jPoolFailed   *Gauge // Total failed acquire attempts (absolute value from driver)

	// Memory pressure metrics (Phase 48.4.4)
	MemoryPressureRejected *Gauge // Requests rejected due to memory pressure (cumulative)
	MemoryHeapBytes        *Gauge // Current heap allocation in bytes
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

	// Neo4j pool metrics (Phase 48.4.1)
	m.Neo4jPoolActive = r.NewGauge("neo4j_pool_active_connections", "Current active Neo4j connections", nil)
	m.Neo4jPoolIdle = r.NewGauge("neo4j_pool_idle_connections", "Current idle Neo4j connections", nil)
	m.Neo4jPoolWaiting = r.NewGauge("neo4j_pool_waiting_requests", "Requests waiting for Neo4j connection", nil)
	m.Neo4jPoolAcquired = r.NewGauge("neo4j_pool_acquired_total", "Total Neo4j connections acquired", nil)
	m.Neo4jPoolCreated = r.NewGauge("neo4j_pool_created_total", "Total Neo4j connections created", nil)
	m.Neo4jPoolClosed = r.NewGauge("neo4j_pool_closed_total", "Total Neo4j connections closed", nil)
	m.Neo4jPoolFailed = r.NewGauge("neo4j_pool_failed_acquire_total", "Total failed Neo4j connection acquire attempts", nil)

	// Memory pressure metrics (Phase 48.4.4)
	m.MemoryPressureRejected = r.NewGauge("memory_pressure_rejected_total", "Requests rejected due to memory pressure", nil)
	m.MemoryHeapBytes = r.NewGauge("memory_heap_bytes", "Current heap allocation in bytes", nil)

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

// CollectNeo4jPoolMetrics updates Neo4j connection pool metrics.
func (m *StandardMetrics) CollectNeo4jPoolMetrics() {
	poolMetrics := db.GetPoolMetrics()

	m.Neo4jPoolActive.Set(float64(poolMetrics.ActiveConnections))
	m.Neo4jPoolIdle.Set(float64(poolMetrics.IdleConnections))
	m.Neo4jPoolWaiting.Set(float64(poolMetrics.WaitingRequests))
	m.Neo4jPoolAcquired.Set(float64(poolMetrics.TotalAcquired))
	m.Neo4jPoolCreated.Set(float64(poolMetrics.TotalCreated))
	m.Neo4jPoolClosed.Set(float64(poolMetrics.TotalClosed))
	m.Neo4jPoolFailed.Set(float64(poolMetrics.TotalFailedAcquire))
}

// CollectMemoryMetrics updates memory metrics from runtime stats.
func (m *StandardMetrics) CollectMemoryMetrics(heapBytes uint64, rejectedCount int64) {
	m.MemoryHeapBytes.Set(float64(heapBytes))
	m.MemoryPressureRejected.Set(float64(rejectedCount))
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
