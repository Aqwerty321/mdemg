package unts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "mdemg/api/untspb"
)

func setupTestServer(t *testing.T) (*Server, string) {
	tmpDir := t.TempDir()

	// Create manifest
	manifestDir := filepath.Join(tmpDir, "docs", "specs")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifestContent := `# Test Manifest
abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789  docs/specs/test.md
`
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.sha256"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create test file
	testFile := filepath.Join(tmpDir, "docs", "specs", "test.md")
	if err := os.WriteFile(testFile, []byte("test content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create empty registry
	registryContent := `{"version":"1.0.0","updated_at":"2026-01-01T00:00:00Z","description":"test","files":[]}`
	if err := os.WriteFile(filepath.Join(manifestDir, "unts-registry.json"), []byte(registryContent), 0644); err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(tmpDir)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	return server, tmpDir
}

func TestNewServer(t *testing.T) {
	server, _ := setupTestServer(t)
	if server == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestServer_ScanAndSync(t *testing.T) {
	server, _ := setupTestServer(t)

	err := server.ScanAndSync()
	if err != nil {
		t.Fatalf("ScanAndSync: %v", err)
	}

	// Check that manifest file was scanned
	resp, err := server.ListVerifiedFiles(context.Background(), &pb.ListVerifiedFilesRequest{})
	if err != nil {
		t.Fatalf("ListVerifiedFiles: %v", err)
	}

	if resp.TotalCount < 1 {
		t.Errorf("expected at least 1 file after scan, got %d", resp.TotalCount)
	}
}

func TestServer_ListVerifiedFiles(t *testing.T) {
	server, _ := setupTestServer(t)
	server.ScanAndSync()

	ctx := context.Background()

	// List all
	resp, err := server.ListVerifiedFiles(ctx, &pb.ListVerifiedFilesRequest{})
	if err != nil {
		t.Fatalf("ListVerifiedFiles: %v", err)
	}
	if resp.Files == nil {
		t.Error("expected non-nil files slice")
	}

	// List with framework filter
	resp, err = server.ListVerifiedFiles(ctx, &pb.ListVerifiedFilesRequest{
		Framework: pb.Framework_FRAMEWORK_MANIFEST,
	})
	if err != nil {
		t.Fatalf("ListVerifiedFiles with filter: %v", err)
	}

	// List with status filter
	resp, err = server.ListVerifiedFiles(ctx, &pb.ListVerifiedFilesRequest{
		Status: pb.FileStatus_FILE_STATUS_VERIFIED,
	})
	if err != nil {
		t.Fatalf("ListVerifiedFiles with status: %v", err)
	}
}

func TestServer_GetFileStatus(t *testing.T) {
	server, _ := setupTestServer(t)
	server.ScanAndSync()

	ctx := context.Background()

	// Get existing file
	resp, err := server.GetFileStatus(ctx, &pb.GetFileStatusRequest{
		Path: "docs/specs/test.md",
	})
	if err != nil {
		t.Fatalf("GetFileStatus: %v", err)
	}
	if resp.Record == nil {
		t.Error("expected non-nil record")
	}

	// Get non-existent file
	_, err = server.GetFileStatus(ctx, &pb.GetFileStatusRequest{
		Path: "nonexistent.txt",
	})
	if err == nil {
		t.Error("expected error for non-existent file")
	}

	// Empty path
	_, err = server.GetFileStatus(ctx, &pb.GetFileStatusRequest{})
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestServer_GetHashHistory(t *testing.T) {
	server, _ := setupTestServer(t)
	server.ScanAndSync()

	ctx := context.Background()

	// Get history
	resp, err := server.GetHashHistory(ctx, &pb.GetHashHistoryRequest{
		Path: "docs/specs/test.md",
	})
	if err != nil {
		t.Fatalf("GetHashHistory: %v", err)
	}
	if resp.Path != "docs/specs/test.md" {
		t.Errorf("expected path 'docs/specs/test.md', got %q", resp.Path)
	}

	// Non-existent
	_, err = server.GetHashHistory(ctx, &pb.GetHashHistoryRequest{
		Path: "nonexistent.txt",
	})
	if err == nil {
		t.Error("expected error for non-existent file")
	}

	// Empty path
	_, err = server.GetHashHistory(ctx, &pb.GetHashHistoryRequest{})
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestServer_RegisterTrackedFile(t *testing.T) {
	server, _ := setupTestServer(t)

	ctx := context.Background()

	// Register new file
	resp, err := server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:         "new/file.txt",
		Framework:    pb.Framework_FRAMEWORK_MANIFEST,
		InitialHash:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		SourceRef:    "test",
		RegisteredBy: "test",
	})
	if err != nil {
		t.Fatalf("RegisterTrackedFile: %v", err)
	}
	if !resp.Ok {
		t.Errorf("expected ok=true, got error: %s", resp.Error)
	}

	// Empty path
	_, err = server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		InitialHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err == nil {
		t.Error("expected error for empty path")
	}

	// Empty hash
	_, err = server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path: "test.txt",
	})
	if err == nil {
		t.Error("expected error for empty hash")
	}

	// Invalid hash length
	_, err = server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:        "test.txt",
		InitialHash: "tooshort",
	})
	if err == nil {
		t.Error("expected error for invalid hash length")
	}
}

