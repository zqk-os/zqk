# CLI Wrapper Pattern

## Context
ZQK requires an architectural standard for invoking third-party CLI tools (e.g., `ffmpeg`, `genmedia`, `curl`). Directly executing these commands via `os/exec` introduces fragmentation, loss of observability, and prevents standard ZQK behaviors like automatic metric collection, failure tracking, and logging to the Knowledge Kernel.

## Decision
All third-party CLI command execution must be modeled via a Spec-Builder. The execution must go through a fluent builder pattern located at `pkg/specbuilder/cli_builders/`.

### Key Requirements
1. **Type-Safe Fluent Builder:** Commands should be assembled fluently (e.g., `NewCLIBuilder("ffmpeg").WithArg("-i").WithArg("input.mp4").Execute()`).
2. **Execution Interception:** The builder must internally handle `os/exec.Command` execution.
3. **Observability & Knowledge Kernel Injection:** Before and after execution, metrics such as execution time, utilization, and failures must be captured. These events must be logged to the ZQK Knowledge Kernel using `logging.FluentEvent` for "osmosis" knowledge mapping.
4. **Context Propagation:** The builder must support `context.Context` for timeouts and cancellation.

## Example Usage
```go
cmdOutput, err := cli_builders.NewCLIBuilder("ffmpeg").
    WithContext(ctx).
    WithArg("-i").
    WithArg("input.mp4").
    WithArg("output.mp4").
    Execute()
```
