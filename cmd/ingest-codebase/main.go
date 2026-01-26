// Command ingest-codebase walks a codebase and ingests files into MDEMG
// with optimized batch processing and configurable timeouts.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
	"mdemg/internal/summarize"
)

var (
	codebasePath   = flag.String("path", "", "Path to codebase to ingest")
	spaceID        = flag.String("space-id", "codebase", "MDEMG space ID")
	mdemgEndpoint  = flag.String("endpoint", "", "MDEMG endpoint (default: from LISTEN_ADDR in .env)")
	batchSize      = flag.Int("batch", 100, "Batch size for ingestion (default: 100, optimal for ~15/s per worker)")
	workers        = flag.Int("workers", 4, "Number of parallel workers (default: 4)")
	timeout        = flag.Int("timeout", 300, "HTTP timeout in seconds (default: 300)")
	delay          = flag.Int("delay", 50, "Delay between batches in ms (default: 50)")
	maxRetries     = flag.Int("retries", 3, "Max retries per batch on failure (default: 3)")
	retryDelay     = flag.Int("retry-delay", 2000, "Initial retry delay in ms, doubles each retry (default: 2000)")
	consolidate    = flag.Bool("consolidate", true, "Run consolidation after ingestion")
	dryRun         = flag.Bool("dry-run", false, "Print what would be ingested without actually doing it")
	verbose        = flag.Bool("verbose", false, "Verbose output")
	excludeDirs    = flag.String("exclude", ".git,vendor,node_modules,.worktrees", "Comma-separated directories to exclude")
	includeTests   = flag.Bool("include-tests", false, "Include test files (*_test.go, *.test.ts, *.spec.ts)")
	includeMd      = flag.Bool("include-md", true, "Include markdown files (*.md)")
	includeTS      = flag.Bool("include-ts", true, "Include TypeScript/JavaScript files (*.ts, *.tsx, *.js, *.jsx)")
	includePy      = flag.Bool("include-py", true, "Include Python files (*.py)")
	limitElements  = flag.Int("limit", 0, "Limit number of elements to ingest (0 = no limit)")
	extractSymbols = flag.Bool("extract-symbols", true, "Extract code symbols (constants, functions, classes) for evidence-locked retrieval")
	incremental    = flag.Bool("incremental", false, "Only ingest files changed since last commit (uses git diff)")
	sinceCommit    = flag.String("since", "HEAD~1", "Git commit to compare against for incremental mode (default: HEAD~1)")
	archiveDeleted = flag.Bool("archive-deleted", true, "Archive nodes for deleted files in incremental mode")

	// LLM summary options
	llmSummary         = flag.Bool("llm-summary", false, "Use LLM to generate semantic summaries (requires OPENAI_API_KEY)")
	llmSummaryModel    = flag.String("llm-summary-model", "gpt-4o-mini", "Model for LLM summaries")
	llmSummaryBatch    = flag.Int("llm-summary-batch", 10, "Files per LLM API call for summaries")
	llmSummaryProvider = flag.String("llm-summary-provider", "openai", "LLM provider for summaries (openai/ollama)")
)

// Global summarize service (nil if disabled)
var summarizeSvc *summarize.Service

type BatchIngestRequest struct {
	SpaceID      string            `json:"space_id"`
	Observations []BatchIngestItem `json:"observations"`
}

type BatchIngestItem struct {
	Timestamp string         `json:"timestamp"`
	Source    string         `json:"source"`
	Name      string         `json:"name"`
	Path      string         `json:"path"`
	Content   string         `json:"content"`
	Summary   string         `json:"summary,omitempty"`
	Tags      []string       `json:"tags"`
	Symbols   []IngestSymbol `json:"symbols,omitempty"`
}

// IngestSymbol represents a code symbol being ingested
type IngestSymbol struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Value          string `json:"value,omitempty"`
	RawValue       string `json:"raw_value,omitempty"`
	LineNumber     int    `json:"line_number"`
	EndLine        int    `json:"end_line,omitempty"`
	Exported       bool   `json:"exported"`
	DocComment     string `json:"doc_comment,omitempty"`
	Signature      string `json:"signature,omitempty"`
	Parent         string `json:"parent,omitempty"`
	TypeAnnotation string `json:"type_annotation,omitempty"`
	Language       string `json:"language,omitempty"`
}

type CodeElement struct {
	Name     string
	Kind     string
	Path     string
	Content  string
	Summary  string         // Brief summary for reranking (generated from docstrings/comments)
	Package  string
	FilePath string
	Tags     []string
	Concerns []string       // Cross-cutting concerns detected in this element
	Symbols  []IngestSymbol // Extracted code symbols (constants, functions, etc.)
}

// Cross-cutting concern patterns for detection
var concernPatterns = map[string][]string{
	"authentication": {
		"auth", "login", "logout", "signin", "signout", "session",
		"token", "jwt", "oauth", "msal", "azure-ad", "passport",
	},
	"authorization": {
		"acl", "rbac", "permission", "role", "guard", "policy",
		"access-control", "authorize", "can-activate",
	},
	"error-handling": {
		"error", "exception", "filter", "catch", "handler",
		"fault", "failure", "recovery",
	},
	"validation": {
		"validat", "schema", "dto", "constraint", "sanitize",
		"class-validator", "joi", "zod",
	},
	"logging": {
		"logger", "logging", "log", "audit", "trace", "monitor",
	},
	"caching": {
		"cache", "redis", "memcache", "ttl", "invalidat",
	},
	"temporal": {
		"validfrom", "validto", "valid_from", "valid_to",
		"effectivedate", "effective_date", "expirationdate", "expiration_date",
		"startdate", "start_date", "enddate", "end_date",
		"createdat", "created_at", "updatedat", "updated_at", "deletedat", "deleted_at",
		"softdelete", "soft_delete", "paranoid",
		"temporal", "bitemporal", "versioned",
		"daterange", "date_range", "timerange", "time_range",
		"historiz", "audit_trail", "snapshot",
	},
}

// NestJS decorator patterns that indicate cross-cutting concerns
var decoratorConcerns = map[string]string{
	"@Guard":           "authorization",
	"@UseGuards":       "authorization",
	"@Interceptor":     "cross-cutting",
	"@UseInterceptors": "cross-cutting",
	"@Filter":          "error-handling",
	"@UseFilters":      "error-handling",
	"@Catch":           "error-handling",
	"@Injectable":      "service",
}

// Track 6: UI/UX patterns for React/Next.js applications
var uiPatterns = map[string][]string{
	"store": {
		"zustand", "create(", "usestore", "redux", "useselector",
		"usedispatch", "configurestore", "createslice",
		"recoil", "atom(", "selector(",
	},
	"component": {
		"usestate", "useeffect", "usememo", "usecallback",
		"useref", "forwardref", "memo(", "react.fc",
		"react.component", "extends component",
	},
	"routing": {
		"userouter", "usepathname", "usesearchparams",
		"usenavigate", "useparams", "uselocation",
		"next/link", "next/navigation", "react-router",
	},
	"data-fetching": {
		"usequery", "usemutation", "useswr", "tanstack",
		"react-query", "@tanstack/react-query",
		"useinfinitequery", "queryclient",
	},
	"ui-library": {
		"dnd-kit", "usedraggable", "usedroppable", "usesortable",
		"framer-motion", "motion.", "animate",
		"radix-ui", "shadcn", "@radix-ui",
		"tailwindcss", "classnames", "clsx",
	},
	"form": {
		"useform", "react-hook-form", "formik", "useformik",
		"usecontroller", "usewatch", "usefieldarray",
	},
	"context": {
		"createcontext", "usecontext", "provider value=",
		"contextprovider", ".provider>",
	},
}

