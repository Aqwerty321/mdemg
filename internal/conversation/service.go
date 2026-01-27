package conversation

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// LearningService defines the interface for the learning service
// This allows dependency injection without circular imports
type LearningService interface {
	CoactivateSession(ctx context.Context, spaceID, sessionID string) error
}

// Service handles conversation observation capture and surprise detection
type Service struct {
	driver           neo4j.DriverWithContext
	embedder         Embedder
	surpriseDetector *SurpriseDetector
	learningService  LearningService
}

// NewService creates a new conversation service
func NewService(driver neo4j.DriverWithContext, embedder Embedder) *Service {
	var surpriseDet *SurpriseDetector
	if embedder != nil {
		surpriseDet = NewSurpriseDetector(embedder, driver)
	}

	return &Service{
		driver:           driver,
		embedder:         embedder,
		surpriseDetector: surpriseDet,
		learningService:  nil, // Set via SetLearningService to avoid circular dependency
	}
}

// SetLearningService injects the learning service (to avoid circular imports)
func (s *Service) SetLearningService(learningService LearningService) {
	s.learningService = learningService
}

// ObserveRequest is the request for capturing an observation
type ObserveRequest struct {
	SpaceID   string         `json:"space_id"`
	SessionID string         `json:"session_id"`
	Content   string         `json:"content"`
	ObsType   string         `json:"obs_type,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ObserveResponse is the response from capturing an observation
type ObserveResponse struct {
	ObsID           string             `json:"obs_id"`
	NodeID          string             `json:"node_id"`
	SurpriseScore   float64            `json:"surprise_score"`
	SurpriseFactors map[string]float64 `json:"surprise_factors"`
	Summary         string             `json:"summary,omitempty"`
}

// CorrectRequest is the request for capturing an explicit correction
type CorrectRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
	Incorrect string `json:"incorrect"`
	Correct   string `json:"correct"`
	Context   string `json:"context,omitempty"`
}

// Observe captures a conversation observation
func (s *Service) Observe(ctx context.Context, req ObserveRequest) (*ObserveResponse, error) {
	// Generate unique IDs
	obsID := uuid.New().String()
	nodeID := uuid.New().String()

	// Validate observation type
	obsType := ObservationType(req.ObsType)
	if req.ObsType == "" {
		obsType = ObsTypeLearning // Default type
	}

	// Validate observation type value
	validTypes := map[ObservationType]bool{
		ObsTypeDecision:   true,
		ObsTypeCorrection: true,
		ObsTypeLearning:   true,
		ObsTypePreference: true,
		ObsTypeError:      true,
		ObsTypeTask:       true,
	}
	if !validTypes[obsType] {
		return nil, fmt.Errorf("invalid observation type: %s", req.ObsType)
	}

	// Generate embedding
	var embedding []float32
	var err error
	if s.embedder != nil {
		embedding, err = s.embedder.Embed(ctx, req.Content)
		if err != nil {
			log.Printf("WARNING: failed to generate embedding for observation: %v", err)
			// Continue without embedding
			embedding = nil
		}
	}

	// Create observation object
	obs := Observation{
		ObsID:     obsID,
		SpaceID:   req.SpaceID,
		SessionID: req.SessionID,
		ObsType:   obsType,
		Content:   req.Content,
		Embedding: embedding,
		Tags:      req.Tags,
		Metadata:  req.Metadata,
		CreatedAt: time.Now().UTC(),
	}

	// Generate summary (first 200 chars)
	obs.Summary = generateSummary(req.Content)

	// Compute surprise score
	var surpriseScore float64
	var factors SurpriseFactors

	if s.surpriseDetector != nil {
		surpriseScore, factors, err = s.surpriseDetector.DetectSurprise(ctx, obs)
		if err != nil {
			log.Printf("WARNING: failed to compute surprise score: %v", err)
			// Continue with default score
			surpriseScore = 0.0
		}
	}

	obs.SurpriseScore = surpriseScore

	// Build tags
	tags := buildObservationTags(req, obsType)

	// Create MemoryNode in Neo4j
	err = s.createObservationNode(ctx, nodeID, obs, tags)
	if err != nil {
		return nil, fmt.Errorf("create observation node: %w", err)
	}

	log.Printf("Created conversation observation %s (node=%s, type=%s, surprise=%.2f)",
		obsID, nodeID, obsType, surpriseScore)

	// Trigger session-based coactivation if learning service is available
	// This creates CO_ACTIVATED_WITH edges between observations in the same session
	if s.learningService != nil && req.SessionID != "" {
		err = s.learningService.CoactivateSession(ctx, req.SpaceID, req.SessionID)
		if err != nil {
			// Log but don't fail - coactivation is a learning enhancement, not critical
			log.Printf("WARNING: failed to coactivate session %s: %v", req.SessionID, err)
		}
	}

	return &ObserveResponse{
		ObsID:         obsID,
		NodeID:        nodeID,
		SurpriseScore: surpriseScore,
		SurpriseFactors: map[string]float64{
			"term_novelty":        factors.TermNovelty,
			"contradiction_score": factors.ContradictionScore,
			"correction_score":    factors.CorrectionScore,
			"embedding_novelty":   factors.EmbeddingNovelty,
		},
		Summary: obs.Summary,
	}, nil
}

// Correct captures an explicit correction (sets high surprise)
func (s *Service) Correct(ctx context.Context, req CorrectRequest) (*ObserveResponse, error) {
	// Build content string
	content := fmt.Sprintf("CORRECTION: Incorrect: %s | Correct: %s", req.Incorrect, req.Correct)
	if req.Context != "" {
		content += fmt.Sprintf(" | Context: %s", req.Context)
	}

	// Create observation with correction type
	obsReq := ObserveRequest{
		SpaceID:   req.SpaceID,
		SessionID: req.SessionID,
		Content:   content,
		ObsType:   string(ObsTypeCorrection),
		Tags:      []string{"correction"},
		Metadata: map[string]any{
			"incorrect": req.Incorrect,
			"correct":   req.Correct,
			"context":   req.Context,
		},
	}

	resp, err := s.Observe(ctx, obsReq)
	if err != nil {
		return nil, err
	}

	// Ensure high surprise score for corrections
	if resp.SurpriseScore < 0.9 {
		resp.SurpriseScore = 0.9
		// Update node with corrected surprise score
		err = s.updateSurpriseScore(ctx, resp.NodeID, 0.9)
		if err != nil {
			log.Printf("WARNING: failed to update surprise score: %v", err)
		}
	}

	return resp, nil
}

// createObservationNode creates a MemoryNode with role_type="conversation_observation"
func (s *Service) createObservationNode(ctx context.Context, nodeID string, obs Observation, tags []string) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			CREATE (n:MemoryNode {
				node_id: $nodeId,
				space_id: $spaceId,
				role_type: 'conversation_observation',
				obs_id: $obsId,
				session_id: $sessionId,
				obs_type: $obsType,
				content: $content,
				summary: $summary,
				embedding: $embedding,
				surprise_score: $surpriseScore,
				tags: $tags,
				layer: 0,
				created_at: datetime($createdAt),
				updated_at: datetime($createdAt)
			})
			RETURN n.node_id as nodeId
		`

		params := map[string]any{
			"nodeId":        nodeID,
			"spaceId":       obs.SpaceID,
			"obsId":         obs.ObsID,
			"sessionId":     obs.SessionID,
			"obsType":       string(obs.ObsType),
			"content":       obs.Content,
			"summary":       obs.Summary,
			"embedding":     obs.Embedding,
			"surpriseScore": obs.SurpriseScore,
			"tags":          tags,
			"createdAt":     obs.CreatedAt.Format(time.RFC3339),
		}

		// Add metadata as properties if present
		if obs.Metadata != nil && len(obs.Metadata) > 0 {
			for k, v := range obs.Metadata {
				params["metadata_"+k] = v
			}
		}

		result, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		return result.Consume(ctx)
	})

	return err
}

