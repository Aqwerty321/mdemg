package auth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"net/http"
	"strings"
	"time"
)

// SAML assertion XML structures for Microsoft Entra ID.

// SAMLAssertion represents a SAML 2.0 Assertion.
type SAMLAssertion struct {
	XMLName            xml.Name            `xml:"Assertion"`
	ID                 string              `xml:"ID,attr"`
	Version            string              `xml:"Version,attr"`
	IssueInstant       string              `xml:"IssueInstant,attr"`
	Issuer             string              `xml:"Issuer"`
	Subject            SAMLSubject         `xml:"Subject"`
	Conditions         SAMLConditions      `xml:"Conditions"`
	AuthnStatement     SAMLAuthnStatement  `xml:"AuthnStatement"`
	AttributeStatement *SAMLAttributeStmt  `xml:"AttributeStatement"`
	Signature          *SAMLSignature      `xml:"Signature"`
}

// SAMLSubject contains the authenticated subject.
type SAMLSubject struct {
	NameID              SAMLNameID              `xml:"NameID"`
	SubjectConfirmation SAMLSubjectConfirmation `xml:"SubjectConfirmation"`
}

// SAMLNameID identifies the subject.
type SAMLNameID struct {
	Format string `xml:"Format,attr"`
	Value  string `xml:",chardata"`
}

// SAMLSubjectConfirmation contains confirmation data.
type SAMLSubjectConfirmation struct {
	Method                  string                      `xml:"Method,attr"`
	SubjectConfirmationData SAMLSubjectConfirmationData `xml:"SubjectConfirmationData"`
}

// SAMLSubjectConfirmationData contains timing constraints.
type SAMLSubjectConfirmationData struct {
	NotOnOrAfter string `xml:"NotOnOrAfter,attr"`
	Recipient    string `xml:"Recipient,attr"`
}

// SAMLConditions contains validity conditions.
type SAMLConditions struct {
	NotBefore           string                  `xml:"NotBefore,attr"`
	NotOnOrAfter        string                  `xml:"NotOnOrAfter,attr"`
	AudienceRestriction SAMLAudienceRestriction `xml:"AudienceRestriction"`
}

// SAMLAudienceRestriction specifies valid audiences.
type SAMLAudienceRestriction struct {
	Audience []string `xml:"Audience"`
}

// SAMLAuthnStatement contains authentication context.
type SAMLAuthnStatement struct {
	AuthnInstant string           `xml:"AuthnInstant,attr"`
	SessionIndex string           `xml:"SessionIndex,attr"`
	AuthnContext SAMLAuthnContext `xml:"AuthnContext"`
}

// SAMLAuthnContext describes how authentication occurred.
type SAMLAuthnContext struct {
	AuthnContextClassRef string `xml:"AuthnContextClassRef"`
}

// SAMLAttributeStmt contains SAML attributes.
type SAMLAttributeStmt struct {
	Attributes []SAMLAttribute `xml:"Attribute"`
}

// SAMLAttribute represents a SAML attribute.
type SAMLAttribute struct {
	Name         string   `xml:"Name,attr"`
	NameFormat   string   `xml:"NameFormat,attr"`
	FriendlyName string   `xml:"FriendlyName,attr"`
	Values       []string `xml:"AttributeValue"`
}

// SAMLSignature represents the XML signature.
type SAMLSignature struct {
	SignedInfo     SAMLSignedInfo `xml:"SignedInfo"`
	SignatureValue string         `xml:"SignatureValue"`
}

// SAMLSignedInfo contains signature algorithm info.
type SAMLSignedInfo struct {
	CanonicalizationMethod SAMLAlgorithm  `xml:"CanonicalizationMethod"`
	SignatureMethod        SAMLAlgorithm  `xml:"SignatureMethod"`
	Reference              SAMLReference  `xml:"Reference"`
}

// SAMLAlgorithm specifies an algorithm URI.
type SAMLAlgorithm struct {
	Algorithm string `xml:"Algorithm,attr"`
}

