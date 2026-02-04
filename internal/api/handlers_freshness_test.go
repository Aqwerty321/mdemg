package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mdemg/internal/models"
)

func TestHandleSpaceFreshness_MethodNotAllowed(t *testing.T) {
	srv := &Server{}

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/memory/spaces/test-space/freshness", nil)
			w := httptest.NewRecorder()

			srv.handleSpaceFreshness(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHandleSpaceFreshness_MissingSpaceID(t *testing.T) {
	srv := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/v1/memory/spaces//freshness", nil)
	w := httptest.NewRecorder()

	srv.handleSpaceFreshness(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["error"]; !ok {
		t.Error("expected error field in response")
	}
}

func TestFreshnessResponse_JSON(t *testing.T) {
	resp := models.FreshnessResponse{
		SpaceID:        "test-space",
		LastIngestAt:   "2025-01-01T00:00:00Z",
		LastIngestType: "codebase-ingest",
		IngestCount:    5,
		IsStale:        false,
		StaleHours:     12,
		ThresholdHours: 24,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded models.FreshnessResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.SpaceID != "test-space" {
		t.Errorf("space_id: got %q, want %q", decoded.SpaceID, "test-space")
	}
	if decoded.LastIngestAt != "2025-01-01T00:00:00Z" {
		t.Errorf("last_ingest_at: got %q, want %q", decoded.LastIngestAt, "2025-01-01T00:00:00Z")
	}
	if decoded.LastIngestType != "codebase-ingest" {
		t.Errorf("last_ingest_type: got %q, want %q", decoded.LastIngestType, "codebase-ingest")
	}
	if decoded.IngestCount != 5 {
		t.Errorf("ingest_count: got %d, want %d", decoded.IngestCount, 5)
	}
	if decoded.IsStale {
		t.Error("is_stale: got true, want false")
	}
	if decoded.StaleHours != 12 {
		t.Errorf("stale_hours: got %d, want %d", decoded.StaleHours, 12)
	}
	if decoded.ThresholdHours != 24 {
		t.Errorf("threshold_hours: got %d, want %d", decoded.ThresholdHours, 24)
	}

	// Verify omitempty works for zero values
	emptyResp := models.FreshnessResponse{
		SpaceID:        "empty",
		IngestCount:    0,
		IsStale:        true,
		ThresholdHours: 24,
	}

	emptyData, err := json.Marshal(emptyResp)
	if err != nil {
		t.Fatalf("marshal empty error: %v", err)
	}

	// Verify omitted fields
	var rawMap map[string]any
	json.Unmarshal(emptyData, &rawMap)
	if _, ok := rawMap["last_ingest_at"]; ok {
		t.Error("last_ingest_at should be omitted when empty")
	}
	if _, ok := rawMap["last_ingest_type"]; ok {
		t.Error("last_ingest_type should be omitted when empty")
	}
}
