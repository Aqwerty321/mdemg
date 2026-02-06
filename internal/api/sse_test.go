package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mdemg/internal/jobs"
)

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		status   jobs.JobStatus
		expected bool
	}{
		{jobs.StatusPending, false},
		{jobs.StatusRunning, false},
		{jobs.StatusCompleted, true},
		{jobs.StatusFailed, true},
		{jobs.StatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := isTerminalStatus(tt.status); got != tt.expected {
				t.Errorf("isTerminalStatus(%s) = %v, want %v", tt.status, got, tt.expected)
			}
		})
	}
}

func TestHandleJobStream_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/test-job/stream", nil)
	rr := httptest.NewRecorder()

	s.handleJobStream(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleJobStream_MissingJobID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs//stream", nil)
	rr := httptest.NewRecorder()

	s.handleJobStream(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleJobStream_JobNotFound(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/nonexistent-job-12345/stream", nil)
	rr := httptest.NewRecorder()

	s.handleJobStream(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandleJobStream_CompletedJob(t *testing.T) {
	// Create a job in the global queue
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-completed-job", "test", nil)
	job.Complete(map[string]any{"result": "ok"})

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-completed-job/stream", nil)
	rr := httptest.NewRecorder()

	// Run in goroutine since it will block until complete
	done := make(chan struct{})
	go func() {
		s.handleJobStream(rr, req)
		close(done)
	}()

	select {
	case <-done:
		// Check SSE headers
		if rr.Header().Get("Content-Type") != "text/event-stream" {
			t.Errorf("expected Content-Type text/event-stream, got %s", rr.Header().Get("Content-Type"))
		}

		// Check that we got events
		body := rr.Body.String()
		if !strings.Contains(body, "event: connected") {
			t.Error("expected 'connected' event")
		}
		if !strings.Contains(body, "event: complete") {
			t.Error("expected 'complete' event for terminal job")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler timed out")
	}
}

func TestHandleJobStream_RunningJobProgress(t *testing.T) {
	// Create a running job
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-running-job", "test", nil)
	queue.StartJob("test-running-job")
	job.SetTotal(100)
	job.UpdateProgress(0, "starting")

	s := &Server{}

	// Create a cancellable context for the request
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cleanup on all paths
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-running-job/stream", nil)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	// Start the handler
	done := make(chan struct{})
	go func() {
		s.handleJobStream(rr, req)
		close(done)
	}()

	// Give handler time to start and send initial event
	time.Sleep(100 * time.Millisecond)

	// Update progress to trigger an event
	job.UpdateProgress(50, "processing")

	// Wait for poll interval + some buffer
	time.Sleep(600 * time.Millisecond)

	// Complete the job to end the stream
	job.Complete(map[string]any{"result": "done"})

	select {
	case <-done:
		body := rr.Body.String()
		if !strings.Contains(body, "event: connected") {
			t.Error("expected 'connected' event")
		}
		// Should have progress event after update
		if !strings.Contains(body, "event: progress") {
			t.Error("expected 'progress' event")
		}
		if !strings.Contains(body, "event: complete") {
			t.Error("expected 'complete' event")
		}
	case <-time.After(3 * time.Second):
		t.Error("handler timed out waiting for completion")
	}
}

func TestHandleJobStream_ClientDisconnect(t *testing.T) {
	// Create a running job
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-disconnect-job", "test", nil)
	queue.StartJob("test-disconnect-job")
	job.SetTotal(100)

	s := &Server{}

	// Create a cancellable context to simulate client disconnect
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-disconnect-job/stream", nil)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleJobStream(rr, req)
		close(done)
	}()

	// Give handler time to start
	time.Sleep(100 * time.Millisecond)

	// Simulate client disconnect
	cancel()

	select {
	case <-done:
		// Handler should have exited
		body := rr.Body.String()
		if !strings.Contains(body, "event: connected") {
			t.Error("expected 'connected' event before disconnect")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler should have exited after client disconnect")
	}

	// Clean up
	job.Complete(nil)
}

func TestSendSSEEvent(t *testing.T) {
	rr := httptest.NewRecorder()
	flusher := rr // httptest.ResponseRecorder implements Flusher

	data := map[string]any{
		"job_id": "test-123",
		"status": "running",
	}

	sendSSEEvent(rr, flusher, "progress", data)

	body := rr.Body.String()

	// Check event line
	if !strings.Contains(body, "event: progress\n") {
		t.Errorf("expected 'event: progress', got %s", body)
	}

	// Check data line contains JSON
	if !strings.Contains(body, "data: {") {
		t.Errorf("expected JSON data, got %s", body)
	}
	if !strings.Contains(body, `"job_id":"test-123"`) {
		t.Errorf("expected job_id in data, got %s", body)
	}

	// Check double newline terminator
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("expected double newline terminator, got %q", body[len(body)-4:])
	}
}

func TestSendSSEEvent_MarshalError(t *testing.T) {
	rr := httptest.NewRecorder()
	flusher := rr

	// Create a value that can't be marshaled (channel)
	data := make(chan int)

	sendSSEEvent(rr, flusher, "error", data)

	body := rr.Body.String()

	// Should still send event with error message
	if !strings.Contains(body, "event: error\n") {
		t.Errorf("expected event even on marshal error, got %s", body)
	}
	if !strings.Contains(body, "marshal error") {
		t.Errorf("expected marshal error message, got %s", body)
	}
}

func TestHandleJobStream_SSEHeaders(t *testing.T) {
	// Create a completed job for quick exit
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-headers-job", "test", nil)
	job.Complete(nil)

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-headers-job/stream", nil)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleJobStream(rr, req)
		close(done)
	}()

	select {
	case <-done:
		// Verify SSE headers
		if rr.Header().Get("Content-Type") != "text/event-stream" {
			t.Errorf("expected Content-Type text/event-stream")
		}
		if rr.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("expected Cache-Control no-cache")
		}
		if rr.Header().Get("Connection") != "keep-alive" {
			t.Errorf("expected Connection keep-alive")
		}
		if rr.Header().Get("X-Accel-Buffering") != "no" {
			t.Errorf("expected X-Accel-Buffering no")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler timed out")
	}
}

func TestHandleJobStream_FailedJob(t *testing.T) {
	// Create a failed job
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-failed-job", "test", nil)
	job.Fail(context.DeadlineExceeded)

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-failed-job/stream", nil)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleJobStream(rr, req)
		close(done)
	}()

	select {
	case <-done:
		body := rr.Body.String()

		// Should have complete event with error
		if !strings.Contains(body, "event: complete") {
			t.Error("expected 'complete' event for failed job")
		}
		if !strings.Contains(body, "deadline exceeded") || !strings.Contains(body, "error") {
			t.Error("expected error in complete event")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler timed out")
	}
}

func TestHandleJobStream_EventFormat(t *testing.T) {
	// Create a completed job
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-format-job", "test", nil)
	job.Complete(map[string]any{"count": 42})

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-format-job/stream", nil)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleJobStream(rr, req)
		close(done)
	}()

	select {
	case <-done:
		// Parse SSE events
		scanner := bufio.NewScanner(strings.NewReader(rr.Body.String()))
		events := make(map[string]string)
		var currentEvent string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				currentEvent = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				events[currentEvent] = strings.TrimPrefix(line, "data: ")
			}
		}

		if _, ok := events["connected"]; !ok {
			t.Error("missing 'connected' event")
		}
		if _, ok := events["complete"]; !ok {
			t.Error("missing 'complete' event")
		}
		if completeData, ok := events["complete"]; ok {
			if !strings.Contains(completeData, `"final_status":"completed"`) {
				t.Errorf("expected final_status in complete event, got %s", completeData)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("handler timed out")
	}
}

// nonFlushingResponseWriter is a ResponseWriter that does NOT implement http.Flusher
type nonFlushingResponseWriter struct {
	header http.Header
	code   int
	body   strings.Builder
}

func newNonFlushingResponseWriter() *nonFlushingResponseWriter {
	return &nonFlushingResponseWriter{
		header: make(http.Header),
		code:   http.StatusOK,
	}
}

func (w *nonFlushingResponseWriter) Header() http.Header {
	return w.header
}

func (w *nonFlushingResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *nonFlushingResponseWriter) WriteHeader(code int) {
	w.code = code
}

func TestHandleJobStreamWithConfig_StreamingNotSupported(t *testing.T) {
	// Create a job
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-no-flusher-job", "test", nil)
	queue.StartJob("test-no-flusher-job")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-no-flusher-job/stream", nil)

	// Use our custom non-flushing writer
	w := newNonFlushingResponseWriter()

	cfg := sseConfig{
		timeout:      time.Minute,
		pollInterval: 100 * time.Millisecond,
		getJob:       queue.GetJob,
	}

	s.handleJobStreamWithConfig(w, req, cfg)

	// Should return 500 because streaming not supported
	if w.code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-flushing writer, got %d", w.code)
	}

	body := w.body.String()
	if !strings.Contains(body, "streaming not supported") {
		t.Errorf("expected 'streaming not supported' error, got %s", body)
	}

	// Clean up
	job.Complete(nil)
}

func TestHandleJobStreamWithConfig_Timeout(t *testing.T) {
	// Create a running job that won't complete
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-timeout-job", "test", nil)
	queue.StartJob("test-timeout-job")
	job.SetTotal(100)

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-timeout-job/stream", nil)
	rr := httptest.NewRecorder()

	// Use very short timeout for testing
	cfg := sseConfig{
		timeout:      50 * time.Millisecond,
		pollInterval: 10 * time.Millisecond,
		getJob:       queue.GetJob,
	}

	done := make(chan struct{})
	go func() {
		s.handleJobStreamWithConfig(rr, req, cfg)
		close(done)
	}()

	select {
	case <-done:
		body := rr.Body.String()
		if !strings.Contains(body, "event: connected") {
			t.Error("expected 'connected' event")
		}
		if !strings.Contains(body, "event: timeout") {
			t.Error("expected 'timeout' event")
		}
		if !strings.Contains(body, "connection timed out") {
			t.Error("expected timeout message")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler should have timed out quickly")
	}

	// Clean up
	job.Complete(nil)
}

func TestHandleJobStreamWithConfig_JobDeletedDuringStream(t *testing.T) {
	// Create a mock job getter that returns the job initially, then returns not found
	var mu sync.Mutex
	callCount := 0
	jobDeleted := false

	// Create a real job first
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-deleted-job", "test", nil)
	queue.StartJob("test-deleted-job")
	job.SetTotal(100)

	mockGetJob := func(id string) (*jobs.Job, bool) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		if jobDeleted {
			return nil, false
		}
		return queue.GetJob(id)
	}

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-deleted-job/stream", nil)
	rr := httptest.NewRecorder()

	cfg := sseConfig{
		timeout:      5 * time.Second,
		pollInterval: 50 * time.Millisecond,
		getJob:       mockGetJob,
	}

	done := make(chan struct{})
	go func() {
		s.handleJobStreamWithConfig(rr, req, cfg)
		close(done)
	}()

	// Give handler time to start and make initial call
	time.Sleep(100 * time.Millisecond)

	// "Delete" the job by marking it as deleted
	mu.Lock()
	jobDeleted = true
	mu.Unlock()

	select {
	case <-done:
		body := rr.Body.String()
		if !strings.Contains(body, "event: connected") {
			t.Error("expected 'connected' event")
		}
		if !strings.Contains(body, "event: error") {
			t.Error("expected 'error' event for deleted job")
		}
		if !strings.Contains(body, "job no longer exists") {
			t.Error("expected 'job no longer exists' message")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler should have exited after job deletion")
	}

	// Clean up
	job.Complete(nil)
}

func TestDefaultSSEConfig(t *testing.T) {
	cfg := defaultSSEConfig()

	if cfg.timeout != defaultSSETimeout {
		t.Errorf("expected timeout %v, got %v", defaultSSETimeout, cfg.timeout)
	}
	if cfg.pollInterval != defaultSSEPollInterval {
		t.Errorf("expected pollInterval %v, got %v", defaultSSEPollInterval, cfg.pollInterval)
	}
	if cfg.getJob == nil {
		t.Error("expected getJob to be set")
	}
}

func TestHandleJobStreamWithConfig_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/test-job/stream", nil)
	rr := httptest.NewRecorder()

	cfg := sseConfig{
		timeout:      time.Minute,
		pollInterval: 100 * time.Millisecond,
		getJob: func(id string) (*jobs.Job, bool) {
			return nil, false
		},
	}

	s.handleJobStreamWithConfig(rr, req, cfg)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleJobStreamWithConfig_MissingJobID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs//stream", nil)
	rr := httptest.NewRecorder()

	cfg := sseConfig{
		timeout:      time.Minute,
		pollInterval: 100 * time.Millisecond,
		getJob: func(id string) (*jobs.Job, bool) {
			return nil, false
		},
	}

	s.handleJobStreamWithConfig(rr, req, cfg)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleJobStreamWithConfig_JobNotFound(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/nonexistent/stream", nil)
	rr := httptest.NewRecorder()

	cfg := sseConfig{
		timeout:      time.Minute,
		pollInterval: 100 * time.Millisecond,
		getJob: func(id string) (*jobs.Job, bool) {
			return nil, false
		},
	}

	s.handleJobStreamWithConfig(rr, req, cfg)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandleJobStreamWithConfig_JobFailsDuringStream(t *testing.T) {
	// Create a running job that will fail during streaming
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-fail-during-stream", "test", nil)
	queue.StartJob("test-fail-during-stream")
	job.SetTotal(100)
	job.UpdateProgress(0, "starting")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-fail-during-stream/stream", nil)
	rr := httptest.NewRecorder()

	cfg := sseConfig{
		timeout:      5 * time.Second,
		pollInterval: 50 * time.Millisecond,
		getJob:       queue.GetJob,
	}

	done := make(chan struct{})
	go func() {
		s.handleJobStreamWithConfig(rr, req, cfg)
		close(done)
	}()

	// Give handler time to start
	time.Sleep(100 * time.Millisecond)

	// Update progress then fail the job - this triggers both progress and complete events with errors
	job.UpdateProgress(50, "processing")
	time.Sleep(100 * time.Millisecond)
	job.Fail(context.DeadlineExceeded)

	select {
	case <-done:
		body := rr.Body.String()
		if !strings.Contains(body, "event: connected") {
			t.Error("expected 'connected' event")
		}
		if !strings.Contains(body, "event: complete") {
			t.Error("expected 'complete' event")
		}
		// The error should appear in the complete event
		if !strings.Contains(body, "deadline exceeded") {
			t.Error("expected error message in output")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler timed out")
	}
}

func TestHandleJobStreamWithConfig_ProgressWithError(t *testing.T) {
	// Test job that has error field set during progress
	queue := jobs.GetQueue()
	job, _ := queue.CreateJob("test-progress-error", "test", nil)
	queue.StartJob("test-progress-error")
	job.SetTotal(100)

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/test-progress-error/stream", nil)
	rr := httptest.NewRecorder()

	cfg := sseConfig{
		timeout:      5 * time.Second,
		pollInterval: 50 * time.Millisecond,
		getJob:       queue.GetJob,
	}

	done := make(chan struct{})
	go func() {
		s.handleJobStreamWithConfig(rr, req, cfg)
		close(done)
	}()

	// Give handler time to start
	time.Sleep(100 * time.Millisecond)

	// Update progress - trigger a progress event
	job.UpdateProgress(25, "phase1")
	time.Sleep(100 * time.Millisecond)

	// Complete the job
	job.Complete(map[string]any{"success": true})

	select {
	case <-done:
		body := rr.Body.String()
		if !strings.Contains(body, "event: progress") {
			t.Error("expected 'progress' event")
		}
		if !strings.Contains(body, "event: complete") {
			t.Error("expected 'complete' event")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler timed out")
	}
}
