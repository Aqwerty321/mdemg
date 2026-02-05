package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleCleanupOrphans_MethodNotAllowed tests that handleCleanupOrphans returns 405 for non-POST methods
func TestHandleCleanupOrphans_MethodNotAllowed(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"GET method not allowed", http.MethodGet},
		{"PUT method not allowed", http.MethodPut},
		{"DELETE method not allowed", http.MethodDelete},
		{"PATCH method not allowed", http.MethodPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}

			req := httptest.NewRequest(tt.method, "/v1/memory/cleanup/orphans", nil)
			w := httptest.NewRecorder()

			s.handleCleanupOrphans(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleCleanupOrphans(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestHandleCleanupOrphans_MissingSpaceID tests that handleCleanupOrphans returns 400 for missing space_id
func TestHandleCleanupOrphans_MissingSpaceID(t *testing.T) {
	s := &Server{}

	body := map[string]any{
		"action": "list",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/memory/cleanup/orphans", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleCleanupOrphans(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("handleCleanupOrphans() status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var respBody map[string]any
	if err := json.NewDecoder(w.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	// Should contain a validation error (either "error" or "errors" key)
	hasError := false
	if _, ok := respBody["error"]; ok {
		hasError = true
	}
	if _, ok := respBody["errors"]; ok {
		hasError = true
	}
	if !hasError {
		t.Errorf("expected error or errors in response body, got: %v", respBody)
	}
}

// TestHandleCleanupOrphans_InvalidAction tests that handleCleanupOrphans returns 400 for invalid action
func TestHandleCleanupOrphans_InvalidAction(t *testing.T) {
	s := &Server{}

	body := map[string]any{
		"space_id": "test-space",
		"action":   "purge",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/memory/cleanup/orphans", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleCleanupOrphans(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("handleCleanupOrphans() status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var respBody map[string]any
	if err := json.NewDecoder(w.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	// Should contain a validation error (either "error" or "errors" key)
	hasError := false
	if _, ok := respBody["error"]; ok {
		hasError = true
	}
	if _, ok := respBody["errors"]; ok {
		hasError = true
	}
	if !hasError {
		t.Errorf("expected error or errors in response body, got: %v", respBody)
	}
}

// TestHandleCleanupOrphans_ProtectedSpaceDelete tests that handleCleanupOrphans returns 403
// when attempting to delete from a protected space
func TestHandleCleanupOrphans_ProtectedSpaceDelete(t *testing.T) {
	s := &Server{}

	body := map[string]any{
		"space_id": "mdemg-dev",
		"action":   "delete",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/memory/cleanup/orphans", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleCleanupOrphans(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("handleCleanupOrphans() status = %d, want %d", w.Code, http.StatusForbidden)
	}

	var respBody map[string]any
	if err := json.NewDecoder(w.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if errMsg, ok := respBody["error"].(string); ok {
		if !strings.Contains(errMsg, "protected") {
			t.Errorf("error message should mention protected, got: %q", errMsg)
		}
	} else {
		t.Errorf("expected error in response body, got: %v", respBody)
	}
}

// TestHandleCleanupOrphans_ArchiveNotProtected tests that archive action is allowed on protected spaces
// (only delete is blocked on protected spaces)
func TestHandleCleanupOrphans_ArchiveNotProtected(t *testing.T) {
	s := &Server{}

	body := map[string]any{
		"space_id": "mdemg-dev",
		"action":   "archive",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/memory/cleanup/orphans", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// This will pass validation but fail on nil driver — which means
	// the protected space check did NOT block the archive action (correct behavior)
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil driver panic means we got past the protected check
				t.Logf("Expected panic due to nil driver: %v", r)
			}
		}()
		s.handleCleanupOrphans(w, req)
	}()

	// Should NOT be 403 — archive is allowed on protected spaces
	if w.Code == http.StatusForbidden {
		t.Errorf("handleCleanupOrphans(archive) on protected space should not return 403")
	}
}
