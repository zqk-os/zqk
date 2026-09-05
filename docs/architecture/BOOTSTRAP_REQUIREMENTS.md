# Bootstrap Requirements for File Backend

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-04  
**Status**: Active  
**Purpose**: Document essential configuration files required for file backend initialization

## Overview

The `zqk system init` command must bootstrap a complete, ready-to-use project structure. This includes not just directory creation, but also copying essential configuration files that the system depends on for proper operation.

## Required Configuration Files

### 1. ID Prefixes Configuration

**File**: `docs/process/_internal/id_prefixes_config.yaml`

**Purpose**: Defines the mapping between object kinds and their ID prefixes. This is critical for:
- ID validation
- ID generation
- Ensuring spec file `id_prefixes` take precedence over inferred prefixes

**Source**: `docs/process/_internal/id_prefixes_config.yaml` (from project root)

**Why Required**: 
- Without this file, the ID validator falls back to default config with hardcoded mappings
- The default config may not include all object kinds defined in spec files
- Spec files with explicit `id_prefixes` should take precedence, but the global config is still needed for:
  - Explicit mappings that override spec prefixes (when config has explicit mapping)
  - Inference rules for kinds without spec files
  - Default behavior when spec files don't define `id_prefixes`

**Implementation Note**: The ID validator fix (2026-01-04) ensures that spec file `id_prefixes` take precedence over *inferred* config prefixes, but explicit config mappings still override spec prefixes (as intended for backward compatibility).

**Test Scenarios**: This file **MUST** be copied to test scenarios:
- Location: `test-scenarios/{scenario-name}/docs/process/_internal/id_prefixes_config.yaml`
- Must be kept in sync with the main project file
- Test scenarios will fail ID validation without this file

### 2. Kind Mappings Configuration

**File**: `docs/process/_internal/configs/kind_mappings_config.yaml`

**Purpose**: Maps object kinds to directory names for file-based storage.

**Source**: `docs/process/_internal/configs/kind_mappings_config.yaml` (from project root)

**Why Required**:
- Determines where objects are stored on disk
- Required for any file-based storage operations
- Without this, the system cannot determine the correct directory for object storage

**Test Scenarios**: This file **MUST** be copied to test scenarios:
- Location: `test-scenarios/{scenario-name}/docs/process/_internal/configs/kind_mappings_config.yaml`
- Must be kept in sync with the main project file
- Test scenarios will fail to store/retrieve objects without this file

### 3. Object Specs

**File**: `docs/process/_internal/object_specs/*.yaml`

**Purpose**: Define object schemas, validation rules, and ID patterns.

**Source**: `docs/process/_internal/object_specs/*.yaml` (from project root)

**Why Required**:
- Required for object validation
- Defines ID prefix patterns (via `id_prefixes` field)
- Defines object structure and required fields
- Without specs, objects cannot be created or validated

**Base Specs**: When a spec file uses `extends:` to inherit from another spec (e.g., `extends: base_metric`), **all specs in the dependency chain** must be present. Common base specs include:
- `auditable.yaml` - Base for all auditable objects (no parent)
- `base_object.yaml` - Base for all objects (extends `auditable`)
- `base_metric.yaml` - Base for all metric objects (extends `base_object`)
- Other base specs as needed

**Dependency Resolution**: The system loads specs recursively, so if `test_audit_aggregation_metric` extends `base_metric`, which extends `base_object`, which extends `auditable`, then all four specs must be present.

**Note**: Specs are typically copied as needed, but a minimal set should be available for basic operations. When copying a spec that extends another, ensure **all base specs in the dependency chain** are also copied.

## Current State

As of 2026-01-04, the `zqk system init` command:
- ✅ Creates directory structure (`.zqk/`, `docs/process/`, etc.)
- ✅ Creates `.zqk/config.yaml`
- ❌ Does NOT copy `id_prefixes_config.yaml`
- ❌ Does NOT copy `configs/kind_mappings_config.yaml`
- ❌ Does NOT copy object specs

## Required Changes

### Archive-Based Bootstrap Approach

