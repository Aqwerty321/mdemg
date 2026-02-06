package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Test certificate and key for SAML tests
var (
	testSAMLCert    string
	testSAMLPrivKey *rsa.PrivateKey
)

func init() {
	// Generate a test certificate for SAML signature validation
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	testSAMLPrivKey = privKey

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test IdP"},
			CommonName:   "test-idp.example.com",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	testSAMLCert = string(certPEM)
}

// createTestSAMLAssertion creates a test SAML assertion XML.
func createTestSAMLAssertion(subject, issuer, audience string, notBefore, notOnOrAfter time.Time) string {
	return fmt.Sprintf(`<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="_test123" Version="2.0" IssueInstant="%s">
	<saml:Issuer>%s</saml:Issuer>
	<saml:Subject>
		<saml:NameID Format="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress">%s</saml:NameID>
		<saml:SubjectConfirmation Method="urn:oasis:names:tc:SAML:2.0:cm:bearer">
			<saml:SubjectConfirmationData NotOnOrAfter="%s" Recipient="https://app.example.com/saml/acs"/>
		</saml:SubjectConfirmation>
	</saml:Subject>
	<saml:Conditions NotBefore="%s" NotOnOrAfter="%s">
		<saml:AudienceRestriction>
			<saml:Audience>%s</saml:Audience>
		</saml:AudienceRestriction>
	</saml:Conditions>
	<saml:AuthnStatement AuthnInstant="%s" SessionIndex="_session123">
		<saml:AuthnContext>
			<saml:AuthnContextClassRef>urn:oasis:names:tc:SAML:2.0:ac:classes:Password</saml:AuthnContextClassRef>
		</saml:AuthnContext>
	</saml:AuthnStatement>
	<saml:AttributeStatement>
		<saml:Attribute Name="email" NameFormat="urn:oasis:names:tc:SAML:2.0:attrname-format:basic">
			<saml:AttributeValue>%s</saml:AttributeValue>
		</saml:Attribute>
		<saml:Attribute Name="groups" NameFormat="urn:oasis:names:tc:SAML:2.0:attrname-format:basic">
			<saml:AttributeValue>admin</saml:AttributeValue>
			<saml:AttributeValue>users</saml:AttributeValue>
		</saml:Attribute>
	</saml:AttributeStatement>
</saml:Assertion>`,
		time.Now().Format(time.RFC3339),
		issuer,
		subject,
		notOnOrAfter.Format(time.RFC3339),
		notBefore.Format(time.RFC3339),
		notOnOrAfter.Format(time.RFC3339),
		audience,
		time.Now().Format(time.RFC3339),
		subject,
	)
}

