# Elicitation Handling in MCP Test Harness

## Understanding Elicitation in MCP Protocol

**Key Insight**: `ElicitationError` implements the `error` interface, but it represents a **normal part of the interactive flow**, not a failure.

### How Elicitation Works

1. **Tool handler returns elicitation**: When a tool needs more parameters, it returns `(nil, *ElicitationError)`
2. **Error contains parameters to elicit**: The `ElicitationError` contains a list of `ElicitationParam` objects describing what's needed
3. **Protocol-level handling**: In the full MCP protocol, this is converted to a JSON-RPC error response with:
   - Error code: `-32602` (InvalidParams)
   - Error data: Contains the elicitation parameters
4. **Client responds**: The client provides the requested parameters and calls the tool again

### For Test Harness

When testing, we call `HandleToolCall` directly, which returns `(result, error)`. If the error is an `*ElicitationError`:

- **This is NOT a failure** - it's the expected behavior for interactive tools
- **Check error type**: Use type assertion to detect `*ElicitationError`
- **Extract parameters**: Access `ElicitationError.Parameters` to validate what was requested
- **Test expectations**: Should check for elicitation errors differently than regular errors

### Example Test Expectation

```yaml
tests:
  - name: "Create object - first call (elicitation expected)"
    tool: "zqk_create_object_interactive"
    args:
      kind: "backlog_item"
    expected:
      elicitation: true  # Expect elicitation, not error
      has_parameters:
        - "title"
        - "description"
```

### Implementation

The test executor should:
1. Check if `err != nil` and `err` is `*ElicitationError`
2. If so, treat as "elicitation requested" (success for interactive flow)
3. Validate the elicitation parameters match expectations
4. Only treat as failure if it's a different error type
