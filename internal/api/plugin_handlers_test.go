package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"mdemg/internal/config"
)

func TestPluginCreate(t *testing.T) {
	// Create a temporary plugins directory
	tempDir, err := os.MkdirTemp("", "mdemg-test-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a minimal server with the temp plugins directory
	cfg := config.Config{
		PluginsDir: tempDir,
	}
	srv := &Server{cfg: cfg}

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, resp map[string]any)
	}{
		{
			name: "create ingestion plugin",
			requestBody: map[string]any{
				"name":        "My Custom Ingestion",
				"type":        "INGESTION",
				"version":     "1.0.0",
				"description": "A test ingestion plugin",
				"author":      "Test Author",
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				if !ok {
					t.Fatalf("expected data object in response")
				}

				if data["plugin_id"] != "my-custom-ingestion" {
					t.Errorf("expected plugin_id 'my-custom-ingestion', got %v", data["plugin_id"])
				}

				filesCreated, ok := data["files_created"].([]any)
				if !ok || len(filesCreated) != 5 {
					t.Errorf("expected 5 files created, got %v", filesCreated)
				}

				validation, ok := data["validation"].(map[string]any)
				if !ok {
					t.Fatalf("expected validation object in response")
				}

				// Manifest validation will fail because binary doesn't exist yet
				// This is expected for newly scaffolded plugins
				// The user needs to run 'make build' first
				_, hasErrors := validation["errors"]
				if !hasErrors {
					t.Errorf("expected validation errors (binary not built yet)")
				}

				nextSteps, ok := data["next_steps"].([]any)
				if !ok || len(nextSteps) != 3 {
					t.Errorf("expected 3 next_steps, got %v", nextSteps)
				}
			},
		},
		{
			name: "create reasoning plugin",
			requestBody: map[string]any{
				"name": "Keyword Booster",
				"type": "REASONING",
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				if !ok {
					t.Fatalf("expected data object in response")
				}

				if data["plugin_id"] != "keyword-booster" {
					t.Errorf("expected plugin_id 'keyword-booster', got %v", data["plugin_id"])
				}
			},
		},
		{
			name: "create APE plugin",
			requestBody: map[string]any{
				"name": "Session Reflector",
				"type": "APE",
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				if !ok {
					t.Fatalf("expected data object in response")
				}

				if data["plugin_id"] != "session-reflector" {
					t.Errorf("expected plugin_id 'session-reflector', got %v", data["plugin_id"])
				}
			},
		},
		{
			name: "missing name",
			requestBody: map[string]any{
				"type": "INGESTION",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "missing type",
			requestBody: map[string]any{
				"name": "Test Plugin",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "invalid type",
			requestBody: map[string]any{
				"name": "Test Plugin",
				"type": "INVALID",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "lowercase type is normalized",
			requestBody: map[string]any{
				"name": "Lowercase Type Test",
				"type": "ingestion",
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				if !ok {
					t.Fatalf("expected data object in response")
				}

				if data["plugin_id"] != "lowercase-type-test" {
					t.Errorf("expected plugin_id 'lowercase-type-test', got %v", data["plugin_id"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/plugins/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.handlePluginCreate(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			var resp map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}

			if tt.expectError {
				if _, ok := resp["error"]; !ok {
					t.Errorf("expected error in response")
				}
			} else if tt.checkResponse != nil {
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestPluginCreateMethodNotAllowed(t *testing.T) {
	cfg := config.Config{
		PluginsDir: os.TempDir(),
	}
	srv := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/create", nil)
	rr := httptest.NewRecorder()
	srv.handlePluginCreate(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestPluginList(t *testing.T) {
	// Create a temporary plugins directory with a test plugin
	tempDir, err := os.MkdirTemp("", "mdemg-test-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test plugin directory with manifest
	pluginDir := filepath.Join(tempDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	manifest := `{
  "id": "test-plugin",
  "name": "Test Plugin",
  "version": "1.0.0",
  "type": "INGESTION",
  "binary": "test-plugin",
  "capabilities": {
    "ingestion_sources": ["test://"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	cfg := config.Config{
		PluginsDir: tempDir,
	}
	srv := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins", nil)
	rr := httptest.NewRecorder()
	srv.handlePluginList(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response")
	}

	plugins, ok := data["plugins"].([]any)
	if !ok {
		t.Fatalf("expected plugins array in response")
	}

	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
	}

	plugin := plugins[0].(map[string]any)
	if plugin["id"] != "test-plugin" {
		t.Errorf("expected plugin id 'test-plugin', got %v", plugin["id"])
	}
	if plugin["status"] != "stopped" {
		t.Errorf("expected status 'stopped', got %v", plugin["status"])
	}
}

func TestPluginListMethodNotAllowed(t *testing.T) {
	cfg := config.Config{
		PluginsDir: os.TempDir(),
	}
	srv := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/plugins", nil)
	rr := httptest.NewRecorder()
	srv.handlePluginList(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestPluginDetail(t *testing.T) {
	// Create a temporary plugins directory with a test plugin
	tempDir, err := os.MkdirTemp("", "mdemg-test-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test plugin directory with manifest
	pluginDir := filepath.Join(tempDir, "detail-test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	manifest := `{
  "id": "detail-test-plugin",
  "name": "Detail Test Plugin",
  "version": "2.0.0",
  "type": "REASONING",
  "binary": "detail-test-plugin",
  "capabilities": {
    "pattern_detectors": ["custom_ranking"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	cfg := config.Config{
		PluginsDir: tempDir,
	}
	srv := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/detail-test-plugin", nil)
	rr := httptest.NewRecorder()
	srv.handlePluginDetail(rr, req, "detail-test-plugin")

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response")
	}

	if data["id"] != "detail-test-plugin" {
		t.Errorf("expected id 'detail-test-plugin', got %v", data["id"])
	}
	if data["name"] != "Detail Test Plugin" {
		t.Errorf("expected name 'Detail Test Plugin', got %v", data["name"])
	}
	if data["version"] != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %v", data["version"])
	}
	if data["type"] != "REASONING" {
		t.Errorf("expected type 'REASONING', got %v", data["type"])
	}
	if data["status"] != "stopped" {
		t.Errorf("expected status 'stopped', got %v", data["status"])
	}

	manifest_data, ok := data["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("expected manifest object in response")
	}
	if manifest_data["id"] != "detail-test-plugin" {
		t.Errorf("expected manifest.id 'detail-test-plugin', got %v", manifest_data["id"])
	}
}

func TestPluginDetailNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mdemg-test-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.Config{
		PluginsDir: tempDir,
	}
	srv := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/nonexistent", nil)
	rr := httptest.NewRecorder()
	srv.handlePluginDetail(rr, req, "nonexistent")

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestPluginValidate(t *testing.T) {
	// Create a temporary plugins directory with a test plugin
	tempDir, err := os.MkdirTemp("", "mdemg-test-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test plugin directory with valid manifest
	pluginDir := filepath.Join(tempDir, "validate-test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	manifest := `{
  "id": "validate-test-plugin",
  "name": "Validate Test Plugin",
  "version": "1.0.0",
  "type": "INGESTION",
  "binary": "validate-test-plugin",
  "capabilities": {
    "ingestion_sources": ["test://"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	cfg := config.Config{
		PluginsDir: tempDir,
	}
	srv := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/validate-test-plugin/validate", nil)
	rr := httptest.NewRecorder()
	srv.handlePluginValidate(rr, req, "validate-test-plugin")

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response")
	}

	// Manifest validation will fail because binary doesn't exist
	// (ValidateManifest checks that the entrypoint binary exists)
	manifestResult, ok := data["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("expected manifest validation result")
	}
	// Binary doesn't exist, so manifest validation should fail
	if manifestResult["valid"] != false {
		t.Errorf("expected manifest to be invalid (binary missing), got %v", manifestResult["valid"])
	}

	// Proto validation should also indicate binary not found
	protoResult, ok := data["proto"].(map[string]any)
	if !ok {
		t.Fatalf("expected proto validation result")
	}
	if protoResult["valid"] != false {
		t.Errorf("expected proto validation to fail (no binary)")
	}

	// Overall should be invalid (no binary)
	if data["valid"] != false {
		t.Errorf("expected overall validation to be false (no binary)")
	}
}

func TestPluginValidateNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mdemg-test-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.Config{
		PluginsDir: tempDir,
	}
	srv := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/nonexistent/validate", nil)
	rr := httptest.NewRecorder()
	srv.handlePluginValidate(rr, req, "nonexistent")

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestPluginValidateMethodNotAllowed(t *testing.T) {
	cfg := config.Config{
		PluginsDir: os.TempDir(),
	}
	srv := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/test/validate", nil)
	rr := httptest.NewRecorder()
	srv.handlePluginValidate(rr, req, "test")

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestPluginOperation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mdemg-test-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.Config{
		PluginsDir: tempDir,
	}
	srv := &Server{cfg: cfg}

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "list plugins",
			method:         http.MethodGet,
			path:           "/v1/plugins",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "list plugins with trailing slash",
			method:         http.MethodGet,
			path:           "/v1/plugins/",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "create requires POST",
			method:         http.MethodGet,
			path:           "/v1/plugins/create",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "unknown action",
			method:         http.MethodGet,
			path:           "/v1/plugins/some-plugin/unknown-action",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			srv.handlePluginOperation(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestGeneratedPluginFilesExist(t *testing.T) {
	// Create a temporary plugins directory
	tempDir, err := os.MkdirTemp("", "mdemg-test-plugins-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.Config{
		PluginsDir: tempDir,
	}
	srv := &Server{cfg: cfg}

	// Create a plugin
	body, _ := json.Marshal(map[string]any{
		"name": "File Check Test",
		"type": "INGESTION",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.handlePluginCreate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rr.Code, rr.Body.String())
	}

	// Check that all expected files exist
	pluginDir := filepath.Join(tempDir, "file-check-test")
	expectedFiles := []string{
		"manifest.json",
		"main.go",
		"handler.go",
		"Makefile",
		"README.md",
	}

	for _, file := range expectedFiles {
		path := filepath.Join(pluginDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", file)
		}
	}

	// Verify manifest content
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if manifest["id"] != "file-check-test" {
		t.Errorf("expected manifest id 'file-check-test', got %v", manifest["id"])
	}
	if manifest["type"] != "INGESTION" {
		t.Errorf("expected manifest type 'INGESTION', got %v", manifest["type"])
	}
}
