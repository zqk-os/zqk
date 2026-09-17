# CLI Architecture

**Last Verified:** 2026-09-09
**Consolidates:** CLI_BUILDER_ARCHITECTURE.md, CLI_ASYNC_AND_PROGRESS.md, CLI_HANDLER_PATTERNS.md, CLI_ARCHITECTURE.md

The ZQK CLI is designed as a unified, spec-driven interface built on strict patterns to ensure consistency, reliable error handling, and robust progression feedback for both human users and AI agents.

## 1. Command DNA (Spec-Driven Architecture)

Commands in ZQK are not manually crafted one-offs. They are defined via YAML specifications (Command DNA) and generated to ensure complete uniformity.

- **Source of Truth:** `pkg/cli/command_builders/` handles codegen. The YAML specs define commands (their flags, required traits, and operations).
- **Generated Code:** Builders are written to versioned directories (e.g., `pkg/cli/bldr_cli_cmd_v1/`).
- **Trait Connections:** Commands declare `required_traits` and `conditional_traits` which map cleanly to the central specbuilder system (see [POL-CODE-015: Specbuilder Requirements](POL-CODE-015)).
- **Spec Index:** Tools use `.zqk/specs/spec_index.json` as a read-optimized view of the system.

## 2. Builder Pattern & Handler Patterns

The CLI relies on Builder patterns for commands, help text, and errors.

### Command Builders
Generated command builders construct `*cobra.Command` instances fluidly.
```go
return clipkg.NewCommandBuilder("get <id>").
    WithShort("Get an object by ID").
    WithHelpBuilder(helpBuilder).
    WithArgs(cobra.ExactArgs(1)).
    WithCommonFlagsDefault(cli.AddCommonFlags).
    Build()
```

### Handler Patterns (Processor & Guard)
All handlers use the `Processor` pattern for unified context, storage, and logging.
```go
proc, err := cli.NewProcessor(cmd)
```
**Error Handling:** Use `cli.EnhanceError(cmd, err)` or the fluent `cli.Guard(cmd)` to wrap errors. These respect context profiles (`ai-agent/debug` = terse, `human` = standard) and ensure users receive actionable next steps.

## 3. Flag Discipline

Commands follow a strict, unified flag discipline. Custom parsing should be avoided in favor of `clipkg` utilities.
- **Common Flags (`cli.AddCommonFlags`):** `--format`, `--output`, `--verbose`, `--quiet`, `--timeout`.
- **Domain Flags:** 
  - CRUD operations use standardized flags (e.g., `--file`, `--data`, `--dry-run`, `--filter`, `--sort-by`).
  - Validation: Use `clipkg` to enforce mutually exclusive flags and required constraints.

## 4. FormatOutput Contract

Output must respond dynamically to the context (interactive vs. script) and flags (`--format`).

1. **Structured Data:** Use `cli.FormatOutput(cmd, data)`. Automatically respects JSON, YAML, or Table formats. Use this for queries and gets.
2. **Simple Messages:** Use `cli.WriteOutput(cmd, []byte(msg))`. For success messages (e.g., "Object created").
3. **Never `fmt.Print`:** Adhere strictly to [POL-CODE-007: Structured Logging Compliance](POL-CODE-007). All output flows through the CLI's configured writers, and logging goes through the `proc.Logger()`.

## 5. Async & Progress (Never Hang)

CLI commands must never block silently.

- **Progress Context:** Every command is wrapped with `BindAsyncProgress` to inject a timeout context and a progress callback.
- **Heartbeat:** `RunWithAsyncProgress` runs a background heartbeat (every 5s) to emit status to the user.
- **Blocking Operations:** Before any blocking call (e.g., locking, semaphore, long I/O), the code must emit a progress status via `pkgctx.GetValidationProgress(ctx)`.
  *Example:* `EmitListCountWaitProgress(ctx)` before acquiring a slot.

## 6. Context Profiles

The CLI adapts its behavior based on the caller profile:
- **Human:** Rich errors, progress bars, helpful suggestions.
- **AI Agent / Scripting (`--profile ai-agent`):** Terse output, strict JSON formatting, suppressed progress bars (or emitted as structured logs).

## Summary
By centralizing command definition (Spec-Driven), enforcing uniform execution (Processor/Async), and standardizing I/O (FormatOutput/Structured Logging), the CLI provides a robust, predictable membrane for all interactions.
