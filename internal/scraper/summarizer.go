package scraper

// Summarizer provides optional LLM-based summaries for scraped content.
// This is a placeholder for Phase 51.7 — reuses existing LLM config.
type Summarizer struct {
	enabled bool
}

// NewSummarizer creates a new summarizer (disabled by default).
func NewSummarizer() *Summarizer {
	return &Summarizer{enabled: false}
}
