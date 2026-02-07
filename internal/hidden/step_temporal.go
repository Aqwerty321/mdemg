package hidden

import "context"

// temporalStep adapts CreateTemporalNodes to the NodeCreator interface.
type temporalStep struct{ svc *Service }

func (s *temporalStep) Name() string   { return "temporal" }
func (s *temporalStep) Phase() int     { return 20 }
func (s *temporalStep) Required() bool { return false }

func (s *temporalStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	r, err := s.svc.CreateTemporalNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	created := 0
	if r.TemporalNodeCreated {
		created = 1
	}
	return &StepResult{
		NodesCreated: created,
		EdgesCreated: r.EdgesCreated,
		Details: map[string]any{
			"patterns_detected": r.PatternsDetected,
		},
	}, nil
}
