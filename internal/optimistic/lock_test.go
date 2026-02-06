package optimistic

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 10*time.Millisecond {
		t.Errorf("BaseDelay = %v, want 10ms", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 1*time.Second {
		t.Errorf("MaxDelay = %v, want 1s", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", cfg.Multiplier)
	}
	if cfg.JitterFactor != 0.2 {
		t.Errorf("JitterFactor = %f, want 0.2", cfg.JitterFactor)
	}
}

func TestRetryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     RetryConfig
		wantErr bool
	}{
		{
			name:    "default config is valid",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
		{
			name: "negative MaxRetries",
			cfg: RetryConfig{
				MaxRetries:   -1,
				BaseDelay:    10 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   2.0,
				JitterFactor: 0.2,
			},
			wantErr: true,
		},
		{
			name: "negative BaseDelay",
			cfg: RetryConfig{
				MaxRetries:   5,
				BaseDelay:    -1 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   2.0,
				JitterFactor: 0.2,
			},
			wantErr: true,
		},
		{
			name: "MaxDelay less than BaseDelay",
			cfg: RetryConfig{
				MaxRetries:   5,
				BaseDelay:    1 * time.Second,
				MaxDelay:     10 * time.Millisecond,
				Multiplier:   2.0,
				JitterFactor: 0.2,
			},
			wantErr: true,
		},
		{
			name: "Multiplier less than 1",
			cfg: RetryConfig{
				MaxRetries:   5,
				BaseDelay:    10 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   0.5,
				JitterFactor: 0.2,
			},
			wantErr: true,
		},
		{
			name: "JitterFactor negative",
			cfg: RetryConfig{
				MaxRetries:   5,
				BaseDelay:    10 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   2.0,
				JitterFactor: -0.1,
			},
			wantErr: true,
		},
		{
			name: "JitterFactor greater than 1",
			cfg: RetryConfig{
				MaxRetries:   5,
				BaseDelay:    10 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   2.0,
				JitterFactor: 1.5,
			},
			wantErr: true,
		},
		{
			name: "zero retries is valid",
			cfg: RetryConfig{
				MaxRetries:   0,
				BaseDelay:    10 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   1.0,
				JitterFactor: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCalculateBackoff_ExponentialGrowth(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		BaseDelay:    10 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0, // No jitter for deterministic tests
	}

	// Expected delays: 10ms, 20ms, 40ms, 80ms, 160ms
	expected := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
		80 * time.Millisecond,
		160 * time.Millisecond,
	}

	for attempt, want := range expected {
		got := cfg.CalculateBackoff(attempt)
		if got != want {
			t.Errorf("CalculateBackoff(%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestCalculateBackoff_MaxDelayCap(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   10,
		BaseDelay:    100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0,
	}

	// Attempt 3: 100 * 2^3 = 800ms, but capped at 500ms
	got := cfg.CalculateBackoff(3)
	if got != cfg.MaxDelay {
		t.Errorf("CalculateBackoff(3) = %v, want MaxDelay %v", got, cfg.MaxDelay)
	}

	// Attempt 10 should still be capped
	got = cfg.CalculateBackoff(10)
	if got != cfg.MaxDelay {
		t.Errorf("CalculateBackoff(10) = %v, want MaxDelay %v", got, cfg.MaxDelay)
	}
}

func TestCalculateBackoff_WithJitter(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		BaseDelay:    100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.2, // ±20%
	}

	// Run multiple times to ensure jitter is applied
	baseExpected := 100 * time.Millisecond
	minExpected := time.Duration(float64(baseExpected) * 0.8)
	maxExpected := time.Duration(float64(baseExpected) * 1.2)

	for i := 0; i < 100; i++ {
		got := cfg.CalculateBackoff(0)
		if got < minExpected || got > maxExpected {
			t.Errorf("CalculateBackoff(0) = %v, want in range [%v, %v]", got, minExpected, maxExpected)
		}
	}
}

func TestCalculateBackoff_NegativeAttempt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JitterFactor = 0

	got := cfg.CalculateBackoff(-1)
	if got != cfg.BaseDelay {
		t.Errorf("CalculateBackoff(-1) = %v, want BaseDelay %v", got, cfg.BaseDelay)
	}
}

func TestWithRetry_SuccessOnFirstTry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond // Fast for testing

	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		return nil
	}

	result := WithRetry(context.Background(), cfg, op)

	if result.FinalError != nil {
		t.Errorf("FinalError = %v, want nil", result.FinalError)
	}
	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}
	if result.VersionConflicts != 0 {
		t.Errorf("VersionConflicts = %d, want 0", result.VersionConflicts)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestWithRetry_SuccessAfterRetry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.JitterFactor = 0

	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		if callCount < 3 {
			return ErrVersionMismatch
		}
		return nil
	}

	result := WithRetry(context.Background(), cfg, op)

	if result.FinalError != nil {
		t.Errorf("FinalError = %v, want nil", result.FinalError)
	}
	if result.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", result.Attempts)
	}
	if result.VersionConflicts != 2 {
		t.Errorf("VersionConflicts = %d, want 2", result.VersionConflicts)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestWithRetry_RetriesExhausted(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   3,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0,
	}

	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		return ErrVersionMismatch
	}

	result := WithRetry(context.Background(), cfg, op)

	if !errors.Is(result.FinalError, ErrRetriesExhausted) {
		t.Errorf("FinalError = %v, want ErrRetriesExhausted", result.FinalError)
	}
	if result.Attempts != 4 { // Initial + 3 retries
		t.Errorf("Attempts = %d, want 4", result.Attempts)
	}
	if result.VersionConflicts != 4 {
		t.Errorf("VersionConflicts = %d, want 4", result.VersionConflicts)
	}
	if callCount != 4 {
		t.Errorf("callCount = %d, want 4", callCount)
	}
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond

	customErr := errors.New("database connection failed")
	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		return customErr
	}

	result := WithRetry(context.Background(), cfg, op)

	if !errors.Is(result.FinalError, customErr) {
		t.Errorf("FinalError = %v, want %v", result.FinalError, customErr)
	}
	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}
	if result.VersionConflicts != 0 {
		t.Errorf("VersionConflicts = %d, want 0", result.VersionConflicts)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (no retry for non-retryable error)", callCount)
	}
}

