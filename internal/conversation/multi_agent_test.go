package conversation

import (
	"testing"
)

// =============================================================================
// Phase 3C: Multi-Agent CMS Tests
// Tests agent isolation, team visibility, and cross-session identity.
// These are unit tests for the data model and filtering logic.
// Integration tests with Neo4j are in tests/integration/.
// =============================================================================

func TestObservation_AgentID(t *testing.T) {
	obs := Observation{
		ObsID:     "obs-1",
		SpaceID:   "test-space",
		SessionID: "session-1",
		AgentID:   "agent-claude",
		UserID:    "user-123",
	}

	if obs.AgentID != "agent-claude" {
		t.Errorf("AgentID = %q, want %q", obs.AgentID, "agent-claude")
	}
	// AgentID and UserID can coexist
	if obs.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", obs.UserID, "user-123")
	}
}

func TestObserveRequest_AgentID(t *testing.T) {
	req := ObserveRequest{
		SpaceID:   "test-space",
		SessionID: "session-1",
		Content:   "test observation",
		AgentID:   "agent-alpha",
		UserID:    "user-123",
	}

	if req.AgentID != "agent-alpha" {
		t.Errorf("AgentID = %q, want %q", req.AgentID, "agent-alpha")
	}
}

func TestCorrectRequest_AgentID(t *testing.T) {
	req := CorrectRequest{
		SpaceID:   "test-space",
		SessionID: "session-1",
		Incorrect: "wrong info",
		Correct:   "right info",
		AgentID:   "agent-beta",
	}

	if req.AgentID != "agent-beta" {
		t.Errorf("AgentID = %q, want %q", req.AgentID, "agent-beta")
	}
}

func TestResumeRequest_CrossSessionAgent(t *testing.T) {
	// When AgentID is set, resume should work across sessions
	req := ResumeRequest{
		SpaceID:         "test-space",
		AgentID:         "agent-claude",
		MaxObservations: 20,
	}

	// SessionID should be empty (cross-session resume)
	if req.SessionID != "" {
		t.Errorf("SessionID should be empty for cross-session resume, got %q", req.SessionID)
	}
	if req.AgentID != "agent-claude" {
		t.Errorf("AgentID = %q, want %q", req.AgentID, "agent-claude")
	}
}

func TestResumeRequest_SessionAndAgent(t *testing.T) {
	// When both are set, agent takes precedence (session filter is skipped)
	req := ResumeRequest{
		SpaceID:   "test-space",
		SessionID: "session-old",
		AgentID:   "agent-claude",
	}

	if req.AgentID == "" {
		t.Error("AgentID should be set")
	}
	if req.SessionID == "" {
		t.Error("SessionID should still be set even with AgentID")
	}
}

func TestRecallRequest_AgentFilter(t *testing.T) {
	req := RecallRequest{
		SpaceID: "test-space",
		Query:   "what decisions were made?",
		AgentID: "agent-alpha",
		TopK:    10,
	}

	if req.AgentID != "agent-alpha" {
		t.Errorf("AgentID = %q, want %q", req.AgentID, "agent-alpha")
	}
}

// TestAgentVisibilityModel verifies the three-tier visibility semantics
// with agent identity.
func TestAgentVisibilityModel(t *testing.T) {
	tests := []struct {
		name              string
		obsVisibility     Visibility
		obsAgentID        string
		requestingAgentID string
		shouldBeVisible   bool
	}{
		{
			name:              "private obs visible to owning agent",
			obsVisibility:     VisibilityPrivate,
			obsAgentID:        "agent-alpha",
			requestingAgentID: "agent-alpha",
			shouldBeVisible:   true,
		},
		{
			name:              "private obs hidden from other agent",
			obsVisibility:     VisibilityPrivate,
			obsAgentID:        "agent-alpha",
			requestingAgentID: "agent-beta",
			shouldBeVisible:   false,
		},
		{
			name:              "team obs visible to any agent in space",
			obsVisibility:     VisibilityTeam,
			obsAgentID:        "agent-alpha",
			requestingAgentID: "agent-beta",
			shouldBeVisible:   true,
		},
		{
			name:              "global obs visible to any agent",
			obsVisibility:     VisibilityGlobal,
			obsAgentID:        "agent-alpha",
			requestingAgentID: "agent-gamma",
			shouldBeVisible:   true,
		},
		{
			name:              "no requesting agent - all visible (backward compat)",
			obsVisibility:     VisibilityPrivate,
			obsAgentID:        "agent-alpha",
			requestingAgentID: "",
			shouldBeVisible:   true, // No filter applied when no requestor
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the visibility check logic from fetchRecentObservations
			visible := isVisibleToAgent(tt.obsVisibility, tt.obsAgentID, tt.requestingAgentID)
			if visible != tt.shouldBeVisible {
				t.Errorf("visibility check = %v, want %v (vis=%s, owner=%s, requestor=%s)",
					visible, tt.shouldBeVisible, tt.obsVisibility, tt.obsAgentID, tt.requestingAgentID)
			}
		})
	}
}

