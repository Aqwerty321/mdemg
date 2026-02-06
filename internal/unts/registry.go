// Package unts implements the Universal Hash Test Specification (Hash Verification Module)
// Alias: Nash Verification module
package unts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	pb "mdemg/api/untspb"
)

const (
	MaxHistoryEntries = 3
	registryFilename  = "docs/specs/unts-registry.json"
)

// Registry holds the in-memory state of all tracked files
type Registry struct {
	mu       sync.RWMutex
	files    map[string]*FileRecord // keyed by path
	basePath string                 // repository root
}

// FileRecord represents a tracked file's verification state
type FileRecord struct {
	Path        string          `json:"path"`
	Framework   string          `json:"framework"`
	CurrentHash string          `json:"current_hash"`
	Status      string          `json:"status"`
	UpdatedAt   string          `json:"updated_at"`
	History     []HistoryEntry  `json:"history"`
	SourceRef   string          `json:"source_ref"`
}

// HistoryEntry represents a historical hash value
type HistoryEntry struct {
	Hash      string `json:"hash"`
	UpdatedAt string `json:"updated_at"`
	Source    string `json:"source"`
}

// RegistryFile is the JSON structure for persistence
type RegistryFile struct {
	Version     string        `json:"version"`
	UpdatedAt   string        `json:"updated_at"`
	Description string        `json:"description"`
	Files       []*FileRecord `json:"files"`
}

// NewRegistry creates a new registry with the given base path
func NewRegistry(basePath string) *Registry {
	return &Registry{
		files:    make(map[string]*FileRecord),
		basePath: basePath,
	}
}

// Load reads the registry from disk
func (r *Registry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	registryPath := filepath.Join(r.basePath, registryFilename)
	data, err := os.ReadFile(registryPath)
	if os.IsNotExist(err) {
		// Empty registry is OK
		return nil
	}
	if err != nil {
		return fmt.Errorf("read registry: %w", err)
	}

	var rf RegistryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("parse registry: %w", err)
	}

	r.files = make(map[string]*FileRecord)
	for _, f := range rf.Files {
		r.files[f.Path] = f
	}
	return nil
}

// Save writes the registry to disk
func (r *Registry) Save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	files := make([]*FileRecord, 0, len(r.files))
	for _, f := range r.files {
		files = append(files, f)
	}

	rf := RegistryFile{
		Version:     "1.0.0",
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		Description: "UNTS Hash Verification Registry - tracks all hash-verified files across MDEMG frameworks",
		Files:       files,
	}

	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}

	registryPath := filepath.Join(r.basePath, registryFilename)
	if err := os.WriteFile(registryPath, data, 0644); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

// Get returns a file record by path
func (r *Registry) Get(path string) (*FileRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.files[path]
	return f, ok
}

// List returns all file records, optionally filtered
func (r *Registry) List(framework, status string) []*FileRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*FileRecord, 0, len(r.files))
	for _, f := range r.files {
		if framework != "" && f.Framework != framework {
			continue
		}
		if status != "" && f.Status != status {
			continue
		}
		result = append(result, f)
	}
	return result
}

// Register adds or updates a file in the registry
func (r *Registry) Register(path, framework, hash, sourceRef, source string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)

	if existing, ok := r.files[path]; ok {
		// Update existing - push current to history
		if existing.CurrentHash != hash {
			existing.History = prependHistory(existing.History, HistoryEntry{
				Hash:      existing.CurrentHash,
				UpdatedAt: existing.UpdatedAt,
				Source:    source,
			})
			existing.CurrentHash = hash
			existing.UpdatedAt = now
		}
		if sourceRef != "" {
			existing.SourceRef = sourceRef
		}
	} else {
		// New file
		r.files[path] = &FileRecord{
			Path:        path,
			Framework:   framework,
			CurrentHash: hash,
			Status:      "unknown",
			UpdatedAt:   now,
			History:     []HistoryEntry{},
			SourceRef:   sourceRef,
		}
	}
	return nil
}

