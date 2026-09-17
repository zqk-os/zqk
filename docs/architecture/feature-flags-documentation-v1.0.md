> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Feature Flags Documentation

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Policy**: POL-FEATURE-001

## Overview

Feature flags allow toggling experimental and advanced features on/off without code changes. This enables:
- Gradual rollout of new features
- Easy rollback if issues occur
- A/B testing and comparison
- Safe experimentation

**⚠️ IMPORTANT**: Feature flags require **elevated privileges** to modify. Only users with `write:system` or `write:config` permissions may enable or disable flags.

## Available Feature Flags

### async_validation
- **Default**: `disabled`
- **Stability**: Experimental
- **Permission Required**: `write:system`
- **Description**: Use async validator for system check (experimental)
- **Impact**: 
  - When enabled, `zqk system check` uses async validation instead of synchronous
  - May improve performance for large object sets
  - Currently experimental - results may differ from sync validation
- **Use Cases**:
  - Testing async validation performance
  - Comparing sync vs async results
  - Large-scale validation scenarios
- **Known Issues**:
  - May not find all objects (integration in progress)
  - May not find all issues (validation logic integration needed)
  - Currently slower than sync (optimization in progress)
- **Migration Path**:
  1. Enable flag in test environment
  2. Run baseline comparison
  3. Verify correctness
  4. Enable in production if results match

### async_validation_parallel
- **Default**: `disabled`
- **Stability**: Experimental
- **Permission Required**: `write:system`
- **Description**: Enable parallel validation in async mode
- **Impact**: Increases parallelism in async validation
- **Use Cases**: Large-scale validation with high concurrency needs
- **Known Issues**: May increase resource usage

### deferred_hash_updates
- **Default**: `enabled`
- **Stability**: Stable
- **Permission Required**: `write:system`
- **Description**: Defer hash updates until all operations complete
- **Impact**: 
  - Hashes are only updated after all pending operations finish
  - Ensures hash integrity reflects final object state
  - Prevents intermediate hash updates
- **Use Cases**: All production environments
- **Known Issues**: None

### storage_orchestration
- **Default**: `enabled`
- **Stability**: Stable
- **Permission Required**: `write:system`
- **Description**: Use storage orchestrator for multi-backend coordination
- **Impact**: 
  - Coordinates operations across multiple storage backends
  - Prevents operations on objects with pending updates
  - Ensures consistency across backends
- **Use Cases**: 
  - Multi-backend environments (file + graph)
  - Production environments requiring consistency
- **Known Issues**: None

## Permission Requirements

### Required Permissions

To **enable** or **disable** feature flags, you must have one of:
- `write:system` - Full system write access
- `write:config` - Configuration write access

### Checking Permissions

```bash
# Check your current permissions
zqk system whoami (PRUNED)

# Verify you have required permissions
zqk system whoami --verbose (PRUNED)
```

### Permission Errors

If you don't have required permissions, you'll see:
```
Error: insufficient permissions: feature flag changes require write:system or write:config permission
```

## Usage

### List All Flags
```bash
zqk system feature-flags list (PRUNED)
```

### Get a Specific Flag
```bash
zqk system feature-flags get async_validation (PRUNED)
```

### Enable a Flag (Requires Elevated Privileges)
```bash
# Check permissions first
zqk system whoami (PRUNED)

# Enable flag (will check permissions)
zqk system feature-flags enable async_validation (PRUNED)
```

### Disable a Flag (Requires Elevated Privileges)
```bash
zqk system feature-flags disable async_validation (PRUNED)
```

## Security

### Privilege Checks
- All flag changes require permission verification
- Permission checks happen before any changes
- Failed permission checks prevent flag modification

### Audit Trail
- All flag changes create audit events
- Audit events include:
  - User account
  - Timestamp
  - Flag name
  - Previous value
  - New value
  - Reason (if provided)

### Viewing Audit Trail
```bash
# View recent feature flag changes
zqk audit list --filter "operation=feature_flag_change" --limit 50
```

## Best Practices

### 1. Test First
- Always test flags in non-production environments first
- Compare results with baseline metrics
- Verify correctness before production use

### 2. Document Changes
- Document why flags are enabled
- Note expected impact
- Record any issues encountered

### 3. Monitor Performance
- Compare metrics before/after enabling flags
- Monitor system health
- Watch for unexpected behavior

### 4. Gradual Rollout
- Enable for specific use cases first
- Monitor for issues
- Expand usage gradually

### 5. Regular Review
- Periodically review enabled flags
- Disable flags that are no longer needed
- Update documentation as needed

## Troubleshooting

### Permission Denied
**Problem**: Cannot enable/disable flags  
**Solution**: 
1. Check your permissions: `zqk system whoami` (PRUNED)
2. Request `write:system` or `write:config` permission
3. Contact system administrator

### Flag Not Taking Effect
**Problem**: Flag enabled but behavior unchanged  
**Solution**:
1. Verify flag status: `zqk system feature-flags get (PRUNED) <flag-name>`
2. Check flag file: `cat .zqk/config/feature_flags.json`
3. Restart CLI if needed
4. Check for errors in logs

### Flag File Corrupted
**Problem**: Flag file is invalid  
**Solution**:
```bash
# Delete and recreate
rm .zqk/config/feature_flags.json
zqk system feature-flags list (PRUNED)  # Recreates with defaults
```

## Related Documentation

- [Feature Flags Policy (POL-FEATURE-001)](../policies/POL-FEATURE-001.yaml)
- [Feature Flags Usage Guide](./feature-flags-usage-v1.0.md)
- [Feature Flags Architecture](./feature-flags-v1.0.md)

