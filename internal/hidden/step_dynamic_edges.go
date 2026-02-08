package hidden

import "context"

// dynamicEdgesStep runs dynamic edge creation as a pipeline step.
// Phase 25 places it after enrichment (20) but before L5 post-processing (30).
type dynamicEdgesStep struct {
	svc *Service
}

func (s *dynamicEdgesStep) Name() string   { return "dynamic_edges" }
func (s *dynamicEdgesStep) Phase() int     { return 25 }
func (s *dynamicEdgesStep) Required() bool { return false }

func (s *dynamicEdgesStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	if !s.svc.cfg.DynamicEdgesEnabled {
		return &StepResult{EdgesCreated: 0}, nil
	}

	created, err := s.svc.CreateDynamicEdges(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		EdgesCreated: created,
	}, nil
}
