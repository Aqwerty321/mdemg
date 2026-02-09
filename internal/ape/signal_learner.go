package ape

import (
	"sync"
)

// SignalEffectiveness tracks the effectiveness of a meta-cognitive signal.
type SignalEffectiveness struct {
	Code         string  `json:"code"`
	Emissions    int     `json:"emissions"`
	Responses    int     `json:"responses"`
	Strength     float64 `json:"strength"`     // Hebbian strength (0.1-1.0)
	ResponseRate float64 `json:"response_rate"` // responses/emissions
}

// SignalLearner tracks signal emission/response patterns using Hebbian learning.
// In-memory only (not Neo4j-persisted) for simplicity.
type SignalLearner struct {
	mu        sync.RWMutex
	signals   map[string]*signalState
	decayRate float64
	boostRate float64
}

type signalState struct {
	emissions int
	responses int
	strength  float64
}

// NewSignalLearner creates a learner with configurable decay/boost rates.
func NewSignalLearner(decayRate, boostRate float64) *SignalLearner {
	if decayRate <= 0 {
		decayRate = 0.05
	}
	if boostRate <= 0 {
		boostRate = 0.1
	}
	return &SignalLearner{
		signals:   make(map[string]*signalState),
		decayRate: decayRate,
		boostRate: boostRate,
	}
}

// RecordEmission records that a signal was emitted (shown to agent).
// If no response follows, strength decays.
func (sl *SignalLearner) RecordEmission(code string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	s, ok := sl.signals[code]
	if !ok {
		s = &signalState{strength: 0.5}
		sl.signals[code] = s
	}
	s.emissions++

	// Decay: emission without response reduces strength
	s.strength -= sl.decayRate
	if s.strength < 0.1 {
		s.strength = 0.1
	}
}

// RecordResponse records that the agent acted on a signal.
// This boosts the signal's strength via Hebbian reinforcement.
func (sl *SignalLearner) RecordResponse(code string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	s, ok := sl.signals[code]
	if !ok {
		s = &signalState{strength: 0.5}
		sl.signals[code] = s
	}
	s.responses++

	// Boost: response increases strength (undo decay from emission)
	s.strength += sl.boostRate + sl.decayRate
	if s.strength > 1.0 {
		s.strength = 1.0
	}
}

// GetStrength returns the current Hebbian strength for a signal code.
func (sl *SignalLearner) GetStrength(code string) float64 {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	s, ok := sl.signals[code]
	if !ok {
		return 0.5 // Default
	}
	return s.strength
}

// GetAllEffectiveness returns effectiveness stats for all tracked signals.
func (sl *SignalLearner) GetAllEffectiveness() []SignalEffectiveness {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	result := make([]SignalEffectiveness, 0, len(sl.signals))
	for code, s := range sl.signals {
		rate := 0.0
		if s.emissions > 0 {
			rate = float64(s.responses) / float64(s.emissions)
		}
		result = append(result, SignalEffectiveness{
			Code:         code,
			Emissions:    s.emissions,
			Responses:    s.responses,
			Strength:     s.strength,
			ResponseRate: rate,
		})
	}
	return result
}
