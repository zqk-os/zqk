# Error Response Builder

## Overview

The `ErrorResponseBuilder` provides a standardized, fluent API for creating JSONRPC error responses. This ensures consistent error structure, proper error codes, and makes common errors easily accessible.

## Usage

### Builder Pattern

```go
// Custom error with builder
err := NewErrorResponseBuilder(NotFound, "Resource not found").
    WithData("resource_type", "object").
    WithData("resource_id", "obj_123").
    Build()
```

### Standard Error Functions

Common errors are available as static functions for clarity and consistency:

```go
// Missing parameter
err := NewMissingParameterError("rootCmd", "RegisterCLITools")

// Permission denied
err := NewPermissionDeniedError("object create", "insufficient permissions")

// Resource not found
err := NewNotFoundError("object", "obj_123")

// Configuration error
err := NewConfigurationError("root command not available", map[string]any{
    "function": "RegisterCLITools",
})

// Validation error
err := NewValidationError("email", "invalid format", "create_user")

// Invalid format
err := NewInvalidFormatError("xml", "format not allowed", "object list")
```

## Available Standard Error Functions

### Client Errors

| Function | Code | Use Case |
|----------|------|----------|
| `NewMissingParameterError(parameter, function)` | -32011 | Required parameter missing |
| `NewInvalidParameterError(parameter, reason, function)` | -32012 | Invalid parameter value |
| `NewValidationError(field, reason, function)` | -32010 | Validation failure |
| `NewInvalidFormatError(format, reason, context)` | -32014 | Invalid format |
| `NewUnauthenticatedError(operation)` | -32000 | Authentication required |
| `NewAuthenticationFailedError(reason, details)` | -32001 | Authentication failed |
| `NewPermissionDeniedError(operation, reason)` | -32002 | Permission denied |
| `NewAccessDeniedError(resource, reason)` | -32003 | Access denied to resource |
| `NewNotFoundError(resourceType, resourceID)` | -32020 | Resource not found |
| `NewAlreadyExistsError(resourceType, resourceID)` | -32021 | Resource already exists |
| `NewInvalidStateError(resourceType, currentState, requiredState)` | -32030 | Invalid resource state |
| `NewStateTransitionError(resourceType, fromState, toState, reason)` | -32031 | Invalid state transition |

### Server Errors

| Function | Code | Use Case |
|----------|------|----------|
| `NewServerError(message, details)` | -32050 | Generic server error |
| `NewConfigurationError(message, details)` | -32053 | Configuration error |
| `NewNotInitializedError(operation)` | -32051 | Server not initialized |
| `NewOperationFailedError(operation, reason)` | -32060 | Operation failed |

## Benefits

1. **Consistency**: All errors follow the same structure
2. **Type Safety**: Error codes are constants, not magic numbers
3. **Clarity**: Standard errors are self-documenting
4. **Extensibility**: Easy to add new standard error functions
5. **Structured Data**: Errors include relevant context in `Data` field
6. **Maintainability**: Changes to error structure happen in one place

## Examples

### Before (Manual Construction)

```go
return nil, &JSONRPCError{
    Code:    PermissionDenied,
    Message: fmt.Sprintf("Command not allowed: %s", reason),
    Data:    map[string]any{"command": commandPath, "reason": reason},
}
```

### After (Standard Function)

```go
return nil, NewPermissionDeniedError(commandPath, reason)
```

### Custom Error with Builder

```go
err := NewErrorResponseBuilder(Conflict, "Resource conflict").
    WithData("resource_type", "object").
    WithData("resource_id", "obj_123").
    WithData("conflicting_field", "name").
    Build()
```

## Migration

When migrating existing error creation:

1. Identify the error type (client vs server)
2. Check if a standard function exists
3. If yes, use the standard function
4. If no, use the builder pattern or create a new standard function

## Adding New Standard Errors

To add a new standard error function:

1. Add the function to `error_response_builder.go`
2. Use appropriate error code from `error_codes.go`
3. Include relevant context in `Data` field
4. Document in this file

Example:

```go
// NewRateLimitExceededError creates an error for rate limit violations
func NewRateLimitExceededError(operation string, limit int, window string) *JSONRPCError {
    return NewErrorResponseBuilder(RateLimitExceeded, "Rate limit exceeded").
        WithData("operation", operation).
        WithData("limit", limit).
        WithData("window", window).
        Build()
}
```
