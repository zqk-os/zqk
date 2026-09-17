> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Migration Spec Design

**Last Verified:** 2026-08-31


**Created:** 2026-01-12  
**Status:** Design  
**Purpose:** Spec-driven migration system for system transformations

## Overview

Migrations should be spec-driven rather than individual commands. A migration spec describes the steps necessary to migrate from state A to state B, with automation where possible.

## Migration Spec Format

```yaml
schema_version: 1.0.0
id: migration-lifecycle-files-to-objects
name: Migrate Lifecycle Files to Objects
description: Convert lifecycle YAML files to lifecycle objects with source_type="built-in"

# Source and target states
from:
  state: lifecycle_files
  description: Lifecycle definitions stored as YAML files in .zqk/specs/lifecycles/
  
to:
  state: lifecycle_objects
  description: Lifecycle definitions stored as lifecycle objects with source_type="built-in"

# Prerequisites
prerequisites:
  - kind: lifecycle
    registered: true
    in_kind_mapper: true
    in_on_demand_kinds: true
  - directory: .zqk/process/lifecycles
    exists: false  # Will be created
    on_demand: true

# Migration steps
steps:
  - id: scan_lifecycle_files
    type: scan_files
    description: Scan for lifecycle YAML files
    config:
      directory: .zqk/specs/lifecycles
      pattern: "*_lifecycle.yaml"
      exclude:
        - "*.bak"
        - "built-in/*"
    
  - id: convert_lifecycle_to_object
    type: transform
    description: Convert lifecycle file to lifecycle object
    for_each: scan_lifecycle_files
    config:
      source: file_content
      target: object_map
      transform:
        builder: lifecycle_instance_builder
        version: v1_0_0
        fields:
          id: "LIFECYCLE-{object_type}-{version}"
          kind: lifecycle
          object_type: "{object_type}"
          source_type: built-in
          schema_version: "2.0.0"
          statuses: "{statuses}"
          transitions: "{transitions}"
          percent_complete: "{percent_complete}"
    
  - id: create_lifecycle_objects
    type: create_objects
    description: Create lifecycle objects in storage
    depends_on: convert_lifecycle_to_object
    config:
      storage: file
      skip_existing: true
      force: false

# Rollback steps (REQUIRED for snapshot compatibility)
rollback:
  strategy: snapshot_restore  # Preferred: restore from snapshot
  fallback: step_by_step      # Fallback: reverse steps
  
  # Snapshot-based rollback
  snapshot:
    tag: "pre_lifecycle_migration"
    apply_location_mappings: true
    apply_field_mappings: true
  
  # Step-by-step rollback (if snapshot unavailable)
  steps:
    - id: delete_lifecycle_objects
      type: delete_objects
      description: Delete lifecycle objects created by migration
      config:
        filter:
          kind: lifecycle
          source_type: built-in
          id_pattern: "LIFECYCLE-*-v1_0_0"

# Validation
validation:
  - type: object_count
    description: Verify all lifecycle files were migrated
    config:
      source_count: scan_lifecycle_files.count
      target_count: create_lifecycle_objects.created
      match: exact

# Snapshot compatibility (REQUIRED)
snapshot_compatible: true
pre_migration_snapshot:
  required: true
  auto_create: true
  tags: ["pre_lifecycle_migration"]
post_migration_snapshot:
  auto_create: true
  tags: ["post_lifecycle_migration"]
  validate_restore: true  # Test restore from pre-migration snapshot

# Tracking (for restore capability)
tracking:
  object_mapping: true  # Track object ID -> location mappings
  field_mapping: true   # Track field transformations
  state_changes: true   # Track state changes

# Options
options:
  dry_run: true
  force: false
  batch_size: 10
  continue_on_error: false
  checkpoint_interval: 100  # Create checkpoint every N objects
```

## Step Types

### scan_files

Scans a directory for files matching a pattern.

**Configuration:**
```yaml
type: scan_files
config:
  directory: string          # Required: Directory path to scan (relative to project root or absolute)
  pattern: string            # Optional: Glob pattern (default: "*")
  exclude: []string          # Optional: List of glob patterns to exclude
  recursive: bool            # Optional: Scan subdirectories recursively (default: false)
```

**Output:**
```yaml
output:
  files: []FileInfo          # List of file information objects
  count: int                 # Number of files found
```

**FileInfo Structure:**
```yaml
FileInfo:
  path: string               # Full file path
  name: string               # File name
  size: int64                # File size in bytes
  mode: os.FileMode          # File permissions/mode
```

