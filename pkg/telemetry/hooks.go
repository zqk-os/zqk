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

// SpanRecord captures a finished span's metadata and duration.
type SpanRecord struct {
	Operation string            `json:"operation"`
	Tags      map[string]string `json:"tags,omitempty"`
	StartTime time.Time         `json:"start_time"`
	Duration  time.Duration     `json:"duration"`
	Err       string            `json:"error,omitempty"`
}

// MetricRecord captures a point-in-time metric measurement.
type MetricRecord struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Tags      map[string]string `json:"tags,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

type spanKey struct{}

type spanContextEntry struct {
	operation string
	tags      map[string]string
	startTime time.Time
}

// InMemoryHook provides an in-memory ring-buffer hook sink for spans and metrics.
type InMemoryHook struct {
	mu       sync.RWMutex
	capacity int
	spans    []SpanRecord
	metrics  []MetricRecord
}

// NewInMemoryHook constructs a bounded in-memory telemetry hook.
func NewInMemoryHook(capacity int) *InMemoryHook {
	if capacity <= 0 {
		capacity = 1024
	}
	return &InMemoryHook{
		capacity: capacity,
		spans:    make([]SpanRecord, 0, capacity),
		metrics:  make([]MetricRecord, 0, capacity),
	}
}

// OnSpanStart tracks span start time and contextual tags.
func (h *InMemoryHook) OnSpanStart(ctx context.Context, operation string, tags map[string]string) context.Context {
	entry := spanContextEntry{
		operation: operation,
		tags:      tags,
		startTime: time.Now(),
	}
	return context.WithValue(ctx, spanKey{}, entry)
}

// OnSpanEnd records the completed span into the ring-buffer.
func (h *InMemoryHook) OnSpanEnd(ctx context.Context, err error) {
	val := ctx.Value(spanKey{})
	if val == nil {
		return
	}
	entry, ok := val.(spanContextEntry)
	if !ok {
		return
	}
	duration := time.Since(entry.startTime)
	record := SpanRecord{
		Operation: entry.operation,
		Tags:      entry.tags,
		StartTime: entry.startTime,
		Duration:  duration,
	}
	if err != nil {
		record.Err = err.Error()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.spans) >= h.capacity {
		h.spans = h.spans[1:]
	}
	h.spans = append(h.spans, record)
}

// RecordMetric appends a metric record to the in-memory ring-buffer.
func (h *InMemoryHook) RecordMetric(ctx context.Context, name string, value float64, tags map[string]string) {
	record := MetricRecord{
		Name:      name,
		Value:     value,
		Tags:      tags,
		Timestamp: time.Now(),
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.metrics) >= h.capacity {
		h.metrics = h.metrics[1:]
	}
	h.metrics = append(h.metrics, record)
}

// GetRecentSpans returns a snapshot of recorded spans.
func (h *InMemoryHook) GetRecentSpans() []SpanRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]SpanRecord, len(h.spans))
	copy(out, h.spans)
	return out
}

// GetRecentMetrics returns a snapshot of recorded metrics.
func (h *InMemoryHook) GetRecentMetrics() []MetricRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]MetricRecord, len(h.metrics))
	copy(out, h.metrics)
	return out
}

// Clear resets the buffered spans and metrics.
func (h *InMemoryHook) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.spans = h.spans[:0]
	h.metrics = h.metrics[:0]
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

// GlobalManager returns the singleton telemetry manager with default in-memory sink registered.
func GlobalManager() *Manager {
	once.Do(func() {
		defaultHook := NewInMemoryHook(1024)
		globalManager = &Manager{
			hooks: []Hook{defaultHook},
		}
	})
	return globalManager
}

// GetHooks returns a copy of registered hooks.
func (m *Manager) GetHooks() []Hook {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Hook, len(m.hooks))
	copy(out, m.hooks)
	return out
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