// SAMLReference contains digest info.
type SAMLReference struct {
	URI          string        `xml:"URI,attr"`
	DigestMethod SAMLAlgorithm `xml:"DigestMethod"`
	DigestValue  string        `xml:"DigestValue"`
}

// SAML validation errors.
var (
	ErrInvalidSAMLAssertion  = errors.New("invalid SAML assertion format")
	ErrSAMLSignatureInvalid  = errors.New("SAML signature verification failed")
	ErrSAMLAssertionExpired  = errors.New("SAML assertion has expired")
	ErrSAMLAssertionNotValid = errors.New("SAML assertion not yet valid")
	ErrSAMLIssuerMismatch    = errors.New("SAML issuer mismatch")
	ErrSAMLAudienceMismatch  = errors.New("SAML audience mismatch")
	ErrSAMLNoCertificate     = errors.New("no certificate configured for SAML validation")
)

// SAMLValidator validates SAML assertions from Microsoft Entra ID.
type SAMLValidator struct {
	entityID       string
	issuer         string
	certificate    *x509.Certificate
	allowClockSkew time.Duration
}

// NewSAMLValidator creates a SAML validator with the given configuration.
func NewSAMLValidator(entityID, issuer, certPEM string, clockSkewSeconds int) (*SAMLValidator, error) {
	if certPEM == "" {
		return nil, ErrSAMLNoCertificate
	}

	// Parse PEM certificate
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, errors.New("saml: failed to decode PEM certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.New("saml: failed to parse certificate: " + err.Error())
	}

	skew := time.Duration(clockSkewSeconds) * time.Second
	if skew == 0 {
		skew = 5 * time.Minute // Default 5 minute clock skew
	}

	return &SAMLValidator{
		entityID:       entityID,
		issuer:         issuer,
		certificate:    cert,
		allowClockSkew: skew,
	}, nil
}

// Validate validates a base64-encoded SAML assertion.
func (v *SAMLValidator) Validate(assertionB64 string) (*SAMLAssertion, error) {
	// Decode base64
	assertionXML, err := base64.StdEncoding.DecodeString(assertionB64)
	if err != nil {
		// Try URL-safe base64
		assertionXML, err = base64.RawURLEncoding.DecodeString(assertionB64)
		if err != nil {
			return nil, ErrInvalidSAMLAssertion
		}
	}

	// Parse XML
	var assertion SAMLAssertion
	if err := xml.Unmarshal(assertionXML, &assertion); err != nil {
		return nil, ErrInvalidSAMLAssertion
	}

	// Validate issuer
	if v.issuer != "" && assertion.Issuer != v.issuer {
		return nil, ErrSAMLIssuerMismatch
	}

	// Validate audience
	if v.entityID != "" {
		audienceValid := false
		for _, aud := range assertion.Conditions.AudienceRestriction.Audience {
			if aud == v.entityID {
				audienceValid = true
				break
			}
		}
		if !audienceValid && len(assertion.Conditions.AudienceRestriction.Audience) > 0 {
			return nil, ErrSAMLAudienceMismatch
		}
	}

	// Validate timing
	now := time.Now()

	if assertion.Conditions.NotBefore != "" {
		notBefore, err := time.Parse(time.RFC3339, assertion.Conditions.NotBefore)
		if err == nil && now.Add(v.allowClockSkew).Before(notBefore) {
			return nil, ErrSAMLAssertionNotValid
		}
	}

	if assertion.Conditions.NotOnOrAfter != "" {
		notOnOrAfter, err := time.Parse(time.RFC3339, assertion.Conditions.NotOnOrAfter)
		if err == nil && now.Add(-v.allowClockSkew).After(notOnOrAfter) {
			return nil, ErrSAMLAssertionExpired
		}
	}

	// Validate signature if present
	if assertion.Signature != nil && v.certificate != nil {
		if err := v.validateSignature(&assertion, assertionXML); err != nil {
			return nil, err
		}
	}

	return &assertion, nil
}

