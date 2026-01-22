// Command ingest-codebase walks a codebase and ingests files into MDEMG
// with optimized batch processing and configurable timeouts.
package main

import (
	"bytes"
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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	codebasePath  = flag.String("path", "", "Path to codebase to ingest")
	spaceID       = flag.String("space-id", "codebase", "MDEMG space ID")
	mdemgEndpoint = flag.String("endpoint", "http://localhost:8082", "MDEMG endpoint")
	batchSize     = flag.Int("batch", 100, "Batch size for ingestion (default: 100)")
	workers       = flag.Int("workers", 4, "Number of parallel workers (default: 4)")
	timeout       = flag.Int("timeout", 300, "HTTP timeout in seconds (default: 300)")
	delay         = flag.Int("delay", 50, "Delay between batches in ms (default: 50)")
	maxRetries    = flag.Int("retries", 3, "Max retries per batch on failure (default: 3)")
	retryDelay    = flag.Int("retry-delay", 2000, "Initial retry delay in ms, doubles each retry (default: 2000)")
	consolidate   = flag.Bool("consolidate", true, "Run consolidation after ingestion")
	dryRun        = flag.Bool("dry-run", false, "Print what would be ingested without actually doing it")
	verbose       = flag.Bool("verbose", false, "Verbose output")
	excludeDirs   = flag.String("exclude", ".git,vendor,node_modules,.worktrees,.auto-claude", "Comma-separated directories to exclude")
	includeTests  = flag.Bool("include-tests", false, "Include test files (*_test.go, *.test.ts, *.spec.ts)")
	includeMd     = flag.Bool("include-md", true, "Include markdown files (*.md)")
	includeTS     = flag.Bool("include-ts", true, "Include TypeScript/JavaScript files (*.ts, *.tsx, *.js, *.jsx)")
	includePy     = flag.Bool("include-py", true, "Include Python files (*.py)")
	limitElements = flag.Int("limit", 0, "Limit number of elements to ingest (0 = no limit)")
)

type BatchIngestRequest struct {
	SpaceID      string            `json:"space_id"`
	Observations []BatchIngestItem `json:"observations"`
}

type BatchIngestItem struct {
	Timestamp string   `json:"timestamp"`
	Source    string   `json:"source"`
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
}

type CodeElement struct {
	Name     string
	Kind     string
	Path     string
	Content  string
	Package  string
	FilePath string
	Tags     []string
	Concerns []string // Cross-cutting concerns detected in this element
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

func main() {
	flag.Parse()

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

	// Collect all code elements
	elements, err := walkCodebase(*codebasePath, excludeSet)
	if err != nil {
		log.Fatalf("Failed to walk codebase: %v", err)
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
	concerns := detectConcerns(relPath, string(content))
	tags := []string{"package", pkgName}
	tags = append(tags, concerns...)

	// Check if this is a configuration file
	kind := "package"
	contentDesc := fmt.Sprintf("Package %s in file %s", pkgName, relPath)
	if isConfigFile(relPath) {
		tags = append(tags, "config")
		kind = "config"
		contentDesc = fmt.Sprintf("Configuration package %s in file %s", pkgName, relPath)
	}

	// Add package-level element
	elements = append(elements, CodeElement{
		Name:     pkgName,
		Kind:     kind,
		Path:     "/" + relPath,
		Content:  contentDesc,
		Package:  pkgName,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
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

	// Build content summary
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("%s file: %s. ", strings.Title(lang), fileName))

	if len(exports) > 0 {
		summary.WriteString(fmt.Sprintf("Exports: %s. ", strings.Join(uniqueStrings(exports), ", ")))
	}
	if len(classes) > 0 {
		summary.WriteString(fmt.Sprintf("Classes: %s. ", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(interfaces) > 0 {
		summary.WriteString(fmt.Sprintf("Interfaces: %s. ", strings.Join(uniqueStrings(interfaces), ", ")))
	}
	if len(functions) > 0 {
		funcList := uniqueStrings(functions)
		if len(funcList) > 10 {
			funcList = funcList[:10]
			summary.WriteString(fmt.Sprintf("Functions: %s (and more). ", strings.Join(funcList, ", ")))
		} else {
			summary.WriteString(fmt.Sprintf("Functions: %s. ", strings.Join(funcList, ", ")))
		}
	}

	// Detect cross-cutting concerns
	concerns := detectConcerns(relPath, contentStr)

	// Build tags including concerns
	tags := []string{lang, fileKind}
	tags = append(tags, concerns...)

	// Check if this is a configuration file
	if isConfigFile(relPath) {
		tags = append(tags, "config")
		fileKind = "config"
		// Enhance summary for config files
		summary.WriteString("Configuration file. ")
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     fileKind,
		Path:     "/" + relPath,
		Content:  summary.String(),
		Package:  lang,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
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

	// Build content summary
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Python file: %s. ", fileName))

	if docstring != "" {
		summary.WriteString(fmt.Sprintf("Docstring: %s ", docstring))
	}
	if len(classes) > 0 {
		summary.WriteString(fmt.Sprintf("Classes: %s. ", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(functions) > 0 {
		funcList := uniqueStrings(functions)
		if len(funcList) > 10 {
			funcList = funcList[:10]
			summary.WriteString(fmt.Sprintf("Functions: %s (and more). ", strings.Join(funcList, ", ")))
		} else {
			summary.WriteString(fmt.Sprintf("Functions: %s. ", strings.Join(funcList, ", ")))
		}
	}
	summary.WriteString(fmt.Sprintf("Imports: %d. ", len(imports)))

	// Detect cross-cutting concerns
	concerns := detectConcerns(relPath, contentStr)
	tags := []string{"python", "module"}
	tags = append(tags, concerns...)

	// Check if this is a configuration file
	pyKind := "python-module"
	if isConfigFile(relPath) {
		tags = append(tags, "config")
		pyKind = "config"
		summary.WriteString("Configuration file. ")
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     pyKind,
		Path:     "/" + relPath,
		Content:  summary.String(),
		Package:  "python",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
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
	return concerns
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

func ingestBatch(client *http.Client, elements []CodeElement) (int, int) {
	items := make([]BatchIngestItem, 0, len(elements))
	timestamp := time.Now().UTC().Format(time.RFC3339)

	for _, elem := range elements {
		items = append(items, BatchIngestItem{
			Timestamp: timestamp,
			Source:    "codebase-ingest",
			Name:      elem.Name,
			Path:      elem.Path,
			Content:   elem.Content,
			Tags:      elem.Tags,
		})
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
