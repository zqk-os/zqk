# MCP Server Refactoring - Fully Capable Implementation

**Last Verified:** 2026-08-31


**Status**: Complete  
**Date**: 2025-12-30  
**Purpose**: Document the refactored MCP server using abstraction layers

## Overview

The MCP server has been fully refactored to use a clean, layered architecture with:
- **Transport Layer**: Handles message format (raw JSON vs Content-Length)
- **Protocol Layer**: Standardizes JSON-RPC 2.0 structure
- **Handler Layer**: Routes method calls with middleware support

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Server.Serve()                        │
├─────────────────────────────────────────────────────────┤
│  Transport Layer (DefaultTransport)                     │
│  └─ Reads/writes messages in correct format             │
├─────────────────────────────────────────────────────────┤
│  Protocol Layer (JSONRPCRequest/Response)                │
│  └─ Unmarshals requests to common shape                  │
│  └─ Marshals responses from common shape                │
├─────────────────────────────────────────────────────────┤
│  Handler Layer (MethodRouter + Middleware)               │
│  └─ Routes to method-specific handlers                  │
│  └─ Applies middleware (trace logging)                  │
└─────────────────────────────────────────────────────────┘
```

## Implementation Details

### 1. Transport Layer (`transport.go`)

**DefaultTransport** automatically detects and handles:
- **Raw JSON**: Messages starting with `{` or `[`
- **Content-Length Framed**: Messages with Content-Length headers

**Key Features**:
- Format detection via peeking at first character
- Response format matches request format
- Pluggable interface for custom transports

### 2. Protocol Layer (`protocol.go`)

**Common Shapes**:
- `JSONRPCRequest`: All requests unmarshal to this
- `JSONRPCResponse`: All responses use this structure
- `JSONRPCError`: Implements `error` interface

**Helper Functions**:
- `UnmarshalRequest()`: Unmarshals raw bytes to `JSONRPCRequest`
- `NewResponse()`: Creates success response
- `NewErrorResponse()`: Creates error response

### 3. Handler Layer (`handler.go` + `server_handlers.go`)

**MethodRouter**:
- Routes method calls to registered handlers
- Supports default handler for unregistered methods
- Returns `JSONRPCError` for method not found

**Handlers** (in `server_handlers.go`):
- `handleInitialize`: Server initialization
- `handleToolsList`: List available tools
- `handleResourcesList`: List available resources
- `handlePromptsList`: List available prompts
- `handleRootsList`: List available roots
- `handleToolsCall`: Execute a tool
- `handleShutdown`: Graceful shutdown
- `handleNotificationInitialized`: Handle initialized notification
- `handleNotificationCancelled`: Handle cancellation notification

**Middleware**:
- `TraceMiddleware`: Logs all requests/responses when trace enabled
- `Chain()`: Composes multiple middlewares

## Server Flow

### Request Processing

1. **Read Message** (`transport.ReadMessage()`)
   - Detects format (raw JSON vs Content-Length)
   - Returns message bytes and format info

2. **Unmarshal Request** (`UnmarshalRequest()`)
   - Validates JSON-RPC version
   - Returns `JSONRPCRequest` with common shape

3. **Route to Handler** (`MethodRouter.Handle()`)
   - Looks up handler for method
   - Applies middleware (trace logging)
   - Calls handler with context and params

4. **Build Response** (`NewResponse()` or `NewErrorResponse()`)
   - Success: `JSONRPCResponse` with result
   - Error: `JSONRPCResponse` with error (from `JSONRPCError`)

5. **Write Response** (`transport.WriteMessage()`)
   - Matches request format (raw JSON ↔ raw JSON)
   - Writes to stdout

### Notification Handling

Notifications (methods without `id`) are handled specially:
- Processed but no response sent
- `NotificationSentinel` error type indicates no response needed
- Used for `notifications/initialized` and `notifications/cancelled`

## Key Features

### 1. Format Matching
- Requests in raw JSON format get raw JSON responses
- Requests in Content-Length format get Content-Length responses
- Automatic detection and matching

### 2. Error Handling
- Standard JSON-RPC 2.0 error codes
- MCP-specific error codes (-32000 to -32099)
- Type-safe error responses

### 3. Middleware Support
- Trace logging middleware (when enabled)
- Easy to add new middleware (auth, metrics, etc.)
- Composable via `Chain()`

### 4. Extensibility
- Easy to add new method handlers
- Pluggable transport implementations
- Custom middleware support

## Code Structure

```
pkg/mcp/
├── transport.go          # Transport layer (message format)
├── protocol.go           # Protocol layer (JSON-RPC structure)
├── handler.go            # Handler layer (routing + middleware)
├── server.go             # Main server implementation
├── server_handlers.go    # Method-specific handlers
├── cli_bridge.go        # CLI command bridge
├── handlers_graph.go    # Graph traversal handlers
└── ...
```

## Benefits

1. **Separation of Concerns**: Each layer has single responsibility
2. **Testability**: Each layer can be tested independently
3. **Maintainability**: Clear boundaries make code easier to understand
4. **Extensibility**: Easy to add new transports, handlers, or middleware
5. **Type Safety**: Common shapes provide compile-time guarantees
6. **Protocol Compliance**: Full JSON-RPC 2.0 compliance

## Migration Notes

The old `writeMCPResponse()` and `writeMCPError()` functions are still present for backwards compatibility but are no longer used by the main server loop. They can be removed in a future cleanup if not needed elsewhere.

## Testing

The server maintains full backward compatibility:
- All existing MCP methods work as before
- Same protocol behavior
- Same error handling
- Same trace logging (when enabled)

## Next Steps

Potential enhancements:
1. Add more middleware (metrics, rate limiting, etc.)
2. Add custom transport implementations (SSE, WebSocket)
3. Add request validation middleware
4. Add response transformation middleware
5. Add handler-level error recovery

## Related Documents

- [MCP Abstraction Layer](./MCP_ABSTRACTION_LAYER.md)
- [MCP Server Architecture](./mcp-server-architecture.md)
- [MCP CLI Bridge](./mcp-cli-bridge-v1.0.md)
- [MCP Scalability](./MCP_SCALABILITY.md)

