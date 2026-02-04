package conversation

import (
	"testing"
	"time"
)

func TestSessionTracker_RecordResume(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	st.RecordResume("sess-1", "test-space")

	state := st.GetState("sess-1")
	if state == nil {
		t.Fatal("expected session state, got nil")
	}
	if !state.Resumed {
		t.Error("expected Resumed=true")
	}
	if state.SpaceID != "test-space" {
		t.Errorf("expected SpaceID=test-space, got %s", state.SpaceID)
	}
	if state.LastResumeAt.IsZero() {
		t.Error("expected LastResumeAt to be set")
	}
}

func TestSessionTracker_RecordObserve(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	st.RecordObserve("sess-2")
	st.RecordObserve("sess-2")
	st.RecordObserve("sess-2")

	state := st.GetState("sess-2")
	if state == nil {
		t.Fatal("expected session state, got nil")
	}
	if state.ObservationsSinceResume != 3 {
		t.Errorf("expected 3 observations, got %d", state.ObservationsSinceResume)
	}
	if state.LastObserveAt.IsZero() {
		t.Error("expected LastObserveAt to be set")
	}
}

func TestSessionTracker_IsResumed(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	// Not tracked yet
	if st.IsResumed("unknown") {
		t.Error("expected unknown session to not be resumed")
	}

	// Observe without resume
	st.RecordObserve("sess-3")
	if st.IsResumed("sess-3") {
		t.Error("expected observe-only session to not be resumed")
	}

	// After resume
	st.RecordResume("sess-3", "")
	if !st.IsResumed("sess-3") {
		t.Error("expected session to be resumed after RecordResume")
	}
}

func TestSessionState_HealthScore(t *testing.T) {
	tests := []struct {
		name     string
		state    SessionState
		wantMin  float64
		wantMax  float64
	}{
		{
			name:    "empty session",
			state:   SessionState{},
			wantMin: 0.0,
			wantMax: 0.01,
		},
		{
			name: "resumed only",
			state: SessionState{
				Resumed:        true,
				LastActivityAt: time.Now().UTC(),
			},
			wantMin: 0.6, // 0.4 (resume) + 0.2 (recency)
			wantMax: 0.61,
		},
		{
			name: "resumed with observations",
			state: SessionState{
				Resumed:                 true,
				ObservationsSinceResume: 5,
				LastActivityAt:          time.Now().UTC(),
			},
			wantMin: 0.89, // 0.4 + 0.3 + 0.2 = 0.9
			wantMax: 0.91,
		},
		{
			name: "fully healthy",
			state: SessionState{
				Resumed:                 true,
				ObservationsSinceResume: 10,
				LastActivityAt:          time.Now().UTC(),
			},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name: "not resumed but observing",
			state: SessionState{
				Resumed:                 false,
				ObservationsSinceResume: 10,
				LastActivityAt:          time.Now().UTC(),
			},
			wantMin: 0.6, // 0.0 + 0.4 + 0.2
			wantMax: 0.61,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := tt.state.HealthScore()
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("HealthScore() = %f, want [%f, %f]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSessionTracker_Cleanup(t *testing.T) {
	st := NewSessionTracker(100 * time.Millisecond)
	defer st.Stop()

	st.RecordResume("old-sess", "")

	// Manually set last activity to past
	state := st.GetState("old-sess")
	state.LastActivityAt = time.Now().UTC().Add(-1 * time.Hour)

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	if st.GetState("old-sess") != nil {
		t.Error("expected old session to be cleaned up")
	}
}

func TestSessionTracker_GetState_Unknown(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	if st.GetState("nonexistent") != nil {
		t.Error("expected nil for unknown session")
	}
}
