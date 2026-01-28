package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/anomaly"
	"mdemg/internal/ape"
	"mdemg/internal/config"
	"mdemg/internal/consulting"
	"mdemg/internal/conversation"
	"mdemg/internal/embeddings"
	"mdemg/internal/gaps"
	"mdemg/internal/hidden"
	"mdemg/internal/learning"
	"mdemg/internal/plugins"
	"mdemg/internal/retrieval"
	"mdemg/internal/symbols"
	"mdemg/internal/validation"
)

type Server struct {
	cfg             config.Config
	driver          neo4j.DriverWithContext
	retriever       *retrieval.Service
	learner         *learning.Service
	embedder        embeddings.Embedder
	anomalyDetector *anomaly.Service
	hiddenLayer     *hidden.Service
	pluginMgr       *plugins.Manager
	apeScheduler    *ape.Scheduler
	symbolStore     *symbols.Store
	consultant      *consulting.Service
	gapDetector     *gaps.GapDetector
	conversationSvc *conversation.Service
	contextCooler   *conversation.ContextCooler
	hiddenSvc       *hidden.Service // alias for handleConversationConsolidate
	stopConsolidate chan struct{}
	stopCooler      chan struct{}
}

func NewServer(cfg config.Config, driver neo4j.DriverWithContext, pluginMgr *plugins.Manager) *Server {
	ret := retrieval.NewService(cfg, driver)

	// Wire reasoning modules into retrieval pipeline
	if pluginMgr != nil {
		reasoningProvider := retrieval.NewPluginReasoningProvider(pluginMgr)
		ret.SetReasoningProvider(reasoningProvider)
	}

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

	// Initialize hidden layer service
	hid := hidden.NewService(cfg, driver)
	if cfg.HiddenLayerEnabled {
		log.Printf("Hidden layer enabled (eps: %.2f, minSamples: %d, maxHidden: %d)",
			cfg.HiddenLayerClusterEps, cfg.HiddenLayerMinSamples, cfg.HiddenLayerMaxHidden)
	}

	// Initialize symbol store
	symStore := symbols.NewStore(driver)
	log.Printf("Symbol store initialized")

	// Initialize consulting service (Agent Consulting API)
	cons := consulting.NewService(cfg, driver, ret, emb, symStore)
	log.Printf("Consulting service initialized")

	// Initialize gap detector for capability gap detection
	// Collect registered ingestion sources from plugins
	var registeredSources []string
	if pluginMgr != nil {
		for _, mod := range pluginMgr.GetIngestionModules() {
			registeredSources = append(registeredSources, mod.Manifest.Capabilities.IngestionSources...)
		}
	}
	gapCfg := gaps.DetectorConfig{
		LowScoreThreshold: cfg.GapLowScoreThreshold,
		MinOccurrences:    cfg.GapMinOccurrences,
		AnalysisWindow:    time.Duration(cfg.GapAnalysisWindowHours) * time.Hour,
		MetricsWindowSize: cfg.GapMetricsWindowSize,
		RegisteredSources: registeredSources,
	}
	gapDet := gaps.NewGapDetector(driver, gapCfg)
	log.Printf("Gap detector initialized (threshold: %.2f, minOccurrences: %d)", gapCfg.LowScoreThreshold, gapCfg.MinOccurrences)

	// Initialize conversation service (Phase 1: Observation Capture with Surprise Detection)
	var convSvc *conversation.Service
	var ctxCooler *conversation.ContextCooler
	if emb != nil {
		convSvc = conversation.NewServiceWithConfig(driver, emb, cfg.VectorIndexName)
		log.Printf("Conversation service initialized (vector index: %s)", cfg.VectorIndexName)

		// Initialize Context Cooler (Phase 3: Graduation logic for volatile observations)
		ctxCooler = conversation.NewContextCooler(driver)
		lea.SetStabilityReinforcer(ctxCooler)
		log.Printf("Context Cooler initialized (graduation threshold: %.2f, decay rate: %.2f)",
			conversation.GraduationStabilityThreshold, conversation.StabilityDecayRate)
	} else {
		log.Printf("Conversation service disabled (requires embedder)")
	}

	// Initialize APE scheduler
	var apeSched *ape.Scheduler
	if pluginMgr != nil {
		modules := pluginMgr.ListModules()
		log.Printf("Loaded %d plugin module(s)", len(modules))
		for _, m := range modules {
			log.Printf("  - %s (%s) [%s]", m.ID, m.Version, m.State)
		}

		// Start APE scheduler
		apeSched = ape.NewScheduler(pluginMgr)
		if err := apeSched.Start(); err != nil {
			log.Printf("WARNING: APE scheduler failed to start: %v", err)
		}
	}

	return &Server{cfg: cfg, driver: driver, retriever: ret, learner: lea, embedder: emb, anomalyDetector: anom, hiddenLayer: hid, hiddenSvc: hid, pluginMgr: pluginMgr, apeScheduler: apeSched, symbolStore: symStore, consultant: cons, gapDetector: gapDet, conversationSvc: convSvc, contextCooler: ctxCooler}
}

