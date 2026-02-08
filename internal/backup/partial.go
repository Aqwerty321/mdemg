package backup

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"mdemg/internal/jobs"
	"mdemg/internal/transfer"
)

// runPartialBackup exports selected spaces via the transfer exporter.
func (s *Service) runPartialBackup(ctx context.Context, job *jobs.Job, record *BackupRecord, spaceIDs []string) error {
	job.UpdateProgress(0, "resolving spaces")

	// If no spaces specified, export all.
	if len(spaceIDs) == 0 {
		all, err := s.queryAllSpaceIDs(ctx)
		if err != nil {
			return fmt.Errorf("query space IDs: %w", err)
		}
		spaceIDs = all
	}

	// Always include the protected space.
	spaceIDs = ensureSpaceIncluded(spaceIDs, "mdemg-dev")
	record.Spaces = spaceIDs

	job.SetTotal(len(spaceIDs))

	// Collect export results from all spaces.
	var allResult transfer.ExportResult
	for i, spaceID := range spaceIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		job.UpdateProgress(i, fmt.Sprintf("exporting space %s", spaceID))

		cfg := transfer.DefaultExportConfig(spaceID)
		result, err := s.exporter.Export(ctx, cfg)
		if err != nil {
			return fmt.Errorf("export space %s: %w", spaceID, err)
		}
		allResult.Chunks = append(allResult.Chunks, result.Chunks...)
	}

	// Write combined .mdemg file.
	outPath := filepath.Join(s.cfg.StorageDir, record.BackupID+".mdemg")
	if err := transfer.WriteFile(outPath, &allResult); err != nil {
		return fmt.Errorf("write mdemg file: %w", err)
	}

	job.UpdateProgress(len(spaceIDs), "computing checksum")

	// Checksum + size.
	checksum, err := sha256File(outPath)
	if err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	size, err := fileSize(outPath)
	if err != nil {
		return fmt.Errorf("file size: %w", err)
	}

	record.Checksum = checksum
	record.SizeBytes = size
	record.Path = outPath
	record.Status = "completed"
	now := time.Now().UTC()
	record.CompletedAt = &now

	// Node/edge counts and schema version for manifest.
	nodeCount, edgeCount, _ := s.queryNodeEdgeCounts(ctx)
	schemaVer, _ := s.querySchemaVersion(ctx)

	manifest := BackupManifest{
		BackupID:      record.BackupID,
		Type:          string(record.Type),
		FormatVersion: "1.0",
		CreatedAt:     record.StartedAt.Format(time.RFC3339),
		Checksum:      checksum,
		SizeBytes:     size,
		Spaces:        spaceIDs,
		NodeCount:     nodeCount,
		EdgeCount:     edgeCount,
		SchemaVersion: schemaVer,
		KeepForever:   record.KeepForever,
		Label:         record.Label,
	}

	return s.writeManifest(record, manifest)
}
