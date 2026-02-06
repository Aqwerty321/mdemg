package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// JWTValidator validates JWT tokens.
// This is a minimal implementation for HMAC-SHA256 tokens.
type JWTValidator struct {
	secret []byte
	issuer string
}

// NewJWTValidator creates a validator with the given secret and expected issuer.
func NewJWTValidator(secret, issuer string) *JWTValidator {
	return &JWTValidator{
		secret: []byte(secret),
		issuer: issuer,
	}
}

// JWTClaims represents standard JWT claims.
type JWTClaims struct {
	Subject   string `json:"sub"`
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	NotBefore int64  `json:"nbf"`
	JWTID     string `json:"jti"`

	// Custom claims
	Scopes []string `json:"scopes,omitempty"`
}

var (
	// ErrInvalidToken indicates the token format is invalid.
	ErrInvalidToken = errors.New("invalid token format")
	// ErrInvalidSignature indicates the signature verification failed.
	ErrInvalidSignature = errors.New("invalid token signature")
	// ErrTokenExpired indicates the token has expired.
	ErrTokenExpired = errors.New("token has expired")
	// ErrTokenNotYetValid indicates the token is not yet valid (nbf claim).
	ErrTokenNotYetValid = errors.New("token not yet valid")
	// ErrInvalidIssuer indicates the issuer doesn't match.
	ErrInvalidIssuer = errors.New("invalid token issuer")
)

// Validate validates a JWT token and returns its claims.
func (v *JWTValidator) Validate(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Verify signature
	signatureInput := parts[0] + "." + parts[1]
	expectedSig := v.sign([]byte(signatureInput))
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrInvalidSignature
	}

	// Decode claims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	// Validate claims
	now := time.Now().Unix()

	if claims.ExpiresAt > 0 && now > claims.ExpiresAt {
		return nil, ErrTokenExpired
	}

	if claims.NotBefore > 0 && now < claims.NotBefore {
		return nil, ErrTokenNotYetValid
	}

	if v.issuer != "" && claims.Issuer != v.issuer {
		return nil, ErrInvalidIssuer
	}

	return &claims, nil
}

// sign creates an HMAC-SHA256 signature.
func (v *JWTValidator) sign(data []byte) []byte {
	h := hmac.New(sha256.New, v.secret)
	h.Write(data)
	return h.Sum(nil)
}

// ExtractBearerToken extracts a Bearer token from the Authorization header.
func ExtractBearerToken(authHeader string) string {
	authHeader = strings.TrimSpace(authHeader)
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}
