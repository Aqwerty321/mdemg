package ratelimit

// Config holds rate limiting configuration.
type Config struct {
	// Enabled controls whether rate limiting is active
	Enabled bool

	// RequestsPerSecond is the sustained rate limit
	RequestsPerSecond float64

	// BurstSize is the maximum burst allowance
	BurstSize int

	// ByIP enables per-IP rate limiting (vs global)
	ByIP bool

	// SkipEndpoints are paths that bypass rate limiting (e.g., health checks)
	SkipEndpoints map[string]bool

	// TrustedProxies for X-Forwarded-For parsing
	TrustedProxies []string
}

// DefaultConfig returns sensible defaults for rate limiting.
func DefaultConfig() Config {
	return Config{
		Enabled:           true,
		RequestsPerSecond: 100,
		BurstSize:         200,
		ByIP:              true,
		SkipEndpoints: map[string]bool{
			"/healthz": true,
			"/readyz":  true,
		},
		TrustedProxies: nil,
	}
}
