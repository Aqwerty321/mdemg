package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// UAMSSpec represents a UAMS specification file.
type UAMSSpec struct {
	UAMSVersion string `json:"uams_version"`
	Method      struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description"`
		RFC         string `json:"rfc,omitempty"`
		Status      string `json:"status"`
	} `json:"method"`
	Credentials struct {
		Extraction []struct {
			Source   string `json:"source"`
			Name     string `json:"name"`
			Prefix   string `json:"prefix,omitempty"`
			Priority int    `json:"priority"`
		} `json:"extraction"`
		Format struct {
			Type      string `json:"type"`
			Pattern   string `json:"pattern,omitempty"`
			MinLength int    `json:"min_length,omitempty"`
			MaxLength int    `json:"max_length,omitempty"`
		} `json:"format"`
	} `json:"credentials"`
	Validation struct {
		Algorithm      string   `json:"algorithm"`
		TimingSafe     bool     `json:"timing_safe"`
		Checks         []string `json:"checks"`
		ConfigRequired []string `json:"config_required"`
	} `json:"validation"`
	Principal struct {
		IDSource       string   `json:"id_source"`
		MetadataFields []string `json:"metadata_fields"`
	} `json:"principal"`
	Errors []struct {
		Code            string `json:"code"`
		Status          int    `json:"status"`
		Message         string `json:"message"`
		WWWAuthenticate string `json:"www_authenticate,omitempty"`
	} `json:"errors"`
	TestFixtures struct {
		Valid   []string `json:"valid"`
		Invalid []string `json:"invalid"`
		Expired []string `json:"expired,omitempty"`
	} `json:"test_fixtures"`
	Metadata struct {
		Author  string `json:"author"`
		Created string `json:"created"`
		Updated string `json:"updated"`
	} `json:"metadata"`
}

// findUAMSSpecDir locates the UAMS specs directory.
func findUAMSSpecDir() string {
	// Try relative paths from common locations
	candidates := []string{
		"../../docs/tests/uams/specs",
		"../../../docs/tests/uams/specs",
		"docs/tests/uams/specs",
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	// Try finding from module root
	wd, _ := os.Getwd()
	for wd != "/" {
		specDir := filepath.Join(wd, "docs/tests/uams/specs")
		if _, err := os.Stat(specDir); err == nil {
			return specDir
		}
		wd = filepath.Dir(wd)
	}

	return "docs/tests/uams/specs"
}

// loadUAMSSpecs loads all UAMS spec files from the given directory.
func loadUAMSSpecs(dir string) ([]UAMSSpec, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.uams.json"))
	if err != nil {
		return nil, err
	}

	var specs []UAMSSpec
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}

		var spec UAMSSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

func TestUAMS_SpecsExist(t *testing.T) {
	specDir := findUAMSSpecDir()
	specs, err := loadUAMSSpecs(specDir)
	if err != nil {
		t.Skipf("UAMS specs not found at %s: %v", specDir, err)
	}

	if len(specs) == 0 {
		t.Fatal("no UAMS specs found")
	}

	// Verify we have specs for all built-in methods
	methodNames := make(map[string]bool)
	for _, spec := range specs {
		methodNames[spec.Method.Name] = true
	}

	required := []string{"none", "apikey", "jwt"}
	for _, name := range required {
		if !methodNames[name] {
			t.Errorf("missing UAMS spec for required method: %s", name)
		}
	}
}

func TestUAMS_RegistryHasAllMethods(t *testing.T) {
	specDir := findUAMSSpecDir()
	specs, err := loadUAMSSpecs(specDir)
	if err != nil {
		t.Skipf("UAMS specs not found: %v", err)
	}

	registry := DefaultRegistry()

	for _, spec := range specs {
		t.Run(spec.Method.Name, func(t *testing.T) {
			if !registry.Has(spec.Method.Name) {
				t.Errorf("method %q defined in UAMS spec but not registered", spec.Method.Name)
			}
		})
	}
}

func TestUAMS_BuildAuthenticators(t *testing.T) {
	registry := DefaultRegistry()

	tests := []struct {
		name   string
		method string
		config AuthMethodConfig
	}{
		{
			name:   "none",
			method: "none",
			config: &NoneConfig{},
		},
		{
			name:   "apikey",
			method: "apikey",
			config: &APIKeyConfig{Keys: []string{"test-key"}},
		},
		{
			name:   "jwt",
			method: "jwt",
			config: &JWTConfig{Secret: "test-secret", Issuer: "test-issuer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := registry.Build(tt.method, tt.config)
			if err != nil {
				t.Fatalf("failed to build authenticator: %v", err)
			}
			if auth.Name() != tt.method {
				t.Errorf("Name() = %q, want %q", auth.Name(), tt.method)
			}
		})
	}
}

func TestUAMS_APIKeyExtraction(t *testing.T) {
	specDir := findUAMSSpecDir()
	specs, err := loadUAMSSpecs(specDir)
	if err != nil {
		t.Skipf("UAMS specs not found: %v", err)
	}

	var apikeySpec UAMSSpec
	for _, s := range specs {
		if s.Method.Name == "apikey" {
			apikeySpec = s
			break
		}
	}

	if apikeySpec.Method.Name == "" {
		t.Skip("apikey spec not found")
	}

	auth, err := DefaultRegistry().Build("apikey", &APIKeyConfig{Keys: []string{"valid-key"}})
	if err != nil {
		t.Fatalf("failed to build apikey authenticator: %v", err)
	}

	// Test extraction from each source defined in spec
	for _, ext := range apikeySpec.Credentials.Extraction {
		t.Run(ext.Source+"_"+ext.Name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)

			switch ext.Source {
			case "header":
				if ext.Prefix != "" {
					req.Header.Set(ext.Name, ext.Prefix+"valid-key")
				} else {
					req.Header.Set(ext.Name, "valid-key")
				}
			case "query":
				q := req.URL.Query()
				q.Set(ext.Name, "valid-key")
				req.URL.RawQuery = q.Encode()
			}

			principal, err := auth.Authenticate(req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if principal == nil {
				t.Error("expected principal, got nil")
			}
		})
	}
}

