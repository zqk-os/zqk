# Check Command Fast Mode

**Date:** 2025-12-25  
**Status:** Implemented

## Overview

The `zqk check` command now supports a `--fast` flag to skip reference integrity checking for faster execution. Reference checking is enabled by default to ensure thorough validation, but can be disabled when speed is more important.

## Usage

### Default Mode (Thorough)
```bash
# Full validation including reference integrity (slower, ~24 seconds)
zqk check all
```

### Fast Mode
```bash
# Skip reference integrity checking (faster, ~11 seconds)
zqk check all --fast
```

## Performance Comparison

| Mode | Time | Reference Checking | Issues Found |
|------|------|-------------------|--------------|
| **Default** | ~24s | ✅ Enabled | 243 total (38 blocking) |
| **Fast** | ~11s | ❌ Disabled | 207 total (7 blocking) |

### Performance Improvement
- **Fast mode**: ~54% faster (13 seconds saved)
- **Trade-off**: 7 reference integrity issues not detected

## When to Use Fast Mode

### Use `--fast` when:
- ✅ Quick health checks during development
- ✅ CI/CD pipelines where speed matters
- ✅ Large-scale checks where reference issues are less critical
- ✅ Iterative development cycles

### Use default mode when:
- ✅ Pre-commit validation
- ✅ Production health checks
- ✅ Release preparation
- ✅ Full compliance verification

## Implementation Details

### Flag Behavior
- `--check-refs` (default: `true`): Explicitly enable/disable reference checking
- `--fast`: Shorthand to disable reference checking (sets `--check-refs=false`)

### Logic
```go
fastMode, _ := cmd.Flags().GetBool("fast")
checkRefs, _ := cmd.Flags().GetBool("check-refs")
if checkRefs && !fastMode {
    // Perform reference integrity checking
}
```

## Reference Integrity Issues

When using `--fast`, the following issues are not detected:
- Broken references (e.g., `goal_refs` pointing to non-existent goals)
- Invalid milestone references
- Missing requirement references
- Orphaned relationships

These are typically **blocking issues** (Tier 1) that should be fixed before production.

## Recommendations

1. **Development**: Use `--fast` for quick iterations
2. **Pre-commit**: Use default mode to catch reference issues
3. **CI/CD**: Consider `--fast` for speed, but run full check before releases
4. **Production**: Always use default mode for thorough validation

## Future Enhancements

- Parallel reference checking for better performance
- Cached reference lookups
- Incremental reference checking (only check changed objects)
- Reference checking as separate command (`zqk check-refs`)

