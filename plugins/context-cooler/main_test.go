package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	pb "mdemg/api/modulepb"
)

func TestHandshake(t *testing.T) {
	s := &server{startTime: time.Now()}

	resp, err := s.Handshake(context.Background(), &pb.HandshakeRequest{
		MdemgVersion: "1.0.0",
	})

	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}
	if resp.ModuleId != moduleID {
		t.Errorf("ModuleId = %s, want %s", resp.ModuleId, moduleID)
	}
	if resp.ModuleVersion != moduleVersion {
		t.Errorf("ModuleVersion = %s, want %s", resp.ModuleVersion, moduleVersion)
	}
	if resp.ModuleType != pb.ModuleType_MODULE_TYPE_APE {
		t.Errorf("ModuleType = %v, want APE", resp.ModuleType)
	}
	if !resp.Ready {
		t.Error("Ready should be true")
	}
	if len(resp.Capabilities) == 0 {
		t.Error("Capabilities should not be empty")
	}
}

func TestHealthCheck(t *testing.T) {
	s := &server{
		startTime:       time.Now().Add(-1 * time.Hour),
		executionsTotal: 5,
		lastExecution:   time.Now().Add(-10 * time.Minute),
		lastGraduated:   3,
		lastTombstoned:  1,
	}

	resp, err := s.HealthCheck(context.Background(), &pb.HealthCheckRequest{})

	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if !resp.Healthy {
		t.Error("Healthy should be true")
	}
	if resp.Status != "ready" {
		t.Errorf("Status = %s, want ready", resp.Status)
	}
	if _, ok := resp.Metrics["uptime"]; !ok {
		t.Error("Metrics should include uptime")
	}
	if resp.Metrics["executions_total"] != "5" {
		t.Errorf("executions_total = %s, want 5", resp.Metrics["executions_total"])
	}
	if resp.Metrics["last_graduated_count"] != "3" {
		t.Errorf("last_graduated_count = %s, want 3", resp.Metrics["last_graduated_count"])
	}
	if resp.Metrics["last_tombstoned_count"] != "1" {
		t.Errorf("last_tombstoned_count = %s, want 1", resp.Metrics["last_tombstoned_count"])
	}
}

func TestHealthCheck_NoExecutions(t *testing.T) {
	s := &server{startTime: time.Now()}

	resp, err := s.HealthCheck(context.Background(), &pb.HealthCheckRequest{})

	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if _, ok := resp.Metrics["last_execution"]; ok {
		t.Error("last_execution should not be present when no executions")
	}
}

func TestShutdown(t *testing.T) {
	s := &server{}

	resp, err := s.Shutdown(context.Background(), &pb.ShutdownRequest{
		Reason: "test shutdown",
	})

	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	if !resp.Success {
		t.Error("Success should be true")
	}
}

func TestGetSchedule(t *testing.T) {
	s := &server{}

	resp, err := s.GetSchedule(context.Background(), &pb.GetScheduleRequest{})

	if err != nil {
		t.Fatalf("GetSchedule failed: %v", err)
	}
	if resp.CronExpression != defaultCronExpression {
		t.Errorf("CronExpression = %s, want %s", resp.CronExpression, defaultCronExpression)
	}
	if len(resp.EventTriggers) != 2 {
		t.Errorf("EventTriggers len = %d, want 2", len(resp.EventTriggers))
	}
	if resp.MinIntervalSeconds != defaultMinInterval {
		t.Errorf("MinIntervalSeconds = %d, want %d", resp.MinIntervalSeconds, defaultMinInterval)
	}
}

func TestGetSchedule_EventTriggers(t *testing.T) {
	s := &server{}

	resp, _ := s.GetSchedule(context.Background(), &pb.GetScheduleRequest{})

	triggers := make(map[string]bool)
	for _, tr := range resp.EventTriggers {
		triggers[tr] = true
	}

	if !triggers["session_end"] {
		t.Error("should include session_end trigger")
	}
	if !triggers["consolidate"] {
		t.Error("should include consolidate trigger")
	}
}

func TestExecute_NoServer(t *testing.T) {
	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: "http://localhost:59999", // Non-existent server
		httpClient:    &http.Client{Timeout: 1 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "test-123",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "test-space"},
	})

	// Should return error response, not Go error
	if err != nil {
		t.Fatalf("Execute should not return Go error: %v", err)
	}
	if resp.Success {
		t.Error("Success should be false when server unreachable")
	}
	if resp.Error == "" {
		t.Error("Error should contain message")
	}
	if resp.Stats == nil {
		t.Error("Stats should not be nil")
	}
}