// updateSurpriseScore updates the surprise score for a node
func (s *Service) updateSurpriseScore(ctx context.Context, nodeID string, score float64) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {node_id: $nodeId})
			SET n.surprise_score = $score,
			    n.updated_at = datetime($updatedAt)
			RETURN n.node_id as nodeId
		`

		params := map[string]any{
			"nodeId":    nodeID,
			"score":     score,
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		}

		result, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		return result.Consume(ctx)
	})

	return err
}

// generateSummary creates a brief summary from content (max 200 chars)
func generateSummary(content string) string {
	const maxLen = 200

	// Clean whitespace
	content = strings.TrimSpace(content)
	content = strings.Join(strings.Fields(content), " ")

	if len(content) <= maxLen {
		return content
	}

	// Truncate at word boundary
	truncated := content[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// buildObservationTags builds the tag list for an observation
func buildObservationTags(req ObserveRequest, obsType ObservationType) []string {
	tags := []string{
		"conversation",
		"session:" + req.SessionID,
		"obs_type:" + string(obsType),
	}

	// Add custom tags
	if req.Tags != nil {
		tags = append(tags, req.Tags...)
	}

	return tags
}

// =============================================================================
// PHASE 5: RESUME AND RECALL
// =============================================================================

// ResumeRequest is the request for resuming context after compaction
type ResumeRequest struct {
	SpaceID          string `json:"space_id"`
	SessionID        string `json:"session_id,omitempty"`
	IncludeTasks     bool   `json:"include_tasks,omitempty"`
	IncludeDecisions bool   `json:"include_decisions,omitempty"`
	IncludeLearnings bool   `json:"include_learnings,omitempty"`
	MaxObservations  int    `json:"max_observations,omitempty"`
}

// ResumeResponse is the response from resuming context
type ResumeResponse struct {
	SpaceID          string                   `json:"space_id"`
	SessionID        string                   `json:"session_id,omitempty"`
	Observations     []ObservationResult      `json:"observations"`
	Themes           []ThemeResult            `json:"themes"`
	EmergentConcepts []EmergentConceptResult  `json:"emergent_concepts"`
	Summary          string                   `json:"summary"`
	Debug            map[string]any           `json:"debug,omitempty"`
}

// ObservationResult represents an observation in resume/recall responses
type ObservationResult struct {
	NodeID        string   `json:"node_id"`
	ObsType       string   `json:"obs_type"`
	Content       string   `json:"content"`
	Summary       string   `json:"summary"`
	SessionID     string   `json:"session_id"`
	SurpriseScore float64  `json:"surprise_score"`
	Score         float64  `json:"score,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

