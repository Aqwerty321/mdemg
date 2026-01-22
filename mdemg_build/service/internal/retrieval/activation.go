package retrieval

import "math"

// SpreadingActivation computes transient activation scores in-memory.
// - cands: candidates from vector recall (used for seeding)
// - edges: bounded neighborhood edges
// - steps: number of propagation steps (typ. 2-5)
// - lambda: step decay (prevents runaway)
func SpreadingActivation(cands []Candidate, edges []Edge, steps int, lambda float64) map[string]float64 {
	act := map[string]float64{}
	if steps <= 0 {
		steps = 2
	}
	if lambda < 0 {
		lambda = 0
	}
	if lambda > 0.9 {
		lambda = 0.9
	}

	// Seed: ALL candidates seeded from vector similarity
	// This enables Hebbian learning by ensuring all returned nodes have activation values.
	// Previously only top-2 were seeded, causing most nodes to have 0 activation
	// and failing the learning threshold filter (see LEARNING_EDGES_ANALYSIS.md).
	for _, c := range cands {
		v := c.VectorSim
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		act[c.NodeID] = v
	}

	// Build incoming lists
	incoming := map[string][]Edge{}
	inhib := map[string][]Edge{}
	for _, e := range edges {
		if e.RelType == "CONTRADICTS" {
			inhib[e.Dst] = append(inhib[e.Dst], e)
			continue
		}
		incoming[e.Dst] = append(incoming[e.Dst], e)
	}

	clamp01 := func(x float64) float64 {
		if x < 0 {
			return 0
		}
		if x > 1 {
			return 1
		}
		return x
	}

	for t := 0; t < steps; t++ {
		next := map[string]float64{}
		// Carry forward existing activations with decay
		for id, a := range act {
			next[id] = clamp01((1 - lambda) * a)
		}

		// Apply incoming excitatory and inhibitory
		for dst, ins := range incoming {
			acc := next[dst]
			for _, e := range ins {
				srcA := act[e.Src]
				w := effectiveWeight(e)
				acc += srcA * w
			}
			// inhibitory edges
			for _, e := range inhib[dst] {
				srcA := act[e.Src]
				w := math.Abs(effectiveWeight(e))
				acc -= srcA * w
			}
			next[dst] = clamp01(acc)
		}

		act = next
	}
	return act
}

func effectiveWeight(e Edge) float64 {
	w := e.Weight
	if w < 0 {
		w = 0
	}
	// Dimension mix; if all dims are 0, treat as semantic=1.0
	mix := 0.0
	mix += 0.6 * e.DimSemantic
	mix += 0.2 * e.DimTemporal
	mix += 0.2 * e.DimCoactivation
	if mix == 0 {
		mix = 1.0
	}
	out := w * mix
	if out > 1 {
		out = 1
	}
	return out
}
