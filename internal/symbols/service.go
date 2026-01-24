// Package symbols provides AST-based symbol extraction for code files.
// This file defines the service interface that plugins can use to access
// the core symbol parser without re-parsing files.
package symbols

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

// Service provides symbol parsing and querying capabilities.
// Plugins can use this service to extract symbols from code files
// or query previously indexed symbols.
type Service struct {
	parser *Parser
	cache  *symbolCache
	mu     sync.RWMutex
}

// NewService creates a new symbol service with the given configuration.
func NewService(config ParserConfig) (*Service, error) {
	parser, err := NewParser(config)
	if err != nil {
		return nil, err
	}
	return &Service{
		parser: parser,
		cache:  newSymbolCache(),
	}, nil
}

// Close releases resources held by the service.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parser != nil {
		s.parser.Close()
	}
}

// ParseFile extracts symbols from a single file.
// Results are cached for subsequent queries.
func (s *Service) ParseFile(ctx context.Context, filePath string) (*FileSymbols, error) {
	s.mu.RLock()
	if cached := s.cache.get(filePath); cached != nil {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	result, err := s.parser.ParseFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache.set(filePath, result)
	s.mu.Unlock()

	return result, nil
}

// ParseDirectory extracts symbols from all supported files in a directory.
// Results are cached for subsequent queries.
func (s *Service) ParseDirectory(ctx context.Context, dir string) ([]*FileSymbols, error) {
	results, err := s.parser.ParseDirectory(ctx, dir)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	for _, fs := range results {
		s.cache.set(fs.FilePath, fs)
	}
	s.mu.Unlock()

	return results, nil
}

// QueryRequest specifies search parameters for symbol lookup.
type QueryRequest struct {
	// Query is a symbol name pattern (supports prefix matching).
	Query string

	// FilePath limits search to a specific file.
	FilePath string

	// SymbolTypes filters by symbol type (const, function, etc.).
	SymbolTypes []SymbolType

	// Language filters by programming language.
	Language Language

	// ExportedOnly returns only exported/public symbols.
	ExportedOnly bool

	// Limit is the maximum number of results (default: 50).
	Limit int
}

// QueryResponse contains matching symbols.
type QueryResponse struct {
	Symbols    []Symbol `json:"symbols"`
	TotalCount int      `json:"total_count"`
}

// Query searches cached symbols based on the request parameters.
// This is the primary interface for plugins to find symbols.
func (s *Service) Query(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if req.Limit <= 0 {
		req.Limit = 50
	}

	var matches []Symbol
	typeSet := make(map[SymbolType]bool)
	for _, t := range req.SymbolTypes {
		typeSet[t] = true
	}

	// Iterate over cached files
	for _, fs := range s.cache.all() {
		// Filter by file path if specified
		if req.FilePath != "" && fs.FilePath != req.FilePath {
			continue
		}

		// Filter by language if specified
		if req.Language != "" && fs.Language != req.Language {
			continue
		}

		for _, sym := range fs.Symbols {
			// Filter by exported status
			if req.ExportedOnly && !sym.Exported {
				continue
			}

			// Filter by symbol type
			if len(typeSet) > 0 && !typeSet[sym.Type] {
				continue
			}

			// Filter by name pattern (prefix match)
			if req.Query != "" && !matchesPattern(sym.Name, req.Query) {
				continue
			}

			matches = append(matches, sym)
		}
	}

	totalCount := len(matches)
	if len(matches) > req.Limit {
		matches = matches[:req.Limit]
	}

	return &QueryResponse{
		Symbols:    matches,
		TotalCount: totalCount,
	}, nil
}

// FindByName looks up a symbol by exact name.
// Returns all matching symbols across all cached files.
func (s *Service) FindByName(ctx context.Context, name string) ([]Symbol, error) {
	resp, err := s.Query(ctx, QueryRequest{
		Query:        name,
		ExportedOnly: false,
		Limit:        100,
	})
	if err != nil {
		return nil, err
	}

	// Filter for exact matches
	var exact []Symbol
	for _, sym := range resp.Symbols {
		if sym.Name == name {
			exact = append(exact, sym)
		}
	}
	return exact, nil
}

// FindConstants returns all constant symbols matching the pattern.
func (s *Service) FindConstants(ctx context.Context, pattern string) ([]Symbol, error) {
	resp, err := s.Query(ctx, QueryRequest{
		Query:        pattern,
		SymbolTypes:  []SymbolType{SymbolTypeConst},
		ExportedOnly: true,
		Limit:        100,
	})
	if err != nil {
		return nil, err
	}
	return resp.Symbols, nil
}

// FindFunctions returns all function/method symbols matching the pattern.
func (s *Service) FindFunctions(ctx context.Context, pattern string) ([]Symbol, error) {
	resp, err := s.Query(ctx, QueryRequest{
		Query:        pattern,
		SymbolTypes:  []SymbolType{SymbolTypeFunction, SymbolTypeMethod},
		ExportedOnly: true,
		Limit:        100,
	})
	if err != nil {
		return nil, err
	}
	return resp.Symbols, nil
}

// GetFileSymbols returns all symbols from a specific file.
// If the file hasn't been parsed, it will be parsed now.
func (s *Service) GetFileSymbols(ctx context.Context, filePath string) (*FileSymbols, error) {
	s.mu.RLock()
	if cached := s.cache.get(filePath); cached != nil {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	// Parse the file if not cached
	return s.ParseFile(ctx, filePath)
}

// InvalidateCache removes cached data for a file.
// Call this when a file is modified.
func (s *Service) InvalidateCache(filePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache.delete(filePath)
}

// ClearCache removes all cached symbol data.
func (s *Service) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache.clear()
}

// CacheStats returns statistics about the symbol cache.
func (s *Service) CacheStats() (fileCount int, symbolCount int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, fs := range s.cache.all() {
		fileCount++
		symbolCount += len(fs.Symbols)
	}
	return
}

// matchesPattern checks if a symbol name matches a query pattern.
// Supports prefix matching and case-insensitive search.
func matchesPattern(name, pattern string) bool {
	// Case-insensitive prefix match
	return strings.HasPrefix(strings.ToLower(name), strings.ToLower(pattern))
}

// symbolCache is a simple in-memory cache for parsed symbols.
type symbolCache struct {
	data map[string]*FileSymbols
	mu   sync.RWMutex
}

func newSymbolCache() *symbolCache {
	return &symbolCache{
		data: make(map[string]*FileSymbols),
	}
}

func (c *symbolCache) get(filePath string) *FileSymbols {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Normalize path for consistent lookups
	return c.data[filepath.Clean(filePath)]
}

func (c *symbolCache) set(filePath string, fs *FileSymbols) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[filepath.Clean(filePath)] = fs
}

func (c *symbolCache) delete(filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, filepath.Clean(filePath))
}

func (c *symbolCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]*FileSymbols)
}

func (c *symbolCache) all() []*FileSymbols {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*FileSymbols, 0, len(c.data))
	for _, fs := range c.data {
		result = append(result, fs)
	}
	return result
}
