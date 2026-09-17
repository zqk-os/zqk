# MCP Error Code System

**Last Verified:** 2026-08-31

## Overview

The MCP server uses a comprehensive, HTTP-like error code system that categorizes errors into distinct ranges for better error handling, debugging, and client behavior determination.

## Error Code Structure

### Standard JSON-RPC 2.0 Error Codes (-32768 to -32000)

These codes are defined by the JSON-RPC 2.0 specification and are used for protocol-level errors:

| Code | Constant | Description |
|------|----------|-------------|
| -32700 | `ParseError` | Invalid JSON was received |
| -32600 | `InvalidRequest` | The JSON sent is not a valid Request object |
| -32601 | `MethodNotFound` | The method does not exist / is not available |
| -32602 | `InvalidParams` | Invalid method parameter(s) |
| -32603 | `InternalError` | Internal JSON-RPC error |

### Client Error Codes (-32000 to -32049)

These errors indicate problems with the client's request. The client should modify the request or provide different parameters.

#### Authentication & Authorization (-32000 to -32009)

| Code | Constant | Description |
|------|----------|-------------|
| -32000 | `Unauthenticated` | Authentication required |
| -32001 | `AuthenticationFailed` | Authentication failed |
| -32002 | `PermissionDenied` | Permission denied for operation |
| -32003 | `AccessDenied` | Access denied to resource |
| -32004 | `Forbidden` | Operation is forbidden |
| -32005 | `AccountNotFound` | Account not found |
| -32006 | `AccountInactive` | Account is inactive |
| -32007 | `RoleMismatch` | Role mismatch |

#### Validation & Request Errors (-32010 to -32019)

| Code | Constant | Description |
|------|----------|-------------|
| -32010 | `ValidationError` | Validation error |
| -32011 | `MissingParameter` | Required parameter missing |
| -32012 | `InvalidParameter` | Invalid parameter value |
| -32013 | `ParameterConflict` | Parameter conflict |
| -32014 | `InvalidFormat` | Invalid format |
| -32015 | `UnsupportedOperation` | Unsupported operation |

#### Resource Errors (-32020 to -32029)

| Code | Constant | Description |
|------|----------|-------------|
| -32020 | `NotFound` | Resource not found |
| -32021 | `AlreadyExists` | Resource already exists |
| -32022 | `Conflict` | Resource conflict |
| -32023 | `Gone` | Resource gone |

#### State & Lifecycle Errors (-32030 to -32039)

| Code | Constant | Description |
|------|----------|-------------|
| -32030 | `InvalidState` | Invalid state |
| -32031 | `StateTransitionError` | State transition error |
| -32032 | `LifecycleViolation` | Lifecycle violation |

#### Rate Limiting & Quota Errors (-32040 to -32049)

| Code | Constant | Description |
|------|----------|-------------|
| -32040 | `RateLimitExceeded` | Rate limit exceeded |
| -32041 | `QuotaExceeded` | Quota exceeded |
| -32042 | `TooManyRequests` | Too many requests |

### Server Error Codes (-32050 to -32099)

These errors indicate problems on the server side. The server should handle, log, or recover from these.

#### Server Initialization & Configuration (-32050 to -32059)

| Code | Constant | Description |
|------|----------|-------------|
| -32050 | `ServerError` | Generic server error |
| -32051 | `NotInitialized` | Server not initialized |
| -32052 | `AlreadyInitialized` | Server already initialized |
| -32053 | `ConfigurationError` | Configuration error |

#### Operation & Execution Errors (-32060 to -32079)

| Code | Constant | Description |
|------|----------|-------------|
| -32060 | `OperationFailed` | Operation failed |
| -32061 | `OperationTimeout` | Operation timeout |
| -32062 | `OperationCancelled` | Operation cancelled |
| -32063 | `ConcurrentModification` | Concurrent modification |
| -32064 | `Deadlock` | Deadlock detected |

#### Storage & Data Errors (-32080 to -32089)

| Code | Constant | Description |
|------|----------|-------------|
| -32080 | `StorageError` | Storage error |
| -32081 | `DataCorruption` | Data corruption |
| -32082 | `StorageUnavailable` | Storage unavailable |

#### System & Infrastructure Errors (-32090 to -32099)

| Code | Constant | Description |
|------|----------|-------------|
| -32090 | `SystemUnavailable` | System unavailable |
| -32091 | `ServiceUnavailable` | Service unavailable |
| -32092 | `OutOfMemory` | Out of memory |
| -32093 | `ResourceExhausted` | Resource exhausted |

## Error Code Categories

Errors are automatically categorized:

- **Standard**: JSON-RPC 2.0 protocol errors
- **Client**: Client-side errors (4xx-like) - client should fix the request
- **Server**: Server-side errors (5xx-like) - server should handle the issue

## Helper Functions

### Categorization

```go
category := CategorizeErrorCode(code)
// Returns: ErrorCategoryStandard, ErrorCategoryClient, or ErrorCategoryServer

isClient := IsClientError(code)
isServer := IsServerError(code)
isStandard := IsStandardError(code)
```

### Criticality Detection

```go
isCritical := IsCriticalErrorCode(code)
```

Critical errors are:
- All server errors (server should handle)
- Certain client errors (authentication, authorization, rate limiting)

## Usage Examples

### Creating Errors

```go
// Client error
return nil, &JSONRPCError{
    Code:    PermissionDenied, // -32002
    Message: "Permission denied for operation",
    Data:    map[string]any{"operation": "write"},
}

// Server error
return nil, &JSONRPCError{
    Code:    OperationFailed, // -32060
    Message: "Operation failed",
    Data:    map[string]any{"reason": "timeout"},
}
```

### Error Handling

```go
if err != nil {
    if jsonrpcErr, ok := err.(*JSONRPCError); ok {
        if IsClientError(jsonrpcErr.Code) {
            // Client should fix the request
        } else if IsServerError(jsonrpcErr.Code) {
            // Server should handle or log
        }
        
        if IsCriticalErrorCode(jsonrpcErr.Code) {
            // Send notification or special handling
        }
    }
}
```

## Configuration

Critical error codes can be configured in `.zqk/mcp/config.yaml`:

```yaml
mcp_server:
  error_handling:
    critical_error_codes:
      - -32002  # PermissionDenied
      - -32003  # AccessDenied
      - -32050  # ServerError
      - -32060  # OperationFailed
```

If not configured, the system uses `IsCriticalErrorCode()` which automatically determines criticality based on error category.

## Benefits

1. **Clear Categorization**: Errors are clearly categorized as client vs server
2. **Logical Ordering**: Related errors are grouped together
3. **Extensible**: Easy to add new error codes in appropriate ranges
4. **Type-Safe**: Constants prevent magic numbers
5. **Configurable**: Critical error codes can be customized
6. **HTTP-Like**: Familiar structure similar to HTTP status codes