**Performance Considerations:**
- **Small directories (< 100 files)**: No special considerations
- **Medium directories (100-10,000 files)**: Consider using `exclude` patterns to reduce scanning overhead
- **Large directories (> 10,000 files)**: Consider splitting into multiple scan steps or using more specific patterns
- **Recursive scans**: Increase checkpoint intervals for large directory trees

---

### read_id_list

Reads a deterministic list of object IDs from a file. This step provides deterministic input for migrations, ensuring repeatable execution without dependency on dynamic query capabilities.

**Configuration:**
```yaml
type: read_id_list
config:
  file: string               # Required: Path to file containing IDs (relative to project root or absolute)
```

**File Format Support:**
- **YAML List** (preferred): Standard YAML list format
  ```yaml
  - ID-001
  - ID-002
  - ID-003
  ```
- **Line-by-line text**: One ID per line, empty lines and `#` comments are ignored
  ```
  ID-001
  ID-002
  # Comment line
  ID-003
  ```

**Output:**
```yaml
output:
  ids: []string              # List of object IDs
  count: int                 # Number of IDs read
```

**Performance Considerations:**
- **Small lists (< 1,000 IDs)**: No special considerations
- **Large lists (1,000-100,000 IDs)**: File read is O(n) and efficient; memory usage is minimal
- **Very large lists (> 100,000 IDs)**: Consider splitting into multiple files or batches if memory is constrained
- **File I/O**: Single file read operation, minimal overhead

**Usage Pattern:**
This step is designed for deterministic migrations. Generate the ID list file separately (e.g., using `zqk system generate-lifecycle-id-list` (PRUNED)) to ensure repeatable migration execution.

---

### read_objects

Reads objects from storage by ID list. This step enables backend-agnostic migrations by reading objects directly from storage.

**Configuration:**
```yaml
type: read_objects
depends_on:
  - step_id                  # Required: Step that outputs IDs (e.g., read_id_list)
config:
  kind: string               # Optional: Filter by object kind (for validation)
```

**Input Source:**
- Must depend on a step that outputs `ids: []string` (typically `read_id_list`)

**Output:**
```yaml
output:
  items: []map[string]any    # List of object maps
  count: int                 # Number of objects successfully read
  errors: int                # Number of objects that failed to read
```

**Performance Considerations:**
- **Small batches (< 100 objects)**: Sequential reads are acceptable
- **Medium batches (100-1,000 objects)**: Consider batch size tuning in subsequent steps
- **Large batches (1,000-10,000 objects)**: 
  - Memory usage: ~1-10 MB per 1,000 objects (depends on object size)
  - Storage I/O: Sequential reads, each object requires a storage read operation
  - **Recommendation**: Process in batches using checkpoint intervals
- **Very large batches (> 10,000 objects)**: 
  - Consider splitting into multiple `read_objects` steps
  - Use checkpoint intervals to manage memory and enable recovery
  - Monitor storage backend performance (file I/O vs. graph queries)

**Device-Specific Recommendations:**
- **Low-memory devices (< 4 GB RAM)**: Use checkpoint intervals of 100-500 objects
- **Standard devices (4-16 GB RAM)**: Checkpoint intervals of 500-2,000 objects are reasonable
- **High-memory devices (> 16 GB RAM)**: Checkpoint intervals of 2,000-10,000 objects are acceptable

---

### transform

Transforms data from one format to another. Supports both file-based (from `scan_files`) and object-based (from `read_objects`) transformations.

**Configuration:**
```yaml
type: transform
for_each: step_id            # Required: Reference to previous step (scan_files or read_objects)
config:
  transform:
    builder: string          # Required: Builder name (e.g., "lifecycle_instance_builder", "lifecycle_id_transform")
    version: string          # Optional: Version string (default: "v1_0_0")
```

**Supported Builders:**
- **lifecycle_instance_builder**: Transforms lifecycle YAML files to lifecycle objects (file-based)
- **lifecycle_id_transform**: Transforms lifecycle object IDs from old format to new format (object-based)

**Input Sources:**
- **From `scan_files`**: Processes `files: []FileInfo` output
- **From `read_objects`**: Processes `items: []map[string]any` output

**Output:**
```yaml
output:
  items: []map[string]any    # List of transformed objects
  count: int                 # Number of items transformed
```

