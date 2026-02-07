package hidden

import "context"

// concernStep adapts CreateConcernNodes to the NodeCreator interface.
type concernStep struct{ svc *Service }

func (s *concernStep) Name() string   { return "concern" }
func (s *concernStep) Phase() int     { return 20 }
func (s *concernStep) Required() bool { return false }

func (s *concernStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	r, err := s.svc.CreateConcernNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return &StepResult{
		NodesCreated: r.ConcernNodesCreated,
		EdgesCreated: r.EdgesCreated,
		Details: map[string]any{
			"concerns": r.Concerns,
		},
	}, nil
}