// Configuration file patterns for detection
var configFilePatterns = []string{
	".config.ts", ".config.js", ".config.json", ".config.yaml", ".config.yml",
	"config.ts", "config.js", "config.json", "config.yaml", "config.yml",
	".env.example", ".env.sample", ".env.template",
	"tsconfig.json", "package.json", "nest-cli.json", "angular.json",
	"webpack.config", "vite.config", "rollup.config", "jest.config",
	"pyproject.toml", "setup.cfg", "setup.py", "requirements.txt",
	"go.mod", "go.sum", "Makefile", "Dockerfile", "docker-compose",
}

// Configuration directory patterns
var configDirPatterns = []string{
	"/config/", "/configuration/", "/configs/", "/settings/",
	"/env/", "/environments/",
}

// isConfigFile checks if a file path indicates a configuration file
func isConfigFile(filePath string) bool {
	lowerPath := strings.ToLower(filePath)

	// Check directory patterns
	for _, pattern := range configDirPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}

	// Check file patterns
	for _, pattern := range configFilePatterns {
		if strings.HasSuffix(lowerPath, pattern) || strings.Contains(filepath.Base(lowerPath), pattern) {
			return true
		}
	}

	// Check for Config suffix in filename (e.g., AppConfig.ts, DatabaseConfig.py)
	base := filepath.Base(filePath)
	baseLower := strings.ToLower(base)
	if strings.Contains(baseLower, "config") && !strings.Contains(baseLower, "test") {
		return true
	}

	return false
}

type IngestStats struct {
	TotalElements int64
	Ingested      int64
	Errors        int64
	StartTime     time.Time
}

// GitDiffResult contains files changed between commits
type GitDiffResult struct {
	Added    []string
	Modified []string
	Deleted  []string
	Renamed  map[string]string // old -> new
}

// getGitChangedFiles returns files changed since the given commit
func getGitChangedFiles(repoPath, sinceCommit string) (*GitDiffResult, error) {
	result := &GitDiffResult{
		Renamed: make(map[string]string),
	}

	// Run git diff with name-status to get file change types
	cmd := exec.Command("git", "-C", repoPath, "diff", "--name-status", sinceCommit)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		switch {
		case status == "A":
			result.Added = append(result.Added, parts[1])
		case status == "M":
			result.Modified = append(result.Modified, parts[1])
		case status == "D":
			result.Deleted = append(result.Deleted, parts[1])
		case strings.HasPrefix(status, "R"):
			// Renamed: R100 old_path new_path
			if len(parts) >= 3 {
				result.Renamed[parts[1]] = parts[2]
				result.Modified = append(result.Modified, parts[2]) // Treat renamed as modified
			}
		}
	}

	return result, nil
}

