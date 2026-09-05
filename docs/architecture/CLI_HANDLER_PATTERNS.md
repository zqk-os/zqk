# CLI Handler Patterns

**Last Verified:** 2026-08-31


This document describes the well-established patterns for CLI handlers in the <brand_substitution> codebase.

## Core Patterns

### 1. Processor Pattern

All CLI handlers should use the `Processor` pattern for unified access to context, storage, security, logging, and formatting.

```go
// Create processor (handles context, storage, security, logging)
proc, err := cli.NewProcessor(cmd)
if err != nil {
    return cli.EnhanceError(cmd, fmt.Errorf("failed to create processor: %w", err))
}
```

**Error returns (BLI-659):** When returning errors from RunE, use `cli.EnhanceError(cmd, err)` so users get consistent, experience-appropriate suggestions (e.g. "Check ID or path; use list to see available items."). Use it for every `return err` or `return fmt.Errorf(...)` that surfaces to the user. Respects `--verbose` and context profile (ai-agent/debug = terse, human = standard).

**Benefits:**
- Single source of truth for all CLI operations
- Consistent context handling across all commands
- Automatic profile and configuration management
- Unified error handling

### 2. Error Returns (EnhanceError and Guard)

When returning errors from RunE, use `cli.EnhanceError(cmd, err)` so users get consistent, experience-appropriate suggestions (BLI-659). Use it for every `return err` or `return fmt.Errorf(...)` that surfaces to the user. Respects `--verbose` and context profile (ai-agent/debug = terse, human = standard).

**Optional: Guard fluent helper** — For a more fluid, builder-style flow (aligned with CommandBuilder/HelpBuilder), use `cli.Guard(cmd)` to chain error checks and return an enhanced error in one expression. See `docs/process/architecture/CLI_ERROR_GUARD_OPTIONS.md` for full options and when to use Guard vs explicit if-blocks.

```go
// ✅ Good - explicit EnhanceError
if err != nil {
    proc.Logger().LogError("Failed to create object", err, logging.String("id", id))
    return cli.EnhanceError(cmd, fmt.Errorf("failed to create object: %w", err))
}
if err != nil {
    return cli.EnhanceError(cmd, err)
}

// ✅ Good - Guard (fluent, same behavior)
if err != nil {
    return cli.Guard(cmd).Err(err).Wrapf("failed to create processor: %w").Return()
}
return cli.Guard(cmd).Require(len(args) >= 1, "object ID is required").Return()

// ❌ Bad - no suggestion
if err != nil {
    return fmt.Errorf("failed to create object: %w", err)
}
```

### 3. Context Handling

Always use `proc.OperationContext()` for all operations. This context:
- Includes timeout handling
- Has proper cancellation support
- Respects command-level timeouts
- Includes tracing and metrics

```go
// Good: Use processor's operation context
result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, filter)

// Bad: Don't use cmd.Context() directly
result, err := proc.Storage().List(cmd.Context(), ...) // ❌
```

**For cache-aware operations:**
```go
opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
```

### 4. Logging

Use structured logging via the processor's logger:

```go
// Debug logging
proc.Logger().LogDebug("Operation started", logging.String("kind", kind))

// Info logging
proc.Logger().LogInfo("Operation completed", logging.String("id", id))

// Warning logging
proc.Logger().LogWarning("Non-critical issue", logging.Error(err))

// Error logging
proc.Logger().LogError("Operation failed", err, logging.String("kind", kind))
```

**Logging Guidelines:**
- Use `LogDebug` for detailed operation information
- Use `LogInfo` for important state changes
- Use `LogWarning` for non-critical issues that don't fail the operation
- Use `LogError` for errors that cause operation failure
- Always include relevant context (kind, id, etc.) using structured fields

### 5. Storage Operations

Always use `proc.Storage()` for storage operations:

```go
// Read
obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)

// Create
err := proc.Storage().Create(proc.OperationContext(), proc.SecurityContext(), obj)

// Update
err := proc.Storage().Update(proc.OperationContext(), proc.SecurityContext(), id, updates)

// List
result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, filter)

// Delete
err := proc.Storage().Delete(proc.OperationContext(), proc.SecurityContext(), id)
```

### 6. Security Context

Always use `proc.SecurityContext()` for security operations:

```go
secCtx := proc.SecurityContext()
// Use secCtx for all security-related operations
```

### 7. Storage Context

Use `proc.StorageContext()` for storage-specific settings:

```go
storageCtx := proc.StorageContext()
// Modify storage context if needed
storageCtx.EnableGrouping = true
storageCtx.MaxPageSize = 100
```

### 8. Output Formatting

Use `proc.Format()` to get the output format and `cli.WriteOutput()` for writing:

```go
format := proc.Format()
switch format {
case cli.FormatJSON:
    return outputJSON(cmd, data)
case cli.FormatYAML:
    return outputYAML(cmd, data)
case cli.FormatTable:
    return outputTable(cmd, data)
default:
    return outputTable(cmd, data)
}
```

