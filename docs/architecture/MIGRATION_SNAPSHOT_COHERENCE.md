# Migration Snapshot Coherence

**Created:** 2026-01-12  
**Status:** Design  
**Purpose:** Ensure migrations maintain system coherence for snapshot/restore capabilities

## Core Principle

**Every aspect of the kernel (the meaningful state of the system) needs to be coherent and consistent at near real-time such that we can take a snapshot from pre-migration and restore it even if objects have changed location or fields.**

## Key Requirements

### 1. Snapshot Compatibility

Migrations must be **snapshot-compatible**:
- Snapshots taken before migration must restore correctly after migration
- Object location changes (e.g., files → CAS, directory changes) must be reversible
- Field transformations (e.g., field renames, type changes) must be reversible
- System state must remain coherent throughout migration

### 2. State Coherence

The system must maintain **near real-time coherence**:
- No inconsistent intermediate states during migration
- Atomic migration operations where possible
- Transaction-like behavior for multi-step migrations
- Rollback capability at any point

### 3. Spore Creation

The system must support **efficient copying (spores)**:
- Create complete system copies efficiently
- Snapshots enable spore creation
- Migrations must not break spore functionality
- Spores must work across migration boundaries

## Migration Design Implications

### Migration Tracking

All migrations must track:

```yaml
# Migration spec with snapshot compatibility
schema_version: 1.0.0
id: migration-lifecycle-files-to-objects
name: Migrate Lifecycle Files to Objects

# Snapshot metadata
snapshot_compatible: true
snapshot_tags:
  - lifecycle_migration_v1
  - pre_migration_state

# Migration tracking
tracking:
  enabled: true
  object_mapping: true  # Track object ID -> location mappings
  field_mapping: true   # Track field transformations
  state_changes: true   # Track state changes

# State coherence
coherence:
  atomic_steps: true    # Steps that must be atomic
  checkpoint_interval: 100  # Create checkpoint every N objects
  rollback_points: true     # Enable rollback at checkpoints

# Pre-migration snapshot
pre_migration_snapshot:
  required: true
  auto_create: true
  tags: ["pre_lifecycle_migration"]

# Post-migration snapshot
post_migration_snapshot:
  auto_create: true
  tags: ["post_lifecycle_migration"]
  validate: true
```

### Object Location Mapping

Migrations that change object locations must track mappings:

```yaml
steps:
  - id: migrate_objects
    type: create_objects
    tracking:
      location_mapping: true
      mapping_format:
        source_location: "{old_path}"
        target_location: "{new_path}"
        object_id: "{id}"
        migration_id: "{migration_id}"
    
    # Store mappings for restore
    output:
      location_mappings: []LocationMapping
```

### Field Transformation Tracking

Migrations that transform fields must track transformations:

```yaml
steps:
  - id: transform_fields
    type: transform
    tracking:
      field_mapping: true
      transformations:
        - source_field: "old_field_name"
          target_field: "new_field_name"
          transform: "rename"
        - source_field: "status"
          transform: "enum_mapping"
          mapping:
            "old_status": "new_status"
    
    output:
      field_mappings: []FieldMapping
```

### Rollback Capability

Every migration must support rollback:

```yaml
rollback:
  strategy: snapshot_restore  # Use snapshot for rollback
  fallback: step_by_step      # Step-by-step reversal if snapshot unavailable
  
  steps:
    # Reverse operations in reverse order
    - id: delete_new_objects
      type: delete_objects
      filter:
        migration_id: "{migration_id}"
    
    - id: restore_old_objects
      type: restore_from_snapshot
      snapshot_tag: "pre_lifecycle_migration"
      location_mappings: "{location_mappings}"
      field_mappings: "{field_mappings}"
```

## Snapshot Integration

### Pre-Migration Snapshots

Before any migration:

1. **Auto-create snapshot** with migration-specific tag
2. **Capture current state**: All object locations, field values
3. **Store metadata**: Migration ID, timestamp, state hash
4. **Validate coherence**: Ensure system is in consistent state

```go
// Pre-migration snapshot creation
snapshotID, err := snapshot.CreatePreMigrationSnapshot(migrationID, tags)
if err != nil {
    return fmt.Errorf("failed to create pre-migration snapshot: %w", err)
}

// Store snapshot reference in migration instance
migrationInstance["pre_migration_snapshot_id"] = snapshotID
```

### Post-Migration Snapshots

After successful migration:

1. **Create post-migration snapshot**
2. **Store migration mappings**: Location and field mappings
3. **Validate restoration**: Test restore from pre-migration snapshot
4. **Update snapshot metadata**: Link pre/post snapshots

```go
// Post-migration snapshot
postSnapshotID, err := snapshot.CreatePostMigrationSnapshot(
    migrationID,
    preMigrationSnapshotID,
    locationMappings,
    fieldMappings,
)
```

### Restore from Snapshot

Restoring a pre-migration snapshot:

