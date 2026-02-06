package unts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanner_ScanManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create manifest directory
	manifestDir := filepath.Join(tmpDir, "docs", "specs")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create manifest file (SHA256 hashes are 64 hex chars)
	manifestContent := `# Test Manifest
# Comment line
abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789  docs/specs/file1.md
0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  docs/specs/file2.md

invalid line without hash
tooshort  docs/specs/invalid.md
`
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.sha256"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	err := scanner.ScanManifest()
	if err != nil {
		t.Fatalf("ScanManifest: %v", err)
	}

	// Check registered files
	files := registry.List("manifest", "")
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestScanner_ScanManifest_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	// Should not error when manifest doesn't exist
	err := scanner.ScanManifest()
	if err != nil {
		t.Fatalf("ScanManifest (no file): %v", err)
	}
}

func TestScanner_ScanUDTSSpecs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create UDTS specs directory
	specsDir := filepath.Join(tmpDir, "docs", "api", "api-spec", "udts", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create UDTS spec with proto_sha256 (64 hex chars)
	spec1 := `{
  "udts_version": "1.0.0",
  "service": "mdemg.devspace.v1.DevSpace",
  "config": {
    "proto_sha256": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
  }
}`
	if err := os.WriteFile(filepath.Join(specsDir, "devspace.udts.json"), []byte(spec1), 0644); err != nil {
		t.Fatal(err)
	}

	// Create UDTS spec without proto_sha256
	spec2 := `{
  "udts_version": "1.0.0",
  "service": "mdemg.test.v1.Test",
  "config": {}
}`
	if err := os.WriteFile(filepath.Join(specsDir, "test.udts.json"), []byte(spec2), 0644); err != nil {
		t.Fatal(err)
	}

	// Create non-UDTS file (should be ignored)
	if err := os.WriteFile(filepath.Join(specsDir, "readme.md"), []byte("# README"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create invalid JSON (should be skipped with warning)
	if err := os.WriteFile(filepath.Join(specsDir, "invalid.udts.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	err := scanner.ScanUDTSSpecs()
	if err != nil {
		t.Fatalf("ScanUDTSSpecs: %v", err)
	}

	// Check registered files
	files := registry.List("udts", "")
	if len(files) != 1 {
		t.Errorf("expected 1 file from UDTS, got %d", len(files))
	}
}

func TestScanner_ScanUDTSSpecs_NoDir(t *testing.T) {
	tmpDir := t.TempDir()

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	// Should not error when specs dir doesn't exist
	err := scanner.ScanUDTSSpecs()
	if err != nil {
		t.Fatalf("ScanUDTSSpecs (no dir): %v", err)
	}
}

func TestScanner_ScanAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Create manifest
	manifestDir := filepath.Join(tmpDir, "docs", "specs")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestContent := `abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789  docs/specs/file1.md`
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.sha256"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create UDTS specs directory
	specsDir := filepath.Join(tmpDir, "docs", "api", "api-spec", "udts", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := `{
  "udts_version": "1.0.0",
  "service": "mdemg.transfer.v1.SpaceTransfer",
  "config": {
    "proto_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  }
}`
	if err := os.WriteFile(filepath.Join(specsDir, "transfer.udts.json"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	err := scanner.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}

	// Check both manifest and UDTS files
	allFiles := registry.List("", "")
	if len(allFiles) < 2 {
		t.Errorf("expected at least 2 files, got %d", len(allFiles))
	}
}

func TestDeriveProtoPath_AllCases(t *testing.T) {
	tests := []struct {
		service  string
		expected string
	}{
		{"mdemg.devspace.v1.DevSpace", "api/proto/devspace.proto"},
		{"mdemg.transfer.v1.SpaceTransfer", "api/proto/space-transfer.proto"},
		{"mdemg.unts.v1.HashVerification", "api/proto/unts.proto"},
		{"mdemg.module.v1.Module", "api/proto/mdemg-module.proto"},
		{"mdemg.custom.v1.Custom", "api/proto/custom.proto"},
		{"", ""},
		{"invalid", ""},
		{"one.two", ""},
		{"one.two.three", ""},
	}

	for _, tc := range tests {
		got := deriveProtoPath(tc.service)
		if got != tc.expected {
			t.Errorf("deriveProtoPath(%q): expected %q, got %q", tc.service, tc.expected, got)
		}
	}
}

func TestScanner_ScanAll_ManifestError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create UDTS specs but no manifest - ScanAll should still work
	specsDir := filepath.Join(tmpDir, "docs", "api", "api-spec", "udts", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	err := scanner.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll should not fail when manifest is missing: %v", err)
	}
}

func TestScanner_ScanManifest_ReadError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create manifest directory but make it unreadable
	manifestDir := filepath.Join(tmpDir, "docs", "specs")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a directory where manifest file should be (can't read as file)
	if err := os.Mkdir(filepath.Join(manifestDir, "manifest.sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	err := scanner.ScanManifest()
	if err == nil {
		t.Error("expected error when manifest is a directory")
	}
}

func TestScanner_ScanUDTSSpecs_EmptyProtoSHA(t *testing.T) {
	tmpDir := t.TempDir()

	specsDir := filepath.Join(tmpDir, "docs", "api", "api-spec", "udts", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create spec with empty service (no proto path derivable)
	spec := `{
  "udts_version": "1.0.0",
  "service": "",
  "config": {
    "proto_sha256": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
  }
}`
	if err := os.WriteFile(filepath.Join(specsDir, "empty_service.udts.json"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	err := scanner.ScanUDTSSpecs()
	if err != nil {
		t.Fatalf("ScanUDTSSpecs: %v", err)
	}

	// Should have 0 files (empty service = no proto path)
	files := registry.List("udts", "")
	if len(files) != 0 {
		t.Errorf("expected 0 files for empty service, got %d", len(files))
	}
}

func TestScanner_ScanAll_UDTSError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid manifest
	manifestDir := filepath.Join(tmpDir, "docs", "specs")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.sha256"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Create UDTS specs directory as a file (will cause error when trying to read as directory)
	udtsDir := filepath.Join(tmpDir, "docs", "api", "api-spec", "udts")
	if err := os.MkdirAll(udtsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create specs as a file instead of directory
	if err := os.WriteFile(filepath.Join(udtsDir, "specs"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	err := scanner.ScanAll()
	if err == nil {
		t.Error("expected error when specs is a file")
	}
}

func TestScanner_ScanUDTSSpecs_ReadError(t *testing.T) {
	tmpDir := t.TempDir()

	specsDir := filepath.Join(tmpDir, "docs", "api", "api-spec", "udts", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a spec file that's actually a directory (will fail to read)
	if err := os.Mkdir(filepath.Join(specsDir, "broken.udts.json"), 0755); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tmpDir)
	scanner := NewScanner(registry, tmpDir)

	// Should not error, but should warn
	err := scanner.ScanUDTSSpecs()
	if err != nil {
		t.Fatalf("ScanUDTSSpecs should not fail on single file error: %v", err)
	}
}