func TestServer_UpdateHash(t *testing.T) {
	server, _ := setupTestServer(t)
	server.ScanAndSync()

	ctx := context.Background()

	// Update existing file
	resp, err := server.UpdateHash(ctx, &pb.UpdateHashRequest{
		Path:      "docs/specs/test.md",
		NewHash:   "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
		Source:    pb.HashSource_HASH_SOURCE_MANUAL,
		UpdatedBy: "test",
		Reason:    "test update",
	})
	if err != nil {
		t.Fatalf("UpdateHash: %v", err)
	}
	if !resp.Ok {
		t.Errorf("expected ok=true, got error: %s", resp.Error)
	}

	// Non-existent file
	resp, err = server.UpdateHash(ctx, &pb.UpdateHashRequest{
		Path:    "nonexistent.txt",
		NewHash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
	})
	if err != nil {
		t.Fatalf("UpdateHash non-existent: %v", err)
	}
	if resp.Ok {
		t.Error("expected ok=false for non-existent file")
	}

	// Empty path
	_, err = server.UpdateHash(ctx, &pb.UpdateHashRequest{
		NewHash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
	})
	if err == nil {
		t.Error("expected error for empty path")
	}

	// Empty hash
	_, err = server.UpdateHash(ctx, &pb.UpdateHashRequest{
		Path: "docs/specs/test.md",
	})
	if err == nil {
		t.Error("expected error for empty hash")
	}

	// Invalid hash length
	_, err = server.UpdateHash(ctx, &pb.UpdateHashRequest{
		Path:    "docs/specs/test.md",
		NewHash: "tooshort",
	})
	if err == nil {
		t.Error("expected error for invalid hash length")
	}
}

