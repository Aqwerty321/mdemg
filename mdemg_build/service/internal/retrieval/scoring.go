package retrieval

import (
	"math"
	"sort"
	"strings"
	"time"

	"mdemg/internal/models"
)

// ScoreAndRank computes the final score per candidate and returns topK results.
func ScoreAndRank(cands []Candidate, act map[string]float64, edges []Edge, topK int) []models.RetrieveResult {
	if topK <= 0 {
		topK = 20
	}

	// Local degree estimate from fetched subgraph
	deg := map[string]int{}
	for _, e := range edges {
		deg[e.Src]++
		deg[e.Dst]++
	}

	// Hyperparameters (start values; move to config later)
	alpha := 0.55 // vector
	beta := 0.30  // activation
	gamma := 0.10 // recency
	delta := 0.05 // confidence
	phi := 0.08   // hub penalty
	kappa := 0.12 // redundancy penalty
	rho := 0.05   // recency decay per day

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
		h := math.Log(1.0 + float64(deg[c.NodeID]))
		d := float64(prefixCount[prefixOf(c.Path)]-1) // 0 if unique

		s := alpha*c.VectorSim + beta*a + gamma*r + delta*c.Confidence - phi*h - kappa*d

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