**Performance Considerations:**
- **Transformation overhead**: Typically O(n) where n is the number of items
- **Memory usage**: Maintains input and output in memory simultaneously (2x memory footprint during transformation)
- **CPU usage**: Transformation logic is typically CPU-bound (parsing, validation, ID generation)
- **Large batches (> 5,000 items)**: 
  - Consider checkpoint intervals to manage memory
  - Monitor CPU usage for complex transformations
- **Error handling**: Transformation errors stop the step unless `continue_on_error: true` in options

**Device-Specific Recommendations:**
- **Low-memory devices**: Process in smaller batches, use checkpoint intervals
- **Low-CPU devices**: Expect longer transformation times for complex transformations
- **Standard devices**: Handle 1,000-5,000 items efficiently
- **High-performance devices**: Can process 10,000+ items without issues

---

### create_objects

Creates objects in storage. Processes objects from a previous transform step.

**Configuration:**
```yaml
type: create_objects
depends_on:
  - step_id                  # Required: Step that outputs items (typically transform)
config:
  skip_existing: bool        # Optional: Skip objects that already exist (default: true)
  force: bool                # Optional: Overwrite existing objects (default: false, overridden by --force flag)
  batch_size: int            # Optional: Process objects in batches (default: use options.batch_size)
```

**Input Source:**
- Must depend on a step that outputs `items: []map[string]any` (typically `transform`)

**Output:**
```yaml
output:
  created: int               # Number of objects successfully created
  skipped: int               # Number of objects skipped (already exist)
  errors: int                # Number of objects that failed to create
```

**Step-Level Configuration:**
```yaml
atomic: bool                 # Optional: Step must complete atomically (default: false)
checkpoint:
  interval: int              # Optional: Create checkpoint every N objects (overrides options.checkpoint_interval)
  snapshot: bool             # Optional: Create snapshot at checkpoint (default: false)
  rollback_point: bool       # Optional: Enable rollback to checkpoint (default: false)
tracking:
  location_mapping: bool     # Optional: Track object location mappings (default: false)
  object_mapping: bool       # Optional: Track object ID mappings (default: false)
```

**Performance Considerations:**
- **Storage I/O**: Each object requires a storage write operation
- **Batch processing**: Objects are processed sequentially (not in parallel) to ensure consistency
- **Checkpoint overhead**: Creating checkpoints adds overhead; balance checkpoint frequency with recovery granularity
- **Memory usage**: Objects are processed one at a time, minimal memory overhead beyond object size
- **Large batches (> 1,000 objects)**:
  - Use checkpoint intervals to enable recovery
  - Monitor storage backend performance (file I/O vs. graph writes)
  - Consider splitting into multiple `create_objects` steps if migration fails frequently

**Device-Specific Recommendations:**
- **Low-storage I/O devices (HDD, network storage)**: 
  - Use smaller checkpoint intervals (50-200 objects)
  - Expect longer execution times
- **High-storage I/O devices (SSD, local storage)**: 
  - Larger checkpoint intervals are acceptable (500-2,000 objects)
  - Faster execution times
- **Checkpoint strategy**: 
  - More frequent checkpoints = faster recovery but more overhead
  - Less frequent checkpoints = less overhead but coarser recovery granularity
  - **Recommended**: Start with checkpoint interval of 10-20% of total objects, adjust based on failure rates

**Error Handling:**
- **skip_existing: true**: Objects that already exist are skipped (counted in `skipped`)
- **skip_existing: false**: Attempting to create existing objects results in errors (unless `force: true`)
- **force: true**: Overwrites existing objects (use with caution)
- Errors are counted but execution continues unless `continue_on_error: false` in options

---

### delete_objects

Deletes objects from storage by ID list. Supports cascade deletion for dependent objects.

**Configuration:**
```yaml
type: delete_objects
depends_on:
  - step_id                  # Required: Step that outputs ids or items (read_id_list, transform, or create_objects)
config:
  cascade: bool              # Optional: Cascade delete to dependent objects (default: false)
```

**Input Sources:**
- **From `read_id_list`**: Uses `ids: []string` output
- **From `transform` or `create_objects`**: Extracts `id` field from `items: []map[string]any` output

**Output:**
```yaml
output:
  deleted: int               # Number of objects successfully deleted
  skipped: int               # Number of objects skipped (not found)
  errors: int                # Number of objects that failed to delete
```

**Performance Considerations:**
- **Cascade delete**: When `cascade: true`, deletion may involve multiple objects per ID (dependents)
  - **Performance impact**: O(d) where d is the depth/breadth of dependency tree
  - **Storage I/O**: Each dependent object requires a delete operation
  - **Use with caution**: Cascade delete can be expensive for objects with many dependents
