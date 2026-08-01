package observability

import (
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const emptyValue = ""

// Recorder is a factory-based metrics recorder that handles its own registration
// and enabled/disabled state. This provides a flexible, decoupled metrics API.
type Recorder interface {
	// IsEnabled returns whether metrics collection is enabled for this recorder
	IsEnabled() bool

	// Record records a metric operation with flexible fields
	// The builder pattern enforces conventions and validates required fields
	Record(operation string, builder Builder) error
}

// Builder provides a fluent builder pattern for constructing metrics
// Enforces conventions and validates required fields, failing fast on errors
type Builder interface {
	// WithField adds a key-value field to the metric
	// Field names should follow snake_case convention
	WithField(key string, value any) Builder

	// WithDuration sets the operation duration
	WithDuration(duration time.Duration) Builder

	// WithError sets the error (nil for success)
	WithError(err error) Builder

	// WithTags adds tags for categorization (e.g., ["infrastructure", "config", "copy"])
	WithTags(tags ...string) Builder

	// Build validates and returns the metric data
	// Fails fast if required fields are missing or invalid
	Build() (Data, error)
}

// Data represents the structured metric data
type Data struct {
	// Operation is the operation type (e.g., "bootstrap_config_copy", "profile_creation")
	Operation string

	// Duration is the operation duration
	Duration time.Duration

	// Success indicates whether the operation succeeded (err == nil)
	Success bool

	// Error is the error message if operation failed (empty if success)
	Error string

	// Tags are categorization tags (e.g., ["infrastructure", "config"])
	Tags []string

	// Fields are additional context fields (e.g., {"filename": "id_prefixes_config.yaml", "is_update": true})
	Fields map[string]any

	// Timestamp is when the metric was recorded
	Timestamp time.Time
}

// defaultBuilder is the default implementation of Builder
type defaultBuilder struct {
	operation string
	duration  time.Duration
	err       error
	tags      []string
	fields    map[string]any
}

// NewBuilder creates a new metric builder for the given operation
// Operation names should follow snake_case convention (e.g., "bootstrap_config_copy")
func NewBuilder(operation string) Builder {
	return &defaultBuilder{
		operation: operation,
		fields:    make(map[string]any),
		tags:      []string{}, // No default tags - caller should specify
	}
}

func (b *defaultBuilder) WithField(key string, value any) Builder {
	if b.fields == nil {
		b.fields = make(map[string]any)
	}
	b.fields[key] = value
	return b
}

func (b *defaultBuilder) WithDuration(duration time.Duration) Builder {
	b.duration = duration
	return b
}

func (b *defaultBuilder) WithError(err error) Builder {
	b.err = err
	return b
}

func (b *defaultBuilder) WithTags(tags ...string) Builder {
	b.tags = append(b.tags, tags...)
	return b
}

func (b *defaultBuilder) Build() (Data, error) {
	// Validate required fields
	if b.operation == emptyValue {
		return Data{}, errfmt.Errorf("metric operation is required")
	}

	// Build metric data
	data := Data{
		Operation: b.operation,
		Duration:  b.duration,
		Success:   b.err == nil,
		Timestamp: time.Now(),
		Tags:      b.tags,
		Fields:    b.fields,
	}

	if b.err != nil {
		data.Error = b.err.Error()
	}

	return data, nil
}

// noOpRecorder is a no-op implementation that always returns disabled
type noOpRecorder struct{}

func (n *noOpRecorder) IsEnabled() bool {
	return false
}

func (n *noOpRecorder) Record(operation string, builder Builder) error {
	return nil // No-op
}

// GetNoOpRecorder returns a no-op metric recorder
func GetNoOpRecorder() Recorder {
	return &noOpRecorder{}
}

// Factory creates and registers metric recorders
// This allows the metrics system to be self-registering and decoupled
type Factory interface {
	// CreateRecorder creates a metric recorder for the given component
	// The recorder handles its own enabled/disabled state
	CreateRecorder(component string) Recorder

	// RegisterRecorder registers a custom recorder for a component
	RegisterRecorder(component string, recorder Recorder)

	// GetRecorder retrieves a recorder for a component (returns no-op if not found)
	GetRecorder(component string) Recorder
}

// defaultFactory is the default implementation
type defaultFactory struct {
	recorders map[string]Recorder
	enabled   bool
}

// NewFactory creates a new metric recorder factory
// enabled determines the default state for new recorders
func NewFactory(enabled bool) Factory {
	return &defaultFactory{
		recorders: make(map[string]Recorder),
		enabled:   enabled,
	}
}

func (f *defaultFactory) CreateRecorder(component string) Recorder {
	if !f.enabled {
		return GetNoOpRecorder()
	}
	// For now, return no-op - can be extended to create actual recorders
	// This allows the factory pattern to be in place without requiring full implementation
	return GetNoOpRecorder()
}

func (f *defaultFactory) RegisterRecorder(component string, recorder Recorder) {
	if f.recorders == nil {
		f.recorders = make(map[string]Recorder)
	}
	f.recorders[component] = recorder
}

func (f *defaultFactory) GetRecorder(component string) Recorder {
	if recorder, ok := f.recorders[component]; ok {
		return recorder
	}
	return GetNoOpRecorder()
}