// ThemeResult represents a conversation theme in resume/recall responses
type ThemeResult struct {
	NodeID           string  `json:"node_id"`
	Name             string  `json:"name"`
	Summary          string  `json:"summary"`
	MemberCount      int     `json:"member_count"`
	DominantObsType  string  `json:"dominant_obs_type,omitempty"`
	AvgSurpriseScore float64 `json:"avg_surprise_score"`
	Score            float64 `json:"score,omitempty"`
}

// EmergentConceptResult represents an emergent concept in resume/recall responses
type EmergentConceptResult struct {
	NodeID       string   `json:"node_id"`
	Name         string   `json:"name"`
	Summary      string   `json:"summary"`
	Layer        int      `json:"layer"`
	Keywords     []string `json:"keywords,omitempty"`
	SessionCount int      `json:"session_count,omitempty"`
	Score        float64  `json:"score,omitempty"`
}

// RecallRequest is the request for recalling conversation knowledge
type RecallRequest struct {
	SpaceID         string    `json:"space_id"`
	Query           string    `json:"query"`
	QueryEmbedding  []float32 `json:"query_embedding,omitempty"`
	TopK            int       `json:"top_k,omitempty"`
	IncludeThemes   bool      `json:"include_themes,omitempty"`
	IncludeConcepts bool      `json:"include_concepts,omitempty"`
}

// RecallResponse is the response from conversation recall
type RecallResponse struct {
	SpaceID string         `json:"space_id"`
	Query   string         `json:"query"`
	Results []RecallResult `json:"results"`
	Debug   map[string]any `json:"debug,omitempty"`
}

