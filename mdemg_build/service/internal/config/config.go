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

	// Scoring hyperparameters for retrieval ranking
	ScoringAlpha       float64 // Vector similarity weight (default: 0.55)
	ScoringBeta        float64 // Activation weight (default: 0.30)
	ScoringGamma       float64 // Recency weight (default: 0.10)
	ScoringDelta       float64 // Confidence weight (default: 0.05)
	ScoringPhi         float64 // Hub penalty coefficient (default: 0.08)
	ScoringKappa       float64 // Redundancy penalty coefficient (default: 0.12)
	ScoringRho         float64 // Recency decay rate per day (default: 0.05)
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
		EmbeddingProvider:         embProvider,
		OpenAIAPIKey: openaiKey,
		OpenAIModel: openaiModel,
		OpenAIEndpoint: openaiEndpoint,
		OllamaEndpoint: ollamaEndpoint,
		OllamaModel: ollamaModel,
		EmbeddingCacheEnabled:     embCacheEnabled,
		EmbeddingCacheSize:        embCacheSize,
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
		ScoringConfigBoost:        scoringConfigBoost,
		ScoringPathBoost:          scoringPathBoost,
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
		HybridRetrievalEnabled:    hybridEnabled,
		BM25TopK:                  bm25TopK,
		BM25Weight:                bm25Weight,
		VectorWeight:              vectorWeight,
		RerankEnabled:             rerankEnabled,
		RerankProvider:            rerankProvider,
		RerankModel:               rerankModel,
		RerankTopN:                rerankTopN,
		RerankWeight:              rerankWeight,
		RerankTimeoutMs:           rerankTimeoutMs,
	}, nil
}
