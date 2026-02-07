package main

import (
	"context"
	"net/http"
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
