# SpecBuilder Safety Plan

**Last Verified:** 2026-08-31


## Overview

This document outlines safety measures and migration strategies for introducing the SpecBuilder pattern across the codebase without corrupting existing functionality.

## Core Principles

1. **Isolation First**: New code is isolated from existing code
2. **Backwards Compatibility**: Existing code continues to work unchanged
3. **Incremental Adoption**: Gradual migration, not big bang
4. **Comprehensive Testing**: Extensive test coverage before integration
5. **Rollback Safety**: Changes can be reverted without breaking existing code

## Safety Measures

### 1. Package Isolation

**Strategy**: Keep specbuilder in its own package (`pkg/specbuilder/`) with no dependencies on existing code (except where explicitly needed).

```go
pkg/specbuilder/          # New, isolated package
pkg/mcp/testing/          # Existing code (unchanged initially)
```

**Benefits**:
- No risk of breaking existing code
- Clear boundaries
- Easy to test in isolation
- Can be removed if needed without affecting existing code

### 2. Adapter Pattern for Integration

**Strategy**: Use adapters to bridge between new specbuilder infrastructure and existing code.

```go
// Adapter wraps existing ScenarioBuilder to work with specbuilder core
type ScenarioBuilderAdapter struct {
    builder *testing.ScenarioBuilder
}

func (a *ScenarioBuilderAdapter) Build() *TestScenario {
    return a.builder.Build()  // Uses existing builder
}
```

**Benefits**:
- Existing code unchanged
- New infrastructure can use existing builders
- Gradual migration path
- Easy to test both paths

### 3. Dual-Path Implementation

**Strategy**: Support both old and new paths simultaneously during migration.

```go
// Option 1: Use existing generator (unchanged)
import "github.com/lanceman/zqk/pkg/mcp/testing"
generator := testing.NewScenarioGenerator(outputDir)

// Option 2: Use new specbuilder-based generator (new code)
import "github.com/lanceman/zqk/pkg/specbuilder/generators"
generator := generators.NewScenarioGenerator(outputDir)
```

**Benefits**:
- Both paths work during transition
- Can compare outputs
- Easy rollback
- No breaking changes

### 4. Comprehensive Test Coverage

**Strategy**: Extensive tests before integration, including:

- Unit tests for each component
- Integration tests for full workflows
- Comparison tests (old vs new outputs)
- Regression tests for existing functionality

**Test Strategy**:
```go
// Test new infrastructure in isolation
TestSpecBuilderIntegration

// Test that existing code still works
TestExistingScenarioGenerator

// Test that outputs are equivalent
TestOutputEquivalence
```

### 5. Feature Flag Strategy (Optional)

**Strategy**: Use feature flags to toggle between old and new implementations.

```go
// Feature flag (can be removed after migration)
var UseSpecBuilderInfrastructure = os.Getenv("USE_SPECBUILDER") == "true"

func NewScenarioGenerator(outputDir string) *ScenarioGenerator {
    if UseSpecBuilderInfrastructure {
        return NewSpecBuilderGenerator(outputDir)  // New path
    }
    return NewLegacyGenerator(outputDir)  // Existing path
}
```

**Benefits**:
- Easy to toggle behavior
- Can test in production with flag off
- Gradual rollout
- Quick rollback

### 6. Migration Checklist

**Phase 1: Foundation (Current)**
- [x] Create specbuilder core package
- [x] Create isolated tests
- [x] Validate pattern works
- [ ] Document migration strategy

**Phase 2: Adapter Layer**
- [ ] Create adapters for existing builders
- [ ] Test adapters work correctly
- [ ] Verify output equivalence

**Phase 3: Parallel Implementation**
- [ ] Add new path alongside existing path
- [ ] Run both paths and compare outputs
- [ ] Fix any discrepancies
- [ ] Document differences (if any)

**Phase 4: Gradual Migration**
- [ ] Migrate one domain at a time (start with test scenarios)
- [ ] Run comprehensive tests after each migration
- [ ] Update documentation
- [ ] Get code review

**Phase 5: Cleanup**
- [ ] Remove old implementation after validation
- [ ] Remove feature flags
- [ ] Update all references
- [ ] Final test suite run

### 7. Code Review Guidelines

