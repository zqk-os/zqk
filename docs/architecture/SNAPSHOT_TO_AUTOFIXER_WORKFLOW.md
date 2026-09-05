# Snapshot to Auto-Fixer Workflow Design

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-06  
**Status**: Design  
**Purpose**: Define the complete workflow from snapshot capture to auto-fixer validation

## Overview

The end-goal is to validate snapshot data transfer and restoration of check violations via a spec-driven auto-fixer. This document outlines the complete workflow and design recommendations.

## Workflow: Snapshot → Init → Check → Auto-Fixer

### Phase 1: Snapshot Capture

**Command**: `zqk system check --snapshot test-scenarios/check-violation-resolver`

**What it does**:
1. Runs full system check
2. Captures all check results (objects with issues)
3. Saves as JSON snapshot (`check-snapshot-{timestamp}.json`)
4. Saves as compressed snapshot (`check-snapshot-{timestamp}.csnap`)
5. Creates symlinks to `check-snapshot-latest.*`

**Output**:
- `check-snapshot-latest.json` - Full check results with metadata
- `check-snapshot-latest.csnap` - Compressed format for efficient storage

**Key Data Captured**:
- All objects checked (with and without issues)
- All validation issues (tier, category, message, auto_fixable flag)
- Object metadata (ID, kind, file path)
- Snapshot metadata (timestamp, project root, command, counts)

### Phase 2: Snapshot Init (Test Scenario)

**Command**: `ZQK_TEST_ROOT=test-scenarios/check-violation-resolver zqk system init --from-snapshot check-snapshot-latest.csnap --wipe`

**What it does**:
1. Detects `ZQK_TEST_ROOT` environment variable
2. Sets project root to test scenario directory
3. Loads compressed snapshot
4. Expands snapshot to get all objects
5. Creates directory structure (`docs/process/` with all kind directories)
6. Extracts bootstrap archive (config files, specs)
7. Restores objects from snapshot:
   - Writes objects to correct directories based on kind
   - Updates hash registries for integrity
   - Handles CAS vs traditional file storage
8. Creates `.zqk/config.yaml`

**Modes**:
- `--wipe`: Remove existing data, restore from snapshot (clean slate)
- `--merge`: Keep existing data, add snapshot data (handle conflicts)

**Critical Requirements**:
- Must respect `ZQK_TEST_ROOT` for test scenarios
- Must create all kind directories (not just common ones)
- Must restore hash registries for integrity checking
- Must handle both CAS and traditional file storage
- Must preserve object structure and metadata

### Phase 3: Check Validation

**Command**: `ZQK_TEST_ROOT=test-scenarios/check-violation-resolver zqk system check --format jsonl`

**What it does**:
1. Uses `ZQK_TEST_ROOT` as project root
2. Scans all objects in test scenario
3. Validates each object (registration, lifecycle, integrity, references)
4. Outputs results in JSONL format

**Expected Results**:
- Should find same issues as original snapshot (1:1 match)
- Hash mismatches may occur if files were modified (expected)
- New issues may appear if snapshot data is incomplete
- Should validate that all objects from snapshot are present

### Phase 4: Auto-Fixer Application

**Command**: `ZQK_TEST_ROOT=test-scenarios/check-violation-resolver zqk system check --auto-fix`

**What it does**:
1. Runs check validation
2. For each issue with `auto_fixable: true`:
   - Loads object spec
   - Determines fix strategy from spec
   - Applies fix (e.g., regenerate hash, add missing field)
   - Updates object file
   - Updates hash registry
   - Creates audit event
3. Re-validates fixed objects
4. Reports what was fixed

**Spec-Driven Fixes**:
- **Hash Mismatch**: Regenerate hash from current content
- **Missing Hash**: Calculate and add hash
- **Missing Field**: Add field with default value from spec
- **Invalid Reference**: Attempt to resolve reference from spec context
- **Type Mismatch**: Coerce to correct type per spec

### Phase 5: Validation & Comparison

**Command**: Compare original snapshot with post-fix check results

**What it does**:
1. Load original snapshot
2. Run check after auto-fix
3. Compare results:
   - Count of issues before/after
   - Which issues were fixed
   - Which issues remain (and why)
   - New issues introduced (if any)

**Success Criteria**:
- All auto-fixable issues are resolved
- No new issues introduced
- Hash integrity maintained
- Object structure preserved

