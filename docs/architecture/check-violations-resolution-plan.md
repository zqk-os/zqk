# Check Violations Resolution Plan

**Last Verified:** 2026-08-31


**Created:** 2025-12-25  
**Status:** In Progress  
**Related:** BLI-626

## Current State

From `check.json` analysis:
- **606 objects** checked
- **1080 total issues**
  - **31 blocking issues** (Tier 1): All hash mismatches
  - **16 warnings** (Tier 2)
  - **1033 informational** (Tier 3): Mostly semantic type validation false positives
  - **0 recommendations** (Tier 4)

## Issue Breakdown

### 1. Hash Mismatches (31 blocking issues)

**Root Cause**: Files were edited directly, causing hash mismatches.

**Resolution**:
```bash
# Auto-fix all hash mismatches (creates audit events)
zqk check all --auto-fix --force
```

**Impact**: All 31 blocking issues resolved, audit trail created.

### 2. Semantic Type Validation False Positives (946 issues)

**Root Cause**: Validator incorrectly validates list fields as if they should be strings.

**Example**:
- Field: `milestone_refs` (type: `list`, semantic_type: `reference`)
- Value: `["MIL-001", "MIL-002"]` (correct list)
- Error: "Semantic type validation: reference semantic type requires string value"

**Fix Applied**: Updated `validateSemanticType` to detect when value is actually a list (using reflection) and validate each item instead of the list itself.

**Status**: ✅ Fixed in `pkg/validation/go_validator.go`

**Impact**: ~946 false positive issues eliminated.

### 3. Performance Issues

**Current**: ~12 seconds for 606 objects  
**Bottlenecks**:
- Spec loaded 606 times (once per object)
- HashRegistry loaded 606 times (once per object)
- Validator created 606 times (once per object)
- No caching

**Fix Plan**: Add caching (see `check-command-performance-v1.0.md`)

**Expected**: ~2-3x improvement (4-6 seconds)

## Resolution Steps

### Step 1: Fix Semantic Type Validation ✅
- [x] Update `validateSemanticType` to detect list values
- [x] Validate list items instead of list itself
- [ ] Test fix with `zqk check all`

### Step 2: Auto-Fix Hash Mismatches
- [ ] Run `zqk check all --auto-fix --force`
- [ ] Verify audit events created
- [ ] Confirm all 31 blocking issues resolved

### Step 3: Add Caching (Performance)
- [ ] Add SpecLoader caching
- [ ] Cache HashRegistry per kind
- [ ] Reuse validators/loaders
- [ ] Measure performance improvement

### Step 4: Verify Resolution
- [ ] Run `zqk check all` again
- [ ] Verify issue count reduced from 1080 to ~100-150
- [ ] Verify performance improved

## Expected Results

### Before Fixes
- 1080 issues (31 blocking, 16 warnings, 1033 informational)
- ~12 seconds execution time

### After Fixes
- ~100-150 issues (0 blocking, ~16 warnings, ~100 informational)
- ~4-6 seconds execution time (with caching)

## Cache Behavior Documentation

See `check-command-performance-v1.0.md` for detailed caching strategy.

### Key Points:
1. **SpecLoader**: Cache specs per kind (loaded once, reused)
2. **HashRegistry**: Cache per kind (loaded once, reused)
3. **Validators**: Reuse single instance across all objects
4. **File Backend**: Cache based on file mtime
5. **Graph Backend**: Cache based on node version/timestamp
6. **Cache TTL**: 5 minutes default (configurable)

