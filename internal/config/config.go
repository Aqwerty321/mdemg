package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr           string
	Neo4jURI             string
	Neo4jUser            string
	Neo4jPass            string
	RequiredSchemaVersion int

	VectorIndexName string
	DefaultCandidateK int
	DefaultTopK int
	DefaultHopDepth int

	MaxNeighborsPerNode int
	MaxTotalEdgesFetched int
	AllowedRelationshipTypes []string

	LearningEdgeCapPerRequest int
	LearningMinActivation     float64
	LearningEta               float64 // Hebbian learning rate (η)
	LearningMu                float64 // Hebbian decay/regularization (μ)
	LearningWMin              float64 // Minimum weight clamp
	LearningWMax              float64 // Maximum weight clamp
	LearningDecayPerDay       float64 // Time-based decay per day of inactivity
	LearningPruneThreshold    float64 // Weight threshold below which edges are pruned
	LearningMaxEdgesPerNode   int     // Max CO_ACTIVATED_WITH edges per node

	// Embedding provider settings
	EmbeddingProvider   string // "openai", "ollama", or "" (disabled)
	OpenAIAPIKey        string
	OpenAIModel         string // default: text-embedding-ada-002
	OpenAIEndpoint      string // default: https://api.openai.com/v1
	OllamaEndpoint      string // default: http://localhost:11434
	OllamaModel         string // default: nomic-embed-text

	// Embedding cache settings
	EmbeddingCacheEnabled bool // Feature toggle (default: true)
	EmbeddingCacheSize    int  // LRU cache capacity (default: 1000)

	// Query result cache settings (Phase 10)
	QueryCacheEnabled    bool // Feature toggle (default: true)
	QueryCacheCapacity   int  // LRU cache capacity (default: 500)
	QueryCacheTTLSeconds int  // TTL in seconds (default: 300)

	// Semantic edge creation on ingest settings
	SemanticEdgeOnIngest      bool    // Feature toggle (default: true)
	SemanticEdgeTopN          int     // Max similar nodes to query (default: 5)
	SemanticEdgeMinSimilarity float64 // Minimum similarity threshold (default: 0.7)
	SemanticEdgeInitialWeight float64 // Initial edge weight (default: 0.5)

	// Batch ingest settings
	BatchIngestMaxItems int // Maximum items per batch request (default: 500)

	// HTTP server timeouts
	HTTPReadTimeout  int // Read timeout in seconds (default: 600)
	HTTPWriteTimeout int // Write timeout in seconds (default: 600)

	// Anomaly detection settings
	AnomalyDetectionEnabled bool    // Feature toggle (default: true)
	AnomalyDuplicateThreshold float64 // Vector similarity threshold for duplicates (default: 0.95)
	AnomalyOutlierStdDevs   float64 // Standard deviations for outlier detection (default: 2.0)
	AnomalyStaleDays        int     // Days after which an update is considered stale (default: 30)
	AnomalyMaxCheckMs       int     // Maximum time for anomaly checks in ms (default: 100)

	// Temporal reasoning settings (Phase 1: Time-Aware Retrieval)
	TemporalEnabled             bool    // Feature toggle for temporal reasoning (default: true)
	TemporalSoftBoostMultiplier float64 // Gamma multiplier for soft-mode recency boost (default: 3.0, range: [1.0, 10.0])
	TemporalHardFilterEnabled   bool    // Enable hard-mode time range filtering (default: true)

	// Phase 2 Temporal: Source-type-specific decay rates
	TemporalSourceTypeDecayEnabled bool    // TEMPORAL_SOURCE_TYPE_DECAY (default: false)
	ScoringRhoDocumentation        float64 // SCORING_RHO_DOCUMENTATION (default: 0.01)
	ScoringRhoConfig               float64 // SCORING_RHO_CONFIG (default: 0.03)
	ScoringRhoConversation         float64 // SCORING_RHO_CONVERSATION (default: 0.10)
	ScoringRhoChangelog            float64 // SCORING_RHO_CHANGELOG (default: 0.08)

	// Phase 2 Temporal: Stale reference detection
	TemporalStaleRefDays    int     // TEMPORAL_STALE_REF_DAYS (default: 0 = disabled)
	TemporalStaleRefMaxPen  float64 // TEMPORAL_STALE_REF_MAX_PENALTY (default: 0.15)

	// Scoring hyperparameters for retrieval ranking
	ScoringAlpha       float64 // Vector similarity weight (default: 0.55)
	ScoringBeta        float64 // Activation weight (default: 0.30)
	ScoringGamma       float64 // Recency weight (default: 0.10)
	ScoringDelta       float64 // Confidence weight (default: 0.05)
	ScoringPhi         float64 // Hub penalty coefficient (default: 0.08)
	ScoringKappa       float64 // Redundancy penalty coefficient (default: 0.12)
	ScoringRho         float64 // Recency decay rate per day (default: 0.05) - legacy, used as fallback
	ScoringRhoL0       float64 // Layer 0 decay rate per day (default: 0.05 - faster decay for files)
	ScoringRhoL1       float64 // Layer 1 decay rate per day (default: 0.02 - slower for hidden/concepts)
	ScoringRhoL2       float64 // Layer 2+ decay rate per day (default: 0.01 - slowest for abstractions)
	ScoringConfigBoost float64 // Score multiplier for config nodes (default: 1.15)
	ScoringPathBoost   float64 // Boost coefficient for path-matching nodes (default: 0.15)

	// Logging settings
	LogFormat     string // "text" (default) or "json"
	LogSkipHealth bool   // Skip logging for /healthz and /readyz endpoints (default: false)

	// Hidden layer settings (V0005)
	HiddenLayerEnabled       bool    // Feature toggle for hidden layer processing (default: true)
	HiddenLayerClusterEps    float64 // DBSCAN epsilon - max distance for neighborhood (default: 0.1)
	HiddenLayerMinSamples    int     // DBSCAN min samples to form cluster (default: 3)
	HiddenLayerMaxHidden     int     // Max hidden nodes to create per consolidation run (default: 100)
	HiddenLayerMaxClusterSize int    // Max members per cluster before splitting (default: 200)
	HiddenLayerPathGroupDepth int    // Path segments for pre-grouping (default: 2)
	HiddenLayerBatchSize     int     // Batch size for clustering (0 = no limit, default: 0)
	HiddenLayerForwardAlpha  float64 // Weight of current embedding in forward pass (default: 0.6)
	HiddenLayerForwardBeta   float64 // Weight of aggregated embedding in forward pass (default: 0.4)
	HiddenLayerBackwardSelf  float64 // Weight of self in backward pass (default: 0.2)
	HiddenLayerBackwardBase  float64 // Weight of base signal in backward pass (default: 0.5)
	HiddenLayerBackwardConc  float64 // Weight of concept signal in backward pass (default: 0.3)

	// Concept merge settings (V0007) - deduplication during consolidation
	ConceptMergeEnabled   bool    // Enable concept deduplication (default: true)
	ConceptMergeThreshold float64 // Cosine similarity threshold for merging (default: 0.90)

	// Edge-type attention settings (V0008) - query-aware edge weighting in activation spreading
	EdgeAttentionEnabled     bool    // Feature toggle (default: true)
	EdgeAttentionCoActivated float64 // Base weight for CO_ACTIVATED_WITH edges (default: 0.85)
	EdgeAttentionAssociated  float64 // Base weight for ASSOCIATED_WITH edges (default: 0.65)
	EdgeAttentionGeneralizes float64 // Base weight for GENERALIZES edges (default: 0.65)
	EdgeAttentionAbstractsTo float64 // Base weight for ABSTRACTS_TO edges (default: 0.60)
	EdgeAttentionTemporal    float64 // Base weight for TEMPORALLY_ADJACENT edges (default: 0.45)
	EdgeAttentionCodeBoost   float64 // Multiplier for CO_ACTIVATED in code queries (default: 1.2)
	EdgeAttentionArchBoost   float64 // Multiplier for hierarchical in arch queries (default: 1.5)

	// Query-Aware Expansion settings (V0009) - attention-based neighbor selection during graph expansion
	QueryAwareExpansionEnabled bool    // Feature toggle (default: true)
	QueryAwareAttentionWeight  float64 // Weight of query-node similarity vs edge weight (default: 0.5)
	NodeEmbeddingCacheSize     int     // LRU cache size for node embeddings (default: 5000)

	// Hybrid Edge Type Strategy settings (V0010) - different edge types at different hop depths
	EdgeTypeStrategy    string   // Strategy: "all", "structural_first", "learned_only", "hybrid" (default: "hybrid")
	StructuralEdgeTypes []string // Edge types for structural hops (default: ASSOCIATED_WITH, GENERALIZES, ABSTRACTS_TO)
	LearnedEdgeTypes    []string // Edge types for learned hops (default: CO_ACTIVATED_WITH)
	HybridSwitchHop     int      // Hop depth at which to switch from structural to learned (default: 1)

	// Hybrid retrieval settings (V0006)
	HybridRetrievalEnabled bool    // Enable hybrid vector+BM25 retrieval (default: true)
	BM25TopK               int     // Candidates from BM25 search (default: 100)
	BM25Weight             float64 // Weight of BM25 in RRF fusion (default: 0.3)
	VectorWeight           float64 // Weight of vector in RRF fusion (default: 0.7)

	// LLM Re-ranking settings (V0006)
	RerankEnabled   bool    // Enable LLM re-ranking (default: false)
	RerankProvider  string  // LLM provider for rerank (openai/ollama)
	RerankModel     string  // Model for re-ranking (default: gpt-4o-mini)
	RerankTopN      int     // Candidates to re-rank (default: 30)
	RerankWeight    float64 // Weight of rerank score in final (default: 0.4)
	RerankTimeoutMs int     // Timeout for rerank call in ms (default: 3000)

	// LLM Summary settings (semantic summaries for ingest)
	LLMSummaryEnabled   bool   // Feature toggle for LLM summaries (default: false)
	LLMSummaryProvider  string // LLM provider for summaries (openai/ollama, default: openai)
	LLMSummaryModel     string // Model for summaries (default: gpt-4o-mini)
	LLMSummaryMaxTokens int    // Max tokens per summary (default: 150)
	LLMSummaryBatchSize int    // Files per API call (default: 10)
	LLMSummaryTimeoutMs int    // Request timeout in ms (default: 30000)
	LLMSummaryCacheSize int    // Max cached summaries (default: 5000)

	// Plugin system settings (V0006)
	PluginsEnabled  bool   // Feature toggle for plugin system (default: true)
	PluginsDir      string // Path to plugins directory (default: ./plugins)
	PluginSocketDir string // Path to Unix socket directory (default: /tmp/mdemg-plugins)
	MdemgVersion    string // MDEMG version string for handshake

	// Linear integration settings (Phase 4)
	LinearTeamID      string // Default team ID for issue creation
	LinearWorkspaceID string // Workspace identifier

	// Linear webhook settings (Phase 9.4)
	LinearWebhookSecret  string // LINEAR_WEBHOOK_SECRET — HMAC-SHA256 signing secret
	LinearWebhookSpaceID string // LINEAR_WEBHOOK_SPACE_ID — space for webhook observations

	// Capability gap detection settings (Task #23)
	GapLowScoreThreshold   float64 // Queries below this avg score are considered poor (default: 0.5)
	GapMinOccurrences      int     // Min occurrences to create a gap (default: 3)
	GapAnalysisWindowHours int     // Time window for pattern analysis in hours (default: 24)
	GapMetricsWindowSize   int     // Number of queries to keep in history (default: 1000)

	// Data transmission optimization settings (Phase 10.3)
	CompressionEnabled  bool // Enable gzip compression for responses (default: true)
	CompressionMinSize  int  // Minimum response size in bytes to compress (default: 1024)
	PaginationMaxLimit  int  // Maximum items per page (default: 500)
	PaginationDefLimit  int  // Default items per page (default: 50)

	// Neo4j connection pool settings (Phase 10.4)
	Neo4jMaxPoolSize         int // Maximum connections in pool (default: 100)
	Neo4jAcquireTimeoutSec   int // Connection acquire timeout in seconds (default: 60)
	Neo4jMaxConnLifetimeSec  int // Maximum connection lifetime in seconds (default: 3600)
	Neo4jConnIdleTimeoutSec  int // Connection idle timeout in seconds (default: 0 = disabled)

	// Dynamic port allocation
	PortRangeStart int    // Start of fallback port range (default: derived from ListenAddr port)
	PortRangeEnd   int    // End of fallback port range (default: PortRangeStart + 100)
	PortFilePath   string // Path to write allocated port for client discovery (default: .mdemg.port)

	// Scheduled sync settings (Phase 9.2)
	SyncIntervalMinutes     int               // Check interval for scheduled sync (default: 0 = disabled)
	SyncSpaceIDs            []string          // Comma-separated space IDs to monitor (empty = all)
	SyncStaleThresholdHours int               // Hours before a space is considered stale (default: 24)
	SyncRepoPathMap         map[string]string // space_id -> repo_path mapping for auto-ingest
}

