# MCP Spec Builder Pattern

## Overview

The MCP Spec Builder Pattern externalizes MCP server configuration (prompts, resources, tools) into declarative YAML specifications, following the same pattern used by test-scenario builder and other spec-driven builders in the codebase.

## Architecture

```
┌─────────────────┐
│ YAML Spec File  │  (Declarative - "what")
│ mcp_spec.yaml   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Spec Loader    │  (Loads YAML specs)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Spec Generator  │  (Orchestration layer)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Spec Builder   │  (Programmatic - "how")
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ MCP Server      │  (Registered components)
└─────────────────┘
```

## Components

### 1. MCPSpec (YAML Structure)

Defines the complete MCP configuration:

```yaml
name: onboarding_prompts
description: Standard onboarding and help prompts
version: "1.0"

prompts:
  - name: welcome
    description: Welcome prompt for new sessions
    arguments:
      - name: context
        description: Additional context
        required: false

resources:
  - uri: file://docs/architecture/README.md
    name: health_monitoring
    description: System health monitoring guide
    mime_type: text/markdown
    category: documentation
    priority: high
    tags:
      - health
      - monitoring

tools:
  - name: custom_tool
    description: A custom tool
    properties:
      param1:
        type: string
        description: First parameter
        default: "default_value"
    required:
      - param1
```

### 2. Spec Builder API

Programmatic construction of specs:

```go
spec := NewMCPSpecBuilder("onboarding_prompts").
    Description("Standard onboarding prompts").
    AddPrompt(NewPromptSpecBuilder("welcome", "Welcome message").
        AddArgument("context", "Additional context", false).
        Build()).
    AddResource(NewResourceSpecBuilder("file://docs/guide.md", "guide", "User guide").
        SetCategory("documentation").
        AddTag("essential").
        Build()).
    Build()
```

### 3. Spec Generator

Applies specs to the server:

```go
generator := NewMCPSpecGenerator(server)
err := generator.GenerateFromSpec(spec)
```

### 4. Spec Loader

Loads specs from YAML files:

```go
loader := NewMCPSpecLoader()
spec, err := loader.LoadSpec("mcp_specs/onboarding.yaml")
```

## Usage

### Storage-Based Discovery (Recommended)

MCP specs are now **formal system objects** stored in the system, making them discoverable via CLI and storage infrastructure:

```go
// Set storage provider on server (from CLI/storage infrastructure)
server.SetStorageProvider(storageProvider)

// Specs are automatically discovered from storage when registering prompts
RegisterOnboardingPrompts(server)
```

The `RegisterOnboardingPrompts` function automatically:
1. **First**: Discovers `mcp_spec` objects from storage (formal system objects)
2. **Second**: Falls back to spec files in common locations:
   - `.zqk/mcp_specs/onboarding_prompts.yaml`
   - `docs/architecture/mcp_specs/onboarding_prompts.yaml`
   - `mcp_specs/onboarding_prompts.yaml`
3. **Finally**: Falls back to programmatic registration if no spec found

### Loading Specs from Files

```go
// Load and apply specs from a directory
err := LoadAndApplyMCPSpecs(server, "mcp_specs/")

// Load and apply a single spec file
err := LoadAndApplyMCPSpecs(server, "mcp_specs/onboarding.yaml")
```

### Creating MCP Spec Objects

MCP specs can be created as system objects using the CLI:

```bash
# Create an mcp_spec object
zqk object create mcp_spec \
  --id MCPSPEC-001 \
  --name onboarding_prompts \
  --spec @mcp_specs/onboarding_prompts.yaml
```

The spec object should have:
- `kind: mcp_spec`
- `name`: Spec name (e.g., "onboarding_prompts")
- `spec`: The YAML spec content (as string or inline object)

### Exporting Current Configuration

Convert programmatic prompts to a spec file:

```go
err := ExportPromptsToSpec(".zqk/mcp_specs/onboarding_prompts.yaml")
```

## Benefits

1. **Externalization**: Prompts, resources, and tools can be defined in YAML files
2. **Version Control**: Specs can be tracked in git, reviewed, and versioned
3. **Rapid Adaptation**: Different specs can quickly adapt MCP to different patterns (e.g., test-scenario builder patterns)
4. **Separation of Concerns**: Specs define "what", builders define "how"
5. **Consistency**: Follows the same pattern as test-scenario builder and other spec-driven builders

## Example: Test-Scenario Pattern

Create a spec for test scenarios:

```yaml
name: test_scenario_mcp
description: MCP configuration for test scenario environments

prompts:
  - name: test_context
    description: Test scenario context and setup

resources:
  - uri: file://test-scenarios/current/README.md
    name: test_scenario_guide
    category: test_documentation

tool_groups:
  - name: test_tools
    description: Tools available in test scenarios
    tools:
      - object_list
      - object_create
```

## Migration Path

1. **Current**: Programmatic registration (still supported as fallback)
2. **Migration**: Export current prompts to spec: `ExportPromptsToSpec()`
3. **Future**: Load from specs by default, programmatic as fallback
