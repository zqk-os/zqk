# pkg/loader

Abstract **component loader** pattern: shared state as atomics, callback on state change, configurable timeouts (default config + profile/thematic overrides). The **retryable component** is implemented as **`Runner`**: one-shot load with wait-for-completion channel and timeout.

- **Design:** [docs/process/architecture/COMPONENT_LOADER_PATTERN.md](../../../docs/process/architecture/COMPONENT_LOADER_PATTERN.md)
- **Types:** `LoadState`, `LoaderTimeoutConfig`, `StateChangeCallback`, `ComponentLoader`, `LoadFn`
- **Runner:** `NewRunner(name, loadFn, opts...)`, `Load(ctx)`, `ResetLoaded()`, `WithCallback`, `WithTimeoutConfig`
- **Config:** `LoadLoaderTimeoutConfig(configPath)`, `MergeLoaderTimeoutOverrides(base, overrides)`, `GetLoaderTimeoutConfig(loaderName)`

**Loaders using Runner:** ID validator (`id_patterns`), `BucketingConfigRegistry` (`bucketing_config`), `BucketStrategyLoader` (`bucket_strategy_loader`), `SpecLoader.EnsureReady` (`spec_loader`), `LifecycleLoader.EnsureReady` (`lifecycle`), `KindSynonymResolver.Initialize` (`kind_synonym_resolver`).

**Retries:** This package configures **timeouts** only (wait-for-completion, publish-channel, load-operation). It does **not** configure retries (retry on load failure). Retries are in `pkg/storage/operation_helper.go` (RetryConfig), `pkg/graph/provider/retry.go`, and scheduler job config (`retry_count`, `retry_delay_seconds`).
