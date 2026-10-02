package service

import (
	"context"
	"fmt"
	"runtime"
	"sync"
)

// Manager coordinates host service lifecycle verbs using a resolved or pluggable ServiceAdapter.
type Manager struct {
	mu      sync.RWMutex
	adapter ServiceAdapter
}

// ManagerOption allows custom configuration when creating a Manager.
type ManagerOption func(*Manager)

// WithAdapter configures a specific ServiceAdapter on the Manager.
func WithAdapter(adapter ServiceAdapter) ManagerOption {
	return func(m *Manager) {
		m.adapter = adapter
	}
}

// AutoDetectAdapter selects the idiomatic ServiceAdapter for the current operating system.
func AutoDetectAdapter() ServiceAdapter {
	switch runtime.GOOS {
	case "darwin":
		la := NewLaunchdAdapter()
		if la.IsAvailable() {
			return la
		}
	case "linux":
		sa := NewSystemdAdapter(true)
		if sa.IsAvailable() {
			return sa
		}
	}
	// Fallback to process supervisor in non-systemd Linux, containers, or test environments
	return NewSupervisorAdapter()
}

// NewManager initializes a Manager instance. If no adapter is provided via WithAdapter,
// AutoDetectAdapter is used to select the host-appropriate adapter.
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{}
	for _, opt := range opts {
		opt(m)
	}
	if m.adapter == nil {
		m.adapter = AutoDetectAdapter()
	}
	return m
}

// Adapter returns the underlying ServiceAdapter.
func (m *Manager) Adapter() ServiceAdapter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.adapter
}

// Install registers a service specification with the underlying adapter.
func (m *Manager) Install(ctx context.Context, spec ServiceSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := spec.Validate(); err != nil {
		return err
	}
	return m.adapter.Install(ctx, spec)
}

func (m *Manager) withServiceOp(id string, op func(id string) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" {
		return fmt.Errorf("%w: missing service id", ErrInvalidSpec)
	}
	return op(id)
}

// Uninstall unregisters a service specification.
func (m *Manager) Uninstall(ctx context.Context, id string) error {
	return m.withServiceOp(id, func(id string) error {
		return m.adapter.Uninstall(ctx, id)
	})
}

// Start instructs the underlying supervisor to start the service.
func (m *Manager) Start(ctx context.Context, id string) error {
	return m.withServiceOp(id, func(id string) error {
		return m.adapter.Start(ctx, id)
	})
}

// Stop instructs the underlying supervisor to stop the service.
func (m *Manager) Stop(ctx context.Context, id string) error {
	return m.withServiceOp(id, func(id string) error {
		return m.adapter.Stop(ctx, id)
	})
}

// Restart restarts the specified service.
func (m *Manager) Restart(ctx context.Context, id string) error {
	return m.withServiceOp(id, func(id string) error {
		return m.adapter.Restart(ctx, id)
	})
}

// Status queries the live status of the service.
func (m *Manager) Status(ctx context.Context, id string) (ServiceStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id == "" {
		return ServiceStatus{}, fmt.Errorf("%w: missing service id", ErrInvalidSpec)
	}
	return m.adapter.Status(ctx, id)
}

// CleanupLegacy removes superseded units matching the specified identifiers.
func (m *Manager) CleanupLegacy(ctx context.Context, legacyIDs []string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(legacyIDs) == 0 {
		return nil, nil
	}
	return m.adapter.CleanupLegacy(ctx, legacyIDs)
}