1. **Load snapshot** with migration tag
2. **Apply location mappings** (reverse): New locations → old locations
3. **Apply field mappings** (reverse): New fields → old fields
4. **Restore objects** to pre-migration state
5. **Validate coherence**: Ensure system is consistent

```go
// Restore from pre-migration snapshot
err := snapshot.RestoreFromSnapshot(
    preMigrationSnapshotID,
    ReverseLocationMappings(locationMappings),
    ReverseFieldMappings(fieldMappings),
)
```

## State Coherence During Migration

### Atomic Operations

Critical steps must be atomic:

```yaml
steps:
  - id: atomic_migration_step
    type: create_objects
    atomic: true  # Must complete entirely or rollback
    coherence_check: true  # Validate coherence after step
    config:
      batch_size: 100
      transaction: true
```

### Checkpoints

Create checkpoints for large migrations:

```yaml
steps:
  - id: large_migration
    type: create_objects
    checkpoint:
      interval: 100  # Checkpoint every 100 objects
      snapshot: true # Create snapshot at checkpoint
      rollback_point: true  # Can rollback to checkpoint
```

### Coherence Validation

Validate coherence at each step:

```yaml
validation:
  - type: coherence_check
    after_each_step: true
    checks:
      - object_references_valid
      - field_references_valid
      - location_consistency
      - state_hash_match
```

## Spore Creation

### Efficient System Copying

Snapshots enable efficient spore creation:

1. **Create snapshot** of entire system state
2. **Copy snapshot** (much faster than copying all files)
3. **Restore snapshot** to new location (spore)
4. **Verify coherence** of new spore

```bash
# Create spore from snapshot
zqk snapshot create --tag system_state
zqk spore create --from-snapshot system_state --target /path/to/spore
```

### Migration-Aware Spores

Spores must work across migrations:

- Spores contain migration metadata
- Can restore spore even if system has migrated
- Migration mappings enable cross-migration compatibility
- Spores can be "upgraded" to current migration state

## Implementation Requirements

### Migration Executor

The migration executor must:

1. **Create pre-migration snapshot** (auto)
2. **Track all changes** (locations, fields, state)
3. **Create checkpoints** (configurable intervals)
4. **Validate coherence** (after each step)
5. **Support rollback** (from snapshot or step-by-step)
6. **Create post-migration snapshot** (auto)
7. **Store mappings** (for restore capability)

### Snapshot System Integration

Snapshots must:

1. **Capture object locations** (paths, CAS hashes, etc.)
2. **Capture field values** (complete state)
3. **Store migration metadata** (mappings, transformations)
4. **Support restore with mappings** (location and field reversal)
5. **Validate coherence** (before/after restore)

### Migration Registry

Track all migrations:

```yaml
# Migration registry entry
migrations:
  - id: migration-lifecycle-files-to-objects
    status: completed
    pre_snapshot: snapshot-pre-lifecycle-migration-001
    post_snapshot: snapshot-post-lifecycle-migration-001
    location_mappings: mappings/lifecycle-migration-locations.yaml
    field_mappings: mappings/lifecycle-migration-fields.yaml
    rollback_available: true
```

## Benefits

1. **Reversibility**: Can restore to any pre-migration state
2. **Safety**: Snapshots provide safety net
3. **Efficiency**: Efficient system copying via snapshots
4. **Coherence**: System state always consistent
5. **Debugging**: Can reproduce issues from snapshots
6. **Testing**: Test migrations safely with snapshot restore
7. **Spores**: Enable efficient system replication

## Migration Spec Updates

Migration specs must include:

```yaml
# Required fields for snapshot compatibility
snapshot_compatible: true
pre_migration_snapshot:
  required: true
  auto_create: true
post_migration_snapshot:
  auto_create: true
tracking:
  object_mapping: true
  field_mapping: true
coherence:
  atomic_steps: true
  checkpoint_interval: 100
  validate_after_each_step: true
rollback:
  strategy: snapshot_restore
  fallback: step_by_step
```

## Examples

### Example: Lifecycle Files → Objects Migration

```yaml
id: migration-lifecycle-files-to-objects
snapshot_compatible: true

pre_migration_snapshot:
  tags: ["pre_lifecycle_migration"]
  auto_create: true

steps:
  - id: scan_files
    type: scan_files
    # ...
  
  - id: convert_and_create
    type: create_objects
    tracking:
      location_mapping: true
      mappings:
        source: "docs/architecture/_internal/lifecycles/{file}"
        target: "docs/architecture/lifecycles/{hash}.yaml"
    checkpoint:
      interval: 50
      snapshot: true

post_migration_snapshot:
  tags: ["post_lifecycle_migration"]
  validate_restore: true  # Test restore from pre-migration snapshot

rollback:
  strategy: snapshot_restore
  snapshot_tag: "pre_lifecycle_migration"
  apply_location_mappings: true
```

This ensures the migration is fully reversible and snapshot-compatible.
