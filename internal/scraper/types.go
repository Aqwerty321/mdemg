package scraper

import "time"

// Job status constants
const (
	StatusPending        = "pending"
	StatusRunning        = "running"
	StatusAwaitingReview = "awaiting_review"
	StatusCompleted      = "completed"
	StatusCancelled      = "cancelled"
	StatusFailed         = "failed"
)

// Content status constants
const (
	ContentPendingReview = "pending_review"
	ContentApproved      = "approved"
	ContentRejected      = "rejected"
	ContentIngested      = "ingested"
)

// Extraction profile constants
const (
	ProfileDocumentation = "documentation"
	ProfileGeneric       = "generic"
)

// ScrapeJobRequest is the request to create a new scraping job.
type ScrapeJobRequest struct {
	URLs          []string      `json:"urls" validate:"required,min=1"`
	TargetSpaceID string        `json:"target_space_id,omitempty"`
	Options       ScrapeOptions `json:"options,omitempty"`
}

// ScrapeOptions configures how URLs are scraped.
type ScrapeOptions struct {
	ExtractionProfile string     `json:"extraction_profile,omitempty"` // "documentation" or "generic"
	MaxDepth          int        `json:"max_depth,omitempty"`
	MaxPages          int        `json:"max_pages,omitempty"`
	FollowLinks       bool       `json:"follow_links,omitempty"`
	Auth              ScrapeAuth `json:"auth,omitempty"`
	DelayMs           int        `json:"delay_ms,omitempty"`
	TimeoutMs         int        `json:"timeout_ms,omitempty"`
}

// ScrapeAuth configures authentication for scraping.
type ScrapeAuth struct {
	Type        string            `json:"type,omitempty"` // "none", "cookie", "header", "basic"
	Credentials map[string]string `json:"credentials,omitempty"`
}

// ScrapeJob represents a scraping job stored in Neo4j.
type ScrapeJob struct {
	JobID         string     `json:"job_id"`
	Status        string     `json:"status"`
	URLs          []string   `json:"urls"`
	TargetSpaceID string     `json:"target_space_id"`
	Options       ScrapeOptions `json:"options,omitempty"`
	TotalURLs     int        `json:"total_urls"`
	ProcessedURLs int        `json:"processed_urls"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// ScrapedContent represents a single scraped page stored in Neo4j.
type ScrapedContent struct {
	ContentID       string   `json:"content_id"`
	JobID           string   `json:"job_id"`
	URL             string   `json:"url"`
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	ContentPreview  string   `json:"content_preview"`
	ContentHash     string   `json:"content_hash"`
	QualityScore    float64  `json:"quality_score"`
	SimilarExisting []string `json:"similar_existing,omitempty"`
	SuggestedTags   []string `json:"suggested_tags,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Status          string   `json:"status"`
	WordCount       int      `json:"word_count"`
	IngestedNodeID  string   `json:"ingested_node_id,omitempty"`
}

// ScrapeJobResponse is the API response for a scrape job.
type ScrapeJobResponse struct {
	JobID         string           `json:"job_id"`
	Status        string           `json:"status"`
	URLs          []string         `json:"urls"`
	TargetSpaceID string           `json:"target_space_id"`
	TotalURLs     int              `json:"total_urls"`
	ProcessedURLs int              `json:"processed_urls"`
	Contents      []ScrapedContent `json:"contents,omitempty"`
	CreatedAt     string           `json:"created_at"`
	UpdatedAt     string           `json:"updated_at"`
	CompletedAt   string           `json:"completed_at,omitempty"`
	Error         string           `json:"error,omitempty"`
}

// ScrapeJobListResponse is the response for listing jobs.
type ScrapeJobListResponse struct {
	Jobs  []ScrapeJobResponse `json:"jobs"`
	Count int                 `json:"count"`
}

// ReviewRequest is the request to review scraped content.
type ReviewRequest struct {
	Decisions []ReviewDecision `json:"decisions" validate:"required,min=1"`
}

// ReviewDecision is a single review decision for a content item.
type ReviewDecision struct {
	ContentID   string   `json:"content_id" validate:"required"`
	Action      string   `json:"action" validate:"required"` // "approve", "reject", "edit"
	EditContent string   `json:"edit_content,omitempty"`
	EditTags    []string `json:"edit_tags,omitempty"`
	SpaceID     string   `json:"space_id,omitempty"` // Override target space
}

// ReviewResponse is the response after processing reviews.
type ReviewResponse struct {
	JobID    string         `json:"job_id"`
	Reviewed int            `json:"reviewed"`
	Ingested []IngestedItem `json:"ingested,omitempty"`
	Rejected int            `json:"rejected"`
	Status   string         `json:"status"`
}

// IngestedItem tracks an ingested content item.
type IngestedItem struct {
	ContentID string `json:"content_id"`
	NodeID    string `json:"node_id"`
	URL       string `json:"url"`
}

// SpaceListResponse lists available target spaces.
type SpaceListResponse struct {
	Spaces []SpaceInfo `json:"spaces"`
	Count  int         `json:"count"`
}

// SpaceInfo describes a space available for scraping.
type SpaceInfo struct {
	SpaceID   string `json:"space_id"`
	NodeCount int    `json:"node_count"`
}
