package functional

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
)

const (
	resultStatusComplete = "complete"
	resultStatusError    = "error"
	resultUnwrapErrFmt   = "Result.Unwrap called on error: %v"
	operationIDFmt       = "%s_%d"
	emptyOperationType   = ""

	metricKeyOperationType = "operation_type"
	metricKeyDurationNs    = "duration_ns"
)

// Result represents a value or an error, similar to Rust's Result type
// This provides a functional approach to error handling with metrics integration
type Result[T any] struct {
	value T
	err   error
}

// Ok creates a successful Result
func Ok[T any](value T) Result[T] {
	return Result[T]{value: value, err: nil}
}

// Err creates a failed Result
func Err[T any](err error) Result[T] {
	var zero T
	return Result[T]{value: zero, err: err}
}

// From creates a Result from a value and error
func From[T any](value T, err error) Result[T] {
	return Result[T]{value: value, err: err}
}

// IsOk returns true if the Result is successful
func (r Result[T]) IsOk() bool {
	return r.err == nil
}

// IsErr returns true if the Result is an error
func (r Result[T]) IsErr() bool {
	return r.err != nil
}

// Value returns the value and error (unwraps the Result)
func (r Result[T]) Value() (T, error) {
	return r.value, r.err
}

// Unwrap returns the value, panicking if there's an error
func (r Result[T]) Unwrap() T {
	if r.err != nil {
		// TRACK: [Must panic on unhandled unwrap]
		panic(fmt.Sprintf(resultUnwrapErrFmt, r.err))
	}
	return r.value
}

// UnwrapOr returns the value or a default if there's an error
func (r Result[T]) UnwrapOr(defaultValue T) T {
	if r.err != nil {
		return defaultValue
	}
	return r.value
}

// UnwrapOrElse returns the value or computes a default from the error
func (r Result[T]) UnwrapOrElse(fn func(error) T) T {
	if r.err != nil {
		return fn(r.err)
	}
	return r.value
}

// Map applies a function to the value if successful, returning a new Result
func Map[T, U any](r Result[T], fn func(T) U) Result[U] {
	if r.err != nil {
		return Err[U](r.err)
	}
	return Ok(fn(r.value))
}

// MapErr applies a function to the error if failed, returning a new Result
func MapErr[T any](r Result[T], fn func(error) error) Result[T] {
	if r.err == nil {
		return r
	}
	return Err[T](fn(r.err))
}

// AndThen chains operations, returning a new Result
func AndThen[T, U any](r Result[T], fn func(T) Result[U]) Result[U] {
	if r.err != nil {
		return Err[U](r.err)
	}
	return fn(r.value)
}

// OrElse returns the Result if successful, otherwise returns the alternative
func OrElse[T any](r Result[T], alternative Result[T]) Result[T] {
	if r.err == nil {
		return r
	}
	return alternative
}

// OrElseGet returns the Result if successful, otherwise computes an alternative
func OrElseGet[T any](r Result[T], fn func(error) Result[T]) Result[T] {
	if r.err == nil {
		return r
	}
	return fn(r.err)
}

// Apply applies a function that returns a Result, with metrics integration
func Apply[T, U any](
	ctx context.Context,
	target T,
	fn func(T) (U, error),
	opts ...ApplyOption,
) Result[U] {
	config := defaultApplyConfig()
	for _, opt := range opts {
		opt(config)
	}

	start := time.Now()
	value, err := fn(target)
	duration := time.Since(start)

	// Record metrics if coordinator available
	if config.coordinator != nil && config.operationType != emptyOperationType {
		status := resultStatusComplete
		if err != nil {
			status = resultStatusError
		}
		eventCtx := coordination.NewEventContext(
			fmt.Sprintf(operationIDFmt, config.operationType, time.Now().UnixNano()),
			config.operationType,
			status,
		).WithDuration(duration).WithChannels(false, false, true, false)

		if err != nil {
			eventCtx = eventCtx.WithError(err)
		}

		eventCtx.EventData = &coordination.EventData{
			MetricsData: map[string]any{
				metricKeyOperationType: config.operationType,
				metricKeyDurationNs:    duration.Nanoseconds(),
			},
		}

		_ = config.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	}

	return From(value, err)
}

