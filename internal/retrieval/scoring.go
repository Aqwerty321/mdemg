package retrieval

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

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
		d := float64(prefixCount[prefixOf(c.Path)]-1) // 0 if unique

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

		// Apply config boost for configuration nodes
		configBoostEffect := 0.0
		if hasTag(c.Tags, "config") {
			originalScore := s
			s *= configBoost
			configBoostEffect = s - originalScore
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
	if len(items) > topK {
		items = items[:topK]
	}
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
