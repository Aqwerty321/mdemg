// Package optimistic provides retry logic with exponential backoff for
// optimistic concurrency control on Neo4j node and edge updates.
package optimistic

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// Error types for optimistic locking operations.
var (
	// ErrVersionMismatch indicates expected version did not match actual version.
	ErrVersionMismatch = errors.New("version mismatch: expected version does not match actual")

	// ErrRetriesExhausted indicates all retry attempts failed.
	ErrRetriesExhausted = errors.New("retries exhausted: operation failed after maximum attempts")

	// ErrContextCanceled indicates the context was canceled during retry.
	ErrContextCanceled = errors.New("context canceled during retry operation")
)

// VersionMismatchError provides detailed information about a version conflict.
type VersionMismatchError struct {
	EntityType string // "node" or "edge"
	EntityID   string
	SpaceID    string
	Expected   int64
	Actual     int64
	Operation  string
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("version mismatch on %s %s (space=%s): expected=%d, actual=%d, op=%s",
		e.EntityType, e.EntityID, e.SpaceID, e.Expected, e.Actual, e.Operation)
}

func (e *VersionMismatchError) Unwrap() error {
	return ErrVersionMismatch
}

// RetryConfig configures exponential backoff behavior for optimistic lock retries.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (default: 5).
	MaxRetries int

	// BaseDelay is the initial delay before first retry (default: 10ms).
	BaseDelay time.Duration

	// MaxDelay is the maximum delay between retries (default: 1s).
	MaxDelay time.Duration

	// Multiplier is the exponential backoff multiplier (default: 2.0).
	Multiplier float64

	// JitterFactor adds randomness to prevent thundering herd (default: 0.2, meaning ±20%).
	JitterFactor float64
}

// DefaultConfig returns a RetryConfig with sensible defaults.
func DefaultConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   5,
		BaseDelay:    10 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.2,
	}
}

// Validate checks that the config has valid values.
func (c RetryConfig) Validate() error {
	if c.MaxRetries < 0 {
		return errors.New("MaxRetries must be >= 0")
	}
	if c.BaseDelay < 0 {
		return errors.New("BaseDelay must be >= 0")
	}
	if c.MaxDelay < c.BaseDelay {
		return errors.New("MaxDelay must be >= BaseDelay")
	}
	if c.Multiplier < 1.0 {
		return errors.New("Multiplier must be >= 1.0")
	}
	if c.JitterFactor < 0 || c.JitterFactor > 1.0 {
		return errors.New("JitterFactor must be in range [0, 1]")
	}
	return nil
}

// CalculateBackoff computes the delay for a given attempt number.
// attempt is 0-indexed (attempt 0 = first retry after initial failure).
func (c RetryConfig) CalculateBackoff(attempt int) time.Duration {
	if attempt < 0 {
		return c.BaseDelay
	}

	// Exponential growth: baseDelay * multiplier^attempt
	delay := float64(c.BaseDelay) * math.Pow(c.Multiplier, float64(attempt))

	// Cap at MaxDelay
	if delay > float64(c.MaxDelay) {
		delay = float64(c.MaxDelay)
	}

	// Add jitter: ±JitterFactor
	if c.JitterFactor > 0 {
		jitter := delay * c.JitterFactor * (2*rand.Float64() - 1) // range: [-jitter, +jitter]
		delay += jitter
	}

	// Ensure non-negative
	if delay < 0 {
		delay = 0
	}

	return time.Duration(delay)
}

// RetryResult contains information about a completed retry operation.
type RetryResult struct {
	// Attempts is the total number of attempts made (1 = success on first try).
	Attempts int

	// TotalDuration is the total time spent including all retries.
	TotalDuration time.Duration

	// FinalError is the error from the final attempt, if operation failed.
	FinalError error

	// VersionConflicts counts how many ErrVersionMismatch errors occurred.
	VersionConflicts int
}

// OperationFunc is the function type for retryable operations.
// The function should return ErrVersionMismatch (or a wrapped VersionMismatchError)
// to trigger a retry, or any other error to fail immediately.
type OperationFunc func(ctx context.Context) error

// WithRetry executes an operation with exponential backoff retry on version conflicts.
// It returns a RetryResult with statistics about the operation.
//
// The operation function should:
// - Return nil on success
// - Return ErrVersionMismatch or *VersionMismatchError to trigger retry
// - Return any other error to fail immediately without retry
func WithRetry(ctx context.Context, cfg RetryConfig, op OperationFunc) RetryResult {
	start := time.Now()
	result := RetryResult{}

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		result.Attempts = attempt + 1

		// Check context before attempting
		if ctx.Err() != nil {
			result.FinalError = ErrContextCanceled
			result.TotalDuration = time.Since(start)
			return result
		}

		// Execute the operation
		err := op(ctx)
		if err == nil {
			// Success
			result.TotalDuration = time.Since(start)
			return result
		}

		// Check if this is a version mismatch error (retryable)
		if errors.Is(err, ErrVersionMismatch) {
			result.VersionConflicts++

			// If we've exhausted retries, fail
			if attempt >= cfg.MaxRetries {
				result.FinalError = fmt.Errorf("%w: %v", ErrRetriesExhausted, err)
				result.TotalDuration = time.Since(start)
				return result
			}

			// Calculate backoff and wait
			delay := cfg.CalculateBackoff(attempt)
			select {
			case <-ctx.Done():
				result.FinalError = ErrContextCanceled
				result.TotalDuration = time.Since(start)
				return result
			case <-time.After(delay):
				// Continue to next attempt
			}
			continue
		}

		// Non-retryable error - fail immediately
		result.FinalError = err
		result.TotalDuration = time.Since(start)
		return result
	}

	// Should not reach here, but just in case
	result.FinalError = ErrRetriesExhausted
	result.TotalDuration = time.Since(start)
	return result
}

// WithRetrySimple is a convenience wrapper that returns only an error.
// Use WithRetry when you need retry statistics.
func WithRetrySimple(ctx context.Context, cfg RetryConfig, op OperationFunc) error {
	result := WithRetry(ctx, cfg, op)
	return result.FinalError
}