// RecallResult represents a single result from conversation recall
type RecallResult struct {
	Type     string         `json:"type"`
	NodeID   string         `json:"node_id"`
	Content  string         `json:"content"`
	Score    float64        `json:"score"`
	Layer    int            `json:"layer"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Resume restores context after context compaction by retrieving
// recent observations, themes, and emergent concepts.
func (s *Service) Resume(ctx context.Context, req ResumeRequest) (*ResumeResponse, error) {
	if req.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}

	maxObs := req.MaxObservations
	if maxObs <= 0 {
		maxObs = 20
	}

	resp := &ResumeResponse{
		SpaceID:          req.SpaceID,
		SessionID:        req.SessionID,
		Observations:     []ObservationResult{},
		Themes:           []ThemeResult{},
		EmergentConcepts: []EmergentConceptResult{},
		Debug:            make(map[string]any),
	}

	// Step 1: Fetch recent observations
	observations, err := s.fetchRecentObservations(ctx, req.SpaceID, req.SessionID, maxObs, req.IncludeTasks, req.IncludeDecisions, req.IncludeLearnings)
	if err != nil {
		return nil, fmt.Errorf("fetch recent observations: %w", err)
	}
	resp.Observations = observations
	resp.Debug["observation_count"] = len(observations)

	// Step 2: Fetch related themes (themes that these observations belong to)
	themes, err := s.fetchRelatedThemes(ctx, req.SpaceID, req.SessionID)
	if err != nil {
		log.Printf("WARNING: failed to fetch related themes: %v", err)
	} else {
		resp.Themes = themes
		resp.Debug["theme_count"] = len(themes)
	}

	// Step 3: Fetch emergent concepts (higher-level abstractions)
	concepts, err := s.fetchEmergentConcepts(ctx, req.SpaceID)
	if err != nil {
		log.Printf("WARNING: failed to fetch emergent concepts: %v", err)
	} else {
		resp.EmergentConcepts = concepts
		resp.Debug["concept_count"] = len(concepts)
	}

	// Step 4: Generate context summary
	resp.Summary = s.generateResumeSummary(resp)

	return resp, nil
}

// Recall retrieves relevant conversation knowledge via semantic query
// Uses vector similarity and spreading activation through the concept hierarchy.
func (s *Service) Recall(ctx context.Context, req RecallRequest) (*RecallResponse, error) {
	if req.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}
	if req.Query == "" && len(req.QueryEmbedding) == 0 {
		return nil, fmt.Errorf("query or query_embedding is required")
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}

	resp := &RecallResponse{
		SpaceID: req.SpaceID,
		Query:   req.Query,
		Results: []RecallResult{},
		Debug:   make(map[string]any),
	}

	// Get or generate query embedding
	var embedding []float32
	if len(req.QueryEmbedding) > 0 {
		embedding = req.QueryEmbedding
	} else if s.embedder != nil {
		var err error
		embedding, err = s.embedder.Embed(ctx, req.Query)
		if err != nil {
			return nil, fmt.Errorf("generate embedding: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no embedding provided and embedder not available")
	}

	// Step 1: Find similar conversation observations
	obsResults, err := s.findSimilarObservations(ctx, req.SpaceID, embedding, topK)
	if err != nil {
		return nil, fmt.Errorf("find similar observations: %w", err)
	}
	resp.Results = append(resp.Results, obsResults...)
	resp.Debug["observation_matches"] = len(obsResults)

	// Step 2: Find similar themes (if enabled)
	if req.IncludeThemes {
		themeResults, err := s.findSimilarThemes(ctx, req.SpaceID, embedding, topK)
		if err != nil {
			log.Printf("WARNING: failed to find similar themes: %v", err)
		} else {
			resp.Results = append(resp.Results, themeResults...)
			resp.Debug["theme_matches"] = len(themeResults)
		}
	}

	// Step 3: Find similar emergent concepts (if enabled)
	if req.IncludeConcepts {
		conceptResults, err := s.findSimilarConcepts(ctx, req.SpaceID, embedding, topK)
		if err != nil {
			log.Printf("WARNING: failed to find similar concepts: %v", err)
		} else {
			resp.Results = append(resp.Results, conceptResults...)
			resp.Debug["concept_matches"] = len(conceptResults)
		}
	}

	// Step 4: Sort by score and limit to topK total
	sortAndLimitRecallResults(&resp.Results, topK)

	return resp, nil
}

// fetchRecentObservations retrieves recent conversation observations
func (s *Service) fetchRecentObservations(ctx context.Context, spaceID, sessionID string, limit int, includeTasks, includeDecisions, includeLearnings bool) ([]ObservationResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build obs_type filter if any specific types requested
	var typeFilter string
	typeFilterEnabled := includeTasks || includeDecisions || includeLearnings
	if typeFilterEnabled {
		types := []string{}
		if includeTasks {
			types = append(types, "'task'")
		}
		if includeDecisions {
			types = append(types, "'decision'")
		}
		if includeLearnings {
			types = append(types, "'learning'")
		}
		typeFilter = fmt.Sprintf(" AND o.obs_type IN [%s]", strings.Join(types, ", "))
	}

	// Build session filter
	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = " AND o.session_id = $sessionId"
	}

	cypher := fmt.Sprintf(`
MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
WHERE o.layer = 0%s%s
RETURN o.node_id AS nodeId, o.obs_type AS obsType, o.content AS content,
       o.summary AS summary, o.session_id AS sessionId, o.surprise_score AS surpriseScore,
       o.tags AS tags, toString(o.created_at) AS createdAt
ORDER BY o.created_at DESC
LIMIT $limit`, sessionFilter, typeFilter)

	params := map[string]any{
		"spaceId":   spaceID,
		"sessionId": sessionID,
		"limit":     limit,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var observations []ObservationResult
		for res.Next(ctx) {
			rec := res.Record()
			observations = append(observations, ObservationResult{
				NodeID:        asString(rec, "nodeId"),
				ObsType:       asString(rec, "obsType"),
				Content:       asString(rec, "content"),
				Summary:       asString(rec, "summary"),
				SessionID:     asString(rec, "sessionId"),
				SurpriseScore: asFloat64(rec, "surpriseScore"),
				Tags:          asStringSlice(rec, "tags"),
				CreatedAt:     asString(rec, "createdAt"),
			})
		}
		return observations, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]ObservationResult), nil
}

// fetchRelatedThemes retrieves themes related to recent observations
func (s *Service) fetchRelatedThemes(ctx context.Context, spaceID, sessionID string) ([]ThemeResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build session filter
	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = " AND o.session_id = $sessionId"
	}

	// Find themes that have GENERALIZES edges from recent observations
	cypher := fmt.Sprintf(`
MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
      -[:GENERALIZES]->(t:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme'})
WHERE o.layer = 0%s
WITH t, count(o) AS memberCount
RETURN DISTINCT t.node_id AS nodeId, t.name AS name, t.summary AS summary,
       memberCount, t.dominant_obs_type AS dominantObsType,
       t.avg_surprise_score AS avgSurpriseScore
ORDER BY memberCount DESC
LIMIT 10`, sessionFilter)

	params := map[string]any{
		"spaceId":   spaceID,
		"sessionId": sessionID,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var themes []ThemeResult
		for res.Next(ctx) {
			rec := res.Record()
			themes = append(themes, ThemeResult{
				NodeID:           asString(rec, "nodeId"),
				Name:             asString(rec, "name"),
				Summary:          asString(rec, "summary"),
				MemberCount:      asInt(rec, "memberCount"),
				DominantObsType:  asString(rec, "dominantObsType"),
				AvgSurpriseScore: asFloat64(rec, "avgSurpriseScore"),
			})
		}
		return themes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]ThemeResult), nil
}

// fetchEmergentConcepts retrieves emergent concepts (higher-level abstractions)
func (s *Service) fetchEmergentConcepts(ctx context.Context, spaceID string) ([]EmergentConceptResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Find emergent concepts ordered by layer (higher = more abstract)
	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept'})
OPTIONAL MATCH (c)<-[:ABSTRACTS_TO]-(m)
WITH c, count(DISTINCT m) AS memberCount
RETURN c.node_id AS nodeId, c.name AS name, c.summary AS summary,
       c.layer AS layer, c.keywords AS keywords, c.session_count AS sessionCount,
       memberCount
ORDER BY c.layer DESC, memberCount DESC
LIMIT 10`

	params := map[string]any{
		"spaceId": spaceID,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var concepts []EmergentConceptResult
		for res.Next(ctx) {
			rec := res.Record()
			concepts = append(concepts, EmergentConceptResult{
				NodeID:       asString(rec, "nodeId"),
				Name:         asString(rec, "name"),
				Summary:      asString(rec, "summary"),
				Layer:        asInt(rec, "layer"),
				Keywords:     asStringSlice(rec, "keywords"),
				SessionCount: asInt(rec, "sessionCount"),
			})
		}
		return concepts, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]EmergentConceptResult), nil
}

