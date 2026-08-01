# Context Architecture: Single Context Principle

**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Architecture documentation for the CLI context system following the Single Context Principle

## Overview

The context system follows a **Single Context Principle**: everything should be derivable from a single starting context. Functions should not need multiple context objects passed in.

## How It Works

1. **Initial Load**: `LoadContext()` processes all layers (system, user, project, command) and returns a single merged context
2. **Single Source**: All functions receive one fully-processed context
3. **Derive Variations**: If you need a different context, derive it from the starting context

## Context Derivation Methods

### On Context Object

```go
ctx := proc.Context()

// Simple overrides
jsonCtx := ctx.WithFormat("json")
verboseCtx := ctx.WithVerbose(true)
quietCtx := ctx.WithQuiet(true)
profileCtx := ctx.WithProfile("ai-agent")

// Complex overrides
customCtx := ctx.Derive(map[string]any{
    "format": "yaml",
    "verbose": true,
    "storage": map[string]any{
        "max_page_size": 100,
        "enable_grouping": true,
    },
})
```

### On Processor Object

```go
proc, _ := cli.NewProcessor(cmd)

// Derive new processor with different format
jsonProc := proc.WithFormat("json")

// Derive new processor with different storage settings
pagedProc := proc.WithStorageSettings(100, 50, true, 10)
```

## Benefits

1. **No Multiple Parameters**: Functions don't need `(systemCtx, userCtx, projectCtx, commandCtx)` - just one context
2. **Clear Precedence**: The starting context already has precedence applied
3. **Easy Testing**: Mock a single context object
4. **Flexible**: Derive any variation you need on-demand
5. **Consistent**: All code works with the same context structure

## When to Use Advanced Methods

The `BuildContextSequential`, `BuildContextHierarchical`, and `BuildContextHybrid` methods are only for:
- Building contexts from scratch (not from an existing context)
- Complex multi-source context assembly
- Specialized use cases

For normal operations, always use `Derive()` or the convenience methods.

## Example: Command Function

```go
func runMyCommand(cmd *cobra.Command, args []string) error {
    // Get single processed context
    proc, err := cli.NewProcessor(cmd)
    if err != nil {
        return err
    }
    
    // Use the context directly
    format := proc.Format()
    verbose := proc.IsVerbose()
    
    // Derive variations if needed
    if needJSON {
        jsonProc := proc.WithFormat("json")
        // Use jsonProc...
    }
    
    // Everything derives from the single starting context
    return nil
}
```

## Related Documentation

- [Context Building Patterns](./patterns-v1.0.md): Detailed patterns and examples for context building
- [CLI Context Package](../../../../internal/cli/context/README.md): Package index