func TestExecute_DefaultSpaceID(t *testing.T) {
	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: "http://localhost:59999",
		httpClient:    &http.Client{Timeout: 1 * time.Second},
	}

	// Without space_id in context
	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "test-456",
		Trigger: "event:session_end",
		Context: map[string]string{}, // No space_id
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should still execute (with default space_id = mdemg-dev)
	// Will fail due to no server, but that's expected
	if resp.Stats == nil {
		t.Error("Stats should not be nil")
	}
}

func TestExecute_IncrementCounters(t *testing.T) {
	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: "http://localhost:59999",
		httpClient:    &http.Client{Timeout: 1 * time.Second},
	}

	// Execute twice
	s.Execute(context.Background(), &pb.ExecuteRequest{TaskId: "1", Trigger: "schedule"})
	s.Execute(context.Background(), &pb.ExecuteRequest{TaskId: "2", Trigger: "schedule"})

	if s.executionsTotal != 2 {
		t.Errorf("executionsTotal = %d, want 2", s.executionsTotal)
	}
	if s.lastExecution.IsZero() {
		t.Error("lastExecution should not be zero")
	}
}

// createMockMDEMGServer creates a mock server that returns a valid GraduateSummary
func createMockMDEMGServer(t *testing.T, statusCode int, summary *GraduateSummary, errorBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/v1/conversation/graduate" {
			t.Errorf("Expected path /v1/conversation/graduate, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Decode request body to verify it's valid
		var reqBody GraduateRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		if statusCode == http.StatusOK && summary != nil {
			json.NewEncoder(w).Encode(summary)
		} else if errorBody != "" {
			w.Write([]byte(errorBody))
		}
	}))
}

func TestExecute_SuccessWithMockServer(t *testing.T) {
	summary := &GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         time.Now(),
		Graduated:         5,
		Tombstoned:        2,
		RemainingVolatile: 10,
		DecayApplied:      3,
	}

	mockServer := createMockMDEMGServer(t, http.StatusOK, summary, "")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "test-success-123",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if !resp.Success {
		t.Errorf("Success = false, want true; Error: %s", resp.Error)
	}
	if resp.Error != "" {
		t.Errorf("Error = %s, want empty", resp.Error)
	}
	if resp.Message == "" {
		t.Error("Message should not be empty on success")
	}
	if resp.Stats == nil {
		t.Fatal("Stats should not be nil")
	}
	if resp.Stats.NodesUpdated != 7 { // 5 graduated + 2 tombstoned
		t.Errorf("NodesUpdated = %d, want 7", resp.Stats.NodesUpdated)
	}
	if resp.Stats.DurationMs < 0 {
		t.Errorf("DurationMs = %d, should be >= 0", resp.Stats.DurationMs)
	}

	// Verify metrics were updated
	if s.lastGraduated != 5 {
		t.Errorf("lastGraduated = %d, want 5", s.lastGraduated)
	}
	if s.lastTombstoned != 2 {
		t.Errorf("lastTombstoned = %d, want 2", s.lastTombstoned)
	}
}

func TestExecute_TriggerSessionEnd(t *testing.T) {
	summary := &GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         time.Now(),
		Graduated:         3,
		Tombstoned:        1,
		RemainingVolatile: 8,
		DecayApplied:      2,
	}

	mockServer := createMockMDEMGServer(t, http.StatusOK, summary, "")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "session-end-test",
		Trigger: "event:session_end",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if !resp.Success {
		t.Errorf("Success = false, want true; Error: %s", resp.Error)
	}
	// Check message contains session_end specific text
	if resp.Message == "" {
		t.Error("Message should not be empty")
	}
	expectedSubstr := "Session end graduation"
	if len(resp.Message) < len(expectedSubstr) || resp.Message[:len(expectedSubstr)] != expectedSubstr {
		t.Errorf("Message should start with '%s', got: %s", expectedSubstr, resp.Message)
	}
}