// archiveDeletedNodes archives MemoryNodes for deleted files
func archiveDeletedNodes(client *http.Client, endpoint, spaceID string, deletedPaths []string) (int, error) {
	if len(deletedPaths) == 0 {
		return 0, nil
	}

	archived := 0
	for _, path := range deletedPaths {
		// Normalize path to match what's stored in the graph
		fullPath := "/" + path

		// Call bulk archive endpoint
		reqBody := map[string]any{
			"space_id": spaceID,
			"filter": map[string]any{
				"path_prefix": fullPath,
			},
			"reason": "File deleted from codebase",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := client.Post(endpoint+"/v1/memory/archive/bulk", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("Warning: failed to archive nodes for %s: %v", path, err)
			continue
		}
		resp.Body.Close()
		archived++
	}

	return archived, nil
}

func main() {
	flag.Parse()

	// Load .env file for dynamic configuration
	if err := godotenv.Load(); err != nil {
		log.Printf("Note: No .env file found, using defaults/flags")
	}

	// Resolve endpoint dynamically from LISTEN_ADDR if not provided
	if *mdemgEndpoint == "" {
		listenAddr := os.Getenv("LISTEN_ADDR")
		if listenAddr == "" {
			listenAddr = ":8090" // fallback default
		}
		// Convert :8090 to http://localhost:8090
		if strings.HasPrefix(listenAddr, ":") {
			*mdemgEndpoint = "http://localhost" + listenAddr
		} else {
			*mdemgEndpoint = "http://" + listenAddr
		}
		log.Printf("Resolved endpoint from LISTEN_ADDR: %s", *mdemgEndpoint)
	}

	if *codebasePath == "" {
		log.Fatal("--path is required")
	}

	excludeSet := make(map[string]bool)
	for _, dir := range strings.Split(*excludeDirs, ",") {
		excludeSet[strings.TrimSpace(dir)] = true
	}

	log.Printf("=== MDEMG Codebase Ingestion ===")
	log.Printf("Path: %s", *codebasePath)
	log.Printf("Space ID: %s", *spaceID)
	log.Printf("Endpoint: %s", *mdemgEndpoint)
	log.Printf("Batch size: %d, Workers: %d, Timeout: %ds", *batchSize, *workers, *timeout)
	log.Printf("Excluded dirs: %s", *excludeDirs)
	log.Printf("Symbol extraction: %v", *extractSymbols)
	log.Printf("Incremental mode: %v", *incremental)
	log.Printf("LLM summaries: %v", *llmSummary)

	// Initialize LLM summarize service if enabled
	if *llmSummary {
		apiKey := os.Getenv("OPENAI_API_KEY")
		ollamaEndpoint := os.Getenv("OLLAMA_ENDPOINT")
		if ollamaEndpoint == "" {
			ollamaEndpoint = "http://localhost:11434"
		}

		if *llmSummaryProvider == "openai" && apiKey == "" {
			log.Println("Warning: LLM summaries enabled but OPENAI_API_KEY not set. Using structural fallback.")
		} else {
			cfg := summarize.Config{
				Enabled:        true,
				Provider:       *llmSummaryProvider,
				Model:          *llmSummaryModel,
				MaxTokens:      150,
				BatchSize:      *llmSummaryBatch,
				TimeoutMs:      30000,
				CacheEnabled:   true,
				CacheSize:      5000,
				Debug:          *verbose,
				OpenAIAPIKey:   apiKey,
				OpenAIEndpoint: os.Getenv("OPENAI_ENDPOINT"),
				OllamaEndpoint: ollamaEndpoint,
			}
			if cfg.OpenAIEndpoint == "" {
				cfg.OpenAIEndpoint = "https://api.openai.com/v1"
			}

			var err error
			summarizeSvc, err = summarize.New(cfg, generateSummaryAdapter)
			if err != nil {
				log.Printf("Warning: Failed to initialize LLM summarize service: %v. Using structural fallback.", err)
			} else {
				log.Printf("LLM summarize service initialized: provider=%s, model=%s, batch=%d",
					*llmSummaryProvider, *llmSummaryModel, *llmSummaryBatch)
			}
		}
	}

	// Handle incremental mode
	var changedFiles *GitDiffResult
	if *incremental {
		log.Printf("Getting changed files since %s...", *sinceCommit)
		var err error
		changedFiles, err = getGitChangedFiles(*codebasePath, *sinceCommit)
		if err != nil {
			log.Fatalf("Failed to get git diff: %v", err)
		}
		log.Printf("Changed files: %d added, %d modified, %d deleted, %d renamed",
			len(changedFiles.Added), len(changedFiles.Modified),
			len(changedFiles.Deleted), len(changedFiles.Renamed))

		if len(changedFiles.Added)+len(changedFiles.Modified) == 0 && len(changedFiles.Deleted) == 0 {
			log.Println("No changes detected. Exiting.")
			return
		}
	}

	// Collect all code elements
	elements, err := walkCodebase(*codebasePath, excludeSet)
	if err != nil {
		log.Fatalf("Failed to walk codebase: %v", err)
	}

	// Filter to only changed files in incremental mode
	if *incremental && changedFiles != nil {
		changedSet := make(map[string]bool)
		for _, f := range changedFiles.Added {
			changedSet[f] = true
		}
		for _, f := range changedFiles.Modified {
			changedSet[f] = true
		}

		var filtered []CodeElement
		for _, elem := range elements {
			// Convert element path to relative path for comparison
			relPath := strings.TrimPrefix(elem.FilePath, "/")
			if changedSet[relPath] || changedSet[elem.FilePath] {
				filtered = append(filtered, elem)
			}
		}
		log.Printf("Filtered from %d to %d elements (changed files only)", len(elements), len(filtered))
		elements = filtered
	}

	log.Printf("Found %d code elements", len(elements))

	// Apply limit if specified
	if *limitElements > 0 && len(elements) > *limitElements {
		log.Printf("Limiting to first %d elements (from %d)", *limitElements, len(elements))
		elements = elements[:*limitElements]
	}

	if *dryRun {
		log.Println("Dry run - not ingesting")
		printSample(elements)
		return
	}

	// Create HTTP client with extended timeout
	client := &http.Client{
		Timeout: time.Duration(*timeout) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	stats := &IngestStats{
		TotalElements: int64(len(elements)),
		StartTime:     time.Now(),
	}

	// Create batches
	var batches [][]CodeElement
	for i := 0; i < len(elements); i += *batchSize {
		end := i + *batchSize
		if end > len(elements) {
			end = len(elements)
		}
		batches = append(batches, elements[i:end])
	}

	log.Printf("Processing %d batches with %d workers", len(batches), *workers)

	// Process batches with worker pool
	batchChan := make(chan []CodeElement, len(batches))
	var wg sync.WaitGroup

	// Start workers
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for batch := range batchChan {
				ingested, errors := ingestBatch(client, batch)
				atomic.AddInt64(&stats.Ingested, int64(ingested))
				atomic.AddInt64(&stats.Errors, int64(errors))

				current := atomic.LoadInt64(&stats.Ingested)
				errCount := atomic.LoadInt64(&stats.Errors)
				elapsed := time.Since(stats.StartTime)
				rate := float64(current) / elapsed.Seconds()

				log.Printf("[Worker %d] Progress: %d/%d (%.1f/s, %d errors)",
					workerID, current, stats.TotalElements, rate, errCount)

				time.Sleep(time.Duration(*delay) * time.Millisecond)
			}
		}(w)
	}

	// Send batches to workers
	for _, batch := range batches {
		batchChan <- batch
	}
	close(batchChan)

	// Wait for all workers to complete
	wg.Wait()

	elapsed := time.Since(stats.StartTime)
	log.Printf("=== Ingestion Complete ===")
	log.Printf("Total: %d, Ingested: %d, Errors: %d", stats.TotalElements, stats.Ingested, stats.Errors)
	log.Printf("Time: %v, Rate: %.1f elements/sec", elapsed, float64(stats.Ingested)/elapsed.Seconds())

	// Print LLM summary stats if service was used
	if summarizeSvc != nil {
		totalCalls, cacheHits, cacheSize := summarizeSvc.Stats()
		log.Printf("LLM Summary stats: calls=%d, cache_hits=%d, cache_size=%d",
			totalCalls, cacheHits, cacheSize)
	}

	// Handle deleted files in incremental mode
	if *incremental && *archiveDeleted && changedFiles != nil && len(changedFiles.Deleted) > 0 {
		log.Printf("Archiving %d deleted files...", len(changedFiles.Deleted))
		archived, err := archiveDeletedNodes(client, *mdemgEndpoint, *spaceID, changedFiles.Deleted)
		if err != nil {
			log.Printf("Warning: archive failed: %v", err)
		} else {
			log.Printf("Archived nodes for %d deleted files", archived)
		}
	}

	// Run consolidation
	if *consolidate && stats.Ingested > 0 {
		log.Println("Running consolidation...")
		if err := runConsolidation(client); err != nil {
			log.Printf("Consolidation failed: %v", err)
		}
	}
}

func walkCodebase(root string, excludeSet map[string]bool) ([]CodeElement, error) {
	var elements []CodeElement
	fset := token.NewFileSet()

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			if excludeSet[info.Name()] {
				if *verbose {
					log.Printf("Skipping excluded directory: %s", path)
				}
				return filepath.SkipDir
			}
			return nil
		}

		// Process Go files
		if strings.HasSuffix(path, ".go") {
			if strings.HasSuffix(path, "_test.go") && !*includeTests {
				return nil
			}
			goElements := parseGoFile(fset, root, path)
			elements = append(elements, goElements...)
			return nil
		}

		// Process Markdown files
		if *includeMd && strings.HasSuffix(path, ".md") {
			mdElement := parseMarkdownFile(root, path)
			if mdElement != nil {
				elements = append(elements, *mdElement)
			}
			return nil
		}

		// Process TypeScript/JavaScript files
		if *includeTS {
			if strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx") ||
				strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx") {
				// Skip test files unless includeTests is set
				if !*includeTests && (strings.HasSuffix(path, ".test.ts") ||
					strings.HasSuffix(path, ".test.tsx") ||
					strings.HasSuffix(path, ".spec.ts") ||
					strings.HasSuffix(path, ".spec.tsx") ||
					strings.HasSuffix(path, ".test.js") ||
					strings.HasSuffix(path, ".spec.js")) {
					return nil
				}
				tsElements := parseTypeScriptFile(root, path)
				elements = append(elements, tsElements...)
				return nil
			}
		}

		// Process Python files
		if *includePy && strings.HasSuffix(path, ".py") {
			if !*includeTests && (strings.HasSuffix(path, "_test.py") ||
				strings.HasSuffix(path, "test_.py") ||
				strings.Contains(path, "/tests/")) {
				return nil
			}
			pyElements := parsePythonFile(root, path)
			elements = append(elements, pyElements...)
			return nil
		}

		// Process JSON/YAML config files
		if strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			if isConfigFile(path) {
				configElement := parseConfigFile(root, path)
				if configElement != nil {
					elements = append(elements, *configElement)
				}
			}
			return nil
		}

		// Process env example files
		if strings.Contains(filepath.Base(path), ".env.") && !strings.HasSuffix(path, ".env") {
			// .env.example, .env.sample, .env.template, etc.
			envElement := parseEnvFile(root, path)
			if envElement != nil {
				elements = append(elements, *envElement)
			}
			return nil
		}

		return nil
	})

	return elements, err
}

