# Osmosis Interceptor Architecture

**Last Verified:** 2026-08-31


## 1. Overview
The **Osmosis Interceptor** provides a transparent wrapping layer around critical developer tools (specifically `git` and `gh`) to enforce graph-traceability, collect telemetry, and dynamically learn command patterns. This design fulfills the requirements of [[REDACTED-ID]](Git and GitHub Osmosis Interceptor) and enforces the strict constraints of [POL-CODE-009](Graph-Level Commit Constraints).

## 2. Goals
- **Enforce Traceability**: Ensure every commit and pull request is bidirectionally linked to a valid node in the ZQK Knowledge Kernel (e.g., a Workstream, Goal, or Requirement).
- **Inject Telemetry**: Capture command execution metrics (latency, success/failure rates, arguments) and log them dynamically to the Knowledge Kernel.
- **Dynamic Command Learning**: Monitor tool usage to detect "capability drift" or undocumented agent behaviors by inferring context from unmapped command sequences.
- **Seamless Developer Experience**: Operate invisibly for compliant operations while providing clear, actionable feedback when policy violations occur.

## 3. Architecture Design

### 3.1 Shim-Based Interception
Instead of requiring developers to use `zqk git`, the system generates transparent shim executables (or symlinks) placed in a dedicated directory (e.g., `~/.zqk/shims`). This directory is prepended to the user's `$PATH`.
- When a user or agent invokes `git` or `gh`, the shim intercepts the call.
- The shim forwards the execution to the ZQK CLI proxy commands: `zqk intercept git -- <args>` and `zqk intercept gh -- <args>`.

### 3.2 Interception Workflow
The interceptor operates in a three-phase pipeline:

1. **Pre-Flight (Validation & Context Extraction)**
   - Parse the command arguments to determine the intent (e.g., `commit`, `pr create`).
   - If the intent modifies state, extract references (e.g., ZQK IDs like `WRK-123` from `-m` flags).
   - **Graph Check (POL-CODE-009)**: Query the Knowledge Kernel. If the reference is missing or invalid, block the execution immediately and return a non-zero exit code with a policy violation message.

2. **Execution (Proxying)**
   - Locate the system's true binary using the original path.
   - Execute the binary with the provided arguments, piping `stdin`, `stdout`, and `stderr` directly to maintain interactivity (using `os/exec` with standard stream attachment).
   - Start a high-resolution timer.

3. **Post-Flight (Telemetry & Learning)**
   - Stop the timer and capture the exit code.
   - Asynchronously log the execution footprint (command signature, duration, success state) to the Knowledge Kernel via `zqk` logging/telemetry pipelines.
   - **Dynamic Learning**: If the command sequence is recognized as a new capability or drifts from expected workstream norms, flag it in the graph for architectural review.

## 4. Graph-Level Commit Constraints (POL-CODE-009)
The interceptor specifically targets state-changing operations:
- **`git commit`**: Requires a ZQK ID in the commit message. The interceptor may inject a Git hook (`commit-msg`) or intercept the `-m`/`-F` arguments.
- **`gh pr create`**: Scans the PR title and body for ZQK IDs. Ensures the PR maps to an active Workstream.

If an operation lacks traceability, the interceptor unconditionally rejects it, preventing "dollar store quality garbage" from polluting the Knowledge Kernel graph.

## 5. Telemetry & Dynamic Command Learning
The ZQK system uses the interceptor's telemetry data to build a real-time behavioral graph:
- **Execution Metrics**: Stores structured data (timestamp, tool, sub-command, arguments, duration, exit code).
- **Capability Drift Detection**: By analyzing the frequency and sequence of intercepted commands against the declared agent capabilities, the Knowledge Kernel identifies drifts. For example, if an agent repeatedly uses `gh api` when it only declared `gh pr` capabilities, an anomaly is logged.

## 6. Implementation Plan
1. **CLI Commands**: Implement `zqk intercept <tool> [args...]` in `cmd/zqk`.
2. **Shim Installer**: Add `zqk setup shims` to automatically configure the `~/.zqk/shims` directory and `.bashrc`/`.zshrc` `PATH` prepends.
3. **Validation Logic**: Implement regex parsers and graph queries for POL-CODE-009 enforcement within `pkg/interceptor`.
4. **Telemetry Agent**: Extend `pkg/storage` or `pkg/metrics` to asynchronously ingest interceptor payloads without blocking the developer's terminal.
