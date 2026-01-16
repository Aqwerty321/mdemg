// Package embeddings provides text embedding generation for MDEMG.
// Supports OpenAI and Ollama backends.
package embeddings

import (
	"context"
	"errors"
)

// ErrNoProvider is returned when no embedding provider is configured.
var ErrNoProvider = errors.New("no embedding provider configured")

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed generates an embedding for the given text.
	// Returns a slice of float32 values (typically 1536 dimensions for OpenAI ada-002).
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embeddings for multiple texts.
	// More efficient than calling Embed multiple times.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimensionality of embeddings produced.
	Dimensions() int

	// Name returns the provider name for logging.
	Name() string
}

// Config holds embedding provider configuration.
type Config struct {
	// Provider: "openai" or "ollama"
	Provider string

	// OpenAI settings
	OpenAIAPIKey   string
	OpenAIModel    string // default: text-embedding-ada-002
	OpenAIEndpoint string // default: https://api.openai.com/v1

	// Ollama settings
	OllamaEndpoint string // default: http://localhost:11434
	OllamaModel    string // default: nomic-embed-text
}

// New creates an Embedder based on the configuration.
// Returns ErrNoProvider if no valid provider is configured.
func New(cfg Config) (Embedder, error) {
	switch cfg.Provider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, errors.New("OPENAI_API_KEY is required for openai provider")
		}
		return NewOpenAI(cfg)
	case "ollama":
		return NewOllama(cfg)
	case "", "none":
		return nil, ErrNoProvider
	default:
		return nil, errors.New("unknown embedding provider: " + cfg.Provider)
	}
}