// UpdateHash sets a new expected hash for a file
func (r *Registry) UpdateHash(path, newHash, source string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.files[path]
	if !ok {
		return fmt.Errorf("file not tracked: %s", path)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Push current to history
	f.History = prependHistory(f.History, HistoryEntry{
		Hash:      f.CurrentHash,
		UpdatedAt: f.UpdatedAt,
		Source:    source,
	})
	f.CurrentHash = newHash
	f.UpdatedAt = now
	f.Status = "unknown" // Will be verified on next VerifyNow

	return nil
}

// RevertHash reverts to a previous hash value
func (r *Registry) RevertHash(path, targetHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.files[path]
	if !ok {
		return fmt.Errorf("file not tracked: %s", path)
	}

	// Find target in history
	found := false
	for _, h := range f.History {
		if h.Hash == targetHash {
			found = true
			break
		}
	}
	if !found && targetHash != "" {
		return fmt.Errorf("target hash not in history: %s", targetHash)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Push current to history
	f.History = prependHistory(f.History, HistoryEntry{
		Hash:      f.CurrentHash,
		UpdatedAt: f.UpdatedAt,
		Source:    "revert",
	})
	f.CurrentHash = targetHash
	f.UpdatedAt = now
	f.Status = "reverted"

	return nil
}

// Verify checks a file's hash against expected
func (r *Registry) Verify(path string) (*VerifyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.files[path]
	if !ok {
		return nil, fmt.Errorf("file not tracked: %s", path)
	}

	fullPath := filepath.Join(r.basePath, path)
	actualHash, err := computeFileHash(fullPath)
	if err != nil {
		f.Status = "unknown"
		f.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return &VerifyResult{
			Path:         path,
			Status:       "unknown",
			ExpectedHash: f.CurrentHash,
			ActualHash:   "",
			Error:        err.Error(),
		}, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	f.UpdatedAt = now

	if actualHash == f.CurrentHash {
		f.Status = "verified"
		return &VerifyResult{
			Path:         path,
			Status:       "verified",
			ExpectedHash: f.CurrentHash,
			ActualHash:   actualHash,
		}, nil
	}

	f.Status = "mismatch"
	return &VerifyResult{
		Path:         path,
		Status:       "mismatch",
		ExpectedHash: f.CurrentHash,
		ActualHash:   actualHash,
	}, nil
}

// VerifyAll checks all tracked files
func (r *Registry) VerifyAll(framework string) []*VerifyResult {
	// Get list under read lock
	r.mu.RLock()
	paths := make([]string, 0, len(r.files))
	for path, f := range r.files {
		if framework == "" || f.Framework == framework {
			paths = append(paths, path)
		}
	}
	r.mu.RUnlock()

	// Verify each (acquires write lock per file)
	results := make([]*VerifyResult, 0, len(paths))
	for _, path := range paths {
		result, _ := r.Verify(path)
		if result != nil {
			results = append(results, result)
		}
	}
	return results
}

// VerifyResult holds the result of a single verification
type VerifyResult struct {
	Path         string
	Status       string
	ExpectedHash string
	ActualHash   string
	Error        string
}

// ToProto converts a FileRecord to protobuf
func (f *FileRecord) ToProto() *pb.VerifiedFileRecord {
	history := make([]*pb.HashHistoryEntry, len(f.History))
	for i, h := range f.History {
		history[i] = &pb.HashHistoryEntry{
			Hash:      h.Hash,
			UpdatedAt: h.UpdatedAt,
			Source:    toProtoSource(h.Source),
		}
	}

	return &pb.VerifiedFileRecord{
		Path:        f.Path,
		Framework:   toProtoFramework(f.Framework),
		CurrentHash: f.CurrentHash,
		Status:      toProtoStatus(f.Status),
		UpdatedAt:   f.UpdatedAt,
		History:     history,
		SourceRef:   f.SourceRef,
	}
}

// Helper functions

func prependHistory(history []HistoryEntry, entry HistoryEntry) []HistoryEntry {
	result := append([]HistoryEntry{entry}, history...)
	if len(result) > MaxHistoryEntries {
		result = result[:MaxHistoryEntries]
	}
	return result
}

func computeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func toProtoFramework(s string) pb.Framework {
	switch s {
	case "manifest":
		return pb.Framework_FRAMEWORK_MANIFEST
	case "udts":
		return pb.Framework_FRAMEWORK_UDTS
	case "uats":
		return pb.Framework_FRAMEWORK_UATS
	case "ubts":
		return pb.Framework_FRAMEWORK_UBTS
	case "usts":
		return pb.Framework_FRAMEWORK_USTS
	case "uots":
		return pb.Framework_FRAMEWORK_UOTS
	case "uams":
		return pb.Framework_FRAMEWORK_UAMS
	case "upts":
		return pb.Framework_FRAMEWORK_UPTS
	default:
		return pb.Framework_FRAMEWORK_UNSPECIFIED
	}
}

func toProtoStatus(s string) pb.FileStatus {
	switch s {
	case "verified":
		return pb.FileStatus_FILE_STATUS_VERIFIED
	case "mismatch":
		return pb.FileStatus_FILE_STATUS_MISMATCH
	case "unknown":
		return pb.FileStatus_FILE_STATUS_UNKNOWN
	case "reverted":
		return pb.FileStatus_FILE_STATUS_REVERTED
	default:
		return pb.FileStatus_FILE_STATUS_UNSPECIFIED
	}
}

func toProtoSource(s string) pb.HashSource {
	switch s {
	case "manifest":
		return pb.HashSource_HASH_SOURCE_MANIFEST
	case "spec":
		return pb.HashSource_HASH_SOURCE_SPEC
	case "revert":
		return pb.HashSource_HASH_SOURCE_REVERT
	case "manual":
		return pb.HashSource_HASH_SOURCE_MANUAL
	case "ci":
		return pb.HashSource_HASH_SOURCE_CI
	default:
		return pb.HashSource_HASH_SOURCE_UNSPECIFIED
	}
}

func fromProtoFramework(f pb.Framework) string {
	switch f {
	case pb.Framework_FRAMEWORK_MANIFEST:
		return "manifest"
	case pb.Framework_FRAMEWORK_UDTS:
		return "udts"
	case pb.Framework_FRAMEWORK_UATS:
		return "uats"
	case pb.Framework_FRAMEWORK_UBTS:
		return "ubts"
	case pb.Framework_FRAMEWORK_USTS:
		return "usts"
	case pb.Framework_FRAMEWORK_UOTS:
		return "uots"
	case pb.Framework_FRAMEWORK_UAMS:
		return "uams"
	case pb.Framework_FRAMEWORK_UPTS:
		return "upts"
	default:
		return ""
	}
}

func fromProtoSource(s pb.HashSource) string {
	switch s {
	case pb.HashSource_HASH_SOURCE_MANIFEST:
		return "manifest"
	case pb.HashSource_HASH_SOURCE_SPEC:
		return "spec"
	case pb.HashSource_HASH_SOURCE_REVERT:
		return "revert"
	case pb.HashSource_HASH_SOURCE_MANUAL:
		return "manual"
	case pb.HashSource_HASH_SOURCE_CI:
		return "ci"
	default:
		return ""
	}
}