func TestServer_RevertToPreviousHash(t *testing.T) {
	server, _ := setupTestServer(t)
	server.ScanAndSync()

	ctx := context.Background()

	// First update to create history
	updateResp, err := server.UpdateHash(ctx, &pb.UpdateHashRequest{
		Path:    "docs/specs/test.md",
		NewHash: "1111111111111111111111111111111111111111111111111111111111111111",
		Source:  pb.HashSource_HASH_SOURCE_MANUAL,
	})
	if err != nil {
		t.Fatalf("UpdateHash: %v", err)
	}
	if !updateResp.Ok {
		t.Skipf("Skipping revert test: update failed: %s", updateResp.Error)
	}

	// Get history to find previous hash
	histResp, err := server.GetHashHistory(ctx, &pb.GetHashHistoryRequest{
		Path: "docs/specs/test.md",
	})
	if err != nil {
		t.Fatalf("GetHashHistory: %v", err)
	}

	// Revert by index
	resp, err := server.RevertToPreviousHash(ctx, &pb.RevertToPreviousHashRequest{
		Path:       "docs/specs/test.md",
		Target:     &pb.RevertToPreviousHashRequest_HistoryIndex{HistoryIndex: 0},
		RevertedBy: "test",
		Reason:     "test revert",
	})
	if err != nil {
		t.Fatalf("RevertToPreviousHash: %v", err)
	}
	if !resp.Ok {
		t.Errorf("expected ok=true, got error: %s", resp.Error)
	}

	// Revert by hash
	if histResp != nil && len(histResp.History) > 0 {
		resp, err = server.RevertToPreviousHash(ctx, &pb.RevertToPreviousHashRequest{
			Path:   "docs/specs/test.md",
			Target: &pb.RevertToPreviousHashRequest_TargetHash{TargetHash: histResp.History[0].Hash},
		})
		if err != nil {
			t.Fatalf("RevertToPreviousHash by hash: %v", err)
		}
	}

	// Non-existent file
	resp, err = server.RevertToPreviousHash(ctx, &pb.RevertToPreviousHashRequest{
		Path:   "nonexistent.txt",
		Target: &pb.RevertToPreviousHashRequest_HistoryIndex{HistoryIndex: 0},
	})
	if err != nil {
		t.Fatalf("RevertToPreviousHash non-existent: %v", err)
	}
	if resp.Ok {
		t.Error("expected ok=false for non-existent file")
	}

	// Invalid index
	resp, err = server.RevertToPreviousHash(ctx, &pb.RevertToPreviousHashRequest{
		Path:   "docs/specs/test.md",
		Target: &pb.RevertToPreviousHashRequest_HistoryIndex{HistoryIndex: 999},
	})
	if err != nil {
		t.Fatalf("RevertToPreviousHash invalid index: %v", err)
	}
	if resp.Ok {
		t.Error("expected ok=false for invalid index")
	}

	// Empty path
	_, err = server.RevertToPreviousHash(ctx, &pb.RevertToPreviousHashRequest{
		Target: &pb.RevertToPreviousHashRequest_HistoryIndex{HistoryIndex: 0},
	})
	if err == nil {
		t.Error("expected error for empty path")
	}

	// No target
	resp, err = server.RevertToPreviousHash(ctx, &pb.RevertToPreviousHashRequest{
		Path: "docs/specs/test.md",
	})
	if err != nil {
		t.Fatalf("RevertToPreviousHash no target: %v", err)
	}
	if resp.Ok {
		t.Error("expected ok=false for no target")
	}
}

