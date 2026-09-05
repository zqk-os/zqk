# Subordinate Namespaces Extension

**Last Verified:** 2026-08-31


**Date**: 2026-01-05  
**Status**: Proposal

## Overview

Extend the existing `namespace_id` system to support **subordinate namespaces** that enable modular organization within each namespace layer. This allows each namespace module (cli, metrics, storage, etc.) to have its own subordinate namespace ID.

## Current Namespace ID Format

Currently supports hierarchical format:
```
{layer}:{domain}:{subdomain}
```

**Examples**:
- `zqk:kernel` - zqk kernel
- `domain:organizational` - Organizational domain
- `domain:financial:accounting` - Financial accounting subdomain
- `integration:github:actions` - GitHub Actions integration

## Extended Format for Subordinate Namespaces

Extend to support unlimited depth:
```
{layer}:{domain}:{subdomain}:{subordinate}:{subordinate}:...
```

**New Examples**:
- `zqk:kernel:cli` - CLI namespace under kernel
- `zqk:kernel:metrics` - Metrics namespace under kernel
- `zqk:kernel:storage` - Storage namespace under kernel
- `zqk:kernel:cli:profiles` - Profiles subordinate under CLI
- `domain:organizational:hr` - HR subdomain
- `domain:organizational:hr:recruiting` - Recruiting under HR

## Use Cases

### 1. Module Organization

Each code module can have its own subordinate namespace:

```
zqk:kernel:cli          # CLI module
zqk:kernel:metrics      # Metrics module
zqk:kernel:storage      # Storage module
zqk:kernel:scheduler    # Scheduler module
```

### 2. Profile Organization

Profiles can be organized by subordinate namespace:

```yaml
# pkg/cli/profiles/base.yaml
namespace_id: "zqk:kernel:cli"
name: "base"
```

```yaml
# .zqk/metrics/profiles/base_sampler.yaml
namespace_id: "zqk:kernel:metrics"
name: "base_sampler"
```

### 3. Configuration Organization

Config files can be scoped to subordinate namespaces:

```yaml
# .zqk/config/cli.yaml
namespace_id: "zqk:kernel:cli"
profiles:
  - ai_agent
  - human
```

```yaml
# .zqk/config/metrics.yaml
namespace_id: "zqk:kernel:metrics"
profiles:
  - high_frequency
  - audit_events
```

### 4. Spec Organization

Object specs can be organized by subordinate namespace:

```
docs/process/_internal/object_specs/
├── cli/              # zqk:kernel:cli
│   └── profile.yaml
├── metrics/          # zqk:kernel:metrics
│   └── sampler_profile.yaml
└── storage/          # zqk:kernel:storage
    └── storage_profile.yaml
```

## Implementation

### Update Namespace ID Validation

Current pattern:
```regex
^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$
```

This already supports unlimited depth! Just need to document and use it.

### Update Profile Schema

Add `namespace_id` to profile definitions:

```yaml
# pkg/cli/profiles/base.yaml
schema_version: "1.0.0"
namespace_id: "zqk:kernel:cli"
name: "base"
extends: null
spec:
  format: "table"
```

```yaml
# .zqk/metrics/profiles/base_sampler.yaml
schema_version: "1.0.0"
namespace_id: "zqk:kernel:metrics"
name: "base_sampler"
extends: null
spec:
  batch_size: 50
```

### Namespace Registry Updates

Extend namespace registry to support subordinate namespace discovery:

```go
// GetSubordinateNamespaces returns all subordinate namespaces under a parent
func (r *NamespaceRegistry) GetSubordinateNamespaces(parentNamespaceID string) []string {
    // Returns all namespaces that start with parentNamespaceID + ":"
}

// IsSubordinateOf checks if a namespace is subordinate to another
func (r *NamespaceRegistry) IsSubordinateOf(child, parent string) bool {
    return strings.HasPrefix(child, parent+":")
}
```

### Profile Loader Updates

Update profile loaders to validate namespace_id:

```go
// pkg/cli/loader.go
func (pl *ProfileLoader) LoadProfile(name string) (*Profile, error) {
    profile, err := pl.loadProfileRecursive(name, visited)
    if err != nil {
        return nil, err
    }
    
    // Validate namespace_id matches expected subordinate namespace
    if profile.NamespaceID != "zqk:kernel:cli" {
        return nil, fmt.Errorf("profile %s has invalid namespace_id: %s (expected zqk:kernel:cli)", 
            name, profile.NamespaceID)
    }
    
    return profile, nil
}
```

## Benefits

1. **Clear Module Boundaries**: Each module has its own namespace ID
2. **Hierarchical Organization**: Supports unlimited nesting
3. **Namespace Discovery**: Can discover all profiles/configs for a module
4. **Isolation**: Each module's resources are clearly scoped
5. **Consistency**: Uses existing namespace_id system

## Migration Path

1. **Update Profile Schemas**: Add `namespace_id` field to all profiles
2. **Update Loaders**: Validate namespace_id matches expected subordinate namespace
3. **Update Registry**: Add subordinate namespace discovery methods
4. **Update Documentation**: Document subordinate namespace usage

## Examples

### CLI Namespace Profiles

```yaml
# pkg/cli/profiles/base.yaml
namespace_id: "zqk:kernel:cli"
name: "base"
```

```yaml
# pkg/cli/profiles/ai_agent.yaml
namespace_id: "zqk:kernel:cli"
name: "ai_agent"
extends: "base"
```

### Metrics Namespace Profiles

```yaml
# .zqk/metrics/profiles/base_sampler.yaml
namespace_id: "zqk:kernel:metrics"
name: "base_sampler"
```

```yaml
# .zqk/metrics/profiles/high_frequency.yaml
namespace_id: "zqk:kernel:metrics"
name: "high_frequency"
extends: "base_sampler"
```

### Domain Subordinate Namespaces

```yaml
# domain:organizational:hr namespace
namespace_id: "domain:organizational:hr"
layer: "domain"
domain: "organizational"
subdomain: "hr"
```

```yaml
# domain:organizational:hr:recruiting namespace
namespace_id: "domain:organizational:hr:recruiting"
layer: "domain"
domain: "organizational"
subdomain: "hr:recruiting"
```

