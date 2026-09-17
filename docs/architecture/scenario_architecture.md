# Validation Scenario Architecture

We have established a robust, deterministic execution framework in `pkg/validation/scenario` to replace error-prone, prompt-driven agent loops with programmatic Go validations. This directly aligns with the project's **Matrix-Gated Autonomy** and **Gap Discovery & Closure** protocols.

## Core Components
- **`pkg/validation/scenario/types.go`**: Defines the `Scenario` and `Step` configuration structures.
- **`pkg/validation/scenario/executor.go`**: Provides the `Executor` to sequentially run and evaluate steps, failing fast on errors.

## Supported Step Types
The architecture currently supports the following step types, which cover the vast majority of agent-like verification workflows:
1. `command`: Executes a CLI command.
2. `regex`: Asserts that the output of the previous step matches a specified regular expression.
3. `exists`: Validates the existence of a specific file or artifact.

## Example YAML Configuration
Because the structs are tagged with `yaml`, you can easily define validation stages externally:

```yaml
name: "CAP Stage Review - Priority Plan Alignment"
steps:
  - name: "Query System State"
    type: "command"
    command: "./bin/zqk"
    args: ["object", "list", "priority_plan", "--format", "json"]

  - name: "Verify Plan Output"
    type: "regex"
    pattern: "active"

  - name: "Check Matrix Existence"
    type: "exists"
    target: ".zqk/logs/cursor-hooks/agent_chat_channel.jsonl"
```

## Go Usage
```go
logger := logging.NewLogger(os.Stdout, logging.DebugLevel, logging.NewTextFormatter(ctx))
executor := scenario.NewExecutor(logger)

res := executor.Run(ctx, myScenario)
if !res.Success {
    // Fails the matrix-gated transition block
}
```
