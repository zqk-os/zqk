> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Feature Flags System

**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active

## Overview

Feature flags allow toggling experimental features on/off without code changes. This enables:
- Gradual rollout of new features
- Easy rollback if issues occur
- A/B testing and comparison
- Safe experimentation

## Available Feature Flags

### async_validation
- **Default**: `disabled`
- **Description**: Use async validator for system check (experimental)
- **Impact**: When enabled, `zqk system check` uses async validation instead of synchronous
- **Status**: Experimental - use with caution

### async_validation_parallel
- **Default**: `disabled`
- **Description**: Enable parallel validation in async mode
- **Impact**: Increases parallelism in async validation
- **Status**: Experimental

### deferred_hash_updates
- **Default**: `enabled`
- **Description**: Defer hash updates until all operations complete
- **Impact**: Hashes are only updated after all pending operations finish
- **Status**: Stable

### storage_orchestration
- **Default**: `enabled`
- **Description**: Use storage orchestrator for multi-backend coordination
- **Impact**: Coordinates operations across multiple storage backends
- **Status**: Stable

## Usage

### List All Flags
```bash
zqk system feature-flags list (PRUNED)
```

### Get a Specific Flag
```bash
zqk system feature-flags get async_validation (PRUNED)
```

### Enable a Flag
```bash
zqk system feature-flags enable async_validation (PRUNED)
```

### Disable a Flag
```bash
zqk system feature-flags disable async_validation (PRUNED)
```

## Toggling Async Validation

### Enable Async Validation
```bash
# Enable the flag
zqk system feature-flags enable async_validation (PRUNED)

# Run check (will use async validation)
zqk system check
```

### Disable Async Validation (Use Sync)
```bash
# Disable the flag
zqk system feature-flags disable async_validation (PRUNED)

# Run check (will use sync validation)
zqk system check
```

## Comparison Workflow

1. **Collect Baseline** (with sync):
   ```bash
   zqk system feature-flags disable async_validation (PRUNED)
   zqk system check-baseline --baseline-output baseline-sync (PRUNED).json
   ```

2. **Enable Async**:
   ```bash
   zqk system feature-flags enable async_validation (PRUNED)
   ```

3. **Run Check** (with async):
   ```bash
   zqk system check
   ```

4. **Compare Results**:
   ```bash
   zqk system check-async-baseline --baseline-file baseline-sync (PRUNED).json --comparison-output comparison.json
   ```

5. **Toggle Back if Needed**:
   ```bash
   zqk system feature-flags disable async_validation (PRUNED)
   ```

## Storage

Feature flags are stored in: `.zqk/feature_flags.json`

This file is:
- Version controlled (optional)
- Project-specific
- Human-readable JSON
- Automatically created on first use

## Safety

- **Default to Safe**: All experimental flags default to `disabled`
- **Easy Rollback**: Simply disable the flag to revert
- **No Code Changes**: Toggle without rebuilding
- **Project-Specific**: Each project has its own flags

## Best Practices

1. **Test in Isolation**: Test new features with flags enabled in test environments first
2. **Monitor Performance**: Compare metrics before/after enabling flags
3. **Gradual Rollout**: Enable for specific use cases before full rollout
4. **Document Changes**: Document which flags are enabled and why
5. **Regular Review**: Periodically review enabled flags and their status

