package conversation

import (
	"testing"
	"time"
)

func TestOrgReviewStatusConstants(t *testing.T) {
	tests := []struct {
		status   OrgReviewStatus
		expected string
	}{
		{OrgReviewNone, "none"},
		{OrgReviewPending, "pending"},
		{OrgReviewApproved, "approved"},
		{OrgReviewRejected, "rejected"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("Expected status %q, got %q", tt.expected, string(tt.status))
		}
	}
}

func TestOrgReviewDecisionConstants(t *testing.T) {
	tests := []struct {
		decision OrgReviewDecision
		expected string
	}{
		{DecisionApprove, "approve"},
		{DecisionReject, "reject"},
	}

	for _, tt := range tests {
		if string(tt.decision) != tt.expected {
			t.Errorf("Expected decision %q, got %q", tt.expected, string(tt.decision))
		}
	}
}

func TestValidOrgReviewStatus(t *testing.T) {
	tests := []struct {
		status string
		valid  bool
	}{
		{"none", true},
		{"pending", true},
		{"approved", true},
		{"rejected", true},
		{"", true}, // Empty defaults to none
		{"invalid", false},
		{"PENDING", false}, // Case sensitive
	}

	for _, tt := range tests {
		result := ValidOrgReviewStatus(tt.status)
		if result != tt.valid {
			t.Errorf("ValidOrgReviewStatus(%q) = %v, want %v", tt.status, result, tt.valid)
		}
	}
}

func TestValidOrgReviewDecision(t *testing.T) {
	tests := []struct {
		decision string
		valid    bool
	}{
		{"approve", true},
		{"reject", true},
		{"", false},
		{"invalid", false},
		{"APPROVE", false}, // Case sensitive
	}

	for _, tt := range tests {
		result := ValidOrgReviewDecision(tt.decision)
		if result != tt.valid {
			t.Errorf("ValidOrgReviewDecision(%q) = %v, want %v", tt.decision, result, tt.valid)
		}
	}
}

func TestFlagForReviewRequest(t *testing.T) {
	req := FlagForReviewRequest{
		ObsID:               "obs-123",
		SpaceID:             "test-space",
		Reason:              "Valuable insight for team",
		SuggestedVisibility: "team",
		FlaggedBy:           "agent-claude",
	}

	if req.ObsID != "obs-123" {
		t.Errorf("Expected obs_id 'obs-123', got %q", req.ObsID)
	}
	if req.SuggestedVisibility != "team" {
		t.Errorf("Expected visibility 'team', got %q", req.SuggestedVisibility)
	}
}

func TestFlagForReviewResponse(t *testing.T) {
	now := time.Now()
	resp := FlagForReviewResponse{
		ObsID:            "obs-123",
		FlaggedForReview: true,
		ReviewStatus:     "pending",
		FlaggedAt:        now,
		FlaggedBy:        "agent-claude",
	}

	if !resp.FlaggedForReview {
		t.Error("Expected flagged_for_review to be true")
	}
	if resp.ReviewStatus != "pending" {
		t.Errorf("Expected review_status 'pending', got %q", resp.ReviewStatus)
	}
}

func TestPendingReview(t *testing.T) {
	now := time.Now()
	review := PendingReview{
		ObsID:               "obs-456",
		SpaceID:             "mdemg-dev",
		Content:             "Important architectural decision",
		ObsType:             "decision",
		FlaggedAt:           now,
		FlaggedBy:           "agent-claude",
		SuggestedVisibility: "team",
		FlagReason:          "Team should know about this",
		CreatedAt:           now.Add(-1 * time.Hour),
	}

	if review.ObsID != "obs-456" {
		t.Errorf("Expected obs_id 'obs-456', got %q", review.ObsID)
	}
	if review.ObsType != "decision" {
		t.Errorf("Expected obs_type 'decision', got %q", review.ObsType)
	}
}

func TestListPendingReviewsRequest(t *testing.T) {
	req := ListPendingReviewsRequest{
		SpaceID: "test-space",
		Limit:   25,
	}

	if req.SpaceID != "test-space" {
		t.Errorf("Expected space_id 'test-space', got %q", req.SpaceID)
	}
	if req.Limit != 25 {
		t.Errorf("Expected limit 25, got %d", req.Limit)
	}
}

