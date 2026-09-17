# Display Length Constraints

**Version**: 1.0.0  
**Status**: Active  
**Created**: 2026-01-25  
**Category**: Specification Guidelines

## Overview

The `display_length` constraint specifies the recommended maximum character width for displaying field values in table formats. This ensures consistent, readable table layouts across all CLI commands and prevents truncation issues.

## Purpose

Display length constraints serve multiple purposes:

1. **Table Formatting**: Provides consistent column widths for table output
2. **UI Consistency**: Ensures all commands using the same field display it with the same width
3. **Truncation Control**: Allows spec authors to control how values are truncated when they exceed the constraint
4. **Validation**: Validators can warn when values exceed display_length, helping identify fields that may need constraint adjustments

## Specification

### Location

The `display_length` constraint is specified in the `validation` section of a field definition:

```yaml
fields:
    job_type:
        type: enum
        traits:
            - listable
        validation:
            required: true
            display_length: 28  # Maximum characters for table display
```

### Requirements

1. **Required for Listable Fields**: All fields with the `listable` trait SHOULD have a `display_length` constraint
2. **Positive Integer**: `display_length` must be a positive integer (greater than 0)
3. **Separate from max_length**: `display_length` is for UI/table formatting, not data validation. Use `max_length` for data validation constraints

### Guidelines

#### Choosing Display Length

- **Short Identifiers** (IDs, codes): 12-15 characters
  - Example: `id: display_length: 12` (for IDs like "SCH-001", "BLI-123")
  
- **Medium Text** (titles, names): 30-50 characters
  - Example: `title: display_length: 50`
  
- **Long Text** (descriptions, bodies): 60-100 characters
  - Example: `description: display_length: 80`
  
- **Enums** (status, type): Match longest enum value + 2-3 characters padding
  - Example: `status: display_length: 15` (for "in_progress", "archived", etc.)
  
- **Cron Expressions**: 20-25 characters
  - Example: `schedule_expression: display_length: 20`

#### Best Practices

1. **Consider Longest Expected Value**: Set `display_length` to accommodate the longest expected value, plus 2-3 characters for padding
2. **Balance Readability vs. Screen Space**: Too wide wastes screen space, too narrow causes excessive truncation
3. **Consistent Across Similar Fields**: Use similar display_length values for similar field types across specs
4. **Review After Adding New Enum Values**: If new enum values are longer than existing ones, consider increasing `display_length`

## Validation

### Spec Validation

The spec validator checks:

1. **Listable Fields**: Fields with `listable` trait should have `display_length` set
2. **Type Validation**: `display_length` must be an integer (not string, float, etc.)
3. **Value Validation**: `display_length` must be positive (greater than 0)

**Error Messages**:
- `Field {name} has 'listable' trait but missing display_length constraint in validation - recommended for proper table formatting`
- `Field {name} has invalid display_length type: must be an integer, got {type}`
- `Field {name} has invalid display_length value: must be positive, got {value}`

### Instance Validation

The instance validator (GoValidator) warns when:

1. **Value Exceeds Constraint**: String values longer than `display_length` generate warnings
2. **Truncation Warning**: Warns that values may be truncated in table displays

**Warning Message**:
- `Field {name} value length ({actual}) exceeds display_length constraint ({constraint}) - may be truncated in table displays`

**Note**: This is a warning, not an error. Values exceeding `display_length` are still valid data - they just may be truncated in table displays.

## Usage in Code

### Getting Display Length

Use the shared utility function:

```go
import "github.com/lanceman/zqk/pkg/objects"

spec, _ := specLoader.LoadSpecWithInheritance("scheduler_job.yaml")
displayLen := objects.GetDisplayLength(spec, "job_type", 28) // 28 is default if not found
```

### Table Formatting

```go
// Get display_length from spec
colWidth := objects.GetDisplayLength(spec, "field_name", 20)

// Truncate if needed
truncate := func(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen-3] + "..."
}

value := truncate(fieldValue, colWidth)
```

## Examples

### Base Object Fields

```yaml
id:
    traits:
        - listable
    validation:
        required: true
        display_length: 12  # For IDs like "SCH-001", "BLI-123"

title:
    traits:
        - listable
    validation:
        required: true
        display_length: 50  # For titles up to 50 chars

status:
    traits:
        - listable
    validation:
        required: true
        display_length: 15  # For statuses like "in_progress"
```

### Scheduler Job Fields

```yaml
job_type:
    traits:
        - listable
    validation:
        required: true
        display_length: 28  # For "audit_event_aggregation", "change_journal_aggregation"

trigger_type:
    traits:
        - listable
    validation:
        required: true
        display_length: 12  # For "timer", "immediate", "lifecycle"

schedule_expression:
    traits:
        - listable
    validation:
        required: false
        display_length: 20  # For cron expressions like "0 2 * * *"
```

## Migration

### Adding display_length to Existing Specs

1. Identify fields with `listable` trait
2. Determine appropriate `display_length` based on:
   - Longest current value in production data
   - Longest possible enum value
   - Expected maximum length
3. Add `display_length` to `validation` section
4. Run spec validation to verify
5. Test table output to ensure proper formatting

### Example Migration

**Before**:
```yaml
job_type:
    traits:
        - listable
    validation:
        required: true
```

**After**:
```yaml
job_type:
    traits:
        - listable
    validation:
        required: true
        display_length: 28
```

## Related

- `pkg/objects/display_length.go` - Shared utility functions
- `pkg/objects/spec_validator.go` - Spec validation for display_length
- `pkg/validation/go_validator.go` - Instance validation warnings for display_length
- Object Spec Guidelines (this directory)

