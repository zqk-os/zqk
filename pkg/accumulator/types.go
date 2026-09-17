package accumulator

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/storage"
)

// Default settings for Accumulators.
const (
	DefaultStalenessTolerance = 2 * time.Minute
	DefaultRebuildTimeout     = 45 * time.Second
	DefaultPollInterval       = 200 * time.Millisecond
	DefaultSchemaVersion      = "1.0.0"
)

// Envelope wraps any domain payload T with standardized telemetry, health, and degradation flags.
type Envelope[T any] struct {
	SchemaVersion  string    `json:"schema_version"`
	MaterializedAt time.Time `json:"materialized_at"`
	Stale          bool      `json:"stale,omitempty"`
	Recovering     bool      `json:"recovering,omitempty"`
	DegradedReason string    `json:"degraded_reason,omitempty"`
	Payload        T         `json:"payload"`
}

// AccumulatorSpec configures an Accumulator instance.
type AccumulatorSpec struct {
	Name               string
	SchemaVersion      string
	ProjectRoot        string
	StoragePath        string // Optional: explicit path override. Default is .zqk/state/<Name>_lite.json
	StalenessTolerance time.Duration
	RebuildTimeout     time.Duration
	PollInterval       time.Duration
}

// Accumulator defines the pure domain hooks that must be implemented by view authors.
type Accumulator[T any] interface {
	// Name returns the canonical name for this accumulator (e.g. "whats_next", "test_dashboard").
	Name() string

	// DefaultPayload returns the cold-boot skeleton payload T when no prior state is available.
	DefaultPayload() T

	// ApplyEvent mutates the in-memory graph given a single lifecycle WAL event.
	// Returns true if the event resulted in a state change requiring persistence.
	ApplyEvent(ev *lifecycle.LifecycleEvent) (mutated bool)

	// BuildPayload renders the current in-memory graph into the serialized domain payload T.
	BuildPayload() T

	// ScanFromStorage performs a full out-of-band rebuild from the object storage provider.
	ScanFromStorage(ctx context.Context, sp storage.ObjectStorageProvider) error
}