func TestExecute_TriggerConsolidate(t *testing.T) {
	summary := &GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         time.Now(),
		Graduated:         4,
		Tombstoned:        0,
		RemainingVolatile: 12,
		DecayApplied:      1,
	}

	mockServer := createMockMDEMGServer(t, http.StatusOK, summary, "")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "consolidate-test",
		Trigger: "event:consolidate",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if !resp.Success {
		t.Errorf("Success = false, want true; Error: %s", resp.Error)
	}
	expectedSubstr := "Post-consolidation graduation"
	if len(resp.Message) < len(expectedSubstr) || resp.Message[:len(expectedSubstr)] != expectedSubstr {
		t.Errorf("Message should start with '%s', got: %s", expectedSubstr, resp.Message)
	}
}

func TestExecute_TriggerSchedule(t *testing.T) {
	summary := &GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         time.Now(),
		Graduated:         2,
		Tombstoned:        3,
		RemainingVolatile: 5,
		DecayApplied:      0,
	}

	mockServer := createMockMDEMGServer(t, http.StatusOK, summary, "")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "schedule-test",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if !resp.Success {
		t.Errorf("Success = false, want true; Error: %s", resp.Error)
	}
	expectedSubstr := "Scheduled graduation"
	if len(resp.Message) < len(expectedSubstr) || resp.Message[:len(expectedSubstr)] != expectedSubstr {
		t.Errorf("Message should start with '%s', got: %s", expectedSubstr, resp.Message)
	}
}

func TestExecute_TriggerUnknown(t *testing.T) {
	summary := &GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         time.Now(),
		Graduated:         1,
		Tombstoned:        1,
		RemainingVolatile: 3,
		DecayApplied:      0,
	}

	mockServer := createMockMDEMGServer(t, http.StatusOK, summary, "")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "unknown-trigger-test",
		Trigger: "custom:unknown_trigger",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if !resp.Success {
		t.Errorf("Success = false, want true; Error: %s", resp.Error)
	}
	// Default message for unknown triggers
	expectedSubstr := "Graduation complete"
	if len(resp.Message) < len(expectedSubstr) || resp.Message[:len(expectedSubstr)] != expectedSubstr {
		t.Errorf("Message should start with '%s', got: %s", expectedSubstr, resp.Message)
	}
}

func TestExecute_ServerReturns500(t *testing.T) {
	mockServer := createMockMDEMGServer(t, http.StatusInternalServerError, nil, "internal server error")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "error-500-test",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute should not return Go error: %v", err)
	}
	if resp.Success {
		t.Error("Success should be false when server returns 500")
	}
	if resp.Error == "" {
		t.Error("Error message should not be empty")
	}
	if resp.Stats == nil {
		t.Error("Stats should not be nil")
	}
}

func TestExecute_ServerReturns400(t *testing.T) {
	mockServer := createMockMDEMGServer(t, http.StatusBadRequest, nil, `{"error":"invalid space_id"}`)
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "error-400-test",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "invalid-space"},
	})

	if err != nil {
		t.Fatalf("Execute should not return Go error: %v", err)
	}
	if resp.Success {
		t.Error("Success should be false when server returns 400")
	}
	if resp.Error == "" {
		t.Error("Error message should not be empty")
	}
}

func TestExecute_ServerReturns404(t *testing.T) {
	mockServer := createMockMDEMGServer(t, http.StatusNotFound, nil, "endpoint not found")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "error-404-test",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute should not return Go error: %v", err)
	}
	if resp.Success {
		t.Error("Success should be false when server returns 404")
	}
	if resp.Error == "" {
		t.Error("Error message should not be empty")
	}
}

func TestExecute_ServerReturns503(t *testing.T) {
	mockServer := createMockMDEMGServer(t, http.StatusServiceUnavailable, nil, "service unavailable")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "error-503-test",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute should not return Go error: %v", err)
	}
	if resp.Success {
		t.Error("Success should be false when server returns 503")
	}
	if resp.Error == "" {
		t.Error("Error message should not be empty")
	}
}

