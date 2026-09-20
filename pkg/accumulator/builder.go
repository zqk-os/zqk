package accumulator

import "time"

// AccumulatorBuilder provides a fluent interface for configuring and assembling an Engine[T].
type AccumulatorBuilder[T any] struct {
	acc  Accumulator[T]
	spec AccumulatorSpec
}

// NewBuilder creates a new fluent builder wrapping the provided domain Accumulator implementation.
func NewBuilder[T any](acc Accumulator[T]) *AccumulatorBuilder[T] {
	return &AccumulatorBuilder[T]{
		acc: acc,
		spec: AccumulatorSpec{
			SchemaVersion:      DefaultSchemaVersion,
			StalenessTolerance: DefaultStalenessTolerance,
			RebuildTimeout:     DefaultRebuildTimeout,
			PollInterval:       DefaultPollInterval,
		},
	}
}

// WithName overrides the accumulator name.
func (b *AccumulatorBuilder[T]) WithName(name string) *AccumulatorBuilder[T] {
	b.spec.Name = name
	return b
}

// WithProjectRoot sets the project root path for WAL discovery and storage resolution.
func (b *AccumulatorBuilder[T]) WithProjectRoot(root string) *AccumulatorBuilder[T] {
	b.spec.ProjectRoot = root
	return b
}

// WithStoragePath explicitly overrides the destination path of the materialized lite file.
func (b *AccumulatorBuilder[T]) WithStoragePath(path string) *AccumulatorBuilder[T] {
	b.spec.StoragePath = path
	return b
}

// WithStalenessTolerance sets the duration after which the projection is treated as stale.
func (b *AccumulatorBuilder[T]) WithStalenessTolerance(tolerance time.Duration) *AccumulatorBuilder[T] {
	b.spec.StalenessTolerance = tolerance
	return b
}

// WithRebuildTimeout sets the maximum duration for out-of-band background rebuilds.
func (b *AccumulatorBuilder[T]) WithRebuildTimeout(timeout time.Duration) *AccumulatorBuilder[T] {
	b.spec.RebuildTimeout = timeout
	return b
}

// WithPollInterval sets the interval for lifecycle WAL replay queries.
func (b *AccumulatorBuilder[T]) WithPollInterval(interval time.Duration) *AccumulatorBuilder[T] {
	b.spec.PollInterval = interval
	return b
}

// WithSchemaVersion sets the schema version written to the universal envelope.
func (b *AccumulatorBuilder[T]) WithSchemaVersion(version string) *AccumulatorBuilder[T] {
	b.spec.SchemaVersion = version
	return b
}

// Build validates configuration and instantiates the runtime Engine[T].
func (b *AccumulatorBuilder[T]) Build() (*Engine[T], error) {
	return NewEngine[T](b.spec, b.acc)
}