// findSimilarObservations finds observations similar to the query embedding
func (s *Service) findSimilarObservations(ctx context.Context, spaceID string, embedding []float32, topK int) ([]RecallResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use vector similarity search on conversation_observation nodes
	cypher := `
CALL db.index.vector.queryNodes('mdemg_vector_index', $topK * 2, $embedding)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.role_type = 'conversation_observation'
  AND node.layer = 0
  AND NOT coalesce(node.is_archived, false)
RETURN node.node_id AS nodeId, node.content AS content, node.summary AS summary,
       node.obs_type AS obsType, node.surprise_score AS surpriseScore,
       node.session_id AS sessionId, score
ORDER BY score DESC
LIMIT $topK`

	params := map[string]any{
		"spaceId":   spaceID,
		"embedding": embedding,
		"topK":      topK,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var results []RecallResult
		for res.Next(ctx) {
			rec := res.Record()
			content := asString(rec, "content")
			if content == "" {
				content = asString(rec, "summary")
			}
			results = append(results, RecallResult{
				Type:    "conversation_observation",
				NodeID:  asString(rec, "nodeId"),
				Content: content,
				Score:   asFloat64(rec, "score"),
				Layer:   0,
				Metadata: map[string]any{
					"obs_type":       asString(rec, "obsType"),
					"surprise_score": asFloat64(rec, "surpriseScore"),
					"session_id":     asString(rec, "sessionId"),
				},
			})
		}
		return results, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]RecallResult), nil
}

