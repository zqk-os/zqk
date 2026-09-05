# Cascade Rules for Object References

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Define cascade behavior when objects with references are deleted or updated

## Overview

When objects are deleted or updated, dependent objects that reference them need to be handled appropriately. This document defines the cascade rules for different reference types and scenarios.

## Cascade Scenarios

### Scenario 1: Deletion of Referenced Object (Parent Deletion)

When an object is deleted, objects that reference it need to be updated. The behavior depends on:
- Whether the reference is **required** or **optional**
- The **criticality** of the reference (composition vs association)
- The **cardinality** (single vs list)

#### Cascade Actions

1. **CASCADE_NULLIFY** (Default for optional references)
   - Remove the reference from dependent objects
   - Used when: reference is optional, association criticality
   - Example: Deleting a `criteria` object removes it from `test_case.criteria_refs` and `requirement.criteria_refs`

2. **CASCADE_DELETE** (For required references)
   - Delete dependent objects that have required references
   - Used when: reference is required, composition criticality
   - Example: Deleting a `goal` that is required by a `requirement` (via `goal_refs` with min_length: 1)

3. **CASCADE_RESTRICT** (For critical references)
   - Prevent deletion if dependents exist
   - Used when: reference is required and deletion would break integrity
   - Example: Cannot delete a `milestone` if `goal.milestone_refs` requires it (min_length: 1)

4. **CASCADE_SET_NULL** (For single optional references)
   - Set single reference field to null/empty
   - Used when: single reference (_ref), optional
   - Example: Deleting a `strategic_plan` sets `milestone.strategic_plan_ref` to null

### Scenario 2: Update of Referenced Object

When a referenced object is updated, dependent objects may need validation updates:
- **CASCADE_VALIDATE**: Re-validate dependent objects when parent changes
- **CASCADE_INVALIDATE_CACHE**: Invalidate validation cache for dependents

## Reference Field Types

### Single References (`_ref`)
- Format: `field_name_ref` (e.g., `strategic_plan_ref`, `goal_ref`)
- Type: `string` (single ID)
- Cascade: `CASCADE_SET_NULL` or `CASCADE_RESTRICT`

### List References (`_refs`)
- Format: `field_name_refs` (e.g., `goal_refs`, `milestone_refs`, `criteria_refs`)
- Type: `list` (array of IDs)
- Cascade: `CASCADE_NULLIFY` (remove from list) or `CASCADE_DELETE`

## Cascade Rules by Object Kind

### Criteria (`criteria`)
- **Referenced by**: `test_case.criteria_refs`, `requirement.criteria_refs`
- **Cascade on delete**: `CASCADE_NULLIFY` (remove from lists)
- **Reason**: Optional references, association criticality

### Goal (`goal`)
- **Referenced by**: `requirement.goal_refs` (required, min_length: 1), `backlog_item.goal_refs` (optional)
- **Cascade on delete**:
  - If required by requirement: `CASCADE_RESTRICT` (prevent deletion)
  - If optional: `CASCADE_NULLIFY` (remove from lists)

### Milestone (`milestone`)
- **Referenced by**: `goal.milestone_refs` (optional), `requirement.milestone_refs` (optional), `test_case.milestone_refs` (optional)
- **Cascade on delete**: `CASCADE_NULLIFY` (remove from lists)
- **Reason**: All references are optional

### Strategic Plan (`strategic_plan`)
- **Referenced by**: `milestone.strategic_plan_ref` (single, optional)
- **Cascade on delete**: `CASCADE_SET_NULL` (set to null)
- **Reason**: Single optional reference

### Requirement (`requirement`)
- **Referenced by**: `backlog_item.requirement_refs` (optional), `test_case.requirement_refs` (optional)
- **Cascade on delete**: `CASCADE_NULLIFY` (remove from lists)
- **Reason**: Optional references

### Test Case (`test_case`)
- **Referenced by**: `requirement.test_case_refs` (optional)
- **Cascade on delete**: `CASCADE_NULLIFY` (remove from lists)
- **Reason**: Optional references

## Implementation Requirements

### Object Spec Extensions

Each reference field should include cascade directives:

```yaml
criteria_refs:
  semantic_type: reference
  cascade:
    on_delete: nullify  # Remove from list
    on_update: validate  # Re-validate dependents
  required: false
```

### Storage Layer Implementation

1. **Cascade Delete Handler**: When deleting an object:
   - Find all dependents (objects that reference it)
   - Apply cascade rules based on reference field definitions
   - Update or delete dependents as specified

2. **Cascade Update Handler**: When updating an object:
   - Invalidate validation cache for dependents
   - Optionally re-validate dependents

3. **Cascade Restrict Check**: Before deleting:
   - Check if any required references exist
   - Prevent deletion if cascade rule is RESTRICT

## Examples

### Example 1: Delete Criteria (CASCADE_NULLIFY)

**Before**:
- `CRIT-001` exists
- `TEST-001.criteria_refs = [CRIT-001, CRIT-002]`
- `REQ-001.criteria_refs = [CRIT-001]`

**Action**: Delete `CRIT-001`

**After**:
- `CRIT-001` deleted
- `TEST-001.criteria_refs = [CRIT-002]` (CRIT-001 removed)
- `REQ-001.criteria_refs = []` (CRIT-001 removed, list may become empty if optional)

### Example 2: Delete Goal with Required Reference (CASCADE_RESTRICT)

**Before**:
- `GOAL-001` exists
- `REQ-001.goal_refs = [GOAL-001]` (required, min_length: 1)

**Action**: Attempt to delete `GOAL-001`

**Result**: Error - "Cannot delete GOAL-001: required by REQ-001 (use cascade=true to delete dependents)"

### Example 3: Delete Strategic Plan (CASCADE_SET_NULL)

**Before**:
- `STRAT-PLAN-001` exists
- `MIL-001.strategic_plan_ref = STRAT-PLAN-001`

**Action**: Delete `STRAT-PLAN-001`

**After**:
- `STRAT-PLAN-001` deleted
- `MIL-001.strategic_plan_ref = null` (set to null)

## Testing Requirements

Test cases should cover:
1. Single reference cascade (CASCADE_SET_NULL)
2. List reference cascade (CASCADE_NULLIFY)
3. Required reference restriction (CASCADE_RESTRICT)
4. Cascade delete (CASCADE_DELETE)
5. Multi-level cascade (parent → child → grandchild)
6. Concurrent cascade operations
7. Circular reference detection