func TestUAMS_JWTExtraction(t *testing.T) {
	specDir := findUAMSSpecDir()
	specs, err := loadUAMSSpecs(specDir)
	if err != nil {
		t.Skipf("UAMS specs not found: %v", err)
	}

	var jwtSpec UAMSSpec
	for _, s := range specs {
		if s.Method.Name == "jwt" {
			jwtSpec = s
			break
		}
	}

	if jwtSpec.Method.Name == "" {
		t.Skip("jwt spec not found")
	}

	secret := "test-secret"
	issuer := "test-issuer"
	auth, err := DefaultRegistry().Build("jwt", &JWTConfig{Secret: secret, Issuer: issuer})
	if err != nil {
		t.Fatalf("failed to build jwt authenticator: %v", err)
	}

	// Create a valid token
	token := createTestToken(secret, JWTClaims{
		Subject:   "user123",
		Issuer:    issuer,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})

	// Test extraction from Authorization header
	for _, ext := range jwtSpec.Credentials.Extraction {
		t.Run(ext.Source+"_"+ext.Name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)

			if ext.Source == "header" && ext.Name == "Authorization" && strings.HasPrefix(ext.Prefix, "Bearer") {
				req.Header.Set(ext.Name, ext.Prefix+token)

				principal, err := auth.Authenticate(req)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if principal == nil {
					t.Error("expected principal, got nil")
				}
			}
		})
	}

	// Verify JWT format pattern from spec
	if jwtSpec.Credentials.Format.Pattern != "" {
		pattern := regexp.MustCompile(jwtSpec.Credentials.Format.Pattern)
		if !pattern.MatchString(token) {
			t.Errorf("token does not match spec pattern: %s", jwtSpec.Credentials.Format.Pattern)
		}
	}
}

func TestUAMS_JWTValidationChecks(t *testing.T) {
	specDir := findUAMSSpecDir()
	specs, err := loadUAMSSpecs(specDir)
	if err != nil {
		t.Skipf("UAMS specs not found: %v", err)
	}

	var jwtSpec UAMSSpec
	for _, s := range specs {
		if s.Method.Name == "jwt" {
			jwtSpec = s
			break
		}
	}

	if jwtSpec.Method.Name == "" {
		t.Skip("jwt spec not found")
	}

	secret := "test-secret"
	issuer := "test-issuer"
	auth, err := DefaultRegistry().Build("jwt", &JWTConfig{Secret: secret, Issuer: issuer})
	if err != nil {
		t.Fatalf("failed to build jwt authenticator: %v", err)
	}

	// Test validation checks from spec
	for _, check := range jwtSpec.Validation.Checks {
		t.Run("check_"+check, func(t *testing.T) {
			var token string
			var expectError bool

			switch check {
			case "signature":
				// Invalid signature
				token = createTestToken("wrong-secret", JWTClaims{
					Subject:   "user123",
					Issuer:    issuer,
					ExpiresAt: time.Now().Add(time.Hour).Unix(),
				})
				expectError = true

			case "expiry":
				// Expired token
				token = createTestToken(secret, JWTClaims{
					Subject:   "user123",
					Issuer:    issuer,
					ExpiresAt: time.Now().Add(-time.Hour).Unix(),
				})
				expectError = true

			case "issuer":
				// Wrong issuer
				token = createTestToken(secret, JWTClaims{
					Subject:   "user123",
					Issuer:    "wrong-issuer",
					ExpiresAt: time.Now().Add(time.Hour).Unix(),
				})
				expectError = true

			default:
				t.Skipf("check %q not implemented in test", check)
				return
			}

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)

			_, err := auth.Authenticate(req)
			if expectError && err == nil {
				t.Errorf("expected error for %s check, got nil", check)
			}
			if !expectError && err != nil {
				t.Errorf("unexpected error for %s check: %v", check, err)
			}
		})
	}
}