// Shutdown gracefully stops background services
func (s *Server) Shutdown() {
	if s.apeScheduler != nil {
		s.apeScheduler.Stop()
	}
	s.StopPeriodicConsolidation()
	s.StopContextCoolerProcessing()
}

// StartPeriodicConsolidation starts a background goroutine that consolidates conversation memory
// on a regular interval. Default interval is 5 minutes.
func (s *Server) StartPeriodicConsolidation(spaceID string, interval time.Duration) {
	if s.hiddenSvc == nil {
		log.Println("periodic consolidation disabled: hidden service not available")
		return
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	s.stopConsolidate = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("periodic conversation consolidation started (space=%s, interval=%v)", spaceID, interval)

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				result, err := s.hiddenSvc.RunFullConversationConsolidation(ctx, spaceID)
				cancel()
				if err != nil {
					log.Printf("periodic consolidation error: %v", err)
				} else {
					themesCreated := 0
					conceptsCreated := 0
					if result.ThemeResult != nil {
						themesCreated = result.ThemeResult.ThemesCreated
					}
					if result.ConceptResult != nil {
						for _, count := range result.ConceptResult.ConceptsCreated {
							conceptsCreated += count
						}
					}
					if themesCreated > 0 || conceptsCreated > 0 {
						log.Printf("periodic consolidation: %d themes, %d concepts created",
							themesCreated, conceptsCreated)
					}
				}
			case <-s.stopConsolidate:
				log.Println("periodic consolidation stopped")
				return
			}
		}
	}()
}

// StopPeriodicConsolidation stops the background consolidation goroutine
func (s *Server) StopPeriodicConsolidation() {
	if s.stopConsolidate != nil {
		close(s.stopConsolidate)
		s.stopConsolidate = nil
	}
}

// StartContextCoolerProcessing starts a background goroutine that processes
// Context Cooler graduations and decay. Default interval is 10 minutes.
func (s *Server) StartContextCoolerProcessing(spaceID string, interval time.Duration) {
	if s.contextCooler == nil {
		log.Println("Context Cooler processing disabled: cooler not available")
		return
	}
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	s.stopCooler = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("Context Cooler processing started (space=%s, interval=%v)", spaceID, interval)

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

				// Step 1: Apply decay to inactive volatile nodes
				decayed, err := s.contextCooler.ApplyDecay(ctx, spaceID)
				if err != nil {
					log.Printf("Context Cooler decay error: %v", err)
				}

				// Step 2: Process graduations and tombstones
				summary, err := s.contextCooler.ProcessGraduations(ctx, spaceID)
				cancel()

				if err != nil {
					log.Printf("Context Cooler graduation error: %v", err)
				} else if summary.Graduated > 0 || summary.Tombstoned > 0 || decayed > 0 {
					log.Printf("Context Cooler: graduated=%d, tombstoned=%d, decayed=%d, remaining_volatile=%d",
						summary.Graduated, summary.Tombstoned, decayed, summary.RemainingVolatile)
				}
			case <-s.stopCooler:
				log.Println("Context Cooler processing stopped")
				return
			}
		}
	}()
}

// StopContextCoolerProcessing stops the background Context Cooler goroutine
func (s *Server) StopContextCoolerProcessing() {
	if s.stopCooler != nil {
		close(s.stopCooler)
		s.stopCooler = nil
	}
}