// findSimilarThemes finds themes similar to the query embedding
func (s *Service) findSimilarThemes(ctx context.Context, spaceID string, embedding []float32, topK int) ([]RecallResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use vector similarity search on conversation_theme nodes
	cypher := `
CALL db.index.vector.queryNodes('mdemg_vector_index', $topK * 2, $embedding)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.role_type = 'conversation_theme'
  AND node.layer = 1
  AND NOT coalesce(node.is_archived, false)
RETURN node.node_id AS nodeId, node.name AS name, node.summary AS summary,
       node.member_count AS memberCount, node.dominant_obs_type AS dominantObsType,
       node.avg_surprise_score AS avgSurpriseScore, score
ORDER BY score DESC
LIMIT $topK`

	params := map[string]any{
		"spaceId":   spaceID,
		"embedding": embedding,
		"topK":      topK,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var results []RecallResult
		for res.Next(ctx) {
			rec := res.Record()
			results = append(results, RecallResult{
				Type:    "conversation_theme",
				NodeID:  asString(rec, "nodeId"),
				Content: asString(rec, "summary"),
				Score:   asFloat64(rec, "score"),
				Layer:   1,
				Metadata: map[string]any{
					"name":              asString(rec, "name"),
					"member_count":      asInt(rec, "memberCount"),
					"dominant_obs_type": asString(rec, "dominantObsType"),
					"avg_surprise_score": asFloat64(rec, "avgSurpriseScore"),
				},
			})
		}
		return results, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]RecallResult), nil
}