// isVisibleToAgent simulates the Cypher visibility filter logic in Go
// for unit testing. The actual filtering happens in Neo4j queries.
func isVisibleToAgent(visibility Visibility, obsAgentID, requestingAgentID string) bool {
	// No requestor = no filtering (backward compatible)
	if requestingAgentID == "" {
		return true
	}
	// Non-private observations are always visible
	if visibility != VisibilityPrivate {
		return true
	}
	// Private: only visible to owning agent
	return obsAgentID == requestingAgentID
}

func TestAgentIsolation_DifferentAgentsSameSpace(t *testing.T) {
	// Simulate two agents in the same space with private observations
	agentA := "agent-alpha"
	agentB := "agent-beta"

	obsA := Observation{
		ObsID:      "obs-a1",
		SpaceID:    "shared-space",
		AgentID:    agentA,
		Visibility: VisibilityPrivate,
		Content:    "Agent A private learning",
	}

	obsB := Observation{
		ObsID:      "obs-b1",
		SpaceID:    "shared-space",
		AgentID:    agentB,
		Visibility: VisibilityPrivate,
		Content:    "Agent B private learning",
	}

	// Agent A should see its own but not B's
	if !isVisibleToAgent(obsA.Visibility, obsA.AgentID, agentA) {
		t.Error("Agent A should see its own private observations")
	}
	if isVisibleToAgent(obsB.Visibility, obsB.AgentID, agentA) {
		t.Error("Agent A should NOT see Agent B's private observations")
	}

	// Agent B should see its own but not A's
	if !isVisibleToAgent(obsB.Visibility, obsB.AgentID, agentB) {
		t.Error("Agent B should see its own private observations")
	}
	if isVisibleToAgent(obsA.Visibility, obsA.AgentID, agentB) {
		t.Error("Agent B should NOT see Agent A's private observations")
	}
}

func TestTeamVisibility_SharedAcrossAgents(t *testing.T) {
	// Team observations should be visible to all agents in the space
	obsTeam := Observation{
		ObsID:      "obs-team-1",
		SpaceID:    "shared-space",
		AgentID:    "agent-alpha",
		Visibility: VisibilityTeam,
		Content:    "Shared team decision",
	}

	agents := []string{"agent-alpha", "agent-beta", "agent-gamma"}
	for _, agent := range agents {
		if !isVisibleToAgent(obsTeam.Visibility, obsTeam.AgentID, agent) {
			t.Errorf("Agent %s should see team-level observation", agent)
		}
	}
}

func TestCrossSessionResume_AgentIdentity(t *testing.T) {
	// Test that an agent's observations from multiple sessions are queryable
	agent := "agent-claude"

	sessions := []Observation{
		{SessionID: "session-1", AgentID: agent, Content: "Learning from session 1"},
		{SessionID: "session-2", AgentID: agent, Content: "Learning from session 2"},
		{SessionID: "session-3", AgentID: agent, Content: "Learning from session 3"},
	}

	// All observations belong to the same agent
	for _, obs := range sessions {
		if obs.AgentID != agent {
			t.Errorf("obs from %s should have AgentID=%s", obs.SessionID, agent)
		}
	}

	// A resume with AgentID and no SessionID should be able to find all three
	req := ResumeRequest{
		SpaceID:         "mdemg-dev",
		AgentID:         agent,
		MaxObservations: 20,
	}

	// When AgentID is set and SessionID is empty, cross-session resume is enabled
	if req.SessionID != "" {
		t.Error("cross-session resume should not have a session filter")
	}
	if req.AgentID == "" {
		t.Error("cross-session resume requires AgentID")
	}
}

func TestBackwardCompatibility_NoAgentID(t *testing.T) {
	// Requests without AgentID should work exactly as before
	observeReq := ObserveRequest{
		SpaceID:   "test-space",
		SessionID: "session-1",
		Content:   "observation without agent",
		UserID:    "user-123",
	}

	if observeReq.AgentID != "" {
		t.Error("AgentID should be empty when not set")
	}

	resumeReq := ResumeRequest{
		SpaceID:          "test-space",
		SessionID:        "session-1",
		RequestingUserID: "user-123",
	}

	if resumeReq.AgentID != "" {
		t.Error("AgentID should be empty when not set")
	}

	recallReq := RecallRequest{
		SpaceID:          "test-space",
		Query:            "test query",
		RequestingUserID: "user-123",
	}

	if recallReq.AgentID != "" {
		t.Error("AgentID should be empty when not set")
	}
}
