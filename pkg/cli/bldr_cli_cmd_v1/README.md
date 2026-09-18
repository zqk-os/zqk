# CLI Command Builders (bldr_cli_cmd_v1)

**Status:** Active  
**Last Updated:** 2026-04-03  
**Package:** `github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1`

## Overview

This package contains **generated** command builders for the zqk CLI. All files in this directory are automatically generated from YAML command specifications located in `.zqk/cli/specs/`.

**⚠️ IMPORTANT: DO NOT EDIT FILES IN THIS DIRECTORY MANUALLY**

All changes must be made to the YAML spec files, then regenerated using:
```bash
zqk system generate-command-builders --overwrite
```

## Architecture

This package follows the **spec-driven builder pattern**, which is architecturally parallel to the specbuilder system (`pkg/specbuilder/bldr_v2/`):

```
YAML Specs (.zqk/cli/specs/*.yaml)
    ↓
Codegen (pkg/cli/command_builders/codegen.go)
    ↓
Generated Builders (this package)
    ↓
Runtime Usage (cmd/zqk/*)
```

## Relationship to pkg/specbuilder

The command builder system mirrors the specbuilder pattern:

| Aspect | pkg/specbuilder | pkg/cli/bldr_cli_cmd_v1 |
|--------|----------------|------------------------|
| **Specs** | `objects.Spec` (YAML) | `CommandSpec` (YAML) |
| **Codegen** | `pkg/specbuilder/builders/codegen.go` | `pkg/cli/command_builders/codegen.go` |
| **Generated Builders** | `pkg/specbuilder/bldr_v2/` | `pkg/cli/bldr_cli_cmd_v1/` |
| **Versioning** | `bldr_v2`, `bldr_trait_v1`, etc. | `bldr_cli_cmd_v1` |
| **System Command** | `zqk system generate-builders` | `zqk system generate-command-builders` |
| **Output Type** | `*objects.Spec` | `*cobra.Command` |

## RunE handlers and pflag getters

Specs use either **`string_array`** or **`stringSlice`** for multi-value flags. The generated Cobra/pflag types differ:

| YAML `type` | pflag kind | Read with |
|-------------|------------|-----------|
| `string_array` | `StringArray` (repeat `--flag v` per value) | **`GetStringArray("flag-name")`** |
| `stringSlice` | `StringSlice` (comma-separated values) | **`GetStringSlice("flag-name")`** |

Using the wrong getter returns an empty slice (pflag returns `[]string{}` on type mismatch); errcheck is often skipped, so the bug is silent. Example fix: `cmd/zqk/matrix` `get`/`update` handlers.

## Key Differences

1. **Domain**: Specbuilders create object specs (system configuration), command builders create CLI commands (user interface)
2. **Output**: Specbuilders output `*objects.Spec`, command builders output `*cobra.Command`
3. **Base Classes**: Specbuilders use `BaseSpecBuilder`, command builders use `CommandBuilder`/`CRUDCommandBuilder`
4. **Integration**: Command builders integrate with Cobra framework, specbuilders integrate with object system

## Generated Files

Each command has a corresponding builder file:
- `*_command_builder.go` - Generated builder functions that return `*cobra.Command`
- Functions follow the pattern: `New{Command}CommandBuilder() *cobra.Command`

## Integration with Traits

Command builders integrate with the trait system from `pkg/specbuilder/bldr_trait_v1/`:

```yaml
# .zqk/cli/specs/list_command.yaml
required_traits:
  - listable  # References trait from pkg/specbuilder/bldr_trait_v1/
conditional_traits:
  - flag: "group-by"
    traits: ["groupable"]
```

This creates semantic relationships in the graph:
- Commands → Traits (via `REQUIRES_TRAIT` edges)
- Commands → Object Kinds (via `OPERATES_ON` edges)
- Enables GraphRAG queries across both systems

## Build Process

The Makefile automatically regenerates these builders before each build:

```makefile
generate-spec-builders:
    go run ./cmd/zqk system generate-command-builders --overwrite
```

This ensures generated code is always up-to-date.

## Related Documentation

- [CLI Package README](../README.md) - Overview of the CLI package (links here for **RunE handlers and pflag getters**)
- [CLI Architecture](../ARCHITECTURE.md) - Detailed architecture documentation
- [Command Spec Documentation](../COMMAND_SPEC.md) - Command spec format reference
- [Specbuilder README](../../specbuilder/README.md) - Parallel specbuilder system

## Versioning

The `v1` suffix indicates this is version 1 of the command builder system. Future breaking changes would result in `bldr_cli_cmd_v2/`, maintaining immutable version history.
