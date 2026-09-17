# Init Command Scenarios

**Last Verified:** 2026-08-31

**Version**: 1.0.0  
**Created**: 2026-01-06  
**Status**: Design  
**Purpose**: Define the three types of initialization scenarios and their requirements

## Overview

The `zqk system init` command must support three distinct initialization scenarios:

1. **Greenfield** - Brand new project, never managed by zqk
2. **Legacy Project** - Existing project being brought under zqk management
3. **Init with Snapshot** - Initialize from snapshot data (merge or wipe)

## Scenario 1: Greenfield Project

**Use Case**: Brand new project that has never been managed by zqk.

**Characteristics**:
- No existing `.zqk/` directory
- No existing `.zqk/process/` structure
- No existing objects or configuration
- Clean slate initialization

**Command**:
```bash
zqk system init [--project-name NAME] [--template TEMPLATE]
```

**Behavior**:
1. Create `.zqk/` directory structure
2. Create `.zqk/process/` directory structure (all kind directories)
3. Extract bootstrap archive (config files, specs)
4. Create initial `.zqk/config.yaml`
5. Update `.gitignore` if `.git` exists

**Flags**:
- `--project-name`: Project name (defaults to directory name)
- `--template`: Template to use (standard, minimal)
- `--force`: Overwrite existing files (should not be needed for greenfield)

**Validation**:
- Must fail if `.zqk/` exists (unless `--force`)
- Must fail if `.zqk/process/` exists (unless `--force`)
- Should warn if directory is not empty (may be legacy project)

## Scenario 2: Legacy Project

**Use Case**: Existing project (with files, code, structure) being brought under zqk management.

**Characteristics**:
- May have existing directory structure
- May have existing files that should be preserved
- Needs zqk structure added without disrupting existing content
- May need to discover and register existing objects

**Command**:
```bash
zqk system init --legacy [--project-name NAME] [--discover] [--register-existing]
```

**Behavior**:
1. Check if `.zqk/` exists - if not, create it
2. Check if `.zqk/process/` exists - if not, create it
3. Extract bootstrap archive (only if files don't exist)
4. Create/update `.zqk/config.yaml` (merge with existing if present)
5. If `--discover`: Run discovery wizard to understand project context
6. If `--register-existing`: Scan for existing objects and register them
7. Preserve all existing files and directories

**Flags**:
- `--legacy`: Enable legacy project mode
- `--project-name`: Project name (defaults to directory name)
- `--discover`: Run interactive discovery wizard
- `--register-existing`: Scan and register existing objects
- `--force`: Overwrite existing zqk files (config, bootstrap files)

**Validation**:
- Should NOT fail if `.zqk/` exists (merge/update instead)
- Should NOT fail if `.zqk/process/` exists (add missing directories)
- Should preserve all existing files
- Should warn about existing objects that may need registration

**Discovery Wizard** (if `--discover`):
- Project type and context
- Existing structure analysis
- Stakeholder identification
- Goal and milestone discovery
- Policy and requirement discovery

## Scenario 3: Init with Snapshot

**Use Case**: Initialize a project from snapshot data (test scenarios, project clones, etc.).

**Characteristics**:
- Snapshot data available (compressed or JSON)
- May have existing data that needs to be handled
- Needs to restore object state from snapshot
- May need to merge with existing or wipe existing

**Command**:
```bash
zqk system init --from-snapshot SNAPSHOT_PATH [--merge|--wipe] [--project-name NAME]
```

**Behavior**:
1. Load snapshot (compressed `.csnap` or JSON)
2. If `--wipe`: Remove existing `.zqk/process/` data (preserve `.zqk/` config)
3. If `--merge`: Keep existing data, add snapshot data (handle conflicts)
4. Create `.zqk/` structure if missing
5. Create `.zqk/process/` structure (all kind directories)
6. Extract bootstrap archive (config files, specs)
7. Restore objects from snapshot to `.zqk/process/` directories
8. Create/update `.zqk/config.yaml`
9. Update hash registries for restored objects

**Flags**:
- `--from-snapshot`: Path to snapshot file (`.csnap` or `.json`)
- `--merge`: Merge snapshot data with existing (default if data exists)
- `--wipe`: Wipe existing data before restoring snapshot (requires confirmation)
- `--project-name`: Project name (defaults to directory name or snapshot metadata)
- `--force`: Skip confirmation prompts

**Validation**:
- Must validate snapshot file exists and is readable
- Must validate snapshot format (version compatibility)
- If `--wipe` and data exists: Require explicit confirmation
- If `--merge` and conflicts: Report conflicts, allow resolution
- Should preserve `.zqk/` config unless `--force`

**Conflict Resolution** (for `--merge`):
- Object ID conflicts: Skip snapshot object (existing wins) or rename snapshot object
- Directory conflicts: Merge directories
- Config conflicts: Merge configs (snapshot metadata as comments)

**Environment Variable Support**:
- `ZQK_PROJECT_ROOT`: Project root directory (defaults to current directory)
- `ZQK_TEST_ROOT`: Test scenario root (for test scenarios)
- When `ZQK_TEST_ROOT` is set, init should operate in that directory

## Implementation Plan

### Phase 1: Refactor Current Init (Greenfield)
- Extract common initialization logic
- Create bootstrap archive extraction
- Support all three scenarios with mode detection

### Phase 2: Legacy Project Support
- Add `--legacy` flag and mode detection
- Implement discovery wizard
- Implement existing object registration

### Phase 3: Snapshot Init Support
- Add `--from-snapshot` flag
- Implement snapshot loading and expansion
- Implement merge/wipe logic
- Integrate with hash registry restoration

## Related Documentation

- [Bootstrap Requirements](./BOOTSTRAP_REQUIREMENTS.md) - Bootstrap archive details
- [Project Discovery](./project-discovery-and-strategic-alignment-v1.0.md) - Discovery wizard
- [Snapshot Format](./COMPRESSED_SNAPSHOT_FORMAT.md) - Snapshot structure
- [CAS Snapshot Integrity](./CAS_SNAPSHOT_INTEGRITY.md) - Hash restoration

## Backlog Items

- **BLI-907**: Archive-Based Bootstrap for zqk system init
- **BLI-XXX**: Legacy Project Discovery and Registration
- **BLI-XXX**: Snapshot-Based Initialization

---

*Last Updated: 2026-01-06*

