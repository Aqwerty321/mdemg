package hidden

import "context"

// comparisonStep adapts CreateComparisonNodes to the NodeCreator interface.
type comparisonStep struct{ svc *Service }

func (s *comparisonStep) Name() string   { return "comparison" }
func (s *comparisonStep) Phase() int     { return 20 }
func (s *comparisonStep) Required() bool { return false }

func (s *comparisonStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	r, err := s.svc.CreateComparisonNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return &StepResult{
		NodesCreated: r.ComparisonNodesCreated,
		EdgesCreated: r.EdgesCreated,
		Details: map[string]any{
			"modules_compared": r.ModulesCompared,
		},
	}, nil
}
