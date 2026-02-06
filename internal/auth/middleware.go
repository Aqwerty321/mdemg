package auth

import (
	"encoding/json"
	"log"
	"net/http"
)

// Middleware returns HTTP middleware that enforces authentication.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	var apiKeyValidator *APIKeyValidator
	var jwtValidator *JWTValidator

	switch cfg.Mode {
	case ModeAPIKey:
		apiKeyValidator = NewAPIKeyValidator(cfg.APIKeys)
	case ModeBearer:
		jwtValidator = NewJWTValidator(cfg.JWTSecret, cfg.JWTIssuer)
	case ModeNone:
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication for configured endpoints
			if cfg.SkipEndpoints != nil && cfg.SkipEndpoints[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			var principal *Principal
			var err error

			switch cfg.Mode {
			case ModeAPIKey:
				principal, err = authenticateAPIKey(r, apiKeyValidator)
			case ModeBearer:
				principal, err = authenticateBearer(r, jwtValidator)
			}

			if err != nil {
				log.Printf("auth failed: %v (path=%s, remote=%s)", err, r.URL.Path, r.RemoteAddr)
				writeAuthError(w, err)
				return
			}

			if principal == nil {
				log.Printf("auth missing (path=%s, remote=%s)", r.URL.Path, r.RemoteAddr)
				writeAuthError(w, errMissingCredentials)
				return
			}

			// Add principal to context and continue
			ctx := WithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

var errMissingCredentials = &authError{
	status:  http.StatusUnauthorized,
	code:    "missing_credentials",
	message: "authentication required",
}

type authError struct {
	status  int
	code    string
	message string
}

func (e *authError) Error() string {
	return e.message
}

// authenticateAPIKey extracts and validates an API key.
func authenticateAPIKey(r *http.Request, validator *APIKeyValidator) (*Principal, error) {
	headers := make(map[string]string)
	headers["X-API-Key"] = r.Header.Get("X-API-Key")
	headers["Authorization"] = r.Header.Get("Authorization")

	query := make(map[string]string)
	query["api_key"] = r.URL.Query().Get("api_key")

	key := ExtractAPIKey(headers, query)
	if key == "" {
		return nil, nil
	}

	id, valid := validator.Validate(key)
	if !valid {
		return nil, &authError{
			status:  http.StatusUnauthorized,
			code:    "invalid_api_key",
			message: "invalid API key",
		}
	}

	return &Principal{
		ID:   id,
		Type: ModeAPIKey,
	}, nil
}

// authenticateBearer extracts and validates a Bearer token.
func authenticateBearer(r *http.Request, validator *JWTValidator) (*Principal, error) {
	token := ExtractBearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return nil, nil
	}

	claims, err := validator.Validate(token)
	if err != nil {
		return nil, &authError{
			status:  http.StatusUnauthorized,
			code:    "invalid_token",
			message: err.Error(),
		}
	}

	return &Principal{
		ID:   claims.Subject,
		Type: ModeBearer,
		Metadata: map[string]any{
			"issuer":  claims.Issuer,
			"scopes":  claims.Scopes,
			"expires": claims.ExpiresAt,
		},
	}, nil
}

// writeAuthError writes an authentication error response.
func writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	code := "unauthorized"
	message := "authentication failed"

	if ae, ok := err.(*authError); ok {
		status = ae.status
		code = ae.code
		message = ae.message
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `ApiKey realm="mdemg"`)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error":   code,
		"message": message,
	})
}
