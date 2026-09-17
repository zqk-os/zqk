# Unified Profile System

**Last Verified:** 2026-08-31


**Date**: 2026-01-05  
**Status**: Implemented

## Overview

The Unified Profile System provides a common schema core for all profile types in zqk while maintaining functional separation between different domains (CLI context, metrics sampling, etc.).

## Architecture

### Shared Header Structure

All profiles use a unified header structure:

```yaml
# Shared Header
schema_version: "1.0.0"
kind: profile
type: cli_context | metrics_sampler
metadata:
  name: "profile_name"
  extends: "parent_profile"  # Optional
  description: "Profile description"
```

### Domain-Specific Spec

Each profile type has its own `spec` block with domain-specific configuration:

**CLI Context Profile:**
```yaml
spec:
  format: "json"
  verbose: false
  quiet: false
  flags: {}
  commands: {}
  storage: {}
```

**Metrics Sampler Profile:**
```yaml
spec:
  enabled: true
  batch_size: 50
  max_batch_size: 1000
  flush_interval: "5m"
  group_by_object_id: true
  applies_to: ["object_kind1", "object_kind2"]
```

## Implementation

### Core Components

1. **`pkg/config/profile.go`** - Unified profile loader
   - `UnifiedProfile` struct
   - `ProfileLoader` with inheritance support
   - Type validation and routing

2. **`internal/cli/context/profile_loader.go`** - CLI adapter
   - Wraps unified loader
   - Converts `UnifiedProfile` to `ProfileSpec`
   - Maintains backward compatibility

3. **`pkg/metrics/config_loader.go`** - Metrics adapter
   - Loads sampler profiles from unified system
   - Applies profiles to sampler registry
   - Supports both profile references and inline profiles

### Profile Locations

- **CLI Context Profiles**: `pkg/cli/profiles/` (`paths.CLIProfilesDir`)
  - `base_profile.yaml`, `ai_agent.yaml`, `human.yaml`, `mcp.yaml`, `debug.yaml`, `pagination.yaml`, `quiet.yaml`

- **Metrics Sampler Profiles**: `.zqk/metrics/profiles/` (`paths.MetricsProfilesDir`) — optional; none ship in-tree

- **Transceiver Router Profiles**: `.zqk/specs/profiles/` (`paths.ProcessInternalProfileSpecsDir`)
  - `base_router.yaml`, `default_router.yaml`, `high_throughput_router.yaml`, `low_latency_router.yaml`

### Configuration Files

- **`metrics_sampler_config.yaml`** - References profiles by name:
  ```yaml
  profile_refs:
    - high_frequency
    - audit_events
    - low_frequency
    - default
  ```

## Benefits

1. **Unified Validation**: Single parser validates `kind`, `type`, and `metadata`
2. **Consistent Inheritance**: Both profile types use the same `extends` mechanism
3. **Type Safety**: Type validation prevents mixing profile types
4. **Simplified Maintenance**: Shared loader logic, domain-specific specs
5. **Clear Intent**: `kind: profile` and `type` make purpose explicit

## Migration

### CLI Profiles
- ✅ Migrated to unified format
- ✅ Maintain backward compatibility via adapter
- ✅ All existing profiles updated

### Sampler Profiles
- Optional YAML under `.zqk/metrics/profiles/` (not shipped; not the `profile_specs/` directory)
- Config file may reference profiles by name when present

## Usage

### Loading CLI Context Profiles

```go
loader := context.NewProfileLoader("")
spec, err := loader.LoadProfileWithInheritance("ai_agent.yaml")
// Returns ProfileSpec with resolved values
```

### Loading Metrics Sampler Profiles

```go
loader := config.NewProfileLoader("")
profile, err := loader.LoadProfileWithInheritance("high_frequency")
// Returns UnifiedProfile with resolved spec
```

### Applying Sampler Profiles

```go
configFile, _ := LoadSamplerConfig(configPath)
ApplySamplerConfig(registry, configFile, projectRoot)
// Loads profiles from profile_refs and applies to registry
```

## Future Enhancements

1. **Additional Profile Types**: Easy to add new types (e.g., `logging_profile`, `cache_profile`)
2. **Profile Validation**: Schema validation for each profile type
3. **Profile Discovery**: Auto-discovery of available profiles
4. **Profile Composition**: Multiple inheritance or mixins

