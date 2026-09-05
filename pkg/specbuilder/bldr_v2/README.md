# Spec Builders (bldr_v2)

**Status:** Active  
**Last Updated:** 2026-01-27  
**Package:** `github.com/lanceman/zqk/pkg/specbuilder/bldr_v2`

## Overview

This package contains **generated** spec builders for system object types. All files in this directory are automatically generated from YAML object specifications located in `docs/process/_internal/object_specs/`.

**⚠️ IMPORTANT: DO NOT EDIT FILES IN THIS DIRECTORY MANUALLY**

All changes must be made to the YAML spec files, then regenerated using:
```bash
zqk system generate-builders --overwrite
```

## Architecture

This package follows the **spec-driven builder pattern**:

```
YAML Specs (docs/process/_internal/object_specs/*.yaml)
    ↓
Codegen (pkg/specbuilder/builders/codegen.go)
    ↓
Generated Builders (this package)
    ↓
Runtime Usage (pkg/objects, pkg/storage, etc.)
```

## Generated Files

Each object type has a corresponding builder file:
- `{ontology}_builder.go` - Generated builder structs that extend `BaseSpecBuilder`
- `{ontology}_constants.go` - Generated field name constants for type-safe field access
- Functions follow the pattern: `New{Ontology}Builder() *{Ontology}Builder`

## Builder Pattern

All builders extend `BaseSpecBuilder` from `pkg/specbuilder/builders/`:

```go
type BacklogItemBuilder struct {
    *builders.BaseSpecBuilder
}

func NewBacklogItemBuilder() *BacklogItemBuilder {
    builder := &BacklogItemBuilder{
        BaseSpecBuilder: builders.NewBaseSpecBuilder("backlog_item", "v2_0_0"),
    }
    // Configure spec...
    return builder
}
```

## Field Constants

Each builder has a corresponding constants file for type-safe field access:

```go
// From backlog_item_constants.go
const (
    FieldID = "id"
    FieldTitle = "title"
    FieldStatus = "status"
    // ...
)
```

**Persisted object maps (CAS YAML, `map[string]any`):** prefer **`pkg/objects` `FieldKey*`** from `field_keys.go` (`zqk system generate-field-keys`). That is the single, spec-union namespace for field names. Per-kind `*_constants.go` names here differ only because this package is flat (Go symbol collisions); they are for **builders and spec tooling**, not the canonical key for cross-kind storage code.

Use `bldr_v2` per-kind constants when working **inside** the generated builder for that ontology; use `objects.FieldKey*` when reading/writing **stored instances** by field name.

## Inheritance and Traits

Builders support:
- **Inheritance**: `SetExtends("base_object")` - Inherit fields from parent specs
- **Traits**: `AddTrait("listable")` - Add trait-based functionality
- **Schema Versioning**: Each builder targets a specific schema version (e.g., `v2_0_0`)

## Integration Points

### With CLI Command Builders

The CLI command builder system (`pkg/cli/bldr_cli_cmd_v1/`) references traits from this system:

```yaml
# .zqk/cli/specs/list_command.yaml
required_traits:
  - listable  # References trait from pkg/specbuilder/bldr_trait_v1/
```

### With Object System

Builders generate `*objects.Spec` instances that are used by:
- `pkg/objects` - Object type system
- `pkg/storage` - Storage backends
- `pkg/validation` - Validation system

## Build Process

The Makefile automatically regenerates these builders before each build:

```makefile
generate-spec-builders:
    go run ./cmd/zqk system generate-builders --overwrite
```

This ensures generated code is always up-to-date.

## Related Documentation

- [Specbuilder Package README](../README.md) - Overview of the specbuilder package
- [Builders README](../builders/README.md) - Base builder implementation
- [CLI Architecture](../../cli/ARCHITECTURE.md) - Parallel CLI command builder system
- [Object Specs Documentation](../../../docs/process/_internal/object_specs/README.md) - Spec file format reference

## Versioning

The `v2` suffix indicates this is version 2 of the spec builder system. Version 1 (`bldr_v1/`) may still exist for backward compatibility. Future breaking changes would result in `bldr_v3/`, maintaining immutable version history.

## Constants Files

Some builders may have empty constants files (e.g., `list_metric_sampler_constants.go`) when all fields are inherited from parent specs. This is expected and normal - inherited fields are accessible via the parent spec's constants.
