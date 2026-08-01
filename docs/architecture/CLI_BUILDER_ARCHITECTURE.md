> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# CLI Builder Architecture

## Relationship to pkg/specbuilder

The command builder system (`pkg/cli/bldr_cli_cmd_v1/`) follows the **exact same architectural pattern** as the specbuilder system (`pkg/specbuilder/bldr_*_v*/`), but for a different domain.

## Parallel Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Spec-Driven Builder Pattern                   │
└─────────────────────────────────────────────────────────────────┘

┌──────────────────────────────┬──────────────────────────────────┐
│   pkg/specbuilder/           │   pkg/cli/                       │
│   (System Objects)           │   (CLI Commands)                 │
├──────────────────────────────┼──────────────────────────────────┤
│                              │                                  │
│ YAML Specs:                  │ YAML Specs:                      │
│ docs/architecture/_internal/      │ .zqk/cli/specs/                  │
│   object_specs/              │   *_command.yaml                 │
│     base_object.yaml          │     get_command.yaml             │
│     backlog_item.yaml         │     create_command.yaml          │
│     trait/listable.yaml       │     list_command.yaml             │
│                              │                                  │
│ Codegen:                     │ Codegen:                         │
│ pkg/specbuilder/             │ pkg/cli/command_builders/         │
│   builders/codegen.go         │   codegen.go                     │
│   trait_builders/codegen.go   │                                  │
│   lifecycle_builders/         │                                  │
│     codegen.go                │                                  │
│                              │                                  │
│ Generated Builders:           │ Generated Builders:              │
│ pkg/specbuilder/             │ pkg/cli/                          │
│   bldr_v2/                   │   bldr_cli_cmd_v1/               │
│     base_object_builder.go    │     get_command_builder.go        │
│     backlog_item_builder.go   │     create_command_builder.go     │
│   bldr_trait_v1/             │                                  │
│     listable_builder.go       │                                  │
│     readable_builder.go       │                                  │
│   bldr_lifecycle_v1/         │                                  │
│     backlog_item_builder.go   │                                  │
│                              │                                  │
│ System Commands:              │ System Commands:                 │
│ zqk system generate-builders (PRUNED)  │ zqk system generate-command-     │
│ zqk system generate-          │   builders                        │
│   instance-builders           │                                  │
│ zqk system generate-          │                                  │
│   trait-builders               │                                  │
│                              │                                  │
│ Output Type:                  │ Output Type:                      │
│ *objects.Spec                 │ *cobra.Command                   │
│                              │                                  │
│ Base Classes:                 │ Base Classes:                     │
│ BaseSpecBuilder               │ CommandBuilder                    │
│ BaseTraitBuilder              │ CRUDCommandBuilder                │
│ BaseLifecycleBuilder          │                                  │
└──────────────────────────────┴──────────────────────────────────┘
```

## Directory Structure Comparison

### pkg/specbuilder/
```
pkg/specbuilder/
├── builders/
│   ├── codegen.go              # Generates bldr_v2/*_builder.go
│   └── base_builder.go         # BaseSpecBuilder
├── trait_builders/
│   ├── codegen.go              # Generates bldr_trait_v1/*_builder.go
│   └── base_builder.go         # BaseTraitBuilder
├── lifecycle_builders/
│   ├── codegen.go              # Generates bldr_lifecycle_v1/*_builder.go
│   └── base_builder.go         # BaseLifecycleBuilder
├── bldr_v2/                    # Generated spec builders
│   ├── base_object_builder.go
│   └── backlog_item_builder.go
├── bldr_trait_v1/              # Generated trait builders
│   ├── listable_builder.go
│   └── readable_builder.go
└── bldr_lifecycle_v1/          # Generated lifecycle builders
    └── backlog_item_builder.go
```

### pkg/cli/
```
pkg/cli/
├── command_builders/
│   ├── codegen.go              # Generates bldr_cli_cmd_v1/*_builder.go
│   └── codegen_test.go
├── command_builder.go           # CommandBuilder (like BaseSpecBuilder)
├── command_spec.go              # CommandSpec (like objects.Spec)
├── bldr_cli_cmd_v1/            # Generated command builders
│   ├── get_command_builder.go
│   ├── create_command_builder.go
│   └── list_command_builder.go
└── testdata/
    └── *_command.yaml          # Example specs
