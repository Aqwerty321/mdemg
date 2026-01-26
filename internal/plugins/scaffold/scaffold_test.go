package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToModuleID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"My Custom Plugin", "my-custom-plugin"},
		{"my-plugin", "my-plugin"},
		{"MY_PLUGIN", "my-plugin"},
		{"Test Plugin 123", "test-plugin-123"},
		{"Plugin--With--Multiple--Dashes", "plugin-with-multiple-dashes"},
		{"--leading-trailing--", "leading-trailing"},
		{"  spaces  around  ", "spaces-around"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ToModuleID(tt.input)
			if result != tt.expected {
				t.Errorf("ToModuleID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToStructName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my-custom-plugin", "MyCustomPlugin"},
		{"MY_PLUGIN", "MyPlugin"},
		{"simple", "Simple"},
		{"ALLCAPS", "Allcaps"},
		{"with spaces", "WithSpaces"},
		{"", "Plugin"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ToStructName(tt.input)
			if result != tt.expected {
				t.Errorf("ToStructName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		checkFiles func(t *testing.T, pluginDir string)
	}{
		{
			name: "ingestion plugin",
			config: Config{
				Name:    "Test Ingestion",
				Type:    ModuleTypeIngestion,
				Version: "1.0.0",
			},
			checkFiles: func(t *testing.T, pluginDir string) {
				// Check manifest
				manifestData, err := os.ReadFile(filepath.Join(pluginDir, "manifest.json"))
				if err != nil {
					t.Fatalf("failed to read manifest: %v", err)
				}

				var manifest map[string]any
				if err := json.Unmarshal(manifestData, &manifest); err != nil {
					t.Fatalf("failed to parse manifest: %v", err)
				}

				if manifest["type"] != "INGESTION" {
					t.Errorf("expected type INGESTION, got %v", manifest["type"])
				}

				caps, ok := manifest["capabilities"].(map[string]any)
				if !ok {
					t.Fatalf("expected capabilities object")
				}

				if _, ok := caps["ingestion_sources"]; !ok {
					t.Error("expected ingestion_sources in capabilities")
				}

				// Check handler.go contains ingestion code
				handlerData, err := os.ReadFile(filepath.Join(pluginDir, "handler.go"))
				if err != nil {
					t.Fatalf("failed to read handler.go: %v", err)
				}

				if !strings.Contains(string(handlerData), "IngestionModuleServer") {
					t.Error("expected IngestionModuleServer in handler.go")
				}
				if !strings.Contains(string(handlerData), "Matches") {
					t.Error("expected Matches function in handler.go")
				}
				if !strings.Contains(string(handlerData), "Parse") {
					t.Error("expected Parse function in handler.go")
				}
			},
		},
		{
			name: "reasoning plugin",
			config: Config{
				Name:    "Test Reasoning",
				Type:    ModuleTypeReasoning,
				Version: "2.0.0",
			},
			checkFiles: func(t *testing.T, pluginDir string) {
				// Check manifest
				manifestData, err := os.ReadFile(filepath.Join(pluginDir, "manifest.json"))
				if err != nil {
					t.Fatalf("failed to read manifest: %v", err)
				}

				var manifest map[string]any
				if err := json.Unmarshal(manifestData, &manifest); err != nil {
					t.Fatalf("failed to parse manifest: %v", err)
				}

				if manifest["type"] != "REASONING" {
					t.Errorf("expected type REASONING, got %v", manifest["type"])
				}

				caps, ok := manifest["capabilities"].(map[string]any)
				if !ok {
					t.Fatalf("expected capabilities object")
				}

				if _, ok := caps["pattern_detectors"]; !ok {
					t.Error("expected pattern_detectors in capabilities")
				}

				// Check handler.go contains reasoning code
				handlerData, err := os.ReadFile(filepath.Join(pluginDir, "handler.go"))
				if err != nil {
					t.Fatalf("failed to read handler.go: %v", err)
				}

				if !strings.Contains(string(handlerData), "ReasoningModuleServer") {
					t.Error("expected ReasoningModuleServer in handler.go")
				}
				if !strings.Contains(string(handlerData), "Process") {
					t.Error("expected Process function in handler.go")
				}
			},
		},
		{
			name: "APE plugin",
			config: Config{
				Name:    "Test APE",
				Type:    ModuleTypeAPE,
				Version: "3.0.0",
			},
			checkFiles: func(t *testing.T, pluginDir string) {
				// Check manifest
				manifestData, err := os.ReadFile(filepath.Join(pluginDir, "manifest.json"))
				if err != nil {
					t.Fatalf("failed to read manifest: %v", err)
				}

				var manifest map[string]any
				if err := json.Unmarshal(manifestData, &manifest); err != nil {
					t.Fatalf("failed to parse manifest: %v", err)
				}

				if manifest["type"] != "APE" {
					t.Errorf("expected type APE, got %v", manifest["type"])
				}

				caps, ok := manifest["capabilities"].(map[string]any)
				if !ok {
					t.Fatalf("expected capabilities object")
				}

				if _, ok := caps["event_triggers"]; !ok {
					t.Error("expected event_triggers in capabilities")
				}

				// Check handler.go contains APE code
				handlerData, err := os.ReadFile(filepath.Join(pluginDir, "handler.go"))
				if err != nil {
					t.Fatalf("failed to read handler.go: %v", err)
				}

				if !strings.Contains(string(handlerData), "APEModuleServer") {
					t.Error("expected APEModuleServer in handler.go")
				}
				if !strings.Contains(string(handlerData), "GetSchedule") {
					t.Error("expected GetSchedule function in handler.go")
				}
				if !strings.Contains(string(handlerData), "Execute") {
					t.Error("expected Execute function in handler.go")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir, err := os.MkdirTemp("", "mdemg-scaffold-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			tt.config.OutputDir = tempDir

			result, err := Generate(tt.config)
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}

			// Check result
			expectedID := ToModuleID(tt.config.Name)
			if result.PluginID != expectedID {
				t.Errorf("expected PluginID %q, got %q", expectedID, result.PluginID)
			}

			if len(result.FilesCreated) != 5 {
				t.Errorf("expected 5 files created, got %d", len(result.FilesCreated))
			}

			// Check that all expected files exist
			expectedFiles := []string{"manifest.json", "main.go", "handler.go", "Makefile", "README.md"}
			for _, file := range expectedFiles {
				path := filepath.Join(result.PluginPath, file)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Errorf("expected file %s to exist", file)
				}
			}

			// Run custom checks
			if tt.checkFiles != nil {
				tt.checkFiles(t, result.PluginPath)
			}
		})
	}
}

func TestGenerateDefaultVersion(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mdemg-scaffold-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := Config{
		Name:      "No Version",
		Type:      ModuleTypeIngestion,
		OutputDir: tempDir,
		// Version is empty
	}

	result, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Check manifest has default version
	manifestData, err := os.ReadFile(filepath.Join(result.PluginPath, "manifest.json"))
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if manifest["version"] != "1.0.0" {
		t.Errorf("expected default version 1.0.0, got %v", manifest["version"])
	}
}

func TestGenerateManifest(t *testing.T) {
	cfg := Config{
		Name:     "Test Plugin",
		Type:     ModuleTypeIngestion,
		Version:  "1.0.0",
		ModuleID: "test-plugin",
	}

	data, err := GenerateManifest(cfg)
	if err != nil {
		t.Fatalf("GenerateManifest failed: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if manifest["id"] != "test-plugin" {
		t.Errorf("expected id 'test-plugin', got %v", manifest["id"])
	}
	if manifest["name"] != "Test Plugin" {
		t.Errorf("expected name 'Test Plugin', got %v", manifest["name"])
	}
	if manifest["version"] != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %v", manifest["version"])
	}
	if manifest["type"] != "INGESTION" {
		t.Errorf("expected type 'INGESTION', got %v", manifest["type"])
	}
}

func TestValidModuleTypes(t *testing.T) {
	validTypes := []string{"INGESTION", "REASONING", "APE"}
	for _, mt := range validTypes {
		if !ValidModuleTypes[mt] {
			t.Errorf("expected %s to be a valid module type", mt)
		}
	}

	invalidTypes := []string{"ingestion", "INVALID", "PLUGIN", ""}
	for _, mt := range invalidTypes {
		if ValidModuleTypes[mt] {
			t.Errorf("expected %s to be an invalid module type", mt)
		}
	}
}