func parseGoFile(fset *token.FileSet, root, path string) []CodeElement {
	var elements []CodeElement

	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		if *verbose {
			log.Printf("Parse error for %s: %v", path, err)
		}
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	pkgName := file.Name.Name

	// Read file content for concern detection
	content, _ := os.ReadFile(path)
	contentStr := string(content)
	concerns := detectConcerns(relPath, contentStr)
	tags := []string{"package", pkgName}
	tags = append(tags, concerns...)

	// Check if this is a configuration file
	kind := "package"
	if isConfigFile(relPath) {
		tags = append(tags, "config")
		kind = "config"
	}

	// Build content - include actual code for embedding quality
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Go package %s in file %s\n", pkgName, relPath))

	// Include actual code content for better embeddings (truncate if too long)
	codeContent := contentStr
	maxCodeLen := 4000
	if len(codeContent) > maxCodeLen {
		codeContent = codeContent[:maxCodeLen] + "\n... [truncated]"
	}
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(codeContent)

	// Extract code symbols (constants)
	extractedSymbols := extractSymbolsFromGo(contentStr, relPath)

	// Add package-level element with symbols
	elements = append(elements, CodeElement{
		Name:     pkgName,
		Kind:     kind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  pkgName,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  extractedSymbols,
	})

	// Extract declarations (pass concerns for propagation)
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if elem := extractFunction(d, pkgName, relPath); elem != nil {
				elem.Tags = append(elem.Tags, concerns...)
				elem.Concerns = concerns
				elements = append(elements, *elem)
			}
		case *ast.GenDecl:
			for _, elem := range extractGenDecl(d, pkgName, relPath) {
				elem.Tags = append(elem.Tags, concerns...)
				elem.Concerns = concerns
				elements = append(elements, elem)
			}
		}
	}

	return elements
}

