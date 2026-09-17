# Feature Flags Usage Guide

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active

## Quick Start

### Toggle Async Validation

**Enable Async Validation:**
```bash
zqk system feature-flags enable async_validation (PRUNED)
zqk system check
```

**Disable Async Validation (Use Sync):**
```bash
zqk system feature-flags disable async_validation (PRUNED)
zqk system check
```

**Check Current Status:**
```bash
zqk system feature-flags get async_validation (PRUNED)
```

## Comparison Workflow

### Step 1: Baseline with Sync
```bash
# Ensure sync is enabled (default)
zqk system feature-flags disable async_validation (PRUNED)

# Collect baseline
zqk system check-baseline --baseline-output baseline-sync (PRUNED).json
```

### Step 2: Test with Async
```bash
# Enable async
zqk system feature-flags enable async_validation (PRUNED)

# Run check (uses async)
zqk system check

# Compare with baseline
zqk system check-async-baseline --baseline-file baseline-sync (PRUNED).json --comparison-output comparison.json
```

### Step 3: Review Results
```bash
# View comparison
cat .zqk/comparison.json | jq .

# Check correctness score
cat .zqk/comparison.json | jq .correctness.correctness_score
```

### Step 4: Toggle Back if Needed
```bash
# If async has issues, toggle back to sync
zqk system feature-flags disable async_validation (PRUNED)
zqk system check
```

## Feature Flag Management

### List All Flags
```bash
zqk system feature-flags list (PRUNED)
```

### Get Specific Flag
```bash
zqk system feature-flags get async_validation (PRUNED)
```

### Enable Flag
```bash
zqk system feature-flags enable (PRUNED) <flag-name>
```

### Disable Flag
```bash
zqk system feature-flags disable (PRUNED) <flag-name>
```

## Available Flags

| Flag | Default | Description |
|------|---------|-------------|
| `async_validation` | disabled | Use async validator for system check |
| `async_validation_parallel` | disabled | Enable parallel validation in async mode |
| `deferred_hash_updates` | enabled | Defer hash updates until all operations complete |
| `storage_orchestration` | enabled | Use storage orchestrator for multi-backend coordination |

## Storage Location

Feature flags are stored in: `.zqk/feature_flags.json`

This file is:
- Project-specific
- Human-readable JSON
- Automatically created
- Version controlled (optional)

**Vocabulary:** This pattern is a **lite file** (bounded project-local JSON with CLI integration, not `.zqk/process/` CAS objects). Glossary: `GLS-1776253895684744000-7799fa3e` (`zqk object get GLS-1776253895684744000-7799fa3e`). Implementation: `pkg/featureflags`, `pkg/datacell.FeatureFlagsPath`.

## Safety Features

1. **Default to Safe**: Experimental flags default to `disabled`
2. **Easy Rollback**: Simply disable the flag
3. **No Code Changes**: Toggle without rebuilding
4. **Project-Specific**: Each project has independent flags

## Best Practices

1. **Test First**: Test new features in isolation before enabling
2. **Monitor Performance**: Compare metrics before/after
3. **Gradual Rollout**: Enable for specific use cases first
4. **Document Changes**: Note which flags are enabled and why
5. **Regular Review**: Periodically review enabled flags

## Troubleshooting

### Flag Not Taking Effect
- Ensure you're in the correct project directory
- Check flag status: `zqk system feature-flags get (PRUNED) <flag-name>`
- Verify flag file exists: `cat .zqk/feature_flags.json`

### Want to Reset All Flags
```bash
# Delete flag file to reset to defaults
rm .zqk/feature_flags.json
zqk system feature-flags list (PRUNED)  # Will recreate with defaults
```

### Flag File Corrupted
```bash
# Delete and recreate
rm .zqk/feature_flags.json
zqk system feature-flags list (PRUNED)  # Recreates with defaults
```

