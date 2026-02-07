package conversation

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TokenBudget is the default maximum tokens for resume
const DefaultTokenBudget = 4000

// TokenEstimator estimates token counts for text
type TokenEstimator struct {
	// avgCharsPerToken varies by language/content
	// English text averages ~4 chars per token for GPT-style tokenizers
	avgCharsPerToken float64
}

// NewTokenEstimator creates a new token estimator
func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{
		avgCharsPerToken: 4.0, // Conservative estimate
	}
}

// EstimateTokens estimates token count for a string
func (e *TokenEstimator) EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	charCount := utf8.RuneCountInString(text)
	tokens := float64(charCount) / e.avgCharsPerToken
	return int(tokens + 0.5) // Round to nearest
}

// EstimateObservationTokens estimates tokens for an observation
func (e *TokenEstimator) EstimateObservationTokens(obs *Observation) int {
	tokens := 0

	// Content is primary
	tokens += e.EstimateTokens(obs.Content)

	// Summary if present
	tokens += e.EstimateTokens(obs.Summary)

	// Structured data (estimate as JSON-ish)
	if len(obs.StructuredData) > 0 {
		tokens += e.estimateMapTokens(obs.StructuredData)
	}

	// Metadata overhead (type, id, timestamp)
	tokens += 20 // Fixed overhead

	return tokens
}

// estimateMapTokens estimates tokens for a map (rough JSON estimate)
func (e *TokenEstimator) estimateMapTokens(m map[string]any) int {
	tokens := 0
	for k, v := range m {
		tokens += e.EstimateTokens(k)
		switch val := v.(type) {
		case string:
			tokens += e.EstimateTokens(val)
		case []string:
			for _, s := range val {
				tokens += e.EstimateTokens(s)
			}
		case []interface{}:
			tokens += len(val) * 5 // Rough estimate per array item
		default:
			tokens += 5 // Default for other types
		}
	}
	return tokens
}

// =============================================================================
// Smart Truncation
// =============================================================================

// TruncationResult contains the result of smart truncation
type TruncationResult struct {
	Observations []EnhancedObservation `json:"observations"`
	Summaries    []EnhancedObservation `json:"summaries,omitempty"`
	TokenCount   int                   `json:"token_count"`
	TokenBudget  int                   `json:"token_budget"`
	OmittedCount int                   `json:"omitted_count"`
	TierCounts   map[Tier]int          `json:"tier_counts"`
}

// SmartTruncator handles intelligent observation truncation
type SmartTruncator struct {
	tokenBudget    int
	estimator      *TokenEstimator
	criticalBudget float64 // Fraction reserved for critical tier
	importantLimit int     // Max observations in important tier
}

// NewSmartTruncator creates a truncator with default settings
func NewSmartTruncator(tokenBudget int) *SmartTruncator {
	if tokenBudget <= 0 {
		tokenBudget = DefaultTokenBudget
	}
	return &SmartTruncator{
		tokenBudget:    tokenBudget,
		estimator:      NewTokenEstimator(),
		criticalBudget: 0.5,  // Reserve 50% for critical
		importantLimit: 10,   // Max 10 important observations
	}
}

// WithCriticalBudget sets the fraction of budget reserved for critical tier
func (t *SmartTruncator) WithCriticalBudget(fraction float64) *SmartTruncator {
	if fraction >= 0 && fraction <= 1 {
		t.criticalBudget = fraction
	}
	return t
}

// WithImportantLimit sets the max number of important observations
func (t *SmartTruncator) WithImportantLimit(limit int) *SmartTruncator {
	if limit > 0 {
		t.importantLimit = limit
	}
	return t
}

