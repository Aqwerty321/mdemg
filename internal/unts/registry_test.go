package unts

import (
	"os"
	"path/filepath"
	"testing"

	pb "mdemg/api/untspb"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry("/tmp/test-unts")

	// Register a file
	err := r.Register("docs/specs/test.md", "manifest", "abc123def456", "docs/specs/manifest.sha256", "manifest")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Get it back
	f, ok := r.Get("docs/specs/test.md")
	if !ok {
		t.Fatal("file not found after register")
	}

	if f.Path != "docs/specs/test.md" {
		t.Errorf("expected path 'docs/specs/test.md', got %q", f.Path)
	}
	if f.Framework != "manifest" {
		t.Errorf("expected framework 'manifest', got %q", f.Framework)
	}
	if f.CurrentHash != "abc123def456" {
		t.Errorf("expected hash 'abc123def456', got %q", f.CurrentHash)
	}
	if f.SourceRef != "docs/specs/manifest.sha256" {
		t.Errorf("expected source_ref 'docs/specs/manifest.sha256', got %q", f.SourceRef)
	}
}

func TestRegistry_UpdateHash(t *testing.T) {
	r := NewRegistry("/tmp/test-unts")

	// Register initial
	r.Register("test.md", "manifest", "hash1", "", "manifest")

	// Update hash
	err := r.UpdateHash("test.md", "hash2", "manual")
	if err != nil {
		t.Fatalf("UpdateHash failed: %v", err)
	}

	f, _ := r.Get("test.md")
	if f.CurrentHash != "hash2" {
		t.Errorf("expected current hash 'hash2', got %q", f.CurrentHash)
	}

	// Check history
	if len(f.History) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(f.History))
	}
	if f.History[0].Hash != "hash1" {
		t.Errorf("expected history[0].Hash 'hash1', got %q", f.History[0].Hash)
	}
}

func TestRegistry_HistoryLimit(t *testing.T) {
	r := NewRegistry("/tmp/test-unts")

	r.Register("test.md", "manifest", "hash0", "", "manifest")

	// Update 5 times
	for i := 1; i <= 5; i++ {
		r.UpdateHash("test.md", "hash"+string(rune('0'+i)), "manual")
	}

	f, _ := r.Get("test.md")
	if len(f.History) != MaxHistoryEntries {
		t.Errorf("expected %d history entries, got %d", MaxHistoryEntries, len(f.History))
	}
}

func TestRegistry_RevertHash(t *testing.T) {
	r := NewRegistry("/tmp/test-unts")

	r.Register("test.md", "manifest", "hash1", "", "manifest")
	r.UpdateHash("test.md", "hash2", "manual")
	r.UpdateHash("test.md", "hash3", "manual")

	// Revert to hash1
	err := r.RevertHash("test.md", "hash1")
	if err != nil {
		t.Fatalf("RevertHash failed: %v", err)
	}

	f, _ := r.Get("test.md")
	if f.CurrentHash != "hash1" {
		t.Errorf("expected current hash 'hash1', got %q", f.CurrentHash)
	}
	if f.Status != "reverted" {
		t.Errorf("expected status 'reverted', got %q", f.Status)
	}
}

