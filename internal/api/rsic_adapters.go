package api

import (
	"context"

	"mdemg/internal/ape"
	"mdemg/internal/conversation"
	"mdemg/internal/hidden"
	"mdemg/internal/learning"
)

// rsicLearningAdapter adapts *learning.Service to ape.LearningStatsProvider.
type rsicLearningAdapter struct {
	svc *learning.Service
}

func (a *rsicLearningAdapter) GetLearningEdgeStats(ctx context.Context, spaceID string) (map[string]any, error) {
	return a.svc.GetLearningEdgeStats(ctx, spaceID)
}

func (a *rsicLearningAdapter) PruneDecayedEdges(ctx context.Context, spaceID string) (int64, error) {
	return a.svc.PruneDecayedEdges(ctx, spaceID)
}

func (a *rsicLearningAdapter) PruneExcessEdgesPerNode(ctx context.Context, spaceID string) (int64, error) {
	return a.svc.PruneExcessEdgesPerNode(ctx, spaceID)
}

// rsicConvAdapter adapts *conversation.ContextCooler to ape.ConversationStatsProvider.
type rsicConvAdapter struct {
	cooler *conversation.ContextCooler
}

func (a *rsicConvAdapter) GetVolatileStats(ctx context.Context, spaceID string) (ape.VolatileStatsResult, error) {
	if a == nil || a.cooler == nil {
		return ape.VolatileStatsResult{}, nil
	}
	stats, err := a.cooler.GetVolatileStats(ctx, spaceID)
	if err != nil {
		return ape.VolatileStatsResult{}, err
	}
	return ape.VolatileStatsResult{
		VolatileCount:        stats.VolatileCount,
		PermanentCount:       stats.PermanentCount,
		AvgVolatileStability: stats.AvgVolatileStability,
	}, nil
}

// rsicHiddenAdapter adapts *hidden.Service to ape.HiddenLayerProvider.
type rsicHiddenAdapter struct {
	svc *hidden.Service
}

func (a *rsicHiddenAdapter) RunFullConversationConsolidation(ctx context.Context, spaceID string) (any, error) {
	return a.svc.RunFullConversationConsolidation(ctx, spaceID)
}
