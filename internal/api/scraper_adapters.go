package api

import (
	"context"

	"mdemg/internal/conversation"
	"mdemg/internal/scraper"
)

// scraperConvAdapter adapts *conversation.Service to scraper.ConversationService.
type scraperConvAdapter struct {
	svc *conversation.Service
}

func (a *scraperConvAdapter) Observe(ctx context.Context, req scraper.ObserveParams) (*scraper.ObserveResult, error) {
	resp, err := a.svc.Observe(ctx, conversation.ObserveRequest{
		SpaceID:        req.SpaceID,
		SessionID:      "web-scraper",
		Content:        req.Content,
		ObsType:        req.ObsType,
		Tags:           req.Tags,
		Metadata:       req.Metadata,
		Pinned:         req.Pinned,
		StructuredData: req.StructuredData,
	})
	if err != nil {
		return nil, err
	}
	return &scraper.ObserveResult{NodeID: resp.NodeID}, nil
}
