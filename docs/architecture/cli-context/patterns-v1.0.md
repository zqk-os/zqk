# Context Building Patterns

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Patterns and examples for building and applying contexts in the zqk CLI

This document describes the patterns for building and applying contexts in the zqk CLI.

## Overview

The context system supports multiple patterns for building contexts:

1. **Sequential Pattern** - Layers processed in order (system → user → project → command)
2. **Hierarchical Pattern** - Tree structure with inheritance
3. **Hybrid Pattern** - Combination of sequential and hierarchical
4. **Derivation Pattern** - Derive new contexts from existing ones

## Standard Pattern: Sequential (Most Common)

The standard pattern for CLI commands is sequential processing:

```
System Defaults → User Config → Project Config → Command Flags
```

### Using ContextBuilderPattern (Recommended)

```go
import clictx "github.com/lanceman/zqk/internal/cli/context"

// Build context with fluent API
ctx, err := clictx.NewContextBuilderPattern().
    WithSystemDefaults(systemDefaults).
    WithUserConfig(userConfig).
    WithProjectConfig(projectRoot, projectConfig).
    WithCommandFlags(commandFlags).
    Build()
```

### Using ContextBuilder Directly

```go
import clictx "github.com/lanceman/zqk/internal/cli/context"

builder := clictx.NewContextBuilder(clictx.ModeSequential)
builder.AddContext(systemCtx, "system")
builder.AddContext(userCtx, "user")
builder.AddContext(projectCtx, "project")
builder.AddContext(commandCtx, "command")
ctx, err := builder.Build()
```

### Using BuildContextFromLayers (Convenience Function)

```go
import clictx "github.com/lanceman/zqk/internal/cli/context"

ctx, err := clictx.BuildContextFromLayers(
    systemDefaults,
    userConfig,
    projectConfig,
    commandFlags,
    projectRoot,
)
```

## Derivation Pattern (For Variations)

When you have an existing context and need a variation:

### Using Derive() Method

```go
proc, _ := cli.NewProcessor(cmd)
baseCtx := proc.Context()

// Derive with overrides
jsonCtx := baseCtx.Derive(map[string]any{
    "format": "json",
    "verbose": true,
})
```

### Using Convenience Methods

```go
proc, _ := cli.NewProcessor(cmd)

// Simple overrides
jsonProc := proc.WithFormat("json")
verboseProc := proc.WithVerbose(true)
quietProc := proc.WithQuiet(true)

// Complex overrides
customProc := proc.WithStorageSettings(100, 50, true, 10)
```

### Using BuildContextWithOverrides

```go
import clictx "github.com/lanceman/zqk/internal/cli/context"

baseCtx := proc.Context()
derivedCtx, err := clictx.BuildContextWithOverrides(baseCtx, map[string]any{
    "format": "json",
    "verbose": true,
})
```

## Hierarchical Pattern (For Complex Structures)

Use hierarchical processing when you have parent-child relationships:

```go
builder := clictx.NewContextBuilder(clictx.ModeHierarchical)
builder.SetRoot(systemCtx, "system")
builder.AddChild(userCtx, "user")
builder.AddChild(projectCtx, "project")
builder.AddNestedChild("project", featureCtx, "feature")
ctx, err := builder.Build()
```

## Hybrid Pattern (For Mixed Structures)

Use hybrid processing when you need both sequential and hierarchical aspects:

```go
builder := clictx.NewContextBuilder(clictx.ModeHybrid)
builder.SetRoot(systemCtx, "system")
builder.AddChild(userCtx, "user")
builder.AddChild(projectCtx, "project")
// Siblings processed sequentially, children inherit hierarchically
ctx, err := builder.Build()
```

## Common Use Cases

### 1. Building Context in Command Handler

```go
func runCommand(cmd *cobra.Command, args []string) error {
    // Standard: Use processor (handles all context building)
    proc, err := cli.NewProcessor(cmd)
    if err != nil {
        return err
    }
    
    // Access context
    ctx := proc.Context()
    format := proc.Format()
    
    // Use context...
    return nil
}
```

### 2. Deriving Context for Sub-Operation

```go
proc, _ := cli.NewProcessor(cmd)

// Need JSON output for this operation
jsonProc := proc.WithFormat("json")

// Use jsonProc for JSON operations
outputJSON(jsonProc.Context(), data)
```

### 3. Building Custom Context

```go
// Build context from scratch
ctx, err := clictx.NewContextBuilderPattern().
    WithSystemDefaults(map[string]any{
        "format": "table",
        "verbose": false,
    }).
    WithUserConfig(userConfig).
    WithOverride("format", "json").  // Override format
    Build()
```

### 4. Building Context for Testing

```go
// Build minimal context for testing
testCtx, err := clictx.NewContextBuilderPattern().
    WithSystemDefaults(map[string]any{
        "format": "json",
        "verbose": true,
    }).
    Build()
```

## Precedence Order

When contexts are merged, precedence is (lowest to highest):

1. **System Defaults** - Built-in defaults
2. **User Config** - `~/.zqk/config.yaml`
3. **Project Config** - `.zqk/config.yaml`
4. **Command Flags** - Command-line arguments (highest precedence)

## Best Practices

1. **Use Processor for Commands**: Always use `cli.NewProcessor(cmd)` in command handlers
2. **Derive for Variations**: Use `Derive()` or convenience methods for variations
3. **Single Context Principle**: Pass one context object, not multiple
4. **Use Builder for Custom**: Use `ContextBuilderPattern` for custom context building
5. **Respect Precedence**: Don't manually override precedence - let the system handle it

## Examples

### Example 1: Standard Command Handler

```go
func runList(cmd *cobra.Command, args []string) error {
    proc, err := cli.NewProcessor(cmd)
    if err != nil {
        return err
    }
    
    // Context is already built and merged
    format := proc.Format()
    verbose := proc.IsVerbose()
    
    // Use processor methods...
    return nil
}
```

### Example 2: Deriving Context for Sub-Operation

```go
func runList(cmd *cobra.Command, args []string) error {
    proc, err := cli.NewProcessor(cmd)
    if err != nil {
        return err
    }
    
    // Main operation uses default format
    result := performList(proc)
    
    // Sub-operation needs JSON
    jsonProc := proc.WithFormat("json")
    outputJSON(jsonProc.Context(), result)
    
    return nil
}
```

### Example 3: Building Context from Scratch

```go
func buildCustomContext() (*clictx.Context, error) {
    return clictx.NewContextBuilderPattern().
        WithSystemDefaults(map[string]any{
            "format": "table",
        }).
        WithUserConfig(loadUserConfig()).
        WithOverride("format", "json").
        Build()
}
```

## Migration Guide

If you're migrating from old patterns:

### Old Pattern (Don't Use)
```go
ctx := cli.GetContext(cmd)
format := cli.GetFormat(cmd)
logger := logging.GetLoggerFromContext(cmd.Context())
```

### New Pattern (Use This)
```go
proc, err := cli.NewProcessor(cmd)
format := proc.Format()
logger := proc.Logger()
ctx := proc.Context()
```

## Related Documentation

- [Context Architecture](./architecture-v1.0.md): Single Context Principle and architecture overview
- [CLI Context Package](../../../../internal/cli/context/README.md): Package index

