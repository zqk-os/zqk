package tde

import (
	"context"
	"fmt"
	"sync"
)

// ActionHandler defines the signature for a modular execution handler
// capable of processing an approved TDE Envelope.
type ActionHandler func(ctx context.Context, env Envelope) error

var (
	registryMu sync.RWMutex
	handlers   = make(map[string]ActionHandler)
)

// RegisterAction binds a modular operation (e.g., from an add-on) to a specific Envelope Operation string.
func RegisterAction(operation string, handler ActionHandler) {
	registryMu.Lock()
	defer registryMu.Unlock()
	handlers[operation] = handler
}

// ExecuteAction dispatches the approved envelope to the corresponding registered module.
func ExecuteAction(ctx context.Context, env Envelope) error {
	registryMu.RLock()
	handler, ok := handlers[env.Operation]
	registryMu.RUnlock()

	if !ok {
		return fmt.Errorf("no registered action handler for operation: %s", env.Operation)
	}

	return handler(ctx, env)
}
