//go:build integration

// Package summarize provides LLM-based semantic summary generation for code elements.
// This file contains integration tests that require real API access.
package summarize

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestOpenAIIntegration tests the summarize service with real OpenAI API.
// Requires OPENAI_API_KEY environment variable.
func TestOpenAIIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		MaxTokens:    150,
		BatchSize:    5,
		TimeoutMs:    30000,
		CacheEnabled: true,
		CacheSize:    100,
		OpenAIAPIKey: apiKey,
		Debug:        testing.Verbose(),
	}

	fallbackFn := func(elem CodeElement) string {
		return "Fallback: " + elem.Name
	}

	svc, err := New(cfg, fallbackFn)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test single element summarization
	t.Run("SingleElement", func(t *testing.T) {
		elem := CodeElement{
			Name:    "AuthService",
			Kind:    "class",
			Path:    "/src/services/auth.ts#AuthService",
			Package: "services",
			Content: `
export class AuthService {
  constructor(private readonly userRepo: UserRepository, private readonly jwt: JwtService) {}

  async validateUser(email: string, password: string): Promise<User | null> {
    const user = await this.userRepo.findByEmail(email);
    if (user && await bcrypt.compare(password, user.passwordHash)) {
      return user;
    }
    return null;
  }

  async login(user: User): Promise<{ accessToken: string }> {
    const payload = { sub: user.id, email: user.email };
    return { accessToken: this.jwt.sign(payload) };
  }
}`,
			Concerns: []string{"authentication"},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		summary := svc.Summarize(ctx, elem)

		if summary == "" {
			t.Error("Expected non-empty summary")
		}

		t.Logf("Summary for AuthService: %s", summary)

		// Verify summary mentions authentication-related concepts
		lowerSummary := strings.ToLower(summary)
		if !strings.Contains(lowerSummary, "auth") &&
			!strings.Contains(lowerSummary, "login") &&
			!strings.Contains(lowerSummary, "user") &&
			!strings.Contains(lowerSummary, "valid") {
			t.Logf("Warning: Summary may not capture authentication concepts: %s", summary)
		}
	})

	// Test batch summarization
	t.Run("BatchSummarization", func(t *testing.T) {
		elements := []CodeElement{
			{
				Name:    "UserRepository",
				Kind:    "class",
				Path:    "/src/repositories/user.ts#UserRepository",
				Package: "repositories",
				Content: `
export class UserRepository {
  constructor(private readonly db: Database) {}

  async findById(id: string): Promise<User | null> {
    return this.db.users.findFirst({ where: { id } });
  }

  async findByEmail(email: string): Promise<User | null> {
    return this.db.users.findFirst({ where: { email } });
  }

  async create(data: CreateUserDto): Promise<User> {
    return this.db.users.create({ data });
  }
}`,
			},
			{
				Name:    "CacheService",
				Kind:    "class",
				Path:    "/src/services/cache.ts#CacheService",
				Package: "services",
				Content: `
export class CacheService {
  constructor(private readonly redis: RedisClient) {}

  async get<T>(key: string): Promise<T | null> {
    const data = await this.redis.get(key);
    return data ? JSON.parse(data) : null;
  }

  async set<T>(key: string, value: T, ttlSeconds: number = 3600): Promise<void> {
    await this.redis.setex(key, ttlSeconds, JSON.stringify(value));
  }

  async delete(key: string): Promise<void> {
    await this.redis.del(key);
  }
}`,
				Concerns: []string{"caching"},
			},
			{
				Name:    "LoggingMiddleware",
				Kind:    "function",
				Path:    "/src/middleware/logging.ts#LoggingMiddleware",
				Package: "middleware",
				Content: `
export function LoggingMiddleware(req: Request, res: Response, next: NextFunction) {
  const start = Date.now();
  const requestId = crypto.randomUUID();

  req.requestId = requestId;

  res.on('finish', () => {
    const duration = Date.now() - start;
    logger.info({
      requestId,
      method: req.method,
      path: req.path,
      statusCode: res.statusCode,
      duration,
    });
  });

  next();
}`,
				Concerns: []string{"logging"},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		summaries := svc.SummarizeBatch(ctx, elements)

		if len(summaries) != len(elements) {
			t.Errorf("Expected %d summaries, got %d", len(elements), len(summaries))
		}

		for i, summary := range summaries {
			if summary == "" {
				t.Errorf("Element %d (%s) has empty summary", i, elements[i].Name)
			}
			t.Logf("Summary for %s: %s", elements[i].Name, summary)
		}
	})

	// Test cache functionality
	t.Run("CacheHits", func(t *testing.T) {
		elem := CodeElement{
			Name:    "ConfigService",
			Kind:    "class",
			Path:    "/src/services/config.ts#ConfigService",
			Package: "services",
			Content: `
export class ConfigService {
  private config: Map<string, string> = new Map();

  get(key: string): string | undefined {
    return this.config.get(key) || process.env[key];
  }

  set(key: string, value: string): void {
    this.config.set(key, value);
  }
}`,
		}

		ctx := context.Background()

		// First call - should hit API
		summary1 := svc.Summarize(ctx, elem)

		// Second call - should hit cache
		summary2 := svc.Summarize(ctx, elem)

		if summary1 != summary2 {
			t.Errorf("Cache should return same summary.\nFirst: %s\nSecond: %s", summary1, summary2)
		}

		totalCalls, cacheHits, cacheSize := svc.Stats()
		t.Logf("Stats: totalCalls=%d, cacheHits=%d, cacheSize=%d", totalCalls, cacheHits, cacheSize)

		if cacheHits < 1 {
			t.Error("Expected at least 1 cache hit")
		}
	})
}

// TestOllamaIntegration tests the summarize service with local Ollama.
// Requires Ollama running locally with a model available.
func TestOllamaIntegration(t *testing.T) {
	endpoint := os.Getenv("OLLAMA_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}

	// Check if Ollama is available
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(endpoint + "/api/tags")
	if err != nil {
		t.Skipf("Ollama not available at %s, skipping integration test", endpoint)
	}
	resp.Body.Close()

	// Use a model that's commonly available
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "llama3.2:3b" // Default to a small model
	}

	cfg := Config{
		Enabled:        true,
		Provider:       "ollama",
		Model:          model,
		MaxTokens:      150,
		BatchSize:      3,
		TimeoutMs:      60000, // Ollama can be slower
		CacheEnabled:   true,
		CacheSize:      100,
		OllamaEndpoint: endpoint,
		Debug:          testing.Verbose(),
	}

	fallbackFn := func(elem CodeElement) string {
		return "Fallback: " + elem.Name
	}

	svc, err := New(cfg, fallbackFn)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	t.Run("SingleElement", func(t *testing.T) {
		elem := CodeElement{
			Name:    "DatabaseConnection",
			Kind:    "class",
			Path:    "/src/db/connection.go#DatabaseConnection",
			Package: "db",
			Content: `
type DatabaseConnection struct {
    pool *pgxpool.Pool
    cfg  Config
}

func NewDatabaseConnection(cfg Config) (*DatabaseConnection, error) {
    pool, err := pgxpool.Connect(context.Background(), cfg.ConnectionString)
    if err != nil {
        return nil, fmt.Errorf("failed to connect: %w", err)
    }
    return &DatabaseConnection{pool: pool, cfg: cfg}, nil
}

func (d *DatabaseConnection) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
    return d.pool.Query(ctx, sql, args...)
}

func (d *DatabaseConnection) Close() {
    d.pool.Close()
}`,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		summary := svc.Summarize(ctx, elem)

		if summary == "" {
			t.Error("Expected non-empty summary")
		}

		t.Logf("Ollama Summary for DatabaseConnection: %s", summary)
	})
}

// TestFallbackOnAPIError tests that fallback is used when API fails.
func TestFallbackOnAPIError(t *testing.T) {
	// Use invalid API key to force failure
	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		MaxTokens:      150,
		TimeoutMs:      5000,
		CacheEnabled:   false,
		OpenAIAPIKey:   "invalid-key-for-testing",
		OpenAIEndpoint: "https://api.openai.com/v1",
		Debug:          testing.Verbose(),
	}

	fallbackCalled := false
	fallbackFn := func(elem CodeElement) string {
		fallbackCalled = true
		return "Structural: " + elem.Kind + " " + elem.Name
	}

	svc, err := New(cfg, fallbackFn)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	elem := CodeElement{
		Name:    "TestFunction",
		Kind:    "function",
		Path:    "/test.go#TestFunction",
		Content: "func TestFunction() {}",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary := svc.Summarize(ctx, elem)

	if !fallbackCalled {
		t.Error("Fallback should have been called on API error")
	}

	if summary != "Structural: function TestFunction" {
		t.Errorf("Expected fallback summary, got: %s", summary)
	}
}

// TestBatchSummarizationWithMixedContent tests batch summarization with diverse file types.
func TestBatchSummarizationWithMixedContent(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		MaxTokens:    150,
		BatchSize:    10,
		TimeoutMs:    60000,
		CacheEnabled: true,
		CacheSize:    100,
		OpenAIAPIKey: apiKey,
		Debug:        testing.Verbose(),
	}

	svc, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Load sample files from test directory if available
	sampleDir := "../../docs/tests/llm-summary/sample_files"
	elements := loadSampleElements(t, sampleDir)

	if len(elements) == 0 {
		// Use inline test elements
		elements = getInlineTestElements()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	start := time.Now()
	summaries := svc.SummarizeBatch(ctx, elements)
	duration := time.Since(start)

	t.Logf("Batch summarization took %v for %d elements (%.2f ms/element)",
		duration, len(elements), float64(duration.Milliseconds())/float64(len(elements)))

	if len(summaries) != len(elements) {
		t.Errorf("Expected %d summaries, got %d", len(elements), len(summaries))
	}

	// Verify each summary
	emptyCount := 0
	for i, summary := range summaries {
		if summary == "" {
			emptyCount++
			t.Errorf("Element %d (%s) has empty summary", i, elements[i].Name)
		} else {
			t.Logf("[%s] %s: %s", elements[i].Kind, elements[i].Name, summary)
		}
	}

	if emptyCount > len(elements)/2 {
		t.Errorf("Too many empty summaries: %d/%d", emptyCount, len(elements))
	}

	// Report stats
	totalCalls, cacheHits, cacheSize := svc.Stats()
	t.Logf("Final stats: totalCalls=%d, cacheHits=%d, cacheSize=%d", totalCalls, cacheHits, cacheSize)
}

// loadSampleElements loads code elements from sample files.
func loadSampleElements(t *testing.T, dir string) []CodeElement {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Logf("Could not read sample directory %s: %v", dir, err)
		return nil
	}

	var elements []CodeElement
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := dir + "/" + entry.Name()
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		kind := "file"
		lang := ""
		switch {
		case strings.HasSuffix(entry.Name(), ".go"):
			kind = "package"
			lang = "go"
		case strings.HasSuffix(entry.Name(), ".ts"), strings.HasSuffix(entry.Name(), ".tsx"):
			kind = "module"
			lang = "typescript"
		case strings.HasSuffix(entry.Name(), ".py"):
			kind = "python-module"
			lang = "python"
		case strings.HasSuffix(entry.Name(), ".js"), strings.HasSuffix(entry.Name(), ".jsx"):
			kind = "module"
			lang = "javascript"
		}

		elements = append(elements, CodeElement{
			Name:     entry.Name(),
			Kind:     kind,
			Path:     "/" + path,
			Content:  string(content),
			Package:  lang,
			FilePath: path,
		})
	}

	return elements
}

// getInlineTestElements returns a set of diverse test elements.
func getInlineTestElements() []CodeElement {
	return []CodeElement{
		{
			Name:    "UserController",
			Kind:    "class",
			Path:    "/src/controllers/user.ts#UserController",
			Package: "typescript",
			Content: `
@Controller('users')
export class UserController {
  constructor(private readonly userService: UserService) {}

  @Get(':id')
  async findOne(@Param('id') id: string): Promise<User> {
    return this.userService.findById(id);
  }

  @Post()
  async create(@Body() createUserDto: CreateUserDto): Promise<User> {
    return this.userService.create(createUserDto);
  }
}`,
		},
		{
			Name:    "retrieval.go",
			Kind:    "package",
			Path:    "/internal/retrieval/service.go",
			Package: "go",
			Content: `
package retrieval

type Service struct {
    db         *Database
    embeddings *EmbeddingService
    cache      *Cache
}

func (s *Service) Search(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
    // Generate embedding for query
    embedding, err := s.embeddings.Embed(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("embed query: %w", err)
    }

    // Perform vector similarity search
    results, err := s.db.VectorSearch(ctx, embedding, opts.Limit)
    if err != nil {
        return nil, fmt.Errorf("vector search: %w", err)
    }

    return results, nil
}`,
		},
		{
			Name:    "data_processor.py",
			Kind:    "python-module",
			Path:    "/scripts/data_processor.py",
			Package: "python",
			Content: `
"""Data processing utilities for ETL pipelines."""

import pandas as pd
from typing import List, Dict, Any

class DataProcessor:
    """Handles data transformation and validation."""

    def __init__(self, config: Dict[str, Any]):
        self.config = config
        self.validators = []

    def transform(self, df: pd.DataFrame) -> pd.DataFrame:
        """Apply all configured transformations to the dataframe."""
        for transform_fn in self.config.get('transforms', []):
            df = transform_fn(df)
        return df

    def validate(self, df: pd.DataFrame) -> List[str]:
        """Run all validators and return list of errors."""
        errors = []
        for validator in self.validators:
            errors.extend(validator.check(df))
        return errors`,
		},
		{
			Name:    "useAuth.ts",
			Kind:    "module",
			Path:    "/src/hooks/useAuth.ts",
			Package: "typescript",
			Content: `
import { useState, useEffect, useCallback } from 'react';
import { User, AuthState } from '../types';

export function useAuth() {
  const [state, setState] = useState<AuthState>({
    user: null,
    isLoading: true,
    error: null,
  });

  const login = useCallback(async (email: string, password: string) => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const response = await fetch('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      });
      const user = await response.json();
      setState({ user, isLoading: false, error: null });
    } catch (error) {
      setState(s => ({ ...s, isLoading: false, error: error.message }));
    }
  }, []);

  const logout = useCallback(async () => {
    await fetch('/api/auth/logout', { method: 'POST' });
    setState({ user: null, isLoading: false, error: null });
  }, []);

  return { ...state, login, logout };
}`,
		},
		{
			Name:    "ErrorBoundary.tsx",
			Kind:    "react-component",
			Path:    "/src/components/ErrorBoundary.tsx",
			Package: "typescript",
			Content: `
import React, { Component, ErrorInfo, ReactNode } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Uncaught error:', error, errorInfo);
    // Report to error tracking service
    reportError(error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback || <h1>Something went wrong.</h1>;
    }
    return this.props.children;
  }
}`,
		},
	}
}

// TestSummaryQualityValidation validates summaries against expected themes.
func TestSummaryQualityValidation(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	// Load expected summaries
	expectedPath := "../../docs/tests/llm-summary/expected_summaries.json"
	expectedData, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Skipf("Expected summaries file not found: %v", err)
	}

	var expectedSummaries map[string]struct {
		ExpectedThemes []string `json:"expected_themes"`
		RequiredTerms  []string `json:"required_terms"`
	}
	if err := json.Unmarshal(expectedData, &expectedSummaries); err != nil {
		t.Fatalf("Failed to parse expected summaries: %v", err)
	}

	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		MaxTokens:    150,
		BatchSize:    5,
		TimeoutMs:    30000,
		CacheEnabled: false, // Disable cache for fresh summaries
		OpenAIAPIKey: apiKey,
	}

	svc, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	elements := getInlineTestElements()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	summaries := svc.SummarizeBatch(ctx, elements)

	for i, elem := range elements {
		expected, ok := expectedSummaries[elem.Name]
		if !ok {
			continue
		}

		summary := summaries[i]
		lowerSummary := strings.ToLower(summary)

		// Check for required terms
		missingTerms := []string{}
		for _, term := range expected.RequiredTerms {
			if !strings.Contains(lowerSummary, strings.ToLower(term)) {
				missingTerms = append(missingTerms, term)
			}
		}

		if len(missingTerms) > 0 {
			t.Logf("Warning: Summary for %s missing terms: %v\nSummary: %s",
				elem.Name, missingTerms, summary)
		}

		// Check for expected themes (at least one should match)
		foundTheme := false
		for _, theme := range expected.ExpectedThemes {
			if strings.Contains(lowerSummary, strings.ToLower(theme)) {
				foundTheme = true
				break
			}
		}

		if !foundTheme && len(expected.ExpectedThemes) > 0 {
			t.Logf("Warning: Summary for %s missing expected themes: %v\nSummary: %s",
				elem.Name, expected.ExpectedThemes, summary)
		}
	}
}
