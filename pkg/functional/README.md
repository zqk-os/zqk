# Functional Error Handling with Metrics

A fluent, functional-style API for error handling with automatic metrics integration via the coordinator pattern.

## Overview

This package provides a Rust-inspired `Result` type and functional combinators for clean error handling, with built-in metrics collection for observability.

## Core Types

### Result[T]

A generic type representing either a value or an error:

```go
type Result[T any] struct {
    value T
    err   error
}
```

## Basic Usage

### Creating Results

```go
// Success
result := functional.Ok(42)

// Error
result := functional.Err[int](fmt.Errorf("failed"))

// From value and error
value, err := someFunction()
result := functional.From(value, err)
```

### Unwrapping Values

```go
// Panic on error
value := result.Unwrap()

// Default value on error
value := result.UnwrapOr(0)

// Compute default from error
value := result.UnwrapOrElse(func(err error) int {
    return -1
})
```

### Transforming Results

```go
// Map successful value
doubled := functional.Map(result, func(x int) int {
    return x * 2
})

// Map error
betterErr := functional.MapErr(result, func(err error) error {
    return fmt.Errorf("wrapped: %w", err)
})

// Chain operations
final := functional.AndThen(result, func(x int) functional.Result[string] {
    return functional.Ok(fmt.Sprintf("%d", x))
})
```

## Metrics Integration

### Apply Operations

`Apply` executes a function and automatically records metrics via the coordinator:

```go
storageProvider := functional.Apply(
    ctx,
    projectRoot,
    func(root string) (storage.ObjectStorageProvider, error) {
        return storage.NewFileObjectStorage(root)
    },
    functional.WithOperationType("storage_init"),
).UnwrapOr(nil)
```

**Before (messy)**:
```go
var storageProvider storage.ObjectStorageProvider
if projectRoot != "" {
    var storageErr error
    storageProvider, storageErr = storage.NewFileObjectStorage(projectRoot)
    if storageErr != nil {
        storageProvider = nil
    }
}
```

**After (clean)**:
```go
storageProvider := functional.Apply(
    ctx,
    projectRoot,
    storage.NewFileObjectStorage,
    functional.WithOperationType("storage_init"),
).UnwrapOr(nil)
```

### ApplyOrElse

Apply with fallback:

```go
result := functional.ApplyOrElse(
    ctx,
    target,
    primaryFunction,
    fallbackFunction,
    functional.WithOperationType("operation"),
)
```

### ApplyAndThen

Chain operations:

```go
result := functional.ApplyAndThen(
    ctx,
    target,
    step1,
    step2,
    functional.WithOperationType("pipeline"),
)
```

## Do Operations

For operations that only return errors:

```go
// Execute operation
err := functional.Do(
    ctx,
    target,
    func(t Target) error {
        return t.Operation()
    },
    functional.WithOperationType("operation"),
)

// With fallback
err := functional.DoOrElse(
    ctx,
    target,
    primaryOperation,
    fallbackOperation,
    functional.WithOperationType("operation"),
)

// Chain operations
err := functional.DoAndThen(
    ctx,
    target,
    step1,  // Returns (U, error)
    step2,  // Takes U, returns error
    functional.WithOperationType("pipeline"),
)
```

## Get Operations

For operations that return values:

```go
// Get value
result := functional.Get(
    ctx,
    func() (Value, error) {
        return fetchValue()
    },
    functional.WithOperationType("fetch"),
)

// Get with default
value := functional.GetOrElse(
    ctx,
    fetchValue,
    defaultValue,
    functional.WithOperationType("fetch"),
)

// Get with computed fallback
result := functional.GetOrElseGet(
    ctx,
    fetchValue,
    func(err error) (Value, error) {
        return fetchFallbackValue()
    },
    functional.WithOperationType("fetch"),
)
```

## Options

### WithCoordinator

Set a specific coordinator for metrics:

```go
coordinator := coordination.GetCoordinator()
result := functional.Apply(
    ctx,
    target,
    fn,
    functional.WithCoordinator(coordinator),
    functional.WithOperationType("operation"),
)
```

### WithOperationType

Set the operation type for metrics:

```go
result := functional.Apply(
    ctx,
    target,
    fn,
    functional.WithOperationType("storage_init"),
)
```

### WithoutMetrics

Disable metrics collection:

```go
result := functional.Apply(
    ctx,
    target,
    fn,
    functional.WithoutMetrics(),
)
```

## Metrics Collection

When using `Apply`, `Do`, or `Get` operations with metrics enabled:

1. **Operation start**: Timestamp recorded
2. **Operation execution**: Function executed
3. **Operation completion**: Duration calculated
4. **Event emission**: Event emitted via coordinator with:
   - Operation type
   - Duration
   - Status (complete/error)
   - Error details (if any)

Metrics are automatically routed to the metrics channel via the coordinator pattern.

## Examples

### Storage Initialization

```go
// Clean, metrics-enabled storage initialization
storageProvider := functional.Get(
    ctx,
    func() (storage.ObjectStorageProvider, error) {
        if projectRoot == "" {
            return nil, nil // Not an error, just no root
        }
        return storage.NewFileObjectStorage(projectRoot)
    },
    functional.WithOperationType("mcp_storage_init"),
).UnwrapOr(nil)

if storageProvider != nil {
    setupCoordinator(storageProvider)
}
```

### Chained Operations

```go
// Chain multiple operations with metrics
result := functional.ApplyAndThen(
    ctx,
    input,
    func(input Input) (Intermediate, error) {
        return processStep1(input)
    },
    func(intermediate Intermediate) (Output, error) {
        return processStep2(intermediate)
    },
    functional.WithOperationType("processing_pipeline"),
)

output := result.UnwrapOr(defaultOutput)
```

### Error Recovery

```go
// Try primary, fallback on error
result := functional.ApplyOrElse(
    ctx,
    target,
    primaryOperation,
    func(err error) (Value, error) {
        // Log error, try fallback
        logger.Warn("Primary failed, using fallback", logging.Error(err))
        return fallbackOperation()
    },
    functional.WithOperationType("operation_with_fallback"),
)
```

## Benefits

1. **Clean Syntax**: No more nested if-else error handling
2. **Automatic Metrics**: Metrics collected automatically via coordinator
3. **Composable**: Chain operations easily
4. **Type Safe**: Generic types ensure type safety
5. **Functional Style**: Inspired by Rust's Result type
6. **Observability**: Built-in integration with coordinator pattern

## Integration with Coordinator

The functional API automatically integrates with the coordinator pattern:

- Operations emit events via coordinator
- Metrics include operation type, duration, and status
- Errors are captured and included in metrics
- All metrics flow through the unified observability system
