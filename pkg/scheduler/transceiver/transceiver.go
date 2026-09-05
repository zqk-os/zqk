// Package transceiver implements protocol routing for scheduler integration.
//
// TODO([REDACTED-ID]): Multiplexing router + transceiver tie into assembly pipeline (storage, metrics, events, notifications); see docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md and pkg/datacell.
package transceiver

import (
	"github.com/lanceman/zqk/pkg/logging"
)

const emptyValue = ""

// NewDefaultRouter creates a router with default adapters registered
// This function should be called after importing the adapters package
// to avoid import cycles. Use RegisterDefaultAdapters instead.
func NewDefaultRouter(logger logging.Logger) *Router {
	return NewRouter(logger)
}

// RegisterDefaultAdapters registers the default protocol adapters
// This is a separate function to avoid import cycles
// Callers should import adapters and call this with adapter instances
func RegisterDefaultAdapters(router *Router, httpAdapter, commandAdapter, eventAdapter ProtocolAdapter) {
	if httpAdapter != nil {
		_ = router.RegisterAdapter(httpAdapter) //nolint:errcheck // Initialization - duplicate registration would be a programming error
	}
	if commandAdapter != nil {
		_ = router.RegisterAdapter(commandAdapter) //nolint:errcheck // Initialization - duplicate registration would be a programming error
	}
	if eventAdapter != nil {
		_ = router.RegisterAdapter(eventAdapter) //nolint:errcheck // Initialization - duplicate registration would be a programming error
	}
}
