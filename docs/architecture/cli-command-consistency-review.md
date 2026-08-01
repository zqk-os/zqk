# CLI Command Structure Consistency Review

## Overview
This document reviews the consistency and clarity of CLI commands that manipulate, sort, group, and aggregate system objects (both internal and public).

## Command Groups

### 1. Object Commands (`zqk object`)
Commands for managing public objects:
- `create <kind>` - Create a new object
- `list [kind]` - List objects with filtering, sorting, grouping, pagination
- `get <id>` - Get an object by ID
- `update <id>` - Update an object
- `delete <id>` - Delete an object
- `count [kind]` - Count objects
- `fields [kind]` - List available fields
- `bulk` - Bulk operations (create, update, get, delete)

### 2. Internal Commands (`zqk internal`)
Commands for managing internal and built-in objects (admin-only):
- `create <kind>` - Create an internal object
- `list [kind]` - List internal/built-in objects
- `get <id>` - Get an internal object by ID
- `update <id>` - Update an internal object
- `delete <id>` - Delete an internal object

### 3. System Commands (`zqk system`)
System-level operations:
- `check` - Object health and compliance check

### 4. Utility Commands (`zqk utility`)
Utility operations:
- `version` - Show version information
- `migrate` - Migration tools

## Consistency Issues Identified

### 1. Missing Capabilities in `internal list`
**Issue**: `internal list` lacks several capabilities that `object list` has:
- ❌ No `--filter` flag (filtering support)
- ❌ No `--sort-by` and `--sort-asc` flags (sorting support)
- ❌ No `--offset` and `--limit` flags (pagination support)
- ❌ No `--group-limit` flag (per-group limit support)

**Impact**: Users cannot filter, sort, or paginate internal objects, making it difficult to work with large sets of internal objects.

**Resolution**: Add all missing flags to `internal list` to match `object list` capabilities.

### 2. Output Format Inconsistency
**Issue**: Inconsistent handling of output formats:
- `object` commands use `cli.GetFormat(cmd)` which returns `cli.FormatJSON`, `cli.FormatYAML`, `cli.FormatTable`
- `internal` commands use `string(cli.GetFormat(cmd))` which returns `"json"`, `"yaml"`, `"table"`

**Impact**: Code duplication and potential bugs when adding new formats.

**Resolution**: Standardize on `cli.OutputFormat` type throughout.

### 3. Filter Parsing
**Issue**: `internal list` doesn't support filtering at all, while `object list` and `object count` use `parseFilterString()` helper.

**Impact**: Inconsistent user experience and code duplication.

**Resolution**: Add `parseFilterString()` helper to `internal` package and use it in `internal list`.

### 4. Help Text and Examples
**Issue**: Help text and examples are inconsistent across commands:
- Some commands have detailed examples, others don't
- Flag descriptions vary in detail
- Examples don't always show all available flags

**Resolution**: Standardize help text format and ensure all commands have comprehensive examples.

## Implementation Plan

### Phase 1: Add Missing Flags to `internal list`
1. Add `--filter`, `--sort-by`, `--sort-asc`, `--offset`, `--limit`, `--group-limit` flags
2. Update function signatures to accept new parameters
3. Implement filtering, sorting, and pagination logic

### Phase 2: Standardize Output Format Handling
1. Update `internal` commands to use `cli.OutputFormat` type
2. Update `outputList()` function signature to accept `cli.OutputFormat`
3. Ensure consistent format checking across all commands

### Phase 3: Add Filter Parsing
1. Add `parseFilterString()` helper to `internal` package
2. Use it in `internal list` command
3. Ensure filter syntax matches `object list`

### Phase 4: Standardize Help Text
1. Create template for command help text
2. Update all commands to follow the template
3. Add comprehensive examples showing all flags

## Command Structure Principles

### Consistent Flag Naming
- `--filter` - Filter by field (format: `field=value` or `field:value`)
- `--sort-by` - Field to sort by
- `--sort-asc` - Sort ascending (default: true)
- `--offset` - Pagination offset
- `--limit` - Pagination limit (0 = no limit)
- `--group-by` - Field to group results by
- `--group-limit` - Maximum items per group (0 = no limit)
- `--format` - Output format (table, json, yaml, markdown)
- `--dry-run` - Show what would happen without executing
- `--cascade` - Cascade operation to dependents

### Consistent Command Structure
- All list commands: `list [kind] [flags]`
- All get commands: `get <id> [flags]`
- All create commands: `create <kind> [flags]`
- All update commands: `update <id> [flags]`
- All delete commands: `delete <id> [flags]`

### Consistent Output Format
- All commands support `--format` flag
- Default format is `table`
- JSON and YAML formats are always available
- Format is determined by context profile or flag

## Testing Requirements

1. Test that all list commands support the same filtering, sorting, grouping, and pagination capabilities
2. Test that output formats are consistent across all commands
3. Test that filter syntax works identically in `object list` and `internal list`
4. Test that help text is accurate and complete for all commands

## Status

- [x] Identified consistency issues
- [ ] Phase 1: Add missing flags to `internal list`
- [ ] Phase 2: Standardize output format handling
- [ ] Phase 3: Add filter parsing
- [ ] Phase 4: Standardize help text

