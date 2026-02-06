package embeddings

import (
	"context"
	"errors"
	"testing"
)

// rateLimitMockEmbedder is a simple mock for rate limit testing
type rateLimitMockEmbedder struct {
	name       string
	dimensions int
	embedFn    func(ctx context.Context, text string) ([]float32, error)
}

func (m *rateLimitMockEmbedder) Name() string      { return m.name }
func (m *rateLimitMockEmbedder) Dimensions() int   { return m.dimensions }
func (m *rateLimitMockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, text)
	}
	return make([]float32, m.dimensions), nil
}
func (m *rateLimitMockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = emb
	}
	return results, nil
}

func TestRateLimitedEmbedder_Name(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 768}
	rl := NewRateLimitedEmbedder(mock, 100, 200, true)

	if rl.Name() != "test+ratelimit" {
		t.Errorf("expected 'test+ratelimit', got '%s'", rl.Name())
	}
}

func TestRateLimitedEmbedder_Dimensions(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 1536}
	rl := NewRateLimitedEmbedder(mock, 100, 200, true)

	if rl.Dimensions() != 1536 {
		t.Errorf("expected 1536 dimensions, got %d", rl.Dimensions())
	}
}

func TestRateLimitedEmbedder_Disabled(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 768}
	rl := NewRateLimitedEmbedder(mock, 0.001, 1, false) // Very low rate but disabled

	// Should succeed even with low rate when disabled
	for i := 0; i < 10; i++ {
		_, err := rl.Embed(context.Background(), "test")
		if err != nil {
			t.Errorf("disabled rate limiter should pass: %v", err)
		}
	}

	if rl.RejectedCount() != 0 {
		t.Errorf("expected 0 rejected when disabled, got %d", rl.RejectedCount())
	}
}

func TestRateLimitedEmbedder_RateLimited(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 768}
	// Very low rate: 0.001 RPS, burst 1 - should allow 1 request then reject
	rl := NewRateLimitedEmbedder(mock, 0.001, 1, true)

	// First request should succeed (uses the initial burst)
	_, err := rl.Embed(context.Background(), "test")
	if err != nil {
		t.Errorf("first request should succeed: %v", err)
	}

	// Second request should be rate limited
	_, err = rl.Embed(context.Background(), "test2")
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}

	if rl.RejectedCount() != 1 {
		t.Errorf("expected 1 rejected, got %d", rl.RejectedCount())
	}
}

func TestRateLimitedEmbedder_EmbedBatch(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 768}
	// Allow burst of 5
	rl := NewRateLimitedEmbedder(mock, 0.001, 5, true)

	// Batch of 3 should succeed
	texts := []string{"a", "b", "c"}
	results, err := rl.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Errorf("batch of 3 should succeed with burst 5: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Next batch of 3 should fail (only 2 tokens left)
	_, err = rl.EmbedBatch(context.Background(), texts)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited for batch exceeding remaining tokens, got %v", err)
	}
}

func TestRateLimitedEmbedder_Stats(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 768}
	rl := NewRateLimitedEmbedder(mock, 100, 200, true)

	stats := rl.Stats()

	if stats["enabled"] != true {
		t.Error("expected enabled=true")
	}
	if stats["rate_per_second"].(float64) != 100 {
		t.Errorf("expected rate 100, got %v", stats["rate_per_second"])
	}
	if stats["burst"].(int) != 200 {
		t.Errorf("expected burst 200, got %v", stats["burst"])
	}
}

func TestRateLimitedEmbedder_Unwrap(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 768}
	rl := NewRateLimitedEmbedder(mock, 100, 200, true)

	unwrapped := rl.Unwrap()
	if unwrapped != mock {
		t.Error("Unwrap should return original embedder")
	}
}

func TestRateLimitedEmbedder_PassesErrors(t *testing.T) {
	expectedErr := errors.New("embed error")
	mock := &rateLimitMockEmbedder{
		name:       "test",
		dimensions: 768,
		embedFn: func(ctx context.Context, text string) ([]float32, error) {
			return nil, expectedErr
		},
	}
	rl := NewRateLimitedEmbedder(mock, 100, 200, true)

	_, err := rl.Embed(context.Background(), "test")
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error from embedder to pass through, got %v", err)
	}
}

func TestRateLimitedEmbedder_AvailableTokens(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 768}
	rl := NewRateLimitedEmbedder(mock, 100, 200, true)

	// Initial tokens should be equal to burst
	tokens := rl.AvailableTokens()
	if tokens != 200.0 {
		t.Errorf("expected 200 initial tokens, got %v", tokens)
	}

	// After one request, tokens should be reduced
	_, _ = rl.Embed(context.Background(), "test")
	tokensAfter := rl.AvailableTokens()
	if tokensAfter >= tokens {
		t.Errorf("expected tokens to decrease after request, got %v (was %v)", tokensAfter, tokens)
	}
}

func TestRateLimitedEmbedder_DisabledAvailableTokens(t *testing.T) {
	mock := &rateLimitMockEmbedder{name: "test", dimensions: 768}
	rl := NewRateLimitedEmbedder(mock, 100, 200, false) // disabled

	// When disabled, should still report tokens (limiter exists but not enforced)
	tokens := rl.AvailableTokens()
	if tokens != 200.0 {
		t.Errorf("expected 200 tokens even when disabled, got %v", tokens)
	}
}
