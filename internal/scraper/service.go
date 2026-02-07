package scraper

import (
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/embeddings"
	"mdemg/internal/plugins"
)

// Config holds scraper-specific configuration.
type Config struct {
	Enabled             bool
	DefaultSpaceID      string
	MaxConcurrentJobs   int
	DefaultDelayMs      int
	DefaultTimeoutMs    int
	CacheTTLSeconds     int
	RespectRobotsTxt    bool
	MaxContentLengthKB  int
}

// Service orchestrates web scraping jobs.
type Service struct {
	cfg       Config
	driver    neo4j.DriverWithContext
	embedder  embeddings.Embedder
	pluginMgr *plugins.Manager
	store     *Store
	orch      *Orchestrator
	reviewer  *Reviewer
}

// NewService creates a new scraper service.
func NewService(cfg Config, driver neo4j.DriverWithContext, embedder embeddings.Embedder, pluginMgr *plugins.Manager) *Service {
	store := NewStore(driver)
	return &Service{
		cfg:       cfg,
		driver:    driver,
		embedder:  embedder,
		pluginMgr: pluginMgr,
		store:     store,
	}
}

// SetConversationService wires the conversation adapter for ingestion.
func (s *Service) SetConversationService(convSvc ConversationService) {
	dedup := NewDedupChecker(s.driver, s.embedder, 0.85)
	s.orch = NewOrchestrator(s.store, s.pluginMgr, s.embedder, dedup, s.cfg)
	s.reviewer = NewReviewer(s.store, convSvc)
}

// Store returns the underlying Neo4j store.
func (s *Service) GetStore() *Store {
	return s.store
}

// GetOrchestrator returns the job orchestrator.
func (s *Service) GetOrchestrator() *Orchestrator {
	return s.orch
}

// GetReviewer returns the review handler.
func (s *Service) GetReviewer() *Reviewer {
	return s.reviewer
}

// GetConfig returns the scraper config.
func (s *Service) GetConfig() Config {
	return s.cfg
}