**Review Checklist**:
- [ ] New code is in isolated package
- [ ] Existing code unchanged
- [ ] Tests pass for both old and new paths
- [ ] Output equivalence verified
- [ ] Documentation updated
- [ ] No breaking changes
- [ ] Rollback plan documented

### 8. Rollback Plan

**If Issues Arise**:

1. **Immediate**: Feature flag off (if used) or revert to old path
2. **Short-term**: Keep both implementations, use old one
3. **Long-term**: Fix issues in new code, retry migration

**Revert Process**:
```bash
# Git revert strategy
git revert <commit-hash>

# Or keep both and switch
USE_SPECBUILDER=false go test ./...
```

### 9. Output Validation

**Strategy**: Verify new infrastructure produces equivalent outputs.

```go
func TestOutputEquivalence(t *testing.T) {
    // Generate with old path
    oldGenerator := testing.NewScenarioGenerator(oldDir)
    oldOutput := generateScenarios(oldGenerator)
    
    // Generate with new path
    newGenerator := specbuilder.NewTestScenarioGenerator(newDir)
    newOutput := generateScenarios(newGenerator)
    
    // Compare (should be equivalent)
    if !scenariosEquivalent(oldOutput, newOutput) {
        t.Errorf("Outputs are not equivalent")
    }
}
```

### 10. Documentation Requirements

**Documentation Checklist**:
- [ ] Architecture documentation
- [ ] Migration guide
- [ ] API documentation
- [ ] Examples and usage
- [ ] Safety measures documented
- [ ] Rollback procedures documented

## Migration Strategy by Domain

### Test Scenarios (First Domain)

**Approach**: Gradual migration with adapter pattern

1. ✅ Keep existing `testing.ScenarioGenerator` unchanged
2. ✅ Create `specbuilder/generators.ScenarioGenerator` using core
3. ✅ Use adapter to bridge existing builder with new infrastructure
4. ✅ Test both paths produce equivalent output
5. 🟡 Migrate usage gradually (Phase 4 - In Progress)
6. ⏳ Remove old implementation after validation (Phase 5 - Future)

**Status**: Phases 1-3 complete. Phase 4 (Gradual Migration) in progress. See `pkg/specbuilder/MIGRATION_GUIDE.md` for migration instructions.

### Future Domains

**Approach**: Use specbuilder from the start

- New domains use specbuilder core directly
- No migration needed
- Clear pattern established
- Faster development

## Safety Guarantees

### What We Guarantee

1. **No Breaking Changes**: Existing code continues to work
2. **Isolated Changes**: New code is in separate package
3. **Test Coverage**: Comprehensive tests before integration
4. **Rollback Safety**: Can revert without breaking existing code
5. **Output Equivalence**: New path produces equivalent outputs

### What We Monitor

1. **Test Coverage**: Ensure >90% coverage for new code
2. **Performance**: New path should not be significantly slower
3. **Output Quality**: Generated outputs should be equivalent or better
4. **Code Complexity**: Keep complexity manageable
5. **Documentation**: Keep docs up to date

## Emergency Procedures

### If Tests Fail

1. **Immediate**: Don't merge/commit
2. **Investigate**: Understand why tests failed
3. **Fix**: Address root cause
4. **Verify**: All tests pass before proceeding

### If Output Differs

1. **Document**: Capture differences
2. **Analyze**: Determine if differences are expected
3. **Fix**: Adjust implementation if needed
4. **Validate**: Verify fixes work correctly

### If Performance Degrades

1. **Profile**: Identify bottlenecks
2. **Optimize**: Improve performance
3. **Compare**: Benchmark old vs new
4. **Decide**: Proceed if acceptable, or optimize further

## Success Criteria

Migration is considered successful when:

1. ✅ All tests pass (new and existing)
2. ✅ Output equivalence verified
3. ✅ No performance regression
4. ✅ Documentation complete
5. ✅ Code review approved
6. ✅ No breaking changes
7. ✅ Rollback plan tested

## Conclusion

By following these safety measures, we can introduce the SpecBuilder pattern safely and incrementally, without risking corruption of the existing codebase. The key is isolation, testing, gradual migration, and maintaining backwards compatibility throughout.