// parseTypeScriptFile extracts elements from TypeScript/JavaScript files
func parseTypeScriptFile(root, path string) []CodeElement {
	var elements []CodeElement

	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)
	contentStr := string(content)

	// Determine file type and language
	var lang, fileKind string
	switch {
	case strings.HasSuffix(path, ".tsx"):
		lang, fileKind = "typescript", "react-component"
	case strings.HasSuffix(path, ".ts"):
		lang, fileKind = "typescript", "module"
	case strings.HasSuffix(path, ".jsx"):
		lang, fileKind = "javascript", "react-component"
	case strings.HasSuffix(path, ".js"):
		lang, fileKind = "javascript", "module"
	}

	// Extract exports, functions, classes using simple pattern matching
	var exports, functions, classes, interfaces []string

	// Find exports
	exportPatterns := []string{
		`export\s+(?:default\s+)?(?:async\s+)?function\s+(\w+)`,
		`export\s+(?:default\s+)?class\s+(\w+)`,
		`export\s+(?:const|let|var)\s+(\w+)`,
		`export\s+interface\s+(\w+)`,
		`export\s+type\s+(\w+)`,
	}
	for _, pattern := range exportPatterns {
		matches := findAllMatches(contentStr, pattern)
		exports = append(exports, matches...)
	}

	// Find function declarations
	funcMatches := findAllMatches(contentStr, `(?:async\s+)?function\s+(\w+)\s*\(`)
	functions = append(functions, funcMatches...)

	// Find arrow functions assigned to const
	arrowMatches := findAllMatches(contentStr, `(?:const|let)\s+(\w+)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>`)
	functions = append(functions, arrowMatches...)

	// Find classes
	classMatches := findAllMatches(contentStr, `class\s+(\w+)`)
	classes = append(classes, classMatches...)

	// Find interfaces (TypeScript)
	if lang == "typescript" {
		interfaceMatches := findAllMatches(contentStr, `interface\s+(\w+)`)
		interfaces = append(interfaces, interfaceMatches...)
	}

	// Build content - include actual code for embedding quality
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("%s file: %s\n", strings.Title(lang), fileName))

	if len(exports) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Exports: %s\n", strings.Join(uniqueStrings(exports), ", ")))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(interfaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Interfaces: %s\n", strings.Join(uniqueStrings(interfaces), ", ")))
	}
	if len(functions) > 0 {
		funcList := uniqueStrings(functions)
		if len(funcList) > 10 {
			funcList = funcList[:10]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(funcList, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(funcList, ", ")))
		}
	}

	// Include actual code content for better embeddings (truncate if too long)
	codeContent := contentStr
	maxCodeLen := 4000 // Keep content reasonable for embedding
	if len(codeContent) > maxCodeLen {
		codeContent = codeContent[:maxCodeLen] + "\n... [truncated]"
	}
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(codeContent)

	// Detect cross-cutting concerns
	concerns := detectConcerns(relPath, contentStr)

	// Build tags including concerns
	tags := []string{lang, fileKind}
	tags = append(tags, concerns...)

	// Check if this is a configuration file
	if isConfigFile(relPath) {
		tags = append(tags, "config")
		fileKind = "config"
	}

	// Extract code symbols (constants, enums)
	extractedSymbols := extractSymbolsFromTypeScript(contentStr, relPath)
	if *verbose && len(extractedSymbols) > 0 {
		log.Printf("  [parseTS] %s: extracted %d symbols", relPath, len(extractedSymbols))
	}

	// Add file-level element with symbols
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     fileKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  lang,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  extractedSymbols,
	})

	// Add individual exports as separate elements for better retrieval
	for _, export := range uniqueStrings(exports) {
		elements = append(elements, CodeElement{
			Name:     export,
			Kind:     "export",
			Path:     fmt.Sprintf("/%s#%s", relPath, export),
			Content:  fmt.Sprintf("Export '%s' from %s file %s", export, lang, fileName),
			Package:  lang,
			FilePath: relPath,
			Tags:     append([]string{lang, "export"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements
}

// parsePythonFile extracts elements from Python files
func parsePythonFile(root, path string) []CodeElement {
	var elements []CodeElement

	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)
	contentStr := string(content)

	// Extract module-level docstring
	docstring := ""
	if strings.HasPrefix(strings.TrimSpace(contentStr), `"""`) ||
		strings.HasPrefix(strings.TrimSpace(contentStr), `'''`) {
		// Simple docstring extraction
		trimmed := strings.TrimSpace(contentStr)
		quote := trimmed[:3]
		end := strings.Index(trimmed[3:], quote)
		if end > 0 {
			docstring = trimmed[3 : 3+end]
			if len(docstring) > 500 {
				docstring = docstring[:500] + "..."
			}
		}
	}

	// Find classes
	classes := findAllMatches(contentStr, `^class\s+(\w+)`)

	// Find functions (top-level and methods)
	functions := findAllMatches(contentStr, `^def\s+(\w+)\s*\(`)

	// Find imports
	imports := findAllMatches(contentStr, `^(?:from\s+(\S+)\s+)?import\s+`)

	// Build content - include actual code for embedding quality
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Python file: %s\n", fileName))

	if docstring != "" {
		contentBuilder.WriteString(fmt.Sprintf("Docstring: %s\n", docstring))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(functions) > 0 {
		funcList := uniqueStrings(functions)
		if len(funcList) > 10 {
			funcList = funcList[:10]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(funcList, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(funcList, ", ")))
		}
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(imports)))

	// Include actual code content for better embeddings (truncate if too long)
	codeContent := contentStr
	maxCodeLen := 4000
	if len(codeContent) > maxCodeLen {
		codeContent = codeContent[:maxCodeLen] + "\n... [truncated]"
	}
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(codeContent)

	// Detect cross-cutting concerns
	concerns := detectConcerns(relPath, contentStr)
	tags := []string{"python", "module"}
	tags = append(tags, concerns...)

	// Check if this is a configuration file
	pyKind := "python-module"
	if isConfigFile(relPath) {
		tags = append(tags, "config")
		pyKind = "config"
	}

	// Extract code symbols (constants)
	extractedSymbols := extractSymbolsFromPython(contentStr, relPath)

	// Add file-level element with symbols
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     pyKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  "python",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  extractedSymbols,
	})

	// Add classes as separate elements
	for _, class := range uniqueStrings(classes) {
		elements = append(elements, CodeElement{
			Name:     class,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, class),
			Content:  fmt.Sprintf("Python class '%s' in file %s", class, fileName),
			Package:  "python",
			FilePath: relPath,
			Tags:     append([]string{"python", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements
}

// extractSymbolsFromTypeScript extracts constants and exports from TypeScript/JavaScript files
func extractSymbolsFromTypeScript(content, filePath string) []IngestSymbol {
	if !*extractSymbols {
		return nil
	}

	var symbols []IngestSymbol
	lines := strings.Split(content, "\n")

	// Patterns for TypeScript/JavaScript constants
	// Pattern: export const NAME = value;
	exportConstPattern := regexp.MustCompile(`^\s*export\s+const\s+([A-Z][A-Z0-9_]*)\s*(?::\s*\w+)?\s*=\s*(.+?);?\s*$`)
	// Pattern: const NAME = value; (module level)
	constPattern := regexp.MustCompile(`^(?:export\s+)?const\s+([A-Z][A-Z0-9_]*)\s*(?::\s*\w+)?\s*=\s*(.+?);?\s*$`)
	// Pattern: enum NAME { ... }
	enumPattern := regexp.MustCompile(`^\s*(?:export\s+)?enum\s+(\w+)\s*\{`)

	inEnum := false
	currentEnum := ""

	for i, line := range lines {
		lineNum := i + 1

		// Check for enum start
		if matches := enumPattern.FindStringSubmatch(line); matches != nil {
			inEnum = true
			currentEnum = matches[1]
			symbols = append(symbols, IngestSymbol{
				Name:       matches[1],
				Type:       "enum",
				LineNumber: lineNum,
				Exported:   strings.Contains(line, "export"),
				Language:   "typescript",
			})
			continue
		}

		// Check for enum end
		if inEnum && strings.Contains(line, "}") {
			inEnum = false
			currentEnum = ""
			continue
		}

		// Extract enum values
		if inEnum {
			// Pattern: VALUE = 'string' or VALUE = number or just VALUE,
			enumValPattern := regexp.MustCompile(`^\s*(\w+)\s*(?:=\s*(.+?))?\s*,?\s*$`)
			if matches := enumValPattern.FindStringSubmatch(line); matches != nil && matches[1] != "" {
				sym := IngestSymbol{
					Name:       matches[1],
					Type:       "enum_value",
					LineNumber: lineNum,
					Parent:     currentEnum,
					Exported:   true,
					Language:   "typescript",
				}
				if len(matches) > 2 && matches[2] != "" {
					sym.Value = strings.TrimSpace(matches[2])
				}
				symbols = append(symbols, sym)
			}
			continue
		}

		// Check for exported constants (UPPER_CASE naming convention)
		if matches := exportConstPattern.FindStringSubmatch(line); matches != nil {
			sym := IngestSymbol{
				Name:       matches[1],
				Type:       "const",
				Value:      cleanValue(matches[2]),
				RawValue:   matches[2],
				LineNumber: lineNum,
				Exported:   true,
				Language:   "typescript",
			}
			symbols = append(symbols, sym)
			continue
		}

		// Check for module-level constants
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			// Skip if we already caught it with export pattern
			if !strings.Contains(line, "export") {
				sym := IngestSymbol{
					Name:       matches[1],
					Type:       "const",
					Value:      cleanValue(matches[2]),
					RawValue:   matches[2],
					LineNumber: lineNum,
					Exported:   false,
					Language:   "typescript",
				}
				symbols = append(symbols, sym)
			}
		}
	}

	return symbols
}

// extractSymbolsFromGo extracts constants from Go files
func extractSymbolsFromGo(content, filePath string) []IngestSymbol {
	if !*extractSymbols {
		return nil
	}

	var symbols []IngestSymbol
	lines := strings.Split(content, "\n")

	// Pattern: const NAME = value or const NAME Type = value
	constPattern := regexp.MustCompile(`^\s*const\s+(\w+)\s*(?:\w+)?\s*=\s*(.+)$`)
	// Pattern: const ( block start
	constBlockStart := regexp.MustCompile(`^\s*const\s*\(\s*$`)
	// Pattern: NAME = value inside const block
	constBlockItem := regexp.MustCompile(`^\s*(\w+)\s*(?:\w+)?\s*=\s*(.+)$`)

	inConstBlock := false

	for i, line := range lines {
		lineNum := i + 1

		// Check for const block start
		if constBlockStart.MatchString(line) {
			inConstBlock = true
			continue
		}

		// Check for const block end
		if inConstBlock && strings.TrimSpace(line) == ")" {
			inConstBlock = false
			continue
		}

		// Extract const from block
		if inConstBlock {
			if matches := constBlockItem.FindStringSubmatch(line); matches != nil {
				// Only extract exported constants (capitalized names)
				if matches[1][0] >= 'A' && matches[1][0] <= 'Z' {
					sym := IngestSymbol{
						Name:       matches[1],
						Type:       "const",
						Value:      cleanValue(matches[2]),
						RawValue:   matches[2],
						LineNumber: lineNum,
						Exported:   true,
						Language:   "go",
					}
					symbols = append(symbols, sym)
				}
			}
			continue
		}

		// Single const declaration
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			exported := matches[1][0] >= 'A' && matches[1][0] <= 'Z'
			sym := IngestSymbol{
				Name:       matches[1],
				Type:       "const",
				Value:      cleanValue(matches[2]),
				RawValue:   matches[2],
				LineNumber: lineNum,
				Exported:   exported,
				Language:   "go",
			}
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// extractSymbolsFromPython extracts constants from Python files
func extractSymbolsFromPython(content, filePath string) []IngestSymbol {
	if !*extractSymbols {
		return nil
	}

	var symbols []IngestSymbol
	lines := strings.Split(content, "\n")

	// Pattern: UPPER_CASE_NAME = value (module-level constants)
	constPattern := regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*(?::\s*\w+)?\s*=\s*(.+)$`)

	for i, line := range lines {
		lineNum := i + 1

		// Skip lines that start with whitespace (not module-level)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			continue
		}

		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			sym := IngestSymbol{
				Name:       matches[1],
				Type:       "const",
				Value:      cleanValue(matches[2]),
				RawValue:   matches[2],
				LineNumber: lineNum,
				Exported:   true, // Python module-level vars are public
				Language:   "python",
			}
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// cleanValue cleans up a constant value, evaluating simple expressions
func cleanValue(value string) string {
	value = strings.TrimSpace(value)

	// Remove trailing semicolons and comments
	if idx := strings.Index(value, "//"); idx != -1 {
		value = strings.TrimSpace(value[:idx])
	}
	if idx := strings.Index(value, "/*"); idx != -1 {
		value = strings.TrimSpace(value[:idx])
	}
	value = strings.TrimSuffix(value, ";")
	value = strings.TrimSpace(value)

	// Try to evaluate simple numeric expressions like "60 * 1000"
	if strings.Contains(value, "*") && !strings.Contains(value, "(") {
		parts := strings.Split(value, "*")
		if len(parts) == 2 {
			var a, b int
			if _, err := fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &a); err == nil {
				if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &b); err == nil {
					return fmt.Sprintf("%d", a*b)
				}
			}
		}
	}

	return value
}

// findAllMatches returns all capture group 1 matches for a pattern
func findAllMatches(content, pattern string) []string {
	re := regexp.MustCompile(`(?m)` + pattern)
	matches := re.FindAllStringSubmatch(content, -1)
	var results []string
	for _, m := range matches {
		if len(m) > 1 && m[1] != "" {
			results = append(results, m[1])
		}
	}
	return results
}

// uniqueStrings returns unique strings from a slice
func uniqueStrings(strs []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range strs {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// detectConcerns analyzes file path and content to detect cross-cutting concerns
func detectConcerns(filePath, content string) []string {
	detected := make(map[string]bool)
	lowerPath := strings.ToLower(filePath)
	lowerContent := strings.ToLower(content)

	// Check file path patterns
	for concern, patterns := range concernPatterns {
		for _, pattern := range patterns {
			if strings.Contains(lowerPath, pattern) {
				detected[concern] = true
				break
			}
		}
	}

	// Check content for NestJS decorators
	for decorator, concern := range decoratorConcerns {
		if strings.Contains(content, decorator) {
			detected[concern] = true
		}
	}

	// Check content for concern patterns (limited to avoid false positives)
	// Only check if the pattern appears multiple times or in specific contexts
	for concern, patterns := range concernPatterns {
		if detected[concern] {
			continue // Already detected from path
		}
		matchCount := 0
		for _, pattern := range patterns {
			if strings.Count(lowerContent, pattern) >= 2 {
				matchCount++
			}
		}
		// Require at least 2 different pattern matches to reduce noise
		if matchCount >= 2 {
			detected[concern] = true
		}
	}

	// Convert to slice with "concern:" prefix for tag filtering
	var concerns []string
	for concern := range detected {
		concerns = append(concerns, "concern:"+concern)
	}

	// Track 6: Detect UI patterns for React/Next.js files
	uiTags := detectUIPatterns(filePath, content)
	concerns = append(concerns, uiTags...)

	return concerns
}

// detectUIPatterns analyzes file path and content to detect UI/UX patterns (Track 6)
func detectUIPatterns(filePath, content string) []string {
	// Only check TSX/JSX/TS/JS files in UI-related directories
	lowerPath := strings.ToLower(filePath)
	isUIFile := strings.HasSuffix(lowerPath, ".tsx") ||
		strings.HasSuffix(lowerPath, ".jsx") ||
		(strings.HasSuffix(lowerPath, ".ts") && (strings.Contains(lowerPath, "/components/") ||
			strings.Contains(lowerPath, "/stores/") ||
			strings.Contains(lowerPath, "/hooks/") ||
			strings.Contains(lowerPath, "/lib/") ||
			strings.Contains(lowerPath, "/app/") ||
			strings.Contains(lowerPath, "/pages/")))

	if !isUIFile {
		return nil
	}

	detected := make(map[string]bool)
	lowerContent := strings.ToLower(content)

	// Check path patterns for specific UI types
	if strings.Contains(lowerPath, "/stores/") || strings.Contains(lowerPath, "-store.") || strings.Contains(lowerPath, "store.ts") {
		detected["store"] = true
	}
	if strings.Contains(lowerPath, "/components/") {
		detected["component"] = true
	}
	if strings.Contains(lowerPath, "/hooks/") || strings.Contains(lowerPath, "use") && strings.HasSuffix(lowerPath, ".ts") {
		detected["hook"] = true
	}
	if strings.Contains(lowerPath, "/providers/") || strings.Contains(lowerPath, "provider") {
		detected["context"] = true
	}

	// Check content for UI patterns
	for uiType, patterns := range uiPatterns {
		matchCount := 0
		for _, pattern := range patterns {
			if strings.Contains(lowerContent, pattern) {
				matchCount++
			}
		}
		// Lower threshold for UI patterns - just need 1 match for specific patterns
		if matchCount >= 1 {
			detected[uiType] = true
		}
	}

	// Convert to slice with "ui:" prefix
	var uiTags []string
	for uiType := range detected {
		uiTags = append(uiTags, "ui:"+uiType)
	}
	return uiTags
}

// parseConfigFile extracts configuration from JSON/YAML files
func parseConfigFile(root, path string) *CodeElement {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)
	contentStr := string(content)

	// Truncate if too long
	if len(contentStr) > 4000 {
		contentStr = contentStr[:4000] + "... [truncated]"
	}

	// Build summary based on file type
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Configuration file: %s. ", name))

	// Try to extract top-level keys for JSON
	if strings.HasSuffix(path, ".json") {
		keys := findAllMatches(contentStr, `"(\w+)":\s*[{\[\"]`)
		if len(keys) > 0 {
			if len(keys) > 10 {
				keys = keys[:10]
			}
			summary.WriteString(fmt.Sprintf("Configuration keys: %s. ", strings.Join(uniqueStrings(keys), ", ")))
		}
	}

	// For YAML, extract top-level keys
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		keys := findAllMatches(contentStr, `^(\w+):`)
		if len(keys) > 0 {
			if len(keys) > 10 {
				keys = keys[:10]
			}
			summary.WriteString(fmt.Sprintf("Configuration sections: %s. ", strings.Join(uniqueStrings(keys), ", ")))
		}
	}

	summary.WriteString(contentStr)

	concerns := detectConcerns(relPath, contentStr)
	tags := []string{"config", "configuration"}
	tags = append(tags, concerns...)

	return &CodeElement{
		Name:     name,
		Kind:     "config",
		Path:     "/" + relPath,
		Content:  summary.String(),
		Package:  "config",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
	}
}

// parseEnvFile extracts environment variable definitions from .env.* files
func parseEnvFile(root, path string) *CodeElement {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)
	contentStr := string(content)

	// Extract environment variable names (excluding values for security)
	envVars := findAllMatches(contentStr, `^([A-Z][A-Z0-9_]*)=`)

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Environment configuration file: %s. ", name))
	summary.WriteString(fmt.Sprintf("Defines %d environment variables. ", len(envVars)))

	if len(envVars) > 0 {
		if len(envVars) > 20 {
			envVars = envVars[:20]
			summary.WriteString(fmt.Sprintf("Variables: %s (and more). ", strings.Join(envVars, ", ")))
		} else {
			summary.WriteString(fmt.Sprintf("Variables: %s. ", strings.Join(envVars, ", ")))
		}
	}

	// Include comments as they often document the variables
	comments := findAllMatches(contentStr, `^#\s*(.+)`)
	if len(comments) > 0 {
		if len(comments) > 5 {
			comments = comments[:5]
		}
		summary.WriteString(fmt.Sprintf("Documentation: %s ", strings.Join(comments, "; ")))
	}

	concerns := detectConcerns(relPath, contentStr)
	tags := []string{"config", "environment", "env-vars"}
	tags = append(tags, concerns...)

	return &CodeElement{
		Name:     name,
		Kind:     "config",
		Path:     "/" + relPath,
		Content:  summary.String(),
		Package:  "config",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
	}
}

func parseMarkdownFile(root, path string) *CodeElement {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)

	// Truncate content if too long
	contentStr := string(content)
	if len(contentStr) > 4000 {
		contentStr = contentStr[:4000] + "... [truncated]"
	}

	// Detect cross-cutting concerns from docs
	concerns := detectConcerns(relPath, contentStr)
	tags := []string{"documentation", "markdown"}
	tags = append(tags, concerns...)

	// Check if this is a configuration doc file
	mdKind := "documentation"
	if isConfigFile(relPath) {
		tags = append(tags, "config")
		mdKind = "config"
	}

	return &CodeElement{
		Name:     name,
		Kind:     mdKind,
		Path:     "/" + relPath,
		Content:  contentStr,
		Package:  "docs",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
	}
}

func extractFunction(fn *ast.FuncDecl, pkg, filePath string) *CodeElement {
	name := fn.Name.Name
	if name == "" {
		return nil
	}

	kind := "function"
	var receiver string
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = "method"
		if t, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
			if ident, ok := t.X.(*ast.Ident); ok {
				receiver = ident.Name
			}
		} else if ident, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
			receiver = ident.Name
		}
	}

	var content strings.Builder
	if receiver != "" {
		content.WriteString(fmt.Sprintf("Method %s on %s in package %s. ", name, receiver, pkg))
	} else {
		content.WriteString(fmt.Sprintf("Function %s in package %s. ", name, pkg))
	}

	if fn.Doc != nil {
		content.WriteString("Documentation: ")
		content.WriteString(fn.Doc.Text())
	}

	if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
		content.WriteString(fmt.Sprintf("Parameters: %d. ", len(fn.Type.Params.List)))
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		content.WriteString(fmt.Sprintf("Returns: %d values. ", len(fn.Type.Results.List)))
	}

	tags := []string{kind, pkg}
	if receiver != "" {
		tags = append(tags, receiver)
	}

	fullName := name
	if receiver != "" {
		fullName = receiver + "." + name
	}

	return &CodeElement{
		Name:     fullName,
		Kind:     kind,
		Path:     fmt.Sprintf("/%s#%s", filePath, name),
		Content:  content.String(),
		Package:  pkg,
		FilePath: filePath,
		Tags:     tags,
	}
}

