package service

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockAdapter is an in-memory thread-safe implementation of ServiceAdapter for unit testing.
type MockAdapter struct {
	mu            sync.RWMutex
	name          string
	available     bool
	installed     map[string]ServiceSpec
	running       map[string]ServiceStatus
	legacyCleaned []string

	// Error injectors for testing negative cases
	InstallErr   error
	UninstallErr error
	StartErr     error
	StopErr      error
	RestartErr   error
	StatusErr    error
	CleanupErr   error
}

// NewMockAdapter constructs a initialized MockAdapter.
func NewMockAdapter(name string, available bool) *MockAdapter {
	return &MockAdapter{
		name:      name,
		available: available,
		installed: make(map[string]ServiceSpec),
		running:   make(map[string]ServiceStatus),
	}
}

func (m *MockAdapter) Name() string {
	return m.name
}

func (m *MockAdapter) IsAvailable() bool {
	return m.available
}

func (m *MockAdapter) Install(ctx context.Context, spec ServiceSpec) error {
	if m.InstallErr != nil {
		return m.InstallErr
	}
	if err := spec.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.installed[spec.ID]; exists {
		return fmt.Errorf("%w: %s", ErrServiceAlreadyExists, spec.ID)
	}

	m.installed[spec.ID] = spec
	if spec.RunAtLoad {
		m.running[spec.ID] = ServiceStatus{
			ID:     spec.ID,
			PID:    12345,
			State:  StateRunning,
			Uptime: time.Second,
		}
	}
	return nil
}

func (m *MockAdapter) Uninstall(ctx context.Context, id string) error {
	if m.UninstallErr != nil {
		return m.UninstallErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.installed[id]; !exists {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}

	delete(m.running, id)
	delete(m.installed, id)
	return nil
}

func (m *MockAdapter) Start(ctx context.Context, id string) error {
	if m.StartErr != nil {
		return m.StartErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.installed[id]; !exists {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}
	if st, ok := m.running[id]; ok && st.State == StateRunning {
		return fmt.Errorf("%w: %s", ErrServiceAlreadyActive, id)
	}

	m.running[id] = ServiceStatus{
		ID:     id,
		PID:    20000 + len(m.running),
		State:  StateRunning,
		Uptime: time.Millisecond * 10,
	}
	return nil
}

func (m *MockAdapter) Stop(ctx context.Context, id string) error {
	if m.StopErr != nil {
		return m.StopErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.installed[id]; !exists {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}

	if st, ok := m.running[id]; ok && st.State == StateRunning {
		delete(m.running, id)
		return nil
	}
	return fmt.Errorf("%w: %s", ErrServiceNotRunning, id)
}

func (m *MockAdapter) Restart(ctx context.Context, id string) error {
	if m.RestartErr != nil {
		return m.RestartErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.installed[id]; !exists {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}

	delete(m.running, id)
	m.running[id] = ServiceStatus{
		ID:     id,
		PID:    30000 + len(m.running),
		State:  StateRunning,
		Uptime: time.Millisecond * 5,
	}
	return nil
}

func (m *MockAdapter) Status(ctx context.Context, id string) (ServiceStatus, error) {
	if m.StatusErr != nil {
		return ServiceStatus{}, m.StatusErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.installed[id]; !exists {
		return ServiceStatus{
			ID:    id,
			State: StateStopped,
		}, nil
	}

	if st, ok := m.running[id]; ok {
		return st, nil
	}

	return ServiceStatus{
		ID:    id,
		State: StateStopped,
	}, nil
}

func (m *MockAdapter) CleanupLegacy(ctx context.Context, legacyIDs []string) ([]string, error) {
	if m.CleanupErr != nil {
		return nil, m.CleanupErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var cleaned []string
	for _, id := range legacyIDs {
		if _, exists := m.installed[id]; exists {
			delete(m.running, id)
			delete(m.installed, id)
			cleaned = append(cleaned, id)
		}
	}
	m.legacyCleaned = append(m.legacyCleaned, cleaned...)
	return cleaned, nil
}

// GetInstalledCount returns the number of currently installed services.
func (m *MockAdapter) GetInstalledCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.installed)
}

// IsRunning reports whether service id is running in mock state.
func (m *MockAdapter) IsRunning(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st, ok := m.running[id]
	return ok && st.State == StateRunning
}