func FromEnv() (Config, error) {
	get := func(k, def string) string {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def
		}
		return v
	}
	atoi := func(k string, def int) (int, error) {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("%s must be int: %w", k, err)
		}
		return n, nil
	}
	atof := func(k string, def float64) (float64, error) {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def, nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be float: %w", k, err)
		}
		return f, nil
	}
	getBool := func(k string, def bool) bool {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
		if v == "" {
			return def
		}
		return v == "true" || v == "1" || v == "yes"
	}

	listen := get("LISTEN_ADDR", ":8080")
	uri := get("NEO4J_URI", "")
	user := get("NEO4J_USER", "")
	pass := get("NEO4J_PASS", "")
	if uri == "" || user == "" || pass == "" {
		return Config{}, errors.New("NEO4J_URI/NEO4J_USER/NEO4J_PASS are required")
	}

	reqVer, err := atoi("REQUIRED_SCHEMA_VERSION", 0)
	if err != nil {
		return Config{}, err
	}
	if reqVer <= 0 {
		return Config{}, errors.New("REQUIRED_SCHEMA_VERSION must be > 0")
	}

	candK, err := atoi("DEFAULT_CANDIDATE_K", 200)
	if err != nil {
		return Config{}, err
	}
	topK, err := atoi("DEFAULT_TOP_K", 20)
	if err != nil {
		return Config{}, err
	}
	hops, err := atoi("DEFAULT_HOP_DEPTH", 2)
	if err != nil {
		return Config{}, err
	}
	maxNbr, err := atoi("MAX_NEIGHBORS_PER_NODE", 50)
	if err != nil {
		return Config{}, err
	}
	maxEdges, err := atoi("MAX_TOTAL_EDGES_FETCHED", 5000)
	if err != nil {
		return Config{}, err
	}
	learnCap, err := atoi("LEARNING_EDGE_CAP_PER_REQUEST", 200)
	if err != nil {
		return Config{}, err
	}

	// Learning minimum activation threshold (0.0-1.0)
	learnMinActStr := get("LEARNING_MIN_ACTIVATION", "0.20")
	learnMinAct, err := strconv.ParseFloat(learnMinActStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_MIN_ACTIVATION must be float: %w", err)
	}
	if learnMinAct < 0 || learnMinAct > 1 {
		return Config{}, errors.New("LEARNING_MIN_ACTIVATION must be in range [0, 1]")
	}

	// Hebbian learning rate (η) - controls how much activation product strengthens edges
	learnEtaStr := get("LEARNING_ETA", "0.02")
	learnEta, err := strconv.ParseFloat(learnEtaStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_ETA must be float: %w", err)
	}
	if learnEta < 0 {
		return Config{}, errors.New("LEARNING_ETA must be >= 0")
	}

	// Hebbian decay/regularization (μ) - controls weight decay toward zero
	learnMuStr := get("LEARNING_MU", "0.01")
	learnMu, err := strconv.ParseFloat(learnMuStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_MU must be float: %w", err)
	}
	if learnMu < 0 || learnMu > 1 {
		return Config{}, errors.New("LEARNING_MU must be in range [0, 1]")
	}

	// Weight clamp bounds for Hebbian updates
	learnWMinStr := get("LEARNING_WMIN", "0.0")
	learnWMin, err := strconv.ParseFloat(learnWMinStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_WMIN must be float: %w", err)
	}

	learnWMaxStr := get("LEARNING_WMAX", "1.0")
	learnWMax, err := strconv.ParseFloat(learnWMaxStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_WMAX must be float: %w", err)
	}
	if learnWMin >= learnWMax {
		return Config{}, errors.New("LEARNING_WMIN must be < LEARNING_WMAX")
	}

	// Learning edge decay parameters
	learnDecayPerDayStr := get("LEARNING_DECAY_PER_DAY", "0.05")
	learnDecayPerDay, err := strconv.ParseFloat(learnDecayPerDayStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_DECAY_PER_DAY must be float: %w", err)
	}

	learnPruneThresholdStr := get("LEARNING_PRUNE_THRESHOLD", "0.05")
	learnPruneThreshold, err := strconv.ParseFloat(learnPruneThresholdStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_PRUNE_THRESHOLD must be float: %w", err)
	}

	learnMaxEdgesPerNodeStr := get("LEARNING_MAX_EDGES_PER_NODE", "50")
	learnMaxEdgesPerNode, err := strconv.Atoi(learnMaxEdgesPerNodeStr)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_MAX_EDGES_PER_NODE must be int: %w", err)
	}

	allowed := get("ALLOWED_RELATIONSHIP_TYPES", "ASSOCIATED_WITH,TEMPORALLY_ADJACENT,CO_ACTIVATED_WITH,CAUSES,ENABLES,ABSTRACTS_TO,INSTANTIATES,GENERALIZES")
	parts := strings.Split(allowed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	idx := get("VECTOR_INDEX_NAME", "memNodeEmbedding")

	// Semantic edge creation on ingest settings
	semEdgeEnabled := getBool("SEMANTIC_EDGE_ON_INGEST", true)
	semEdgeTopN, err := atoi("SEMANTIC_EDGE_TOP_N", 5)
	if err != nil {
		return Config{}, err
	}
	if semEdgeTopN < 1 {
		return Config{}, errors.New("SEMANTIC_EDGE_TOP_N must be >= 1")
	}
	semEdgeMinSim, err := atof("SEMANTIC_EDGE_MIN_SIMILARITY", 0.7)
	if err != nil {
		return Config{}, err
	}
	if semEdgeMinSim < 0 || semEdgeMinSim > 1 {
		return Config{}, errors.New("SEMANTIC_EDGE_MIN_SIMILARITY must be in range [0, 1]")
	}
	semEdgeInitWeight, err := atof("SEMANTIC_EDGE_INITIAL_WEIGHT", 0.5)
	if err != nil {
		return Config{}, err
	}
	if semEdgeInitWeight < 0 || semEdgeInitWeight > 1 {
		return Config{}, errors.New("SEMANTIC_EDGE_INITIAL_WEIGHT must be in range [0, 1]")
	}

	// Batch ingest settings
	batchMaxItems, err := atoi("BATCH_INGEST_MAX_ITEMS", 500)
	if err != nil {
		return Config{}, err
	}
	if batchMaxItems < 1 || batchMaxItems > 2000 {
		return Config{}, errors.New("BATCH_INGEST_MAX_ITEMS must be in range [1, 2000]")
	}

	// HTTP server timeouts
	httpReadTimeout, err := atoi("HTTP_READ_TIMEOUT", 600)
	if err != nil {
		return Config{}, err
	}
	if httpReadTimeout < 1 {
		return Config{}, errors.New("HTTP_READ_TIMEOUT must be >= 1")
	}
	httpWriteTimeout, err := atoi("HTTP_WRITE_TIMEOUT", 600)
	if err != nil {
		return Config{}, err
	}
	if httpWriteTimeout < 1 {
		return Config{}, errors.New("HTTP_WRITE_TIMEOUT must be >= 1")
	}

	// Anomaly detection settings
	anomalyEnabled := getBool("ANOMALY_DETECTION_ENABLED", true)
	anomalyDupThreshold, err := atof("ANOMALY_DUPLICATE_THRESHOLD", 0.95)
	if err != nil {
		return Config{}, err
	}
	if anomalyDupThreshold < 0 || anomalyDupThreshold > 1 {
		return Config{}, errors.New("ANOMALY_DUPLICATE_THRESHOLD must be in range [0, 1]")
	}
	anomalyOutlierStdDevs, err := atof("ANOMALY_OUTLIER_STDDEVS", 2.0)
	if err != nil {
		return Config{}, err
	}
	if anomalyOutlierStdDevs <= 0 {
		return Config{}, errors.New("ANOMALY_OUTLIER_STDDEVS must be > 0")
	}
	anomalyStaleDays, err := atoi("ANOMALY_STALE_DAYS", 30)
	if err != nil {
		return Config{}, err
	}
	if anomalyStaleDays < 0 {
		return Config{}, errors.New("ANOMALY_STALE_DAYS must be >= 0")
	}
	anomalyMaxCheckMs, err := atoi("ANOMALY_MAX_CHECK_MS", 100)
	if err != nil {
		return Config{}, err
	}
	if anomalyMaxCheckMs < 1 {
		return Config{}, errors.New("ANOMALY_MAX_CHECK_MS must be >= 1")
	}

	// Scoring hyperparameters for retrieval ranking
	// Weights (alpha, beta, gamma, delta) must be in [0, 1]
	scoringAlpha, err := atof("SCORING_ALPHA", 0.55)
	if err != nil {
		return Config{}, err
	}
	if scoringAlpha < 0 || scoringAlpha > 1 {
		return Config{}, errors.New("SCORING_ALPHA must be in range [0, 1]")
	}
	scoringBeta, err := atof("SCORING_BETA", 0.30)
	if err != nil {
		return Config{}, err
	}
	if scoringBeta < 0 || scoringBeta > 1 {
		return Config{}, errors.New("SCORING_BETA must be in range [0, 1]")
	}
	scoringGamma, err := atof("SCORING_GAMMA", 0.10)
	if err != nil {
		return Config{}, err
	}
	if scoringGamma < 0 || scoringGamma > 1 {
		return Config{}, errors.New("SCORING_GAMMA must be in range [0, 1]")
	}
	scoringDelta, err := atof("SCORING_DELTA", 0.05)
	if err != nil {
		return Config{}, err
	}
	if scoringDelta < 0 || scoringDelta > 1 {
		return Config{}, errors.New("SCORING_DELTA must be in range [0, 1]")
	}
	scoringPhi, err := atof("SCORING_PHI", 0.08)
	if err != nil {
		return Config{}, err
	}
	if scoringPhi < 0 {
		return Config{}, errors.New("SCORING_PHI must be >= 0")
	}
	scoringKappa, err := atof("SCORING_KAPPA", 0.12)
	if err != nil {
		return Config{}, err
	}
	if scoringKappa < 0 {
		return Config{}, errors.New("SCORING_KAPPA must be >= 0")
	}
	scoringRho, err := atof("SCORING_RHO", 0.05)
	if err != nil {
		return Config{}, err
	}
	if scoringRho < 0 {
		return Config{}, errors.New("SCORING_RHO must be >= 0")
	}
	// Layer-specific decay rates (faster decay for L0 files, slower for abstractions)
	scoringRhoL0, err := atof("SCORING_RHO_L0", 0.05)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoL0 < 0 {
		return Config{}, errors.New("SCORING_RHO_L0 must be >= 0")
	}
	scoringRhoL1, err := atof("SCORING_RHO_L1", 0.02)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoL1 < 0 {
		return Config{}, errors.New("SCORING_RHO_L1 must be >= 0")
	}
	scoringRhoL2, err := atof("SCORING_RHO_L2", 0.01)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoL2 < 0 {
		return Config{}, errors.New("SCORING_RHO_L2 must be >= 0")
	}
	scoringConfigBoost, err := atof("SCORING_CONFIG_BOOST", 1.15)
	if err != nil {
		return Config{}, err
	}
	if scoringConfigBoost < 1.0 {
		return Config{}, errors.New("SCORING_CONFIG_BOOST must be >= 1.0")
	}
	scoringPathBoost, err := atof("SCORING_PATH_BOOST", 0.15)
	if err != nil {
		return Config{}, err
	}
	if scoringPathBoost < 0 {
		return Config{}, errors.New("SCORING_PATH_BOOST must be >= 0")
	}

	// Temporal reasoning settings (Phase 1: Time-Aware Retrieval)
	temporalEnabled := getBool("TEMPORAL_ENABLED", true)
	temporalSoftBoost, err := atof("TEMPORAL_SOFT_BOOST", 3.0)
	if err != nil {
		return Config{}, err
	}
	if temporalSoftBoost < 1.0 || temporalSoftBoost > 10.0 {
		return Config{}, errors.New("TEMPORAL_SOFT_BOOST must be in range [1.0, 10.0]")
	}
	temporalHardFilterEnabled := getBool("TEMPORAL_HARD_FILTER", true)

	// Phase 2 Temporal: Source-type-specific decay rates
	temporalSourceTypeDecayEnabled := getBool("TEMPORAL_SOURCE_TYPE_DECAY", false)
	scoringRhoDocumentation, err := atof("SCORING_RHO_DOCUMENTATION", 0.01)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoDocumentation < 0 {
		return Config{}, errors.New("SCORING_RHO_DOCUMENTATION must be >= 0")
	}
	scoringRhoConfig, err := atof("SCORING_RHO_CONFIG", 0.03)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoConfig < 0 {
		return Config{}, errors.New("SCORING_RHO_CONFIG must be >= 0")
	}
	scoringRhoConversation, err := atof("SCORING_RHO_CONVERSATION", 0.10)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoConversation < 0 {
		return Config{}, errors.New("SCORING_RHO_CONVERSATION must be >= 0")
	}
	scoringRhoChangelog, err := atof("SCORING_RHO_CHANGELOG", 0.08)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoChangelog < 0 {
		return Config{}, errors.New("SCORING_RHO_CHANGELOG must be >= 0")
	}

	// Phase 2 Temporal: Stale reference detection
	temporalStaleRefDays, err := atoi("TEMPORAL_STALE_REF_DAYS", 0)
	if err != nil {
		return Config{}, err
	}
	if temporalStaleRefDays < 0 {
		return Config{}, errors.New("TEMPORAL_STALE_REF_DAYS must be >= 0")
	}
	temporalStaleRefMaxPen, err := atof("TEMPORAL_STALE_REF_MAX_PENALTY", 0.15)
	if err != nil {
		return Config{}, err
	}
	if temporalStaleRefMaxPen < 0 {
		return Config{}, errors.New("TEMPORAL_STALE_REF_MAX_PENALTY must be >= 0")
	}

	// Embedding provider settings
	embProvider := get("EMBEDDING_PROVIDER", "")
	openaiKey := get("OPENAI_API_KEY", "")
	openaiModel := get("OPENAI_MODEL", "text-embedding-ada-002")
	openaiEndpoint := get("OPENAI_ENDPOINT", "https://api.openai.com/v1")
	ollamaEndpoint := get("OLLAMA_ENDPOINT", "http://localhost:11434")
	ollamaModel := get("OLLAMA_MODEL", "nomic-embed-text")

	// Embedding cache settings
	embCacheEnabled := getBool("EMBEDDING_CACHE_ENABLED", true)
	embCacheSize, err := atoi("EMBEDDING_CACHE_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	if embCacheEnabled && embCacheSize <= 0 {
		return Config{}, errors.New("EMBEDDING_CACHE_SIZE must be > 0 when caching is enabled")
	}

	// Query result cache settings (Phase 10)
	queryCacheEnabled := getBool("QUERY_CACHE_ENABLED", true)
	queryCacheCapacity, err := atoi("QUERY_CACHE_CAPACITY", 500)
	if err != nil {
		return Config{}, err
	}
	queryCacheTTL, err := atoi("QUERY_CACHE_TTL_SECONDS", 300)
	if err != nil {
		return Config{}, err
	}

	// Logging settings
	logFormat := get("LOG_FORMAT", "text")
	if logFormat != "text" && logFormat != "json" {
		return Config{}, errors.New("LOG_FORMAT must be 'text' or 'json'")
	}
	logSkipHealth := getBool("LOG_SKIP_HEALTH", false)

	// Hidden layer settings (V0005)
	hiddenEnabled := getBool("HIDDEN_LAYER_ENABLED", true)
	hiddenClusterEps, err := atof("HIDDEN_LAYER_CLUSTER_EPS", 0.1)
	if err != nil {
		return Config{}, err
	}
	if hiddenClusterEps <= 0 || hiddenClusterEps > 1 {
		return Config{}, errors.New("HIDDEN_LAYER_CLUSTER_EPS must be in range (0, 1]")
	}
	hiddenMinSamples, err := atoi("HIDDEN_LAYER_MIN_SAMPLES", 3)
	if err != nil {
		return Config{}, err
	}
	if hiddenMinSamples < 2 {
		return Config{}, errors.New("HIDDEN_LAYER_MIN_SAMPLES must be >= 2")
	}
	hiddenMaxHidden, err := atoi("HIDDEN_LAYER_MAX_HIDDEN", 100)
	if err != nil {
		return Config{}, err
	}
	if hiddenMaxHidden < 1 {
		return Config{}, errors.New("HIDDEN_LAYER_MAX_HIDDEN must be >= 1")
	}
	hiddenBatchSize, err := atoi("HIDDEN_LAYER_BATCH_SIZE", 0)
	if err != nil {
		return Config{}, err
	}
	if hiddenBatchSize < 0 {
		return Config{}, errors.New("HIDDEN_LAYER_BATCH_SIZE must be >= 0")
	}
	hiddenMaxClusterSize, err := atoi("HIDDEN_LAYER_MAX_CLUSTER_SIZE", 200)
	if err != nil {
		return Config{}, err
	}
	if hiddenMaxClusterSize < 10 {
		return Config{}, errors.New("HIDDEN_LAYER_MAX_CLUSTER_SIZE must be >= 10")
	}
	hiddenPathGroupDepth, err := atoi("HIDDEN_LAYER_PATH_GROUP_DEPTH", 2)
	if err != nil {
		return Config{}, err
	}
	if hiddenPathGroupDepth < 1 || hiddenPathGroupDepth > 5 {
		return Config{}, errors.New("HIDDEN_LAYER_PATH_GROUP_DEPTH must be in range [1, 5]")
	}
	hiddenForwardAlpha, err := atof("HIDDEN_LAYER_FORWARD_ALPHA", 0.6)
	if err != nil {
		return Config{}, err
	}
	hiddenForwardBeta, err := atof("HIDDEN_LAYER_FORWARD_BETA", 0.4)
	if err != nil {
		return Config{}, err
	}
	hiddenBackwardSelf, err := atof("HIDDEN_LAYER_BACKWARD_SELF", 0.2)
	if err != nil {
		return Config{}, err
	}
	hiddenBackwardBase, err := atof("HIDDEN_LAYER_BACKWARD_BASE", 0.5)
	if err != nil {
		return Config{}, err
	}
	hiddenBackwardConc, err := atof("HIDDEN_LAYER_BACKWARD_CONC", 0.3)
	if err != nil {
		return Config{}, err
	}

	// Concept merge settings (V0007)
	conceptMergeEnabled := getBool("CONCEPT_MERGE_ENABLED", true)
	conceptMergeThreshold, err := atof("CONCEPT_MERGE_THRESHOLD", 0.90)
	if err != nil {
		return Config{}, err
	}
	if conceptMergeThreshold < 0.5 || conceptMergeThreshold > 1.0 {
		return Config{}, errors.New("CONCEPT_MERGE_THRESHOLD must be in range [0.5, 1.0]")
	}

	// Edge-type attention settings (V0008)
	edgeAttentionEnabled := getBool("EDGE_ATTENTION_ENABLED", true)
	edgeAttentionCoActivated, err := atof("EDGE_ATTENTION_CO_ACTIVATED", 0.85)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionCoActivated < 0 || edgeAttentionCoActivated > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_CO_ACTIVATED must be in range [0, 1]")
	}
	edgeAttentionAssociated, err := atof("EDGE_ATTENTION_ASSOCIATED", 0.65)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionAssociated < 0 || edgeAttentionAssociated > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_ASSOCIATED must be in range [0, 1]")
	}
	edgeAttentionGeneralizes, err := atof("EDGE_ATTENTION_GENERALIZES", 0.65)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionGeneralizes < 0 || edgeAttentionGeneralizes > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_GENERALIZES must be in range [0, 1]")
	}
	edgeAttentionAbstractsTo, err := atof("EDGE_ATTENTION_ABSTRACTS_TO", 0.60)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionAbstractsTo < 0 || edgeAttentionAbstractsTo > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_ABSTRACTS_TO must be in range [0, 1]")
	}
	edgeAttentionTemporal, err := atof("EDGE_ATTENTION_TEMPORAL", 0.45)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionTemporal < 0 || edgeAttentionTemporal > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_TEMPORAL must be in range [0, 1]")
	}
	edgeAttentionCodeBoost, err := atof("EDGE_ATTENTION_CODE_BOOST", 1.2)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionCodeBoost < 0.5 || edgeAttentionCodeBoost > 3.0 {
		return Config{}, errors.New("EDGE_ATTENTION_CODE_BOOST must be in range [0.5, 3.0]")
	}
	edgeAttentionArchBoost, err := atof("EDGE_ATTENTION_ARCH_BOOST", 1.5)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionArchBoost < 0.5 || edgeAttentionArchBoost > 3.0 {
		return Config{}, errors.New("EDGE_ATTENTION_ARCH_BOOST must be in range [0.5, 3.0]")
	}

	// Query-Aware Expansion settings (V0009)
	queryAwareExpansionEnabled := getBool("QUERY_AWARE_EXPANSION_ENABLED", true)
	queryAwareAttentionWeight, err := atof("QUERY_AWARE_ATTENTION_WEIGHT", 0.5)
	if err != nil {
		return Config{}, err
	}
	if queryAwareAttentionWeight < 0 || queryAwareAttentionWeight > 1 {
		return Config{}, errors.New("QUERY_AWARE_ATTENTION_WEIGHT must be in range [0, 1]")
	}
	nodeEmbeddingCacheSize, err := atoi("NODE_EMBEDDING_CACHE_SIZE", 5000)
	if err != nil {
		return Config{}, err
	}
	if nodeEmbeddingCacheSize < 100 {
		return Config{}, errors.New("NODE_EMBEDDING_CACHE_SIZE must be >= 100")
	}

	// Hybrid Edge Type Strategy settings (V0010)
	edgeTypeStrategy := get("EDGE_TYPE_STRATEGY", "hybrid")
	validStrategies := map[string]bool{"all": true, "structural_first": true, "learned_only": true, "hybrid": true}
	if !validStrategies[edgeTypeStrategy] {
		return Config{}, errors.New("EDGE_TYPE_STRATEGY must be one of: all, structural_first, learned_only, hybrid")
	}

	structuralEdgeTypesStr := get("STRUCTURAL_EDGE_TYPES", "ASSOCIATED_WITH,GENERALIZES,ABSTRACTS_TO")
	structuralEdgeTypes := make([]string, 0)
	for _, p := range strings.Split(structuralEdgeTypesStr, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			structuralEdgeTypes = append(structuralEdgeTypes, p)
		}
	}
	if len(structuralEdgeTypes) == 0 {
		return Config{}, errors.New("STRUCTURAL_EDGE_TYPES must not be empty")
	}

	learnedEdgeTypesStr := get("LEARNED_EDGE_TYPES", "CO_ACTIVATED_WITH")
	learnedEdgeTypes := make([]string, 0)
	for _, p := range strings.Split(learnedEdgeTypesStr, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			learnedEdgeTypes = append(learnedEdgeTypes, p)
		}
	}
	if len(learnedEdgeTypes) == 0 {
		return Config{}, errors.New("LEARNED_EDGE_TYPES must not be empty")
	}

	hybridSwitchHop, err := atoi("HYBRID_SWITCH_HOP", 1)
	if err != nil {
		return Config{}, err
	}
	if hybridSwitchHop < 0 || hybridSwitchHop > 3 {
		return Config{}, errors.New("HYBRID_SWITCH_HOP must be in range [0, 3]")
	}

	// Hybrid retrieval settings (V0006)
	hybridEnabled := getBool("HYBRID_RETRIEVAL_ENABLED", true)
	bm25TopK, err := atoi("BM25_TOP_K", 100)
	if err != nil {
		return Config{}, err
	}
	if bm25TopK < 1 {
		return Config{}, errors.New("BM25_TOP_K must be >= 1")
	}
	bm25Weight, err := atof("BM25_WEIGHT", 0.3)
	if err != nil {
		return Config{}, err
	}
	if bm25Weight < 0 || bm25Weight > 1 {
		return Config{}, errors.New("BM25_WEIGHT must be in range [0, 1]")
	}
	vectorWeight, err := atof("VECTOR_WEIGHT", 0.7)
	if err != nil {
		return Config{}, err
	}
	if vectorWeight < 0 || vectorWeight > 1 {
		return Config{}, errors.New("VECTOR_WEIGHT must be in range [0, 1]")
	}

	// LLM Re-ranking settings (V0006)
	rerankEnabled := getBool("RERANK_ENABLED", false)
	rerankProvider := get("RERANK_PROVIDER", "openai")
	rerankModel := get("RERANK_MODEL", "gpt-4o-mini")
	rerankTopN, err := atoi("RERANK_TOP_N", 30)
	if err != nil {
		return Config{}, err
	}
	if rerankTopN < 5 {
		return Config{}, errors.New("RERANK_TOP_N must be >= 5")
	}
	rerankWeight, err := atof("RERANK_WEIGHT", 0.4)
	if err != nil {
		return Config{}, err
	}
	if rerankWeight < 0 || rerankWeight > 1 {
		return Config{}, errors.New("RERANK_WEIGHT must be in range [0, 1]")
	}
	rerankTimeoutMs, err := atoi("RERANK_TIMEOUT_MS", 3000)
	if err != nil {
		return Config{}, err
	}
	if rerankTimeoutMs < 100 {
		return Config{}, errors.New("RERANK_TIMEOUT_MS must be >= 100")
	}

	// Plugin system settings (V0006)
	pluginsEnabled := getBool("PLUGINS_ENABLED", true)
	pluginsDir := get("PLUGINS_DIR", "./plugins")
	pluginSocketDir := get("PLUGIN_SOCKET_DIR", "/tmp/mdemg-plugins")
	mdemgVersion := get("MDEMG_VERSION", "0.6.0")

	// Linear integration settings (Phase 4)
	linearTeamID := get("LINEAR_TEAM_ID", "")
	linearWorkspaceID := get("LINEAR_WORKSPACE_ID", "")

	// Linear webhook settings (Phase 9.4)
	linearWebhookSecret := get("LINEAR_WEBHOOK_SECRET", "")
	linearWebhookSpaceID := get("LINEAR_WEBHOOK_SPACE_ID", "")

	// LLM Summary settings (semantic summaries for ingest)
	llmSummaryEnabled := getBool("LLM_SUMMARY_ENABLED", false)
	llmSummaryProvider := get("LLM_SUMMARY_PROVIDER", "openai")
	llmSummaryModel := get("LLM_SUMMARY_MODEL", "gpt-4o-mini")
	llmSummaryMaxTokens, err := atoi("LLM_SUMMARY_MAX_TOKENS", 150)
	if err != nil {
		return Config{}, err
	}
	if llmSummaryMaxTokens < 50 || llmSummaryMaxTokens > 500 {
		return Config{}, errors.New("LLM_SUMMARY_MAX_TOKENS must be in range [50, 500]")
	}
	llmSummaryBatchSize, err := atoi("LLM_SUMMARY_BATCH_SIZE", 10)
	if err != nil {
		return Config{}, err
	}
	if llmSummaryBatchSize < 1 || llmSummaryBatchSize > 50 {
		return Config{}, errors.New("LLM_SUMMARY_BATCH_SIZE must be in range [1, 50]")
	}
	llmSummaryTimeoutMs, err := atoi("LLM_SUMMARY_TIMEOUT_MS", 30000)
	if err != nil {
		return Config{}, err
	}
	if llmSummaryTimeoutMs < 1000 {
		return Config{}, errors.New("LLM_SUMMARY_TIMEOUT_MS must be >= 1000")
	}
	llmSummaryCacheSize, err := atoi("LLM_SUMMARY_CACHE_SIZE", 5000)
	if err != nil {
		return Config{}, err
	}
	if llmSummaryCacheSize < 0 {
		return Config{}, errors.New("LLM_SUMMARY_CACHE_SIZE must be >= 0")
	}

	// Capability gap detection settings (Task #23)
	gapLowScoreThreshold, err := atof("GAP_LOW_SCORE_THRESHOLD", 0.5)
	if err != nil {
		return Config{}, err
	}
	if gapLowScoreThreshold < 0 || gapLowScoreThreshold > 1 {
		return Config{}, errors.New("GAP_LOW_SCORE_THRESHOLD must be in range [0, 1]")
	}
	gapMinOccurrences, err := atoi("GAP_MIN_OCCURRENCES", 3)
	if err != nil {
		return Config{}, err
	}
	if gapMinOccurrences < 1 {
		return Config{}, errors.New("GAP_MIN_OCCURRENCES must be >= 1")
	}
	gapAnalysisWindowHours, err := atoi("GAP_ANALYSIS_WINDOW_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	if gapAnalysisWindowHours < 1 {
		return Config{}, errors.New("GAP_ANALYSIS_WINDOW_HOURS must be >= 1")
	}
	gapMetricsWindowSize, err := atoi("GAP_METRICS_WINDOW_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	if gapMetricsWindowSize < 100 {
		return Config{}, errors.New("GAP_METRICS_WINDOW_SIZE must be >= 100")
	}

	// Data transmission optimization settings
	compressionEnabled := getBool("COMPRESSION_ENABLED", true)
	compressionMinSize, err := atoi("COMPRESSION_MIN_SIZE", 1024)
	if err != nil {
		return Config{}, err
	}
	paginationMaxLimit, err := atoi("PAGINATION_MAX_LIMIT", 500)
	if err != nil {
		return Config{}, err
	}
	paginationDefLimit, err := atoi("PAGINATION_DEFAULT_LIMIT", 50)
	if err != nil {
		return Config{}, err
	}

	// Neo4j connection pool settings
	neo4jMaxPoolSize, err := atoi("NEO4J_MAX_POOL_SIZE", 100)
	if err != nil {
		return Config{}, err
	}
	neo4jAcquireTimeout, err := atoi("NEO4J_ACQUIRE_TIMEOUT_SEC", 60)
	if err != nil {
		return Config{}, err
	}
	neo4jMaxConnLifetime, err := atoi("NEO4J_MAX_CONN_LIFETIME_SEC", 3600)
	if err != nil {
		return Config{}, err
	}
	neo4jConnIdleTimeout, err := atoi("NEO4J_CONN_IDLE_TIMEOUT_SEC", 0)
	if err != nil {
		return Config{}, err
	}

	// Dynamic port allocation
	// Derive default PortRangeStart from ListenAddr port
	defaultPortStart := 9999
	if idx := strings.LastIndex(listen, ":"); idx >= 0 {
		if p, parseErr := strconv.Atoi(listen[idx+1:]); parseErr == nil {
			defaultPortStart = p
		}
	}
	portRangeStart, err := atoi("PORT_RANGE_START", defaultPortStart)
	if err != nil {
		return Config{}, err
	}
	// Default PORT_RANGE_END to portRangeStart + 100 to ensure a valid ascending range
	portRangeEnd, err := atoi("PORT_RANGE_END", portRangeStart+100)
	if err != nil {
		return Config{}, err
	}
	if portRangeStart > portRangeEnd {
		return Config{}, errors.New("PORT_RANGE_START must be <= PORT_RANGE_END")
	}
	portFilePath := get("PORT_FILE_PATH", ".mdemg.port")

	// Scheduled sync settings (Phase 9.2)
	syncIntervalMinutes, err := atoi("SYNC_INTERVAL_MINUTES", 0)
	if err != nil {
		return Config{}, err
	}
	if syncIntervalMinutes < 0 {
		return Config{}, errors.New("SYNC_INTERVAL_MINUTES must be >= 0")
	}
	syncStaleThresholdHours, err := atoi("SYNC_STALE_THRESHOLD_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	if syncStaleThresholdHours < 1 {
		return Config{}, errors.New("SYNC_STALE_THRESHOLD_HOURS must be >= 1")
	}
	var syncSpaceIDs []string
	if v := get("SYNC_SPACE_IDS", ""); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				syncSpaceIDs = append(syncSpaceIDs, s)
			}
		}
	}
	syncRepoPathMap := make(map[string]string)
	if v := get("SYNC_REPO_PATHS", ""); v != "" {
		for _, pair := range strings.Split(v, ",") {
			kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(kv) == 2 {
				syncRepoPathMap[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	return Config{
		ListenAddr: listen,
		Neo4jURI: uri,
		Neo4jUser: user,
		Neo4jPass: pass,
		RequiredSchemaVersion: reqVer,
		VectorIndexName: idx,
		DefaultCandidateK: candK,
		DefaultTopK: topK,
		DefaultHopDepth: hops,
		MaxNeighborsPerNode: maxNbr,
		MaxTotalEdgesFetched: maxEdges,
		AllowedRelationshipTypes:  out,
		LearningEdgeCapPerRequest: learnCap,
		LearningMinActivation:     learnMinAct,
		LearningEta:               learnEta,
		LearningMu:                learnMu,
		LearningWMin:              learnWMin,
		LearningWMax:              learnWMax,
		LearningDecayPerDay:       learnDecayPerDay,
		LearningPruneThreshold:    learnPruneThreshold,
		LearningMaxEdgesPerNode:   learnMaxEdgesPerNode,
		EmbeddingProvider:         embProvider,
		OpenAIAPIKey: openaiKey,
		OpenAIModel: openaiModel,
		OpenAIEndpoint: openaiEndpoint,
		OllamaEndpoint: ollamaEndpoint,
		OllamaModel: ollamaModel,
		EmbeddingCacheEnabled:     embCacheEnabled,
		EmbeddingCacheSize:        embCacheSize,
		QueryCacheEnabled:         queryCacheEnabled,
		QueryCacheCapacity:        queryCacheCapacity,
		QueryCacheTTLSeconds:      queryCacheTTL,
		SemanticEdgeOnIngest:      semEdgeEnabled,
		SemanticEdgeTopN:          semEdgeTopN,
		SemanticEdgeMinSimilarity: semEdgeMinSim,
		SemanticEdgeInitialWeight: semEdgeInitWeight,
		BatchIngestMaxItems:       batchMaxItems,
		HTTPReadTimeout:           httpReadTimeout,
		HTTPWriteTimeout:          httpWriteTimeout,
		AnomalyDetectionEnabled:   anomalyEnabled,
		AnomalyDuplicateThreshold: anomalyDupThreshold,
		AnomalyOutlierStdDevs:     anomalyOutlierStdDevs,
		AnomalyStaleDays:          anomalyStaleDays,
		AnomalyMaxCheckMs:         anomalyMaxCheckMs,
		ScoringAlpha:              scoringAlpha,
		ScoringBeta:               scoringBeta,
		ScoringGamma:              scoringGamma,
		ScoringDelta:              scoringDelta,
		ScoringPhi:                scoringPhi,
		ScoringKappa:              scoringKappa,
		ScoringRho:                scoringRho,
		ScoringRhoL0:              scoringRhoL0,
		ScoringRhoL1:              scoringRhoL1,
		ScoringRhoL2:              scoringRhoL2,
		ScoringConfigBoost:        scoringConfigBoost,
		ScoringPathBoost:          scoringPathBoost,
		TemporalEnabled:                temporalEnabled,
		TemporalSoftBoostMultiplier:    temporalSoftBoost,
		TemporalHardFilterEnabled:      temporalHardFilterEnabled,
		TemporalSourceTypeDecayEnabled: temporalSourceTypeDecayEnabled,
		ScoringRhoDocumentation:        scoringRhoDocumentation,
		ScoringRhoConfig:               scoringRhoConfig,
		ScoringRhoConversation:         scoringRhoConversation,
		ScoringRhoChangelog:            scoringRhoChangelog,
		TemporalStaleRefDays:           temporalStaleRefDays,
		TemporalStaleRefMaxPen:         temporalStaleRefMaxPen,
		LogFormat:                 logFormat,
		LogSkipHealth:             logSkipHealth,
		HiddenLayerEnabled:        hiddenEnabled,
		HiddenLayerClusterEps:     hiddenClusterEps,
		HiddenLayerMinSamples:     hiddenMinSamples,
		HiddenLayerMaxHidden:      hiddenMaxHidden,
		HiddenLayerMaxClusterSize: hiddenMaxClusterSize,
		HiddenLayerPathGroupDepth: hiddenPathGroupDepth,
		HiddenLayerBatchSize:      hiddenBatchSize,
		HiddenLayerForwardAlpha:   hiddenForwardAlpha,
		HiddenLayerForwardBeta:    hiddenForwardBeta,
		HiddenLayerBackwardSelf:   hiddenBackwardSelf,
		HiddenLayerBackwardBase:   hiddenBackwardBase,
		HiddenLayerBackwardConc:   hiddenBackwardConc,
		ConceptMergeEnabled:       conceptMergeEnabled,
		ConceptMergeThreshold:     conceptMergeThreshold,
		EdgeAttentionEnabled:      edgeAttentionEnabled,
		EdgeAttentionCoActivated:  edgeAttentionCoActivated,
		EdgeAttentionAssociated:   edgeAttentionAssociated,
		EdgeAttentionGeneralizes:  edgeAttentionGeneralizes,
		EdgeAttentionAbstractsTo:  edgeAttentionAbstractsTo,
		EdgeAttentionTemporal:     edgeAttentionTemporal,
		EdgeAttentionCodeBoost:       edgeAttentionCodeBoost,
		EdgeAttentionArchBoost:       edgeAttentionArchBoost,
		QueryAwareExpansionEnabled:   queryAwareExpansionEnabled,
		QueryAwareAttentionWeight:    queryAwareAttentionWeight,
		NodeEmbeddingCacheSize:       nodeEmbeddingCacheSize,
		EdgeTypeStrategy:             edgeTypeStrategy,
		StructuralEdgeTypes:          structuralEdgeTypes,
		LearnedEdgeTypes:             learnedEdgeTypes,
		HybridSwitchHop:              hybridSwitchHop,
		HybridRetrievalEnabled:       hybridEnabled,
		BM25TopK:                  bm25TopK,
		BM25Weight:                bm25Weight,
		VectorWeight:              vectorWeight,
		RerankEnabled:             rerankEnabled,
		RerankProvider:            rerankProvider,
		RerankModel:               rerankModel,
		RerankTopN:                rerankTopN,
		RerankWeight:              rerankWeight,
		RerankTimeoutMs:           rerankTimeoutMs,
		PluginsEnabled:            pluginsEnabled,
		PluginsDir:                pluginsDir,
		PluginSocketDir:           pluginSocketDir,
		MdemgVersion:              mdemgVersion,
		LinearTeamID:              linearTeamID,
		LinearWorkspaceID:         linearWorkspaceID,
		LinearWebhookSecret:      linearWebhookSecret,
		LinearWebhookSpaceID:     linearWebhookSpaceID,
		LLMSummaryEnabled:         llmSummaryEnabled,
		LLMSummaryProvider:        llmSummaryProvider,
		LLMSummaryModel:           llmSummaryModel,
		LLMSummaryMaxTokens:       llmSummaryMaxTokens,
		LLMSummaryBatchSize:       llmSummaryBatchSize,
		LLMSummaryTimeoutMs:       llmSummaryTimeoutMs,
		LLMSummaryCacheSize:       llmSummaryCacheSize,
		GapLowScoreThreshold:      gapLowScoreThreshold,
		GapMinOccurrences:         gapMinOccurrences,
		GapAnalysisWindowHours:    gapAnalysisWindowHours,
		GapMetricsWindowSize:      gapMetricsWindowSize,
		CompressionEnabled:        compressionEnabled,
		CompressionMinSize:        compressionMinSize,
		PaginationMaxLimit:        paginationMaxLimit,
		PaginationDefLimit:        paginationDefLimit,
		Neo4jMaxPoolSize:          neo4jMaxPoolSize,
		Neo4jAcquireTimeoutSec:    neo4jAcquireTimeout,
		Neo4jMaxConnLifetimeSec:   neo4jMaxConnLifetime,
		Neo4jConnIdleTimeoutSec:   neo4jConnIdleTimeout,
		PortRangeStart:            portRangeStart,
		PortRangeEnd:              portRangeEnd,
		PortFilePath:              portFilePath,
		SyncIntervalMinutes:       syncIntervalMinutes,
		SyncSpaceIDs:              syncSpaceIDs,
		SyncStaleThresholdHours:   syncStaleThresholdHours,
		SyncRepoPathMap:           syncRepoPathMap,
	}, nil
}

// ResolveEndpoint determines the MDEMG API endpoint using a priority chain:
//  1. MDEMG_ENDPOINT env var (explicit override)
//  2. .mdemg.port file (dynamic port discovery)
//  3. LISTEN_ADDR env var (static config)
//  4. defaultAddr fallback
func ResolveEndpoint(defaultAddr string) string {
	// Priority 1: explicit env override
	if ep := strings.TrimSpace(os.Getenv("MDEMG_ENDPOINT")); ep != "" {
		return ep
	}

	// Priority 2: read port file
	portFile := strings.TrimSpace(os.Getenv("PORT_FILE_PATH"))
	if portFile == "" {
		portFile = ".mdemg.port"
	}
	if data, err := os.ReadFile(portFile); err == nil {
		port := strings.TrimSpace(string(data))
		if port != "" {
			return "http://localhost:" + port
		}
	}

	// Priority 3: LISTEN_ADDR env var
	if addr := strings.TrimSpace(os.Getenv("LISTEN_ADDR")); addr != "" {
		if strings.HasPrefix(addr, ":") {
			return "http://localhost" + addr
		}
		return "http://" + addr
	}

	// Priority 4: fallback
	return defaultAddr
}
