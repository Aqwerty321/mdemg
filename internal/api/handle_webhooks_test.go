package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mdemg/internal/config"
)

func TestVerifyLinearSignature_Valid(t *testing.T) {
	secret := "test-secret-key"
	body := []byte(`{"action":"create","type":"Issue","data":{"id":"123"}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	if !verifyLinearSignature(body, signature, secret) {
		t.Error("expected valid signature to pass verification")
	}
}

func TestVerifyLinearSignature_Invalid(t *testing.T) {
	secret := "test-secret-key"
	body := []byte(`{"action":"create","type":"Issue","data":{"id":"123"}}`)

	if verifyLinearSignature(body, "invalid-signature", secret) {
		t.Error("expected invalid signature to fail verification")
	}
}

func TestVerifyLinearSignature_WrongSecret(t *testing.T) {
	secret := "test-secret-key"
	wrongSecret := "wrong-secret"
	body := []byte(`{"action":"create","type":"Issue","data":{"id":"123"}}`)

	mac := hmac.New(sha256.New, []byte(wrongSecret))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	if verifyLinearSignature(body, signature, secret) {
		t.Error("expected signature with wrong secret to fail")
	}
}

func TestHandleLinearWebhook_MethodNotAllowed(t *testing.T) {
	s := &Server{
		cfg:              config.Config{LinearWebhookSecret: "secret"},
		webhookDebouncer: newLinearWebhookDebouncer(),
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/webhooks/linear", nil)
	w := httptest.NewRecorder()

	s.handleLinearWebhook(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleLinearWebhook_NoSecretConfigured(t *testing.T) {
	s := &Server{
		cfg:              config.Config{LinearWebhookSecret: ""},
		webhookDebouncer: newLinearWebhookDebouncer(),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/linear",
		strings.NewReader(`{"action":"create","type":"Issue","data":{"id":"123"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleLinearWebhook(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleLinearWebhook_MissingSignature(t *testing.T) {
	s := &Server{
		cfg:              config.Config{LinearWebhookSecret: "secret"},
		webhookDebouncer: newLinearWebhookDebouncer(),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/linear",
		strings.NewReader(`{"action":"create","type":"Issue","data":{"id":"123"}}`))
	req.Header.Set("Content-Type", "application/json")
	// No Linear-Signature header
	w := httptest.NewRecorder()

	s.handleLinearWebhook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleLinearWebhook_InvalidSignature(t *testing.T) {
	s := &Server{
		cfg:              config.Config{LinearWebhookSecret: "secret"},
		webhookDebouncer: newLinearWebhookDebouncer(),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/linear",
		strings.NewReader(`{"action":"create","type":"Issue","data":{"id":"123"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Linear-Signature", "bad-signature")
	w := httptest.NewRecorder()

	s.handleLinearWebhook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleLinearWebhook_IgnoredEventType(t *testing.T) {
	secret := "test-secret"
	body := `{"action":"remove","type":"Comment","data":{"id":"c1"}}`

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	signature := hex.EncodeToString(mac.Sum(nil))

	s := &Server{
		cfg:              config.Config{LinearWebhookSecret: secret},
		webhookDebouncer: newLinearWebhookDebouncer(),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/linear",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Linear-Signature", signature)
	w := httptest.NewRecorder()

	s.handleLinearWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ignored" {
		t.Errorf("expected status=ignored, got %v", resp["status"])
	}
}

func TestHandleLinearWebhook_AcceptsValidIssueCreate(t *testing.T) {
	secret := "test-secret"
	body := `{"action":"create","type":"Issue","data":{"id":"ISS-1","title":"Test Issue"}}`

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	signature := hex.EncodeToString(mac.Sum(nil))

	s := &Server{
		cfg:              config.Config{LinearWebhookSecret: secret},
		webhookDebouncer: newLinearWebhookDebouncer(),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/linear",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Linear-Signature", signature)
	w := httptest.NewRecorder()

	s.handleLinearWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "accepted" {
		t.Errorf("expected status=accepted, got %v", resp["status"])
	}
}

func TestLinearWebhookDebouncer_CoalescesRapidEvents(t *testing.T) {
	var mu = struct {
		sync.Mutex
		calls int
	}{}

	deb := newLinearWebhookDebouncer()

	// Fire 5 rapid events for the same key
	for i := 0; i < 5; i++ {
		deb.debounce("Issue:123", func() {
			mu.Lock()
			mu.calls++
			mu.Unlock()
		})
	}

	// Wait for debounce window (10s) + buffer
	// Note: In production the window is 10s, but for testing we verify
	// that after the window, only one call was made.
	// We use a shorter check first to confirm no premature firing.
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	earlyCount := mu.calls
	mu.Unlock()
	if earlyCount != 0 {
		t.Errorf("expected 0 calls before debounce window, got %d", earlyCount)
	}

	// Wait for full debounce window
	time.Sleep(10*time.Second + 500*time.Millisecond)
	mu.Lock()
	finalCount := mu.calls
	mu.Unlock()
	if finalCount != 1 {
		t.Errorf("expected exactly 1 call after debounce, got %d", finalCount)
	}
}