// TriggerAPEEvent triggers APE modules subscribed to the given event
func (s *Server) TriggerAPEEvent(event string) {
	if s.apeScheduler != nil {
		s.apeScheduler.TriggerEvent(event)
	}
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
	mux.HandleFunc("/v1/memory/consolidate", s.handleConsolidate)
	mux.HandleFunc("/v1/modules", s.handleModules)
	mux.HandleFunc("/v1/modules/", s.handleModuleSync)
	mux.HandleFunc("/v1/plugins", s.handlePluginOperation)
	mux.HandleFunc("/v1/plugins/", s.handlePluginOperation)
	mux.HandleFunc("/v1/ape/status", s.handleAPEStatus)
	mux.HandleFunc("/v1/ape/trigger", s.handleAPETrigger)
	mux.HandleFunc("/v1/learning/prune", s.handleLearningPrune)
	mux.HandleFunc("/v1/learning/stats", s.handleLearningStats)
	mux.HandleFunc("/v1/learning/freeze", s.handleLearningFreeze)
	mux.HandleFunc("/v1/learning/unfreeze", s.handleLearningUnfreeze)
	mux.HandleFunc("/v1/learning/freeze/status", s.handleLearningFreezeStatus)
	mux.HandleFunc("/v1/memory/consult", s.handleConsult)
	mux.HandleFunc("/v1/memory/suggest", s.handleSuggest)
	mux.HandleFunc("/v1/memory/cache/stats", s.handleCacheStats)
	mux.HandleFunc("/v1/memory/query/metrics", s.handleQueryMetrics)
	mux.HandleFunc("/v1/memory/distribution", s.handleDistributionStats)
	mux.HandleFunc("/v1/memory/symbols", s.handleSymbolSearch)

	// Ingestion job management endpoints
	mux.HandleFunc("/v1/memory/ingest/trigger", s.handleIngestTrigger)
	mux.HandleFunc("/v1/memory/ingest/status/", s.handleIngestStatus)
	mux.HandleFunc("/v1/memory/ingest/cancel/", s.handleIngestCancel)
	mux.HandleFunc("/v1/memory/ingest/jobs", s.handleIngestJobs)

	// Capability gap detection endpoints
	mux.HandleFunc("/v1/system/capability-gaps", s.handleCapabilityGaps)
	mux.HandleFunc("/v1/system/capability-gaps/", s.handleCapabilityGapOperation)
	mux.HandleFunc("/v1/feedback", s.handleFeedback)

	// System metrics endpoints
	mux.HandleFunc("/v1/system/pool-metrics", s.handlePoolMetrics)

	// Conversation memory endpoints (Phase 1-5: Observation Capture, Resume, Recall)
	mux.HandleFunc("/v1/conversation/observe", s.handleObserve)
	mux.HandleFunc("/v1/conversation/correct", s.handleCorrect)
	mux.HandleFunc("/v1/conversation/resume", s.handleResume)
	mux.HandleFunc("/v1/conversation/recall", s.handleRecall)
	mux.HandleFunc("/v1/conversation/consolidate", s.handleConversationConsolidate)
	mux.HandleFunc("/v1/conversation/volatile/stats", s.handleVolatileStats)
	mux.HandleFunc("/v1/conversation/graduate", s.handleProcessGraduations)

	// Wrap mux with middleware stack
	// Order: compression (outermost) -> logging (innermost)
	logCfg := LogConfig{
		Format:     s.cfg.LogFormat,
		SkipHealth: s.cfg.LogSkipHealth,
	}

	handler := LoggingMiddleware(mux, logCfg)

	// Enable gzip compression for responses > 1KB when CompressionEnabled
	if s.cfg.CompressionEnabled {
		handler = CompressionMiddleware(handler, s.cfg.CompressionMinSize)
	}

	return handler
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

// sanitizeError logs the detailed error for debugging but returns a generic
// message suitable for client responses. This prevents internal details
// (stack traces, file paths, database errors) from leaking to clients.
func sanitizeError(err error, operation string) string {
	// Log the full error for debugging
	log.Printf("ERROR [%s]: %v", operation, err)
	// Return generic message to client
	return "internal error during " + operation
}

// writeInternalError writes a sanitized internal server error response.
// The detailed error is logged but not exposed to the client.
func writeInternalError(w http.ResponseWriter, err error, operation string) {
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error": sanitizeError(err, operation),
	})
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
