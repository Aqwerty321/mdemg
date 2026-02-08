// Package symbols provides AST-based symbol extraction and Neo4j storage.
package symbols

import "context"

// GoTypesAnalyzer performs deep type analysis using go/types.
// Currently a stub — golang.org/x/tools is not yet in go.mod.
type GoTypesAnalyzer struct{}

// NewGoTypesAnalyzer creates a new go/types analyzer.
func NewGoTypesAnalyzer() *GoTypesAnalyzer {
	return &GoTypesAnalyzer{}
}

// AnalyzeImplements discovers interface implementations via go/types.
// Stub: returns nil until golang.org/x/tools is added to go.mod.
func (a *GoTypesAnalyzer) AnalyzeImplements(ctx context.Context, spaceID, projectRoot string) ([]RelationshipRecord, error) {
	return nil, nil
}
