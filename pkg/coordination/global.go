package coordination

import (
	"sync"
	"time"
)

var (
	globalCoordinator   EventCoordinator
	globalCoordinatorMu sync.RWMutex
)

// GetCoordinator returns the global EventCoordinator instance
// This provides system-wide access to the coordination system
func GetCoordinator() EventCoordinator {
	globalCoordinatorMu.RLock()
	if c := globalCoordinator; c != nil {
		globalCoordinatorMu.RUnlock()
		return c
	}
	globalCoordinatorMu.RUnlock()

	globalCoordinatorMu.Lock()
	defer globalCoordinatorMu.Unlock()

	if globalCoordinator == nil {
		globalCoordinator = NewCoordinator(CoordinatorConfig{})
	}

	return globalCoordinator
}

// SetGlobalCoordinator sets the global EventCoordinator instance
// This should be called during system initialization with configured routers
func SetGlobalCoordinator(coordinator EventCoordinator) {
	globalCoordinatorMu.Lock()
	defer globalCoordinatorMu.Unlock()

	globalCoordinator = coordinator
}

// ResetGlobalCoordinator resets the global coordinator (useful for testing)
func ResetGlobalCoordinator() {
	globalCoordinatorMu.Lock()
	defer globalCoordinatorMu.Unlock()

	globalCoordinator = nil
}

// DrainGlobalCoordinator waits up to timeout for all in-flight router and subscriber goroutines on the global coordinator to complete.
func DrainGlobalCoordinator(timeout time.Duration) error {
	globalCoordinatorMu.RLock()
	c := globalCoordinator
	globalCoordinatorMu.RUnlock()

	if coord, ok := c.(*Coordinator); ok && coord != nil {
		return coord.Drain(timeout)
	}
	return nil
}
