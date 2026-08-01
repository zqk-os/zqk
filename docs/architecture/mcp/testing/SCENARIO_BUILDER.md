# Scenario Builder API

The Scenario Builder provides a fluent, programmatic API for creating test scenarios without manually writing YAML files. It uses YAML-friendly shapes (simple Go types) that serialize cleanly to YAML.

## Overview

The builder API consists of several builder types:

- **`ScenarioBuilder`**: Builds complete test scenarios
- **`StepBuilder`**: Builds individual test steps
- **`ExpectationBuilder`**: Builds test expectations
- **`ErrorExpectationBuilder`**: Builds error expectations
- **`ScenarioWriter`**: Writes scenarios to YAML files or strings

## Basic Usage

### Creating a Simple Scenario

```go
scenario := NewScenarioBuilder().
    Name("My Test Scenario").
    Description("Tests the object creation tool").
    AddTestStep(SimpleStep("Create object", "zqk_create_object_interactive")).
    Build()
```

### Building Steps

```go
step := NewStepBuilder().
    Name("Create backlog item").
    Tool("zqk_create_object_interactive").
    Arg("kind", "backlog_item").
    Arg("title", "Test Item").
    Expected(ExpectSuccess()).
    Build()
```

### Building Expectations

```go
exp := NewExpectationBuilder().
    Success(true).
    HasField("id", nil).
    HasFields("title", "status").
    Matches("id", "^BLI-\\d+$").
    Equals("status", "exploring").
    Build()
```

### Complete Example

```go
scenario := NewScenarioBuilder().
    Name("Object Creation Test").
    Description("Tests interactive object creation").
    ResponseProcessor("noop").
    AddSetupStep(NewStepBuilder().
        Name("Setup").
        Tool("zqk_test_echo").
        Arg("message", "setup").
        Build()).
    AddTestStep(NewStepBuilder().
        Name("Create item").
        Tool("zqk_create_object_interactive").
        Arg("kind", "backlog_item").
        Arg("title", "Test Item").
        Expected(NewExpectationBuilder().
            Success(true).
            HasFields("id", "title", "status").
            Matches("id", "^BLI-\\d+$").
            Build()).
        StoreResult("created_item").
        Build()).
    AddTestStep(NewStepBuilder().
        Name("Get item").
        Tool("zqk_object_get").
        Arg("id", "${created_item.id}").
        DependsOn("created_item").
        Expected(ExpectSuccess()).
        Build()).
    AddCleanupStep(SimpleStep("Cleanup", "zqk_test_echo")).
    Build()
```

## Writing Scenarios

### Write to File

```go
writer := NewScenarioWriter()
err := writer.WriteToFile(scenario, "test-scenarios/my-test.yaml")
```

### Write to String

```go
writer := NewScenarioWriter()
yamlStr, err := writer.WriteString(scenario)
```

### Write to Bytes

```go
writer := NewScenarioWriter()
yamlBytes, err := writer.WriteToBytes(scenario)
```

### Write to Writer

```go
writer := NewScenarioWriter()
err := writer.Write(scenario, os.Stdout)
```

## Convenience Functions

### Step Helpers

- `SimpleStep(name, tool string)`: Creates a step with just name and tool
- `StepWithArgs(name, tool string, args map[string]interface{})`: Creates a step with name, tool, and args

### Expectation Helpers

- `ExpectSuccess()`: Creates a simple success expectation
- `ExpectFailure()`: Creates a simple failure expectation
- `ExpectElicitation()`: Creates an expectation for elicitation error

## YAML-Friendly Shapes

All builder types work with YAML-friendly Go types:

- **Primitives**: `string`, `int`, `bool`, `float64`
- **Slices**: `[]string`, `[]interface{}`
- **Maps**: `map[string]interface{}`
- **Pointers**: `*bool`, `*int` (for optional values)

These types serialize cleanly to YAML without custom marshaling logic.

## Benefits

1. **Type Safety**: Compile-time checking of scenario structure
2. **Code Reuse**: Build scenarios programmatically, share common patterns
3. **IDE Support**: Autocomplete and validation in your IDE
4. **Test Generation**: Generate test scenarios from test data or templates
5. **Refactoring**: Easier to refactor scenarios as code changes

## Example: Generating Multiple Scenarios

```go
kinds := []string{"backlog_item", "milestone", "goal"}

for _, kind := range kinds {
    scenario := NewScenarioBuilder().
        Name(fmt.Sprintf("Test %s creation", kind)).
        AddTestStep(NewStepBuilder().
            Name(fmt.Sprintf("Create %s", kind)).
            Tool("zqk_create_object_interactive").
            Arg("kind", kind).
            Expected(ExpectSuccess()).
            Build()).
        Build()
    
    writer := NewScenarioWriter()
    filePath := fmt.Sprintf("test-scenarios/%s-creation.yaml", kind)
    writer.WriteToFile(scenario, filePath)
}
```
