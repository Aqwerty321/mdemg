// Package symbols provides AST-based symbol extraction for code files.
// This file defines the provider interface that plugins use to access
// symbol extraction capabilities from the core.
package symbols

import (
	"context"
)

// Provider is the interface that plugins use to access symbol extraction.
// This is the contract between plugins and the core symbol service.
type Provider interface {
	// ParseFile extracts symbols from a single file.
	ParseFile(ctx context.Context, filePath string) (*FileSymbols, error)

	// Query searches for symbols based on request parameters.
	Query(ctx context.Context, req QueryRequest) (*QueryResponse, error)

	// FindByName looks up symbols by exact name.
	FindByName(ctx context.Context, name string) ([]Symbol, error)

	// FindConstants returns constant symbols matching a pattern.
	FindConstants(ctx context.Context, pattern string) ([]Symbol, error)

	// FindFunctions returns function/method symbols matching a pattern.
	FindFunctions(ctx context.Context, pattern string) ([]Symbol, error)

	// GetFileSymbols returns all symbols from a specific file (cached).
	GetFileSymbols(ctx context.Context, filePath string) (*FileSymbols, error)
}

// Ensure Service implements Provider.
var _ Provider = (*Service)(nil)

// NoOpProvider is a Provider implementation that returns empty results.
// Used when symbol extraction is disabled or unavailable.
type NoOpProvider struct{}

// Ensure NoOpProvider implements Provider.
var _ Provider = (*NoOpProvider)(nil)

// ParseFile returns empty results.
func (p *NoOpProvider) ParseFile(ctx context.Context, filePath string) (*FileSymbols, error) {
	return &FileSymbols{
		FilePath: filePath,
		Language: LangUnknown,
		Symbols:  []Symbol{},
	}, nil
}

// Query returns empty results.
func (p *NoOpProvider) Query(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	return &QueryResponse{
		Symbols:    []Symbol{},
		TotalCount: 0,
	}, nil
}

// FindByName returns empty results.
func (p *NoOpProvider) FindByName(ctx context.Context, name string) ([]Symbol, error) {
	return []Symbol{}, nil
}

// FindConstants returns empty results.
func (p *NoOpProvider) FindConstants(ctx context.Context, pattern string) ([]Symbol, error) {
	return []Symbol{}, nil
}

// FindFunctions returns empty results.
func (p *NoOpProvider) FindFunctions(ctx context.Context, pattern string) ([]Symbol, error) {
	return []Symbol{}, nil
}

// GetFileSymbols returns empty results.
func (p *NoOpProvider) GetFileSymbols(ctx context.Context, filePath string) (*FileSymbols, error) {
	return &FileSymbols{
		FilePath: filePath,
		Language: LangUnknown,
		Symbols:  []Symbol{},
	}, nil
}

// ToEvidence converts a Symbol to a format suitable for retrieval responses.
// This helper makes it easy for plugins to include symbol evidence.
func ToEvidence(sym Symbol) SymbolEvidence {
	return SymbolEvidence{
		Name:           sym.Name,
		Type:           string(sym.Type),
		Value:          sym.Value,
		Line:           sym.Line, // UPTS standard: 1-indexed line number
		Snippet:        sym.Snippet,
		DocComment:     sym.DocComment,
		Signature:      sym.Signature,
		TypeAnnotation: sym.TypeAnnotation,
	}
}

// ToEvidenceList converts multiple Symbols to evidence format.
func ToEvidenceList(symbols []Symbol) []SymbolEvidence {
	result := make([]SymbolEvidence, len(symbols))
	for i, sym := range symbols {
		result[i] = ToEvidence(sym)
	}
	return result
}

// SymbolEvidence represents a code symbol in a format suitable for retrieval responses.
// This mirrors models.SymbolEvidence but is defined here to avoid circular imports.
// Field names follow UPTS (Universal Parser Test Specification) v1.0.0
type SymbolEvidence struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Value          string `json:"value,omitempty"`
	Line           int    `json:"line"`           // 1-indexed start line (UPTS standard)
	Snippet        string `json:"snippet,omitempty"`
	DocComment     string `json:"doc_comment,omitempty"`
	Signature      string `json:"signature,omitempty"`
	TypeAnnotation string `json:"type_annotation,omitempty"`
}