// Truncate applies smart truncation to scored observations
func (t *SmartTruncator) Truncate(scored []ScoredObservation) *TruncationResult {
	result := &TruncationResult{
		Observations: make([]EnhancedObservation, 0),
		Summaries:    make([]EnhancedObservation, 0),
		TokenBudget:  t.tokenBudget,
		TierCounts:   make(map[Tier]int),
	}

	if len(scored) == 0 {
		return result
	}

	// Separate by tier
	critical := make([]ScoredObservation, 0)
	important := make([]ScoredObservation, 0)
	background := make([]ScoredObservation, 0)

	for _, s := range scored {
		switch s.Tier {
		case TierCritical:
			critical = append(critical, s)
		case TierImportant:
			important = append(important, s)
		default:
			background = append(background, s)
		}
	}

	usedTokens := 0
	criticalBudget := int(float64(t.tokenBudget) * t.criticalBudget)

	// Phase 1: Add all critical observations (up to budget)
	for _, s := range critical {
		tokens := t.estimator.EstimateObservationTokens(s.Observation)
		if usedTokens+tokens <= criticalBudget || usedTokens == 0 {
			result.Observations = append(result.Observations, toEnhanced(s, false))
			usedTokens += tokens
			result.TierCounts[TierCritical]++
		} else {
			// Critical overflow - truncate content if needed
			if tokens > 200 { // Only truncate if substantial
				truncated := t.truncateContent(s.Observation, criticalBudget-usedTokens)
				enhanced := toEnhanced(s, true)
				enhanced.Content = truncated
				result.Observations = append(result.Observations, enhanced)
				usedTokens = criticalBudget
				result.TierCounts[TierCritical]++
			} else {
				result.OmittedCount++
			}
		}
	}

	// Phase 2: Add important observations (limited count and remaining budget)
	remainingBudget := t.tokenBudget - usedTokens
	importantAdded := 0

	for _, s := range important {
		if importantAdded >= t.importantLimit {
			break
		}

		tokens := t.estimator.EstimateObservationTokens(s.Observation)
		if usedTokens+tokens <= t.tokenBudget*9/10 { // Keep 10% for background summaries
			result.Observations = append(result.Observations, toEnhanced(s, false))
			usedTokens += tokens
			result.TierCounts[TierImportant]++
			importantAdded++
		} else {
			// Truncate if close to budget
			truncated := t.truncateContent(s.Observation, remainingBudget/2)
			if len(truncated) > 50 { // Only include if meaningful
				enhanced := toEnhanced(s, true)
				enhanced.Content = truncated
				result.Observations = append(result.Observations, enhanced)
				usedTokens += t.estimator.EstimateTokens(truncated)
				result.TierCounts[TierImportant]++
				importantAdded++
			}
			break // Stop adding important once we start truncating
		}
	}

	// Track omitted important
	result.OmittedCount += len(important) - importantAdded

	// Phase 3: Summarize background observations
	if len(background) > 0 && usedTokens < t.tokenBudget {
		summaryBudget := t.tokenBudget - usedTokens
		summary := t.summarizeBackground(background, summaryBudget)
		if summary != nil {
			result.Summaries = append(result.Summaries, *summary)
			usedTokens += t.estimator.EstimateTokens(summary.Content)
			result.TierCounts[TierBackground] = len(background)
		}
	} else {
		result.OmittedCount += len(background)
	}

	result.TokenCount = usedTokens
	return result
}

// truncateContent truncates observation content to fit token budget
func (t *SmartTruncator) truncateContent(obs *Observation, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}

	content := obs.Content
	tokens := t.estimator.EstimateTokens(content)

	if tokens <= maxTokens {
		return content
	}

	// Truncate to approximate character count
	maxChars := int(float64(maxTokens) * t.estimator.avgCharsPerToken)
	runes := []rune(content)

	if len(runes) <= maxChars {
		return content
	}

	// Find a good break point (sentence or word boundary)
	truncated := string(runes[:maxChars])

	// Try to break at sentence
	lastSentence := strings.LastIndexAny(truncated, ".!?")
	if lastSentence > maxChars/2 {
		return truncated[:lastSentence+1] + "..."
	}

	// Try to break at word
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxChars/2 {
		return truncated[:lastSpace] + "..."
	}

	return truncated + "..."
}

