// Package loader provides an abstracted component/service loader pattern:
// shared state as atomics, callbacks on state change, and configurable timeouts
// (default config file + profile/thematic overrides). See docs/architecture/README.md.
package loader

import (
	"context"
	"time"
)

// LoadState represents the lifecycle of a component load.
type LoadState int

const (
	LoadStateUnloaded LoadState = iota
	LoadStateLoading
	LoadStateLoaded
	LoadStateError
)

// String returns a short label for the state.
func (s LoadState) String() string {
	switch s {
	case LoadStateUnloaded:
		return "unloaded"
	case LoadStateLoading:
		return "loading"
	case LoadStateLoaded:
		return "loaded"
	case LoadStateError:
		return "error"
	default:
		return "unknown"
	}
}

// LoaderTimeoutConfig is per-loader timeout configuration.
// Used for wait-for-completion and for the actual load operation.
type LoaderTimeoutConfig struct {
	// WaitForCompletion is the max time to wait for a concurrent load to complete (e.g. 5s).
	WaitForCompletion time.Duration
	// PublishChannel is the max time to wait for the completion channel to appear (e.g. 1s).
	PublishChannel time.Duration
	// LoadOperation is the timeout for the actual I/O load (optional; 0 = use WaitForCompletion or no limit).
	LoadOperation time.Duration
}

// StateChangeCallback is invoked when loader state transitions.
// Implementations can be chainable or sequential.
type StateChangeCallback interface {
	OnLoading()
	OnLoaded(value any)
	OnError(err error)
	OnTimeout()
}

// NoOpStateChangeCallback is a no-op implementation for tests or when callbacks are optional.
type NoOpStateChangeCallback struct{}

func (NoOpStateChangeCallback) OnLoading()    {}
func (NoOpStateChangeCallback) OnLoaded(any)  {}
func (NoOpStateChangeCallback) OnError(error) {}
func (NoOpStateChangeCallback) OnTimeout()    {}

// ComponentLoader is the interface implementers satisfy.
// The implementer owns shared state (atomics), runs the load (or delegates to a Runner),
// and invokes callbacks on state change. Timeouts come from LoaderTimeoutConfig.
type ComponentLoader interface {
	// Load ensures the component is loaded, then returns nil or an error (including timeout).
	// Callers may run Load concurrently; one goroutine performs the load, others wait with timeout.
	Load(ctx context.Context) error
	// TimeoutConfig returns the timeout configuration for this loader (from default + overrides).
	TimeoutConfig() LoaderTimeoutConfig
}

// LoadFn is a function that performs the actual load. Used by a Runner to execute load with timeout.
type LoadFn func(ctx context.Context) error
