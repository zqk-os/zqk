package transceiver

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/adapters"
)

// NewRouterWithDefaults creates a router with default adapters already registered
// This is a convenience function that avoids import cycles
func NewRouterWithDefaults(logger logging.Logger) *Router {
	router := NewRouter(logger)

	// Register default adapters
	_ = router.RegisterAdapter(adapters.NewHTTPAdapter(logger))    //nolint:errcheck // Initialization - duplicate registration would be a programming error
	_ = router.RegisterAdapter(adapters.NewCommandAdapter(logger)) //nolint:errcheck // Initialization - duplicate registration would be a programming error
	_ = router.RegisterAdapter(adapters.NewEventAdapter(logger))   //nolint:errcheck // Initialization - duplicate registration would be a programming error

	return router
}
