# Validation Package

This package provides object validation for zqk: instance validation (schema, lifecycle, semantic types), ID validation (prefixes and patterns), and async validation with a queue and worker pool.

## Overview

- **Validator interface**: Pluggable validators (Go, SHACL, custom) via `Validator`, `ValidatorRegistry`, `ValidationOptions`, and `ValidationResult`.
- **Go validator**: SHACL-inspired Go-based validator (`go_validator.go`, `field_validators.go`, `type_validator.go`) used by default.
- **ID validator**: Validates object IDs against configurable prefixes and patterns; loads config from file or graph (`id_validator*.go`, `id_prefixes_config.go`, `namespaces_config.go`).
- **Instance validator**: Lifecycle and state transition validation (`instance_validator.go`).
- **Async validator**: Queue-based validation with workers, cache, and configurable timeouts (`async_validator*.go`, `validation_timeout_config.go`).
- **Tier and timeout config**: Validation tiers and per-object timeouts via `.zqk/config` (`validation_tier_config.go`, `validation_timeout_config.go`).
- **Ontology and namespace**: Registry and discovery for ontologies and namespaces (`ontology_registry.go`, `namespace_registry.go`, `namespace_discovery.go`).

## Usage

```go
import "github.com/zqk-os/zqk/pkg/validation"

// Sync validation via registry
registry := validation.NewValidatorRegistry()
v, _ := registry.Get("go")
result, err := v.Validate(ctx, obj, kind, validation.DefaultValidationOptions())

// ID validation (global instance)
idVal := validation.GetIDValidator()
err := idVal.ValidateID(ctx, id, kind, nil)

// Async validation (enqueue and process via scheduler/coordinator)
asyncVal := validation.GetAsyncValidator()
asyncVal.Enqueue(ctx, task)
```

## Validation patterns

- **Pluggable validators**: Implement `Validator` and register via `ValidatorRegistry`; use `ValidationOptions` and `ValidationResult` for consistent results.
- **Shared field helpers** (`field_validators.go`): Use `ValidateRequiredField` (required + optional empty-collection check) and `ValidateEnumField` (allowed values, case-insensitive for strings) so all validators behave consistently.
- **Sync vs async**: Use sync validation (registry + `Validate`) for inline checks (e.g. CLI, single object); use async validator (`Enqueue` + workers) for batch or storage-layer validation with timeouts and caching.
- **Instance + ID**: Instance validator handles lifecycle/state; ID validator handles prefixes/patterns from config. Both are used by storage on write.

## Configuration

- **ID prefixes and namespaces**: `.zqk/config/id_prefixes.yaml`, namespaces config; see `id_prefixes_config.go`, `namespaces_config.go`.
- **Validation tiers**: `validation_tier_config.go`; tier overrides per kind.
- **Per-object timeout**: `.zqk/config/config.yaml` `validation.per_object_timeout` (default_seconds **5**, empty kind_overrides by default — fail-fast); see `validation_timeout_config.go`. `stuck_timeout_seconds` default **30**.
- **Async validator timeouts and buffer**: `AsyncValidatorConfig` in `async_validator_config.go` — `WorkerStopTimeout` (default 60s), `CacheSaveTimeout` (default 20s), `ProgressChannelSize` (default 10000). Pass as optional last arg to `NewAsyncValidator(..., opts...)` to override.

## Related

- [pkg/storage](../storage/README.md) — Storage layer that uses validation on write.
- [pkg/context](../context/) — Context and pipeline used during validation.
- [.zqk/process/object_specs](../../.zqk/process/) — Object specs that drive validation.
