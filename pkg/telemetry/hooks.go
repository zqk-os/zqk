package telemetry

import (
	"context"
	"sync"
	"time"
)

// Hook defines an interface for telemetry hooks that can be injected into the system.
type Hook interface {
	// OnSpanStart is called when a new observable span begins.
	OnSpanStart(ctx context.Context, operation string, tags map[string]string) context.Context

	// OnSpanEnd is called when an observable span completes.
	OnSpanEnd(ctx context.Context, err error)

	// RecordMetric records a raw metric event.
	RecordMetric(ctx context.Context, name string, value float64, tags map[string]string)
}

// Manager manages a collection of telemetry hooks.
type Manager struct {
	hooks []Hook
	mu    sync.RWMutex
}

var (
	globalManager *Manager
	once          sync.Once
)

// GlobalManager returns the singleton telemetry manager.
func GlobalManager() *Manager {
	once.Do(func() {
		globalManager = &Manager{
			hooks: make([]Hook, 0),
		}
	})
	return globalManager
}

// RegisterHook adds a new hook to the manager.
func (m *Manager) RegisterHook(hook Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// StartSpan starts a span across all registered hooks and returns a decorated context
// and a completion function to be deferred.
func (m *Manager) StartSpan(ctx context.Context, operation string, tags map[string]string) (context.Context, func(error)) {
	m.mu.RLock()
	hooks := make([]Hook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.RUnlock()

	currentCtx := ctx
	if currentCtx == nil {
		currentCtx = context.Background()
	}

	for _, hook := range hooks {
		currentCtx = hook.OnSpanStart(currentCtx, operation, tags)
	}

	start := time.Now()

	return currentCtx, func(err error) {
		duration := time.Since(start)

		for _, hook := range hooks {
			hook.OnSpanEnd(currentCtx, err)
		}

		m.RecordMetric(currentCtx, operation+".duration_ms", float64(duration.Milliseconds()), tags)
	}
}

// RecordMetric records a metric across all registered hooks.
func (m *Manager) RecordMetric(ctx context.Context, name string, value float64, tags map[string]string) {
	m.mu.RLock()
	hooks := make([]Hook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		hook.RecordMetric(ctx, name, value, tags)
	}
}

// NewManager creates a new telemetry manager.
func NewManager() *Manager {
	return &Manager{
		hooks: make([]Hook, 0),
	}
}
