package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// MockQueueShutdownHandler is a test double for QueueShutdownHandler used by queue shutdown tests.
type MockQueueShutdownHandler struct {
	Name           string
	critical       bool
	PendingCount   atomic.Int64
	DrainDelay     time.Duration
	DrainError     error
	InitiateError  error
	InitiateCalled atomic.Int32
	DrainCalled    atomic.Int32
	mu             sync.Mutex
	Operations     []string
}

// NewMockQueueShutdownHandler creates a mock queue handler for tests.
func NewMockQueueShutdownHandler(name string, isCritical bool) *MockQueueShutdownHandler {
	return &MockQueueShutdownHandler{
		Name:       name,
		critical:   isCritical,
		Operations: make([]string, 0),
	}
}

// SetPendingCount sets the simulated pending operation count.
func (m *MockQueueShutdownHandler) SetPendingCount(count int64) {
	m.PendingCount.Store(count)
}

func (m *MockQueueShutdownHandler) InitiateShutdown() error {
	m.InitiateCalled.Add(1)
	m.mu.Lock()
	m.Operations = append(m.Operations, "initiate")
	m.mu.Unlock()
	return m.InitiateError
}

func (m *MockQueueShutdownHandler) Drain(ctx context.Context) error {
	m.DrainCalled.Add(1)
	m.mu.Lock()
	m.Operations = append(m.Operations, "drain_start")
	m.mu.Unlock()

	if m.DrainDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.DrainDelay):
		}
	}

	m.PendingCount.Store(0)

	m.mu.Lock()
	m.Operations = append(m.Operations, "drain_complete")
	m.mu.Unlock()

	return m.DrainError
}

func (m *MockQueueShutdownHandler) IsDrained() bool {
	return m.PendingCount.Load() == 0
}

func (m *MockQueueShutdownHandler) GetPendingCount() int64 {
	return m.PendingCount.Load()
}

func (m *MockQueueShutdownHandler) GetName() string {
	return m.Name
}

func (m *MockQueueShutdownHandler) IsCritical() bool {
	return m.critical
}

// GetOperations returns a copy of recorded operation names (for debugging tests).
func (m *MockQueueShutdownHandler) GetOperations() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.Operations))
	copy(result, m.Operations)
	return result
}