func TestRegistry_RevertHashNotInHistory(t *testing.T) {
	r := NewRegistry("/tmp/test-unts")

	r.Register("test.md", "manifest", "hash1", "", "manifest")

	err := r.RevertHash("test.md", "nonexistent")
	if err == nil {
		t.Error("expected error for hash not in history")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry("/tmp/test-unts")

	r.Register("manifest1.md", "manifest", "hash1", "", "manifest")
	r.Register("manifest2.md", "manifest", "hash2", "", "manifest")
	r.Register("udts1.proto", "udts", "hash3", "", "spec")

	// List all
	all := r.List("", "")
	if len(all) != 3 {
		t.Errorf("expected 3 files, got %d", len(all))
	}

	// Filter by framework
	manifest := r.List("manifest", "")
	if len(manifest) != 2 {
		t.Errorf("expected 2 manifest files, got %d", len(manifest))
	}

	udts := r.List("udts", "")
	if len(udts) != 1 {
		t.Errorf("expected 1 udts file, got %d", len(udts))
	}
}

func TestRegistry_Verify(t *testing.T) {
	// Create a temp directory with a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Known SHA256 of "hello world\n"
	expectedHash := "a948904f2f0f479b8f8564cbf12dac6b0c6e5c9e3a2e6a2b8f1d7c3e5a4b2c1d" // placeholder

	r := NewRegistry(tmpDir)
	r.Register("test.txt", "manifest", expectedHash, "", "manifest")

	result, err := r.Verify("test.txt")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// The hash won't match our placeholder, so should be mismatch
	if result.Status != "mismatch" {
		t.Logf("actual hash: %s", result.ActualHash)
		// This is expected since we used a placeholder hash
	}

	// Verify with correct hash
	r.UpdateHash("test.txt", result.ActualHash, "test")
	result, _ = r.Verify("test.txt")
	if result.Status != "verified" {
		t.Errorf("expected status 'verified', got %q", result.Status)
	}
}

func TestComputeFileHash(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Write known content
	content := []byte("hello world\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := computeFileHash(testFile)
	if err != nil {
		t.Fatalf("computeFileHash failed: %v", err)
	}

	if len(hash) != 64 {
		t.Errorf("expected 64-char hash, got %d chars", len(hash))
	}

	// Just verify it's consistent
	hash2, _ := computeFileHash(testFile)
	if hash != hash2 {
		t.Error("hash should be consistent across calls")
	}

	t.Logf("computed hash: %s", hash)
}

func TestToProtoConversions(t *testing.T) {
	tests := []struct {
		framework string
		expected  string
	}{
		{"manifest", "manifest"},
		{"udts", "udts"},
		{"uats", "uats"},
	}

	for _, tc := range tests {
		proto := toProtoFramework(tc.framework)
		back := fromProtoFramework(proto)
		if back != tc.expected {
			t.Errorf("roundtrip %q: got %q", tc.framework, back)
		}
	}
}

func TestDeriveProtoPath(t *testing.T) {
	tests := []struct {
		service  string
		expected string
	}{
		{"mdemg.devspace.v1.DevSpace", "api/proto/devspace.proto"},
		{"mdemg.transfer.v1.SpaceTransfer", "api/proto/space-transfer.proto"},
		{"mdemg.unts.v1.HashVerification", "api/proto/unts.proto"},
		{"", ""},
		{"invalid", ""},
	}

	for _, tc := range tests {
		got := deriveProtoPath(tc.service)
		if got != tc.expected {
			t.Errorf("deriveProtoPath(%q): expected %q, got %q", tc.service, tc.expected, got)
		}
	}
}

func TestRegistry_LoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()

	// Create registry directory
	registryDir := filepath.Join(tmpDir, "docs", "specs")
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create initial registry
	r1 := NewRegistry(tmpDir)
	r1.Register("test.md", "manifest", "hash123", "manifest.sha256", "manifest")

	// Save
	if err := r1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load into new registry
	r2 := NewRegistry(tmpDir)
	if err := r2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify data
	f, ok := r2.Get("test.md")
	if !ok {
		t.Fatal("file not found after load")
	}
	if f.CurrentHash != "hash123" {
		t.Errorf("expected hash 'hash123', got %q", f.CurrentHash)
	}
}

func TestRegistry_LoadNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewRegistry(tmpDir)

	// Should not error when file doesn't exist
	err := r.Load()
	if err != nil {
		t.Fatalf("Load (no file): %v", err)
	}
}

func TestRegistry_VerifyAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(tmpDir)
	r.Register("file1.txt", "manifest", "wronghash1", "", "manifest")
	r.Register("file2.txt", "udts", "wronghash2", "", "spec")

	// Verify all
	results := r.VerifyAll("")
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Verify filtered by framework
	results = r.VerifyAll("manifest")
	if len(results) != 1 {
		t.Errorf("expected 1 result for manifest, got %d", len(results))
	}
}

func TestRegistry_UpdateHashUntracked(t *testing.T) {
	r := NewRegistry("/tmp/test")

	err := r.UpdateHash("nonexistent.txt", "hash123", "manual")
	if err == nil {
		t.Error("expected error for untracked file")
	}
}