```

## Codegen Pattern Comparison

### Specbuilder Codegen (pkg/specbuilder/builders/codegen.go)
```go
// GenerateBuilderFromYAML reads a YAML spec file and generates a builder Go file
func GenerateBuilderFromYAML(yamlPath, outputDir string, version string, constantsFactory *ConstantsFactory) error {
    // Read YAML
    // Parse into objects.Spec
    // Generate builder code
    // Write to pkg/specbuilder/bldr_v2/{ontology}_builder.go
}
```

### Command Builder Codegen (pkg/cli/command_builders/codegen.go)
```go
// GenerateCommandBuilderFromYAML reads a YAML command spec file and generates a command builder Go file
func GenerateCommandBuilderFromYAML(yamlPath, outputDir string) error {
    // Read YAML
    // Parse into CommandSpec or CRUDCommandSpec
    // Generate builder code
    // Write to pkg/cli/bldr_cli_cmd_v1/{command}_command_builder.go
}
```

## Generated Builder Comparison

### Specbuilder Generated Builder (bldr_v2/base_object_builder.go)
```go
package bldr_v2

type BaseObjectBuilder struct {
    *builders.BaseSpecBuilder
}

func NewBaseObjectBuilder() *BaseObjectBuilder {
    builder := &BaseObjectBuilder{
        BaseSpecBuilder: builders.NewBaseSpecBuilder("base_object", "v2_0_0"),
    }
    // Configure spec...
    return builder
}

func (b *BaseObjectBuilder) Build() *objects.Spec {
    return b.BaseSpecBuilder.Build()
}
```

### Command Builder Generated Builder (bldr_cli_cmd_v1/get_command_builder.go)
```go
package bldr_cli_cmd_v1

func NewGetCommandBuilder() *cobra.Command {
    return clipkg.NewCommandBuilder("get <id>").
        WithShort("Get an object by ID").
        WithHelpBuilder(/* ... */).
        WithArgs(cobra.ExactArgs(1)).
        WithCommonFlagsDefault(cli.AddCommonFlags).
        Build()
}
```

## Key Architectural Principles (Shared)

1. **Specs as Source of Truth**: YAML specs define structure, codegen generates builders
2. **Versioned Directories**: `bldr_*_v1/`, `bldr_*_v2/` for immutable version history
3. **Generated Code**: Builders are generated, not manually written
4. **Fluent API**: Both use builder pattern with chainable methods
5. **Separation of Concerns**: Specs define "what", builders define "how"
6. **Graph Integration**: Both can be stored in graph for GraphRAG

## Integration Points

### Trait System Connection
Command builders connect to the trait system (from `pkg/specbuilder/bldr_trait_v1/`):

```yaml
# .zqk/cli/specs/list_command.yaml
required_traits:
  - listable  # References trait from pkg/specbuilder/bldr_trait_v1/listable_builder.go
conditional_traits:
  - flag: "group-by"
    traits: ["groupable"]  # References pkg/specbuilder/bldr_trait_v1/groupable_builder.go
```

This creates semantic relationships:
- Commands -> Traits (via `REQUIRES_TRAIT` edges)
- Commands -> Object Kinds (via `OPERATES_ON` edges)
- Enables GraphRAG queries across both systems

## System Commands

Both systems have corresponding `zqk system` commands:

| Specbuilder | CLI |
|------------|-----|
| `zqk system generate-builders` (PRUNED) | `zqk system generate-command-builders` |
| `zqk system generate-trait-builders` (PRUNED) | (N/A - traits are part of specbuilder) |
| `zqk system generate-instance-builders` | (N/A - commands don't have instances) |

In addition, the **spec index** generator bridges object specs and CLI:

- `zqk system generate-spec-index` (PRUNED) builds `docs/architecture/_internal/spec_index.json` (resolved via path-cache as `prefix:process_internal/spec_index.json`).
- The spec index is a **read-optimized view of fields by kind** (including traits such as groupable/filterable/sortable, enum values, and key validation hints).
- CLI helpers and completions use the index as a fast, immutable source of truth for:
  - Listing kinds (`object fields --list-kinds`),
  - Offering field-aware completions (e.g. `object <kind> list --group-by/--filter/--sort [TAB]`),
  - Surfacing constraints (enum values, numeric ranges, time/date formats) inline in completion suggestions.

## Summary

The command builder system (`pkg/cli/bldr_cli_cmd_v1/`) is **architecturally parallel** to the specbuilder system (`pkg/specbuilder/bldr_*_v*/`):

- **Same pattern**: YAML specs -> codegen -> generated builders
- **Same structure**: Versioned directories, generated code, fluent APIs
- **Different domain**: Commands (CLI) vs. Objects (system configuration)
- **Different output**: `*cobra.Command` vs. `*objects.Spec`
- **Connected**: Commands reference traits from specbuilder system

This maintains architectural consistency while serving different purposes.
