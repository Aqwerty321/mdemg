package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIKeyValidator_Validate(t *testing.T) {
	keys := []string{"key1", "key2", "super-secret-key"}
	v := NewAPIKeyValidator(keys)

	tests := []struct {
		key   string
		valid bool
	}{
		{"key1", true},
		{"key2", true},
		{"super-secret-key", true},
		{"wrong-key", false},
		{"", false},
		{"KEY1", false}, // Case-sensitive
	}

	for _, tt := range tests {
		_, valid := v.Validate(tt.key)
		if valid != tt.valid {
			t.Errorf("Validate(%q) = %v, want %v", tt.key, valid, tt.valid)
		}
	}
}

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		query   map[string]string
		want    string
	}{
		{
			name:    "X-API-Key header",
			headers: map[string]string{"X-API-Key": "mykey"},
			query:   nil,
			want:    "mykey",
		},
		{
			name:    "Authorization ApiKey",
			headers: map[string]string{"Authorization": "ApiKey mykey"},
			query:   nil,
			want:    "mykey",
		},
		{
			name:    "Authorization Bearer",
			headers: map[string]string{"Authorization": "Bearer mytoken"},
			query:   nil,
			want:    "mytoken",
		},
		{
			name:    "Query parameter",
			headers: nil,
			query:   map[string]string{"api_key": "querykey"},
			want:    "querykey",
		},
		{
			name:    "Header takes precedence over query",
			headers: map[string]string{"X-API-Key": "headerkey"},
			query:   map[string]string{"api_key": "querykey"},
			want:    "headerkey",
		},
		{
			name:    "Empty",
			headers: nil,
			query:   nil,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.headers == nil {
				tt.headers = map[string]string{}
			}
			if tt.query == nil {
				tt.query = map[string]string{}
			}
			got := ExtractAPIKey(tt.headers, tt.query)
			if got != tt.want {
				t.Errorf("ExtractAPIKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJWTValidator_Validate(t *testing.T) {
	secret := "test-secret"
	issuer := "test-issuer"
	v := NewJWTValidator(secret, issuer)

	// Create a valid token
	validToken := createTestToken(secret, JWTClaims{
		Subject:   "user123",
		Issuer:    issuer,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
	})

	t.Run("valid token", func(t *testing.T) {
		claims, err := v.Validate(validToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if claims.Subject != "user123" {
			t.Errorf("subject = %q, want %q", claims.Subject, "user123")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		token := createTestToken(secret, JWTClaims{
			Subject:   "user123",
			Issuer:    issuer,
			ExpiresAt: time.Now().Add(-time.Hour).Unix(),
		})
		_, err := v.Validate(token)
		if err != ErrTokenExpired {
			t.Errorf("expected ErrTokenExpired, got %v", err)
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		token := createTestToken(secret, JWTClaims{
			Subject:   "user123",
			Issuer:    "wrong-issuer",
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		})
		_, err := v.Validate(token)
		if err != ErrInvalidIssuer {
			t.Errorf("expected ErrInvalidIssuer, got %v", err)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		token := createTestToken("wrong-secret", JWTClaims{
			Subject:   "user123",
			Issuer:    issuer,
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		})
		_, err := v.Validate(token)
		if err != ErrInvalidSignature {
			t.Errorf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := v.Validate("not.a.valid.token")
		if err != ErrInvalidToken {
			t.Errorf("expected ErrInvalidToken, got %v", err)
		}
	})
}

// createTestToken creates a test JWT token.
func createTestToken(secret string, claims JWTClaims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signatureInput := header + "." + payload
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signatureInput))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return header + "." + payload + "." + signature
}

func TestMiddleware_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	middleware := Middleware(cfg)

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_APIKey(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Mode:    ModeAPIKey,
		APIKeys: []string{"valid-key"},
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := GetPrincipal(r.Context())
		if p == nil {
			t.Error("principal should be set")
		}
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("valid key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-API-Key", "valid-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-API-Key", "invalid-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}

func TestMiddleware_SkipEndpoints(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Mode:    ModeAPIKey,
		APIKeys: []string{"valid-key"},
		SkipEndpoints: map[string]bool{
			"/healthz": true,
		},
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Health check should pass without auth
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("healthz should bypass auth, got %d", rec.Code)
	}
}

func TestPrincipal_Context(t *testing.T) {
	ctx := context.Background()

	// Initially no principal
	if IsAuthenticated(ctx) {
		t.Error("should not be authenticated initially")
	}
	if GetPrincipal(ctx) != nil {
		t.Error("principal should be nil initially")
	}

	// Add principal
	p := &Principal{ID: "test", Type: ModeAPIKey}
	ctx = WithPrincipal(ctx, p)

	// Should now be authenticated
	if !IsAuthenticated(ctx) {
		t.Error("should be authenticated after WithPrincipal")
	}
	if got := GetPrincipal(ctx); got != p {
		t.Errorf("GetPrincipal() = %v, want %v", got, p)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("default should be disabled")
	}
	if cfg.Mode != ModeNone {
		t.Errorf("expected ModeNone, got %s", cfg.Mode)
	}
	if !cfg.SkipEndpoints["/healthz"] {
		t.Error("default should skip /healthz")
	}
	if !cfg.SkipEndpoints["/readyz"] {
		t.Error("default should skip /readyz")
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "valid bearer",
			header: "Bearer mytoken",
			want:   "mytoken",
		},
		{
			name:   "lowercase bearer",
			header: "bearer mytoken",
			want:   "mytoken",
		},
		{
			name:   "Bearer with spaces",
			header: "  Bearer   mytoken  ",
			want:   "mytoken",
		},
		{
			name:   "not bearer",
			header: "ApiKey mykey",
			want:   "",
		},
		{
			name:   "empty",
			header: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractBearerToken(tt.header)
			if got != tt.want {
				t.Errorf("ExtractBearerToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestMiddleware_Bearer(t *testing.T) {
	secret := "test-secret"
	issuer := "test-issuer"

	cfg := Config{
		Enabled:   true,
		Mode:      ModeBearer,
		JWTSecret: secret,
		JWTIssuer: issuer,
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := GetPrincipal(r.Context())
		if p == nil {
			t.Error("principal should be set")
		}
		if p.Type != ModeBearer {
			t.Errorf("expected ModeBearer, got %s", p.Type)
		}
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("valid token", func(t *testing.T) {
		token := createTestToken(secret, JWTClaims{
			Subject:   "user123",
			Issuer:    issuer,
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
			IssuedAt:  time.Now().Unix(),
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid.token.here")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}

func TestAPIKeyValidator_EmptyKeys(t *testing.T) {
	v := NewAPIKeyValidator(nil)

	// Should not validate any key
	_, valid := v.Validate("anykey")
	if valid {
		t.Error("should not validate with empty key list")
	}
}

func TestExtractAPIKey_AllSources(t *testing.T) {
	// Test case-insensitive header check
	tests := []struct {
		name    string
		headers map[string]string
		query   map[string]string
		want    string
	}{
		{
			name:    "Authorization Basic (unsupported)",
			headers: map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
			query:   nil,
			want:    "",
		},
		{
			name:    "Authorization with leading spaces",
			headers: map[string]string{"Authorization": "  ApiKey   mykey"},
			query:   nil,
			want:    "mykey",
		},
		{
			name:    "lowercase x-api-key header",
			headers: map[string]string{"x-api-key": "lowercasekey"},
			query:   nil,
			want:    "lowercasekey",
		},
		{
			name:    "lowercase authorization header",
			headers: map[string]string{"authorization": "ApiKey lowerauth"},
			query:   nil,
			want:    "lowerauth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.headers == nil {
				tt.headers = map[string]string{}
			}
			if tt.query == nil {
				tt.query = map[string]string{}
			}
			got := ExtractAPIKey(tt.headers, tt.query)
			if got != tt.want {
				t.Errorf("ExtractAPIKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJWTValidator_Validate_InvalidSignatureBase64(t *testing.T) {
	v := NewJWTValidator("secret", "issuer")

	// Create a token with an invalid base64 signature
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user123","iss":"issuer","exp":9999999999}`))
	// Invalid base64: contains characters not in URL-safe base64
	invalidSig := "!!!invalid-base64-signature!!!"

	token := header + "." + claims + "." + invalidSig

	_, err := v.Validate(token)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for invalid base64 signature, got %v", err)
	}
}

func TestJWTValidator_Validate_InvalidClaimsBase64(t *testing.T) {
	secret := "test-secret"
	issuer := "test-issuer"
	v := NewJWTValidator(secret, issuer)

	// Create a token with invalid base64 claims
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	invalidClaims := "!!!invalid-base64-claims!!!" // Invalid base64

	// Create a valid signature for this malformed token
	signatureInput := header + "." + invalidClaims
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signatureInput))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	token := header + "." + invalidClaims + "." + signature

	_, err := v.Validate(token)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for invalid claims base64, got %v", err)
	}
}

func TestJWTValidator_Validate_InvalidClaimsJSON(t *testing.T) {
	secret := "test-secret"
	issuer := "test-issuer"
	v := NewJWTValidator(secret, issuer)

	// Create a token with invalid JSON in claims
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	invalidJSON := base64.RawURLEncoding.EncodeToString([]byte(`{invalid json`))

	// Create a valid signature for this malformed token
	signatureInput := header + "." + invalidJSON
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signatureInput))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	token := header + "." + invalidJSON + "." + signature

	_, err := v.Validate(token)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for invalid claims JSON, got %v", err)
	}
}

func TestJWTValidator_Validate_TokenNotYetValid(t *testing.T) {
	secret := "test-secret"
	issuer := "test-issuer"
	v := NewJWTValidator(secret, issuer)

	// Create a token with NotBefore in the future
	token := createTestToken(secret, JWTClaims{
		Subject:   "user123",
		Issuer:    issuer,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		NotBefore: time.Now().Add(time.Hour).Unix(), // Not valid until 1 hour from now
	})

	_, err := v.Validate(token)
	if err != ErrTokenNotYetValid {
		t.Errorf("expected ErrTokenNotYetValid, got %v", err)
	}
}

func TestMiddleware_ModeNone(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Mode:    ModeNone, // None mode passes through
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// ModeNone passes through without auth (for testing/dev)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for ModeNone, got %d", rec.Code)
	}
}
