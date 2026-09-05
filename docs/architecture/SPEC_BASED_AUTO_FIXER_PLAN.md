# Spec-Based Auto-Fixer Plan

**Last Verified:** 2026-08-31


**Created**: 2026-01-06  
**Status**: Design  
**Related**: BLI-626, Check Command Improvements; [BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md](./BACKLOG_REFS_AUTOLINK_AND_CREATE_VENEERS.md) (document reference auto-linking and future CLI creation veneers)

## Overview

Extend the existing auto-fix functionality (currently only handles hash mismatches) to use object specifications to automatically fix common validation issues.

## Current Auto-Fix Capabilities

- ✅ **Hash Mismatches**: Regenerates integrity hashes for files
- ✅ **Missing Hashes**: Adds missing hash entries to registry

## Proposed Spec-Based Auto-Fix Capabilities

### 1. Missing Required Fields
**Issue**: Object missing required field according to spec  
**Auto-Fix**: Use spec to determine default value or infer from context
- Check spec for field defaults
- Use enum default if available
- For reference fields, attempt to infer from context
- For computed fields, calculate value

### 2. Type Mismatches
**Issue**: Field value doesn't match spec type  
**Auto-Fix**: Coerce value to correct type
- String → Number (if numeric)
- Number → String (if string type)
- String → Boolean (if boolean type)
- Array → Single value (if single type expected)
- Single value → Array (if array type expected)

### 3. Enum Value Validation
**Issue**: Field value not in allowed enum list  
**Auto-Fix**: Suggest closest match or use default
- Fuzzy match against enum values
- Use spec default if available
- Log suggestion for manual review

### 4. Reference Integrity (Limited)
**Issue**: Referenced object doesn't exist  
**Auto-Fix**: 
- For optional references: Remove invalid reference
- For required references: Suggest creating missing object
- For inferred references: Attempt to find correct reference

### 5. Lifecycle Preconditions
**Issue**: Lifecycle precondition not met  
**Auto-Fix**: 
- For milestone_ref: Suggest valid milestones
- For priority_tier: Suggest matching tier
- For status transitions: Suggest valid transitions

## Implementation Plan

### Phase 1: Infrastructure
1. Create `SpecBasedAutoFixer` struct
2. Load spec for each object kind
3. Identify fixable issues based on spec
4. Track fixes in `AutoFixed` array

### Phase 2: Field-Level Fixes
1. Missing required fields
2. Type coercion
3. Enum validation

### Phase 3: Reference Fixes
1. Optional reference removal
2. Reference inference
3. Reference validation

### Phase 4: Lifecycle Fixes
1. Precondition resolution
2. Status transition suggestions

## Safety Considerations

- **Dry-Run Mode**: Always support `--dry-run` to preview fixes
- **Audit Trail**: All fixes must create audit events
- **Backup**: Create backup before applying fixes
- **Confirmation**: Require `--force` for destructive fixes
- **Validation**: Re-validate after each fix

## Usage

```bash
# Preview fixes (dry-run)
zqk system check all --auto-fix --dry-run

# Apply spec-based fixes
zqk system check all --auto-fix

# Force destructive fixes
zqk system check all --auto-fix --force
```

## Files to Create/Modify

1. `pkg/validation/spec_auto_fixer.go` - Core auto-fix logic
2. `cmd/zqk/system/check_impl.go` - Integration with check command
3. `pkg/objects/spec.go` - Add default value extraction
4. Tests for each fix type

