package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/anomaly"
	"mdemg/internal/ape"
	"mdemg/internal/auth"
	"mdemg/internal/backpressure"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/config"
	"mdemg/internal/consulting"
	"mdemg/internal/conversation"
	"mdemg/internal/embeddings"
	"mdemg/internal/filewatcher"
	"mdemg/internal/gaps"
	"mdemg/internal/hidden"
	"mdemg/internal/jobs"
	"mdemg/internal/learning"
	"mdemg/internal/metrics"
	"mdemg/internal/plugins"
	"mdemg/internal/ratelimit"
	"mdemg/internal/retrieval"
	"mdemg/internal/models"
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
	gapInterviewer  *gaps.GapInterviewer
	conversationSvc *conversation.Service
	contextCooler   *conversation.ContextCooler
	sessionTracker  *conversation.SessionTracker
	hiddenSvc       *hidden.Service // alias for handleConversationConsolidate
	webhookDebouncer        *linearWebhookDebouncer
	genericWebhookDebouncer *webhookDebouncer
	fileWatcherMgr          *filewatcher.Manager
	stopConsolidate         chan struct{}
	stopCooler         chan struct{}
	stopInterviewer    chan struct{}
	stopScheduledSync  chan struct{}

	// Phase 3: Production readiness components
	cbRegistry     *circuitbreaker.Registry
	metricsRegistry *metrics.Registry

	// Phase 48.4: Connection pooling components
	memoryPressure *backpressure.MemoryPressure

	// Phase 60: CMS Advanced II
	templateService  *conversation.TemplateService
	snapshotService  *conversation.SnapshotService
	orgReviewService *conversation.OrgReviewService

	// Phase 60b: RSIC (Recursive Self-Improvement Cycle)
	rsicCycle    *ape.CycleOrchestrator
	rsicWatchdog *ape.Watchdog
	stopRSIC     chan struct{}
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

	// Initialize gap interviewer for weekly gap interview processing
	gapInt := gaps.NewGapInterviewer(driver)
	log.Printf("Gap interviewer initialized")

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

	// Initialize session tracker (CMS enforcement — Phase 3A)
	sessTracker := conversation.NewSessionTracker(2 * time.Hour)
	log.Printf("Session tracker initialized (TTL: 2h)")

	// Phase 3: Initialize circuit breaker registry
	cbCfg := circuitbreaker.Config{
		Enabled:          cfg.CircuitBreakerEnabled,
		FailureThreshold: cfg.CircuitBreakerThreshold,
		SuccessThreshold: 2,
		Timeout:          time.Duration(cfg.CircuitBreakerTimeoutSec) * time.Second,
		MaxConcurrent:    1,
	}
	cbRegistry := circuitbreaker.NewRegistry(cbCfg)
	if cfg.CircuitBreakerEnabled {
		log.Printf("Circuit breaker enabled (threshold: %d, timeout: %ds)",
			cfg.CircuitBreakerThreshold, cfg.CircuitBreakerTimeoutSec)
	}

	// Wire circuit breaker registry to services that make external API calls
	ret.SetCircuitBreakerRegistry(cbRegistry)

	// Wire circuit breaker to embedder if it supports it (OpenAI and Ollama)
	if emb != nil {
		if openAIEmb, ok := emb.(*embeddings.OpenAI); ok {
			openAIEmb.SetCircuitBreaker(cbRegistry.Get("openai-embeddings"))
			log.Printf("Circuit breaker wired to OpenAI embedder")
		} else if ollamaEmb, ok := emb.(*embeddings.Ollama); ok {
			ollamaEmb.SetCircuitBreaker(cbRegistry.Get("ollama-embeddings"))
			log.Printf("Circuit breaker wired to Ollama embedder")
		}

		// Wrap embedder with rate limiting if enabled (Phase 48.4.3)
		if cfg.EmbeddingRateLimitEnabled {
			var rps float64
			var burst int
			if cfg.EmbeddingProvider == "openai" {
				rps = cfg.EmbeddingOpenAIRPS
				burst = cfg.EmbeddingOpenAIBurst
			} else {
				rps = cfg.EmbeddingOllamaRPS
				burst = cfg.EmbeddingOllamaBurst
			}
			emb = embeddings.NewRateLimitedEmbedder(emb, rps, burst, true)
			log.Printf("Embedding rate limiting enabled (%.0f rps, burst: %d)", rps, burst)
		}
	}

	// Phase 3: Initialize metrics registry
	// Start with defaults (includes histogram buckets) and override specific fields
	metricsCfg := metrics.DefaultConfig()
	metricsCfg.Enabled = cfg.MetricsEnabled
	if cfg.MetricsNamespace != "" {
		metricsCfg.Namespace = cfg.MetricsNamespace
	}
	metricsRegistry := metrics.NewRegistry(metricsCfg)
	metrics.SetGlobalRegistry(metricsRegistry)
	if cfg.MetricsEnabled {
		metrics.InitStandardMetrics()
		log.Printf("Prometheus metrics enabled (namespace: %s)", cfg.MetricsNamespace)
	}

	// Phase 48.4.4: Initialize memory pressure monitor
	memPressure := backpressure.NewMemoryPressure(uint64(cfg.MemoryPressureThresholdMB), cfg.MemoryPressureEnabled)
	if cfg.MemoryPressureEnabled {
		log.Printf("Memory pressure monitoring enabled (threshold: %dMB)", cfg.MemoryPressureThresholdMB)
	}

	// Phase 60b: Initialize RSIC components
	var rsicCycle *ape.CycleOrchestrator
	var rsicWatchdog *ape.Watchdog

	// Create adapters for RSIC interfaces
	learnerAdapter := &rsicLearningAdapter{svc: lea}
	var convAdapter *rsicConvAdapter
	if ctxCooler != nil {
		convAdapter = &rsicConvAdapter{cooler: ctxCooler}
	}
	hiddenAdapter := &rsicHiddenAdapter{svc: hid}

	rsicAssessor := ape.NewAssessor(cfg, driver, learnerAdapter, convAdapter)
	rsicReflector := ape.NewReflector(cfg, driver)
	rsicPlanner := ape.NewPlanner(cfg)
	rsicDispatcher := ape.NewDispatcher(driver, learnerAdapter, convAdapter, hiddenAdapter)
	rsicMonitor := ape.NewMonitor(rsicDispatcher)
	rsicCalibrator := ape.NewCalibrator(convAdapter)

	// Watchdog and cycle orchestrator (watchdog trigger wired after cycle creation)
	rsicWatchdog = ape.NewWatchdog(cfg, "mdemg-dev", nil)
	rsicCycle = ape.NewCycleOrchestrator(cfg, rsicAssessor, rsicReflector, rsicPlanner, rsicDispatcher, rsicMonitor, rsicCalibrator, rsicWatchdog)
	// Wire the watchdog's force-trigger to the cycle orchestrator
	rsicWatchdog = ape.NewWatchdog(cfg, "mdemg-dev", func(ctx context.Context, spaceID string) {
		_, _ = rsicCycle.RunCycle(ctx, spaceID, ape.TierMeso)
	})
	rsicCycle = ape.NewCycleOrchestrator(cfg, rsicAssessor, rsicReflector, rsicPlanner, rsicDispatcher, rsicMonitor, rsicCalibrator, rsicWatchdog)
	log.Printf("RSIC initialized (watchdog=%v, micro=%v)", cfg.RSICWatchdogEnabled, cfg.RSICMicroEnabled)

	return &Server{
		cfg:             cfg,
		driver:          driver,
		retriever:       ret,
		learner:         lea,
		embedder:        emb,
		anomalyDetector: anom,
		hiddenLayer:     hid,
		hiddenSvc:       hid,
		pluginMgr:       pluginMgr,
		apeScheduler:    apeSched,
		symbolStore:     symStore,
		consultant:      cons,
		gapDetector:     gapDet,
		gapInterviewer:  gapInt,
		conversationSvc: convSvc,
		contextCooler:   ctxCooler,
		sessionTracker:  sessTracker,
		webhookDebouncer:        newLinearWebhookDebouncer(),
		genericWebhookDebouncer: newWebhookDebouncer(),
		fileWatcherMgr:          filewatcher.NewManager(),
		cbRegistry:              cbRegistry,
		metricsRegistry:         metricsRegistry,
		memoryPressure:          memPressure,
		templateService:         conversation.NewTemplateService(driver),
		snapshotService:         conversation.NewSnapshotService(driver),
		orgReviewService:        conversation.NewOrgReviewService(driver),
		rsicCycle:               rsicCycle,
		rsicWatchdog:            rsicWatchdog,
	}
}

