# Programmable Response Handling

**Last Verified:** 2026-08-31


The test harness supports configurable response processing, allowing you to customize how tool call responses and errors are handled before validation.

## Concept: "Recycled Response"

Responses can be **processed, transformed, or filtered** before validation through configurable processors. This allows:

- **Error translation**: Convert errors to different formats or success states
- **Response filtering**: Remove or transform response data
- **Conditional processing**: Apply different processors based on response content
- **Custom interactions**: Define your own response handling logic

## Built-in Processors

### `elicitation_to_success` (Default)

Converts `ElicitationError` to a success response with elicitation data. This treats elicitation as expected behavior for interactive tools.

```yaml
response_processor: "elicitation_to_success"
```

**Transformation**:
- `ElicitationError` → Success response with `elicitation: true` and parameter data
- Other errors pass through unchanged

### `ignore_errors`

Ignores all errors, treating them as success responses with error information included.

```yaml
response_processor: "ignore_errors"
```

**Transformation**:
- Any error → Success response with `error_ignored: true` and error message
- Useful for testing error handling paths without failing tests

### `noop`

Passes responses through unchanged (no processing).

```yaml
response_processor: "noop"
```

**Transformation**:
- No transformation - responses pass through as-is

## Usage

### Scenario-Level Configuration

```yaml
name: "Interactive creation test"
response_processor: "elicitation_to_success"

tests:
  - name: "Create object"
    tool: "zqk_create_object_interactive"
    args:
      kind: "backlog_item"
    expected:
      elicitation: true  # Now checks for elicitation flag in result
```

### Programmatic Configuration

```go
// Use default (elicitation_to_success)
executor := NewScenarioExecutor(server)

// Use no-op (pass through unchanged)
config := NewNoOpResponseProcessorConfig()
executor := NewScenarioExecutorWithProcessor(server, config)

// Custom processor
customProcessor := &ErrorTranslatorProcessor{
    TranslateFunc: func(err error) (interface{}, error) {
        // Custom error translation logic
        return nil, fmt.Errorf("translated: %v", err)
    },
}
config := &ResponseProcessorConfig{
    DefaultProcessor: customProcessor,
}
executor.SetResponseProcessor(config)
```

### Step-Specific Processors

```go
config := &ResponseProcessorConfig{
    GlobalProcessor: &ElicitationToSuccessProcessor{},
    StepProcessors: map[string]ResponseProcessor{
        "create_object": &IgnoreErrorsProcessor{},  // Ignore errors for this step
        "validate": &NoOpProcessor{},                // No processing for this step
    },
}
executor.SetResponseProcessor(config)
```

## Processor Types

### ResponseProcessor Interface

```go
type ResponseProcessor interface {
    ProcessResponse(result interface{}, err error) (interface{}, error, bool)
    // Returns: (transformed result, transformed error, should continue validation)
}
```

### Chaining Processors

```go
chain := &ChainedResponseProcessor{
    Processors: []ResponseProcessor{
        &ElicitationToSuccessProcessor{},
        &CustomFilterProcessor{},
    },
}
```

### Conditional Processing

```go
conditional := &ConditionalProcessor{
    Condition: func(result interface{}, err error) bool {
        // Return true to apply processor
        return err != nil
    },
    Processor: &IgnoreErrorsProcessor{},
    Otherwise: &ElicitationToSuccessProcessor{},
}
```

## Registering Custom Processors

```go
RegisterProcessor("my_custom_processor", func() ResponseProcessor {
    return &MyCustomProcessor{}
})
```

Then use in YAML:

```yaml
response_processor: "my_custom_processor"
```

## Examples

### Example 1: Treat Elicitation as Success

```yaml
name: "Interactive creation"
response_processor: "elicitation_to_success"

tests:
  - name: "First call"
    tool: "zqk_create_object_interactive"
    args:
      kind: "backlog_item"
    expected:
      has_field:
        elicitation: true
        parameters: []
```

### Example 2: Ignore All Errors

```yaml
name: "Error handling test"
response_processor: "ignore_errors"

tests:
  - name: "Tool that may fail"
    tool: "some_tool"
    args: {}
    expected:
      has_field:
        error_ignored: true
```

### Example 3: Custom Error Translation

```go
translator := &ErrorTranslatorProcessor{
    TranslateFunc: func(err error) (interface{}, error) {
        if strings.Contains(err.Error(), "permission denied") {
            return map[string]interface{}{
                "auth_required": true,
            }, nil
        }
        return nil, err
    },
}
```

## Benefits

1. **Flexibility**: Customize response handling for different test scenarios
2. **Reusability**: Register processors once, use across multiple tests
3. **Composability**: Chain processors for complex transformations
4. **Testability**: Easy to test error handling paths without failing tests
5. **Clarity**: Explicit response transformation makes test intent clear
