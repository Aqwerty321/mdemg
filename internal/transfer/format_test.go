package transfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "mdemg/api/transferpb"
)

func TestWriteFileReadFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.mdemg")

	// Minimal valid export: metadata + summary
	result := &ExportResult{
		Chunks: []*pb.SpaceChunk{
			{
				ChunkType:     pb.ChunkType_CHUNK_TYPE_METADATA,
				SpaceId:       "test-space",
				SchemaVersion: 4,
				Sequence:      0,
				Metadata: &pb.SpaceMetadata{
					SpaceId:             "test-space",
					SchemaVersion:       4,
					ExportedAt:          "2026-01-01T00:00:00Z",
					TotalNodes:          0,
					TotalEdges:          0,
					TotalObservations:   0,
					TotalSymbols:        0,
					EmbeddingDimensions: 1536,
				},
			},
			{
				ChunkType:     pb.ChunkType_CHUNK_TYPE_SUMMARY,
				SpaceId:       "test-space",
				SchemaVersion: 4,
				Sequence:      1,
				Summary: &pb.TransferSummary{
					NodesExported:        0,
					EdgesExported:        0,
					ObservationsExported: 0,
					SymbolsExported:      0,
					DurationMs:           100,
					CompletedAt:          "2026-01-01T00:00:01Z",
				},
			},
		},
	}

	if err := WriteFile(path, result); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	chunks, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if chunks[0].GetChunkType() != pb.ChunkType_CHUNK_TYPE_METADATA {
		t.Errorf("chunk 0 type: got %v", chunks[0].GetChunkType())
	}
	if chunks[0].GetSpaceId() != "test-space" {
		t.Errorf("chunk 0 space_id: got %q", chunks[0].GetSpaceId())
	}
	if chunks[0].GetMetadata().GetSchemaVersion() != 4 {
		t.Errorf("metadata schema version: got %d", chunks[0].GetMetadata().GetSchemaVersion())
	}
	if chunks[1].GetChunkType() != pb.ChunkType_CHUNK_TYPE_SUMMARY {
		t.Errorf("chunk 1 type: got %v", chunks[1].GetChunkType())
	}
}

func TestReadFileInvalidFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.mdemg")
	// Write valid JSON but wrong format field
	if err := os.WriteFile(path, []byte(`{"header":{"format":"wrong-format","version":"1.0.0"},"chunks":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFile(path)
	if err == nil {
		t.Fatal("ReadFile expected error for wrong format")
	}
	if !strings.Contains(err.Error(), "invalid file format") {
		t.Errorf("error message: %v", err)
	}
}

func TestExportConfigForProfile(t *testing.T) {
	tests := []struct {
		profile      string
		wantErr      bool
		wantObs      bool
		wantSym      bool
		wantLearn    bool
		onlyLearn    bool
		metadataOnly bool
	}{
		{"", false, true, true, true, false, false},
		{ProfileFull, false, true, true, true, false, false},
		{ProfileCodebase, false, false, true, true, false, false},
		{ProfileCMS, false, true, false, true, false, false},
		{ProfileLearned, false, false, false, true, true, false},
		{ProfileMetadata, false, false, false, false, false, true},
		{"unknown", true, false, false, false, false, false},
	}
	for _, tt := range tests {
		cfg, err := ExportConfigForProfile("x", tt.profile)
		if (err != nil) != tt.wantErr {
			t.Errorf("profile %q: err=%v wantErr=%v", tt.profile, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if cfg.IncludeObservations != tt.wantObs {
			t.Errorf("profile %q IncludeObservations: got %v", tt.profile, cfg.IncludeObservations)
		}
		if cfg.IncludeSymbols != tt.wantSym {
			t.Errorf("profile %q IncludeSymbols: got %v", tt.profile, cfg.IncludeSymbols)
		}
		if cfg.IncludeLearnedEdges != tt.wantLearn {
			t.Errorf("profile %q IncludeLearnedEdges: got %v", tt.profile, cfg.IncludeLearnedEdges)
		}
		if cfg.OnlyLearnedEdges != tt.onlyLearn {
			t.Errorf("profile %q OnlyLearnedEdges: got %v", tt.profile, cfg.OnlyLearnedEdges)
		}
		if cfg.MetadataOnly != tt.metadataOnly {
			t.Errorf("profile %q MetadataOnly: got %v", tt.profile, cfg.MetadataOnly)
		}
	}
}

func TestExportFromRequest(t *testing.T) {
	req := &pb.ExportRequest{
		SpaceId:             "my-space",
		ChunkSize:           100,
		IncludeEmbeddings:   false,
		IncludeObservations: false,
		IncludeSymbols:      true,
		IncludeLearnedEdges: true,
		MinLayer:            0,
		MaxLayer:            2,
	}
	cfg := ExportFromRequest(req)
	if cfg.SpaceID != "my-space" {
		t.Errorf("SpaceID: got %q", cfg.SpaceID)
	}
	if cfg.ChunkSize != 100 {
		t.Errorf("ChunkSize: got %d", cfg.ChunkSize)
	}
	if cfg.IncludeEmbeddings {
		t.Error("IncludeEmbeddings: expected false")
	}
	if cfg.IncludeObservations {
		t.Error("IncludeObservations: expected false")
	}
	if !cfg.IncludeSymbols {
		t.Error("IncludeSymbols: expected true")
	}
	if cfg.MaxLayer != 2 {
		t.Errorf("MaxLayer: got %d", cfg.MaxLayer)
	}
}

func TestExportFromRequest_Phase4Delta(t *testing.T) {
	// When since_timestamp is set, it is used
	req := &pb.ExportRequest{SpaceId: "x", SinceTimestamp: "2026-01-01T00:00:00Z"}
	cfg := ExportFromRequest(req)
	if cfg.SinceTimestamp != "2026-01-01T00:00:00Z" {
		t.Errorf("SinceTimestamp: got %q", cfg.SinceTimestamp)
	}
	// When only since_cursor is set, it is copied to SinceTimestamp (MVP: cursor is timestamp)
	req2 := &pb.ExportRequest{SpaceId: "x", SinceCursor: "2026-01-15T12:00:00Z"}
	cfg2 := ExportFromRequest(req2)
	if cfg2.SinceTimestamp != "2026-01-15T12:00:00Z" {
		t.Errorf("SinceTimestamp from cursor: got %q", cfg2.SinceTimestamp)
	}
	// When both set, since_timestamp wins
	req3 := &pb.ExportRequest{SpaceId: "x", SinceTimestamp: "2026-01-10Z", SinceCursor: "other"}
	cfg3 := ExportFromRequest(req3)
	if cfg3.SinceTimestamp != "2026-01-10Z" {
		t.Errorf("SinceTimestamp with both set: got %q", cfg3.SinceTimestamp)
	}
}

func TestWriteFileReadFileWithEmbeddings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "embed.mdemg")

	emb := []float32{0.1, -0.2, 0.3}
	result := &ExportResult{
		Chunks: []*pb.SpaceChunk{
			{
				ChunkType:     pb.ChunkType_CHUNK_TYPE_METADATA,
				SpaceId:       "embed-space",
				SchemaVersion: 4,
				Sequence:      0,
				Metadata: &pb.SpaceMetadata{
					SpaceId:             "embed-space",
					SchemaVersion:       4,
					TotalNodes:          1,
					EmbeddingDimensions: 3,
				},
			},
			{
				ChunkType:     pb.ChunkType_CHUNK_TYPE_NODES,
				SpaceId:       "embed-space",
				SchemaVersion: 4,
				Sequence:      1,
				Nodes: &pb.NodeBatch{
					Nodes: []*pb.NodeData{
						{
							NodeId:    "n1",
							SpaceId:   "embed-space",
							Layer:     0,
							Embedding: emb,
						},
					},
				},
			},
			{
				ChunkType:     pb.ChunkType_CHUNK_TYPE_SUMMARY,
				SpaceId:       "embed-space",
				SchemaVersion: 4,
				Sequence:      2,
				Summary:       &pb.TransferSummary{NodesExported: 1},
			},
		},
	}

	if err := WriteFile(path, result); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	chunks, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(chunks))
	}
	gotEmb := chunks[1].GetNodes().GetNodes()[0].GetEmbedding()
	if len(gotEmb) != 3 || gotEmb[0] != 0.1 || gotEmb[1] != -0.2 || gotEmb[2] != 0.3 {
		t.Errorf("embedding round-trip: got %v", gotEmb)
	}
}