func TestSAMLValidator_Validate(t *testing.T) {
	entityID := "https://app.example.com"
	issuer := "https://login.microsoftonline.com/tenant-id/saml2"

	v, err := NewSAMLValidator(entityID, issuer, testSAMLCert, 300)
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	t.Run("valid assertion", func(t *testing.T) {
		xml := createTestSAMLAssertion(
			"user@example.com",
			issuer,
			entityID,
			time.Now().Add(-time.Minute),
			time.Now().Add(time.Hour),
		)
		assertionB64 := base64.StdEncoding.EncodeToString([]byte(xml))

		assertion, err := v.Validate(assertionB64)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if assertion.Subject.NameID.Value != "user@example.com" {
			t.Errorf("subject = %q, want %q", assertion.Subject.NameID.Value, "user@example.com")
		}
	})

	t.Run("expired assertion", func(t *testing.T) {
		xml := createTestSAMLAssertion(
			"user@example.com",
			issuer,
			entityID,
			time.Now().Add(-2*time.Hour),
			time.Now().Add(-time.Hour), // Expired
		)
		assertionB64 := base64.StdEncoding.EncodeToString([]byte(xml))

		_, err := v.Validate(assertionB64)
		if err != ErrSAMLAssertionExpired {
			t.Errorf("expected ErrSAMLAssertionExpired, got %v", err)
		}
	})

	t.Run("not yet valid assertion", func(t *testing.T) {
		xml := createTestSAMLAssertion(
			"user@example.com",
			issuer,
			entityID,
			time.Now().Add(time.Hour), // Future
			time.Now().Add(2*time.Hour),
		)
		assertionB64 := base64.StdEncoding.EncodeToString([]byte(xml))

		_, err := v.Validate(assertionB64)
		if err != ErrSAMLAssertionNotValid {
			t.Errorf("expected ErrSAMLAssertionNotValid, got %v", err)
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		xml := createTestSAMLAssertion(
			"user@example.com",
			"https://wrong-issuer.com/saml2",
			entityID,
			time.Now().Add(-time.Minute),
			time.Now().Add(time.Hour),
		)
		assertionB64 := base64.StdEncoding.EncodeToString([]byte(xml))

		_, err := v.Validate(assertionB64)
		if err != ErrSAMLIssuerMismatch {
			t.Errorf("expected ErrSAMLIssuerMismatch, got %v", err)
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		xml := createTestSAMLAssertion(
			"user@example.com",
			issuer,
			"https://wrong-audience.com",
			time.Now().Add(-time.Minute),
			time.Now().Add(time.Hour),
		)
		assertionB64 := base64.StdEncoding.EncodeToString([]byte(xml))

		_, err := v.Validate(assertionB64)
		if err != ErrSAMLAudienceMismatch {
			t.Errorf("expected ErrSAMLAudienceMismatch, got %v", err)
		}
	})

	t.Run("invalid base64", func(t *testing.T) {
		_, err := v.Validate("not-valid-base64!!!")
		if err != ErrInvalidSAMLAssertion {
			t.Errorf("expected ErrInvalidSAMLAssertion, got %v", err)
		}
	})

	t.Run("invalid XML", func(t *testing.T) {
		invalidXML := base64.StdEncoding.EncodeToString([]byte("<not-valid-xml"))
		_, err := v.Validate(invalidXML)
		if err != ErrInvalidSAMLAssertion {
			t.Errorf("expected ErrInvalidSAMLAssertion, got %v", err)
		}
	})
}

func TestExtractSAMLAssertion(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "SAML Authorization header",
			headers: map[string]string{"Authorization": "SAML base64assertion"},
			want:    "base64assertion",
		},
		{
			name:    "lowercase saml Authorization header",
			headers: map[string]string{"Authorization": "saml base64assertion"},
			want:    "base64assertion",
		},
		{
			name:    "X-SAML-Assertion header",
			headers: map[string]string{"X-SAML-Assertion": "base64assertion"},
			want:    "base64assertion",
		},
		{
			name:    "Authorization takes precedence",
			headers: map[string]string{"Authorization": "SAML primary", "X-SAML-Assertion": "secondary"},
			want:    "primary",
		},
		{
			name:    "Bearer token (not SAML)",
			headers: map[string]string{"Authorization": "Bearer token"},
			want:    "",
		},
		{
			name:    "empty",
			headers: map[string]string{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := ExtractSAMLAssertion(req)
			if got != tt.want {
				t.Errorf("ExtractSAMLAssertion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSAMLAuthenticator_Authenticate(t *testing.T) {
	entityID := "https://app.example.com"
	issuer := "https://login.microsoftonline.com/tenant-id/saml2"

	config := &SAMLConfig{
		EntityID:    entityID,
		Certificate: testSAMLCert,
		Issuer:      issuer,
	}

	auth, err := newSAMLAuthenticator(config)
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	t.Run("valid assertion", func(t *testing.T) {
		xml := createTestSAMLAssertion(
			"user@example.com",
			issuer,
			entityID,
			time.Now().Add(-time.Minute),
			time.Now().Add(time.Hour),
		)
		assertionB64 := base64.StdEncoding.EncodeToString([]byte(xml))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "SAML "+assertionB64)

		principal, err := auth.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if principal == nil {
			t.Fatal("expected principal, got nil")
		}
		if principal.ID != "user@example.com" {
			t.Errorf("principal.ID = %q, want %q", principal.ID, "user@example.com")
		}
		if principal.Type != ModeSAML {
			t.Errorf("principal.Type = %q, want %q", principal.Type, ModeSAML)
		}

		// Check metadata
		if principal.Metadata["issuer"] != issuer {
			t.Errorf("metadata[issuer] = %v, want %v", principal.Metadata["issuer"], issuer)
		}

		attrs, ok := principal.Metadata["attributes"].(map[string]any)
		if !ok {
			t.Fatal("expected attributes in metadata")
		}
		if attrs["email"] != "user@example.com" {
			t.Errorf("attributes[email] = %v, want %v", attrs["email"], "user@example.com")
		}
	})

	t.Run("no credentials", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)

		principal, err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if principal != nil {
			t.Error("expected nil principal for missing credentials")
		}
	})

	t.Run("invalid assertion", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "SAML invalid-base64!!!")

		_, err := auth.Authenticate(req)
		if err == nil {
			t.Error("expected error for invalid assertion")
		}
		authErr, ok := err.(*AuthError)
		if !ok {
			t.Errorf("expected *AuthError, got %T", err)
		}
		if authErr.Status != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", authErr.Status, http.StatusUnauthorized)
		}
	})
}

func TestSAMLConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *SAMLConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &SAMLConfig{
				EntityID:    "https://app.example.com",
				Certificate: testSAMLCert,
				Issuer:      "https://idp.example.com",
			},
			wantErr: false,
		},
		{
			name: "missing entity ID",
			config: &SAMLConfig{
				Certificate: testSAMLCert,
				Issuer:      "https://idp.example.com",
			},
			wantErr: true,
		},
		{
			name: "missing certificate",
			config: &SAMLConfig{
				EntityID: "https://app.example.com",
				Issuer:   "https://idp.example.com",
			},
			wantErr: true,
		},
		{
			name: "missing issuer",
			config: &SAMLConfig{
				EntityID:    "https://app.example.com",
				Certificate: testSAMLCert,
			},
			wantErr: true,
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

func TestNewSAMLValidator_InvalidCert(t *testing.T) {
	t.Run("empty certificate", func(t *testing.T) {
		_, err := NewSAMLValidator("entity", "issuer", "", 0)
		if err != ErrSAMLNoCertificate {
			t.Errorf("expected ErrSAMLNoCertificate, got %v", err)
		}
	})

	t.Run("invalid PEM", func(t *testing.T) {
		_, err := NewSAMLValidator("entity", "issuer", "not-a-pem-cert", 0)
		if err == nil {
			t.Error("expected error for invalid PEM")
		}
	})

	t.Run("invalid certificate", func(t *testing.T) {
		invalidPEM := "-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----"
		_, err := NewSAMLValidator("entity", "issuer", invalidPEM, 0)
		if err == nil {
			t.Error("expected error for invalid certificate")
		}
	})
}

func TestSAML_RegistryIntegration(t *testing.T) {
	registry := DefaultRegistry()

	// Verify SAML is registered
	if !registry.Has("saml") {
		t.Error("saml method should be registered")
	}

	// Verify it's in the list
	methods := registry.List()
	found := false
	for _, m := range methods {
		if m == "saml" {
			found = true
			break
		}
	}
	if !found {
		t.Error("saml should be in registry list")
	}

	// Verify we can build it with valid config
	config := &SAMLConfig{
		EntityID:    "https://app.example.com",
		Certificate: testSAMLCert,
		Issuer:      "https://idp.example.com",
	}
	auth, err := registry.Build("saml", config)
	if err != nil {
		t.Fatalf("failed to build SAML authenticator: %v", err)
	}
	if auth.Name() != "saml" {
		t.Errorf("Name() = %q, want %q", auth.Name(), "saml")
	}
}

func TestSAML_GetMethodConfig(t *testing.T) {
	cfg := Config{
		Mode:            ModeSAML,
		SAMLEntityID:    "https://app.example.com",
		SAMLCertificate: testSAMLCert,
		SAMLIssuer:      "https://idp.example.com",
	}

	methodCfg := cfg.GetMethodConfig()
	if methodCfg.MethodName() != "saml" {
		t.Errorf("MethodName() = %q, want %q", methodCfg.MethodName(), "saml")
	}

	samlCfg, ok := methodCfg.(*SAMLConfig)
	if !ok {
		t.Fatalf("expected *SAMLConfig, got %T", methodCfg)
	}
	if samlCfg.EntityID != cfg.SAMLEntityID {
		t.Errorf("EntityID = %q, want %q", samlCfg.EntityID, cfg.SAMLEntityID)
	}
}
