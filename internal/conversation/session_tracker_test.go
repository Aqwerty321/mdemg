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

	for i := range tests {
		tt := &tests[i] // Use pointer to avoid copying mutex
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

	// Manually set last activity to past using the safe setter
	state := st.GetState("old-sess")
	state.SetLastActivityAt(time.Now().UTC().Add(-1 * time.Hour))

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

func TestSessionTracker_RecordRSICCall(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	t.Run("creates session if not exists", func(t *testing.T) {
		st.RecordRSICCall("sess-rsic-1")

		state := st.GetState("sess-rsic-1")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.RSICCallCount != 1 {
			t.Errorf("expected RSICCallCount=1, got %d", state.RSICCallCount)
		}
		if state.LastActivityAt.IsZero() {
			t.Error("expected LastActivityAt to be set")
		}
		if state.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
	})

	t.Run("increments RSICCallCount on existing session", func(t *testing.T) {
		st.RecordRSICCall("sess-rsic-2")
		st.RecordRSICCall("sess-rsic-2")
		st.RecordRSICCall("sess-rsic-2")

		state := st.GetState("sess-rsic-2")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.RSICCallCount != 3 {
			t.Errorf("expected RSICCallCount=3, got %d", state.RSICCallCount)
		}
	})
}

func TestSessionTracker_RecordSignalEmitted(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	t.Run("creates session with signal", func(t *testing.T) {
		st.RecordSignalEmitted("sess-signal-1", "WARN_NO_RESUME")

		state := st.GetState("sess-signal-1")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if len(state.SignalsEmitted) != 1 {
			t.Fatalf("expected 1 signal, got %d", len(state.SignalsEmitted))
		}
		if state.SignalsEmitted[0] != "WARN_NO_RESUME" {
			t.Errorf("expected signal=WARN_NO_RESUME, got %s", state.SignalsEmitted[0])
		}
		if state.LastActivityAt.IsZero() {
			t.Error("expected LastActivityAt to be set")
		}
	})

	t.Run("appends signals to existing session", func(t *testing.T) {
		st.RecordSignalEmitted("sess-signal-2", "SIGNAL_A")
		st.RecordSignalEmitted("sess-signal-2", "SIGNAL_B")
		st.RecordSignalEmitted("sess-signal-2", "SIGNAL_C")

		state := st.GetState("sess-signal-2")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if len(state.SignalsEmitted) != 3 {
			t.Fatalf("expected 3 signals, got %d", len(state.SignalsEmitted))
		}
		expected := []string{"SIGNAL_A", "SIGNAL_B", "SIGNAL_C"}
		for i, exp := range expected {
			if state.SignalsEmitted[i] != exp {
				t.Errorf("signal[%d]: expected %s, got %s", i, exp, state.SignalsEmitted[i])
			}
		}
	})
}

func TestSessionTracker_RecordObserveCall(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	t.Run("creates session if not exists", func(t *testing.T) {
		st.RecordObserveCall("sess-obs-1")

		state := st.GetState("sess-obs-1")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.ObserveCallCount != 1 {
			t.Errorf("expected ObserveCallCount=1, got %d", state.ObserveCallCount)
		}
		if state.LastActivityAt.IsZero() {
			t.Error("expected LastActivityAt to be set")
		}
	})

	t.Run("increments ObserveCallCount on existing session", func(t *testing.T) {
		st.RecordObserveCall("sess-obs-2")
		st.RecordObserveCall("sess-obs-2")
		st.RecordObserveCall("sess-obs-2")
		st.RecordObserveCall("sess-obs-2")

		state := st.GetState("sess-obs-2")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.ObserveCallCount != 4 {
			t.Errorf("expected ObserveCallCount=4, got %d", state.ObserveCallCount)
		}
	})

	t.Run("ObserveCallCount is distinct from ObservationsSinceResume", func(t *testing.T) {
		st.RecordObserveCall("sess-obs-3")
		st.RecordObserve("sess-obs-3")

		state := st.GetState("sess-obs-3")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.ObserveCallCount != 1 {
			t.Errorf("expected ObserveCallCount=1, got %d", state.ObserveCallCount)
		}
		if state.ObservationsSinceResume != 1 {
			t.Errorf("expected ObservationsSinceResume=1, got %d", state.ObservationsSinceResume)
		}
	})
}

func TestSessionState_NewFields(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	t.Run("RSICCallCount defaults to 0", func(t *testing.T) {
		st.RecordResume("sess-fields-1", "test-space")

		state := st.GetState("sess-fields-1")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.RSICCallCount != 0 {
			t.Errorf("expected RSICCallCount=0, got %d", state.RSICCallCount)
		}
	})

	t.Run("ObserveCallCount defaults to 0", func(t *testing.T) {
		st.RecordResume("sess-fields-2", "test-space")

		state := st.GetState("sess-fields-2")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.ObserveCallCount != 0 {
			t.Errorf("expected ObserveCallCount=0, got %d", state.ObserveCallCount)
		}
	})

	t.Run("SignalsEmitted defaults to nil", func(t *testing.T) {
		st.RecordResume("sess-fields-3", "test-space")

		state := st.GetState("sess-fields-3")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.SignalsEmitted != nil {
			t.Errorf("expected SignalsEmitted=nil, got %v", state.SignalsEmitted)
		}
	})

	t.Run("all new fields set correctly", func(t *testing.T) {
		st.RecordRSICCall("sess-fields-4")
		st.RecordRSICCall("sess-fields-4")
		st.RecordObserveCall("sess-fields-4")
		st.RecordObserveCall("sess-fields-4")
		st.RecordObserveCall("sess-fields-4")
		st.RecordSignalEmitted("sess-fields-4", "TEST_SIGNAL")

		state := st.GetState("sess-fields-4")
		if state == nil {
			t.Fatal("expected session state, got nil")
		}
		if state.RSICCallCount != 2 {
			t.Errorf("expected RSICCallCount=2, got %d", state.RSICCallCount)
		}
		if state.ObserveCallCount != 3 {
			t.Errorf("expected ObserveCallCount=3, got %d", state.ObserveCallCount)
		}
		if len(state.SignalsEmitted) != 1 {
			t.Errorf("expected 1 signal, got %d", len(state.SignalsEmitted))
		}
		if state.SignalsEmitted[0] != "TEST_SIGNAL" {
			t.Errorf("expected TEST_SIGNAL, got %s", state.SignalsEmitted[0])
		}
	})
}

func TestSessionTracker_CombinedOperations(t *testing.T) {
	st := NewSessionTracker(1 * time.Hour)
	defer st.Stop()

	t.Run("RecordResume + RecordRSICCall + RecordObserveCall", func(t *testing.T) {
		sessionID := "sess-combined-1"

		// Resume first
		st.RecordResume(sessionID, "combined-space")

		// Then RSIC calls
		st.RecordRSICCall(sessionID)
		st.RecordRSICCall(sessionID)

		// Then observe calls
		st.RecordObserveCall(sessionID)
		st.RecordObserveCall(sessionID)
		st.RecordObserveCall(sessionID)

		state := st.GetState(sessionID)
		if state == nil {
			t.Fatal("expected session state, got nil")
		}

		// Verify all fields
		if !state.Resumed {
			t.Error("expected Resumed=true")
		}
		if state.SpaceID != "combined-space" {
			t.Errorf("expected SpaceID=combined-space, got %s", state.SpaceID)
		}
		if state.RSICCallCount != 2 {
			t.Errorf("expected RSICCallCount=2, got %d", state.RSICCallCount)
		}
		if state.ObserveCallCount != 3 {
			t.Errorf("expected ObserveCallCount=3, got %d", state.ObserveCallCount)
		}
		if state.LastResumeAt.IsZero() {
			t.Error("expected LastResumeAt to be set")
		}
		if state.LastActivityAt.IsZero() {
			t.Error("expected LastActivityAt to be set")
		}
	})

	t.Run("all operations without resume", func(t *testing.T) {
		sessionID := "sess-combined-2"

		st.RecordRSICCall(sessionID)
		st.RecordObserveCall(sessionID)
		st.RecordSignalEmitted(sessionID, "NO_RESUME")
		st.RecordObserve(sessionID) // Also test old observe method

		state := st.GetState(sessionID)
		if state == nil {
			t.Fatal("expected session state, got nil")
		}

		if state.Resumed {
			t.Error("expected Resumed=false")
		}
		if state.RSICCallCount != 1 {
			t.Errorf("expected RSICCallCount=1, got %d", state.RSICCallCount)
		}
		if state.ObserveCallCount != 1 {
			t.Errorf("expected ObserveCallCount=1, got %d", state.ObserveCallCount)
		}
		if state.ObservationsSinceResume != 1 {
			t.Errorf("expected ObservationsSinceResume=1, got %d", state.ObservationsSinceResume)
		}
		if len(state.SignalsEmitted) != 1 {
			t.Fatalf("expected 1 signal, got %d", len(state.SignalsEmitted))
		}
		if state.SignalsEmitted[0] != "NO_RESUME" {
			t.Errorf("expected NO_RESUME signal, got %s", state.SignalsEmitted[0])
		}
	})

	t.Run("multiple signals with other operations", func(t *testing.T) {
		sessionID := "sess-combined-3"

		st.RecordResume(sessionID, "signal-space")
		st.RecordSignalEmitted(sessionID, "SIG_1")
		st.RecordRSICCall(sessionID)
		st.RecordSignalEmitted(sessionID, "SIG_2")
		st.RecordObserveCall(sessionID)
		st.RecordSignalEmitted(sessionID, "SIG_3")

		state := st.GetState(sessionID)
		if state == nil {
			t.Fatal("expected session state, got nil")
		}

		if !state.Resumed {
			t.Error("expected Resumed=true")
		}
		if state.RSICCallCount != 1 {
			t.Errorf("expected RSICCallCount=1, got %d", state.RSICCallCount)
		}
		if state.ObserveCallCount != 1 {
			t.Errorf("expected ObserveCallCount=1, got %d", state.ObserveCallCount)
		}
		if len(state.SignalsEmitted) != 3 {
			t.Fatalf("expected 3 signals, got %d", len(state.SignalsEmitted))
		}

		expectedSignals := []string{"SIG_1", "SIG_2", "SIG_3"}
		for i, exp := range expectedSignals {
			if state.SignalsEmitted[i] != exp {
				t.Errorf("signal[%d]: expected %s, got %s", i, exp, state.SignalsEmitted[i])
			}
		}
	})

	t.Run("LastActivityAt updated by all operations", func(t *testing.T) {
		sessionID := "sess-combined-4"

		st.RecordResume(sessionID, "activity-space")
		time1 := st.GetState(sessionID).GetLastActivityAt()

		time.Sleep(10 * time.Millisecond)

		st.RecordRSICCall(sessionID)
		time2 := st.GetState(sessionID).GetLastActivityAt()
		if !time2.After(time1) {
			t.Error("expected LastActivityAt to be updated by RecordRSICCall")
		}

		time.Sleep(10 * time.Millisecond)

		st.RecordObserveCall(sessionID)
		time3 := st.GetState(sessionID).GetLastActivityAt()
		if !time3.After(time2) {
			t.Error("expected LastActivityAt to be updated by RecordObserveCall")
		}

		time.Sleep(10 * time.Millisecond)

		st.RecordSignalEmitted(sessionID, "ACTIVITY_TEST")
		time4 := st.GetState(sessionID).GetLastActivityAt()
		if !time4.After(time3) {
			t.Error("expected LastActivityAt to be updated by RecordSignalEmitted")
		}
	})
}