Or use the helper:
```go
return cli.WriteOutput(cmd, []byte(output))
```

### 9. Validation

Validation is primarily handled by the storage layer during Create/Update operations. For pre-validation:

```go
if err := proc.ValidateObject(obj, kind, currentState); err != nil {
    return fmt.Errorf("validation failed: %w", err)
}
```

### 10. Profiles

Profiles are handled automatically via CLI context precedence:
1. System defaults
2. User config (~/.<cli_substitution>/config.yaml)
3. Project config (.<cli_substitution>/config.yaml)
4. Command flags (highest precedence)

No manual profile handling needed - the processor handles it.

### 11. Transactions

For operations that need transaction support, use context-aware operations:

```go
// Cache-aware operations
opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")

// Transaction-aware operations (if needed)
// Storage layer handles transactions automatically
```

## Refactoring Guidelines

### Reducing Complexity

When refactoring high-complexity CLI handlers:

1. **Extract Flag Parsing**: Create helper functions for flag parsing
2. **Extract Filter Building**: Create helper functions for building filters
3. **Extract Output Formatting**: Create helper functions for different output formats
4. **Extract Validation Logic**: Move validation to separate functions
5. **Extract Business Logic**: Move business logic to separate functions
6. **Use Early Returns**: Reduce nesting with early returns
7. **Extract Common Patterns**: Identify and extract repeated patterns

### Example Refactoring

**Before (high complexity):**
```go
func runList(cmd *cobra.Command, args []string) error {
    proc, err := cli.NewProcessor(cmd)
    if err != nil {
        return cli.EnhanceError(cmd, fmt.Errorf("failed to create processor: %w", err))
    }
    
    // 200+ lines of complex logic
    // Multiple nested conditions
    // Repeated patterns
    // ...
}
```

**After (reduced complexity):**
```go
func runList(cmd *cobra.Command, args []string) error {
    proc, err := cli.NewProcessor(cmd)
    if err != nil {
        return fmt.Errorf("failed to create processor: %w", err)
    }
    
    // Parse flags
    filters, err := parseListFilters(cmd, proc)
    if err != nil {
        return cli.EnhanceError(cmd, err)
    }
    
    // Build list filter
    listFilter, err := buildListFilter(cmd, args, filters, proc)
    if err != nil {
        return cli.EnhanceError(cmd, err)
    }
    
    // Execute list operation
    result, err := executeListOperation(cmd, proc, listFilter)
    if err != nil {
        return cli.EnhanceError(cmd, err)
    }
    
    // Output results
    return outputListResults(cmd, proc, result)
}
```

## Common Helper Functions

### Flag Parsing

```go
func parseListFilters(cmd *cobra.Command, proc *cli.Processor) (map[string]any, error) {
    filters := make(map[string]any)
    filterStrs, err := cmd.Flags().GetStringArray("filter")
    if err != nil {
        return filters, nil
    }
    
    for _, filterStr := range filterStrs {
        fieldName, filterValue, err := ParseFilterString(filterStr)
        if err != nil {
            proc.Logger().LogError("Failed to parse filter", err, logging.String("filter", filterStr))
            return nil, err
        }
        filters[fieldName] = filterValue
    }
    
    return filters, nil
}
```

### Filter Building

```go
func buildListFilter(cmd *cobra.Command, args []string, filters map[string]any, proc *cli.Processor) (storage.ListFilter, error) {
    // Extract kind from args or flags
    kind := extractKind(cmd, args)
    
    // Build list filter
    listFilter := storage.ListFilter{
        Kind:    kind,
        Filters: filters,
    }
    
    // Add sorting
    if sortBy, err := cmd.Flags().GetString("sort-by"); err == nil && sortBy != "" {
        listFilter.SortBy = sortBy
        if sortAsc, err := cmd.Flags().GetBool("sort-asc"); err == nil {
            listFilter.SortAsc = sortAsc
        }
    }
    
    // Add pagination
    if offset, err := cmd.Flags().GetInt("offset"); err == nil {
        listFilter.Offset = offset
    }
    if limit, err := cmd.Flags().GetInt("limit"); err == nil {
        listFilter.Limit = limit
    }
    
    return listFilter, nil
}
```

## Anti-Patterns to Avoid

1. **Don't use `cmd.Context()` directly** - Always use `proc.OperationContext()`
2. **Don't create storage providers manually** - Use `proc.Storage()`
3. **Don't create loggers manually** - Use `proc.Logger()`
4. **Don't parse flags without error handling** - Always handle flag parsing errors
5. **Don't ignore context cancellation** - Always respect context cancellation
6. **Don't mix business logic with CLI logic** - Extract business logic to separate functions
7. **Don't duplicate flag parsing logic** - Extract to helper functions
8. **Don't ignore storage context** - Always use `proc.StorageContext()`

## Testing Patterns

When testing CLI handlers:

1. Use `cli.NewProcessor(cmd)` in tests
2. Mock storage provider if needed
3. Test flag parsing separately
4. Test business logic separately
5. Test output formatting separately