// findSimilarConcepts finds emergent concepts similar to the query embedding
func (s *Service) findSimilarConcepts(ctx context.Context, spaceID string, embedding []float32, topK int) ([]RecallResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use vector similarity search on emergent_concept nodes
	cypher := `
CALL db.index.vector.queryNodes('mdemg_vector_index', $topK * 2, $embedding)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.role_type = 'emergent_concept'
  AND node.layer >= 2
  AND NOT coalesce(node.is_archived, false)
RETURN node.node_id AS nodeId, node.name AS name, node.summary AS summary,
       node.layer AS layer, node.keywords AS keywords,
       node.session_count AS sessionCount, score
ORDER BY score DESC
LIMIT $topK`

	params := map[string]any{
		"spaceId":   spaceID,
		"embedding": embedding,
		"topK":      topK,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var results []RecallResult
		for res.Next(ctx) {
			rec := res.Record()
			results = append(results, RecallResult{
				Type:    "emergent_concept",
				NodeID:  asString(rec, "nodeId"),
				Content: asString(rec, "summary"),
				Score:   asFloat64(rec, "score"),
				Layer:   asInt(rec, "layer"),
				Metadata: map[string]any{
					"name":          asString(rec, "name"),
					"keywords":      asStringSlice(rec, "keywords"),
					"session_count": asInt(rec, "sessionCount"),
				},
			})
		}
		return results, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]RecallResult), nil
}

// generateResumeSummary creates a human-readable summary of the resumed context
func (s *Service) generateResumeSummary(resp *ResumeResponse) string {
	parts := []string{}

	if len(resp.Observations) > 0 {
		parts = append(parts, fmt.Sprintf("Resuming with %d recent observations", len(resp.Observations)))

		// Count by type
		typeCounts := make(map[string]int)
		for _, obs := range resp.Observations {
			typeCounts[obs.ObsType]++
		}

		typeList := []string{}
		for typ, count := range typeCounts {
			typeList = append(typeList, fmt.Sprintf("%d %s", count, typ))
		}
		if len(typeList) > 0 {
			parts = append(parts, fmt.Sprintf("(%s)", strings.Join(typeList, ", ")))
		}
	}

	if len(resp.Themes) > 0 {
		themeNames := make([]string, 0, min(3, len(resp.Themes)))
		for i := 0; i < min(3, len(resp.Themes)); i++ {
			themeNames = append(themeNames, resp.Themes[i].Name)
		}
		parts = append(parts, fmt.Sprintf("Active themes: %s", strings.Join(themeNames, ", ")))
	}

	if len(resp.EmergentConcepts) > 0 {
		conceptNames := make([]string, 0, min(2, len(resp.EmergentConcepts)))
		for i := 0; i < min(2, len(resp.EmergentConcepts)); i++ {
			conceptNames = append(conceptNames, resp.EmergentConcepts[i].Name)
		}
		parts = append(parts, fmt.Sprintf("Emergent concepts: %s", strings.Join(conceptNames, ", ")))
	}

	if len(parts) == 0 {
		return "No prior context found."
	}

	return strings.Join(parts, ". ") + "."
}

// sortAndLimitRecallResults sorts results by score and limits to topK
func sortAndLimitRecallResults(results *[]RecallResult, topK int) {
	if len(*results) <= 1 {
		return
	}

	// Sort by score descending
	for i := 0; i < len(*results)-1; i++ {
		for j := i + 1; j < len(*results); j++ {
			if (*results)[i].Score < (*results)[j].Score {
				(*results)[i], (*results)[j] = (*results)[j], (*results)[i]
			}
		}
	}

	// Limit to topK
	if len(*results) > topK {
		*results = (*results)[:topK]
	}
}

// Helper functions for extracting values from Neo4j records
func asString(rec *neo4j.Record, key string) string {
	if rec == nil {
		return ""
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

func asFloat64(rec *neo4j.Record, key string) float64 {
	if rec == nil {
		return 0
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}

func asInt(rec *neo4j.Record, key string) int {
	if rec == nil {
		return 0
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return int(v)
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

func asStringSlice(rec *neo4j.Record, key string) []string {
	if rec == nil {
		return nil
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// min returns the smaller of a and b
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
