package ambient

import "context"

// NoopService is the zero-cost fallback implementation of the ambient Service.
// It is injected into the core scheduler when the ZQK_ENABLE_AMBIENT_WATCHER
// feature gate is disabled, ensuring no CPU or memory penalty is incurred.
type NoopService struct{}

// NewNoopService creates a new zero-cost ambient service.
func NewNoopService() *NoopService {
	return &NoopService{}
}

// Start does nothing and returns immediately.
func (s *NoopService) Start(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Status indicates that the service is disabled.
func (s *NoopService) Status() string {
	return "disabled (noop)"
}