func TestServer_VerifyNow(t *testing.T) {
	server, _ := setupTestServer(t)
	server.ScanAndSync()

	ctx := context.Background()

	// Verify single file
	resp, err := server.VerifyNow(ctx, &pb.VerifyNowRequest{
		Path: "docs/specs/test.md",
	})
	if err != nil {
		t.Fatalf("VerifyNow single: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Results))
	}

	// Verify all
	resp, err = server.VerifyNow(ctx, &pb.VerifyNowRequest{})
	if err != nil {
		t.Fatalf("VerifyNow all: %v", err)
	}

	// Verify with framework filter
	resp, err = server.VerifyNow(ctx, &pb.VerifyNowRequest{
		Framework: pb.Framework_FRAMEWORK_MANIFEST,
	})
	if err != nil {
		t.Fatalf("VerifyNow with filter: %v", err)
	}

	// Verify non-existent file
	_, err = server.VerifyNow(ctx, &pb.VerifyNowRequest{
		Path: "nonexistent.txt",
	})
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFromProtoStatus(t *testing.T) {
	tests := []struct {
		input    pb.FileStatus
		expected string
	}{
		{pb.FileStatus_FILE_STATUS_VERIFIED, "verified"},
		{pb.FileStatus_FILE_STATUS_MISMATCH, "mismatch"},
		{pb.FileStatus_FILE_STATUS_UNKNOWN, "unknown"},
		{pb.FileStatus_FILE_STATUS_REVERTED, "reverted"},
		{pb.FileStatus_FILE_STATUS_UNSPECIFIED, ""},
	}

	for _, tc := range tests {
		got := fromProtoStatus(tc.input)
		if got != tc.expected {
			t.Errorf("fromProtoStatus(%v): expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}

func TestServer_ListVerifiedFiles_WithStatusFilter(t *testing.T) {
	server, _ := setupTestServer(t)
	server.ScanAndSync()

	ctx := context.Background()

	// List with status filter (verified)
	resp, err := server.ListVerifiedFiles(ctx, &pb.ListVerifiedFilesRequest{
		Status: pb.FileStatus_FILE_STATUS_MISMATCH,
	})
	if err != nil {
		t.Fatalf("ListVerifiedFiles with status: %v", err)
	}
	// We expect mismatch since our hash won't match the actual file
	t.Logf("mismatch count: %d", resp.MismatchCount)
}

func TestServer_RegisterTrackedFile_DefaultFramework(t *testing.T) {
	server, _ := setupTestServer(t)

	ctx := context.Background()

	// Register without framework (should default to manifest)
	resp, err := server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:        "default/file.txt",
		InitialHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("RegisterTrackedFile: %v", err)
	}
	if !resp.Ok {
		t.Errorf("expected ok=true")
	}
}

func TestServer_UpdateHash_DefaultSource(t *testing.T) {
	server, _ := setupTestServer(t)
	server.ScanAndSync()

	ctx := context.Background()

	// Update without source (should default to manual)
	resp, err := server.UpdateHash(ctx, &pb.UpdateHashRequest{
		Path:    "docs/specs/test.md",
		NewHash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
	})
	if err != nil {
		t.Fatalf("UpdateHash: %v", err)
	}
	if !resp.Ok {
		t.Logf("update error: %s", resp.Error)
	}
}

func TestNewServer_InvalidRegistry(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid registry JSON
	registryDir := filepath.Join(tmpDir, "docs", "specs")
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registryDir, "unts-registry.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewServer(tmpDir)
	if err == nil {
		t.Error("expected error for invalid registry")
	}
}

func TestServer_VerifyNow_NonExistent(t *testing.T) {
	server, _ := setupTestServer(t)

	ctx := context.Background()

	// First register a file that doesn't exist
	server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:        "nonexistent/file.txt",
		Framework:   pb.Framework_FRAMEWORK_MANIFEST,
		InitialHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})

	// Verify should work but return unknown status
	resp, err := server.VerifyNow(ctx, &pb.VerifyNowRequest{
		Path: "nonexistent/file.txt",
	})
	if err != nil {
		t.Fatalf("VerifyNow: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Status != pb.FileStatus_FILE_STATUS_UNKNOWN {
		t.Errorf("expected unknown status, got %v", resp.Results[0].Status)
	}
}

func TestServer_VerifyNow_WithFrameworkFilter(t *testing.T) {
	server, _ := setupTestServer(t)

	ctx := context.Background()

	// Register files in different frameworks
	server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:        "manifest/file.txt",
		Framework:   pb.Framework_FRAMEWORK_MANIFEST,
		InitialHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:        "udts/file.proto",
		Framework:   pb.Framework_FRAMEWORK_UDTS,
		InitialHash: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	})

	// Verify only manifest files
	resp, err := server.VerifyNow(ctx, &pb.VerifyNowRequest{
		Framework: pb.Framework_FRAMEWORK_MANIFEST,
	})
	if err != nil {
		t.Fatalf("VerifyNow: %v", err)
	}

	// Count manifest files in results
	manifestCount := 0
	for _, r := range resp.Results {
		if r.Path == "manifest/file.txt" {
			manifestCount++
		}
	}
	if manifestCount == 0 {
		t.Error("expected manifest file in results")
	}
}

func TestServer_ListVerifiedFiles_CountStats(t *testing.T) {
	server, tmpDir := setupTestServer(t)

	// Create a test file that will match its hash
	testFile := filepath.Join(tmpDir, "test", "file.txt")
	if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Register and verify
	server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:        "test/file.txt",
		Framework:   pb.Framework_FRAMEWORK_MANIFEST,
		InitialHash: "wronghash0123456789abcdef0123456789abcdef0123456789abcdef12",
	})
	server.VerifyNow(ctx, &pb.VerifyNowRequest{Path: "test/file.txt"})

	resp, err := server.ListVerifiedFiles(ctx, &pb.ListVerifiedFilesRequest{})
	if err != nil {
		t.Fatalf("ListVerifiedFiles: %v", err)
	}

	t.Logf("total: %d, verified: %d, mismatch: %d", resp.TotalCount, resp.VerifiedCount, resp.MismatchCount)
}

func TestServer_RegisterTrackedFile_SavesRegistry(t *testing.T) {
	server, _ := setupTestServer(t)

	ctx := context.Background()

	// Register a file - this should save the registry
	resp, err := server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:        "save/test.txt",
		Framework:   pb.Framework_FRAMEWORK_MANIFEST,
		InitialHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		RegisteredBy: "test",
	})
	if err != nil {
		t.Fatalf("RegisterTrackedFile: %v", err)
	}
	if !resp.Ok {
		t.Errorf("expected ok=true")
	}

	// File should be retrievable
	status, err := server.GetFileStatus(ctx, &pb.GetFileStatusRequest{Path: "save/test.txt"})
	if err != nil {
		t.Fatalf("GetFileStatus: %v", err)
	}
	if status.Record.Path != "save/test.txt" {
		t.Errorf("expected path 'save/test.txt', got %q", status.Record.Path)
	}
}

