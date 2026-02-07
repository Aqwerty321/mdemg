package hidden

import "context"

// uiStep adapts CreateUINodes to the NodeCreator interface.
type uiStep struct{ svc *Service }

func (s *uiStep) Name() string   { return "ui" }
func (s *uiStep) Phase() int     { return 20 }
func (s *uiStep) Required() bool { return false }

func (s *uiStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	r, err := s.svc.CreateUINodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return &StepResult{
		NodesCreated: r.UINodesCreated,
		EdgesCreated: r.EdgesCreated,
		Details: map[string]any{
			"patterns_detected": r.PatternsDetected,
		},
	}, nil
}
