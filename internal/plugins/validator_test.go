package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateManifest_ValidManifest(t *testing.T) {
	// Create temp directory with valid manifest
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a dummy binary file
	binaryPath := filepath.Join(pluginDir, "test-plugin")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]interface{}{
		"id":      "test-plugin",
		"name":    "Test Plugin",
		"version": "1.0.0",
		"type":    "INGESTION",
		"binary":  "test-plugin",
		"capabilities": map[string]interface{}{
			"ingestion_sources": []string{"test://"},
			"content_types":     []string{"text/plain"},
		},
		"health_check_interval_ms": 5000,
		"startup_timeout_ms":       10000,
	}

	manifestData, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateManifest(pluginDir)
	if err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected valid manifest, got errors: %v", result.Errors)
	}

	if result.Manifest == nil {
		t.Fatal("Expected manifest to be parsed")
	}

	if result.Manifest.ID != "test-plugin" {
		t.Errorf("Expected ID 'test-plugin', got %q", result.Manifest.ID)
	}

	if result.Manifest.Type != "INGESTION" {
		t.Errorf("Expected Type 'INGESTION', got %q", result.Manifest.Type)
	}

	if len(result.Warnings) > 0 {
		t.Logf("Warnings: %v", result.Warnings)
	}
}

func TestValidateManifest_MissingRequiredFields(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name            string
		manifest        map[string]interface{}
		expectedError   string
		createBinary    bool
	}{
		{
			name:          "missing id",
			manifest:      map[string]interface{}{"name": "Test", "version": "1.0.0", "type": "INGESTION", "binary": "test"},
			expectedError: "missing required field: id",
			createBinary:  true,
		},
		{
			name:          "missing name",
			manifest:      map[string]interface{}{"id": "test", "version": "1.0.0", "type": "INGESTION", "binary": "test"},
			expectedError: "missing required field: name",
			createBinary:  true,
		},
		{
			name:          "missing version",
			manifest:      map[string]interface{}{"id": "test", "name": "Test", "type": "INGESTION", "binary": "test"},
			expectedError: "missing required field: version",
			createBinary:  true,
		},
		{
			name:          "missing type",
			manifest:      map[string]interface{}{"id": "test", "name": "Test", "version": "1.0.0", "binary": "test"},
			expectedError: "missing required field: type",
			createBinary:  true,
		},
		{
			name:          "missing binary",
			manifest:      map[string]interface{}{"id": "test", "name": "Test", "version": "1.0.0", "type": "INGESTION"},
			expectedError: "missing required field: binary",
			createBinary:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test directory
			testDir := filepath.Join(tmpDir, tc.name)
			if err := os.MkdirAll(testDir, 0755); err != nil {
				t.Fatal(err)
			}

			// Create binary if needed
			if tc.createBinary {
				binaryPath := filepath.Join(testDir, "test")
				if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
					t.Fatal(err)
				}
			}

			manifestData, _ := json.Marshal(tc.manifest)
			manifestPath := filepath.Join(testDir, "manifest.json")
			if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
				t.Fatal(err)
			}

			result, err := ValidateManifest(testDir)
			if err != nil {
				t.Fatalf("ValidateManifest returned error: %v", err)
			}

			if result.Valid {
				t.Error("Expected validation to fail")
			}

			found := false
			for _, e := range result.Errors {
				if e == tc.expectedError {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected error %q, got errors: %v", tc.expectedError, result.Errors)
			}
		})
	}
}

