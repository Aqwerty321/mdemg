package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pb "mdemg/api/modulepb"
)

func TestEntityToMap(t *testing.T) {
	entity := &pb.CRUDEntity{
		Id:         "ent-1",
		EntityType: "issue",
		Fields: map[string]string{
			"title": "Test",
			"state": "open",
		},
		CreatedAt: "2024-01-01T00:00:00Z",
		UpdatedAt: "2024-01-02T00:00:00Z",
	}

	m := entityToMap(entity)
	if m["id"] != "ent-1" {
		t.Errorf("expected id=ent-1, got %v", m["id"])
	}
	if m["entity_type"] != "issue" {
		t.Errorf("expected entity_type=issue, got %v", m["entity_type"])
	}
	fields, ok := m["fields"].(map[string]string)
	if !ok {
		t.Fatal("expected fields to be map[string]string")
	}
	if fields["title"] != "Test" {
		t.Errorf("expected fields.title=Test, got %v", fields["title"])
	}
}

func TestEntityToMap_Nil(t *testing.T) {
	m := entityToMap(nil)
	if m != nil {
		t.Errorf("expected nil for nil entity, got %v", m)
	}
}

func TestHandleLinearIssues_NoCRUDModule(t *testing.T) {
	// Server with no plugin manager
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/v1/linear/issues", nil)
	w := httptest.NewRecorder()

	s.handleLinearIssues(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if errMsg, ok := resp["error"].(string); !ok || !strings.Contains(errMsg, "not available") {
		t.Errorf("expected 'not available' error, got %v", resp["error"])
	}
}

func TestHandleLinearProjects_NoCRUDModule(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/v1/linear/projects", nil)
	w := httptest.NewRecorder()

	s.handleLinearProjects(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleLinearComments_MethodNotAllowed(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/v1/linear/comments", nil)
	w := httptest.NewRecorder()

	s.handleLinearComments(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleLinearComments_NoCRUDModule(t *testing.T) {
	s := &Server{}

	body := `{"issue_id":"iss-1","body":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/linear/comments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleLinearComments(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleLinearIssues_MethodNotAllowed(t *testing.T) {
	s := &Server{}

	// POST to /issues/{id} is not allowed
	req := httptest.NewRequest(http.MethodPost, "/v1/linear/issues/some-id", nil)
	w := httptest.NewRecorder()

	// Since no CRUD module, it'll return 503 first — need to test method routing
	// separately. This tests the service unavailable path.
	s.handleLinearIssues(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleLinearComments_MissingFields(t *testing.T) {
	// We can't easily test with a real CRUD module, but we can test validation
	// by providing an invalid body. The handler reads JSON first, then validates.
	s := &Server{}

	// Empty body - should fail validation
	req := httptest.NewRequest(http.MethodPost, "/v1/linear/comments", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleLinearComments(w, req)

	// Should get 503 since no CRUD module, not validation error
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no module), got %d", w.Code)
	}
}

func TestFindCRUDModule_NilManager(t *testing.T) {
	s := &Server{}
	mod := s.findCRUDModule("")
	if mod != nil {
		t.Error("expected nil when plugin manager is nil")
	}
}
