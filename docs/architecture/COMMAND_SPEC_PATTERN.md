# Command Spec Pattern

## Overview

The Command Spec pattern extends the CommandBuilder with a spec-driven approach, following the same pattern used throughout the codebase (specbuilder). This allows commands to be defined declaratively in YAML while maintaining the flexibility of programmatic construction.

## Architecture

```
┌─────────────────┐
│  YAML Spec File │  (Declarative - "what")
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ CommandSpec     │  (Go struct, YAML-serializable)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ SpecBuilder     │  (Bridges spec -> builder)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ CommandBuilder  │  (Programmatic - "how")
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ cobra.Command   │  (Final command)
└─────────────────┘
```

## Example: YAML Spec

```yaml
name: "get <id>"
short: "Get an object by ID"
description: |
  Get an object by its ID.
  
  The object kind is inferred from the ID format (e.g., BLI-001 -> backlog_item).

args:
  type: "exact"
  count: 1

help:
  examples:
    - comment: "Get a backlog item"
      command: "%s get BLI-626"
    - comment: "Get with JSON output"
      command: "%s get BLI-626 --format json"
  exclude_flags:
    - "format"
    - "output"
    - "verbose"
    - "quiet"
    - "timeout"
    - "columns"

run_e: "runGet"
common_flags: true
```

## Example: CRUD Command Spec

```yaml
name: "delete <id>"
short: "Delete an object by ID"
description: |
  Delete an object by its ID.
  
  By default, deletion will fail if the object has dependents.
  Use --cascade to delete the object and all its dependents recursively.

operation_type: "delete"

args:
  type: "exact"
  count: 1

help:
  examples:
    - comment: "Delete an object (fails if it has dependents)"
      command: "%s delete BLI-626"
    - comment: "Delete with cascade"
      command: "%s delete BLI-626 --cascade"
    - comment: "Dry-run to see what would be deleted"
      command: "%s delete BLI-626 --cascade --dry-run"
  exclude_flags:
    - "format"
    - "output"
    - "verbose"
    - "quiet"
    - "timeout"
    - "columns"

flags:
  - name: "cascade"
    type: "bool"
    description: "Delete object and all objects that reference it"
    default: false
  - name: "dry-run"
    type: "bool"
    description: "Show what would be deleted without actually deleting it"
    default: false

run_e: "runDelete"
common_flags: true
```

## Usage

### Programmatic (Current)

```go
func NewGetCmd() *cobra.Command {
	return clipkg.NewCommandBuilder("get <id>").
		WithShort("Get an object by ID").
		WithHelpBuilder(clipkg.DynamicHelpBuilder(...)).
		WithArgs(cobra.ExactArgs(1)).
		WithRunE(runGet).
		WithCommonFlagsDefault(cli.AddCommonFlags).
		Build()
}
```

### Spec-Driven (New)

```go
// Load spec from YAML
spec := loadCommandSpec("get_command.yaml")

// Build command from spec with processor (coordinator pattern)
proc, err := cli.NewProcessor(cmd)
builder := clipkg.NewCommandSpecBuilder(spec).
	WithProcessor(proc).  // Enables context-constrained loading
	WithSpecsDir(".zqk/cli/specs").  // Optional: specify base directory
	RegisterRunE("runGet", runGet).
	RegisterRunE("runCreate", runCreate).
	// ... register other RunE functions
	Build()

cmd, err := builder.Build()
```

**External Spec References (Coordinator Pattern):**
```yaml
subcommands:
  - name: "subcommand"
    spec_ref: "subcommand.yaml"  # Resolved relative to project root
```

The coordinator pattern ensures:
- Spec paths are resolved relative to project root (from processor)
- Path traversal attempts are blocked (security constraint)
- Context constraints are respected (project root validation)

## Benefits

1. **Semantic Organization**: Commands defined in human-readable YAML
2. **Version Control**: Spec files can be tracked and reviewed
3. **Consistency**: All commands follow the same spec structure
4. **Flexibility**: Can still use programmatic builders when needed
5. **GraphRAG**: Specs provide semantic structure for AI understanding
6. **Maintainability**: Changes to command structure visible in spec files

## Spec Structure

### CommandSpec

- `name`: Command use string (e.g., "get <id>")
- `short`: Short description
- `description`: Long description (multi-line)
- `args`: Argument validation spec
- `flags`: Custom flags
- `subcommands`: Subcommand specs
- `help`: Help text configuration
- `run_e`: RunE function name (registered at build time)
- `common_flags`: Whether to include common flags

### CRUDCommandSpec

Extends `CommandSpec` with:
- `operation_type`: "create", "read", "update", "delete", "list"
- `data_input`: Include file/data flags
- `update_flags`: Include update-specific flags
- `dry_run`: Include dry-run flag
- `cascade`: Include cascade flag
- `query_flags`: Include query flags

## Future Enhancements

1. **YAML Loader**: Load specs from YAML files
2. **Spec Validation**: Validate specs against schema
3. **Code Generation**: Generate command code from specs
4. **Spec Registry**: Central registry of all command specs
5. **Spec Documentation**: Auto-generate docs from specs
