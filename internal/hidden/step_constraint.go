package hidden

import "context"

// constraintStep adapts CreateConstraintNodes to the NodeCreator interface.
type constraintStep struct{ svc *Service }

func (s *constraintStep) Name() string   { return "constraint" }
func (s *constraintStep) Phase() int     { return 20 }
func (s *constraintStep) Required() bool { return false }

func (s *constraintStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	r, err := s.svc.CreateConstraintNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return &StepResult{
		NodesCreated: r.Created,
		NodesUpdated: r.Updated,
		EdgesCreated: r.Linked,
	}, nil
}
