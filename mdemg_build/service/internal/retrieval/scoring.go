package retrieval

import (
	"math"
	"sort"
	"strings"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// ScoreAndRank computes the final score per candidate and returns topK results.
func ScoreAndRank(cands []Candidate, act map[string]float64, edges []Edge, topK int, cfg config.Config) []models.RetrieveResult {
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

	items := make([]models.RetrieveResult, 0, len(cands))
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

		s := alpha*c.VectorSim + beta*a + gamma*r + delta*c.Confidence - phi*h - kappa*d

		// Apply config boost for configuration nodes
		if hasTag(c.Tags, "config") {
			s *= configBoost
		}

		items = append(items, models.RetrieveResult{
			NodeID: c.NodeID,
			Path: c.Path,
			Name: c.Name,
			Summary: c.Summary,
			Score: s,
			VectorSim: c.VectorSim,
			Activation: a,
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
