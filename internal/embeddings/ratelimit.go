package embeddings

import (
	"context"
	"errors"
	"sync/atomic"

	"mdemg/internal/ratelimit"
)

// ErrRateLimited is returned when a request is rejected due to rate limiting.
var ErrRateLimited = errors.New("embedding rate limit exceeded")

// RateLimitedEmbedder wraps an Embedder with rate limiting.
type RateLimitedEmbedder struct {
	embedder        Embedder
	limiter         *ratelimit.Limiter
	enabled         bool
	rejectedCount   atomic.Int64
}

// NewRateLimitedEmbedder creates a rate-limited wrapper around an embedder.
// rps is the requests per second limit, burst is the maximum burst allowance.
func NewRateLimitedEmbedder(embedder Embedder, rps float64, burst int, enabled bool) *RateLimitedEmbedder {
	return &RateLimitedEmbedder{
		embedder: embedder,
		limiter:  ratelimit.NewLimiter(rps, burst),
		enabled:  enabled,
	}
}

// Name returns the underlying embedder's name with a rate-limit suffix.
func (r *RateLimitedEmbedder) Name() string {
	return r.embedder.Name() + "+ratelimit"
}

// Dimensions returns the embedding dimensions.
func (r *RateLimitedEmbedder) Dimensions() int {
	return r.embedder.Dimensions()
}

// Embed generates an embedding for a single text with rate limiting.
func (r *RateLimitedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if r.enabled && !r.limiter.Allow() {
		r.rejectedCount.Add(1)
		return nil, ErrRateLimited
	}
	return r.embedder.Embed(ctx, text)
}

// EmbedBatch generates embeddings for multiple texts with rate limiting.
// Uses AllowN to check if the batch can proceed.
func (r *RateLimitedEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if r.enabled && !r.limiter.AllowN(len(texts)) {
		r.rejectedCount.Add(1)
		return nil, ErrRateLimited
	}
	return r.embedder.EmbedBatch(ctx, texts)
}

// RejectedCount returns the total number of requests rejected by rate limiting.
func (r *RateLimitedEmbedder) RejectedCount() int64 {
	return r.rejectedCount.Load()
}

// AvailableTokens returns the current number of available rate limit tokens.
func (r *RateLimitedEmbedder) AvailableTokens() float64 {
	return r.limiter.Tokens()
}

// Unwrap returns the underlying embedder.
func (r *RateLimitedEmbedder) Unwrap() Embedder {
	return r.embedder
}

// Stats returns rate limiting statistics.
func (r *RateLimitedEmbedder) Stats() map[string]any {
	return map[string]any{
		"enabled":          r.enabled,
		"rate_per_second":  r.limiter.Rate(),
		"burst":            r.limiter.Burst(),
		"available_tokens": r.limiter.Tokens(),
		"rejected_count":   r.rejectedCount.Load(),
	}
}
