package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// OrgReviewStatus represents the review state of an observation
type OrgReviewStatus string

const (
	OrgReviewNone     OrgReviewStatus = "none"
	OrgReviewPending  OrgReviewStatus = "pending"
	OrgReviewApproved OrgReviewStatus = "approved"
	OrgReviewRejected OrgReviewStatus = "rejected"
)

// OrgReviewDecision represents a reviewer's decision
type OrgReviewDecision string

const (
	DecisionApprove OrgReviewDecision = "approve"
	DecisionReject  OrgReviewDecision = "reject"
)

// OrgReviewService handles org-level review workflows
type OrgReviewService struct {
	driver neo4j.DriverWithContext
}

// NewOrgReviewService creates a new org review service
func NewOrgReviewService(driver neo4j.DriverWithContext) *OrgReviewService {
	return &OrgReviewService{driver: driver}
}

// FlagForReviewRequest is the request to flag an observation for org review
type FlagForReviewRequest struct {
	ObsID               string `json:"obs_id"`
	SpaceID             string `json:"space_id"`
	Reason              string `json:"reason,omitempty"`
	SuggestedVisibility string `json:"suggested_visibility,omitempty"` // team or global
	FlaggedBy           string `json:"flagged_by,omitempty"`
}

// FlagForReviewResponse is the response after flagging
type FlagForReviewResponse struct {
	ObsID            string    `json:"obs_id"`
	FlaggedForReview bool      `json:"flagged_for_review"`
	ReviewStatus     string    `json:"review_status"`
	FlaggedAt        time.Time `json:"flagged_at"`
	FlaggedBy        string    `json:"flagged_by,omitempty"`
}

