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

	// Embedding provider settings
	EmbeddingProvider   string // "openai", "ollama", or "" (disabled)
	OpenAIAPIKey        string
	OpenAIModel         string // default: text-embedding-ada-002
	OpenAIEndpoint      string // default: https://api.openai.com/v1
	OllamaEndpoint      string // default: http://localhost:11434
	OllamaModel         string // default: nomic-embed-text
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

	allowed := get("ALLOWED_RELATIONSHIP_TYPES", "ASSOCIATED_WITH,TEMPORALLY_ADJACENT,CO_ACTIVATED_WITH,CAUSES,ENABLES,ABSTRACTS_TO,INSTANTIATES")
	parts := strings.Split(allowed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	idx := get("VECTOR_INDEX_NAME", "memNodeEmbedding")

	// Embedding provider settings
	embProvider := get("EMBEDDING_PROVIDER", "")
	openaiKey := get("OPENAI_API_KEY", "")
	openaiModel := get("OPENAI_MODEL", "text-embedding-ada-002")
	openaiEndpoint := get("OPENAI_ENDPOINT", "https://api.openai.com/v1")
	ollamaEndpoint := get("OLLAMA_ENDPOINT", "http://localhost:11434")
	ollamaModel := get("OLLAMA_MODEL", "nomic-embed-text")

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
		EmbeddingProvider:         embProvider,
		OpenAIAPIKey: openaiKey,
		OpenAIModel: openaiModel,
		OpenAIEndpoint: openaiEndpoint,
		OllamaEndpoint: ollamaEndpoint,
		OllamaModel: ollamaModel,
	}, nil
}