// validateSignature validates the XML signature on the assertion.
func (v *SAMLValidator) validateSignature(assertion *SAMLAssertion, rawXML []byte) error {
	if assertion.Signature == nil {
		return nil // No signature to validate
	}

	// Decode signature value
	sigBytes, err := base64.StdEncoding.DecodeString(
		strings.ReplaceAll(assertion.Signature.SignatureValue, " ", ""),
	)
	if err != nil {
		return ErrSAMLSignatureInvalid
	}

	// Get the public key
	pubKey, ok := v.certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("saml: certificate does not contain RSA public key")
	}

	// Hash the signed info (simplified - real implementation needs canonicalization)
	// For production, use a proper XML canonicalization library
	hash := sha256.Sum256(rawXML)

	// Verify signature
	err = rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sigBytes)
	if err != nil {
		return ErrSAMLSignatureInvalid
	}

	return nil
}

// ExtractSAMLAssertion extracts a SAML assertion from the request.
func ExtractSAMLAssertion(r *http.Request) string {
	// Check Authorization header with SAML scheme
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToUpper(auth), "SAML ") {
		return strings.TrimSpace(auth[5:])
	}

	// Check X-SAML-Assertion header
	if assertion := r.Header.Get("X-SAML-Assertion"); assertion != "" {
		return strings.TrimSpace(assertion)
	}

	return ""
}

// samlAuthenticator implements the Authenticator interface for SAML.
type samlAuthenticator struct {
	validator *SAMLValidator
}

// newSAMLAuthenticator creates a SAML authenticator from config.
func newSAMLAuthenticator(cfg AuthMethodConfig) (Authenticator, error) {
	samlCfg, ok := cfg.(*SAMLConfig)
	if !ok {
		return nil, errors.New("saml: invalid config type")
	}
	if err := samlCfg.Validate(); err != nil {
		return nil, err
	}

	validator, err := NewSAMLValidator(
		samlCfg.EntityID,
		samlCfg.Issuer,
		samlCfg.Certificate,
		samlCfg.AllowClockSkew,
	)
	if err != nil {
		return nil, err
	}

	return &samlAuthenticator{
		validator: validator,
	}, nil
}

// Name returns the authenticator name.
func (a *samlAuthenticator) Name() string { return "saml" }

// Authenticate validates a SAML assertion from the request.
func (a *samlAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	assertionB64 := ExtractSAMLAssertion(r)
	if assertionB64 == "" {
		return nil, nil // No credentials provided
	}

	assertion, err := a.validator.Validate(assertionB64)
	if err != nil {
		return nil, &AuthError{
			Status:  http.StatusUnauthorized,
			Code:    samlErrorCode(err),
			Message: err.Error(),
		}
	}

	// Extract attributes into metadata
	metadata := map[string]any{
		"issuer":        assertion.Issuer,
		"session_index": assertion.AuthnStatement.SessionIndex,
		"authn_instant": assertion.AuthnStatement.AuthnInstant,
	}

	if assertion.AttributeStatement != nil {
		attrs := make(map[string]any)
		for _, attr := range assertion.AttributeStatement.Attributes {
			if len(attr.Values) == 1 {
				attrs[attr.Name] = attr.Values[0]
			} else if len(attr.Values) > 1 {
				attrs[attr.Name] = attr.Values
			}
		}
		metadata["attributes"] = attrs
	}

	return &Principal{
		ID:       assertion.Subject.NameID.Value,
		Type:     ModeSAML,
		Metadata: metadata,
	}, nil
}

// samlErrorCode maps SAML errors to error codes.
func samlErrorCode(err error) string {
	switch err {
	case ErrInvalidSAMLAssertion:
		return "invalid_assertion"
	case ErrSAMLSignatureInvalid:
		return "invalid_signature"
	case ErrSAMLAssertionExpired:
		return "assertion_expired"
	case ErrSAMLAssertionNotValid:
		return "assertion_not_yet_valid"
	case ErrSAMLIssuerMismatch:
		return "invalid_issuer"
	case ErrSAMLAudienceMismatch:
		return "invalid_audience"
	default:
		return "invalid_assertion"
	}
}

func init() {
	// Register SAML authenticator with the default registry
	defaultRegistry.MustRegister("saml", newSAMLAuthenticator)
}