// Shutdown gracefully stops background services
func (s *Server) Shutdown() {
	if s.apeScheduler != nil {
		s.apeScheduler.Stop()
	}
	if s.sessionTracker != nil {
		s.sessionTracker.Stop()
	}
	if s.fileWatcherMgr != nil {
		s.fileWatcherMgr.StopAll()
	}
	s.StopPeriodicConsolidation()
	s.StopContextCoolerProcessing()
	s.StopWeeklyGapInterviews()
	s.StopScheduledSync()
	s.StopRSICWatchdog()
}

// StartFileWatchers starts file watchers based on configuration.
// Called during server startup if FILE_WATCHER_ENABLED=true.
func (s *Server) StartFileWatchers() {
	if !s.cfg.FileWatcherEnabled {
		log.Println("file watcher disabled (FILE_WATCHER_ENABLED=false)")
		return
	}

	if s.cfg.FileWatcherConfigs == "" {
		log.Println("file watcher enabled but no configs (FILE_WATCHER_CONFIGS empty)")
		return
	}

	configs := filewatcher.ParseConfigs(s.cfg.FileWatcherConfigs)
	if len(configs) == 0 {
		log.Println("file watcher: no valid configs found")
		return
	}

	for _, cfg := range configs {
		cfg.OnChange = s.handleFileWatcherChange
		if err := s.fileWatcherMgr.AddWatcher(cfg); err != nil {
			log.Printf("file watcher: failed to start watcher for space %s: %v", cfg.SpaceID, err)
		}
	}

	log.Printf("file watcher: started %d watchers", len(configs))
}

