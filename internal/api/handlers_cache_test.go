package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleCacheClear_MethodNotAllowed tests that handleCacheClear returns 405 for non-DELETE methods
func TestHandleCacheClear_MethodNotAllowed(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"GET method not allowed", http.MethodGet},
		{"POST method not allowed", http.MethodPost},
		{"PUT method not allowed", http.MethodPut},
		{"PATCH method not allowed", http.MethodPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}

			req := httptest.NewRequest(tt.method, "/v1/memory/cache?confirm=true", nil)
			w := httptest.NewRecorder()

			s.handleCacheClear(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleCacheClear(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestHandleCacheClear_RequiresConfirmation tests that handleCacheClear returns 400 without confirm=true
func TestHandleCacheClear_RequiresConfirmation(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"no confirm param", "/v1/memory/cache"},
		{"confirm=false", "/v1/memory/cache?confirm=false"},
		{"confirm empty", "/v1/memory/cache?confirm="},
		{"confirm with space_id but no confirm", "/v1/memory/cache?space_id=test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}

			req := httptest.NewRequest(http.MethodDelete, tt.url, nil)
			w := httptest.NewRecorder()

			s.handleCacheClear(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("handleCacheClear() status = %d, want %d", w.Code, http.StatusBadRequest)
			}

			var resp map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}

			if resp["error"] != "confirmation required" {
				t.Errorf("expected error 'confirmation required', got %v", resp["error"])
			}

			if msg, ok := resp["message"].(string); !ok || msg == "" {
				t.Error("expected non-empty message field")
			}
		})
	}
}

// TestHandleCacheClear_ConfirmationMessage tests that the error message includes instructions
func TestHandleCacheClear_ConfirmationMessage(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodDelete, "/v1/memory/cache", nil)
	w := httptest.NewRecorder()

	s.handleCacheClear(w, req)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	msg, ok := resp["message"].(string)
	if !ok {
		t.Fatal("expected message to be a string")
	}

	// Message should tell user how to confirm
	if msg == "" {
		t.Error("message should not be empty")
	}
	if len(msg) < 10 {
		t.Errorf("message should be informative, got: %s", msg)
	}
}