func TestUAMS_ErrorCodes(t *testing.T) {
	specDir := findUAMSSpecDir()
	specs, err := loadUAMSSpecs(specDir)
	if err != nil {
		t.Skipf("UAMS specs not found: %v", err)
	}

	for _, spec := range specs {
		if spec.Method.Type == "none" {
			continue // No errors for none type
		}

		t.Run(spec.Method.Name+"_errors", func(t *testing.T) {
			// Verify error codes are defined
			if len(spec.Errors) == 0 {
				t.Error("expected error definitions in spec")
			}

			for _, errDef := range spec.Errors {
				if errDef.Status < 400 || errDef.Status >= 600 {
					t.Errorf("error %q has invalid status %d", errDef.Code, errDef.Status)
				}
				if errDef.Code == "" {
					t.Error("error code cannot be empty")
				}
				if errDef.Message == "" {
					t.Error("error message cannot be empty")
				}
			}
		})
	}
}

func TestUAMS_NonePassthrough(t *testing.T) {
	auth, err := DefaultRegistry().Build("none", &NoneConfig{})
	if err != nil {
		t.Fatalf("failed to build none authenticator: %v", err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	principal, err := auth.Authenticate(req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if principal == nil {
		t.Error("expected anonymous principal, got nil")
	}
	if principal.ID != "anonymous" {
		t.Errorf("expected anonymous ID, got %q", principal.ID)
	}
}

func TestUAMS_MiddlewareWithRegistry(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Mode:    ModeAPIKey,
		APIKeys: []string{"valid-key"},
	}

	middleware := MiddlewareWithRegistry(cfg, DefaultRegistry())

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
}

func TestUAMS_RegistryList(t *testing.T) {
	registry := DefaultRegistry()
	methods := registry.List()

	expected := []string{"apikey", "jwt", "none", "saml"}
	if len(methods) != len(expected) {
		t.Errorf("expected %d methods, got %d", len(expected), len(methods))
	}

	for _, e := range expected {
		found := false
		for _, m := range methods {
			if m == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected method %q not found in registry", e)
		}
	}
}

func TestUAMS_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  AuthMethodConfig
		wantErr bool
	}{
		{
			name:    "valid apikey config",
			config:  &APIKeyConfig{Keys: []string{"key1"}},
			wantErr: false,
		},
		{
			name:    "empty apikey config",
			config:  &APIKeyConfig{Keys: []string{}},
			wantErr: true,
		},
		{
			name:    "nil apikey config",
			config:  &APIKeyConfig{Keys: nil},
			wantErr: true,
		},
		{
			name:    "valid jwt config",
			config:  &JWTConfig{Secret: "secret", Issuer: "issuer"},
			wantErr: false,
		},
		{
			name:    "empty jwt secret",
			config:  &JWTConfig{Secret: "", Issuer: "issuer"},
			wantErr: true,
		},
		{
			name:    "none config",
			config:  &NoneConfig{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUAMS_MethodConfigFromConfig(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		wantMethod string
	}{
		{
			name:       "apikey mode",
			cfg:        Config{Mode: ModeAPIKey, APIKeys: []string{"key1"}},
			wantMethod: "apikey",
		},
		{
			name:       "bearer mode",
			cfg:        Config{Mode: ModeBearer, JWTSecret: "secret", JWTIssuer: "issuer"},
			wantMethod: "jwt",
		},
		{
			name:       "none mode",
			cfg:        Config{Mode: ModeNone},
			wantMethod: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			methodCfg := tt.cfg.GetMethodConfig()
			if methodCfg.MethodName() != tt.wantMethod {
				t.Errorf("MethodName() = %q, want %q", methodCfg.MethodName(), tt.wantMethod)
			}
		})
	}
}
