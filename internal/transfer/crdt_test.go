package transfer

import (
	"testing"

	pb "mdemg/api/transferpb"
)

func TestCRDTConflictModeExists(t *testing.T) {
	// Verify CONFLICT_CRDT mode is defined
	mode := pb.ConflictMode_CONFLICT_CRDT
	if mode != 3 {
		t.Errorf("expected CONFLICT_CRDT = 3, got %d", mode)
	}

	// Verify it's a valid enum value
	name := pb.ConflictMode_name[int32(mode)]
	if name != "CONFLICT_CRDT" {
		t.Errorf("expected name 'CONFLICT_CRDT', got %q", name)
	}
}

func TestLineageStructure(t *testing.T) {
	// Verify Lineage message structure
	lineage := &pb.Lineage{
		OriginSpaceId: "test-space",
		OriginHost:    "localhost",
		CreatedAt:     "2026-02-06T00:00:00Z",
		History: []*pb.LineageEvent{
			{
				EventType:     "export",
				Timestamp:     "2026-02-06T00:00:00Z",
				SourceHost:    "localhost",
				AgentId:       "test-agent",
				TargetSpaceId: "",
				Notes:         "Initial export",
			},
		},
	}

	if lineage.OriginSpaceId != "test-space" {
		t.Errorf("expected OriginSpaceId 'test-space', got %q", lineage.OriginSpaceId)
	}

	if len(lineage.History) != 1 {
		t.Errorf("expected 1 history event, got %d", len(lineage.History))
	}

	event := lineage.History[0]
	if event.EventType != "export" {
		t.Errorf("expected EventType 'export', got %q", event.EventType)
	}
}

func TestEdgeDataDimensionalWeights(t *testing.T) {
	// Verify EdgeData has dimensional weight fields
	edge := &pb.EdgeData{
		FromNodeId:    "node-a",
		ToNodeId:      "node-b",
		RelType:       "CO_ACTIVATED_WITH",
		SpaceId:       "test-space",
		Weight:        0.8,
		EvidenceCount: 5,
		DimTemporal:   0.6,
		DimSemantic:   0.9,
		DimCausal:     0.3,
	}

	if edge.DimTemporal != 0.6 {
		t.Errorf("expected DimTemporal 0.6, got %f", edge.DimTemporal)
	}
	if edge.DimSemantic != 0.9 {
		t.Errorf("expected DimSemantic 0.9, got %f", edge.DimSemantic)
	}
	if edge.DimCausal != 0.3 {
		t.Errorf("expected DimCausal 0.3, got %f", edge.DimCausal)
	}
}

func TestImportStatsMergedField(t *testing.T) {
	// Verify ImportStats has EdgesMerged field
	stats := &pb.ImportStats{
		NodesCreated:        10,
		EdgesCreated:        5,
		EdgesMerged:         3,
		ObservationsCreated: 2,
	}

	if stats.EdgesMerged != 3 {
		t.Errorf("expected EdgesMerged 3, got %d", stats.EdgesMerged)
	}
}

func TestSpaceMetadataLineage(t *testing.T) {
	// Verify SpaceMetadata can contain Lineage
	metadata := &pb.SpaceMetadata{
		SpaceId:       "test-space",
		SchemaVersion: 7,
		ExportedAt:    "2026-02-06T00:00:00Z",
		SourceHost:    "localhost",
		Lineage: &pb.Lineage{
			OriginSpaceId: "test-space",
			OriginHost:    "localhost",
			CreatedAt:     "2026-02-06T00:00:00Z",
		},
	}

	if metadata.Lineage == nil {
		t.Fatal("expected Lineage to be set")
	}

	if metadata.Lineage.OriginSpaceId != "test-space" {
		t.Errorf("expected OriginSpaceId 'test-space', got %q", metadata.Lineage.OriginSpaceId)
	}
}

func TestImportResultToProtoWithMerged(t *testing.T) {
	result := &ImportResult{
		NodesCreated:        10,
		EdgesCreated:        5,
		EdgesMerged:         3,
		ObservationsCreated: 2,
	}

	proto := result.ToProto()

	if proto.Stats.EdgesMerged != 3 {
		t.Errorf("expected EdgesMerged 3 in proto, got %d", proto.Stats.EdgesMerged)
	}
}

func TestBatchStatsTrackssMerged(t *testing.T) {
	// Verify batchStats includes merged counter
	stats := batchStats{
		created: 5,
		skipped: 2,
		merged:  3,
	}

	if stats.merged != 3 {
		t.Errorf("expected merged 3, got %d", stats.merged)
	}
}