func TestListPendingReviewsResponse(t *testing.T) {
	resp := ListPendingReviewsResponse{
		Reviews: []PendingReview{
			{ObsID: "1", Content: "Review 1"},
			{ObsID: "2", Content: "Review 2"},
		},
		Count: 2,
	}

	if resp.Count != 2 {
		t.Errorf("Expected count 2, got %d", resp.Count)
	}
	if len(resp.Reviews) != 2 {
		t.Errorf("Expected 2 reviews, got %d", len(resp.Reviews))
	}
}

func TestReviewDecisionRequest(t *testing.T) {
	req := ReviewDecisionRequest{
		ObsID:         "obs-789",
		SpaceID:       "test-space",
		Decision:      DecisionApprove,
		NewVisibility: "global",
		ReviewedBy:    "admin-user",
		Notes:         "Approved for global visibility",
	}

	if req.Decision != DecisionApprove {
		t.Errorf("Expected decision 'approve', got %q", req.Decision)
	}
	if req.NewVisibility != "global" {
		t.Errorf("Expected visibility 'global', got %q", req.NewVisibility)
	}
}

func TestReviewDecisionResponse(t *testing.T) {
	now := time.Now()
	resp := ReviewDecisionResponse{
		ObsID:         "obs-789",
		Decision:      "approve",
		NewVisibility: "team",
		ReviewedAt:    now,
		ReviewedBy:    "admin-user",
	}

	if resp.Decision != "approve" {
		t.Errorf("Expected decision 'approve', got %q", resp.Decision)
	}
	if resp.NewVisibility != "team" {
		t.Errorf("Expected visibility 'team', got %q", resp.NewVisibility)
	}
}

func TestOrgReviewStats(t *testing.T) {
	stats := OrgReviewStats{
		Pending:  5,
		Approved: 20,
		Rejected: 3,
	}

	if stats.Pending != 5 {
		t.Errorf("Expected pending 5, got %d", stats.Pending)
	}
	if stats.Approved != 20 {
		t.Errorf("Expected approved 20, got %d", stats.Approved)
	}
	if stats.Rejected != 3 {
		t.Errorf("Expected rejected 3, got %d", stats.Rejected)
	}

	// Total should be sum
	total := stats.Pending + stats.Approved + stats.Rejected
	if total != 28 {
		t.Errorf("Expected total 28, got %d", total)
	}
}

func TestGenerateOrgLevelAlert(t *testing.T) {
	obs := &Observation{
		ObsID:   "alert-obs-1",
		Content: "This is an important observation that should be reviewed by the organization before being shared widely.",
		ObsType: ObsTypeDecision,
	}

	alert := GenerateOrgLevelAlert(obs, "Architectural decision")

	if alert.ObsID != "alert-obs-1" {
		t.Errorf("Expected obs_id 'alert-obs-1', got %q", alert.ObsID)
	}
	if alert.AlertType != "org_review_required" {
		t.Errorf("Expected alert_type 'org_review_required', got %q", alert.AlertType)
	}
	if !alert.RequiresACK {
		t.Error("Expected requires_ack to be true")
	}
	if alert.Message == "" {
		t.Error("Expected non-empty message")
	}
}

func TestGenerateOrgLevelAlert_NoReason(t *testing.T) {
	obs := &Observation{
		ObsID:   "alert-obs-2",
		Content: "Some content",
		ObsType: ObsTypeLearning,
	}

	alert := GenerateOrgLevelAlert(obs, "")

	// Should have default message
	if alert.Message == "" {
		t.Error("Expected non-empty default message")
	}
}

func TestTruncateForAlert(t *testing.T) {
	tests := []struct {
		content  string
		maxLen   int
		expected string
	}{
		{"Short", 10, "Short"},
		{"Hello World", 8, "Hello..."},
		{"", 10, ""},
	}

	for _, tt := range tests {
		result := truncateForAlert(tt.content, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateForAlert(%q, %d) = %q, want %q", tt.content, tt.maxLen, result, tt.expected)
		}
	}
}

func TestOrgLevelAlert(t *testing.T) {
	now := time.Now()
	alert := OrgLevelAlert{
		ObsID:       "obs-alert",
		Content:     "Alert content",
		ObsType:     "decision",
		AlertType:   "org_review_required",
		Message:     "Please review this observation",
		FlaggedAt:   now,
		RequiresACK: true,
	}

	if alert.AlertType != "org_review_required" {
		t.Errorf("Expected alert_type 'org_review_required', got %q", alert.AlertType)
	}
	if !alert.RequiresACK {
		t.Error("Expected requires_ack to be true")
	}
}