func TestValidateManifest_InvalidSemver(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy binary
	binaryPath := filepath.Join(pluginDir, "test-plugin")
	if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	invalidVersions := []string{
		"1.0",       // missing patch
		"v1.0.0",    // has 'v' prefix
		"1",         // only major
		"1.0.0.0",   // too many parts
		"one.two.three", // non-numeric
		"",          // empty (handled separately)
	}

	for _, version := range invalidVersions {
		t.Run("version_"+version, func(t *testing.T) {
			manifest := map[string]interface{}{
				"id":      "test-plugin",
				"name":    "Test Plugin",
				"version": version,
				"type":    "INGESTION",
				"binary":  "test-plugin",
			}

			manifestData, _ := json.Marshal(manifest)
			manifestPath := filepath.Join(pluginDir, "manifest.json")
			if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
				t.Fatal(err)
			}

			result, err := ValidateManifest(pluginDir)
			if err != nil {
				t.Fatalf("ValidateManifest returned error: %v", err)
			}

			if result.Valid {
				t.Errorf("Expected validation to fail for version %q", version)
			}
		})
	}
}

func TestValidateManifest_ValidSemverFormats(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy binary
	binaryPath := filepath.Join(pluginDir, "test-plugin")
	if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	validVersions := []string{
		"1.0.0",
		"0.1.0",
		"10.20.30",
		"1.0.0-alpha",
		"1.0.0-beta.1",
		"1.0.0-rc.1",
		"1.0.0+build",
		"1.0.0-alpha+build",
	}

	for _, version := range validVersions {
		t.Run("version_"+version, func(t *testing.T) {
			manifest := map[string]interface{}{
				"id":      "test-plugin",
				"name":    "Test Plugin",
				"version": version,
				"type":    "INGESTION",
				"binary":  "test-plugin",
			}

			manifestData, _ := json.Marshal(manifest)
			manifestPath := filepath.Join(pluginDir, "manifest.json")
			if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
				t.Fatal(err)
			}

			result, err := ValidateManifest(pluginDir)
			if err != nil {
				t.Fatalf("ValidateManifest returned error: %v", err)
			}

			// Check specifically for version errors
			hasVersionError := false
			for _, e := range result.Errors {
				if len(e) > 7 && e[:7] == "version" {
					hasVersionError = true
					break
				}
			}
			if hasVersionError {
				t.Errorf("Expected valid version %q, got version error", version)
			}
		})
	}
}

func TestValidateManifest_InvalidModuleType(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy binary
	binaryPath := filepath.Join(pluginDir, "test-plugin")
	if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	invalidTypes := []string{
		"ingestion",  // lowercase
		"PARSER",     // not a valid type
		"APE_MODULE", // wrong format
	}

	for _, moduleType := range invalidTypes {
		t.Run("type_"+moduleType, func(t *testing.T) {
			manifest := map[string]interface{}{
				"id":      "test-plugin",
				"name":    "Test Plugin",
				"version": "1.0.0",
				"type":    moduleType,
				"binary":  "test-plugin",
			}

			manifestData, _ := json.Marshal(manifest)
			manifestPath := filepath.Join(pluginDir, "manifest.json")
			if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
				t.Fatal(err)
			}

			result, err := ValidateManifest(pluginDir)
			if err != nil {
				t.Fatalf("ValidateManifest returned error: %v", err)
			}

			if result.Valid {
				t.Errorf("Expected validation to fail for type %q", moduleType)
			}

			found := false
			for _, e := range result.Errors {
				if len(e) > 19 && e[:19] == "invalid module type" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected 'invalid module type' error, got: %v", result.Errors)
			}
		})
	}
}

func TestValidateManifest_ValidModuleTypes(t *testing.T) {
	tmpDir := t.TempDir()

	validTypes := []string{"INGESTION", "REASONING", "APE"}

	for _, moduleType := range validTypes {
		t.Run("type_"+moduleType, func(t *testing.T) {
			pluginDir := filepath.Join(tmpDir, "test-"+moduleType)
			if err := os.MkdirAll(pluginDir, 0755); err != nil {
				t.Fatal(err)
			}

			// Create dummy binary
			binaryPath := filepath.Join(pluginDir, "test-plugin")
			if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
				t.Fatal(err)
			}

			manifest := map[string]interface{}{
				"id":      "test-plugin",
				"name":    "Test Plugin",
				"version": "1.0.0",
				"type":    moduleType,
				"binary":  "test-plugin",
			}

			manifestData, _ := json.Marshal(manifest)
			manifestPath := filepath.Join(pluginDir, "manifest.json")
			if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
				t.Fatal(err)
			}

			result, err := ValidateManifest(pluginDir)
			if err != nil {
				t.Fatalf("ValidateManifest returned error: %v", err)
			}

			if !result.Valid {
				t.Errorf("Expected valid manifest for type %q, got errors: %v", moduleType, result.Errors)
			}
		})
	}
}

