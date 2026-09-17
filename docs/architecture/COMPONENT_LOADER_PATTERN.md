# Component Loader Pattern

**Last Verified:** 2026-08-31


**Status:** Implemented  
**Purpose:** Abstract pattern for load-once components (e.g. ID patterns, bucketing config, bucket strategy loader) with shared state as atomics, completion callback/channel, and configurable timeouts (default config + profile/thematic overrides). The **retryable component** (atomics + completion channel + timeout) is implemented as `pkg/loader.Runner`.

## Goals

1. **Shared state as atomics** – Implementer declares loading/loaded (and optionally value or error) as atomics for lock-free fast path and clear visibility.
2. **Callback on state change** – Listeners can attach to loading start, loaded (with value), error, or timeout; chainable or sequential.
3. **Single responsibility** – Pattern tracks: initializing/loading state, loaded state, and the loaded value or an error (including timeout).
4. **Configurable timeouts** – Per implementation (e.g. per loader name). Default from config file; overridable by profile or thematic config.

## State Model

- **Unloaded** – Not started.
- **Loading** – Load in progress; one writer (loader), many readers (waiters).
- **Loaded** – Success; value available.
- **Error** – Failed or timed out; error available.

Implementations may use `atomic.Bool` for loading/loaded and an `atomic.Value` for a completion channel (callback/notify) and/or the result/error.

## Interface (Conceptual)

```go
// LoadState represents the lifecycle of a component load.
type LoadState int
const (
    LoadStateUnloaded LoadState = iota
    LoadStateLoading
    LoadStateLoaded
    LoadStateError
)

// ComponentLoader is the interface implementers satisfy.
// Shared state (atomics) is owned by the implementer; the runner or implementer
// updates them and invokes callbacks.
type ComponentLoader interface {
    // State (atomics) – implementer provides
    Loaded() bool
    Loading() bool
    // Load runs the actual load; may be called once (winner) or used to wait.
    Load(ctx context.Context) error
    // TimeoutConfig returns the timeout to use for this loader (from default + overrides).
    TimeoutConfig() LoaderTimeoutConfig
}

// LoaderTimeoutConfig is per-loader timeout configuration.
type LoaderTimeoutConfig struct {
    WaitForCompletion time.Duration // max wait for "loading" to complete (e.g. 5s)
    PublishChannel    time.Duration // max wait for completion channel to appear (e.g. 1s)
    LoadOperation     time.Duration // timeout for the actual I/O load (optional)
}

// StateChangeCallback is invoked when state transitions (loading -> loaded | error).
// Implementations can be chainable or sequential.
type StateChangeCallback interface {
    OnLoading()
    OnLoaded(value any)
    OnError(err error)
    OnTimeout()
}
```

## Callback / Notify

- **Completion signal:** Use a channel closed when load completes (success or error). Waiters `select` on that channel with `TimeoutConfig().WaitForCompletion`.
- **Optional listener:** Implementer or runner calls `OnLoading()` when starting, `OnLoaded(value)` or `OnError(err)` / `OnTimeout()` when done.

## Timeout Configuration

1. **Default config file** – e.g. `.zqk/config/config.yaml` under a key such as `component_loaders` or `loaders`, keyed by loader name:
   ```yaml
   component_loaders:
     id_patterns:
       wait_for_completion_seconds: 5
       publish_channel_seconds: 1
     lifecycle:
       wait_for_completion_seconds: 10
   ```
2. **Profile override** – When a profile is active (e.g. `ai-agent`, `human`), profile spec or context can supply overrides for the same keys (e.g. longer timeouts for debug profile).
3. **Thematic override** – Another config layer (e.g. theme or environment) can override again. Resolution order: default file → profile → theme/env.

Implementations receive a `LoaderTimeoutConfig` (or a resolver that returns it given profile/context) so they are not hardcoded.

## Usage (loaders using the abstraction)

All of the following use `pkg/loader.Runner` (retryable component: atomics, completion channel, configurable timeouts):

