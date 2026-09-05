# Command Result Builder

**Last Verified:** 2026-08-31

## Overview

The `CommandResultBuilder` provides a fluent API for building standardized CLI command result responses. It integrates with the error codes scheme and ensures consistent structure for both success and error cases.

## Usage

### Basic Success Case

```go
result := NewCommandResultBuilder("object list", []string{"--format", "json"}).
    WithResult(parsedOutput).
    WithStdout(stdout).
    WithStderr(stderr).
    WithSuccess(true).
    Build()
```

### Error Case

```go
result := NewCommandResultBuilder("object create", []string{"--id", "OBJ-001"}).
    WithResult(partialResult).
    WithStdout(stdout).
    WithStderr(stderr).
    WithError(execErr).
    WithErrorCode(AccessDenied).
    Build()
```

### Convenience Functions

```go
// Error case with automatic error code
result := NewCommandResultFromError(
    "object delete",
    []string{"OBJ-001"},
    err,
    AccessDenied,
).Build()

// Success case
result := NewCommandResultFromSuccess(
    "object list",
    []string{"--format", "json"},
    parsedOutput,
).Build()
```

## Features

### Automatic Error Code Inference

If an error occurs but no error code is explicitly set, the builder attempts to infer the error code from the error message:

- `"permission denied"`, `"access denied"`, `"forbidden"` → `PermissionDenied` (-32002)
- `"not found"`, `"does not exist"`, `"no such"` → `NotFound` (-32020)
- `"already exists"`, `"duplicate"`, `"conflict"` → `AlreadyExists` (-32021)
- `"invalid"`, `"malformed"`, `"bad format"` → `InvalidParameter` (-32012)
- `"timeout"`, `"timed out"` → `OperationTimeout` (-32061)
- Default → `InternalError` (-32603)

### Standardized Structure

All command results include:

```go
{
    "command": "object list",           // Command path
    "args": ["--format", "json"],       // Command arguments
    "success": true,                    // Success flag
    // ... result data from command output
}
```

Error results additionally include:

```go
{
    "command": "object create",
    "args": ["--id", "OBJ-001"],
    "success": false,
    "execution_error": "error message",
    "error_code": -32003,              // Numeric error code
    "error_type": "access_denied",     // String identifier
    "stderr": "...",                   // If available
    "stdout": "...",                   // If available (partial output)
    // ... any partial result data
}
```

## Integration

The builder is used by `buildCommandResult()` which is called from CLI command execution. This ensures all CLI command results have consistent structure and proper error handling.

## Benefits

1. **Consistency**: All command results follow the same structure
2. **Error Codes**: Integrates with the error codes scheme
3. **Type Safety**: Fluent API prevents common mistakes
4. **Automatic Inference**: Reduces boilerplate for common error cases
5. **Extensibility**: Easy to add custom data or error handling