func TestValidateManifest_BinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Don't create the binary file

	manifest := map[string]interface{}{
		"id":      "test-plugin",
		"name":    "Test Plugin",
		"version": "1.0.0",
		"type":    "INGESTION",
		"binary":  "nonexistent-binary",
	}

	manifestData, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateManifest(pluginDir)
	if err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when binary not found")
	}

	found := false
	for _, e := range result.Errors {
		if len(e) > 24 && e[:24] == "entrypoint binary not fo" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'entrypoint binary not found' error, got: %v", result.Errors)
	}
}

func TestValidateManifest_UnknownFields(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy binary
	binaryPath := filepath.Join(pluginDir, "test-plugin")
	if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]interface{}{
		"id":           "test-plugin",
		"name":         "Test Plugin",
		"version":      "1.0.0",
		"type":         "INGESTION",
		"binary":       "test-plugin",
		"unknown_field": "should warn",
		"another_unknown": 123,
	}

	manifestData, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateManifest(pluginDir)
	if err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}

	// Should be valid but with warnings
	if !result.Valid {
		t.Errorf("Expected valid manifest with warnings, got errors: %v", result.Errors)
	}

	if len(result.Warnings) < 2 {
		t.Errorf("Expected at least 2 warnings for unknown fields, got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestValidateManifest_CapabilityMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy binary
	binaryPath := filepath.Join(pluginDir, "test-plugin")
	if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	// INGESTION module with APE-specific capabilities
	manifest := map[string]interface{}{
		"id":      "test-plugin",
		"name":    "Test Plugin",
		"version": "1.0.0",
		"type":    "INGESTION",
		"binary":  "test-plugin",
		"capabilities": map[string]interface{}{
			"event_triggers":    []string{"session_end"}, // APE capability
			"pattern_detectors": []string{"keyword"},     // REASONING capability
		},
	}

	manifestData, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateManifest(pluginDir)
	if err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}

	// Should be valid but with warnings about capability mismatch
	if !result.Valid {
		t.Errorf("Expected valid manifest with warnings, got errors: %v", result.Errors)
	}

	if len(result.Warnings) < 2 {
		t.Errorf("Expected at least 2 warnings for capability mismatch, got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestValidateManifest_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateManifest(pluginDir)
	if err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail for invalid JSON")
	}

	found := false
	for _, e := range result.Errors {
		if len(e) > 12 && e[:12] == "invalid JSON" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'invalid JSON' error, got: %v", result.Errors)
	}
}

func TestValidateManifest_ManifestNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "empty-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateManifest(pluginDir)
	if err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when manifest.json not found")
	}

	found := false
	for _, e := range result.Errors {
		if len(e) > 18 && e[:18] == "failed to read man" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'failed to read manifest' error, got: %v", result.Errors)
	}
}

func TestValidateManifest_DirectManifestPath(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy binary
	binaryPath := filepath.Join(pluginDir, "test-plugin")
	if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]interface{}{
		"id":      "test-plugin",
		"name":    "Test Plugin",
		"version": "1.0.0",
		"type":    "INGESTION",
		"binary":  "test-plugin",
	}

	manifestData, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	// Test passing the manifest file path directly instead of directory
	result, err := ValidateManifest(manifestPath)
	if err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected valid manifest when passing file path directly, got errors: %v", result.Errors)
	}
}

