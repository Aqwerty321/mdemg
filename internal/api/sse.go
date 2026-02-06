package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mdemg/internal/jobs"
)

const (
	// SSE connection timeout (default)
	defaultSSETimeout = 5 * time.Minute
	// Polling interval for job status (default)
	defaultSSEPollInterval = 500 * time.Millisecond
)

// sseConfig holds configurable parameters for SSE streaming.
// Used internally for testing with shorter timeouts.
type sseConfig struct {
	timeout      time.Duration
	pollInterval time.Duration
	getJob       func(id string) (*jobs.Job, bool) // injectable job getter for testing
}

// defaultSSEConfig returns the default SSE configuration.
func defaultSSEConfig() sseConfig {
	queue := jobs.GetQueue()
	return sseConfig{
		timeout:      defaultSSETimeout,
		pollInterval: defaultSSEPollInterval,
		getJob:       queue.GetJob,
	}
}

// handleJobStream provides Server-Sent Events (SSE) streaming for job progress.
// GET /v1/jobs/{job_id}/stream
func (s *Server) handleJobStream(w http.ResponseWriter, r *http.Request) {
	s.handleJobStreamWithConfig(w, r, defaultSSEConfig())
}

// handleJobStreamWithConfig is the internal implementation with injectable config.
func (s *Server) handleJobStreamWithConfig(w http.ResponseWriter, r *http.Request, cfg sseConfig) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path: /v1/jobs/{job_id}/stream
	path := r.URL.Path
	jobID := strings.TrimPrefix(path, "/v1/jobs/")
	jobID = strings.TrimSuffix(jobID, "/stream")

	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "job_id is required"})
		return
	}

	// Check if job exists
	job, exists := cfg.getJob(jobID)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "job not found"})
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Ensure writer supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming not supported"})
		return
	}

	// Send initial event
	sendSSEEvent(w, flusher, "connected", map[string]any{
		"job_id": jobID,
		"status": job.Status,
	})

	// If job is already in terminal state, send complete and exit immediately
	if isTerminalStatus(job.Status) {
		completeData := map[string]any{
			"job_id":       jobID,
			"final_status": job.Status,
		}
		if job.Result != nil {
			completeData["result"] = job.Result
		}
		if job.Error != "" {
			completeData["error"] = job.Error
		}
		sendSSEEvent(w, flusher, "complete", completeData)
		return
	}

	// Create timeout context
	ctx := r.Context()
	timeout := time.After(cfg.timeout)
	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()

	lastStatus := job.Status
	lastProgress := job.Progress.Current

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			sendSSEEvent(w, flusher, "disconnected", map[string]any{
				"reason": "client_closed",
			})
			return

		case <-timeout:
			// Connection timeout
			sendSSEEvent(w, flusher, "timeout", map[string]any{
				"message": "connection timed out after 5 minutes",
			})
			return

		case <-ticker.C:
			// Poll job status
			job, exists = cfg.getJob(jobID)
			if !exists {
				sendSSEEvent(w, flusher, "error", map[string]any{
					"message": "job no longer exists",
				})
				return
			}

			// Only send updates when something changes
			if job.Status != lastStatus || job.Progress.Current != lastProgress {
				lastStatus = job.Status
				lastProgress = job.Progress.Current

				eventData := map[string]any{
					"job_id":  jobID,
					"status":  job.Status,
					"progress": map[string]any{
						"total":      job.Progress.Total,
						"current":    job.Progress.Current,
						"percentage": job.Progress.Percentage,
						"phase":      job.Progress.Phase,
						"rate":       job.Progress.Rate,
					},
				}

				if job.Error != "" {
					eventData["error"] = job.Error
				}

				sendSSEEvent(w, flusher, "progress", eventData)

				// Send complete event for terminal statuses
				if isTerminalStatus(job.Status) {
					completeData := map[string]any{
						"job_id":       jobID,
						"final_status": job.Status,
					}
					if job.Result != nil {
						completeData["result"] = job.Result
					}
					if job.Error != "" {
						completeData["error"] = job.Error
					}
					sendSSEEvent(w, flusher, "complete", completeData)
					return
				}
			}
		}
	}
}

// sendSSEEvent writes a Server-Sent Event to the response.
func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		jsonData = []byte(fmt.Sprintf(`{"error":"marshal error: %v"}`, err))
	}

	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

// isTerminalStatus returns true if the job status is a terminal state.
func isTerminalStatus(status jobs.JobStatus) bool {
	return status == jobs.StatusCompleted || status == jobs.StatusFailed || status == jobs.StatusCancelled
}
