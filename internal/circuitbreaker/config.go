package circuitbreaker

import "time"

// Config holds circuit breaker configuration.
type Config struct {
	// Enabled controls whether circuit breaking is active
	Enabled bool

	// FailureThreshold is the number of failures before opening the circuit
	FailureThreshold int

	// SuccessThreshold is the number of successes in half-open state before closing
	SuccessThreshold int

	// Timeout is how long to wait before transitioning from open to half-open
	Timeout time.Duration

	// MaxConcurrent limits concurrent requests in half-open state (0 = no limit)
	MaxConcurrent int
}

// DefaultConfig returns sensible defaults for circuit breaking.
func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		MaxConcurrent:    1,
	}
}