func TestValidateManifest_HealthCheckIntervalWarnings(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy binary
	binaryPath := filepath.Join(pluginDir, "test-plugin")
	if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name             string
		intervalMs       int
		expectWarning    bool
		warningSubstring string
	}{
		{"very_short", 100, true, "less than 1000ms"},
		{"short", 500, true, "less than 1000ms"},
		{"normal", 5000, false, ""},
		{"negative", -100, true, "should be positive"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := map[string]interface{}{
				"id":                       "test-plugin",
				"name":                     "Test Plugin",
				"version":                  "1.0.0",
				"type":                     "INGESTION",
				"binary":                   "test-plugin",
				"health_check_interval_ms": tc.intervalMs,
			}

			manifestData, _ := json.Marshal(manifest)
			manifestPath := filepath.Join(pluginDir, "manifest.json")
			if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
				t.Fatal(err)
			}

			result, err := ValidateManifest(pluginDir)
			if err != nil {
				t.Fatalf("ValidateManifest returned error: %v", err)
			}

			hasWarning := false
			for _, w := range result.Warnings {
				if len(w) > len(tc.warningSubstring) {
					for i := 0; i <= len(w)-len(tc.warningSubstring); i++ {
						if w[i:i+len(tc.warningSubstring)] == tc.warningSubstring {
							hasWarning = true
							break
						}
					}
				}
			}

			if tc.expectWarning && !hasWarning {
				t.Errorf("Expected warning containing %q, got warnings: %v", tc.warningSubstring, result.Warnings)
			}
			if !tc.expectWarning && hasWarning {
				t.Errorf("Did not expect warning containing %q, but got warnings: %v", tc.warningSubstring, result.Warnings)
			}
		})
	}
}

func TestValidateHealthCheck_SocketNotFound(t *testing.T) {
	result, err := ValidateHealthCheck("/tmp/nonexistent-socket-12345.sock")
	if err != nil {
		t.Fatalf("ValidateHealthCheck returned error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when socket not found")
	}

	found := false
	for _, e := range result.Errors {
		if len(e) > 10 && e[:10] == "socket not" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'socket not found' error, got: %v", result.Errors)
	}
}

func TestValidateLifecycle_SocketNotFound(t *testing.T) {
	result, err := ValidateLifecycle("/tmp/nonexistent-socket-12345.sock")
	if err != nil {
		t.Fatalf("ValidateLifecycle returned error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when socket not found")
	}

	found := false
	for _, e := range result.Errors {
		if len(e) > 10 && e[:10] == "socket not" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'socket not found' error, got: %v", result.Errors)
	}
}

func TestValidateProtoCompliance_InvalidModuleType(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "test-binary")
	if err := os.WriteFile(binaryPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateProtoCompliance(binaryPath, "INVALID_TYPE")
	if err != nil {
		t.Fatalf("ValidateProtoCompliance returned error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail for invalid module type")
	}

	found := false
	for _, e := range result.Errors {
		if len(e) > 19 && e[:19] == "invalid module type" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'invalid module type' error, got: %v", result.Errors)
	}
}

func TestValidateProtoCompliance_BinaryNotFound(t *testing.T) {
	result, err := ValidateProtoCompliance("/tmp/nonexistent-binary-12345", "INGESTION")
	if err != nil {
		t.Fatalf("ValidateProtoCompliance returned error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when binary not found")
	}

	found := false
	for _, e := range result.Errors {
		if len(e) > 13 && e[:13] == "binary not fo" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'binary not found' error, got: %v", result.Errors)
	}
}

func TestValidationResult_Empty(t *testing.T) {
	result := ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	if !result.Valid {
		t.Error("Expected empty ValidationResult to be valid")
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(result.Errors))
	}

	if len(result.Warnings) != 0 {
		t.Errorf("Expected 0 warnings, got %d", len(result.Warnings))
	}
}