func TestProcessGraduation_Success(t *testing.T) {
	summary := &GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         time.Now(),
		Graduated:         10,
		Tombstoned:        5,
		RemainingVolatile: 20,
		DecayApplied:      3,
	}

	mockServer := createMockMDEMGServer(t, http.StatusOK, summary, "")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	result, err := s.processGraduation(context.Background(), "test-space")

	if err != nil {
		t.Fatalf("processGraduation returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Result should not be nil")
	}
	if result.Graduated != 10 {
		t.Errorf("Graduated = %d, want 10", result.Graduated)
	}
	if result.Tombstoned != 5 {
		t.Errorf("Tombstoned = %d, want 5", result.Tombstoned)
	}
	if result.RemainingVolatile != 20 {
		t.Errorf("RemainingVolatile = %d, want 20", result.RemainingVolatile)
	}
	if result.DecayApplied != 3 {
		t.Errorf("DecayApplied = %d, want 3", result.DecayApplied)
	}
	if result.SpaceID != "test-space" {
		t.Errorf("SpaceID = %s, want test-space", result.SpaceID)
	}
}

func TestProcessGraduation_ErrorStatus(t *testing.T) {
	mockServer := createMockMDEMGServer(t, http.StatusInternalServerError, nil, "graduation service error")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	result, err := s.processGraduation(context.Background(), "test-space")

	if err == nil {
		t.Fatal("processGraduation should return error on non-200 status")
	}
	if result != nil {
		t.Error("Result should be nil on error")
	}
}

func TestProcessGraduation_InvalidJSON(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json{{{"))
	}))
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	result, err := s.processGraduation(context.Background(), "test-space")

	if err == nil {
		t.Fatal("processGraduation should return error on invalid JSON")
	}
	if result != nil {
		t.Error("Result should be nil on error")
	}
}

func TestProcessGraduation_EmptyResponse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return empty JSON object - valid but with zero values
		w.Write([]byte("{}"))
	}))
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	result, err := s.processGraduation(context.Background(), "test-space")

	if err != nil {
		t.Fatalf("processGraduation should succeed with empty JSON: %v", err)
	}
	if result == nil {
		t.Fatal("Result should not be nil")
	}
	// All values should be zero/empty
	if result.Graduated != 0 {
		t.Errorf("Graduated = %d, want 0", result.Graduated)
	}
	if result.Tombstoned != 0 {
		t.Errorf("Tombstoned = %d, want 0", result.Tombstoned)
	}
}

func TestProcessGraduation_ContextCanceled(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := s.processGraduation(ctx, "test-space")

	if err == nil {
		t.Fatal("processGraduation should return error when context is canceled")
	}
	if result != nil {
		t.Error("Result should be nil on error")
	}
}

func TestExecute_ZeroGraduations(t *testing.T) {
	summary := &GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         time.Now(),
		Graduated:         0,
		Tombstoned:        0,
		RemainingVolatile: 0,
		DecayApplied:      0,
	}

	mockServer := createMockMDEMGServer(t, http.StatusOK, summary, "")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "zero-graduation-test",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if !resp.Success {
		t.Errorf("Success = false, want true; Error: %s", resp.Error)
	}
	if resp.Stats.NodesUpdated != 0 {
		t.Errorf("NodesUpdated = %d, want 0", resp.Stats.NodesUpdated)
	}
}

func TestExecute_LargeNumbers(t *testing.T) {
	summary := &GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         time.Now(),
		Graduated:         10000,
		Tombstoned:        5000,
		RemainingVolatile: 50000,
		DecayApplied:      1000,
	}

	mockServer := createMockMDEMGServer(t, http.StatusOK, summary, "")
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := s.Execute(context.Background(), &pb.ExecuteRequest{
		TaskId:  "large-numbers-test",
		Trigger: "schedule",
		Context: map[string]string{"space_id": "test-space"},
	})

	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if !resp.Success {
		t.Errorf("Success = false, want true; Error: %s", resp.Error)
	}
	if resp.Stats.NodesUpdated != 15000 { // 10000 + 5000
		t.Errorf("NodesUpdated = %d, want 15000", resp.Stats.NodesUpdated)
	}
	if s.lastGraduated != 10000 {
		t.Errorf("lastGraduated = %d, want 10000", s.lastGraduated)
	}
	if s.lastTombstoned != 5000 {
		t.Errorf("lastTombstoned = %d, want 5000", s.lastTombstoned)
	}
}

func TestProcessGraduation_InvalidURL(t *testing.T) {
	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: "://invalid-url", // Invalid URL scheme
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	result, err := s.processGraduation(context.Background(), "test-space")

	if err == nil {
		t.Fatal("processGraduation should return error for invalid URL")
	}
	if result != nil {
		t.Error("Result should be nil on error")
	}
}