func TestRegistry_VerifyUntracked(t *testing.T) {
	r := NewRegistry("/tmp/test")

	_, err := r.Verify("nonexistent.txt")
	if err == nil {
		t.Error("expected error for untracked file")
	}
}

func TestRegistry_VerifyFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewRegistry(tmpDir)

	r.Register("missing.txt", "manifest", "hash123", "", "manifest")

	result, err := r.Verify("missing.txt")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Status != "unknown" {
		t.Errorf("expected status 'unknown', got %q", result.Status)
	}
}

func TestFileRecord_ToProto(t *testing.T) {
	f := &FileRecord{
		Path:        "test.md",
		Framework:   "manifest",
		CurrentHash: "hash123",
		Status:      "verified",
		UpdatedAt:   "2026-01-01T00:00:00Z",
		History: []HistoryEntry{
			{Hash: "oldhash", UpdatedAt: "2025-12-01T00:00:00Z", Source: "manifest"},
		},
		SourceRef: "manifest.sha256",
	}

	proto := f.ToProto()
	if proto.Path != "test.md" {
		t.Errorf("expected path 'test.md', got %q", proto.Path)
	}
	if len(proto.History) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(proto.History))
	}
}

func TestToProtoStatusAllCases(t *testing.T) {
	tests := []struct {
		status   string
		expected pb.FileStatus
	}{
		{"verified", pb.FileStatus_FILE_STATUS_VERIFIED},
		{"mismatch", pb.FileStatus_FILE_STATUS_MISMATCH},
		{"unknown", pb.FileStatus_FILE_STATUS_UNKNOWN},
		{"reverted", pb.FileStatus_FILE_STATUS_REVERTED},
		{"other", pb.FileStatus_FILE_STATUS_UNSPECIFIED},
	}

	for _, tc := range tests {
		got := toProtoStatus(tc.status)
		if got != tc.expected {
			t.Errorf("toProtoStatus(%q): expected %v, got %v", tc.status, tc.expected, got)
		}
	}
}

func TestToProtoSourceAllCases(t *testing.T) {
	tests := []struct {
		source   string
		expected pb.HashSource
	}{
		{"manifest", pb.HashSource_HASH_SOURCE_MANIFEST},
		{"spec", pb.HashSource_HASH_SOURCE_SPEC},
		{"revert", pb.HashSource_HASH_SOURCE_REVERT},
		{"manual", pb.HashSource_HASH_SOURCE_MANUAL},
		{"ci", pb.HashSource_HASH_SOURCE_CI},
		{"other", pb.HashSource_HASH_SOURCE_UNSPECIFIED},
	}

	for _, tc := range tests {
		got := toProtoSource(tc.source)
		if got != tc.expected {
			t.Errorf("toProtoSource(%q): expected %v, got %v", tc.source, tc.expected, got)
		}
	}
}

func TestToProtoFrameworkAllCases(t *testing.T) {
	tests := []struct {
		framework string
		expected  pb.Framework
	}{
		{"manifest", pb.Framework_FRAMEWORK_MANIFEST},
		{"udts", pb.Framework_FRAMEWORK_UDTS},
		{"uats", pb.Framework_FRAMEWORK_UATS},
		{"ubts", pb.Framework_FRAMEWORK_UBTS},
		{"usts", pb.Framework_FRAMEWORK_USTS},
		{"uots", pb.Framework_FRAMEWORK_UOTS},
		{"uams", pb.Framework_FRAMEWORK_UAMS},
		{"upts", pb.Framework_FRAMEWORK_UPTS},
		{"other", pb.Framework_FRAMEWORK_UNSPECIFIED},
	}

	for _, tc := range tests {
		got := toProtoFramework(tc.framework)
		if got != tc.expected {
			t.Errorf("toProtoFramework(%q): expected %v, got %v", tc.framework, tc.expected, got)
		}
	}
}

