package mcp

import (
	"sync"
	"sync/atomic"
)

// ToolHandlerRegistry manages named ToolHandler registrations for spec-driven tool generation.
type ToolHandlerRegistry struct {
	handlers                map[string]ToolHandler
	handlersRegisteredTotal atomic.Int64
	lookupsTotal            atomic.Int64
	hitsTotal               atomic.Int64
	mu                      sync.RWMutex
}

// GlobalToolHandlerRegistry is the default singleton registry.
var GlobalToolHandlerRegistry = NewToolHandlerRegistry()

// GetToolHandlerRegistryStats returns lifetime counters for handlers registered, total lookups, and lookup hits.
func (r *ToolHandlerRegistry) GetToolHandlerRegistryStats() (registered, lookups, hits int64) {
	if r == nil {
		return 0, 0, 0
	}
	return r.handlersRegisteredTotal.Load(), r.lookupsTotal.Load(), r.hitsTotal.Load()
}

// NewToolHandlerRegistry creates a new ToolHandlerRegistry instance.
func NewToolHandlerRegistry() *ToolHandlerRegistry {
	return &ToolHandlerRegistry{
		handlers: make(map[string]ToolHandler),
	}
}

// Register registers a named ToolHandler.
func (r *ToolHandlerRegistry) Register(name string, handler ToolHandler) {
	r.mu.Lock()
	r.handlers[name] = handler
	r.mu.Unlock()
	r.handlersRegisteredTotal.Add(1)
}

// Get returns the ToolHandler associated with name, if present.
func (r *ToolHandlerRegistry) Get(name string) (ToolHandler, bool) {
	r.mu.RLock()
	h, ok := r.handlers[name]
	r.mu.RUnlock()
	r.lookupsTotal.Add(1)
	if ok {
		r.hitsTotal.Add(1)
	}
	return h, ok
}