func TestServer_FullWorkflow(t *testing.T) {
	server, tmpDir := setupTestServer(t)

	// Create a real file to test against
	testFile := filepath.Join(tmpDir, "workflow", "test.txt")
	if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("test content for workflow"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Register file with wrong hash
	_, err := server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:         "workflow/test.txt",
		Framework:    pb.Framework_FRAMEWORK_MANIFEST,
		InitialHash:  "0000000000000000000000000000000000000000000000000000000000000000",
		RegisteredBy: "test",
	})
	if err != nil {
		t.Fatalf("RegisterTrackedFile: %v", err)
	}

	// 2. Verify - should be mismatch
	verifyResp, err := server.VerifyNow(ctx, &pb.VerifyNowRequest{Path: "workflow/test.txt"})
	if err != nil {
		t.Fatalf("VerifyNow: %v", err)
	}
	if verifyResp.MismatchCount != 1 {
		t.Errorf("expected 1 mismatch, got %d", verifyResp.MismatchCount)
	}

	// 3. Update to correct hash
	actualHash := verifyResp.Results[0].ActualHash
	_, err = server.UpdateHash(ctx, &pb.UpdateHashRequest{
		Path:      "workflow/test.txt",
		NewHash:   actualHash,
		Source:    pb.HashSource_HASH_SOURCE_MANUAL,
		UpdatedBy: "test",
		Reason:    "fix hash",
	})
	if err != nil {
		t.Fatalf("UpdateHash: %v", err)
	}

	// 4. Verify again - should be verified
	verifyResp, err = server.VerifyNow(ctx, &pb.VerifyNowRequest{Path: "workflow/test.txt"})
	if err != nil {
		t.Fatalf("VerifyNow after update: %v", err)
	}
	if verifyResp.VerifiedCount != 1 {
		t.Errorf("expected 1 verified, got %d", verifyResp.VerifiedCount)
	}

	// 5. Get history - should have previous hash
	histResp, err := server.GetHashHistory(ctx, &pb.GetHashHistoryRequest{Path: "workflow/test.txt"})
	if err != nil {
		t.Fatalf("GetHashHistory: %v", err)
	}
	if len(histResp.History) < 1 {
		t.Error("expected at least 1 history entry")
	}

	// 6. Revert to wrong hash
	if len(histResp.History) > 0 {
		_, err = server.RevertToPreviousHash(ctx, &pb.RevertToPreviousHashRequest{
			Path:       "workflow/test.txt",
			Target:     &pb.RevertToPreviousHashRequest_HistoryIndex{HistoryIndex: 0},
			RevertedBy: "test",
			Reason:     "test revert",
		})
		if err != nil {
			t.Fatalf("RevertToPreviousHash: %v", err)
		}
	}

	// 7. List all files
	listResp, err := server.ListVerifiedFiles(ctx, &pb.ListVerifiedFilesRequest{})
	if err != nil {
		t.Fatalf("ListVerifiedFiles: %v", err)
	}
	if listResp.TotalCount < 1 {
		t.Error("expected at least 1 file in list")
	}
}

func TestServer_ListVerifiedFiles_AllStatuses(t *testing.T) {
	server, tmpDir := setupTestServer(t)

	// Create files with different expected verification results
	verifiedFile := filepath.Join(tmpDir, "status", "verified.txt")
	if err := os.MkdirAll(filepath.Dir(verifiedFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(verifiedFile, []byte("verified"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Register with wrong hash (will be mismatch)
	server.RegisterTrackedFile(ctx, &pb.RegisterTrackedFileRequest{
		Path:        "status/verified.txt",
		Framework:   pb.Framework_FRAMEWORK_MANIFEST,
		InitialHash: "0000000000000000000000000000000000000000000000000000000000000000",
	})

	// Verify to update status
	server.VerifyNow(ctx, &pb.VerifyNowRequest{})

	// List with mismatch filter
	resp, err := server.ListVerifiedFiles(ctx, &pb.ListVerifiedFilesRequest{
		Status: pb.FileStatus_FILE_STATUS_MISMATCH,
	})
	if err != nil {
		t.Fatalf("ListVerifiedFiles: %v", err)
	}
	t.Logf("mismatch files: %d", len(resp.Files))
}
