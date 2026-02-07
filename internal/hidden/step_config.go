package hidden

import "context"

// configStep adapts CreateConfigNodes to the NodeCreator interface.
type configStep struct{ svc *Service }

func (s *configStep) Name() string   { return "config" }
func (s *configStep) Phase() int     { return 20 }
func (s *configStep) Required() bool { return false }

func (s *configStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	r, err := s.svc.CreateConfigNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	created := 0
	if r.ConfigNodeCreated {
		created = 1
	}
	return &StepResult{
		NodesCreated: created,
		EdgesCreated: r.EdgesCreated,
	}, nil
}