// FlagForReview flags an observation for org-level review
func (s *OrgReviewService) FlagForReview(ctx context.Context, req *FlagForReviewRequest) (*FlagForReviewResponse, error) {
	if req.ObsID == "" {
		return nil, fmt.Errorf("obs_id is required")
	}
	if req.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}

	now := time.Now()
	visibility := req.SuggestedVisibility
	if visibility == "" {
		visibility = string(VisibilityTeam)
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (o:Observation {obs_id: $obsId, space_id: $spaceId})
			SET o.org_review_status = $status,
			    o.org_flagged_at = datetime($flaggedAt),
			    o.org_flagged_by = $flaggedBy,
			    o.org_suggested_visibility = $visibility,
			    o.org_flag_reason = $reason
			RETURN o.obs_id AS id
		`
		params := map[string]interface{}{
			"obsId":     req.ObsID,
			"spaceId":   req.SpaceID,
			"status":    string(OrgReviewPending),
			"flaggedAt": now.Format(time.RFC3339),
			"flaggedBy": req.FlaggedBy,
			"visibility": visibility,
			"reason":    req.Reason,
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("observation not found")
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}

	return &FlagForReviewResponse{
		ObsID:            req.ObsID,
		FlaggedForReview: true,
		ReviewStatus:     string(OrgReviewPending),
		FlaggedAt:        now,
		FlaggedBy:        req.FlaggedBy,
	}, nil
}

// PendingReview represents a pending org review item
type PendingReview struct {
	ObsID               string    `json:"obs_id"`
	SpaceID             string    `json:"space_id"`
	Content             string    `json:"content"`
	ObsType             string    `json:"obs_type"`
	FlaggedAt           time.Time `json:"flagged_at"`
	FlaggedBy           string    `json:"flagged_by,omitempty"`
	SuggestedVisibility string    `json:"suggested_visibility,omitempty"`
	FlagReason          string    `json:"flag_reason,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// ListPendingReviewsRequest is the request to list pending reviews
type ListPendingReviewsRequest struct {
	SpaceID string `json:"space_id"`
	Limit   int    `json:"limit,omitempty"`
}

// ListPendingReviewsResponse is the response containing pending reviews
type ListPendingReviewsResponse struct {
	Reviews []PendingReview `json:"reviews"`
	Count   int             `json:"count"`
}

// ListPendingReviews returns observations pending org-level review
func (s *OrgReviewService) ListPendingReviews(ctx context.Context, req *ListPendingReviewsRequest) (*ListPendingReviewsResponse, error) {
	if req.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (o:Observation {space_id: $spaceId, org_review_status: $status})
			RETURN o
			ORDER BY o.org_flagged_at DESC
			LIMIT $limit
		`
		params := map[string]interface{}{
			"spaceId": req.SpaceID,
			"status":  string(OrgReviewPending),
			"limit":   int64(limit),
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		reviews := make([]PendingReview, 0)
		for res.Next(ctx) {
			node := res.Record().Values[0].(neo4j.Node)
			props := node.Props

			review := PendingReview{
				ObsID:   props["obs_id"].(string),
				SpaceID: props["space_id"].(string),
				Content: getStringProp(props, "content"),
				ObsType: getStringProp(props, "obs_type"),
			}

			if flaggedAt, ok := props["org_flagged_at"].(time.Time); ok {
				review.FlaggedAt = flaggedAt
			}
			if createdAt, ok := props["created_at"].(time.Time); ok {
				review.CreatedAt = createdAt
			}

			review.FlaggedBy = getStringProp(props, "org_flagged_by")
			review.SuggestedVisibility = getStringProp(props, "org_suggested_visibility")
			review.FlagReason = getStringProp(props, "org_flag_reason")

			reviews = append(reviews, review)
		}
		return reviews, nil
	})

	if err != nil {
		return nil, err
	}

	reviews := result.([]PendingReview)
	return &ListPendingReviewsResponse{
		Reviews: reviews,
		Count:   len(reviews),
	}, nil
}

// ReviewDecisionRequest is the request to approve/reject an observation
type ReviewDecisionRequest struct {
	ObsID         string            `json:"obs_id"`
	SpaceID       string            `json:"space_id"`
	Decision      OrgReviewDecision `json:"decision"`
	NewVisibility string            `json:"visibility,omitempty"` // team or global
	ReviewedBy    string            `json:"reviewed_by,omitempty"`
	Notes         string            `json:"notes,omitempty"`
}

// ReviewDecisionResponse is the response after a review decision
type ReviewDecisionResponse struct {
	ObsID         string    `json:"obs_id"`
	Decision      string    `json:"decision"`
	NewVisibility string    `json:"new_visibility,omitempty"`
	ReviewedAt    time.Time `json:"reviewed_at"`
	ReviewedBy    string    `json:"reviewed_by,omitempty"`
}

// ProcessDecision processes an approve/reject decision
func (s *OrgReviewService) ProcessDecision(ctx context.Context, req *ReviewDecisionRequest) (*ReviewDecisionResponse, error) {
	if req.ObsID == "" {
		return nil, fmt.Errorf("obs_id is required")
	}
	if req.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}
	if req.Decision != DecisionApprove && req.Decision != DecisionReject {
		return nil, fmt.Errorf("decision must be 'approve' or 'reject'")
	}

	now := time.Now()

	// Determine final status and visibility
	var finalStatus OrgReviewStatus
	var finalVisibility string

	if req.Decision == DecisionApprove {
		finalStatus = OrgReviewApproved
		finalVisibility = req.NewVisibility
		if finalVisibility == "" {
			finalVisibility = string(VisibilityTeam)
		}
	} else {
		finalStatus = OrgReviewRejected
		finalVisibility = string(VisibilityPrivate) // Rejected stays private
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (o:Observation {obs_id: $obsId, space_id: $spaceId})
			WHERE o.org_review_status = $pendingStatus
			SET o.org_review_status = $newStatus,
			    o.visibility = $visibility,
			    o.org_reviewed_at = datetime($reviewedAt),
			    o.org_reviewed_by = $reviewedBy,
			    o.org_review_notes = $notes
			RETURN o.obs_id AS id
		`
		params := map[string]interface{}{
			"obsId":         req.ObsID,
			"spaceId":       req.SpaceID,
			"pendingStatus": string(OrgReviewPending),
			"newStatus":     string(finalStatus),
			"visibility":    finalVisibility,
			"reviewedAt":    now.Format(time.RFC3339),
			"reviewedBy":    req.ReviewedBy,
			"notes":         req.Notes,
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("observation not found or not pending review")
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}

	return &ReviewDecisionResponse{
		ObsID:         req.ObsID,
		Decision:      string(req.Decision),
		NewVisibility: finalVisibility,
		ReviewedAt:    now,
		ReviewedBy:    req.ReviewedBy,
	}, nil
}

// GetReviewStats returns statistics about org reviews
type OrgReviewStats struct {
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Rejected int `json:"rejected"`
}

// GetReviewStats returns review statistics for a space
func (s *OrgReviewService) GetReviewStats(ctx context.Context, spaceID string) (*OrgReviewStats, error) {
	if spaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (o:Observation {space_id: $spaceId})
			WHERE o.org_review_status IS NOT NULL
			RETURN o.org_review_status AS status, count(o) AS count
		`
		params := map[string]interface{}{
			"spaceId": spaceID,
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		stats := &OrgReviewStats{}
		for res.Next(ctx) {
			status, _ := res.Record().Get("status")
			count, _ := res.Record().Get("count")
			countInt := int(count.(int64))

			switch status.(string) {
			case string(OrgReviewPending):
				stats.Pending = countInt
			case string(OrgReviewApproved):
				stats.Approved = countInt
			case string(OrgReviewRejected):
				stats.Rejected = countInt
			}
		}
		return stats, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*OrgReviewStats), nil
}

// =============================================================================
// User Alert Integration
// =============================================================================

// OrgLevelAlert represents an alert for org-level observation
type OrgLevelAlert struct {
	ObsID       string    `json:"obs_id"`
	Content     string    `json:"content"`
	ObsType     string    `json:"obs_type"`
	AlertType   string    `json:"alert_type"` // "org_review_required"
	Message     string    `json:"message"`
	FlaggedAt   time.Time `json:"flagged_at"`
	RequiresACK bool      `json:"requires_ack"`
}

// GenerateOrgLevelAlert creates an alert for user review
func GenerateOrgLevelAlert(obs *Observation, reason string) *OrgLevelAlert {
	message := "This observation has been flagged for org-level review. Please review before ingestion."
	if reason != "" {
		message = fmt.Sprintf("Flagged for org review: %s", reason)
	}

	return &OrgLevelAlert{
		ObsID:       obs.ObsID,
		Content:     truncateForAlert(obs.Content, 200),
		ObsType:     string(obs.ObsType),
		AlertType:   "org_review_required",
		Message:     message,
		FlaggedAt:   time.Now(),
		RequiresACK: true,
	}
}

// truncateForAlert truncates content for display in alert
func truncateForAlert(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen-3] + "..."
}

// =============================================================================
// Validation Helpers
// =============================================================================

// ValidOrgReviewStatus checks if a status value is valid
func ValidOrgReviewStatus(status string) bool {
	switch OrgReviewStatus(status) {
	case OrgReviewNone, OrgReviewPending, OrgReviewApproved, OrgReviewRejected:
		return true
	case "": // Empty defaults to none
		return true
	}
	return false
}

// ValidOrgReviewDecision checks if a decision value is valid
func ValidOrgReviewDecision(decision string) bool {
	switch OrgReviewDecision(decision) {
	case DecisionApprove, DecisionReject:
		return true
	}
	return false
}
