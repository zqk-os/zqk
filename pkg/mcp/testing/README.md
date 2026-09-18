# MCP Testing Package

This package provides a test harness for MCP (Model Context Protocol) tools, allowing you to define test scenarios in YAML or JSON format.

## Features

- **YAML/JSON Test Scenarios**: Define tests declaratively
- **Import Support**: Link external data files to avoid cluttering test files
- **Setup/Cleanup**: Run setup and cleanup steps
- **Assertions**: Validate tool responses
- **Result Storage**: Store tool results for use in later steps
- **Dependencies**: Define dependencies between test steps

## Test Scenario Format

### Basic Structure

```yaml
name: "Interactive object creation test"
description: "Test creating a backlog item interactively"

setup:
  - name: "List milestones"
    tool: "zqk_object_list"
    args:
      kind: "milestone"
    store_result: "milestones"

tests:
  - name: "Create object interactively"
    tool: "zqk_create_object_interactive"
    args:
      kind: "backlog_item"
      title: "Test Item"
    import: "references.yaml"
    import_path: "args"
    expected:
      success: true
      has_field:
        session_id: true
```

### Import Support

Import external data files to keep test scenarios clean:

**references.yaml**:
```yaml
milestone_refs:
  - "MIL-001"
  - "MIL-002"
goal_refs:
  - "GOAL-001"
```

**test-scenario.yaml**:
```yaml
tests:
  - name: "Create object with references"
    tool: "zqk_create_object_interactive"
    args:
      kind: "backlog_item"
      title: "Test Item"
    import: "references.yaml"
    import_path: "args"  # Merge into args
```

### Top-Level Imports

Import complete scenario files:

```yaml
name: "Composite test"
imports:
  - "common-setup.yaml"
  - "common-cleanup.yaml"

tests:
  - name: "My test"
    tool: "zqk_object_list"
    args:
      kind: "backlog_item"
```

### Assertions

Validate tool responses:

```yaml
tests:
  - name: "Create object"
    tool: "zqk_create_object_interactive"
    args:
      kind: "backlog_item"
    expected:
      success: true
      has_fields:
        - "session_id"
        - "filled_template"
      equals:
        kind: "backlog_item"
      matches:
        session_id: "^interactive-.*"
```

### Result Storage and Dependencies

Store results and use them in later steps:

```yaml
setup:
  - name: "Get milestone"
    tool: "zqk_object_get"
    args:
      id: "MIL-001"
    store_result: "milestone"

tests:
  - name: "Create item with milestone"
    tool: "zqk_create_object_interactive"
    depends_on:
      - "milestone"
    args:
      kind: "backlog_item"
      milestone_refs:
        - "${milestone.id}"  # Variable interpolation (future)
```

## Usage

```go
package main

import (
    "github.com/lanceman/zqk/pkg/mcp/testing"
)

func main() {
    loader := testing.NewScenarioLoader("./test-scenarios")
    scenario, err := loader.LoadScenario("my-test.yaml")
    if err != nil {
        log.Fatal(err)
    }
    
    executor := testing.NewScenarioExecutor(server)
    results, err := executor.RunScenario(scenario)
    if err != nil {
        log.Fatal(err)
    }
    
    // Process results
}
```

## File Organization

```
test-scenarios/
├── scenarios/
│   ├── interactive-creation.yaml
│   └── object-query.yaml
├── data/
│   ├── references.yaml
│   ├── large-reference-list.yaml
│   └── test-data.yaml
└── common/
    ├── setup.yaml
    └── cleanup.yaml
```
