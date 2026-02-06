package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBreaker_StartssClosed(t *testing.T) {
	b := New("test", DefaultConfig())
	if b.State() != StateClosed {
		t.Errorf("expected closed, got %s", b.State())
	}
}

func TestBreaker_OpensAfterFailures(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 3
	b := New("test", cfg)

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		if b.State() != StateClosed {
			t.Errorf("should still be closed after %d failures", i)
		}
		b.RecordFailure()
	}

	// Should now be open
	if b.State() != StateOpen {
		t.Errorf("expected open after 3 failures, got %s", b.State())
	}

	// Allow should return false
	if b.Allow() {
		t.Error("should not allow requests when open")
	}
}

func TestBreaker_TransitionsToHalfOpen(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 1
	cfg.Timeout = 50 * time.Millisecond
	b := New("test", cfg)

	// Open the circuit
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatalf("expected open, got %s", b.State())
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should now be half-open
	if b.State() != StateHalfOpen {
		t.Errorf("expected half-open after timeout, got %s", b.State())
	}
}

func TestBreaker_ClosesOnSuccessInHalfOpen(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 1
	cfg.SuccessThreshold = 2
	cfg.Timeout = 50 * time.Millisecond
	b := New("test", cfg)

	// Open the circuit
	b.RecordFailure()

	// Wait for timeout to enter half-open
	time.Sleep(60 * time.Millisecond)

	// Record successes
	b.RecordSuccess()
	if b.State() != StateHalfOpen {
		t.Errorf("should still be half-open after 1 success, got %s", b.State())
	}

	b.RecordSuccess()
	if b.State() != StateClosed {
		t.Errorf("expected closed after 2 successes, got %s", b.State())
	}
}

func TestBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 1
	cfg.Timeout = 50 * time.Millisecond
	b := New("test", cfg)

	// Open the circuit
	b.RecordFailure()

	// Wait for timeout to enter half-open
	time.Sleep(60 * time.Millisecond)

	// Verify half-open
	if b.State() != StateHalfOpen {
		t.Fatalf("expected half-open, got %s", b.State())
	}

	// Any failure reopens
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Errorf("expected open after failure in half-open, got %s", b.State())
	}
}

func TestBreaker_Execute(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 2
	b := New("test", cfg)

	// Successful execution
	err := b.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	// Failed execution
	testErr := errors.New("test error")
	err = b.Execute(context.Background(), func(ctx context.Context) error {
		return testErr
	})
	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}

	// Another failure should open the circuit
	b.Execute(context.Background(), func(ctx context.Context) error {
		return testErr
	})

	// Circuit should be open now
	err = b.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestBreaker_DisabledPassesThrough(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	b := New("test", cfg)

	// Record many failures
	for i := 0; i < 100; i++ {
		b.RecordFailure()
	}

	// Should still allow (disabled)
	if !b.Allow() {
		t.Error("disabled breaker should always allow")
	}
}

func TestBreaker_Reset(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 1
	b := New("test", cfg)

	// Open the circuit
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatalf("expected open, got %s", b.State())
	}

	// Reset
	b.Reset()
	if b.State() != StateClosed {
		t.Errorf("expected closed after reset, got %s", b.State())
	}

	// Should allow again
	if !b.Allow() {
		t.Error("should allow after reset")
	}
}

func TestBreaker_Concurrent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 100
	b := New("test", cfg)

	var wg sync.WaitGroup

	// Concurrent failures
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.RecordFailure()
		}()
	}

	// Concurrent successes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.RecordSuccess()
		}()
	}

	wg.Wait()

	// Should still be closed (failures reset on success, successes don't reset failures in closed state)
	// Actually, failures only reset when we record a success, so behavior depends on ordering
	// The main thing is it shouldn't panic
}

func TestBreaker_StateCallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 1
	cfg.Timeout = 50 * time.Millisecond
	b := New("test", cfg)

	transitions := make([]struct{ from, to State }, 0)
	var mu sync.Mutex

	b.OnStateChange(func(name string, from, to State) {
		mu.Lock()
		defer mu.Unlock()
		transitions = append(transitions, struct{ from, to State }{from, to})
	})

	// Open the circuit
	b.RecordFailure()

	// Wait for callback
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].from != StateClosed || transitions[0].to != StateOpen {
		t.Errorf("expected closed->open, got %s->%s", transitions[0].from, transitions[0].to)
	}
	mu.Unlock()
}

func TestRegistry_GetCreatesBreakers(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	b1 := r.Get("service1")
	b2 := r.Get("service2")
	b3 := r.Get("service1") // Same name

	if b1 == b2 {
		t.Error("different names should return different breakers")
	}

	if b1 != b3 {
		t.Error("same name should return same breaker")
	}
}

func TestRegistry_States(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 1
	r := NewRegistry(cfg)

	// Create and open one breaker
	r.Get("service1").RecordFailure()
	r.Get("service2") // Just create, leave closed

	states := r.States()

	if states["service1"] != StateOpen {
		t.Errorf("service1 should be open, got %s", states["service1"])
	}
	if states["service2"] != StateClosed {
		t.Errorf("service2 should be closed, got %s", states["service2"])
	}
}

func TestBreaker_MaxConcurrentHalfOpen(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 1
	cfg.Timeout = 10 * time.Millisecond
	cfg.MaxConcurrent = 1
	b := New("test", cfg)

	// Open and wait for half-open
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	// First request should be allowed
	if !b.Allow() {
		t.Error("first half-open request should be allowed")
	}

	// Second should be rejected (only 1 concurrent allowed)
	if b.Allow() {
		t.Error("second half-open request should be rejected")
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