func TestProcessGraduation_NilContext(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&GraduateSummary{})
	}))
	defer mockServer.Close()

	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mockServer.URL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	// Note: passing nil context should cause http.NewRequestWithContext to fail
	// However, in Go 1.13+, nil context is handled by the http package
	// This test ensures the code path is exercised
	result, err := s.processGraduation(nil, "test-space")

	// Behavior depends on Go version - nil context may panic or return error
	// We just want to ensure the code handles it without panicking
	_ = result
	_ = err
}

// =============================================================================
// DefaultConfig tests
// =============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MDEMGEndpoint != defaultMDEMGEndpoint {
		t.Errorf("DefaultConfig().MDEMGEndpoint = %s, want %s", cfg.MDEMGEndpoint, defaultMDEMGEndpoint)
	}
	if cfg.HTTPTimeout != 30*time.Second {
		t.Errorf("DefaultConfig().HTTPTimeout = %v, want %v", cfg.HTTPTimeout, 30*time.Second)
	}
	if cfg.SocketPath != "" {
		t.Errorf("DefaultConfig().SocketPath = %s, want empty", cfg.SocketPath)
	}
}

// =============================================================================
// newServer tests
// =============================================================================

func TestNewServer_WithConfig(t *testing.T) {
	cfg := Config{
		MDEMGEndpoint: "http://custom:8080",
		HTTPTimeout:   60 * time.Second,
	}

	s := newServer(cfg)

	if s == nil {
		t.Fatal("newServer returned nil")
	}
	if s.mdemgEndpoint != "http://custom:8080" {
		t.Errorf("newServer.mdemgEndpoint = %s, want http://custom:8080", s.mdemgEndpoint)
	}
	if s.httpClient.Timeout != 60*time.Second {
		t.Errorf("newServer.httpClient.Timeout = %v, want 60s", s.httpClient.Timeout)
	}
	if s.startTime.IsZero() {
		t.Error("newServer.startTime should not be zero")
	}
}

func TestNewServer_DefaultEndpoint(t *testing.T) {
	cfg := Config{
		MDEMGEndpoint: "", // Empty, should use default
	}

	s := newServer(cfg)

	if s.mdemgEndpoint != defaultMDEMGEndpoint {
		t.Errorf("newServer.mdemgEndpoint = %s, want %s", s.mdemgEndpoint, defaultMDEMGEndpoint)
	}
}

func TestNewServer_DefaultTimeout(t *testing.T) {
	cfg := Config{
		HTTPTimeout: 0, // Zero, should use default
	}

	s := newServer(cfg)

	if s.httpClient.Timeout != 30*time.Second {
		t.Errorf("newServer.httpClient.Timeout = %v, want 30s", s.httpClient.Timeout)
	}
}

// =============================================================================
// run function tests
// =============================================================================

func TestRun_EmptySocketPath(t *testing.T) {
	cfg := Config{
		SocketPath: "",
	}

	stopCh := make(chan struct{})
	close(stopCh) // Immediately close to prevent blocking

	err := run(cfg, stopCh)

	if err == nil {
		t.Fatal("run() should return error with empty socket path")
	}
	if err.Error() != "socket path is required" {
		t.Errorf("run() error = %v, want 'socket path is required'", err)
	}
}

func TestRun_InvalidSocketPath(t *testing.T) {
	cfg := Config{
		SocketPath: "/nonexistent/directory/socket.sock",
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	err := run(cfg, stopCh)

	if err == nil {
		t.Fatal("run() should return error with invalid socket path")
	}
	// Error should mention socket creation failure
	if err.Error() == "" {
		t.Error("run() should return non-empty error")
	}
}

func TestRun_SuccessfulShutdown(t *testing.T) {
	// Use /tmp for Unix socket - t.TempDir() path can be too long on macOS
	socketPath := "/tmp/context_cooler_shutdown_test_" + time.Now().Format("20060102150405") + ".sock"

	// Ensure socket doesn't exist
	os.Remove(socketPath)

	cfg := Config{
		SocketPath:    socketPath,
		MDEMGEndpoint: "http://localhost:59999",
		HTTPTimeout:   1 * time.Second,
	}

	stopCh := make(chan struct{})

	// Start run in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(cfg, stopCh)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Send stop signal
	close(stopCh)

	// Wait for run to complete with timeout
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run() returned error on graceful shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not complete in time")
	}
}

