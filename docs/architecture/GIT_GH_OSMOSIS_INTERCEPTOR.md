# Git and GitHub Osmosis Interceptor Architecture

**Epic**: `PRI-EXAMPLE`  
**Status**: Draft  
**Focus**: Telemetry, Traceability (POL-CODE-009), and Dynamic Command Learning

## 1. Context & Motivation

To maintain compliance with **POL-CODE-009** (Graph-Level Commit Constraints), every agent/human commit or PR must possess bidirectional traceability in the Knowledge Kernel. A commit lacking an associated workstream or goal must be unconditionally rejected. Furthermore, we must capture telemetry and execution sequences to enable "Dynamic Command Learning" across all `git` and `gh` operations. 

While ZQK already standardizes internal CLI execution via the `cli_wrapper` pattern (`pkg/specbuilder/cli_builders/`), we need a mechanism to intercept direct terminal executions of `git` and `gh` so they also route through this pattern.

## 2. Architectural Design: The Proxy Shim Pattern

To hook the `cli_wrapper` pattern into all `git` and `gh` calls effortlessly, we will implement the **Proxy Shim Pattern**.

### 2.1 Shim Binaries in `$PATH`
We will introduce lightweight shim binaries (or symlinks pointing to a `zqk-osmosis` router) placed in a dedicated ZQK shim directory (e.g., `~/.zqk/shims`). When agents or humans initialize their terminal via `zqk init` or equivalent, this directory is prepended to the `$PATH`.
- When an agent executes `git commit ...`, the OS resolves the `git` shim.

### 2.2 Routing to the CLI Wrapper
Once the shim is invoked, it delegates the operation to the established ZQK `cli_wrapper` pattern:
1. **Parse**: Extract the target executable name from `os.Args[0]` (e.g., `git` or `gh`) and the arguments `os.Args[1:]`.
2. **Resolve Original**: The `cli_wrapper` must be configured to find the actual system binary (e.g., `/usr/bin/git` or `/opt/homebrew/bin/git`) rather than relying on `$PATH`, preventing infinite recursion.
3. **Execute via Builder**:
   ```go
   builder := cli_builders.NewCLIBuilder(actualSystemPath).
       WithArgs(os.Args[1:]...).
       WithContext(ctx)
   ```
4. **Osmosis Telemetry**: The wrapper automatically emits pre- and post-execution telemetry via `logging.FluentEvent` to the Knowledge Kernel, recording execution times, command sequences, and exit codes for Dynamic Command Learning.

### 2.3 POL-CODE-009 Enforcement Gate
Before the shim executes the command via the builder, it inspects the arguments for mutating operations (specifically `git commit` and `gh pr create`).
- **Validation**: If a commit is detected, the shim evaluates the context (current branch, active workstream object in ZQK).
- **Enforcement**: If the context lacks bidirectional traceability (e.g., branch not linked to a prioritized workstream), the shim rejects the operation immediately with a fatal error, never calling the underlying executable.

## 3. Technical Prerequisites & Work Breakdown

1. **`cli_wrapper` Path Resolution Bypass**: 
   - Update `pkg/specbuilder/cli_builders/` to support an explicit binary path override, ensuring the shim can call the real `git`/`gh` instead of invoking itself.
2. **Shim Executable (`cmd/zqk-shim`)**: 
   - Create a generic shim entrypoint.
   - Update the `Makefile` to compile `zqk-shim` and generate `git` and `gh` symlinks in `bin/shims/`.
3. **Constraint Evaluator Hooks**: 
   - Implement the `commit` and `pr` validation logic that queries the ZQK graph backend for POL-CODE-009 compliance before execution.
4. **Agent Profile/PATH Updates**: 
   - Update agent bootstrapping (and human onboarding instructions) to ensure `bin/shims` is pre-pended to the active `$PATH`.
