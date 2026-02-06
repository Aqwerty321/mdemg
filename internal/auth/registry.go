package auth

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// AuthenticatorFactory creates an Authenticator from config.
type AuthenticatorFactory func(cfg AuthMethodConfig) (Authenticator, error)

// Registry manages available authentication methods.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]AuthenticatorFactory
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]AuthenticatorFactory),
	}
}

// Register adds an auth method to the registry.
func (r *Registry) Register(name string, factory AuthenticatorFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("auth method %q already registered", name)
	}
	r.factories[name] = factory
	return nil
}

// MustRegister panics on error (for init-time registration).
func (r *Registry) MustRegister(name string, factory AuthenticatorFactory) {
	if err := r.Register(name, factory); err != nil {
		panic(err)
	}
}

// Build creates an Authenticator by name.
func (r *Registry) Build(name string, cfg AuthMethodConfig) (Authenticator, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown auth method: %s", name)
	}
	return factory(cfg)
}

// Has returns true if the method is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[name]
	return ok
}

// List returns all registered method names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// defaultRegistry is the global registry with built-in methods.
var defaultRegistry *Registry

func init() {
	defaultRegistry = NewRegistry()
	defaultRegistry.MustRegister("none", newNoneAuthenticator)
	defaultRegistry.MustRegister("apikey", newAPIKeyAuthenticator)
	defaultRegistry.MustRegister("jwt", newJWTAuthenticator)
}

// DefaultRegistry returns the global registry with built-in methods.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// noneAuthenticator implements Authenticator for pass-through mode.
type noneAuthenticator struct{}

func newNoneAuthenticator(_ AuthMethodConfig) (Authenticator, error) {
	return &noneAuthenticator{}, nil
}

func (a *noneAuthenticator) Name() string { return "none" }

func (a *noneAuthenticator) Authenticate(_ *http.Request) (*Principal, error) {
	// Pass-through: return anonymous principal
	return &Principal{
		ID:   "anonymous",
		Type: ModeNone,
	}, nil
}
