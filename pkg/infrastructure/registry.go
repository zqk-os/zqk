package infrastructure

import (
	"context"
	"fmt"
	"sync"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// Registry manages the lifecycle and discovery of infrastructure drivers.
type Registry struct {
	mu      sync.RWMutex
	spines  map[string]SpinalSpine
	drivers map[string]DriverFactory
}

// DriverFactory creates a SpinalSpine from an endpoint and credentials.
type DriverFactory func(ctx context.Context, endpoint string, creds string) (SpinalSpine, error)

var (
	globalRegistry *Registry
	once           sync.Once
)

// GetRegistry returns the singleton infrastructure registry.
func GetRegistry() *Registry {
	once.Do(func() {
		globalRegistry = &Registry{
			spines:  make(map[string]SpinalSpine),
			drivers: make(map[string]DriverFactory),
		}
	})
	return globalRegistry
}

// RegisterDriver registers a new driver factory (e.g., "kafka", "kinesis").
func (r *Registry) RegisterDriver(utilityType string, factory DriverFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers[utilityType] = factory
}

// Engage activates a spine for a specific infrastructure adapter.
func (r *Registry) Engage(ctx context.Context, id string, utilityType string, endpoint string, creds string) (SpinalSpine, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.spines[id]; ok {
		return s, nil
	}

	factory, ok := r.drivers[utilityType]
	if !ok {
		return nil, errfmt.Errorf("no infrastructure driver registered for utility_type: %s", utilityType)
	}

	spine, err := factory(ctx, endpoint, creds)
	if err != nil {
		return nil, errfmt.Newf("failed to engage %s spine", utilityType).Wrap(err)
	}

	r.spines[id] = spine
	return spine, nil
}

// GetSpine retrieves an active spine by adapter ID.
func (r *Registry) GetSpine(id string) (SpinalSpine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	spine, ok := r.spines[id]
	if !ok {
		return nil, fmt.Errorf("spine not engaged for adapter: %s", id)
	}
	return spine, nil
}
