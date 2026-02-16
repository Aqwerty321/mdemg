package conversation

import (
	"sync"
	"time"
)

// SessionState tracks the CMS usage state for an agent session.
type SessionState struct {
	mu                      sync.RWMutex `json:"-"`
	SessionID               string       `json:"session_id"`
	SpaceID                 string       `json:"space_id,omitempty"`
	Resumed                 bool         `json:"resumed"`
	LastResumeAt            time.Time    `json:"last_resume_at,omitempty"`
	ObservationsSinceResume int          `json:"observations_since_resume"`
	LastObserveAt           time.Time    `json:"last_observe_at,omitempty"`
	LastActivityAt          time.Time    `json:"last_activity_at"`
	RSICCallCount           int          `json:"rsic_call_count"`
	ObserveCallCount        int          `json:"observe_call_count"`
	SignalsEmitted          []string     `json:"signals_emitted,omitempty"`
	CreatedAt               time.Time    `json:"created_at"`
}

// SetLastActivityAt safely sets the LastActivityAt field.
func (s *SessionState) SetLastActivityAt(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActivityAt = t
}

// GetLastActivityAt safely gets the LastActivityAt field.
func (s *SessionState) GetLastActivityAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastActivityAt
}

// HealthScore computes a session health score (0.0 - 1.0).
// Penalizes: no resume, low observation count.
// Rewards: resumed session, regular observations.
func (s *SessionState) HealthScore() float64 {
	score := 0.0

	// Base: resumed (0.4)
	if s.Resumed {
		score += 0.4
	}

	// Observations: up to 0.4 (1+ = 0.1, 3+ = 0.2, 5+ = 0.3, 10+ = 0.4)
	switch {
	case s.ObservationsSinceResume >= 10:
		score += 0.4
	case s.ObservationsSinceResume >= 5:
		score += 0.3
	case s.ObservationsSinceResume >= 3:
		score += 0.2
	case s.ObservationsSinceResume >= 1:
		score += 0.1
	}

	// Recency bonus: activity within last 10 minutes (0.2)
	if !s.LastActivityAt.IsZero() && time.Since(s.LastActivityAt) < 10*time.Minute {
		score += 0.2
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// SessionTracker tracks per-session CMS usage via an in-memory sync.Map.
// Sessions auto-expire after the configured TTL.
type SessionTracker struct {
	sessions sync.Map // map[string]*SessionState
	ttl      time.Duration
	stopCh   chan struct{}
}

// NewSessionTracker creates a tracker with TTL-based cleanup.
// The cleanup goroutine runs every ttl/2.
func NewSessionTracker(ttl time.Duration) *SessionTracker {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	st := &SessionTracker{
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	go st.cleanupLoop()
	return st
}

// RecordResume marks that a session has called /resume.
func (st *SessionTracker) RecordResume(sessionID, spaceID string) {
	now := time.Now().UTC()
	val, loaded := st.sessions.LoadOrStore(sessionID, &SessionState{
		SessionID:      sessionID,
		SpaceID:        spaceID,
		Resumed:        true,
		LastResumeAt:   now,
		LastActivityAt: now,
		CreatedAt:      now,
	})
	if loaded {
		state := val.(*SessionState)
		state.mu.Lock()
		state.Resumed = true
		state.LastResumeAt = now
		state.LastActivityAt = now
		if spaceID != "" {
			state.SpaceID = spaceID
		}
		state.mu.Unlock()
	}
}

// RecordObserve records that a session made an observation.
func (st *SessionTracker) RecordObserve(sessionID string) {
	now := time.Now().UTC()
	val, loaded := st.sessions.LoadOrStore(sessionID, &SessionState{
		SessionID:               sessionID,
		ObservationsSinceResume: 1,
		LastObserveAt:           now,
		LastActivityAt:          now,
		CreatedAt:               now,
	})
	if loaded {
		state := val.(*SessionState)
		state.mu.Lock()
		state.ObservationsSinceResume++
		state.LastObserveAt = now
		state.LastActivityAt = now
		state.mu.Unlock()
	}
}

// RecordActivity records generic activity for a session (keeps it alive for TTL).
func (st *SessionTracker) RecordActivity(sessionID string) {
	now := time.Now().UTC()
	val, loaded := st.sessions.LoadOrStore(sessionID, &SessionState{
		SessionID:      sessionID,
		LastActivityAt: now,
		CreatedAt:      now,
	})
	if loaded {
		state := val.(*SessionState)
		state.mu.Lock()
		state.LastActivityAt = now
		state.mu.Unlock()
	}
}

// RecordRSICCall records that a session called an RSIC endpoint.
func (st *SessionTracker) RecordRSICCall(sessionID string) {
	now := time.Now().UTC()
	val, loaded := st.sessions.LoadOrStore(sessionID, &SessionState{
		SessionID:      sessionID,
		RSICCallCount:  1,
		LastActivityAt: now,
		CreatedAt:      now,
	})
	if loaded {
		state := val.(*SessionState)
		state.mu.Lock()
		state.RSICCallCount++
		state.LastActivityAt = now
		state.mu.Unlock()
	}
}

// RecordSignalEmitted records that a signal was emitted for this session.
func (st *SessionTracker) RecordSignalEmitted(sessionID, code string) {
	now := time.Now().UTC()
	val, loaded := st.sessions.LoadOrStore(sessionID, &SessionState{
		SessionID:      sessionID,
		SignalsEmitted: []string{code},
		LastActivityAt: now,
		CreatedAt:      now,
	})
	if loaded {
		state := val.(*SessionState)
		state.mu.Lock()
		state.SignalsEmitted = append(state.SignalsEmitted, code)
		state.LastActivityAt = now
		state.mu.Unlock()
	}
}

// RecordObserveCall records that a session made an observe call (distinct from RecordObserve).
func (st *SessionTracker) RecordObserveCall(sessionID string) {
	now := time.Now().UTC()
	val, loaded := st.sessions.LoadOrStore(sessionID, &SessionState{
		SessionID:        sessionID,
		ObserveCallCount: 1,
		LastActivityAt:   now,
		CreatedAt:        now,
	})
	if loaded {
		state := val.(*SessionState)
		state.mu.Lock()
		state.ObserveCallCount++
		state.LastActivityAt = now
		state.mu.Unlock()
	}
}

// GetState returns the current state for a session, or nil if not tracked.
func (st *SessionTracker) GetState(sessionID string) *SessionState {
	val, ok := st.sessions.Load(sessionID)
	if !ok {
		return nil
	}
	return val.(*SessionState)
}

// GetAllStates returns all currently tracked session states.
func (st *SessionTracker) GetAllStates() []*SessionState {
	var states []*SessionState
	st.sessions.Range(func(_, val any) bool {
		states = append(states, val.(*SessionState))
		return true
	})
	return states
}

// IsResumed returns whether the session has called /resume.
func (st *SessionTracker) IsResumed(sessionID string) bool {
	state := st.GetState(sessionID)
	if state == nil {
		return false
	}
	return state.Resumed
}

// Stop terminates the cleanup goroutine.
func (st *SessionTracker) Stop() {
	close(st.stopCh)
}

// cleanupLoop periodically removes expired sessions.
func (st *SessionTracker) cleanupLoop() {
	ticker := time.NewTicker(st.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			st.cleanup()
		case <-st.stopCh:
			return
		}
	}
}

func (st *SessionTracker) cleanup() {
	cutoff := time.Now().UTC().Add(-st.ttl)
	st.sessions.Range(func(key, value any) bool {
		state := value.(*SessionState)
		state.mu.RLock()
		expired := state.LastActivityAt.Before(cutoff)
		state.mu.RUnlock()
		if expired {
			st.sessions.Delete(key)
		}
		return true
	})
}
