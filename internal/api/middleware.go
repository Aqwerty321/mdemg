package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// LogConfig holds configuration for request logging middleware.
type LogConfig struct {
	Format     string // "text" (default) or "json"
	SkipHealth bool   // Skip logging for /healthz and /readyz endpoints
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// WriteHeader captures the status code and delegates to the wrapped ResponseWriter.
func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

// Write ensures status is set before writing body (implicit 200 OK).
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// generateRequestID creates a unique request ID using crypto/rand.
func generateRequestID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return hex.EncodeToString([]byte(time.Now().String()[:16]))
	}
	return hex.EncodeToString(b)
}

// isHealthEndpoint returns true if the path is a health check endpoint.
func isHealthEndpoint(path string) bool {
	return path == "/healthz" || path == "/readyz"
}

// logEntry represents a structured log entry for JSON format.
type logEntry struct {
	Timestamp string `json:"timestamp"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	Duration  int64  `json:"duration_ms"`
	RequestID string `json:"request_id"`
}

// LoggingMiddleware returns middleware that logs HTTP requests.
func LoggingMiddleware(next http.Handler, cfg LogConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate and set request ID
		requestID := generateRequestID()
		w.Header().Set("X-Request-ID", requestID)

		// Wrap response writer to capture status
		wrapped := &responseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		// Serve the request
		next.ServeHTTP(wrapped, r)

		// Skip logging for health endpoints if configured
		if cfg.SkipHealth && isHealthEndpoint(r.URL.Path) {
			return
		}

		duration := time.Since(start)

		// Log based on format
		if strings.ToLower(cfg.Format) == "json" {
			entry := logEntry{
				Timestamp: start.UTC().Format(time.RFC3339),
				Method:    r.Method,
				Path:      r.URL.Path,
				Status:    wrapped.status,
				Duration:  duration.Milliseconds(),
				RequestID: requestID,
			}
			b, err := json.Marshal(entry)
			if err == nil {
				log.Println(string(b))
			}
		} else {
			// Default text format
			log.Printf("method=%s path=%s status=%d duration=%dms request_id=%s",
				r.Method, r.URL.Path, wrapped.status, duration.Milliseconds(), requestID)
		}
	})
}
