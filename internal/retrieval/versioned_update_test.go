package retrieval

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mdemg/internal/optimistic"
)

// TestConcurrentNodeUpdates tests that version checking detects concurrent updates.
// This is a unit test that simulates the behavior without a real database.
func TestConcurrentNodeUpdates_VersionMismatch(t *testing.T) {
	// Simulate a shared version counter
	var currentVersion int64 = 1
	var mu sync.Mutex

	// Simulate multiple concurrent updaters
	updateCount := 0
	conflictCount := 0

	cfg := optimistic.RetryConfig{
		MaxRetries:   5,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0,
	}

	var wg sync.WaitGroup
	numWorkers := 5

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			result := optimistic.WithRetry(context.Background(), cfg, func(ctx context.Context) error {
				mu.Lock()
				expectedVersion := currentVersion
				mu.Unlock()

				// Simulate delay during update (allows other workers to interleave)
				time.Sleep(1 * time.Millisecond)

				mu.Lock()
				defer mu.Unlock()

				if currentVersion != expectedVersion {
					// Version changed - conflict!
					return &optimistic.VersionMismatchError{
						EntityType: "node",
						EntityID:   "test-node",
						SpaceID:    "test-space",
						Expected:   expectedVersion,
						Actual:     currentVersion,
						Operation:  "update",
					}
				}

				// Success - increment version
				currentVersion++
				updateCount++
				return nil
			})

			if result.VersionConflicts > 0 {
				mu.Lock()
				conflictCount += result.VersionConflicts
				mu.Unlock()
			}

			if result.FinalError != nil {
				t.Errorf("worker %d failed: %v", workerID, result.FinalError)
			}
		}(i)
	}

	wg.Wait()

	// All workers should succeed
	if updateCount != numWorkers {
		t.Errorf("updateCount = %d, want %d", updateCount, numWorkers)
	}

	// With concurrent updates, we expect some conflicts
	t.Logf("Completed with %d total version conflicts resolved", conflictCount)
}

// TestVersionMismatchError tests the error type behavior.
func TestVersionMismatchError(t *testing.T) {
	err := &optimistic.VersionMismatchError{
		EntityType: "node",
		EntityID:   "n123",
		SpaceID:    "test-space",
		Expected:   5,
		Actual:     6,
		Operation:  "update_content",
	}

	// Test Error() returns non-empty string
	msg := err.Error()
	if msg == "" {
		t.Error("Error() returned empty string")
	}
	t.Logf("Error message: %s", msg)

	// Test Unwrap() returns ErrVersionMismatch
	if !errors.Is(err, optimistic.ErrVersionMismatch) {
		t.Error("VersionMismatchError should unwrap to ErrVersionMismatch")
	}
}

// TestConcurrentEdgeUpdates_VersionMismatch simulates concurrent edge updates.
func TestConcurrentEdgeUpdates_VersionMismatch(t *testing.T) {
	var currentVersion int64 = 1
	var mu sync.Mutex
	var successfulUpdates int32

	cfg := optimistic.RetryConfig{
		MaxRetries:   10, // More retries due to higher contention
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0.2, // Add jitter to reduce thundering herd
	}

	var wg sync.WaitGroup
	numWorkers := 10

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			result := optimistic.WithRetry(context.Background(), cfg, func(ctx context.Context) error {
				mu.Lock()
				expectedVersion := currentVersion
				mu.Unlock()

				// Simulate database round-trip delay
				time.Sleep(time.Duration(workerID%3+1) * time.Millisecond)

				mu.Lock()
				defer mu.Unlock()

				if currentVersion != expectedVersion {
					return &optimistic.VersionMismatchError{
						EntityType: "edge",
						EntityID:   "src->dst",
						SpaceID:    "test-space",
						Expected:   expectedVersion,
						Actual:     currentVersion,
						Operation:  "update_weight",
					}
				}

				currentVersion++
				atomic.AddInt32(&successfulUpdates, 1)
				return nil
			})

			if result.FinalError != nil {
				t.Errorf("worker %d failed: %v", workerID, result.FinalError)
			}
		}(i)
	}

	wg.Wait()

	// All workers should eventually succeed
	if int(successfulUpdates) != numWorkers {
		t.Errorf("successfulUpdates = %d, want %d", successfulUpdates, numWorkers)
	}

	// Final version should reflect all updates
	expectedFinalVersion := int64(1 + numWorkers)
	if currentVersion != expectedFinalVersion {
		t.Errorf("final version = %d, want %d", currentVersion, expectedFinalVersion)
	}
}

// TestRetryExhaustion tests behavior when retries are exhausted.
func TestRetryExhaustion(t *testing.T) {
	cfg := optimistic.RetryConfig{
		MaxRetries:   2,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0,
	}

	attemptCount := 0
	result := optimistic.WithRetry(context.Background(), cfg, func(ctx context.Context) error {
		attemptCount++
		// Always fail with version mismatch
		return &optimistic.VersionMismatchError{
			EntityType: "node",
			EntityID:   "always-fail",
			SpaceID:    "test",
			Expected:   1,
			Actual:     2,
			Operation:  "test",
		}
	})

	// Should have tried 3 times (initial + 2 retries)
	if attemptCount != 3 {
		t.Errorf("attemptCount = %d, want 3", attemptCount)
	}

	// Should have exhausted retries
	if !errors.Is(result.FinalError, optimistic.ErrRetriesExhausted) {
		t.Errorf("FinalError = %v, want ErrRetriesExhausted", result.FinalError)
	}

	if result.VersionConflicts != 3 {
		t.Errorf("VersionConflicts = %d, want 3", result.VersionConflicts)
	}
}

// TestNonRetryableError tests that non-version errors fail immediately.
func TestNonRetryableError(t *testing.T) {
	cfg := optimistic.DefaultConfig()
	cfg.BaseDelay = 1 * time.Millisecond

	customErr := errors.New("database connection failed")
	attemptCount := 0

	result := optimistic.WithRetry(context.Background(), cfg, func(ctx context.Context) error {
		attemptCount++
		return customErr
	})

	// Should only try once
	if attemptCount != 1 {
		t.Errorf("attemptCount = %d, want 1", attemptCount)
	}

	// Should return the original error
	if !errors.Is(result.FinalError, customErr) {
		t.Errorf("FinalError = %v, want %v", result.FinalError, customErr)
	}

	if result.VersionConflicts != 0 {
		t.Errorf("VersionConflicts = %d, want 0", result.VersionConflicts)
	}
}

// TestContextCancellation tests that context cancellation stops retries.
func TestContextCancellation(t *testing.T) {
	cfg := optimistic.RetryConfig{
		MaxRetries:   10,
		BaseDelay:    100 * time.Millisecond, // Long delay to allow cancellation
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	attemptCount := 0

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result := optimistic.WithRetry(ctx, cfg, func(ctx context.Context) error {
		attemptCount++
		return optimistic.ErrVersionMismatch
	})

	// Should have been canceled before exhausting retries
	if attemptCount >= 10 {
		t.Errorf("attemptCount = %d, should be less than max retries due to cancellation", attemptCount)
	}

	if !errors.Is(result.FinalError, optimistic.ErrContextCanceled) {
		t.Errorf("FinalError = %v, want ErrContextCanceled", result.FinalError)
	}
}
