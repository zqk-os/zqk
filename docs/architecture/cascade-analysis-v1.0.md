# Cascade Deletion and Update Analysis

**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Analyze cascade scenarios and propose implementation plan

## Current State

### What Exists
1. **Cascade DELETE** (deletes dependents): Implemented in `pkg/storage/object_storage_file.go` and `pkg/storage/object_storage_graph.go`
   - When `cascade=true`, deletes all objects that reference the deleted object
   - Recursive: deletes dependents of dependents
   - Problem: This is too aggressive - it deletes objects instead of updating references

2. **Reference Detection**: `findDependents()` finds all objects that reference a given object
   - Scans all object files
   - Uses `objectReferences()` to check reference fields

### What's Missing
1. **Cascade UPDATE** (removes references): Not implemented
   - When an object is deleted, references in dependent objects should be removed
   - Example: Delete `CRIT-001` → remove from `TEST-001.criteria_refs` and `REQ-001.criteria_refs`

2. **Cascade Rules in Specs**: Object specs don't define cascade behavior
   - No `cascade` directives in reference field definitions
   - No distinction between required vs optional references for cascade purposes

3. **Cascade RESTRICT**: Not implemented
   - Should prevent deletion if required references exist
   - Example: Cannot delete `GOAL-001` if `REQ-001.goal_refs` requires it (min_length: 1)

## Cascade Scenarios Analysis

### Scenario 1: Optional List Reference (CASCADE_NULLIFY)

**Example**: Delete `CRIT-001` (criteria)

**Before**:
- `TEST-001.criteria_refs = [CRIT-001, CRIT-002]`
- `REQ-001.criteria_refs = [CRIT-001]`

**After**:
- `TEST-001.criteria_refs = [CRIT-002]` (CRIT-001 removed)
- `REQ-001.criteria_refs = []` (CRIT-001 removed, list empty but valid)

**Rule**: Remove deleted ID from list references
**Applies to**: All optional `_refs` fields (list references)

### Scenario 2: Required List Reference (CASCADE_RESTRICT or CASCADE_DELETE)

**Example**: Delete `GOAL-001` (goal)

**Before**:
- `REQ-001.goal_refs = [GOAL-001]` (required, min_length: 1)

**Options**:
- **CASCADE_RESTRICT**: Prevent deletion, error: "Cannot delete GOAL-001: required by REQ-001"
- **CASCADE_DELETE**: Delete REQ-001 (current behavior with cascade=true)

**Rule**: If reference is required (min_length >= 1), either restrict or delete dependent
**Applies to**: Required `_refs` fields (e.g., `requirement.goal_refs`, `requirement.criteria_refs`)

### Scenario 3: Single Optional Reference (CASCADE_SET_NULL)

**Example**: Delete `STRAT-PLAN-EXAMPLE` (strategic plan)

**Before**:
- `MIL-001.strategic_plan_ref = STRAT-PLAN-EXAMPLE`

**After**:
- `MIL-001.strategic_plan_ref = null` (set to null)

**Rule**: Set single reference field to null
**Applies to**: Optional `_ref` fields (single references)

### Scenario 4: Single Required Reference (CASCADE_RESTRICT)

**Example**: Delete `GOAL-001` if it's referenced as a single required reference

**Rule**: Prevent deletion if single required reference exists
**Applies to**: Required `_ref` fields (if any exist)

### Scenario 5: Multi-Level Cascade