// summarizeBackground creates a summary of background observations
func (t *SmartTruncator) summarizeBackground(background []ScoredObservation, maxTokens int) *EnhancedObservation {
	if len(background) == 0 || maxTokens < 50 {
		return nil
	}

	// Group by type
	typeCounts := make(map[ObservationType]int)
	obsIDs := make([]string, 0, len(background))

	for _, s := range background {
		typeCounts[s.Observation.ObsType]++
		obsIDs = append(obsIDs, s.Observation.ObsID)
	}

	// Build summary
	var summaryParts []string
	for obsType, count := range typeCounts {
		summaryParts = append(summaryParts, fmt.Sprintf("%d %s", count, obsType))
	}

	summaryContent := fmt.Sprintf("Background: %s observations (%s)",
		pluralize(len(background), "observation"),
		strings.Join(summaryParts, ", "))

	// Add recent topics if space
	if maxTokens > 100 && len(background) > 0 {
		// Extract first few words from most recent background items
		var topics []string
		for i := 0; i < min(3, len(background)); i++ {
			topic := extractTopic(background[i].Observation.Content)
			if topic != "" {
				topics = append(topics, topic)
			}
		}
		if len(topics) > 0 {
			summaryContent += fmt.Sprintf(". Topics: %s", strings.Join(topics, "; "))
		}
	}

	return &EnhancedObservation{
		ObsID:      "summary-background",
		Tier:       string(TierBackground),
		ObsType:    string(ObsTypeContext),
		Content:    summaryContent,
		Truncated:  true,
		Summarizes: obsIDs,
	}
}

// extractTopic extracts a brief topic from content
func extractTopic(content string) string {
	// Take first sentence or first N words
	sentences := strings.SplitN(content, ".", 2)
	topic := sentences[0]

	words := strings.Fields(topic)
	if len(words) > 6 {
		return strings.Join(words[:6], " ") + "..."
	}

	return topic
}

// toEnhanced converts a scored observation to enhanced format
func toEnhanced(s ScoredObservation, truncated bool) EnhancedObservation {
	return EnhancedObservation{
		ObsID:          s.Observation.ObsID,
		Tier:           string(s.Tier),
		ObsType:        string(s.Observation.ObsType),
		TemplateID:     s.Observation.TemplateID,
		Content:        s.Observation.Content,
		StructuredData: s.Observation.StructuredData,
		RelevanceScore: s.RelevanceScore,
		CreatedAt:      s.Observation.CreatedAt,
		Truncated:      truncated,
	}
}

// pluralize returns singular or plural form
func pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

// Note: min is defined in service.go

// =============================================================================
// Integration with Resume
// =============================================================================

// TruncatedResumeBuilder builds a resume response with smart truncation
type TruncatedResumeBuilder struct {
	scorer     *RelevanceScorer
	truncator  *SmartTruncator
}

// NewTruncatedResumeBuilder creates a builder with given budget
func NewTruncatedResumeBuilder(tokenBudget int) *TruncatedResumeBuilder {
	return &TruncatedResumeBuilder{
		scorer:    NewRelevanceScorer(),
		truncator: NewSmartTruncator(tokenBudget),
	}
}

// WithWeights configures relevance weights
func (b *TruncatedResumeBuilder) WithWeights(weights *RelevanceWeights) *TruncatedResumeBuilder {
	b.scorer.WithWeights(weights)
	return b
}

// WithQueryEmbedding sets embedding for task-relevance scoring
func (b *TruncatedResumeBuilder) WithQueryEmbedding(embedding []float32) *TruncatedResumeBuilder {
	b.scorer.WithQueryEmbedding(embedding)
	return b
}

// Build creates a truncated resume from observations
func (b *TruncatedResumeBuilder) Build(observations []*Observation) *EnhancedResumeResponse {
	// Score all observations
	scored := b.scorer.ScoreObservations(observations)

	// Apply smart truncation
	truncated := b.truncator.Truncate(scored)

	return &EnhancedResumeResponse{
		Observations:        truncated.Observations,
		SummaryObservations: truncated.Summaries,
		TokenCount:          truncated.TokenCount,
		TokenBudget:         truncated.TokenBudget,
		OmittedCount:        truncated.OmittedCount,
	}
}
