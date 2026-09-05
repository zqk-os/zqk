# Namespace-Based Modular Architecture

**Last Verified:** 2026-08-31


**Date**: 2026-01-05  
**Status**: Proposal

## Overview

Organize by **namespace** where each namespace (`cli`, `metrics`, `storage`, etc.) is fully self-contained with its own:
- Profile definitions
- Spec loaders
- Spec writers
- Configuration systems
- Validation logic

## Architecture

### Namespace Structure

Each namespace becomes a self-contained module:

```
pkg/
├── cli/
│   ├── profiles/          # CLI-specific profiles
│   │   ├── base.yaml
│   │   ├── ai_agent.yaml
│   │   ├── human.yaml
│   │   └── ...
│   ├── loader.go          # CLI profile loader
│   └── config.go          # CLI configuration
│
├── metrics/
│   ├── profiles/          # Metrics-specific profiles
│   │   ├── base_sampler.yaml
│   │   ├── high_frequency.yaml
│   │   └── ...
│   ├── loader.go          # Metrics profile loader
│   └── config.go          # Metrics configuration
│
├── storage/
│   ├── profiles/          # Storage-specific profiles (if needed)
│   ├── loader.go
│   └── config.go
│
└── config/                # Shared utilities (optional, minimal)
    └── interfaces.go      # Common interfaces only (no implementations)
```

### Benefits

1. **Full Modularity**: Each namespace is independent
2. **Clear Boundaries**: No cross-domain dependencies
3. **Independent Evolution**: Each namespace can evolve its own conventions
4. **Easier Testing**: Test each namespace in isolation
5. **Better Organization**: Related code stays together
6. **No Shared Bottlenecks**: Each namespace can optimize independently

### Namespace Responsibilities

#### CLI Namespace (`pkg/cli/`)
- **Profiles**: CLI context profiles (format, verbose, quiet, storage)
- **Loader**: Loads and resolves CLI profiles with inheritance
- **Config**: CLI-specific configuration (user config, project config)
- **No dependencies** on metrics, storage, etc.

#### Metrics Namespace (`pkg/metrics/`)
- **Profiles**: Sampler profiles (batch_size, flush_interval, applies_to)
- **Loader**: Loads and resolves metrics profiles
- **Config**: Metrics configuration (sampler config, aggregation config)
- **No dependencies** on CLI, storage, etc.

#### Storage Namespace (`pkg/storage/`)
- **Profiles**: Storage profiles (if needed)
- **Loader**: Storage-specific loaders
- **Config**: Storage configuration
- **No dependencies** on CLI, metrics, etc.

### Common Interfaces (Minimal)

If needed, define minimal interfaces in `pkg/config/`:

```go
// pkg/config/interfaces.go
package config

// Profile represents a loaded profile (minimal interface)
type Profile interface {
    Name() string
    Namespace() string
    Spec() map[string]any
}
```

But **avoid** shared implementations - each namespace implements its own.

### Profile Schema (Per Namespace)

Each namespace can define its own profile schema:

**CLI Profile:**
```yaml
# pkg/cli/profiles/base.yaml
schema_version: "1.0.0"
namespace: "cli"
name: "base"
spec:
  format: "table"
  verbose: false
  quiet: false
```

**Metrics Profile:**
```yaml
# .zqk/metrics/profiles/base_sampler.yaml
schema_version: "1.0.0"
namespace: "metrics"
name: "base_sampler"
spec:
  batch_size: 50
  flush_interval: "5m"
  enabled: true
```

### Migration Path

1. ✅ **Create namespace directories** (`pkg/cli/profiles/`; `.zqk/metrics/profiles/` is optional at runtime)
2. ✅ **CLI profiles** live in `pkg/cli/profiles/` — shadow copies removed from `docs/process/_internal/profile_specs/`
3. ✅ **Metrics sampler YAML** is not shipped under `profile_specs/` — loader looks at `.zqk/metrics/profiles/`
4. **Create namespace-specific loaders** in each namespace
5. **Remove unified loader** from `pkg/config/` (or keep minimal interfaces only) — still used for transceiver router profiles in `profile_specs/`
6. **Update references** to use namespace-specific loaders

### Example: CLI Namespace Loader

```go
// pkg/cli/loader.go
package cli

type Profile struct {
    SchemaVersion string
    Namespace     string
    Name          string
    Extends       string
    Spec          map[string]any
    ResolvedSpec  map[string]any // After inheritance
}

type ProfileLoader struct {
    profilesDir string
    cache       map[string]*Profile
    mu          sync.RWMutex
}

func (pl *ProfileLoader) LoadProfile(name string) (*Profile, error) {
    // CLI-specific loading logic
    // No knowledge of metrics profiles
}
```

### Example: Metrics Namespace Loader

```go
// pkg/metrics/loader.go
package metrics

type SamplerProfile struct {
    SchemaVersion string
    Namespace     string
    Name          string
    Extends       string
    Spec          map[string]any
    ResolvedSpec  map[string]any
}

type ProfileLoader struct {
    profilesDir string
    cache       map[string]*SamplerProfile
    mu          sync.RWMutex
}

func (pl *ProfileLoader) LoadProfile(name string) (*SamplerProfile, error) {
    // Metrics-specific loading logic
    // No knowledge of CLI profiles
}
```

## Comparison

### Current (Unified)
- ✅ Single loader for all profiles
- ❌ Tight coupling between domains
- ❌ Harder to evolve independently
- ❌ Shared code can become a bottleneck
- ❌ Type validation at runtime

### Proposed (Namespace-Based)
- ✅ Each namespace is independent
- ✅ Clear boundaries
- ✅ Easy to test and maintain
- ✅ Can evolve independently
- ✅ Compile-time type safety
- ❌ Some code duplication (but that's OK - it's intentional separation)

## Decision

**Recommendation**: Adopt namespace-based architecture for better modularity and independence.
