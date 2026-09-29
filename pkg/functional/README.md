# Functional Error Handling with Metrics (`pkg/functional`)

A fluent, functional-style Go API for monadic error handling, safe map operations, and telemetry-integrated execution with automatic metrics capture via the coordinator pattern.

---

## Overview

Go's idiomatic `(value, error)` convention frequently leads to verbose, repetitive boilerplate and deeply nested conditional branches. The `pkg/functional` package provides:

1. **`Result[T]` Monad**: A generic, type-safe container encapsulating either a successful value (`T`) or an `error`, inspired by Rust's `Result<T, E>`.
2. **Functional Combinators**: Composable operators (`Map`, `MapErr`, `AndThen`, `OrElse`, `OrElseGet`) that transform and chain computations without manual error branching.
3. **Telemetry-Integrated Execution**: Higher-order functions (`Apply`, `Do`, `Get`) that execute operations while automatically recording execution duration, status, and error metadata into the ZQK coordinator event pipeline.
4. **Nil-Safe Map Utilities**: Helper functions for pointer maps, default lookups, and key/value slice extractions.
5. **Fluent Conditional Execution**: Re-exported `When` builder for expressive conditional pipelines.

---

## Core Types & API Contract

### 1. `Result[T any]`

The primary generic type representing either an operation's successful payload or an error:

```go
type Result[T any] struct {
    value T     // Encapsulated success value (unexported)
    err   error // Encapsulated error state (unexported)
}
```

#### Constructors

| Constructor | Signature | Description |
| :--- | :--- | :--- |
| `Ok` | `func Ok[T any](value T) Result[T]` | Wraps a successful value into a `Result[T]` with `err = nil`. |
| `Err` | `func Err[T any](err error) Result[T]` | Wraps an error into a `Result[T]` with zero-value `T`. |
| `From` | `func From[T any](value T, err error) Result[T]` | Lifts a standard Go `(T, error)` tuple directly into `Result[T]`. |

#### Inspection & Unwrapping Methods

| Method | Signature | Description |
| :--- | :--- | :--- |
| `IsOk()` | `func (r Result[T]) IsOk() bool` | Returns `true` if the result represents a success (`err == nil`). |
| `IsErr()` | `func (r Result[T]) IsErr() bool` | Returns `true` if the result represents an error (`err != nil`). |
| `Value()` | `func (r Result[T]) Value() (T, error)` | Unpacks the `Result[T]` back into a standard Go `(T, error)` tuple. |
| `Unwrap()` | `func (r Result[T]) Unwrap() T` | Returns the inner value. **Panics** with error details if `IsErr() == true`. |
| `UnwrapOr()` | `func (r Result[T]) UnwrapOr(defaultValue T) T` | Returns the inner value if successful; otherwise returns `defaultValue`. |
| `UnwrapOrElse()` | `func (r Result[T]) UnwrapOrElse(fn func(error) T) T` | Returns the inner value if successful; otherwise computes fallback value via `fn(err)`. |

---

### 2. Functional Combinators

Combinators allow transforming and chaining `Result` values declaratively:

| Function | Signature | Semantic Meaning |
| :--- | :--- | :--- |
| `Map` | `func Map[T, U any](r Result[T], fn func(T) U) Result[U]` | Applies `fn` to the inner value if `IsOk()`. If `IsErr()`, propagates the error. |
| `MapErr` | `func MapErr[T any](r Result[T], fn func(error) error) Result[T]` | Applies `fn` to the error if `IsErr()`. If `IsOk()`, leaves value untouched. |
| `AndThen` | `func AndThen[T, U any](r Result[T], fn func(T) Result[U]) Result[U]` | Chains an operation that returns another `Result[U]` (monadic flatMap). |
| `OrElse` | `func OrElse[T any](r Result[T], alternative Result[T]) Result[T]` | Returns `r` if `IsOk()`; otherwise returns `alternative`. |
| `OrElseGet` | `func OrElseGet[T any](r Result[T], fn func(error) Result[T]) Result[T]` | Returns `r` if `IsOk()`; otherwise evaluates `fn(err)` to produce a fallback `Result[T]`. |

---

### 3. Telemetry Configuration Types

`Apply`, `Do`, and `Get` accept functional options to configure metrics and event routing:

```go
// ApplyConfig holds runtime configuration for telemetry-enabled operations
type ApplyConfig struct {
    coordinator   coordination.EventCoordinator // Coordinator destination for metrics
    operationType string                        // Telemetry label identifying the operation
}

// ApplyOption modifies ApplyConfig
type ApplyOption func(*ApplyConfig)
```

#### Available Options

- `WithCoordinator(c coordination.EventCoordinator) ApplyOption`: Sets an explicit event coordinator.
- `WithOperationType(opType string) ApplyOption`: Labels the emitted metric event (e.g. `"storage_init"`, `"query_exec"`).
- `WithoutMetrics() ApplyOption`: Disables metrics collection (useful in lightweight inner loops or unit tests).

---

## Basic Usage

### Constructing and Inspecting Results

```go
package main

import (
    "errors"
    "fmt"

    "github.com/zqk-os/zqk/pkg/functional"
)

func main() {
    // 1. Success result
    res1 := functional.Ok(42)
    fmt.Println(res1.IsOk()) // true
    fmt.Println(res1.Unwrap()) // 42

    // 2. Error result
    res2 := functional.Err[int](errors.New("disk full"))
    fmt.Println(res2.IsErr()) // true
    fmt.Println(res2.UnwrapOr(0)) // 0

    // 3. From standard Go call
    value, err := computeSomething()
    res3 := functional.From(value, err)
    
    // 4. Safe fallback with error inspect
    finalVal := res3.UnwrapOrElse(func(err error) int {
        fmt.Printf("Computation failed: %v, using default\n", err)
        return -1
    })
    _ = finalVal
}

func computeSomething() (int, error) {
    return 100, nil
}
```

