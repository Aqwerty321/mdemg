package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleMetrics_MethodNotAllowed tests that handleMetrics returns 405 for non-GET methods
func TestHandleMetrics_MethodNotAllowed(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"POST method not allowed", http.MethodPost},
		{"PUT method not allowed", http.MethodPut},
		{"DELETE method not allowed", http.MethodDelete},
		{"PATCH method not allowed", http.MethodPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server (no driver needed for method validation)
			s := &Server{}

			// Create test request with the specified method
			req := httptest.NewRequest(tt.method, "/v1/metrics", nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleMetrics(w, req)

			// Verify response
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleMetrics(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}
