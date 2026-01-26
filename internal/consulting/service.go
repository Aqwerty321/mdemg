// Package consulting provides the Agent Consulting Service.
// This service acts as an SME (Subject Matter Expert) for coding agents,
// providing context-aware suggestions based on accumulated knowledge.
package consulting

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/models"
	"mdemg/internal/retrieval"
	"mdemg/internal/symbols"
)

// Service provides the Agent Consulting API.
type Service struct {
	cfg         config.Config
	driver      neo4j.DriverWithContext
	retriever   *retrieval.Service
	embedder    embeddings.Embedder
	symbolStore *symbols.Store
}

// NewService creates a new consulting service.
func NewService(cfg config.Config, driver neo4j.DriverWithContext, retriever *retrieval.Service, embedder embeddings.Embedder, symbolStore *symbols.Store) *Service {
	return &Service{
		cfg:         cfg,
		driver:      driver,
		retriever:   retriever,
		embedder:    embedder,
		symbolStore: symbolStore,
	}
}

// Consult processes a consultation request and returns SME suggestions.
func (s *Service) Consult(ctx context.Context, req models.ConsultRequest) (models.ConsultResponse, error) {
	resp := models.ConsultResponse{
		SpaceID:     req.SpaceID,
		Suggestions: []models.Suggestion{},
		Debug:       make(map[string]any),
	}

	maxSuggestions := req.MaxSuggestions
	if maxSuggestions <= 0 {
		maxSuggestions = 5
	}

	// Combine context and question for retrieval
	queryText := fmt.Sprintf("%s\n\nQuestion: %s", req.Context, req.Question)

	// Step 1: Retrieve relevant memories using existing retrieval pipeline
	retrieveReq := models.RetrieveRequest{
		SpaceID:   req.SpaceID,
		QueryText: queryText,
		TopK:      maxSuggestions * 3, // Get more candidates for filtering
		HopDepth:  2,                  // Include related nodes
	}

	retrieveResp, err := s.retriever.Retrieve(ctx, retrieveReq)
	if err != nil {
		return resp, fmt.Errorf("retrieval failed: %w", err)
	}

	resp.Debug["retrieved_count"] = len(retrieveResp.Results)

	// Step 2: Fetch concept nodes (higher layers) for related concepts
	concepts, err := s.fetchRelatedConcepts(ctx, req.SpaceID, retrieveResp.Results)
	if err != nil {
		// Log but don't fail - concepts are optional enrichment
		resp.Debug["concept_error"] = err.Error()
	} else {
		resp.RelatedConcepts = concepts
	}

	// Step 3: Generate suggestions from retrieved results
	suggestions, err := s.generateSuggestions(ctx, req, retrieveResp.Results)
	if err != nil {
		return resp, fmt.Errorf("suggestion generation failed: %w", err)
	}

	// Limit to max suggestions
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}

	// Step 4: Optionally enrich with symbol evidence
	if req.IncludeEvidence && s.symbolStore != nil {
		for i := range suggestions {
			for _, nodeID := range suggestions[i].SourceNodes {
				syms, err := s.symbolStore.GetSymbolsForMemoryNode(ctx, req.SpaceID, nodeID)
				if err != nil {
					continue
				}
				for _, sym := range syms {
					suggestions[i].Evidence = append(suggestions[i].Evidence, models.SymbolEvidence{
						SymbolName: sym.Name,
						SymbolType: sym.SymbolType,
						FilePath:   sym.FilePath,
						LineNumber: sym.LineNumber,
						EndLine:    sym.EndLine,
						Value:      sym.Value,
						RawValue:   sym.RawValue,
						Signature:  sym.Signature,
						DocComment: sym.DocComment,
					})
				}
			}
		}
	}

	resp.Suggestions = suggestions

	// Step 5: Calculate overall confidence and rationale
	resp.Confidence = s.calculateOverallConfidence(suggestions, len(retrieveResp.Results))
	resp.Rationale = s.generateRationale(req, suggestions, concepts)

	return resp, nil
}

