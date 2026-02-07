package hidden

import "context"

// hiddenStep adapts CreateHiddenNodes to the NodeCreator interface.
type hiddenStep struct{ svc *Service }

func (s *hiddenStep) Name() string   { return "hidden" }
func (s *hiddenStep) Phase() int     { return 10 }
func (s *hiddenStep) Required() bool { return true }

func (s *hiddenStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	created, err := s.svc.CreateHiddenNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return &StepResult{NodesCreated: created}, nil
}
