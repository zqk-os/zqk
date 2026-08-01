# Field Versioning System

**Version**: 1.0.0  
**Status**: Implemented  
**Date**: 2026-01-05  
**Related**: Spec Management, Breaking Changes

## Overview

The field versioning system tracks the lifecycle and changes of fields in object specifications. It enables:
- **Field Creation**: Track when fields are added
- **Field Modification**: Track changes with breaking change detection
- **Field Deprecation**: Mark fields as deprecated before removal
- **Field Archival**: Archive fields while keeping history
- **Field Deletion**: Track deleted fields with replacement information

## Field Lifecycle States

Fields can be in one of the following states:

1. **`active`**: Field is active and in use (default)
2. **`created`**: Field was just created
3. **`modified`**: Field was modified (may be breaking)
4. **`deprecated`**: Field is deprecated (will be removed)
5. **`archived`**: Field is archived (removed but kept for history)
6. **`deleted`**: Field is deleted

## Field Version Info Structure

Each field can have a `version_info` section:

```yaml
fields:
    my_field:
        type: string
        traits:
            - readable
            - writable
        version_info:
            state: active
            created_at: "2026-01-05T12:00:00Z"
            created_by: account:system
            version: "1.0.0"
            breaking_change: false
```

### Version Info Fields

- **`state`**: Current lifecycle state
- **`created_at`**: When field was created (RFC3339)
- **`created_by`**: Who created the field
- **`modified_at`**: When field was last modified
- **`modified_by`**: Who last modified the field
- **`deprecated_at`**: When field was deprecated
- **`deprecated_by`**: Who deprecated the field
- **`archived_at`**: When field was archived
- **`archived_by`**: Who archived the field
- **`deleted_at`**: When field was deleted
- **`deleted_by`**: Who deleted the field
- **`version`**: Field version (SemVer, e.g., "1.0.0")
- **`previous_version`**: Previous version (for modified fields)
- **`breaking_change`**: Whether this change is breaking
- **`change_reason`**: Reason for the change
- **`migration_notes`**: Notes for migrating existing data
- **`replaced_by`**: Field that replaces this one (for deprecated/deleted)

## Breaking Change Detection

The system automatically detects breaking changes when fields are modified:

### Breaking Changes Include:

1. **Type Changes**: Changing field type (e.g., `string` → `integer`)
2. **Required Changes**: Making an optional field required
3. **Enum Value Removal**: Removing values from enum
4. **Pattern Changes**: Changing validation pattern (stricter)
5. **Trait Removal**: Removing critical traits (`readable`, `writable`)

### Non-Breaking Changes Include:

1. **Adding Enum Values**: Adding new enum values
2. **Making Required Optional**: Making a required field optional
3. **Adding Traits**: Adding new traits
4. **Relaxing Patterns**: Making validation patterns less strict

## Usage

### Creating a Field

```bash
# Create a new field
zqk system update-specs backlog_item \
  --field new_field_name \
  --operation define \
  --reason "Adding new field for feature X"
```

### Modifying a Field

```bash
# Modify a field (breaking change)
zqk system update-specs backlog_item \
  --field existing_field \
  --operation modify \
  --breaking-change \
  --reason "Changing type to support new requirements" \
  --migration-notes "Existing data will be migrated automatically"

# Modify a field (non-breaking)
zqk system update-specs backlog_item \
  --field existing_field \
  --operation modify \
  --reason "Adding new enum value"
```

### Deprecating a Field

```bash
# Deprecate a field
zqk system update-specs backlog_item \
  --field old_field \
  --operation deprecate \
  --replaced-by new_field \
  --reason "Replaced by new_field for better semantics"
```

### Archiving a Field

```bash
# Archive a field
zqk system update-specs backlog_item \
  --field deprecated_field \
  --operation archive \
  --reason "Field no longer used, keeping for history"
```

### Deleting a Field

```bash
# Delete a field (marks as deleted, keeps in spec for history)
zqk system update-specs backlog_item \
  --field obsolete_field \
  --operation delete \
  --replaced-by new_field \
  --reason "Field replaced by new_field"
```

## Version Numbering

Field versions follow SemVer:
- **Major version bump**: Breaking changes (e.g., "1.0.0" → "2.0.0")
- **Minor version bump**: Non-breaking changes (e.g., "1.0.0" → "1.1.0")
- **Initial version**: "1.0.0" for new fields

## Field Definition File Format

For **define** (or **add** / **create**) and **modify** operations, provide a field definition file:

```yaml
# backlog_item_field_new_field.yaml
type: string
traits:
    - readable
    - writable
    - modifiable
checklist:
    purpose: Description of the field
    authority: owner
    # ... other checklist items
validation:
    required: false
```

## Programmatic API

### Go API

```go
import "github.com/lanceman/zqk/pkg/objects"

// Get version info from field
versionInfo := objects.GetFieldVersionInfo(fieldDef)

// Mark field as created
objects.MarkFieldCreated(fieldDef, "account:user")

// Mark field as modified
objects.MarkFieldModified(fieldDef, "account:user", true, "reason", "migration notes")

// Detect breaking changes
breaking, reasons := objects.DetectBreakingChange(oldFieldDef, newFieldDef)
```

## Best Practices

1. **Always provide reasons**: Document why fields are changed
2. **Migration notes**: Provide guidance for breaking changes
3. **Replacement fields**: Always specify `replaced_by` when deprecating/deleting
4. **Version tracking**: Let the system auto-increment versions
5. **Breaking change detection**: Review detected breaking changes before applying

## Integration with Spec Writer

The `SpecWriter` automatically:
- Tracks field lifecycle states
- Detects breaking changes
- Increments version numbers
- Maintains change history
- Provides migration guidance

## Related Documentation

- [Spec Management](./spec-persistence-gap-analysis.md)
- [Guided Spec Management](./guided-spec-management-v1.0.md)
- [Breaking Changes Policy](../policies/BREAKING_CHANGES.md) (to be created)