**Example**: Delete `STRAT-PLAN-EXAMPLE` in a chain:
- `STRAT-PLAN-EXAMPLE` → `MIL-001.strategic_plan_ref`
- `MIL-001` → `GOAL-001.milestone_refs`
- `GOAL-001` → `REQ-001.goal_refs`
- `REQ-001` → BLI-001.requirement_refs`

**Cascade Actions**:
1. `MIL-001.strategic_plan_ref = null` (CASCADE_SET_NULL)
2. `GOAL-001.milestone_refs` remove `MIL-001` (CASCADE_NULLIFY)
3. `REQ-001.goal_refs` remove `GOAL-001` (CASCADE_NULLIFY or RESTRICT if required)
4. `BLI-001.requirement_refs` remove `REQ-001` (CASCADE_NULLIFY)

## Reference Field Classification

### By Type
1. **Single References (`_ref`)**: One ID, type `string`
   - Examples: `strategic_plan_ref`, `goal_ref` (if single)
   - Cascade: `CASCADE_SET_NULL` or `CASCADE_RESTRICT`

2. **List References (`_refs`)**: Multiple IDs, type `list`
   - Examples: `goal_refs`, `milestone_refs`, `criteria_refs`
   - Cascade: `CASCADE_NULLIFY` (remove from list) or `CASCADE_DELETE`/`CASCADE_RESTRICT`

### By Requirement
1. **Optional** (`required: false` or no `required` field)
   - Cascade: `CASCADE_NULLIFY` or `CASCADE_SET_NULL`
   - Can be removed/set to null without breaking integrity

2. **Required** (`required: true` or `min_length >= 1`)
   - Cascade: `CASCADE_RESTRICT` (prevent deletion) or `CASCADE_DELETE` (delete dependent)
   - Removal would break integrity

### By Criticality
1. **Association** (`criticality: association`)
   - Cascade: `CASCADE_NULLIFY` or `CASCADE_SET_NULL`
   - Loose coupling, can be removed

2. **Composition** (`criticality: composition`)
   - Cascade: `CASCADE_RESTRICT` or `CASCADE_DELETE`
   - Tight coupling, parent owns child

## Proposed Object Spec Extensions

### Add `cascade` field to reference field definitions:

```yaml
criteria_refs:
  semantic_type: reference
  type: list
  validation:
    required: false
  cascade:
    on_delete: nullify  # nullify, set_null, restrict, delete
    on_update: validate   # validate, invalidate_cache
  cascade_priority: 1     # Order of cascade operations (1 = highest)
```

### Cascade Action Values

- **`nullify`**: Remove from list (for `_refs` fields)
- **`set_null`**: Set to null (for `_ref` fields)
- **`restrict`**: Prevent deletion if reference exists
- **`delete`**: Delete dependent object (current cascade=true behavior)
- **`validate`**: Re-validate dependent objects when parent changes
- **`invalidate_cache`**: Invalidate validation cache for dependents

## Implementation Plan

### Phase 1: Spec Updates
1. Add `cascade` directives to all reference fields in object specs
2. Document cascade rules in architecture docs
3. Create validation to ensure cascade rules are consistent

### Phase 2: Cascade Update Logic
1. Implement `cascadeUpdateReferences()` in storage layer
2. When deleting an object:
   - Find all dependents (existing `findDependents()`)
   - For each dependent:
     - Load object spec to determine cascade rules
     - Apply cascade action (nullify, set_null, restrict)
     - Update dependent object
3. Handle multi-level cascade (recursive)

### Phase 3: Cascade Restrict
1. Before deletion, check if any required references exist
2. If cascade rule is `restrict`, prevent deletion
3. Return clear error message with dependent IDs

### Phase 4: Testing
1. Unit tests for each cascade scenario
2. Integration tests for multi-level cascade
3. Concurrent cascade tests
4. Performance tests for large dependency graphs

## Common Object Reference Patterns

### Criteria (`criteria`)
- **Referenced by**: `test_case.criteria_refs`, `requirement.criteria_refs`
- **Cascade**: `nullify` (optional, association)

### Goal (`goal`)
- **Referenced by**: `requirement.goal_refs` (required, min_length: 1), `backlog_item.goal_refs` (optional)
- **Cascade**: `restrict` for required, `nullify` for optional

### Milestone (`milestone`)
- **Referenced by**: `goal.milestone_refs` (optional), `requirement.milestone_refs` (optional)
- **Cascade**: `nullify` (all optional)

### Strategic Plan (`strategic_plan`)
- **Referenced by**: `milestone.strategic_plan_ref` (single, optional)
- **Cascade**: `set_null` (single optional reference)

### Requirement (`requirement`)
- **Referenced by**: `backlog_item.requirement_refs` (optional), `test_case.requirement_refs` (optional)
- **Cascade**: `nullify` (all optional)

## Edge Cases

1. **Circular References**: Detect and prevent infinite loops
2. **Concurrent Deletions**: Handle race conditions when multiple objects are deleted simultaneously
3. **Partial Failures**: If cascade update fails for one dependent, continue with others
4. **Transaction Integrity**: Ensure all cascade updates are atomic
5. **Validation Cache**: Invalidate cache for updated dependents

## Migration Strategy

1. **Backward Compatibility**: Current `cascade=true` behavior (delete dependents) remains default
2. **Gradual Migration**: Add cascade rules to specs incrementally
3. **Feature Flag**: Enable new cascade update logic behind a flag
4. **Validation**: System check should warn about missing cascade rules

