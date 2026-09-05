# Profile Systems Comparison

**Last Verified:** 2026-08-31


**Date**: 2026-01-05  
**Status**: Analysis

## Overview

zqk has two profile systems serving different purposes:

1. **CLI Context Profiles** - For CLI output formatting
2. **Sampler Profiles** - For metrics sampling configuration

## CLI Context Profiles

**Location**: `pkg/cli/profiles/` (not `docs/process/_internal/profile_specs/`, which holds transceiver router profiles only)

**Purpose**: Configure CLI output format, verbosity, storage settings

**Structure**: Simple YAML files (not full object specs)
```yaml
schema_version: "1.0.0"
ontology: "ai_agent"
extends: base_profile
format: json
verbose: false
```

**Examples**:
- `ai_agent.yaml` - JSON output for AI agents
- `human.yaml` - Table output for humans
- `mcp.yaml` - JSON output for MCP server

**Loader**: `ProfileLoader` in `internal/cli/context/profile_loader.go`

**Usage**: `zqk command --context ai-agent`

## Sampler Profiles

**Location**: `docs/process/_internal/object_specs/`

**Purpose**: Configure metrics sampling (batching, flush intervals)

**Structure**: Full object specs (extend `base_sampler`, which extends `base_object`)
```yaml
extends: base_sampler
fields:
    profile_name: ...
    applies_to: ...
    batch_size: ...
```

**Examples**:
- `sampler_profile.yaml` - Base sampler profile spec
- `scalar_metric_sampler.yaml` - For scalar metrics
- `list_metric_sampler.yaml` - For list metrics

**Loader**: Object storage system (can be created as objects)

**Usage**: Created as objects or via config file

## Key Differences

| Aspect | CLI Profiles | Sampler Profiles |
|--------|-------------|------------------|
| **Domain** | CLI output formatting | Metrics sampling |
| **Structure** | Simple YAML | Full object spec |
| **Inheritance** | `extends: base_profile` | `extends: base_sampler` (which extends `base_object`) |
| **Storage** | YAML files | System objects (or config file) |
| **Fields** | `format`, `verbose`, `quiet`, `flags`, `storage` | `batch_size`, `flush_interval`, `applies_to` |
| **Lifecycle** | Static files | Can be created/updated as objects |
| **Validation** | Basic YAML parsing | Full object validation |

## Integration Options

### Option 1: Keep Separate (Current)
- ✅ Different domains, different purposes
- ✅ Different structures (simple YAML vs object specs)
- ✅ No confusion between CLI and metrics concerns

### Option 2: Unified Profile System
- Create a `base_profile` object spec
- CLI profiles become objects too
- Sampler profiles extend the same base
- More consistent, but might be overkill for CLI profiles

### Option 3: Align Naming/Structure
- Keep separate but use consistent patterns
- Both use "profile" terminology
- Both support inheritance
- Both can be versioned

## Recommendation

**Keep them separate** - They serve different purposes:
- CLI profiles are lightweight configuration for output formatting
- Sampler profiles are domain objects for metrics configuration

However, we should:
1. ✅ Use consistent naming (both use "profile")
2. ✅ Document the distinction clearly
3. ✅ Consider if sampler profiles should be simpler YAML files (like CLI profiles) or full objects

## Questions

1. Should sampler profiles be full object specs (current) or simple YAML files (like CLI profiles)?
2. Should we create a unified `base_profile` object spec that both extend?
3. Should sampler profiles be stored as objects or just config files?

