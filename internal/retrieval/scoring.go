package retrieval

import (
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// scoringDebugEnabled enables detailed tracing in scoring
var scoringDebugEnabled = false

// scoringDebugTerm is the term to trace through scoring
var scoringDebugTerm = "transformerconfig"

// pathPatterns matches common path patterns in query text
// Captures paths like "lib/graphql", "frontend/src", "services/auth", etc.
var pathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b((?:lib|src|cmd|internal|pkg|services?|modules?|components?|frontend|backend|api|apps?)/[\w\-./]+)`),
	regexp.MustCompile(`(?i)\b([\w\-]+/[\w\-]+/[\w\-./]+\.(?:ts|tsx|js|jsx|go|py|rs|java|rb))\b`),
	regexp.MustCompile(`(?i)\b([\w\-]+(?:Module|Service|Controller|Handler|Repository))\b`),
}

// extractPathHints extracts likely path patterns from a query string
func extractPathHints(query string) []string {
	hints := make(map[string]struct{})

	for _, re := range pathPatterns {
		matches := re.FindAllStringSubmatch(query, -1)
		for _, m := range matches {
			if len(m) > 1 {
				hint := strings.ToLower(m[1])
				// Normalize: remove trailing slashes, clean up
				hint = strings.TrimSuffix(hint, "/")
				if len(hint) > 2 { // Skip very short matches
					hints[hint] = struct{}{}
				}
			}
		}
	}

	// Also extract quoted paths or explicit directory mentions
	quoteRe := regexp.MustCompile(`["']([^"']+/[^"']+)["']`)
	matches := quoteRe.FindAllStringSubmatch(query, -1)
	for _, m := range matches {
		if len(m) > 1 {
			hints[strings.ToLower(m[1])] = struct{}{}
		}
	}

	result := make([]string, 0, len(hints))
	for h := range hints {
		result = append(result, h)
	}
	return result
}