- **Object ID cache** (`cmd/zqk/system`) – `EnsureObjectIDCacheReady(ctx, ...)` via `getEnsureRunner().Load()`; runner name `object_id_cache`. Load/build and optional warm; progress via `ObjectIDCacheProgressNotifier`.
- **ID validator** (`pkg/validation`) – `LoadPatterns()` / `ReloadPatterns()` via `getPatternsRunner().Load()`; runner name `id_patterns`.
- **BucketingConfigRegistry** (`pkg/storage`) – `Load()` via runner; runner name `bucketing_config`.
- **BucketStrategyLoader** (`pkg/storage`) – `Initialize(ctx)` / `Reload(ctx)` via runner; runner name `bucket_strategy_loader`.
- **SpecLoader** (`pkg/objects`) – `EnsureReady(ctx)` warms base specs (base_object, auditable) once via runner; runner name `spec_loader`. Per-spec loading remains lock-free.
- **LifecycleLoader** (`pkg/objects`) – `EnsureReady(ctx)` warms lifecycles dir and base_object lifecycle once via runner; runner name `lifecycle`.
- **DynamicKindMapper** (`pkg/objects`) – `EnsureReady(ctx)` runs `Initialize()` once via runner; runner name `kind_mapper`. Used before warm CAS and in init so kind→directory mappings are ready with timeouts and telemetry.
- **KindSynonymResolver** (`pkg/objects`) – `Initialize()` via runner; runner name `kind_synonym_resolver`. Reload after `SetSynonymLoader` uses `ResetLoaded()`.

**Call sites (all embrace the pattern for standardization and telemetry):**
- **System check:** Before cache build, runs **four** loaders in parallel: `SpecLoader.EnsureReady(ctx)`, `LifecycleLoader.EnsureReady(ctx)`, `GetIDValidator().LoadPatterns()` (id_patterns Runner), and storage creation. Object ID cache via `EnsureObjectIDCacheReady` (Runner). Warm phase calls `GetGlobalKindMapper().EnsureReady(ctx)` (kind_mapper Runner) before per-kind pre-init.
- **Init / createAllKindDirectories:** `GetGlobalKindMapper().EnsureReady(ctx)` (kind_mapper Runner) instead of raw `Initialize()`.

## Where timeouts and retries are configured

- **Component loader timeouts** (this pattern): `.zqk/config/config.yaml` under `component_loaders.<name>`:
  - `wait_for_completion_seconds` – max wait for a concurrent load to complete
  - `publish_channel_seconds` – max wait for completion channel to appear
  - `load_operation_seconds` – optional timeout for the load I/O itself  
  Defaults are in `pkg/loader/config.go` (`DefaultLoaderTimeoutConfigMap()`); file overrides merge on top.

- **Retries** (retry on failure): The loader package does **not** configure retries. If a load fails, the Runner returns the error; callers can call `Load` again or use `ResetLoaded()` and then `Load()` for a full reload. Retry-on-failure with backoff is configured elsewhere:
  - **Storage operations:** `pkg/storage/operation_helper.go` – `RetryConfig` (MaxAttempts, InitialDelay, MaxDelay, BackoffFactor), `ExecuteWithConfig(..., RetryConfig)`.
  - **Graph backend:** `pkg/graph/provider/retry.go` – graph connection retry logic.
  - **Scheduler jobs:** Job YAML – `retry_count`, `retry_delay_seconds` (see `docs/architecture/SCHEDULER_EXECUTION_BOUNDARIES.md`).
  - **Lifecycle I/O:** `pkg/objects/lifecycle_loader.go` – `LifecycleIOConfig.RetryMaxAttempts`, etc.

## Package

- **`pkg/loader`** – Defines `LoadState`, `LoaderTimeoutConfig`, `StateChangeCallback`, `ComponentLoader`, `LoadFn`, and **`Runner`**. **Runner** runs a `LoadFn` with the retryable pattern: lock-free fast path, wait-for-completion channel with timeout, optional callback. Timeout config: `LoadLoaderTimeoutConfig(configPath)` loads from `.zqk/config/config.yaml` under `component_loaders`; `MergeLoaderTimeoutOverrides(base, overrides)` applies profile/thematic overrides (call from context layer). `GetLoaderTimeoutConfig(loaderName)` returns config for a named loader (e.g. `object_id_cache`, `id_patterns`, `lifecycle`, `bucketing_config`, `bucket_strategy_loader`, `spec_loader`, `kind_mapper`, `kind_synonym_resolver`).

## References

- Concurrency: `docs/architecture/concurrency/CONCURRENCY_STANDARDIZATION_*.md` (callback/notify, channel + timeout).
- Validation timeout precedent: `pkg/validation/validation_timeout_config.go` (default file + overrides).
- Profile context: `internal/cli/context` (profile, storage context overrides).