## Design Recommendations

### 1. Snapshot Format Enhancement

**Current**: Check snapshot contains check results (objects + issues)

**Recommendation**: Add object content to snapshot for full restoration

**Options**:
- **Option A**: Include full object YAML in snapshot (larger, but complete)
- **Option B**: Store object file paths, restore by reading from original project
- **Option C**: Hybrid - include object content for objects with issues, paths for others

**Recommendation**: **Option A** for test scenarios (complete isolation), **Option C** for production (efficiency)

### 2. Init Command Enhancements

**Current**: Basic directory creation

**Recommendations**:
1. **Dynamic Kind Directory Creation**: Query all object kinds from specs, create all directories
2. **Bootstrap Archive Integration**: Extract config files and specs during init
3. **Hash Registry Restoration**: Restore hash registries from snapshot metadata
4. **CAS Support**: Handle both CAS and traditional file storage during restoration
5. **Conflict Resolution**: Smart merging for `--merge` mode (ID conflicts, directory conflicts)

### 3. Object Restoration Strategy

**Challenge**: Restoring objects while maintaining integrity

**Recommendations**:
1. **Use Storage Layer**: Don't write files directly, use `FileObjectStorage.Create()` to ensure:
   - Proper validation
   - Hash registry updates
   - CAS index updates (if applicable)
   - Audit event creation
2. **Bypass Validation for Snapshot Restore**: Add `--skip-validation` flag to init for faster restoration
3. **Batch Operations**: Restore objects in batches to improve performance
4. **Progress Reporting**: Show progress during large snapshot restores

### 4. Auto-Fixer Integration

**Challenge**: Applying fixes based on spec definitions

**Recommendations**:
1. **Spec-Based Fix Rules**: Define fix strategies in object specs:
   ```yaml
   auto_fix_rules:
     - issue_category: integrity
       issue_pattern: "Hash mismatch"
       fix_strategy: regenerate_hash
     - issue_category: registration
       issue_pattern: "Missing required field"
       fix_strategy: add_default_value
       default_value_path: spec.fields.{field}.default
   ```
2. **Fix Validation**: After applying fix, re-validate to ensure fix worked
3. **Fix Rollback**: If fix fails validation, rollback and report
4. **Fix Audit Trail**: Create audit events for all fixes applied

### 5. Test Scenario Workflow

**Current**: Manual snapshot capture and comparison

**Recommendations**:
1. **Automated Workflow Script**: Create script that:
   - Captures snapshot
   - Initializes test scenario
   - Runs check
   - Applies auto-fixer
   - Compares results
   - Reports differences
2. **CI Integration**: Run workflow in CI to validate snapshot → auto-fixer pipeline
3. **Regression Testing**: Use snapshots to test auto-fixer against known issues

### 6. Environment Variable Handling

**Current**: `ZQK_TEST_ROOT` respected by `FindProjectRoot()`

**Recommendations**:
1. **Explicit Project Root Flag**: Add `--project-root` flag to all commands
2. **Environment Variable Precedence**: `ZQK_PROJECT_ROOT` > `ZQK_TEST_ROOT` > auto-discovery
3. **Context Propagation**: Ensure project root propagates through all command contexts
4. **Validation**: Validate project root exists and has `.zqk/` or `docs/process/` structure

## Implementation Priority

### Phase 1: Core Functionality (Immediate)
1. ✅ Snapshot capture (already implemented)
2. 🔄 Snapshot init with object restoration
3. 🔄 Auto-fixer spec-based rules
4. 🔄 Test scenario workflow script

### Phase 2: Enhancements
1. Bootstrap archive extraction
2. Hash registry restoration
3. CAS support in restoration
4. Conflict resolution for merge mode

### Phase 3: Validation & Testing
1. Automated workflow script
2. CI integration
3. Regression test suite
4. Performance optimization

## Related Documentation

- [Init Scenarios](./INIT_SCENARIOS.md) - Three init scenario types
- [Spec-Based Auto-Fixer Plan](./SPEC_BASED_AUTO_FIXER_PLAN.md) - Auto-fixer design
- [Compressed Snapshot Format](./COMPRESSED_SNAPSHOT_FORMAT.md) - Snapshot structure
- [Bootstrap Requirements](./BOOTSTRAP_REQUIREMENTS.md) - Bootstrap archive

---

*Last Updated: 2026-01-06*

