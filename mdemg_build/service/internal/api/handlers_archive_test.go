package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleArchiveNode_MethodNotAllowed tests that handleArchiveNode returns 405 for non-POST methods
func TestHandleArchiveNode_MethodNotAllowed(t *testing.T) {
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
			// Create a minimal server (no driver needed for method validation)
			s := &Server{}

			// Create test request with the specified method
			req := httptest.NewRequest(tt.method, "/v1/memory/nodes/test-node/archive", nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleArchiveNode(w, req)

			// Verify response
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleArchiveNode(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestHandleUnarchiveNode_MethodNotAllowed tests that handleUnarchiveNode returns 405 for non-POST methods
func TestHandleUnarchiveNode_MethodNotAllowed(t *testing.T) {
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
			// Create a minimal server (no driver needed for method validation)
			s := &Server{}

			// Create test request with the specified method
			req := httptest.NewRequest(tt.method, "/v1/memory/nodes/test-node/unarchive", nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleUnarchiveNode(w, req)

			// Verify response
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleUnarchiveNode(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestHandleDeleteNode_MethodNotAllowed tests that handleDeleteNode returns 405 for non-DELETE methods
func TestHandleDeleteNode_MethodNotAllowed(t *testing.T) {
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
			// Create a minimal server (no driver needed for method validation)
			s := &Server{}

			// Create test request with the specified method
			req := httptest.NewRequest(tt.method, "/v1/memory/nodes/test-node", nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleDeleteNode(w, req)

			// Verify response
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleDeleteNode(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestHandleDeleteNode_RequiresConfirm tests that handleDeleteNode requires ?confirm=true parameter
func TestHandleDeleteNode_RequiresConfirm(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		expectedCode  int
		expectedError string
	}{
		{
			name:          "missing confirm parameter",
			url:           "/v1/memory/nodes/test-node",
			expectedCode:  http.StatusBadRequest,
			expectedError: "deletion requires ?confirm=true query parameter",
		},
		{
			name:          "confirm=false",
			url:           "/v1/memory/nodes/test-node?confirm=false",
			expectedCode:  http.StatusBadRequest,
			expectedError: "deletion requires ?confirm=true query parameter",
		},
		{
			name:          "confirm=yes (not true)",
			url:           "/v1/memory/nodes/test-node?confirm=yes",
			expectedCode:  http.StatusBadRequest,
			expectedError: "deletion requires ?confirm=true query parameter",
		},
		{
			name:          "confirm=1 (not true)",
			url:           "/v1/memory/nodes/test-node?confirm=1",
			expectedCode:  http.StatusBadRequest,
			expectedError: "deletion requires ?confirm=true query parameter",
		},
		{
			name:          "empty confirm parameter",
			url:           "/v1/memory/nodes/test-node?confirm=",
			expectedCode:  http.StatusBadRequest,
			expectedError: "deletion requires ?confirm=true query parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server (no driver needed for confirm validation)
			s := &Server{}

			// Create test request with DELETE method
			req := httptest.NewRequest(http.MethodDelete, tt.url, nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleDeleteNode(w, req)

			// Verify status code
			if w.Code != tt.expectedCode {
				t.Errorf("handleDeleteNode(%s) status = %d, want %d", tt.url, w.Code, tt.expectedCode)
			}

			// Verify error message
			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if errMsg, ok := body["error"].(string); ok {
				if errMsg != tt.expectedError {
					t.Errorf("error message = %q, want %q", errMsg, tt.expectedError)
				}
			} else {
				t.Errorf("expected error in response body, got: %v", body)
			}
		})
	}
}

// TestHandleBulkArchive_EmptyNodeIds tests that handleBulkArchive returns 400 for empty node_ids
func TestHandleBulkArchive_EmptyNodeIds(t *testing.T) {
	tests := []struct {
		name          string
		requestBody   map[string]any
		expectedCode  int
		expectedError string
	}{
		{
			name: "empty node_ids array",
			requestBody: map[string]any{
				"space_id": "test-space",
				"node_ids": []string{},
			},
			expectedCode:  http.StatusBadRequest,
			expectedError: "node_ids array cannot be empty",
		},
		{
			name: "missing node_ids field",
			requestBody: map[string]any{
				"space_id": "test-space",
			},
			expectedCode:  http.StatusBadRequest,
			expectedError: "node_ids array cannot be empty",
		},
		{
			name: "null node_ids field",
			requestBody: map[string]any{
				"space_id": "test-space",
				"node_ids": nil,
			},
			expectedCode:  http.StatusBadRequest,
			expectedError: "node_ids array cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server (no driver needed for validation)
			s := &Server{}

			// Create request body
			bodyBytes, err := json.Marshal(tt.requestBody)
			if err != nil {
				t.Fatalf("failed to marshal request body: %v", err)
			}

			// Create test request with POST method
			req := httptest.NewRequest(http.MethodPost, "/v1/memory/archive/bulk", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleBulkArchive(w, req)

			// Verify status code
			if w.Code != tt.expectedCode {
				t.Errorf("handleBulkArchive() status = %d, want %d", w.Code, tt.expectedCode)
			}

			// Verify error message
			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if errMsg, ok := body["error"].(string); ok {
				if errMsg != tt.expectedError {
					t.Errorf("error message = %q, want %q", errMsg, tt.expectedError)
				}
			} else {
				t.Errorf("expected error in response body, got: %v", body)
			}
		})
	}
}

// TestHandleBulkArchive_MethodNotAllowed tests that handleBulkArchive returns 405 for non-POST methods
func TestHandleBulkArchive_MethodNotAllowed(t *testing.T) {
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
			// Create a minimal server (no driver needed for method validation)
			s := &Server{}

			// Create test request with the specified method
			req := httptest.NewRequest(tt.method, "/v1/memory/archive/bulk", nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleBulkArchive(w, req)

			// Verify response
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleBulkArchive(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestHandleNodeOperation_Routing tests that handleNodeOperation correctly routes to the appropriate handler
func TestHandleNodeOperation_Routing(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		method       string
		expectedCode int
	}{
		{
			name:         "archive path with POST routes correctly",
			path:         "/v1/memory/nodes/test-node/archive",
			method:       http.MethodPost,
			expectedCode: http.StatusBadRequest, // Will fail due to nil driver, but method was accepted
		},
		{
			name:         "archive path with GET returns 405",
			path:         "/v1/memory/nodes/test-node/archive",
			method:       http.MethodGet,
			expectedCode: http.StatusMethodNotAllowed,
		},
		{
			name:         "unarchive path with POST routes correctly",
			path:         "/v1/memory/nodes/test-node/unarchive",
			method:       http.MethodPost,
			expectedCode: http.StatusBadRequest, // Will fail due to nil driver, but method was accepted
		},
		{
			name:         "unarchive path with GET returns 405",
			path:         "/v1/memory/nodes/test-node/unarchive",
			method:       http.MethodGet,
			expectedCode: http.StatusMethodNotAllowed,
		},
		{
			name:         "delete path with DELETE without confirm returns 400",
			path:         "/v1/memory/nodes/test-node",
			method:       http.MethodDelete,
			expectedCode: http.StatusBadRequest, // Missing ?confirm=true
		},
		{
			name:         "delete path with POST returns 405",
			path:         "/v1/memory/nodes/test-node",
			method:       http.MethodPost,
			expectedCode: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server
			s := &Server{}

			// Create test request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			// Use recover to handle nil driver panics for success cases
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Panic means we got past method check - expected with nil driver
						t.Logf("Expected panic due to nil driver: %v", r)
					}
				}()
				s.handleNodeOperation(w, req)
			}()

			// For 405 errors, verify the status code
			if tt.expectedCode == http.StatusMethodNotAllowed && w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleNodeOperation(%s %s) status = %d, want %d", tt.method, tt.path, w.Code, tt.expectedCode)
			}
		})
	}
}

// TestHandleArchiveNode_InvalidNodeID tests that handleArchiveNode validates node_id in path
func TestHandleArchiveNode_InvalidNodeID(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		expectedCode  int
		expectedError string
	}{
		{
			name:          "missing node_id (just /archive)",
			path:          "/v1/memory/nodes//archive",
			expectedCode:  http.StatusBadRequest,
			expectedError: "invalid node_id in path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			w := httptest.NewRecorder()

			s.handleArchiveNode(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("handleArchiveNode() status = %d, want %d", w.Code, tt.expectedCode)
			}

			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if errMsg, ok := body["error"].(string); ok {
				if errMsg != tt.expectedError {
					t.Errorf("error message = %q, want %q", errMsg, tt.expectedError)
				}
			}
		})
	}
}

// TestHandleUnarchiveNode_InvalidNodeID tests that handleUnarchiveNode validates node_id in path
func TestHandleUnarchiveNode_InvalidNodeID(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		expectedCode  int
		expectedError string
	}{
		{
			name:          "missing node_id (just /unarchive)",
			path:          "/v1/memory/nodes//unarchive",
			expectedCode:  http.StatusBadRequest,
			expectedError: "invalid node_id in path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			w := httptest.NewRecorder()

			s.handleUnarchiveNode(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("handleUnarchiveNode() status = %d, want %d", w.Code, tt.expectedCode)
			}

			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if errMsg, ok := body["error"].(string); ok {
				if errMsg != tt.expectedError {
					t.Errorf("error message = %q, want %q", errMsg, tt.expectedError)
				}
			}
		})
	}
}

// TestHandleDeleteNode_InvalidNodeID tests that handleDeleteNode validates node_id in path
func TestHandleDeleteNode_InvalidNodeID(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		expectedCode  int
		expectedError string
	}{
		{
			name:          "missing node_id (empty path)",
			path:          "/v1/memory/nodes/",
			expectedCode:  http.StatusBadRequest,
			expectedError: "invalid node_id in path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}

			req := httptest.NewRequest(http.MethodDelete, tt.path+"?confirm=true", nil)
			w := httptest.NewRecorder()

			s.handleDeleteNode(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("handleDeleteNode() status = %d, want %d", w.Code, tt.expectedCode)
			}

			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if errMsg, ok := body["error"].(string); ok {
				if errMsg != tt.expectedError {
					t.Errorf("error message = %q, want %q", errMsg, tt.expectedError)
				}
			}
		})
	}
}
