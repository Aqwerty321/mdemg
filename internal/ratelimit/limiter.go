package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a token bucket rate limiter.
// It is safe for concurrent use.
type Limiter struct {
	mu sync.Mutex

	// rate is tokens per second
	rate float64

	// burst is max tokens
	burst int

	// tokens is current token count
	tokens float64

	// lastRefill is the last time tokens were added
	lastRefill time.Time
}

// NewLimiter creates a rate limiter with the given rate (tokens/sec) and burst size.
func NewLimiter(rate float64, burst int) *Limiter {
	return &Limiter{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst), // Start full
		lastRefill: time.Now(),
	}
}

// Allow returns true if a request is allowed, consuming one token.
// Returns false if the rate limit is exceeded.
func (l *Limiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN returns true if n tokens can be consumed.
func (l *Limiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return true
	}
	return false
}

// refill adds tokens based on elapsed time since last refill.
// Must be called with l.mu held.
func (l *Limiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.lastRefill = now

	l.tokens += elapsed * l.rate
	if l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}
}

// Tokens returns the current number of available tokens.
func (l *Limiter) Tokens() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill()
	return l.tokens
}

// Rate returns the rate limit in tokens per second.
func (l *Limiter) Rate() float64 {
	return l.rate
}

// Burst returns the burst size.
func (l *Limiter) Burst() int {
	return l.burst
}

// LimiterStore manages per-key rate limiters (e.g., per-IP).
type LimiterStore struct {
	mu       sync.RWMutex
	limiters map[string]*Limiter
	rate     float64
	burst    int

	// cleanupInterval controls how often expired limiters are cleaned
	cleanupInterval time.Duration
	// maxAge is how long since last use before a limiter is removed
	maxAge time.Duration
}

// NewLimiterStore creates a store for per-key rate limiters.
func NewLimiterStore(rate float64, burst int) *LimiterStore {
	ls := &LimiterStore{
		limiters:        make(map[string]*Limiter),
		rate:            rate,
		burst:           burst,
		cleanupInterval: 5 * time.Minute,
		maxAge:          10 * time.Minute,
	}
	go ls.periodicCleanup()
	return ls
}

// GetLimiter returns the limiter for a key, creating one if needed.
func (ls *LimiterStore) GetLimiter(key string) *Limiter {
	ls.mu.RLock()
	limiter, exists := ls.limiters[key]
	ls.mu.RUnlock()

	if exists {
		return limiter
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists = ls.limiters[key]; exists {
		return limiter
	}

	limiter = NewLimiter(ls.rate, ls.burst)
	ls.limiters[key] = limiter
	return limiter
}

// periodicCleanup removes stale limiters to prevent memory leaks.
func (ls *LimiterStore) periodicCleanup() {
	ticker := time.NewTicker(ls.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		ls.cleanup()
	}
}

// cleanup removes limiters that haven't been used recently.
func (ls *LimiterStore) cleanup() {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	now := time.Now()
	for key, limiter := range ls.limiters {
		limiter.mu.Lock()
		age := now.Sub(limiter.lastRefill)
		limiter.mu.Unlock()

		if age > ls.maxAge {
			delete(ls.limiters, key)
		}
	}
}

// Size returns the number of active limiters.
func (ls *LimiterStore) Size() int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return len(ls.limiters)
}