- **Non-cascade delete**: Simple O(1) operation per ID
- **Large batches (> 1,000 objects)**:
  - Sequential deletion ensures consistency
  - Monitor storage backend performance
  - Consider checkpoint intervals for very large batches (though deletion is typically fast)
- **Dry-run mode**: No actual deletions, useful for validation

**Device-Specific Recommendations:**
- **Standard devices**: Handle 1,000-10,000 deletions efficiently (non-cascade)
- **Cascade deletions**: Performance depends on dependency graph depth; test with small batches first
- **Network storage**: Expect slower deletion times, consider batching

**Safety Considerations:**
- **Always test with `--dry-run` first** to validate which objects will be deleted
- **Cascade delete**: Ensure you understand the dependency graph before enabling
- **Backup**: Ensure snapshots are created before deletion steps (configure `pre_migration_snapshot`)

---

### execute_command

Executes a CLI command for complex migrations that cannot be expressed through standard steps.

**Configuration:**
```yaml
type: execute_command
config:
  command: string            # Required: Command to execute
  args: []string             # Optional: Command arguments
  env: map[string]string     # Optional: Environment variables
```

**Output:**
```yaml
output:
  success: bool              # Whether command succeeded
  output: string             # Command stdout
  error: string              # Command stderr (if failed)
```

**Performance Considerations:**
- **Command execution**: Performance depends entirely on the command being executed
- **Blocking**: Command execution blocks migration until completion
- **Resource usage**: Command may consume significant CPU, memory, or I/O resources
- **Use sparingly**: Prefer standard steps when possible for better observability and error handling

---

## Performance Optimization Guidelines

### Batch Size Recommendations

**Default Batch Sizes:**
- **Small migrations (< 100 objects)**: `batch_size: 10-50`
- **Medium migrations (100-1,000 objects)**: `batch_size: 50-200`
- **Large migrations (1,000-10,000 objects)**: `batch_size: 200-1,000`
- **Very large migrations (> 10,000 objects)**: `batch_size: 500-2,000`

**Considerations:**
- Larger batch sizes reduce overhead but increase memory usage and recovery granularity
- Smaller batch sizes provide finer-grained progress and recovery but increase overhead
- Adjust based on device capabilities and storage backend performance

### Checkpoint Interval Strategy

**Checkpoint Frequency Guidelines:**
- **Small migrations (< 500 objects)**: Checkpoint interval of 50-100 objects
- **Medium migrations (500-5,000 objects)**: Checkpoint interval of 100-500 objects
- **Large migrations (5,000-50,000 objects)**: Checkpoint interval of 500-2,000 objects
- **Very large migrations (> 50,000 objects)**: Checkpoint interval of 1,000-5,000 objects

**Checkpoint Overhead:**
- Creating checkpoints adds I/O overhead (writes checkpoint state)
- More frequent checkpoints = faster recovery but more overhead
- Less frequent checkpoints = less overhead but coarser recovery granularity
- **Rule of thumb**: Checkpoint interval should be 10-20% of total objects, adjusted for device capabilities

**Checkpoint Configuration:**
```yaml
checkpoint:
  interval: 500              # Create checkpoint every N objects
  snapshot: false            # Create snapshot at checkpoint (expensive, use sparingly)
  rollback_point: true       # Enable rollback to checkpoint (recommended for critical migrations)
```

### Device-Specific Performance Tuning

**Low-Memory Devices (< 4 GB RAM):**
- Use smaller batch sizes (10-50)
- Use smaller checkpoint intervals (50-200)
- Process in smaller chunks (split large migrations into multiple steps)
- Monitor memory usage during execution

**Standard Devices (4-16 GB RAM):**
- Default batch sizes and checkpoint intervals are appropriate
- Can handle migrations of 1,000-10,000 objects efficiently
- Adjust based on storage backend performance

**High-Memory Devices (> 16 GB RAM):**
- Can use larger batch sizes (500-2,000)
- Can use larger checkpoint intervals (1,000-5,000)
- Can handle very large migrations (10,000+ objects) efficiently

**Storage Backend Considerations:**
- **File storage (HDD)**: Smaller batch sizes, more frequent checkpoints
- **File storage (SSD)**: Standard batch sizes and checkpoint intervals
- **Graph backend**: Performance depends on graph implementation; test and adjust accordingly
- **Network storage**: Smaller batch sizes, account for network latency

### Resource Usage Estimates

