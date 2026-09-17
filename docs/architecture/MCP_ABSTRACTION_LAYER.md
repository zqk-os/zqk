# MCP Abstraction Layer Architecture

**Last Verified:** 2026-08-31


**Status**: Design Document  
**Created**: 2025-12-30  
**Purpose**: Formalize MCP protocol handling with pluggable transport, protocol, and handler layers

## Overview

The MCP (Model Context Protocol) abstraction layer provides a clean separation of concerns for handling MCP protocol communication. It consists of three main layers:

1. **Transport Layer** - Handles message format (raw JSON vs Content-Length framed)
2. **Protocol Layer** - Handles JSON-RPC 2.0 structure (requests/responses)
3. **Handler Layer** - Routes method calls to appropriate handlers with middleware support

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    MCP Server                           │
├─────────────────────────────────────────────────────────┤
│  Handler Layer (MethodRouter, Middleware)               │
│  └─ Routes method calls to handlers                    │
│  └─ Supports middleware (logging, auth, etc.)         │
├─────────────────────────────────────────────────────────┤
│  Protocol Layer (JSONRPCRequest, JSONRPCResponse)      │
│  └─ Standard JSON-RPC 2.0 structure                     │
│  └─ Common unmarshaling interface                       │
├─────────────────────────────────────────────────────────┤
│  Transport Layer (Transport interface)                  │
│  └─ Handles raw JSON vs Content-Length framed          │
│  └─ Pluggable transport implementations                 │
└─────────────────────────────────────────────────────────┘
```

## Components

### 1. Transport Layer (`transport.go`)

**Purpose**: Abstract message format handling

**Interface**:
```go
type Transport interface {
    ReadMessage(reader *bufio.Reader) ([]byte, *MessageFormat, error)
    WriteMessage(writer *bufio.Writer, data []byte, format *MessageFormat) error
}
```

**Default Implementation**: `DefaultTransport`
- Automatically detects message format (raw JSON vs Content-Length)
- Supports both formats seamlessly
- Can be replaced with custom implementations

**Message Format**:
```go
type MessageFormat struct {
    IsRawJSON bool // true = raw JSON, false = Content-Length framed
}
```

**Benefits**:
- Pluggable transport implementations
- Easy to add new formats (e.g., SSE, WebSocket)
- Testable in isolation

### 2. Protocol Layer (`protocol.go`)

**Purpose**: Standardize JSON-RPC 2.0 structure

**Common Shapes**:
```go
// All requests unmarshal to this shape
type JSONRPCRequest struct {
    JSONRPC string          `json:"jsonrpc"`  // Always "2.0"
    ID      interface{}     `json:"id,omitempty"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

// All responses use this shape
type JSONRPCResponse struct {
    JSONRPC string      `json:"jsonrpc"`  // Always "2.0"
    ID      interface{} `json:"id,omitempty"`
    Result  interface{} `json:"result,omitempty"`
    Error   *JSONRPCError `json:"error,omitempty"`
}
```

**Standard Error Codes**:
- `-32700`: Parse error
- `-32600`: Invalid request
- `-32601`: Method not found
- `-32602`: Invalid params
- `-32603`: Internal error
- `-32000` to `-32099`: Server-specific errors

**Benefits**:
- Single point of unmarshaling
- Type-safe error handling
- Consistent response structure

### 3. Handler Layer (`handler.go`)

**Purpose**: Route method calls and support middleware

**Interface**:
```go
type Handler interface {
    Handle(ctx context.Context, method string, params json.RawMessage) (interface{}, error)
}
```

**MethodRouter**:
- Routes method calls to registered handlers
- Supports default handler for unregistered methods
- Can be extended with middleware

**Middleware Support**:
```go
type Middleware func(Handler) Handler

// Example: Trace middleware
func TraceMiddleware(traceWriter io.Writer) Middleware {
    return func(next Handler) Handler {
        return HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
            // Log request
            result, err := next.Handle(ctx, method, params)
            // Log response
            return result, err
        })
    }
}
```

**Benefits**:
- Pluggable handlers
- Middleware for cross-cutting concerns (logging, auth, metrics)
- Easy to test individual handlers

## Usage Example

### Basic Server Setup

```go
// Create transport
transport := NewDefaultTransport()

// Create method router
router := NewMethodRouter()

// Register handlers
router.RegisterFunc("initialize", handleInitialize)
router.RegisterFunc("tools/list", handleToolsList)
router.RegisterFunc("tools/call", handleToolCall)

// Apply middleware
handler := Chain(
    TraceMiddleware(traceWriter),
    AuthMiddleware(),
)(router)

// Serve loop
for {
    msg, format, err := transport.ReadMessage(reader)
    if err != nil {
        // handle error
    }
    
    req, err := UnmarshalRequest(msg)
    if err != nil {
        // handle error
    }
    
    result, err := handler.Handle(ctx, req.Method, req.Params)
    
    resp := NewResponse(req.ID)
    if err != nil {
        resp.Error = &JSONRPCError{...}
    } else {
        resp.Result = result
    }
    
    data, _ := resp.Marshal()
    transport.WriteMessage(writer, data, format)
}
```

### Custom Transport

```go
// Custom transport that always uses raw JSON
type RawJSONTransport struct{}

func (t *RawJSONTransport) ReadMessage(reader *bufio.Reader) ([]byte, *MessageFormat, error) {
    // Always read as raw JSON
}

func (t *RawJSONTransport) WriteMessage(writer *bufio.Writer, data []byte, format *MessageFormat) error {
    // Always write as raw JSON (ignore format)
}
```

### Custom Middleware

```go
// Metrics middleware
func MetricsMiddleware(metricsCollector MetricsCollector) Middleware {
    return func(next Handler) Handler {
        return HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
            start := time.Now()
            result, err := next.Handle(ctx, method, params)
            duration := time.Since(start)
            
            metricsCollector.Record(method, duration, err == nil)
            return result, err
        })
    }
}
```

## Protocol Scope

The MCP protocol is based on **JSON-RPC 2.0**, which is a well-defined standard:

- **Request Structure**: Fixed (jsonrpc, id, method, params)
- **Response Structure**: Fixed (jsonrpc, id, result/error)
- **Error Structure**: Fixed (code, message, data)
- **Method Names**: MCP-specific (e.g., "initialize", "tools/call", "resources/list")
- **Params/Results**: Method-specific (flexible JSON)

**What's Standardized**:
- Message envelope (JSON-RPC 2.0)
- Error codes (standard + MCP-specific)
- Transport format (raw JSON or Content-Length)

**What's Flexible**:
- Method-specific params/results
- Custom error data
- Transport implementations

## Benefits of This Architecture

1. **Separation of Concerns**: Each layer has a single responsibility
2. **Testability**: Each layer can be tested independently
3. **Extensibility**: Easy to add new transports, handlers, or middleware
4. **Maintainability**: Clear boundaries make code easier to understand
5. **Reusability**: Components can be reused across different MCP servers
6. **Type Safety**: Common shapes provide compile-time guarantees

## Migration Path

The current `Server` implementation can be incrementally refactored to use these abstractions:

1. **Phase 1**: Extract transport layer (✅ Done)
2. **Phase 2**: Extract protocol layer (✅ Done)
3. **Phase 3**: Extract handler layer (✅ Done)
4. **Phase 4**: Refactor `Server.Serve()` to use abstractions (In Progress)
5. **Phase 5**: Add middleware support to existing handlers
6. **Phase 6**: Document and stabilize API

## Related Documents

- [MCP Server Architecture](./mcp-server-architecture.md)
- [MCP CLI Bridge](./mcp-cli-bridge-v1.0.md)
- [MCP Scalability](./MCP_SCALABILITY.md)
- [MCP Output Routing](./MCP_OUTPUT_ROUTING.md)

