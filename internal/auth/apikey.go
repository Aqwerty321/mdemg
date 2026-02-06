package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// APIKeyValidator validates API keys.
type APIKeyValidator struct {
	// validKeys is a set of valid API key hashes for O(1) lookup
	validKeys map[string]bool
}

// NewAPIKeyValidator creates a validator with the given valid keys.
func NewAPIKeyValidator(keys []string) *APIKeyValidator {
	validKeys := make(map[string]bool, len(keys))
	for _, key := range keys {
		// Store hash of key for comparison (prevents timing attacks on map lookup)
		h := hashKey(key)
		validKeys[h] = true
	}
	return &APIKeyValidator{validKeys: validKeys}
}

// Validate checks if the given key is valid.
// Returns the key's hash as an identifier if valid.
func (v *APIKeyValidator) Validate(key string) (string, bool) {
	if len(v.validKeys) == 0 {
		return "", false
	}

	h := hashKey(key)

	// Use constant-time comparison on the hash
	for validHash := range v.validKeys {
		if subtle.ConstantTimeCompare([]byte(h), []byte(validHash)) == 1 {
			return h, true
		}
	}

	return "", false
}

// hashKey creates a SHA-256 hash of the key.
func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// ExtractAPIKey extracts an API key from common locations:
// 1. X-API-Key header
// 2. Authorization: ApiKey <key>
// 3. Authorization: Bearer <key> (if configured)
// 4. api_key query parameter
func ExtractAPIKey(headers map[string]string, query map[string]string) string {
	// Check X-API-Key header first
	if key := headers["X-API-Key"]; key != "" {
		return strings.TrimSpace(key)
	}
	if key := headers["x-api-key"]; key != "" {
		return strings.TrimSpace(key)
	}

	// Check Authorization header
	if auth := headers["Authorization"]; auth != "" {
		return extractFromAuthorization(auth)
	}
	if auth := headers["authorization"]; auth != "" {
		return extractFromAuthorization(auth)
	}

	// Check query parameter (least preferred)
	if key := query["api_key"]; key != "" {
		return strings.TrimSpace(key)
	}

	return ""
}

// extractFromAuthorization extracts a key from an Authorization header.
func extractFromAuthorization(auth string) string {
	auth = strings.TrimSpace(auth)

	// ApiKey scheme
	if strings.HasPrefix(strings.ToLower(auth), "apikey ") {
		return strings.TrimSpace(auth[7:])
	}

	// Bearer scheme (some systems use Bearer for API keys)
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}

	return ""
}
