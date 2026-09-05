# SpecBuilder Migration Guide

**Last Verified:** 2026-08-31


## Quick Reference

### Safety Principles
1. **Isolation**: New code in `pkg/specbuilder/`, existing code unchanged
2. **Adapter Pattern**: Bridge old and new implementations
3. **Dual Path**: Support both old and new paths during migration
4. **Test Coverage**: Comprehensive tests before integration
5. **Rollback Safety**: Can revert without breaking existing code

### Current Status

**Phase 1: Foundation** ✅ Complete
- Core infrastructure created
- Isolated tests passing
- Pattern validated

**Phase 2: Adapter Layer** ✅ Complete
- Adapters created for existing builders
- Output equivalence verified
- All adapter tests passing

**Phase 3: Parallel Implementation** ✅ Complete
- New `specbuilder/generators.ScenarioGenerator` created
- Both implementations coexist safely
- Parallel tests validating compatibility

**Phase 4: Gradual Migration** ✅ Complete
- Migration guide and documentation complete
- New generator adopted as standard
- Old implementation deprecated but available for backward compatibility

**Phase 5: Cleanup** ⏳ Future
- Remove old implementation after full migration
- Clean up adapter layer if no longer needed

## ScenarioGenerator Migration Guide

The `ScenarioGenerator` has been migrated to use the specbuilder infrastructure. Both implementations are API-compatible and can coexist safely.

### API Compatibility

Both generators have identical APIs:

```go
// Old: pkg/mcp/testing
generator := testing.NewScenarioGenerator(outputDir)
err := generator.GenerateFromFile(specFile)
err := generator.GenerateFromSpecs(specs)

// New: pkg/specbuilder/generators
generator := generators.NewScenarioGenerator(outputDir)
err := generator.GenerateFromFile(specFile)
err := generator.GenerateFromSpecs(specs)
```

The only difference is the package path. Migration is as simple as changing the import!

## Migration Steps for Test Scenarios

### Step 1: Update Import Statement

Change the import from:
```go
import (
    "github.com/lanceman/zqk/pkg/mcp/testing"
)
```

To:
```go
import (
    mcptesting "github.com/lanceman/zqk/pkg/mcp/testing"
    "github.com/lanceman/zqk/pkg/specbuilder/generators"
)
```

### Step 2: Replace Generator Creation

Change from:
```go
// Old
generator := testing.NewScenarioGenerator(outputDir)
```

To:
```go
// New
generator := generators.NewScenarioGenerator(outputDir)
```

### Step 3: Update Type References (if any)

If you stored the generator in a variable with explicit type:
```go
// Old
var generator *testing.ScenarioGenerator = testing.NewScenarioGenerator(outputDir)

// New
var generator *generators.ScenarioGenerator = generators.NewScenarioGenerator(outputDir)
```

Note: The `ScenarioSpec` type remains in `pkg/mcp/testing`, so you'll still need that import for spec types.

### Step 4: Test Your Changes

```bash
# Run your tests
go test ./your/package/... -v

# Verify output is equivalent (if applicable)
# Both generators produce identical output
```

### Complete Migration Example

**Before:**
```go
package mypackage

import (
    "github.com/lanceman/zqk/pkg/mcp/testing"
)

func GenerateScenarios(outputDir string, specFile string) error {
    generator := testing.NewScenarioGenerator(outputDir)
    return generator.GenerateFromFile(specFile)
}
```

**After:**
```go
package mypackage

import (
    "github.com/lanceman/zqk/pkg/specbuilder/generators"
)

func GenerateScenarios(outputDir string, specFile string) error {
    generator := generators.NewScenarioGenerator(outputDir)
    return generator.GenerateFromFile(specFile)
}
```

### Convenience Function

The new generator also provides a convenience function for simple use cases:

```go
import "github.com/lanceman/zqk/pkg/specbuilder/generators"

// One-liner for file-based generation
err := generators.GenerateScenariosFromSpecFile(specFile, outputDir)
```

### Coexistence Strategy

Both implementations can safely coexist in the same codebase:

- ✅ **Safe**: Both use the same underlying builder API
- ✅ **Tested**: Output equivalence validated in tests
- ✅ **Compatible**: Identical APIs, drop-in replacement
- ✅ **Flexible**: Migrate gradually, one file at a time

You can migrate code incrementally without risk to existing functionality.

## Rollback Procedure

### If Issues Occur

1. **Immediate**: Use existing implementation
2. **Short-term**: Keep both implementations
3. **Long-term**: Fix issues, retry migration

### Git Rollback

```bash
# Revert specific commit
git revert <commit-hash>

# Or checkout previous version
git checkout <previous-commit>
```

## Migration Checklist

For each file/module you migrate:

- [ ] Update import statement to use `specbuilder/generators`
- [ ] Replace `testing.NewScenarioGenerator` with `generators.NewScenarioGenerator`
- [ ] Update any explicit type declarations (if applicable)
- [ ] Run tests to verify functionality
- [ ] Verify output files are generated correctly (if applicable)
- [ ] Update any related documentation/comments

## Testing Checklist

Before migrating any domain:

- [x] All existing tests pass
- [x] New infrastructure tests pass
- [x] Output equivalence verified (Phase 2)
- [x] Parallel implementation tested (Phase 3)
- [x] API compatibility verified
- [x] Migration documentation complete
- [ ] Code review complete

## Benefits of Migration

Migrating to `specbuilder/generators.ScenarioGenerator` provides:

1. **Future-Proof Infrastructure**: Built on the reusable specbuilder pattern
2. **Consistency**: Uses the same infrastructure as other spec-driven builders
3. **Extensibility**: Easy to extend with new features (validation, transformations, etc.)
4. **Maintainability**: Centralized generator logic in the specbuilder core
5. **No Breaking Changes**: API-compatible, drop-in replacement

## Migration Summary

### What Changed
- Package path: `pkg/mcp/testing` → `pkg/specbuilder/generators`
- Implementation: Now uses specbuilder core infrastructure
- Internal architecture: Uses adapters to bridge to existing builders

### What Stayed the Same
- Public API: Identical method signatures
- Input/Output: Same spec format, same output format
- Behavior: Output equivalence validated and tested
- Type compatibility: `ScenarioSpec` types remain in `pkg/mcp/testing`

### Migration Effort
- **Difficulty**: ⭐ Easy (just change imports)
- **Time**: < 5 minutes per file
- **Risk**: ⭐⭐ Low (API-compatible, fully tested)

## Questions?

See `docs/process/architecture/SPECBUILDER_SAFETY_PLAN.md` for detailed safety measures.