func extractGenDecl(gd *ast.GenDecl, pkg, filePath string) []CodeElement {
	var elements []CodeElement

	for _, spec := range gd.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if elem := extractType(s, gd.Doc, pkg, filePath); elem != nil {
				elements = append(elements, *elem)
			}
		case *ast.ValueSpec:
			kind := "var"
			if gd.Tok == token.CONST {
				kind = "const"
			}
			for _, name := range s.Names {
				if name.Name == "_" || !name.IsExported() {
					continue
				}
				elements = append(elements, CodeElement{
					Name:     name.Name,
					Kind:     kind,
					Path:     fmt.Sprintf("/%s#%s", filePath, name.Name),
					Content:  fmt.Sprintf("%s %s in package %s", strings.Title(kind), name.Name, pkg),
					Package:  pkg,
					FilePath: filePath,
					Tags:     []string{kind, pkg},
				})
			}
		}
	}

	return elements
}

func extractType(ts *ast.TypeSpec, doc *ast.CommentGroup, pkg, filePath string) *CodeElement {
	name := ts.Name.Name
	if name == "" || !ts.Name.IsExported() {
		return nil
	}

	var kind, content string
	tags := []string{"type", pkg}

	switch t := ts.Type.(type) {
	case *ast.StructType:
		kind = "struct"
		fieldCount := 0
		if t.Fields != nil {
			fieldCount = len(t.Fields.List)
		}
		content = fmt.Sprintf("Struct %s in package %s with %d fields. ", name, pkg, fieldCount)
		tags = append(tags, "struct")
	case *ast.InterfaceType:
		kind = "interface"
		methodCount := 0
		if t.Methods != nil {
			methodCount = len(t.Methods.List)
		}
		content = fmt.Sprintf("Interface %s in package %s with %d methods. ", name, pkg, methodCount)
		tags = append(tags, "interface")
	default:
		kind = "type"
		content = fmt.Sprintf("Type %s in package %s. ", name, pkg)
	}

	if doc != nil {
		content += "Documentation: " + doc.Text()
	}

	return &CodeElement{
		Name:     name,
		Kind:     kind,
		Path:     fmt.Sprintf("/%s#%s", filePath, name),
		Content:  content,
		Package:  pkg,
		FilePath: filePath,
		Tags:     tags,
	}
}