// fetchRelatedConcepts retrieves concept nodes (layer >= 2) related to the results.
func (s *Service) fetchRelatedConcepts(ctx context.Context, spaceID string, results []models.RetrieveResult) ([]models.RelatedConcept, error) {
	if len(results) == 0 {
		return nil, nil
	}

	// Collect node IDs from results
	nodeIDs := make([]string, 0, len(results))
	for _, r := range results {
		nodeIDs = append(nodeIDs, r.NodeID)
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Find concept nodes connected to the retrieved nodes
		cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})-[:ABSTRACTS_TO*1..3]->(c:MemoryNode {space_id: $spaceId})
WHERE n.node_id IN $nodeIds AND c.layer >= 2
WITH c, count(DISTINCT n) AS relevance_count
RETURN c.node_id AS node_id,
       coalesce(c.name, c.node_id) AS name,
       c.summary AS summary,
       c.layer AS layer,
       toFloat(relevance_count) / toFloat(size($nodeIds)) AS relevance
ORDER BY relevance DESC, c.layer ASC
LIMIT 5`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var concepts []models.RelatedConcept
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			layer, _ := rec.Get("layer")
			relevance, _ := rec.Get("relevance")

			concept := models.RelatedConcept{
				NodeID:    fmt.Sprint(nodeID),
				Name:      fmt.Sprint(name),
				Layer:     int(layer.(int64)),
				Relevance: relevance.(float64),
			}
			if summary != nil {
				concept.Summary = fmt.Sprint(summary)
			}
			concepts = append(concepts, concept)
		}
		return concepts, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]models.RelatedConcept), nil
}

// generateSuggestions creates categorized suggestions from retrieved results.
func (s *Service) generateSuggestions(ctx context.Context, req models.ConsultRequest, results []models.RetrieveResult) ([]models.Suggestion, error) {
	var suggestions []models.Suggestion

	// Categorize results and generate suggestions
	for _, r := range results {
		suggestionType := s.classifySuggestionType(r)
		content := s.formatSuggestionContent(suggestionType, r, req.Question)

		if content == "" {
			continue
		}

		suggestions = append(suggestions, models.Suggestion{
			Type:        suggestionType,
			Content:     content,
			Confidence:  r.Score, // Use retrieval score as base confidence
			SourceNodes: []string{r.NodeID},
		})
	}

	// Sort by confidence and deduplicate similar suggestions
	suggestions = s.deduplicateSuggestions(suggestions)

	return suggestions, nil
}

// classifySuggestionType determines the type of suggestion based on result characteristics.
func (s *Service) classifySuggestionType(r models.RetrieveResult) models.SuggestionType {
	name := strings.ToLower(r.Name)
	path := strings.ToLower(r.Path)
	summary := strings.ToLower(r.Summary)

	// Risk indicators
	riskKeywords := []string{"error", "fail", "issue", "problem", "bug", "fix", "workaround", "hack", "todo", "deprecated"}
	for _, kw := range riskKeywords {
		if strings.Contains(name, kw) || strings.Contains(summary, kw) {
			return models.SuggestionRisk
		}
	}

	// Process indicators (workflows, procedures)
	processKeywords := []string{"workflow", "process", "step", "guide", "howto", "readme", "doc", "procedure"}
	for _, kw := range processKeywords {
		if strings.Contains(name, kw) || strings.Contains(path, kw) {
			return models.SuggestionProcess
		}
	}

	// Concept indicators (patterns, architecture)
	conceptKeywords := []string{"pattern", "architecture", "design", "concept", "abstract", "interface", "protocol"}
	for _, kw := range conceptKeywords {
		if strings.Contains(name, kw) || strings.Contains(path, kw) || strings.Contains(summary, kw) {
			return models.SuggestionConcept
		}
	}

	// Default to context
	return models.SuggestionContext
}

// formatSuggestionContent formats the suggestion text based on type.
func (s *Service) formatSuggestionContent(suggType models.SuggestionType, r models.RetrieveResult, question string) string {
	// Use summary if available, otherwise use name
	content := r.Summary
	if content == "" {
		content = r.Name
	}
	if content == "" {
		return ""
	}

	// Truncate long content
	if len(content) > 500 {
		content = content[:497] + "..."
	}

	switch suggType {
	case models.SuggestionContext:
		return fmt.Sprintf("Based on this codebase's patterns: %s", content)
	case models.SuggestionProcess:
		return fmt.Sprintf("The typical workflow for this type of change: %s", content)
	case models.SuggestionConcept:
		return fmt.Sprintf("This relates to the higher-level principle: %s", content)
	case models.SuggestionRisk:
		return fmt.Sprintf("Caution - previous related finding: %s", content)
	default:
		return content
	}
}

// deduplicateSuggestions removes similar suggestions and sorts by confidence.
func (s *Service) deduplicateSuggestions(suggestions []models.Suggestion) []models.Suggestion {
	if len(suggestions) <= 1 {
		return suggestions
	}

	// Simple deduplication by checking content similarity
	seen := make(map[string]bool)
	var unique []models.Suggestion

	for _, sugg := range suggestions {
		// Create a simple key from first 50 chars of content
		key := sugg.Content
		if len(key) > 50 {
			key = key[:50]
		}
		key = strings.ToLower(key)

		if !seen[key] {
			seen[key] = true
			unique = append(unique, sugg)
		}
	}

	return unique
}

// calculateOverallConfidence computes an overall confidence score.
func (s *Service) calculateOverallConfidence(suggestions []models.Suggestion, totalRetrieved int) float64 {
	if len(suggestions) == 0 {
		return 0.0
	}

	// Average confidence of suggestions, weighted by retrieval coverage
	var sum float64
	for _, sugg := range suggestions {
		sum += sugg.Confidence
	}
	avgConfidence := sum / float64(len(suggestions))

	// Boost if we found many relevant results
	coverageBoost := 1.0
	if totalRetrieved >= 5 {
		coverageBoost = 1.1
	}
	if totalRetrieved >= 10 {
		coverageBoost = 1.2
	}

	confidence := avgConfidence * coverageBoost
	if confidence > 1.0 {
		confidence = 1.0
	}
	return confidence
}

// generateRationale creates an explanation for the suggestions.
func (s *Service) generateRationale(req models.ConsultRequest, suggestions []models.Suggestion, concepts []models.RelatedConcept) string {
	if len(suggestions) == 0 {
		return "No relevant patterns found in the knowledge base for this query."
	}

	// Count suggestion types
	typeCounts := make(map[models.SuggestionType]int)
	for _, sugg := range suggestions {
		typeCounts[sugg.Type]++
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Found %d relevant patterns", len(suggestions)))

	if typeCounts[models.SuggestionContext] > 0 {
		parts = append(parts, fmt.Sprintf("%d context patterns", typeCounts[models.SuggestionContext]))
	}
	if typeCounts[models.SuggestionRisk] > 0 {
		parts = append(parts, fmt.Sprintf("%d risk warnings", typeCounts[models.SuggestionRisk]))
	}
	if typeCounts[models.SuggestionProcess] > 0 {
		parts = append(parts, fmt.Sprintf("%d process guidelines", typeCounts[models.SuggestionProcess]))
	}
	if typeCounts[models.SuggestionConcept] > 0 {
		parts = append(parts, fmt.Sprintf("%d related concepts", typeCounts[models.SuggestionConcept]))
	}

	if len(concepts) > 0 {
		parts = append(parts, fmt.Sprintf("connected to %d higher-level abstractions", len(concepts)))
	}

	return strings.Join(parts, ", ") + "."
}