// handleFileWatcherChange handles file changes from the file watcher.
func (s *Server) handleFileWatcherChange(ctx context.Context, spaceID string, files []string) {
	log.Printf("[filewatcher] %d files changed in space %s", len(files), spaceID)

	// Call the internal file ingest API
	resp, err := s.ingestFilesInternal(ctx, spaceID, files)
	if err != nil {
		log.Printf("[filewatcher] ingest failed for space %s: %v", spaceID, err)
		return
	}

	log.Printf("[filewatcher] ingested %d/%d files for space %s",
		resp.SuccessCount, resp.TotalFiles, spaceID)

	// Trigger APE event
	s.TriggerAPEEventWithContext("source_changed", map[string]string{
		"space_id":    spaceID,
		"ingest_type": "file-watcher",
	})
}

// ingestFilesInternal is the internal version of file ingestion that doesn't require HTTP.
func (s *Server) ingestFilesInternal(ctx context.Context, spaceID string, files []string) (*models.IngestFilesResponse, error) {
	resp := &models.IngestFilesResponse{
		SpaceID:    spaceID,
		TotalFiles: len(files),
	}

	results := make([]models.IngestFileResult, 0, len(files))
	for _, filePath := range files {
		result := models.IngestFileResult{File: filePath}

		// Check if file exists and is readable
		content, err := os.ReadFile(filePath)
		if err != nil {
			result.Status = "error"
			result.Error = fmt.Sprintf("failed to read file: %v", err)
			resp.ErrorCount++
			results = append(results, result)
			continue
		}

		// Build ingest request
		req := models.IngestRequest{
			SpaceID:   spaceID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Source:    "file-watcher",
			Content:   string(content),
			Path:      filePath,
			Name:      filepath.Base(filePath),
		}

		// Ingest the file
		ingestResp, err := s.retriever.IngestObservation(ctx, req)
		if err != nil {
			result.Status = "error"
			result.Error = fmt.Sprintf("ingest failed: %v", err)
			resp.ErrorCount++
		} else {
			result.Status = "success"
			result.NodeID = ingestResp.NodeID
			resp.SuccessCount++
		}
		results = append(results, result)
	}

	resp.Results = results

	return resp, nil
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

// StartWeeklyGapInterviews starts a background goroutine that runs gap interviews
// on a weekly schedule. Default interval is 7 days.
func (s *Server) StartWeeklyGapInterviews(interval time.Duration) {
	if s.gapInterviewer == nil {
		log.Println("Weekly gap interviews disabled: interviewer not available")
		return
	}
	if interval <= 0 {
		interval = 7 * 24 * time.Hour // Default: weekly
	}

	s.stopInterviewer = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("Weekly gap interviews started (interval=%v)", interval)

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

				cfg := gaps.DefaultInterviewConfig()
				result, err := s.gapInterviewer.RunWeeklyInterview(ctx, cfg)
				cancel()

				if err != nil {
					log.Printf("Weekly gap interview error: %v", err)
				} else if result.PromptsGenerated > 0 {
					log.Printf("Weekly gap interview: analyzed=%d gaps, generated=%d prompts, high_priority=%d",
						result.TotalGapsAnalyzed, result.PromptsGenerated, result.HighPriorityCount)
				}
			case <-s.stopInterviewer:
				log.Println("Weekly gap interviews stopped")
				return
			}
		}
	}()
}