// generateSummaryAdapter adapts generateSummary for use with the summarize package.
// This allows the LLM summarize service to fall back to structural summaries.
func generateSummaryAdapter(elem summarize.CodeElement) string {
	return generateSummary(CodeElement{
		Name:     elem.Name,
		Kind:     elem.Kind,
		Path:     elem.Path,
		Content:  elem.Content,
		Package:  elem.Package,
		FilePath: elem.FilePath,
		Tags:     elem.Tags,
		Concerns: elem.Concerns,
	})
}

// generateSummary creates a brief summary of a code element for reranking.
// This summary helps the LLM reranker understand what each node contains
// without needing to read the full content.
func generateSummary(elem CodeElement) string {
	var summary strings.Builder
	maxLen := 500 // Keep summaries concise for reranking

	// Start with the kind and name
	switch elem.Kind {
	case "package", "config":
		summary.WriteString(fmt.Sprintf("%s: %s", strings.Title(elem.Kind), elem.Name))
	case "function", "method":
		summary.WriteString(fmt.Sprintf("%s %s", strings.Title(elem.Kind), elem.Name))
	case "struct", "interface", "type":
		summary.WriteString(fmt.Sprintf("%s %s", strings.Title(elem.Kind), elem.Name))
	case "const", "var":
		summary.WriteString(fmt.Sprintf("Constant %s", elem.Name))
	case "export":
		summary.WriteString(fmt.Sprintf("Export %s", elem.Name))
	case "class":
		summary.WriteString(fmt.Sprintf("Class %s", elem.Name))
	case "module", "react-component", "typescript", "javascript":
		summary.WriteString(fmt.Sprintf("Module: %s", elem.Name))
	case "python-module":
		summary.WriteString(fmt.Sprintf("Python module: %s", elem.Name))
	case "documentation":
		summary.WriteString(fmt.Sprintf("Documentation: %s", elem.Name))
	default:
		summary.WriteString(fmt.Sprintf("%s: %s", elem.Kind, elem.Name))
	}

	// Add package/location context
	if elem.Package != "" && elem.Package != elem.Name {
		summary.WriteString(fmt.Sprintf(" in %s", elem.Package))
	}

	// Add concern tags if present
	if len(elem.Concerns) > 0 {
		// Filter and dedupe concern names (remove "concern:" prefix)
		var concerns []string
		seen := make(map[string]bool)
		for _, c := range elem.Concerns {
			name := strings.TrimPrefix(c, "concern:")
			name = strings.TrimPrefix(name, "ui:")
			if !seen[name] && name != "" {
				seen[name] = true
				concerns = append(concerns, name)
			}
		}
		if len(concerns) > 0 {
			if len(concerns) > 3 {
				concerns = concerns[:3]
			}
			summary.WriteString(fmt.Sprintf(". Related to: %s", strings.Join(concerns, ", ")))
		}
	}

	// Add symbol count if symbols were extracted
	if len(elem.Symbols) > 0 {
		constCount := 0
		funcCount := 0
		for _, s := range elem.Symbols {
			switch s.Type {
			case "const", "enum_value":
				constCount++
			case "function":
				funcCount++
			}
		}
		if constCount > 0 || funcCount > 0 {
			summary.WriteString(". Contains ")
			parts := []string{}
			if constCount > 0 {
				parts = append(parts, fmt.Sprintf("%d constants", constCount))
			}
			if funcCount > 0 {
				parts = append(parts, fmt.Sprintf("%d functions", funcCount))
			}
			summary.WriteString(strings.Join(parts, " and "))
		}
	}

	// Extract brief content preview (first meaningful sentence from content)
	content := elem.Content
	if idx := strings.Index(content, ". "); idx > 0 && idx < 200 {
		// Get first sentence if it's reasonable length
		firstSentence := content[:idx]
		// Check if it provides useful information beyond what we already have
		lowerSentence := strings.ToLower(firstSentence)
		if !strings.Contains(lowerSentence, "package "+strings.ToLower(elem.Name)) &&
			!strings.Contains(lowerSentence, "file: "+strings.ToLower(elem.Name)) {
			summary.WriteString(". " + firstSentence)
		}
	}

	result := summary.String()
	if len(result) > maxLen {
		result = result[:maxLen-3] + "..."
	}
	return result
}

