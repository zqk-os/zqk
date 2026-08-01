package concurrency

import (
	"time"
)

// OperationCallback provides standard callback interface for all async operations
// This ensures consistent coordination and observability across all concurrent operations
//
// NOTE: The interface is defined here (pkg/concurrency) to avoid import cycles.
// Implementations that require coordinator/storage should be in pkg/coordination.
type OperationCallback interface {
	// OnStart is called when operation starts
	OnStart(operationID string, metadata map[string]any)

	// OnProgress is called for progress updates
	OnProgress(operationID string, progress int, total int, message string)

	// OnComplete is called when operation completes successfully
	OnComplete(operationID string, result any, duration time.Duration)

	// OnError is called when operation fails
	OnError(operationID string, err error)

	// OnCancel is called when operation is cancelled
	OnCancel(operationID string, reason string)
}

// NoOpOperationCallback is a no-op implementation for testing or when coordinator is unavailable
type NoOpOperationCallback struct{}

func (n *NoOpOperationCallback) OnStart(operationID string, metadata map[string]any) {}
func (n *NoOpOperationCallback) OnProgress(operationID string, progress int, total int, message string) {
}
func (n *NoOpOperationCallback) OnComplete(operationID string, result any, duration time.Duration) {
}
func (n *NoOpOperationCallback) OnError(operationID string, err error)      {}
func (n *NoOpOperationCallback) OnCancel(operationID string, reason string) {}