// comparisonPatterns detects comparison-style queries ("X vs Y", "difference between", etc.)
var comparisonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:difference|differences)\s+between\s+(.+?)\s+and\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\b(.+?)\s+(?:vs\.?|versus|compared\s+to)\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\b(?:both|having both)\s+(.+?)\s+and\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\b(?:compare|comparing)\s+(.+?)\s+(?:and|with|to)\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\bwhy\s+(?:have|are there|do we have)\s+both\s+(.+?)\s+and\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\brelationship\s+between\s+(.+?)\s+and\s+(.+?)\b`),
}

// extractComparisonTargets extracts names being compared in a query
func extractComparisonTargets(query string) []string {
	targets := make(map[string]struct{})

	for _, re := range comparisonPatterns {
		matches := re.FindAllStringSubmatch(query, -1)
		for _, m := range matches {
			for i := 1; i < len(m); i++ {
				target := strings.TrimSpace(m[i])
				// Clean up the target - extract just module/service names
				target = cleanModuleName(target)
				if len(target) > 2 {
					targets[strings.ToLower(target)] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0, len(targets))
	for t := range targets {
		result = append(result, t)
	}
	return result
}

// cleanModuleName extracts a clean module/service name from a phrase
func cleanModuleName(name string) string {
	// Remove common phrases
	name = regexp.MustCompile(`(?i)^(?:the\s+)?`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`(?i)\s*(?:module|service|controller|handler)s?\s*$`).ReplaceAllString(name, "")

	// Keep just alphanumeric and common separators
	parts := strings.Fields(name)
	if len(parts) > 0 {
		// Take the last word if it looks like a module name
		for i := len(parts) - 1; i >= 0; i-- {
			if regexp.MustCompile(`^[A-Z][a-zA-Z0-9]+$`).MatchString(parts[i]) ||
				strings.Contains(parts[i], "Module") ||
				strings.Contains(parts[i], "Service") {
				return parts[i]
			}
		}
		return parts[len(parts)-1]
	}
	return name
}

// isComparisonQuery checks if a query is asking for a comparison
func isComparisonQuery(query string) bool {
	return len(extractComparisonTargets(query)) >= 2
}

// comparisonMatchScore returns a boost for comparison nodes that match query targets
func comparisonMatchScore(name, summary string, tags []string, comparisonTargets []string) float64 {
	if len(comparisonTargets) == 0 {
		return 0.0
	}

	// Strong boost for comparison nodes when query is a comparison
	isComparisonNode := hasTag(tags, "comparison")

	// Check if this node's name/summary mentions the comparison targets
	nameLower := strings.ToLower(name)
	summaryLower := strings.ToLower(summary)
	matchCount := 0

	for _, target := range comparisonTargets {
		if strings.Contains(nameLower, target) || strings.Contains(summaryLower, target) {
			matchCount++
		}
	}

	if matchCount == 0 {
		return 0.0
	}

	// Ratio of targets matched
	matchRatio := float64(matchCount) / float64(len(comparisonTargets))

	if isComparisonNode {
		// Comparison nodes get a strong boost (0.3-0.6) when they match
		return 0.3 + 0.3*matchRatio
	}

	// Regular nodes get a smaller boost (0.1-0.2) for being part of a comparison
	return 0.1 + 0.1*matchRatio
}

// isCodeQuery returns true if the query appears to be asking about code elements
// rather than documentation or configuration.
func isCodeQuery(query string) bool {
	if query == "" {
		return false
	}
	queryLower := strings.ToLower(query)

	// Code-related keywords that suggest the user wants code, not config
	// Use word boundaries via space, start-of-string, or end-of-string
	codeKeywords := []string{
		" class", "class ",  // "X class" or "class X"
		" function", "function ",
		" method", "method ",
		" struct", "struct ",
		" interface", "interface ",
		" def ", "def ",
		"implement", "defined", "definition",
		"where is", "how does", "what does",
		" code", "code ",
		" source", "source ",
	}

	for _, kw := range codeKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}

	// Also check for words at query boundaries
	boundaryKeywords := []string{"class", "function", "method", "struct", "interface", "code", "source"}
	for _, kw := range boundaryKeywords {
		// Match at end of query: "X class"
		if strings.HasSuffix(queryLower, kw) {
			return true
		}
		// Match at start of query: "class X"
		if strings.HasPrefix(queryLower, kw+" ") {
			return true
		}
	}

	return false
}

// codeTypeBoost returns a multiplier for the score based on code vs config files.
// For code queries: code files get 1.0, config/doc get penalty (< 1.0)
// For non-code queries: all get 1.0 (no change)
func codeTypeBoost(tags []string, isCodeQuery bool) float64 {
	if !isCodeQuery {
		return 1.0 // No adjustment for non-code queries
	}

	// Code file tags get no penalty
	codeTags := []string{"python", "go", "rust", "java", "typescript", "javascript", "c", "cpp", "csharp", "ruby", "php", "scala", "kotlin", "swift"}
	for _, ct := range codeTags {
		if hasTag(tags, ct) {
			return 1.0
		}
	}

	// Config and documentation get penalized for code queries
	if hasTag(tags, "config") || hasTag(tags, "configuration") {
		return 0.5 // 50% penalty
	}
	if hasTag(tags, "documentation") || hasTag(tags, "markdown") {
		return 0.7 // 30% penalty
	}

	return 1.0
}

// pathMatchScore returns a boost score (0.0-1.0) based on how well a node path matches query hints
func pathMatchScore(nodePath string, hints []string) float64 {
	if len(hints) == 0 || nodePath == "" {
		return 0.0
	}

	normalizedPath := strings.ToLower(nodePath)
	bestScore := 0.0

	for _, hint := range hints {
		// Exact substring match (strongest)
		if strings.Contains(normalizedPath, hint) {
			// Score based on how much of the path is matched
			matchRatio := float64(len(hint)) / float64(len(normalizedPath))
			score := 0.5 + 0.5*matchRatio // 0.5 to 1.0
			if score > bestScore {
				bestScore = score
			}
			continue
		}

		// Partial segment match (moderate)
		hintParts := strings.Split(hint, "/")
		pathParts := strings.Split(normalizedPath, "/")
		matchedParts := 0
		for _, hp := range hintParts {
			for _, pp := range pathParts {
				if strings.Contains(pp, hp) || strings.Contains(hp, pp) {
					matchedParts++
					break
				}
			}
		}
		if matchedParts > 0 {
			score := 0.3 * float64(matchedParts) / float64(len(hintParts))
			if score > bestScore {
				bestScore = score
			}
		}
	}

	return bestScore
}

// ScoreAndRank computes the final score per candidate and returns topK results.
// queryText is optional - when provided, enables path-based boosting for architecture queries.
func ScoreAndRank(cands []Candidate, act map[string]float64, edges []Edge, topK int, cfg config.Config, queryText string) []models.RetrieveResult {
	scored := ScoreAndRankWithBreakdown(cands, act, edges, topK, cfg, queryText)
	results := make([]models.RetrieveResult, len(scored))
	for i, sc := range scored {
		results[i] = sc.RetrieveResult
	}
	return results
}

// ScoreAndRankWithBreakdown computes scores with detailed breakdowns for Jiminy.
// Returns ScoredCandidate with both the result and the score breakdown.
func ScoreAndRankWithBreakdown(cands []Candidate, act map[string]float64, edges []Edge, topK int, cfg config.Config, queryText string) []ScoredCandidate {
	if topK <= 0 {
		topK = 20
	}

	// Local degree estimate from fetched subgraph
	deg := map[string]int{}
	for _, e := range edges {
		deg[e.Src]++
		deg[e.Dst]++
	}

	// Hyperparameters from config (see config.Config for defaults)
	alpha := cfg.ScoringAlpha       // vector similarity weight
	beta := cfg.ScoringBeta         // activation weight
	gamma := cfg.ScoringGamma       // recency weight
	delta := cfg.ScoringDelta       // confidence weight
	phi := cfg.ScoringPhi           // hub penalty coefficient
	kappa := cfg.ScoringKappa       // redundancy penalty coefficient
	rho := cfg.ScoringRho           // recency decay rate per day
	configBoost := cfg.ScoringConfigBoost // config node boost multiplier
	pathBoost := cfg.ScoringPathBoost     // path match boost coefficient

	// Extract path hints from query text for architecture-style queries
	pathHints := extractPathHints(queryText)

	// Extract comparison targets for comparison-style queries
	comparisonTargets := extractComparisonTargets(queryText)

	// Detect if this is a code-focused query (for code type boost)
	codeQueryDetected := isCodeQuery(queryText)

	// Redundancy: simple path-prefix clustering
	prefixCount := map[string]int{}
	prefixOf := func(path string) string {
		p := strings.TrimSpace(path)
		if p == "" {
			return ""
		}
		idx := strings.LastIndex(p, "/")
		if idx <= 0 {
			return p
		}
		return p[:idx]
	}
	for _, c := range cands {
		prefixCount[prefixOf(c.Path)]++
	}

	items := make([]ScoredCandidate, 0, len(cands))
	now := time.Now()

	if scoringDebugEnabled {
		log.Printf("[DEBUG Scoring] Processing %d candidates, query='%s', codeQueryDetected=%v", len(cands), queryText, codeQueryDetected)
	}

	for _, c := range cands {
		a := act[c.NodeID]
		ageDays := now.Sub(c.UpdatedAt).Hours() / 24.0
		r := math.Exp(-rho * ageDays)
		if r < 0 {
			r = 0
		}
		if r > 1 {
			r = 1
		}

		// Hub penalty: exempt concern/hidden/concept nodes (layer > 0) since they're designed as hubs
		h := 0.0
		if c.Layer == 0 {
			h = math.Log(1.0 + float64(deg[c.NodeID]))
		}
		d := float64(prefixCount[prefixOf(c.Path)] - 1) // 0 if unique
		// Cap the redundancy factor to avoid excessive penalties for files in large directories
		if d > 3 {
			d = 3 // Max penalty is 3 * kappa (0.36 at default kappa=0.12)
		}

		// Path boost: reward nodes whose paths match patterns mentioned in query
		pbRaw := pathMatchScore(c.Path, pathHints)
		pb := pbRaw * pathBoost

		// Comparison boost: reward comparison nodes and modules mentioned in comparison queries
		cbRaw := comparisonMatchScore(c.Name, c.Summary, c.Tags, comparisonTargets)
		cb := cbRaw * pathBoost

		// Calculate individual weighted components
		vecComponent := alpha * c.VectorSim
		actComponent := beta * a
		recComponent := gamma * r
		confComponent := delta * c.Confidence
		hubPenComponent := phi * h
		redPenComponent := kappa * d

		s := vecComponent + actComponent + recComponent + confComponent + pb + cb - hubPenComponent - redPenComponent

		// Apply code type boost/penalty
		// For code queries: config/doc files get penalized, code files unchanged
		// For non-code queries: apply the configBoost to config files
		codeTypeMult := codeTypeBoost(c.Tags, codeQueryDetected)
		configBoostEffect := 0.0
		if codeQueryDetected {
			// For code queries, apply the penalty/boost from codeTypeBoost
			originalScore := s
			s *= codeTypeMult
			configBoostEffect = s - originalScore // Will be negative for config files
		} else {
			// For non-code queries, use the existing configBoost logic
			if hasTag(c.Tags, "config") {
				originalScore := s
				s *= configBoost
				configBoostEffect = s - originalScore
			}
		}

		breakdown := ScoreBreakdown{
			VectorSimilarity:  vecComponent,
			Activation:        actComponent,
			Recency:           recComponent,
			Confidence:        confComponent,
			PathBoost:         pb,
			ComparisonBoost:   cb,
			ConfigBoost:       configBoostEffect,
			HubPenalty:        -hubPenComponent,
			RedundancyPenalty: -redPenComponent,
			RerankDelta:       0, // Set later by reranker
			LearningEdgeBoost: 0, // Set later if learning edges contributed
			FinalScore:        s,
		}

		// Debug: trace target term through scoring
		if scoringDebugEnabled {
			if strings.Contains(strings.ToLower(c.Name), scoringDebugTerm) ||
				strings.Contains(strings.ToLower(c.Path), scoringDebugTerm) {
				log.Printf("[DEBUG Scoring] '%s': NodeID=%s, Name=%s", scoringDebugTerm, c.NodeID, c.Name)
				log.Printf("[DEBUG Scoring]   Components: vec=%.4f, act=%.4f, rec=%.4f, conf=%.4f, pathBoost=%.4f, compBoost=%.4f",
					vecComponent, actComponent, recComponent, confComponent, pb, cb)
				log.Printf("[DEBUG Scoring]   Penalties: hub=%.4f (degree=%d), redundancy=%.4f (prefixCount=%d)",
					hubPenComponent, deg[c.NodeID], redPenComponent, prefixCount[prefixOf(c.Path)])
				log.Printf("[DEBUG Scoring]   Final: %.4f, codeTypeMult=%.2f", s, codeTypeMult)
			}
		}

		items = append(items, ScoredCandidate{
			RetrieveResult: models.RetrieveResult{
				NodeID:     c.NodeID,
				Path:       c.Path,
				Name:       c.Name,
				Summary:    c.Summary,
				Score:      s,
				VectorSim:  c.VectorSim,
				Activation: a,
			},
			Breakdown: breakdown,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })

	// Debug: check if target term is in top results before truncation
	if scoringDebugEnabled {
		found := false
		rank := -1
		for i, item := range items {
			if strings.Contains(strings.ToLower(item.Name), scoringDebugTerm) ||
				strings.Contains(strings.ToLower(item.Path), scoringDebugTerm) {
				found = true
				rank = i + 1
				log.Printf("[DEBUG Scoring] '%s' ranked #%d of %d (before truncation to topK=%d): Score=%.4f, Name=%s",
					scoringDebugTerm, rank, len(items), topK, item.Score, item.Name)
				break
			}
		}
		if !found {
			log.Printf("[DEBUG Scoring] '%s' NOT FOUND in %d scored items", scoringDebugTerm, len(items))
		}
	}

	if len(items) > topK {
		items = items[:topK]
	}

	// Debug: check if target term survived truncation
	if scoringDebugEnabled {
		found := false
		for i, item := range items {
			if strings.Contains(strings.ToLower(item.Name), scoringDebugTerm) ||
				strings.Contains(strings.ToLower(item.Path), scoringDebugTerm) {
				found = true
				log.Printf("[DEBUG Scoring] '%s' SURVIVED truncation at rank #%d", scoringDebugTerm, i+1)
				break
			}
		}
		if !found {
			log.Printf("[DEBUG Scoring] '%s' DID NOT survive truncation to topK=%d", scoringDebugTerm, topK)
		}
	}

	// Apply percentile-based normalized confidence
	// This makes confidence immune to learning edge density changes
	ApplyNormalizedConfidence(items)

	return items
}

// hasTag checks if a tag slice contains a specific tag
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ScoreBreakdown tracks each component's contribution to the final score.
// Used by Jiminy to explain why a result was ranked where it is.
type ScoreBreakdown struct {
	VectorSimilarity  float64 `json:"vector_similarity"`   // α * VectorSim
	Activation        float64 `json:"activation"`          // β * activation
	Recency           float64 `json:"recency"`             // γ * recency factor
	Confidence        float64 `json:"confidence"`          // δ * confidence
	PathBoost         float64 `json:"path_boost"`          // path match boost
	ComparisonBoost   float64 `json:"comparison_boost"`    // comparison query boost
	ConfigBoost       float64 `json:"config_boost"`        // config node multiplier effect
	HubPenalty        float64 `json:"hub_penalty"`         // -φ * hub factor
	RedundancyPenalty float64 `json:"redundancy_penalty"`  // -κ * redundancy factor
	RerankDelta       float64 `json:"rerank_delta"`        // score change from LLM re-ranking
	LearningEdgeBoost float64 `json:"learning_edge_boost"` // boost from CO_ACTIVATED_WITH traversal
	FinalScore        float64 `json:"final_score"`         // sum of all components
}

// ScoredCandidate extends RetrieveResult with ScoreBreakdown for Jiminy.
type ScoredCandidate struct {
	models.RetrieveResult
	Breakdown ScoreBreakdown `json:"breakdown"`
}

// ApplyNormalizedConfidence computes percentile-based confidence for each result.
// This makes confidence immune to learning edge density changes (the "activation dilution" problem).
//
// Instead of using absolute score thresholds (which degrade as CO_ACTIVATED_WITH edges accumulate),
// we compute each result's percentile rank within the current result set.
//
// Confidence levels:
//   - HIGH: top 10% (percentile >= 90)
//   - MEDIUM: middle 40-90% (percentile >= 40)
//   - LOW: bottom 40% (percentile < 40)
func ApplyNormalizedConfidence(items []ScoredCandidate) {
	n := len(items)
	if n == 0 {
		return
	}

	// Items are already sorted by score descending
	// Compute percentile for each item based on rank
	// Formula: percentile = 100 * (n-1-rank) / (n-1)
	// - Rank 0 (best) = 100th percentile
	// - Rank n-1 (worst) = 0th percentile
	for i := range items {
		rank := i
		var percentile float64
		if n == 1 {
			percentile = 100.0 // Single result gets top percentile
		} else {
			percentile = 100.0 * float64(n-1-rank) / float64(n-1)
		}

		items[i].NormalizedConfidence = math.Round(percentile*10) / 10 // Round to 1 decimal

		// Assign confidence level based on percentile
		switch {
		case percentile >= 90:
			items[i].ConfidenceLevel = "HIGH"
		case percentile >= 40:
			items[i].ConfidenceLevel = "MEDIUM"
		default:
			items[i].ConfidenceLevel = "LOW"
		}
	}
}

// ConfidenceLevelFromPercentile returns the confidence level for a given percentile.
// Exported for use in other packages that may need to compute confidence levels.
func ConfidenceLevelFromPercentile(percentile float64) string {
	switch {
	case percentile >= 90:
		return "HIGH"
	case percentile >= 40:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// ApplyNormalizedConfidenceToResults applies percentile-based confidence to final results.
// This should be called AFTER all post-processing (reasoning modules, reranking, truncation)
// to ensure the confidence levels reflect the final ordering.
func ApplyNormalizedConfidenceToResults(results []models.RetrieveResult) {
	n := len(results)
	if n == 0 {
		return
	}

	// Results are already sorted by score descending
	// Compute percentile for each result based on rank
	for i := range results {
		rank := i
		var percentile float64
		if n == 1 {
			percentile = 100.0
		} else {
			percentile = 100.0 * float64(n-1-rank) / float64(n-1)
		}

		results[i].NormalizedConfidence = math.Round(percentile*10) / 10
		results[i].ConfidenceLevel = ConfidenceLevelFromPercentile(percentile)
	}
}
