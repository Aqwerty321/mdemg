package auth

import "context"

// AuthMode specifies the authentication method.
type AuthMode string

const (
	// ModeNone disables authentication.
	ModeNone AuthMode = "none"
	// ModeAPIKey requires an API key in X-API-Key header or Authorization: ApiKey <key>.
	ModeAPIKey AuthMode = "apikey"
	// ModeBearer requires a Bearer token in Authorization header.
	ModeBearer AuthMode = "bearer"
)

// Config holds authentication configuration.
type Config struct {
	// Enabled controls whether authentication is required
	Enabled bool

	// Mode specifies the authentication method
	Mode AuthMode

	// APIKeys is the list of valid API keys (for ModeAPIKey)
	APIKeys []string

	// JWTSecret is the secret for validating JWT tokens (for ModeBearer)
	JWTSecret string

	// JWTIssuer is the expected issuer for JWT tokens
	JWTIssuer string

	// SkipEndpoints are paths that bypass authentication
	SkipEndpoints map[string]bool
}

// DefaultConfig returns sensible defaults for authentication.
func DefaultConfig() Config {
	return Config{
		Enabled: false, // Disabled by default for development
		Mode:    ModeNone,
		SkipEndpoints: map[string]bool{
			"/healthz": true,
			"/readyz":  true,
		},
	}
}

// Principal represents an authenticated identity.
type Principal struct {
	// ID is the unique identifier (API key hash, user ID, etc.)
	ID string

	// Type indicates the authentication method used
	Type AuthMode

	// Metadata contains additional claims or info
	Metadata map[string]any
}

// contextKey is a private type for context keys.
type contextKey int

const (
	principalKey contextKey = iota
)

// WithPrincipal adds the principal to the context.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// GetPrincipal retrieves the principal from context, or nil if not authenticated.
func GetPrincipal(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

// IsAuthenticated returns true if the context has a principal.
func IsAuthenticated(ctx context.Context) bool {
	return GetPrincipal(ctx) != nil
}