// ApplyOrElse applies a function with a fallback, with metrics integration
func ApplyOrElse[T, U any](
	ctx context.Context,
	target T,
	fn func(T) (U, error),
	fallback func(error) (U, error),
	opts ...ApplyOption,
) Result[U] {
	result := Apply(ctx, target, fn, opts...)
	if result.IsOk() {
		return result
	}

	// Apply fallback with same options
	fallbackResult := Apply(ctx, target, func(T) (U, error) {
		return fallback(result.err)
	}, opts...)

	return fallbackResult
}

// ApplyAndThen chains operations with metrics integration
func ApplyAndThen[T, U, V any](
	ctx context.Context,
	target T,
	fn1 func(T) (U, error),
	fn2 func(U) (V, error),
	opts ...ApplyOption,
) Result[V] {
	result1 := Apply(ctx, target, fn1, opts...)
	if result1.IsErr() {
		return Err[V](result1.err)
	}

	// Chain second operation
	return Apply(ctx, result1.value, fn2, opts...)
}

// Do executes an operation that returns only an error, with metrics integration
func Do[T any](
	ctx context.Context,
	target T,
	fn func(T) error,
	opts ...ApplyOption,
) error {
	_, err := Apply(ctx, target, func(T) (struct{}, error) {
		return struct{}{}, fn(target)
	}, opts...).Value()
	return err
}

// DoOrElse executes an operation with a fallback, with metrics integration
func DoOrElse[T any](
	ctx context.Context,
	target T,
	fn func(T) error,
	fallback func(error) error,
	opts ...ApplyOption,
) error {
	err := Do(ctx, target, fn, opts...)
	if err == nil {
		return nil
	}
	return fallback(err)
}

// DoAndThen chains operations that return only errors, with metrics integration
func DoAndThen[T, U any](
	ctx context.Context,
	target T,
	fn1 func(T) (U, error),
	fn2 func(U) error,
	opts ...ApplyOption,
) error {
	result := Apply(ctx, target, fn1, opts...)
	if result.IsErr() {
		return result.err
	}
	return Do(ctx, result.value, fn2, opts...)
}

// Get retrieves a value with metrics integration
func Get[T any](
	ctx context.Context,
	fn func() (T, error),
	opts ...ApplyOption,
) Result[T] {
	return Apply(ctx, struct{}{}, func(struct{}) (T, error) {
		return fn()
	}, opts...)
}

// GetOrElse retrieves a value with a fallback, with metrics integration
func GetOrElse[T any](
	ctx context.Context,
	fn func() (T, error),
	fallback T,
	opts ...ApplyOption,
) T {
	result := Get(ctx, fn, opts...)
	return result.UnwrapOr(fallback)
}

// GetOrElseGet retrieves a value with a computed fallback, with metrics integration
func GetOrElseGet[T any](
	ctx context.Context,
	fn func() (T, error),
	fallback func(error) (T, error),
	opts ...ApplyOption,
) Result[T] {
	result := Get(ctx, fn, opts...)
	if result.IsOk() {
		return result
	}
	return Get(ctx, func() (T, error) {
		return fallback(result.err)
	}, opts...)
}

// ApplyConfig holds configuration for Apply operations
type ApplyConfig struct {
	coordinator   coordination.EventCoordinator
	operationType string
}

func defaultApplyConfig() *ApplyConfig {
	return &ApplyConfig{
		coordinator: coordination.GetCoordinator(),
	}
}

// ApplyOption configures Apply operations
type ApplyOption func(*ApplyConfig)

// WithCoordinator sets the coordinator for metrics
func WithCoordinator(coordinator coordination.EventCoordinator) ApplyOption {
	return func(c *ApplyConfig) {
		c.coordinator = coordinator
	}
}

// WithOperationType sets the operation type for metrics
func WithOperationType(operationType string) ApplyOption {
	return func(c *ApplyConfig) {
		c.operationType = operationType
	}
}

// WithoutMetrics disables metrics collection
func WithoutMetrics() ApplyOption {
	return func(c *ApplyConfig) {
		c.coordinator = nil
	}
}