func TestWithRetry_VersionMismatchErrorWrapped(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxRetries = 1
	cfg.BaseDelay = 1 * time.Millisecond

	vmErr := &VersionMismatchError{
		EntityType: "node",
		EntityID:   "n123",
		SpaceID:    "test-space",
		Expected:   5,
		Actual:     6,
		Operation:  "update",
	}

	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		return vmErr
	}

	result := WithRetry(context.Background(), cfg, op)

	// Should retry because VersionMismatchError wraps ErrVersionMismatch
	if !errors.Is(result.FinalError, ErrRetriesExhausted) {
		t.Errorf("FinalError = %v, want ErrRetriesExhausted", result.FinalError)
	}
	if result.Attempts != 2 { // Initial + 1 retry
		t.Errorf("Attempts = %d, want 2", result.Attempts)
	}
	if result.VersionConflicts != 2 {
		t.Errorf("VersionConflicts = %d, want 2", result.VersionConflicts)
	}
}

func TestWithRetry_ContextCanceled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 100 * time.Millisecond // Longer delay to allow cancellation

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		if callCount == 1 {
			// Cancel after first call
			cancel()
		}
		return ErrVersionMismatch
	}

	result := WithRetry(ctx, cfg, op)

	if !errors.Is(result.FinalError, ErrContextCanceled) {
		t.Errorf("FinalError = %v, want ErrContextCanceled", result.FinalError)
	}
}

func TestWithRetry_ContextAlreadyCanceled(t *testing.T) {
	cfg := DefaultConfig()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before calling

	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		return nil
	}

	result := WithRetry(ctx, cfg, op)

	if !errors.Is(result.FinalError, ErrContextCanceled) {
		t.Errorf("FinalError = %v, want ErrContextCanceled", result.FinalError)
	}
	if callCount != 0 {
		t.Errorf("callCount = %d, want 0 (should not execute if context already canceled)", callCount)
	}
}

func TestWithRetry_ZeroRetries(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   0,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0,
	}

	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		return ErrVersionMismatch
	}

	result := WithRetry(context.Background(), cfg, op)

	// With 0 retries, should try once and fail
	if !errors.Is(result.FinalError, ErrRetriesExhausted) {
		t.Errorf("FinalError = %v, want ErrRetriesExhausted", result.FinalError)
	}
	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestWithRetrySimple(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond

	err := WithRetrySimple(context.Background(), cfg, func(ctx context.Context) error {
		return nil
	})

	if err != nil {
		t.Errorf("WithRetrySimple error = %v, want nil", err)
	}
}

func TestVersionMismatchError(t *testing.T) {
	err := &VersionMismatchError{
		EntityType: "node",
		EntityID:   "n123",
		SpaceID:    "test-space",
		Expected:   5,
		Actual:     6,
		Operation:  "update_content",
	}

	// Test Error() string
	msg := err.Error()
	if msg == "" {
		t.Error("Error() returned empty string")
	}

	// Test Unwrap()
	if !errors.Is(err, ErrVersionMismatch) {
		t.Error("VersionMismatchError should unwrap to ErrVersionMismatch")
	}
}

func TestWithRetry_TotalDuration(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   2,
		BaseDelay:    10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0,
	}

	callCount := 0
	op := func(ctx context.Context) error {
		callCount++
		if callCount < 3 {
			return ErrVersionMismatch
		}
		return nil
	}

	start := time.Now()
	result := WithRetry(context.Background(), cfg, op)
	elapsed := time.Since(start)

	// Expected delays: 10ms after 1st fail, 20ms after 2nd fail = 30ms minimum
	if result.TotalDuration < 30*time.Millisecond {
		t.Errorf("TotalDuration = %v, want >= 30ms", result.TotalDuration)
	}

	// Sanity check that elapsed time matches
	if elapsed < 30*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 30ms", elapsed)
	}
}