// StopWeeklyGapInterviews stops the background gap interview goroutine
func (s *Server) StopWeeklyGapInterviews() {
	if s.stopInterviewer != nil {
		close(s.stopInterviewer)
		s.stopInterviewer = nil
	}
}

// StartScheduledSync starts a background goroutine that periodically checks for
// stale spaces and triggers incremental re-ingestion for those with configured repo paths.
func (s *Server) StartScheduledSync(interval time.Duration) {
	if interval <= 0 {
		log.Println("scheduled sync disabled (interval <= 0)")
		return
	}

	s.stopScheduledSync = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("scheduled sync started (interval=%v, threshold=%dh)", interval, s.cfg.SyncStaleThresholdHours)

		for {
			select {
			case <-ticker.C:
				s.runScheduledSyncCheck()
			case <-s.stopScheduledSync:
				log.Println("scheduled sync stopped")
				return
			}
		}
	}()
}

// StopScheduledSync stops the background scheduled sync goroutine
func (s *Server) StopScheduledSync() {
	if s.stopScheduledSync != nil {
		close(s.stopScheduledSync)
		s.stopScheduledSync = nil
	}
}

// StartRSICWatchdog starts the RSIC decay watchdog.
func (s *Server) StartRSICWatchdog() {
	if s.rsicWatchdog != nil {
		s.rsicWatchdog.Start()
	}
}

// StopRSICWatchdog stops the RSIC decay watchdog.
func (s *Server) StopRSICWatchdog() {
	if s.rsicWatchdog != nil {
		s.rsicWatchdog.Stop()
	}
}

// runScheduledSyncCheck queries all TapRoot nodes for staleness and triggers
// incremental re-ingestion for stale spaces with configured repo paths.
func (s *Server) runScheduledSyncCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allFreshness, err := s.retriever.GetAllTapRootFreshness(ctx)
	if err != nil {
		log.Printf("scheduled sync: failed to query TapRoot freshness: %v", err)
		return
	}

	threshold := time.Duration(s.cfg.SyncStaleThresholdHours) * time.Hour
	filterSpaces := make(map[string]bool)
	for _, sid := range s.cfg.SyncSpaceIDs {
		filterSpaces[sid] = true
	}

	for _, props := range allFreshness {
		spaceID, _ := props["space_id"].(string)
		if spaceID == "" {
			continue
		}

		// Filter to configured space IDs if set
		if len(filterSpaces) > 0 && !filterSpaces[spaceID] {
			continue
		}

		// Check if stale
		isStale := true
		if lastIngest, ok := props["last_ingest_at"]; ok {
			var lastTime time.Time
			switch v := lastIngest.(type) {
			case time.Time:
				lastTime = v
			case string:
				if parsed, parseErr := time.Parse(time.RFC3339, v); parseErr == nil {
					lastTime = parsed
				}
			}
			if !lastTime.IsZero() {
				isStale = time.Since(lastTime) >= threshold
			}
		}

		if !isStale {
			continue
		}

		// Check if we have a repo path configured for this space
		repoPath, hasPath := s.cfg.SyncRepoPathMap[spaceID]
		if !hasPath {
			log.Printf("scheduled sync: space %s is stale but no repo path configured", spaceID)
			continue
		}

		log.Printf("scheduled sync: triggering incremental re-ingest for stale space %s (path=%s)", spaceID, repoPath)
		s.triggerScheduledIngest(spaceID, repoPath)
	}
}

