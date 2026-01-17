package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/anomaly"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/learning"
	"mdemg/internal/retrieval"
	"mdemg/internal/validation"
)

type Server struct {
	cfg             config.Config
	driver          neo4j.DriverWithContext
	retriever       *retrieval.Service
	learner         *learning.Service
	embedder        embeddings.Embedder
	anomalyDetector *anomaly.Service
}

func NewServer(cfg config.Config, driver neo4j.DriverWithContext) *Server {
	ret := retrieval.NewService(cfg, driver)
	lea := learning.NewService(cfg, driver)

	// Initialize embedder (optional - nil if not configured)
	var emb embeddings.Embedder
	if cfg.EmbeddingProvider != "" {
		embCfg := embeddings.Config{
			Provider:       cfg.EmbeddingProvider,
			OpenAIAPIKey:   cfg.OpenAIAPIKey,
			OpenAIModel:    cfg.OpenAIModel,
			OpenAIEndpoint: cfg.OpenAIEndpoint,
			OllamaEndpoint: cfg.OllamaEndpoint,
			OllamaModel:    cfg.OllamaModel,
			CacheEnabled:   cfg.EmbeddingCacheEnabled,
			CacheSize:      cfg.EmbeddingCacheSize,
		}
		var err error
		emb, err = embeddings.New(embCfg)
		if err != nil {
			log.Printf("WARNING: embedding provider %q failed to initialize: %v", cfg.EmbeddingProvider, err)
		} else {
			log.Printf("Embedding provider initialized: %s (dimensions: %d)", emb.Name(), emb.Dimensions())
		}
	} else {
		log.Printf("No embedding provider configured (set EMBEDDING_PROVIDER=openai or ollama)")
	}

	// Initialize anomaly detector
	anomalyCfg := anomaly.Config{
		Enabled:            cfg.AnomalyDetectionEnabled,
		DuplicateThreshold: cfg.AnomalyDuplicateThreshold,
		OutlierStdDevs:     cfg.AnomalyOutlierStdDevs,
		StaleDays:          cfg.AnomalyStaleDays,
		MaxCheckMs:         cfg.AnomalyMaxCheckMs,
		VectorIndexName:    cfg.VectorIndexName,
	}
	anom := anomaly.NewService(driver, anomalyCfg)
	if anomalyCfg.Enabled {
		log.Printf("Anomaly detection enabled (duplicate threshold: %.2f, timeout: %dms)", anomalyCfg.DuplicateThreshold, anomalyCfg.MaxCheckMs)
	}

	return &Server{cfg: cfg, driver: driver, retriever: ret, learner: lea, embedder: emb, anomalyDetector: anom}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/v1/memory/retrieve", s.handleRetrieve)
	mux.HandleFunc("/v1/memory/ingest", s.handleIngest)
	mux.HandleFunc("/v1/memory/ingest/batch", s.handleBatchIngest)
	mux.HandleFunc("/v1/memory/reflect", s.handleReflect)
	mux.HandleFunc("/v1/memory/stats", s.handleStats)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/memory/archive/bulk", s.handleBulkArchive)
	mux.HandleFunc("/v1/memory/nodes/", s.handleNodeOperation)

	// Wrap mux with logging middleware
	logCfg := LogConfig{
		Format:     s.cfg.LogFormat,
		SkipHealth: s.cfg.LogSkipHealth,
	}
	return LoggingMiddleware(mux, logCfg)
}

// handleNodeOperation routes requests under /v1/memory/nodes/{node_id}/... to the appropriate handler
// based on the path suffix and HTTP method:
//   - POST /v1/memory/nodes/{node_id}/archive   -> handleArchiveNode
//   - POST /v1/memory/nodes/{node_id}/unarchive -> handleUnarchiveNode
//   - DELETE /v1/memory/nodes/{node_id}         -> handleDeleteNode
func (s *Server) handleNodeOperation(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/archive"):
		s.handleArchiveNode(w, r)
	case strings.HasSuffix(path, "/unarchive"):
		s.handleUnarchiveNode(w, r)
	default:
		// DELETE /v1/memory/nodes/{node_id} - permanent deletion
		s.handleDeleteNode(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return false
	}
	return true
}

// validateRequest validates a request struct using the validation package.
// Returns false and writes an error response if validation fails.
// Use after readJSON with the same pattern: if !validateRequest(w, &req) { return }
func validateRequest(w http.ResponseWriter, v any) bool {
	if err := validation.Validate(v); err != nil {
		resp := validation.FormatValidationErrors(err)
		writeJSON(w, http.StatusBadRequest, resp)
		return false
	}
	return true
}