func ingestBatch(client *http.Client, elements []CodeElement) (int, int) {
	items := make([]BatchIngestItem, 0, len(elements))
	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Generate LLM summaries in batch if service is available
	var llmSummaries []string
	if summarizeSvc != nil {
		// Convert CodeElement to summarize.CodeElement
		sumElements := make([]summarize.CodeElement, len(elements))
		for i, elem := range elements {
			sumElements[i] = summarize.CodeElement{
				Name:     elem.Name,
				Kind:     elem.Kind,
				Path:     elem.Path,
				Content:  elem.Content,
				Package:  elem.Package,
				FilePath: elem.FilePath,
				Tags:     elem.Tags,
				Concerns: elem.Concerns,
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		llmSummaries = summarizeSvc.SummarizeBatch(ctx, sumElements)
		cancel()

		if *verbose && len(llmSummaries) > 0 {
			log.Printf("  [llm-summary] Generated %d LLM summaries", len(llmSummaries))
		}
	}

	for i, elem := range elements {
		// Generate structural summary
		structuralSummary := elem.Summary
		if structuralSummary == "" {
			structuralSummary = generateSummary(elem)
		}

		// Combine with LLM semantic summary if available
		finalSummary := structuralSummary
		if i < len(llmSummaries) && llmSummaries[i] != "" {
			// Check if LLM summary is different from structural (not just fallback)
			if llmSummaries[i] != structuralSummary {
				finalSummary = summarize.CombineSummary(structuralSummary, llmSummaries[i])
				if *verbose {
					log.Printf("  [llm-summary] %s: %s", elem.Name, llmSummaries[i])
				}
			}
		}

		item := BatchIngestItem{
			Timestamp: timestamp,
			Source:    "codebase-ingest",
			Name:      elem.Name,
			Path:      elem.Path,
			Content:   elem.Content,
			Summary:   finalSummary,
			Tags:      elem.Tags,
		}
		// Include symbols if extraction is enabled
		if *extractSymbols && len(elem.Symbols) > 0 {
			item.Symbols = elem.Symbols
			if *verbose {
				log.Printf("  [symbols] %s: %d symbols extracted", elem.Name, len(elem.Symbols))
			}
		}
		items = append(items, item)
	}

	req := BatchIngestRequest{
		SpaceID:      *spaceID,
		Observations: items,
	}

	body, _ := json.Marshal(req)

	// Retry loop with exponential backoff
	var lastErr error
	retryDelayMs := *retryDelay

	for attempt := 0; attempt <= *maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retry %d/%d after %dms delay...", attempt, *maxRetries, retryDelayMs)
			time.Sleep(time.Duration(retryDelayMs) * time.Millisecond)
			retryDelayMs *= 2 // Exponential backoff
		}

		resp, err := client.Post(*mdemgEndpoint+"/v1/memory/ingest/batch", "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			if *verbose {
				log.Printf("Batch ingest request failed (attempt %d): %v", attempt+1, err)
			}
			continue
		}

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusMultiStatus {
			var result struct {
				SuccessCount int `json:"success_count"`
				ErrorCount   int `json:"error_count"`
			}
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()
			return result.SuccessCount, result.ErrorCount
		}

		// Non-retryable error (bad request, etc.)
		if resp.StatusCode == http.StatusBadRequest {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("Batch rejected (non-retryable): status %d: %s", resp.StatusCode, string(bodyBytes))
			return 0, len(elements)
		}

		// Retryable server error
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
		if *verbose {
			log.Printf("Batch ingest failed (attempt %d): %v", attempt+1, lastErr)
		}
	}

	log.Printf("Batch failed after %d retries: %v", *maxRetries, lastErr)
	return 0, len(elements)
}

func runConsolidation(client *http.Client) error {
	req := map[string]string{"space_id": *spaceID}
	body, _ := json.Marshal(req)

	resp, err := client.Post(*mdemgEndpoint+"/v1/memory/consolidate", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			HiddenNodesCreated  int     `json:"hidden_nodes_created"`
			HiddenNodesUpdated  int     `json:"hidden_nodes_updated"`
			ConceptNodesUpdated int     `json:"concept_nodes_updated"`
			DurationMs          float64 `json:"duration_ms"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	log.Printf("Consolidation: created=%d, updated=%d, concept=%d, duration=%.0fms",
		result.Data.HiddenNodesCreated,
		result.Data.HiddenNodesUpdated,
		result.Data.ConceptNodesUpdated,
		result.Data.DurationMs)

	return nil
}

func printSample(elements []CodeElement) {
	counts := make(map[string]int)
	for _, e := range elements {
		counts[e.Kind]++
	}

	log.Println("Element breakdown:")
	for kind, count := range counts {
		log.Printf("  %s: %d", kind, count)
	}

	log.Println("\nSample elements:")
	shown := 0
	for _, e := range elements {
		if shown >= 20 && !*verbose {
			break
		}
		fmt.Printf("  [%s] %s (%s)\n", e.Kind, e.Name, e.FilePath)
		shown++
	}

	if len(elements) > 20 && !*verbose {
		fmt.Printf("  ... and %d more (use --verbose to see all)\n", len(elements)-20)
	}
}