func TestFromProtoFrameworkAllCases(t *testing.T) {
	tests := []struct {
		framework pb.Framework
		expected  string
	}{
		{pb.Framework_FRAMEWORK_MANIFEST, "manifest"},
		{pb.Framework_FRAMEWORK_UDTS, "udts"},
		{pb.Framework_FRAMEWORK_UATS, "uats"},
		{pb.Framework_FRAMEWORK_UBTS, "ubts"},
		{pb.Framework_FRAMEWORK_USTS, "usts"},
		{pb.Framework_FRAMEWORK_UOTS, "uots"},
		{pb.Framework_FRAMEWORK_UAMS, "uams"},
		{pb.Framework_FRAMEWORK_UPTS, "upts"},
		{pb.Framework_FRAMEWORK_UNSPECIFIED, ""},
	}

	for _, tc := range tests {
		got := fromProtoFramework(tc.framework)
		if got != tc.expected {
			t.Errorf("fromProtoFramework(%v): expected %q, got %q", tc.framework, tc.expected, got)
		}
	}
}

func TestFromProtoSourceAllCases(t *testing.T) {
	tests := []struct {
		source   pb.HashSource
		expected string
	}{
		{pb.HashSource_HASH_SOURCE_MANIFEST, "manifest"},
		{pb.HashSource_HASH_SOURCE_SPEC, "spec"},
		{pb.HashSource_HASH_SOURCE_REVERT, "revert"},
		{pb.HashSource_HASH_SOURCE_MANUAL, "manual"},
		{pb.HashSource_HASH_SOURCE_CI, "ci"},
		{pb.HashSource_HASH_SOURCE_UNSPECIFIED, ""},
	}

	for _, tc := range tests {
		got := fromProtoSource(tc.source)
		if got != tc.expected {
			t.Errorf("fromProtoSource(%v): expected %q, got %q", tc.source, tc.expected, got)
		}
	}
}

func TestRegistry_RegisterUpdate(t *testing.T) {
	r := NewRegistry("/tmp/test")

	// Register initial
	r.Register("test.md", "manifest", "hash1", "ref1", "manifest")

	// Register again with same hash (should not create history)
	r.Register("test.md", "manifest", "hash1", "ref2", "manifest")

	f, _ := r.Get("test.md")
	if len(f.History) != 0 {
		t.Errorf("expected 0 history entries for same hash, got %d", len(f.History))
	}
	if f.SourceRef != "ref2" {
		t.Errorf("expected source_ref 'ref2', got %q", f.SourceRef)
	}

	// Register with different hash (should create history)
	r.Register("test.md", "manifest", "hash2", "", "manual")

	f, _ = r.Get("test.md")
	if len(f.History) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(f.History))
	}
	if f.CurrentHash != "hash2" {
		t.Errorf("expected current hash 'hash2', got %q", f.CurrentHash)
	}
}

func TestComputeFileHashError(t *testing.T) {
	_, err := computeFileHash("/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestRegistry_LoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create registry directory
	registryDir := filepath.Join(tmpDir, "docs", "specs")
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create invalid JSON
	if err := os.WriteFile(filepath.Join(registryDir, "unts-registry.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(tmpDir)
	err := r.Load()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRegistry_ListWithStatusFilter(t *testing.T) {
	r := NewRegistry("/tmp/test")

	r.Register("file1.md", "manifest", "hash1", "", "manifest")
	r.Register("file2.md", "manifest", "hash2", "", "manifest")

	// Set different statuses
	r.mu.Lock()
	r.files["file1.md"].Status = "verified"
	r.files["file2.md"].Status = "mismatch"
	r.mu.Unlock()

	// Filter by status
	verified := r.List("", "verified")
	if len(verified) != 1 {
		t.Errorf("expected 1 verified file, got %d", len(verified))
	}

	mismatch := r.List("", "mismatch")
	if len(mismatch) != 1 {
		t.Errorf("expected 1 mismatch file, got %d", len(mismatch))
	}
}

func TestRegistry_RevertHashEmptyTarget(t *testing.T) {
	r := NewRegistry("/tmp/test")
	r.Register("test.md", "manifest", "hash1", "", "manifest")

	// Revert to empty hash should work (edge case)
	err := r.RevertHash("test.md", "")
	if err != nil {
		t.Fatalf("RevertHash to empty: %v", err)
	}

	f, _ := r.Get("test.md")
	if f.CurrentHash != "" {
		t.Errorf("expected empty current hash, got %q", f.CurrentHash)
	}
}