### Transforming and Chaining Results

```go
// Map value: double the integer if Ok
doubled := functional.Map(res1, func(x int) int {
    return x * 2
})

// Map error: annotate failure context
betterErr := functional.MapErr(res2, func(err error) error {
    return fmt.Errorf("storage layer failure: %w", err)
})

// AndThen: Monadic chain returning a different Result type
stringified := functional.AndThen(res1, func(x int) functional.Result[string] {
    if x < 0 {
        return functional.Err[string](errors.New("negative value"))
    }
    return functional.Ok(fmt.Sprintf("val=%d", x))
})
```

---

## Metrics Integration & Telemetry Execution

`pkg/functional` bridges execution with the ZQK **coordination** spinal cord. Any operation wrapped via `Apply`, `Do`, or `Get` automatically measures duration, handles error states, and emits an event to the metrics channel.

```
Caller
  │
  ▼
functional.Apply(ctx, target, fn, WithOperationType("storage_init"))
  │
  ├── 1. time.Now()
  ├── 2. fn(target) ──> (U, error)
  ├── 3. duration = time.Since(start)
  ├── 4. coordinator.Emit(ctx, eventCtx)  ──> Metrics / Event Stream
  │
  ▼
Result[U] (Ok or Err)
```

### 1. `Apply` Family (Transforms Input `T` to Output `U`)

#### Standard Apply
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

#### Apply with Fallback (`ApplyOrElse`)
Executes the primary function, falling back to an alternative if it fails:
```go
result := functional.ApplyOrElse(
    ctx,
    target,
    primaryFunction,
    func(err error) (Output, error) {
        logger.Warn("Primary failed, attempting fallback", "err", err)
        return fallbackFunction(target)
    },
    functional.WithOperationType("resilient_operation"),
)
```

#### Chained Apply (`ApplyAndThen`)
Pipelines two functions sequentially with automatic propagation:
```go
result := functional.ApplyAndThen(
    ctx,
    rawInput,
    parseStage,   // func(Raw) (Parsed, error)
    enrichStage,  // func(Parsed) (Enriched, error)
    functional.WithOperationType("pipeline_stage"),
)
```

---

### 2. `Do` Family (Side Effects Returning Only `error`)

For operations that do not yield values, only errors:

```go
// Execute side-effect operation
err := functional.Do(
    ctx,
    task,
    func(t Task) error {
        return t.Execute()
    },
    functional.WithOperationType("task_execution"),
)

// Do with fallback
err = functional.DoOrElse(
    ctx,
    task,
    primaryAction,
    fallbackAction,
    functional.WithOperationType("task_recovery"),
)

// Chained Do: step1 produces U, step2 consumes U and returns error
err = functional.DoAndThen(
    ctx,
    input,
    func(in Input) (Intermediate, error) {
        return buildIntermediate(in)
    },
    func(inter Intermediate) error {
        return commitIntermediate(inter)
    },
    functional.WithOperationType("commit_pipeline"),
)
```

---

### 3. `Get` Family (Value Producers Without Input Target)

For zero-argument producer functions:

```go
// Fetch value with metrics
res := functional.Get(
    ctx,
    fetchRemoteConfig,
    functional.WithOperationType("fetch_config"),
)

// Get with static fallback
val := functional.GetOrElse(
    ctx,
    fetchRemoteConfig,
    defaultConfig,
    functional.WithOperationType("fetch_config"),
)

// Get with computed fallback
res = functional.GetOrElseGet(
    ctx,
    fetchRemoteConfig,
    func(err error) (Config, error) {
        return loadLocalCacheConfig()
    },
    functional.WithOperationType("fetch_config"),
)
```

---

## Nil-Safe Map Utilities (`maps.go`)

`pkg/functional` provides nil-tolerant and generic map helper routines:

```go
import "github.com/zqk-os/zqk/pkg/functional"

// 1. Pointer Map Lookup (nil-safe, returns nil if absent or val is nil)
var ptrMap map[string]*Session
session := functional.MapGetPtr(ptrMap, "session-123") // nil, does not panic

// 2. Pointer Map Membership Check (true only if present AND pointer != nil)
hasValidSession := functional.MapHasPtr(ptrMap, "session-123")

// 3. Map Get With Fallback
portMap := map[string]int{"http": 8080}
grpcPort := functional.MapGetOrDefault(portMap, "grpc", 9090) // 9090

// 4. Map Keys Extraction
keys := functional.MapKeys(portMap) // []string{"http"}

// 5. Map Values Extraction
vals := functional.MapValues(portMap) // []int{8080}
```

---

## Fluent Conditional Execution (`when.go`)

Re-exports `When` from `pkg/when` to enable clean, declarative conditional execution without multi-tier `if` statements:

```go
import "github.com/zqk-os/zqk/pkg/functional"

functional.When(func() bool {
    return isProduction && featureFlagEnabled
}).Then(func() {
    startHighFrequencyMonitor()
})
```

---

## Benefits & Architectural Alignment

1. **Elimination of Defensive Boilerplate**: Replaces 5-line `if err != nil` nesting with clean, one-line functional transformations.
2. **First-Class Observability**: Built directly into the `Apply`, `Do`, and `Get` dispatch mechanisms, ensuring uniform latency and error tracking.
3. **Rust-Grade Type Safety**: Generics ensure compile-time type verification with zero runtime reflection overhead.
4. **Resilient Failure Recovery**: Seamless fallback chains with `ApplyOrElse`, `OrElseGet`, and `UnwrapOrElse`.
5. **Zero Magic Literals**: Perfectly integrates with domain constants and `coordination.EventContext`.
