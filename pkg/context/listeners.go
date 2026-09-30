package context

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
)

// ContextState represents the state of a context object
type ContextState string

const (
	// StatePending indicates the context is waiting to be processed
	StatePending ContextState = "pending"
	// StateProcessing indicates the context is currently being processed
	StateProcessing ContextState = "processing"
	// StateCompleted indicates the context processing completed successfully
	StateCompleted ContextState = "completed"
	// StateFailed indicates the context processing failed
	StateFailed ContextState = "failed"
	// StateCancelled indicates the context processing was cancelled
	StateCancelled ContextState = "cancelled"
)

// ContextListener is a callback function that gets invoked when a context reaches a certain state
// It receives the context object and can return a result or error
// If the listener returns an error, processing stops and the context moves to StateFailed
// If the listener returns a result, it's passed to the next stage in the pipeline
type ContextListener func(ctx context.Context, contextObj any) (any, error)

// ListenerConfig configures how a listener should behave
type ListenerConfig struct {
	// Name identifies the listener (for logging/debugging)
	Name string

	// Async indicates whether the listener should run in a goroutine (non-blocking)
	Async bool

	// Priority determines the order listeners are executed (lower = higher priority)
	Priority int

	// Required indicates whether this listener must succeed for processing to continue
	Required bool

	// Timeout is the maximum time the listener can run (0 = no timeout)
	Timeout time.Duration
}

// ContextListenerRegistry manages listeners for context objects.
// Narrow scope: context pipeline pending→processing→completed only — not a general PubSub.
// Prefer pkg/coordination for in-process fan-out and pkg/contextevents for durable JSONL.
// See docs/architecture/CEF_EVENT_PATH_AND_GLOBALS.md.
type ContextListenerRegistry struct {
	mu        sync.RWMutex
	listeners map[ContextState][]registeredListener
}

type registeredListener struct {
	listener ContextListener
	config   ListenerConfig
}

var globalRegistry = &ContextListenerRegistry{
	listeners: make(map[ContextState][]registeredListener),
}

// RegisterListener registers a listener for a specific context state
// Listeners are executed in priority order (lower priority = executed first)
func RegisterListener(state ContextState, listener ContextListener, config ListenerConfig) {
	_ = concurrency.WithLockCtx(
		&globalRegistry.mu,
		NewSystemContext(),
		"context_listener_register",
		func() error {
			if globalRegistry.listeners[state] == nil {
				globalRegistry.listeners[state] = make([]registeredListener, 0)
			}

			globalRegistry.listeners[state] = append(globalRegistry.listeners[state], registeredListener{
				listener: listener,
				config:   config,
			})

			// Sort by priority (lower priority = higher priority)
			// Simple insertion sort for small lists
			listeners := globalRegistry.listeners[state]
			for i := len(listeners) - 1; i > 0; i-- {
				if listeners[i].config.Priority < listeners[i-1].config.Priority {
					listeners[i], listeners[i-1] = listeners[i-1], listeners[i]
				} else {
					break
				}
			}
			return nil
		},
	)
}

// GetListeners returns all listeners registered for a state
func GetListeners(state ContextState) []registeredListener {
	var result []registeredListener
	_ = concurrency.WithRLockCtx(
		&globalRegistry.mu,
		NewSystemContext(),
		"context_listener_get",
		func() error {
			listeners := globalRegistry.listeners[state]
			if listeners == nil {
				result = []registeredListener{}
				return nil
			}

			// Return a copy to prevent external modification
			result = make([]registeredListener, len(listeners))
			copy(result, listeners)
			return nil
		},
	)
	return result
}

// ClearListeners removes all listeners for a state (useful for testing)
func ClearListeners(state ContextState) {
	_ = concurrency.WithLockCtx(
		&globalRegistry.mu,
		NewSystemContext(),
		"context_listener_clear",
		func() error {
			delete(globalRegistry.listeners, state)
			return nil
		},
	)
}

// ClearAllListeners removes all registered listeners (useful for testing)
func ClearAllListeners() {
	_ = concurrency.WithLockCtx(
		&globalRegistry.mu,
		NewSystemContext(),
		"context_listener_clear_all",
		func() error {
			globalRegistry.listeners = make(map[ContextState][]registeredListener)
			return nil
		},
	)
}