// =============================================================================
// Config struct tests
// =============================================================================

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		SocketPath:    "/path/to/socket.sock",
		MDEMGEndpoint: "http://example.com:9999",
		HTTPTimeout:   45 * time.Second,
	}

	if cfg.SocketPath != "/path/to/socket.sock" {
		t.Errorf("Config.SocketPath = %s, want /path/to/socket.sock", cfg.SocketPath)
	}
	if cfg.MDEMGEndpoint != "http://example.com:9999" {
		t.Errorf("Config.MDEMGEndpoint = %s, want http://example.com:9999", cfg.MDEMGEndpoint)
	}
	if cfg.HTTPTimeout != 45*time.Second {
		t.Errorf("Config.HTTPTimeout = %v, want 45s", cfg.HTTPTimeout)
	}
}

// =============================================================================
// Constants tests
// =============================================================================

func TestConstants(t *testing.T) {
	if moduleID != "context-cooler" {
		t.Errorf("moduleID = %s, want context-cooler", moduleID)
	}
	if moduleVersion != "1.0.0" {
		t.Errorf("moduleVersion = %s, want 1.0.0", moduleVersion)
	}
	if defaultMDEMGEndpoint != "http://localhost:9999" {
		t.Errorf("defaultMDEMGEndpoint = %s, want http://localhost:9999", defaultMDEMGEndpoint)
	}
	if defaultCronExpression != "*/30 * * * *" {
		t.Errorf("defaultCronExpression = %s, want */30 * * * *", defaultCronExpression)
	}
	if defaultMinInterval != 300 {
		t.Errorf("defaultMinInterval = %d, want 300", defaultMinInterval)
	}
}

// =============================================================================
// Additional edge case tests for run()
// =============================================================================

func TestRun_SocketCreationAndCleanup(t *testing.T) {
	// Use /tmp for Unix socket - t.TempDir() path can be too long on macOS
	socketPath := "/tmp/context_cooler_test_" + time.Now().Format("20060102150405") + ".sock"

	// Ensure socket doesn't exist
	os.Remove(socketPath)

	cfg := Config{
		SocketPath:    socketPath,
		MDEMGEndpoint: "http://localhost:59999",
		HTTPTimeout:   1 * time.Second,
	}

	stopCh := make(chan struct{})

	// Start run in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(cfg, stopCh)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Verify socket was created
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("Socket file should exist after run starts")
	}

	// Send stop signal
	close(stopCh)

	// Wait for run to complete
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not complete in time")
	}

	// Verify socket was cleaned up
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket file should be cleaned up after run completes")
	}
}

// =============================================================================
// GraduateRequest and GraduateSummary struct tests
// =============================================================================

func TestGraduateRequest_JSON(t *testing.T) {
	req := GraduateRequest{SpaceID: "test-space"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal GraduateRequest: %v", err)
	}

	var decoded GraduateRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal GraduateRequest: %v", err)
	}

	if decoded.SpaceID != "test-space" {
		t.Errorf("Decoded SpaceID = %s, want test-space", decoded.SpaceID)
	}
}

func TestGraduateSummary_JSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	summary := GraduateSummary{
		SpaceID:           "test-space",
		Timestamp:         now,
		Graduated:         10,
		Tombstoned:        5,
		RemainingVolatile: 20,
		DecayApplied:      3,
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Failed to marshal GraduateSummary: %v", err)
	}

	var decoded GraduateSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal GraduateSummary: %v", err)
	}

	if decoded.SpaceID != "test-space" {
		t.Errorf("Decoded SpaceID = %s, want test-space", decoded.SpaceID)
	}
	if decoded.Graduated != 10 {
		t.Errorf("Decoded Graduated = %d, want 10", decoded.Graduated)
	}
	if decoded.Tombstoned != 5 {
		t.Errorf("Decoded Tombstoned = %d, want 5", decoded.Tombstoned)
	}
	if decoded.RemainingVolatile != 20 {
		t.Errorf("Decoded RemainingVolatile = %d, want 20", decoded.RemainingVolatile)
	}
	if decoded.DecayApplied != 3 {
		t.Errorf("Decoded DecayApplied = %d, want 3", decoded.DecayApplied)
	}
}
