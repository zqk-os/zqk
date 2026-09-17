package ambient

import (
	"context"
	"errors"
)

var (
	// ErrNotEnabled is returned when an ambient operation is attempted but the engine is not running.
	ErrNotEnabled = errors.New("ambient engine is not enabled")
)

// Service defines the contract for the Ambience Engine.
// The core scheduler uses this interface to interact with ambient context
// without needing to know the implementation details of the filesystem watcher.
type Service interface {
	// Start initializes the background filesystem watchers and debouncers.
	// It should block until the context is canceled.
	Start(ctx context.Context) error

	// Status returns the current health and operational status of the watcher.
	Status() string
}
