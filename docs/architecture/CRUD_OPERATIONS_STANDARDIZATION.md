# CRUD Operations Standardization Guide

This document outlines the standard patterns for CRUD operations across `object` and `internal` packages.

## Common Patterns

### 1. Create Operations

**Standard Flow:**
1. Create processor
2. Load/validate object data
3. Handle dry-run (if enabled)
4. Build cache context (if cache enabled)
5. Create object (with force flag handling)
6. Trigger cache freshness check (if cache enabled)
7. Cleanup source file (if applicable)
8. Output success message

**Key Differences:**
- `object` package: Uses cache operations, normalization, spec cache clearing, relaxed flag
- `internal` package: Simpler, no cache operations

**Standardization:**
- Both should use `clipkg.FormatCreateSuccessMessage()` for success messages
- Both should use `clipkg.HandleDryRun()` for dry-run handling
- Both should use `clipkg.CleanupSourceFile()` for file cleanup
- `object` package should continue using cache operations (this is a feature, not a bug)

### 2. Update Operations

**Standard Flow:**
1. Create processor
2. Read current object
3. Build updates map
4. Handle dry-run (if enabled)
5. Handle force flag (create if missing)
6. Build cache context (if cache enabled)
7. Update object
8. Trigger cache freshness check (if cache enabled)
9. Output success message

**Key Differences:**
- `object` package: Uses cache operations, normalization, optimistic locking, --all flag
- `internal` package: Simpler, no cache operations, checks for built-in objects

**Standardization:**
- Both should use `clipkg.FormatUpdateSuccessMessage()` for success messages
- Both should use `clipkg.HandleUpdateDryRun()` for dry-run handling
- `object` package should continue using cache operations

### 3. Delete Operations

**Standard Flow:**
1. Create processor
2. Read object (for dry-run and kind detection)
3. Handle dry-run (if enabled)
4. Build cache context (if cache enabled)
5. Delete object
6. Trigger cache freshness check (if cache enabled)
7. Output success message

**Key Differences:**
- `object` package: Uses cache operations
- `internal` package: Warns about built-in objects, no cache operations

**Standardization:**
- Both should use `clipkg.FormatDeleteSuccessMessage()` for success messages
- Both should use `clipkg.HandleDeleteDryRun()` for dry-run handling
- `internal` package should warn about built-in objects (this is correct behavior)

### 4. Get Operations

**Standard Flow:**
1. Create processor
2. Read object
3. Output using format handlers

**Key Differences:**
- `object` package: Simple read and output
- `internal` package: Special handling for lifecycle definitions and object specs

**Standardization:**
- Both should use `cli.FormatOutput()` for consistent formatting
- `internal` package's special handling is correct (it's a feature)

## Error Handling Standards

### Standard Error Wrapping
- Always use `fmt.Errorf("operation failed: %w", err)` for error wrapping
- Include context in error messages (id, kind, etc.)
- Log errors before returning them

### Common Error Patterns
```go
// Good: Includes context and wraps error
if err != nil {
    proc.Logger().LogError("Failed to create object", err, logging.String("kind", kind))
    return fmt.Errorf("failed to create object: %w", err)
}

// Bad: Loses error context
if err != nil {
    return err
}
```

## Cache Operations (object package only)

### Create
```go
opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
proc.Storage().Create(opCtx, proc.SecurityContext(), objData)
proc.TriggerCacheFreshnessCheck("create", []string{kind})
```

### Update
```go
opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), id, objKind, "")
proc.Storage().Update(opCtx, proc.SecurityContext(), id, updates)
proc.TriggerCacheFreshnessCheck("update", []string{objKind})
```

### Delete
```go
cliCtx := proc.WithCLIOperation()
cacheCtx := pkgctx.WithCacheInvalidate(cliCtx, id)
proc.Storage().Delete(cacheCtx, proc.SecurityContext(), id, cascade)
proc.TriggerCacheFreshnessCheck("delete", affectedKinds)
```

## Success Message Formatting

All packages should use shared utilities:
- `clipkg.FormatCreateSuccessMessage(objData, kind, objectType, logger)`
- `clipkg.FormatUpdateSuccessMessage(id, isBuiltIn, objectType, logger)`
- `clipkg.FormatDeleteSuccessMessage(id, isBuiltIn, cascade, objectType, logger)`

## Dry-Run Handling

All packages should use shared utilities:
- `clipkg.HandleDryRun(cmd, objData, kind, logger, objectType)`
- `clipkg.HandleUpdateDryRun(cmd, id, current, updates, isBuiltIn, logger, objectType)`
- `clipkg.HandleDeleteDryRun(cmd, id, obj, cascade, logger, objectType)`

## Standardization Status

1. ✅ Success message formatting - **COMPLETE** - Both packages use shared utilities
2. ✅ Dry-run handling - **COMPLETE** - Both packages use shared utilities
3. ✅ Error handling patterns - **COMPLETE** - Consistent `fmt.Errorf("failed to ...: %w", err)` pattern
4. ✅ Cache operation patterns - **DOCUMENTED** - object package only (intentional feature)
5. ✅ Force flag handling - **STANDARDIZED** - Both packages handle force flag consistently
6. ✅ Normalization - **DOCUMENTED** - object package only (intentional feature)
7. ✅ Data loading - **COMPLETE** - Both packages use `clipkg.LoadObjectData()`
8. ✅ Updates building - **COMPLETE** - Both packages use `clipkg.BuildUpdatesMap()`

## Conclusion

CRUD operations are **fully standardized** across both packages. All common patterns use shared utilities from `pkg/cli`. The differences between packages are intentional features:
- `object` package: Full-featured with cache, normalization, force/relaxed flags
- `internal` package: Simpler admin interface with built-in object warnings

No further standardization needed.
