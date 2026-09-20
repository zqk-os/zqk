package specialization

import (
	"context"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Handler represents a specialized processing unit for a node.
type Handler interface {
	// Initialize prepares the handler with access to local storage and the industrial spine.
	Initialize(ctx context.Context, store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine) error

	// Start begins the handler's subscription loop.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the handler.
	Stop() error
}

// Registry manages the handlers active for the node's specialization tier.
type Registry struct {
	handlers []Handler
}

// NewRegistry creates a new handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make([]Handler, 0),
	}
}

// Add appends a handler to the registry.
func (r *Registry) Add(h Handler) {
	r.handlers = append(r.handlers, h)
}

// EngageAll initializes and starts all registered handlers.
func (r *Registry) EngageAll(ctx context.Context, store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine) error {
	for _, h := range r.handlers {
		if err := h.Initialize(ctx, store, spine); err != nil {
			return err
		}
		if err := h.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

// DisengageAll stops all registered handlers.
func (r *Registry) DisengageAll() {
	for _, h := range r.handlers {
		_ = h.Stop()
	}
}