// triggerScheduledIngest creates a background ingest job for a stale space.
func (s *Server) triggerScheduledIngest(spaceID, repoPath string) {
	queue := jobs.GetQueue()
	jobID := "sync-" + spaceID + "-" + time.Now().Format("20060102150405")
	config := map[string]any{
		"space_id":    spaceID,
		"path":        repoPath,
		"incremental": true,
		"since":       "HEAD~1",
	}

	job, ctx := queue.CreateJob(jobID, "scheduled-sync", config)
	go s.runIngestJob(ctx, job)

	log.Printf("scheduled sync: created job %s for space %s", jobID, spaceID)
}

// TriggerAPEEvent triggers APE modules subscribed to the given event
func (s *Server) TriggerAPEEvent(event string) {
	if s.apeScheduler != nil {
		s.apeScheduler.TriggerEvent(event)
	}
}

// TriggerAPEEventWithContext triggers APE modules subscribed to the given event,
// passing additional context (e.g., space_id, ingest_type) to module execution.
func (s *Server) TriggerAPEEventWithContext(event string, ctx map[string]string) {
	if s.apeScheduler != nil {
		s.apeScheduler.TriggerEventWithContext(event, ctx)
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/v1/embedding/health", s.handleEmbeddingHealth)
	mux.HandleFunc("/v1/memory/retrieve", s.handleRetrieve)
	mux.HandleFunc("/v1/memory/ingest", s.handleIngest)
	mux.HandleFunc("/v1/memory/ingest/batch", s.handleBatchIngest)
	mux.HandleFunc("/v1/memory/reflect", s.handleReflect)
	mux.HandleFunc("/v1/memory/stats", s.handleStats)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/prometheus", s.handlePrometheusMetrics)
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
	mux.HandleFunc("/v1/memory/cache", s.handleCacheClear)
	mux.HandleFunc("/v1/memory/query/metrics", s.handleQueryMetrics)
	mux.HandleFunc("/v1/memory/distribution", s.handleDistributionStats)
	mux.HandleFunc("/v1/memory/symbols", s.handleSymbolSearch)
	mux.HandleFunc("/v1/memory/edges/stale/stats", s.handleStaleEdgeStats)
	mux.HandleFunc("/v1/memory/edges/stale/refresh", s.handleRefreshStaleEdges)

	// Ingestion job management endpoints
	mux.HandleFunc("/v1/memory/ingest/trigger", s.handleIngestTrigger)
	mux.HandleFunc("/v1/memory/ingest/status/", s.handleIngestStatus)
	mux.HandleFunc("/v1/memory/ingest/cancel/", s.handleIngestCancel)
	mux.HandleFunc("/v1/memory/ingest/jobs", s.handleIngestJobs)
	mux.HandleFunc("/v1/memory/ingest/files", s.handleIngestFiles)

	// Capability gap detection endpoints
	mux.HandleFunc("/v1/system/capability-gaps", s.handleCapabilityGaps)
	mux.HandleFunc("/v1/system/capability-gaps/", s.handleCapabilityGapOperation)
	mux.HandleFunc("/v1/feedback", s.handleFeedback)

	// Gap interview endpoints (weekly APE job for addressing capability gaps)
	mux.HandleFunc("/v1/system/gap-interviews", s.handleGapInterviews)
	mux.HandleFunc("/v1/system/gap-interviews/", s.handleGapInterviewOperation)

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
	mux.HandleFunc("/v1/conversation/session/health", s.handleSessionHealth)

	// CMS Templates (Phase 60)
	mux.HandleFunc("/v1/conversation/templates", s.handleTemplates)
	mux.HandleFunc("/v1/conversation/templates/", s.handleTemplateByID)

	// CMS Snapshots (Phase 60)
	mux.HandleFunc("/v1/conversation/snapshot", s.handleSnapshots)
	mux.HandleFunc("/v1/conversation/snapshot/latest", s.handleLatestSnapshot)
	mux.HandleFunc("/v1/conversation/snapshot/cleanup", s.handleCleanupSnapshots)
	mux.HandleFunc("/v1/conversation/snapshot/", s.handleSnapshotByID)

	// CMS Org Reviews (Phase 60)
	mux.HandleFunc("/v1/conversation/org-reviews", s.handleListOrgReviews)
	mux.HandleFunc("/v1/conversation/org-reviews/stats", s.handleOrgReviewStats)
	mux.HandleFunc("/v1/conversation/org-reviews/", s.handleOrgReviewDecision)
	mux.HandleFunc("/v1/conversation/observations/", s.handleFlagForOrgReview)

	// RSIC (Recursive Self-Improvement Cycle) endpoints (Phase 60b)
	mux.HandleFunc("/v1/self-improve/assess", s.handleSelfImproveAssess)
	mux.HandleFunc("/v1/self-improve/report", s.handleSelfImproveReport)
	mux.HandleFunc("/v1/self-improve/report/", s.handleSelfImproveReportByID)
	mux.HandleFunc("/v1/self-improve/cycle", s.handleSelfImproveCycle)
	mux.HandleFunc("/v1/self-improve/history", s.handleSelfImproveHistory)
	mux.HandleFunc("/v1/self-improve/calibration", s.handleSelfImproveCalibration)
	mux.HandleFunc("/v1/self-improve/health", s.handleSelfImproveHealth)

	// Linear CRUD endpoints (Phase 4)
	mux.HandleFunc("/v1/linear/issues", s.handleLinearIssues)
	mux.HandleFunc("/v1/linear/issues/", s.handleLinearIssues)
	mux.HandleFunc("/v1/linear/projects", s.handleLinearProjects)
	mux.HandleFunc("/v1/linear/projects/", s.handleLinearProjects)
	mux.HandleFunc("/v1/linear/comments", s.handleLinearComments)

	// Cleanup endpoints (Phase 9.5)
	mux.HandleFunc("/v1/memory/cleanup/orphans", s.handleCleanupOrphans)
	mux.HandleFunc("/v1/memory/cleanup/schedule", s.handleScheduleCleanup)
	mux.HandleFunc("/v1/memory/cleanup/schedules", s.handleListCleanupSchedules)
	mux.HandleFunc("/v1/memory/cleanup/stats", s.handleCleanupStats)

	// Webhook endpoints (Phase 9.4)
	mux.HandleFunc("/v1/webhooks/linear", s.handleLinearWebhook)
	mux.HandleFunc("/v1/webhooks/", s.handleGenericWebhook)

	// Space freshness endpoints (Phase 9.2)
	mux.HandleFunc("/v1/memory/spaces/", s.handleSpacesRoute)
	mux.HandleFunc("/v1/memory/freshness", s.handleBatchFreshness)

	// Codebase ingestion endpoint
	mux.HandleFunc("/v1/memory/ingest-codebase", s.handleIngestCodebaseRoute)
	mux.HandleFunc("/v1/memory/ingest-codebase/", s.handleIngestCodebaseRoute)

	// SSE streaming endpoint for job progress (Phase 48.3.3)
	mux.HandleFunc("/v1/jobs/", s.handleJobStream)

	// Wrap mux with middleware stack
	// Order (outermost to innermost):
	// 1. Compression (outermost)
	// 2. Logging
	// 3. Metrics
	// 4. CORS
	// 5. Auth
	// 6. Rate Limit
	// 7. Session Warning (innermost before handler)

	var handler http.Handler = mux

	// Session-not-resumed warning middleware (Phase 3A: CMS enforcement) - innermost
	handler = SessionResumeWarningMiddleware(handler, s.sessionTracker)

	// Rate limiting middleware (Phase 3.1)
	if s.cfg.RateLimitEnabled {
		rlSkip := map[string]bool{"/healthz": true, "/readyz": true, "/v1/metrics": true}
		rlCfg := ratelimit.Config{
			Enabled:           true,
			RequestsPerSecond: s.cfg.RateLimitRPS,
			BurstSize:         s.cfg.RateLimitBurst,
			ByIP:              s.cfg.RateLimitByIP,
			SkipEndpoints:     rlSkip,
		}
		handler = ratelimit.Middleware(rlCfg)(handler)
		log.Printf("Rate limiting enabled (%.0f rps, burst: %d, by_ip: %v)",
			s.cfg.RateLimitRPS, s.cfg.RateLimitBurst, s.cfg.RateLimitByIP)
	}

	// Authentication middleware (Phase 3.2)
	if s.cfg.AuthEnabled {
		authSkip := make(map[string]bool)
		for _, ep := range s.cfg.AuthSkipEndpoints {
			authSkip[ep] = true
		}
		authCfg := auth.Config{
			Enabled:       true,
			Mode:          auth.AuthMode(s.cfg.AuthMode),
			APIKeys:       s.cfg.AuthAPIKeys,
			JWTSecret:     s.cfg.AuthJWTSecret,
			JWTIssuer:     s.cfg.AuthJWTIssuer,
			SkipEndpoints: authSkip,
		}
		handler = auth.Middleware(authCfg)(handler)
		log.Printf("Authentication enabled (mode: %s)", s.cfg.AuthMode)
	}

	// CORS middleware (Phase 3.2)
	if s.cfg.CORSEnabled {
		corsCfg := CORSConfig{
			Enabled:          true,
			AllowedOrigins:   s.cfg.CORSAllowedOrigins,
			AllowedMethods:   s.cfg.CORSAllowedMethods,
			AllowedHeaders:   s.cfg.CORSAllowedHeaders,
			AllowCredentials: s.cfg.CORSAllowCredentials,
			MaxAge:           86400,
		}
		handler = CORSMiddleware(corsCfg)(handler)
		log.Printf("CORS enabled (origins: %v)", s.cfg.CORSAllowedOrigins)
	}

	// Prometheus metrics middleware (Phase 3.3)
	if s.cfg.MetricsEnabled {
		handler = metrics.HTTPMiddleware(s.metricsRegistry)(handler)
	}

	// Logging middleware
	logCfg := LogConfig{
		Format:     s.cfg.LogFormat,
		SkipHealth: s.cfg.LogSkipHealth,
	}
	handler = LoggingMiddleware(handler, logCfg)

	// Enable gzip compression for responses > 1KB when CompressionEnabled (outermost)
	if s.cfg.CompressionEnabled {
		handler = CompressionMiddleware(handler, s.cfg.CompressionMinSize)
	}

	// Memory pressure monitoring middleware (Phase 48.4.4) - outermost
	if s.memoryPressure != nil && s.cfg.MemoryPressureEnabled {
		handler = s.memoryPressure.Middleware(handler)
	}

	return handler
}

// handleSpacesRoute routes requests under /v1/memory/spaces/{space_id}/... to the appropriate handler
func (s *Server) handleSpacesRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/freshness") {
		s.handleSpaceFreshness(w, r)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
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

// handlePrometheusMetrics serves Prometheus-format metrics.
func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.MetricsEnabled {
		m := metrics.Metrics()

		// Collect circuit breaker metrics
		if s.cbRegistry != nil {
			// Ensure known circuit breakers are registered (they're created on-demand)
			// This ensures metrics are emitted even if services haven't been called yet
			_ = s.cbRegistry.Get("openai-embeddings")
			_ = s.cbRegistry.Get("openai-rerank")
			_ = s.cbRegistry.Get("ollama-rerank")
			_ = s.cbRegistry.Get("ollama-embeddings")
			m.CollectCircuitBreakerMetrics(s.cbRegistry)
		}

		// Collect cache hit ratio metrics
		if s.retriever != nil {
			cacheStats := map[string]map[string]any{
				"query":     s.retriever.QueryCacheStats(),
				"embedding": s.retriever.EmbeddingCacheStats(),
			}
			m.CollectCacheMetrics(cacheStats)
		}

		// Collect Neo4j pool metrics (Phase 48.4.1)
		m.CollectNeo4jPoolMetrics()

		// Collect memory metrics (Phase 48.4.4)
		if s.memoryPressure != nil {
			m.CollectMemoryMetrics(s.memoryPressure.HeapUsageMB()*1024*1024, s.memoryPressure.RejectedCount())
		}
	}

	metrics.MetricsHandler(s.metricsRegistry)(w, r)
}

// CircuitBreaker returns the circuit breaker for a given service name.
// Used by embeddings and other packages to wrap external API calls.
func (s *Server) CircuitBreaker(service string) *circuitbreaker.Breaker {
	if s.cbRegistry == nil {
		return nil
	}
	return s.cbRegistry.Get(service)
}