**Solution**: Bundle essential files as an archive that mirrors `docs/process/_internal` structure. The build process creates the archive, and the init command extracts it.

#### Build Process

1. **Create Bootstrap Archive**:
   - During build, create an archive (e.g., `bootstrap.tar.gz` or embedded in binary)
   - Archive mirrors `docs/process/_internal/` directory structure
   - Include essential files:
     - `id_prefixes_config.yaml`
     - `configs/kind_mappings_config.yaml`
     - Core object specs (`object_specs/*.yaml`)
     - `paths_config.yaml` (if exists)
   - Include version information in archive manifest

2. **Build Validation**:
   - Validate that all required files are present before creating archive
   - Fail build if essential files are missing
   - Generate manifest listing all bundled files

#### Init Command

1. **Extract Bootstrap Archive**:
   ```go
   // Extract archive to new project's docs/process/_internal directory
   archivePath := getEmbeddedBootstrapArchive() // or read from bundled file
   extractDir := filepath.Join(cwd, "docs", "process", "_internal")
   extractArchive(archivePath, extractDir, force)
   ```

2. **Extraction Logic**:
   - Extract archive preserving directory structure
   - Preserve file permissions
   - Handle `--force` flag (overwrite existing files)
   - Provide clear error messages on failure

### Implementation Considerations

1. **Archive Format**: 
   - Use standard archive format (tar.gz) for portability
   - Or embed as Go `embed.FS` for single-binary distribution
   - Consider compression for size optimization

2. **Template Support**: 
   - Different templates (standard, minimal) might need different archive contents
   - Or single archive with all files, init selects based on template

3. **Versioning**: 
   - Archive manifest includes version information
   - Init command can validate compatibility
   - Support for archive format versioning

4. **Traceability**:
   - Build process generates manifest of bundled files
   - Documentation lists what files are included
   - Tests verify extracted files match expected structure

## Test Scenario Pattern

The test scenario pattern (`test-scenarios/`) demonstrates the correct bootstrap approach:

1. Run `zqk system init` to create directory structure
2. **REQUIRED**: Copy `id_prefixes_config.yaml` to test scenario
   - Location: `test-scenarios/{scenario-name}/docs/process/_internal/id_prefixes_config.yaml`
   - Purpose: Enables ID validation and generation for all object kinds in the test scenario
   - Without this file, ID validation will fail for objects that don't have explicit prefixes in their spec files
3. **REQUIRED**: Copy `configs/kind_mappings_config.yaml` to test scenario
   - Location: `test-scenarios/{scenario-name}/docs/process/_internal/configs/kind_mappings_config.yaml`
   - Purpose: Maps object kinds to directory names for file-based storage
   - Without this file, the system cannot determine where to store objects
4. Copy test-specific spec files as needed
   - Only copy specs that are actually used in the test scenario
   - Ensure all base specs in dependency chains are included (see "Object Specs" section above)

**Note**: These files are **required** for test scenarios to function correctly. The test scenario will fail if these files are missing or outdated.

This pattern should be reflected in the init command itself.

## Related Files

- `cmd/zqk/system/init.go` - Init command implementation
- `pkg/storage/content_addressable_storage_comprehensive_test.go` - Test scenario bootstrap example
- `docs/process/_internal/id_prefixes_config.yaml` - ID prefixes config source
- `docs/process/_internal/configs/kind_mappings_config.yaml` - Kind mappings config source

## Backlog Item

This requirement is tracked as:
- **Backlog Item**: [BLI-907](../backlog/BLI-907.yaml) - Archive-Based Bootstrap for zqk system init
- **Requirements**: 
  - [REQ-9009](../requirements/REQ-9009.yaml) - Build Process Creates Bootstrap Archive
  - [REQ-9010](../requirements/REQ-9010.yaml) - Init Command Extracts Bootstrap Archive
  - [REQ-9011](../requirements/REQ-9011.yaml) - Traceability for Bundled Bootstrap Files
- **Criteria**: See requirement files for linked criteria

---

*Last Updated: 2026-01-05*