**Memory Usage:**
- **Per object**: ~1-10 KB (depends on object complexity)
- **Batch of 1,000 objects**: ~1-10 MB
- **Batch of 10,000 objects**: ~10-100 MB
- **Checkpoint state**: Minimal (~1-5 KB per checkpoint)

**Storage I/O:**
- **Read operations**: 1 per object in `read_objects` step
- **Write operations**: 1 per object in `create_objects` step
- **Delete operations**: 1 per object (plus dependents if cascade) in `delete_objects` step
- **Checkpoint writes**: 1 per checkpoint interval

**CPU Usage:**
- **Transform operations**: CPU-bound, depends on transformation complexity
- **Storage operations**: I/O-bound, minimal CPU usage
- **Checkpoint operations**: Minimal CPU usage

### Performance Monitoring

**Metrics to Monitor:**
- Execution time per step
- Objects processed per second
- Memory usage during execution
- Storage I/O operations
- Error rates

**Optimization Strategy:**
1. Start with conservative batch sizes and checkpoint intervals
2. Monitor performance metrics during dry-run or small test migrations
3. Gradually increase batch sizes and checkpoint intervals based on device capabilities
4. Adjust based on failure rates and recovery needs

## Snapshot Compatibility (CRITICAL)

**Every aspect of the kernel must be coherent and consistent at near real-time to enable snapshot/restore capabilities. Snapshots and compression are among the most valuable aspects of this system - the ability to copy itself very efficiently (spores).**

### Requirements

1. **Pre-Migration Snapshots**: Auto-create snapshot before migration
2. **Reversibility**: Can restore pre-migration snapshot even if objects changed location/fields
3. **State Coherence**: System state remains consistent during migration
4. **Location Tracking**: Track object location changes (e.g., files → CAS, directory changes)
5. **Field Tracking**: Track field transformations (renames, type changes)
6. **Post-Migration Snapshots**: Auto-create snapshot after migration
7. **Spore Compatibility**: Snapshots enable efficient system copying (spores)

See `MIGRATION_SNAPSHOT_COHERENCE.md` for detailed requirements.

## Migration Execution

### Command Structure

```bash
# Run a migration spec (auto-creates snapshots)
zqk system migrate (PRUNED) <spec-file>

# Dry run (no snapshots)
zqk system migrate (PRUNED) <spec-file> --dry-run

# Force (skip validation, overwrite existing)
zqk system migrate (PRUNED) <spec-file> --force

# Continue on error
zqk system migrate (PRUNED) <spec-file> --continue-on-error

# Rollback to pre-migration snapshot
zqk system migrate (PRUNED) <spec-file> --rollback

# List available migration specs
zqk system migrate --list (PRUNED)

# Show migration status
zqk system migrate --status (PRUNED)
```

### Execution Flow

1. **Load Spec**: Parse migration spec YAML
2. **Validate Prerequisites**: Check all prerequisites are met
3. **Create Pre-Migration Snapshot**: Auto-create snapshot if required
4. **Execute Steps**: Run steps in order, respecting dependencies
   - Track object location mappings
   - Track field transformations
   - Create checkpoints at intervals
   - Validate coherence after each step
5. **Validate Results**: Run validation checks
6. **Create Post-Migration Snapshot**: Auto-create snapshot
7. **Test Restore**: Validate restore from pre-migration snapshot (if enabled)
8. **Store Mappings**: Save location and field mappings for future restore
9. **Report Results**: Show summary of migration

### Step Execution

- Steps execute sequentially
- `depends_on` defines step dependencies
- `for_each` processes items from previous step
- Steps can output data for subsequent steps
- Errors stop execution unless `continue_on_error: true`

## Migration Spec Location

Migration specs should be stored in:

```
.zqk/specs/migrations/
├── lifecycle-files-to-objects.yaml
├── lifecycle-id-migration.yaml
├── audit-to-buckets.yaml
├── cas-migration.yaml
└── README.md
```

## Benefits

1. **Declarative**: Migration steps are clearly defined
2. **Reusable**: Steps can be reused across migrations
3. **Testable**: Can test migration specs in isolation
4. **Auditable**: Migration specs serve as documentation
5. **Rollback**: Rollback steps defined in spec
6. **Automated**: Common patterns can be automated

## Migration from Individual Commands

Existing migration commands should be refactored to migration specs:

1. `migrate-lifecycles` → `lifecycle-files-to-objects.yaml`
2. `migrate-audit-buckets` → `audit-to-buckets.yaml`
3. `migrate-cas` → `cas-migration.yaml`

The individual commands can be deprecated or converted to wrappers that load the migration spec.