func TestManifestValidation_WithManifest(t *testing.T) {
	result := &ManifestValidation{
		ValidationResult: ValidationResult{
			Valid:    true,
			Errors:   []string{},
			Warnings: []string{},
		},
		Manifest: &Manifest{
			ID:      "test",
			Name:    "Test",
			Version: "1.0.0",
			Type:    "INGESTION",
			Binary:  "test-binary",
		},
	}

	if result.Manifest == nil {
		t.Error("Expected manifest to be set")
	}

	if result.Manifest.ID != "test" {
		t.Errorf("Expected ID 'test', got %q", result.Manifest.ID)
	}
}

func TestProtoValidation_Services(t *testing.T) {
	result := &ProtoValidation{
		ValidationResult: ValidationResult{
			Valid: true,
		},
		ServicesRegistered: []string{"ModuleLifecycle", "IngestionModule"},
		RPCsImplemented:    []string{"Handshake", "HealthCheck", "Parse"},
		RPCsMissing:        []string{},
	}

	if len(result.ServicesRegistered) != 2 {
		t.Errorf("Expected 2 services registered, got %d", len(result.ServicesRegistered))
	}

	if len(result.RPCsImplemented) != 3 {
		t.Errorf("Expected 3 RPCs implemented, got %d", len(result.RPCsImplemented))
	}
}

func TestHealthValidation_Metrics(t *testing.T) {
	result := &HealthValidation{
		ValidationResult: ValidationResult{
			Valid: true,
		},
		Healthy: true,
		Status:  "ok",
		Metrics: map[string]string{
			"uptime":   "1h30m",
			"requests": "1234",
		},
		ResponseTimeMs: 50,
	}

	if !result.Healthy {
		t.Error("Expected healthy to be true")
	}

	if result.Metrics["uptime"] != "1h30m" {
		t.Errorf("Expected uptime '1h30m', got %q", result.Metrics["uptime"])
	}

	if result.ResponseTimeMs != 50 {
		t.Errorf("Expected response time 50ms, got %d", result.ResponseTimeMs)
	}
}

func TestLifecycleValidation_AllSteps(t *testing.T) {
	result := &LifecycleValidation{
		ValidationResult: ValidationResult{
			Valid: true,
		},
		HandshakeOK:      true,
		HealthOK:         true,
		ShutdownOK:       true,
		ModuleID:         "test-module",
		ModuleVersion:    "1.0.0",
		TotalDurationMs:  250,
	}

	if !result.HandshakeOK {
		t.Error("Expected HandshakeOK to be true")
	}

	if !result.HealthOK {
		t.Error("Expected HealthOK to be true")
	}

	if !result.ShutdownOK {
		t.Error("Expected ShutdownOK to be true")
	}

	if result.ModuleID != "test-module" {
		t.Errorf("Expected ModuleID 'test-module', got %q", result.ModuleID)
	}
}

func TestPluginValidation_AllResults(t *testing.T) {
	result := &PluginValidation{
		PluginPath: "/path/to/plugin",
		Valid:      true,
		Manifest: &ManifestValidation{
			ValidationResult: ValidationResult{Valid: true},
			Manifest:         &Manifest{ID: "test"},
		},
		Proto: &ProtoValidation{
			ValidationResult: ValidationResult{Valid: true},
		},
		Health: &HealthValidation{
			ValidationResult: ValidationResult{Valid: true},
			Healthy:          true,
		},
		Lifecycle: &LifecycleValidation{
			ValidationResult: ValidationResult{Valid: true},
		},
	}

	if !result.Valid {
		t.Error("Expected plugin validation to be valid")
	}

	if result.Manifest == nil {
		t.Error("Expected manifest validation to be set")
	}

	if result.Proto == nil {
		t.Error("Expected proto validation to be set")
	}

	if result.Health == nil {
		t.Error("Expected health validation to be set")
	}

	if result.Lifecycle == nil {
		t.Error("Expected lifecycle validation to be set")
	}
}
