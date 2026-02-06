package auth

import "net/http"

// Authenticator validates credentials and returns a Principal.
// This interface enables pluggable authentication methods via the Registry.
type Authenticator interface {
	// Name returns the authenticator's identifier (e.g., "apikey", "jwt").
	Name() string

	// Authenticate validates credentials from the request.
	// Returns:
	//   - (*Principal, nil): successful authentication
	//   - (nil, nil): no credentials provided (may be optional)
	//   - (nil, error): validation failure
	Authenticate(r *http.Request) (*Principal, error)
}

// AuthError represents an authentication error with HTTP status.
type AuthError struct {
	Status  int
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// NewAuthError creates a new authentication error.
func NewAuthError(status int, code, message string) *AuthError {
	return &AuthError{
		Status:  status,
		Code:    code,
		Message: message,
	}
}

// Common authentication errors.
var (
	ErrMissingCredentials = NewAuthError(401, "missing_credentials", "authentication required")
	ErrInvalidCredentials = NewAuthError(401, "invalid_credentials", "invalid credentials")
)
