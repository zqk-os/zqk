# MCP Protocol Interface

**Last Verified:** 2026-08-31


## Overview

This package provides explicit, type-safe interfaces for the MCP (Model Context Protocol) specification. The interfaces are aligned with the MCP protocol and provide both synchronous and asynchronous operations.

## Core Interface: `MCPServer`

The `MCPServer` interface explicitly implements all MCP protocol methods:

### Lifecycle Methods
- `Initialize(ctx, params)` - Initialize the MCP server
- `NotifyInitialized(ctx, params)` - Notify server client is ready
- `Shutdown(ctx)` - Gracefully shutdown the server

### Tools Methods
- `ListTools(ctx)` - List all available tools
- `CallTool(ctx, params)` - Execute a tool

### Resources Methods
- `ListResources(ctx, params)` - List all available resources
- `GetResource(ctx, params)` - Retrieve a specific resource

### Prompts Methods
- `ListPrompts(ctx)` - List all available prompts
- `GetPrompt(ctx, params)` - Retrieve a prompt template

### Roots Methods
- `ListRoots(ctx)` - List root resource URIs

### Notification Methods (Server → Client)
- `SendLogMessage(ctx, level, message, fields)` - Send log message
- `SendEvent(ctx, event)` - Send event notification
- `SendMessage(ctx, message, type, priority)` - Send message notification
- `NotifyCancelled(ctx, params)` - Notify request cancellation

## Async Extension: `MCPAsyncServer`

The `MCPAsyncServer` interface extends `MCPServer` with asynchronous operations:

### Async Tool Operations
- `CallToolAsync(ctx, params)` - Execute tool asynchronously (returns channel)
- `ListToolsAsync(ctx)` - List tools asynchronously

### Async Resource Operations
- `GetResourceAsync(ctx, params)` - Get resource asynchronously
- `ListResourcesAsync(ctx, params)` - List resources asynchronously

### Async Prompt Operations
- `GetPromptAsync(ctx, params)` - Get prompt asynchronously
- `ListPromptsAsync(ctx)` - List prompts asynchronously

### Batch Operations
- `CallToolsBatch(ctx, params[])` - Execute multiple tools in parallel
- `GetResourcesBatch(ctx, params[])` - Get multiple resources in parallel

## Usage

### Creating an Adapter

The `MCPServerAdapter` wraps the existing `Server` to implement the interfaces:

```go
server := mcp.NewServer()
adapter := mcp.NewMCPServerAdapter(server)

// Use as MCPServer
var mcpServer mcp.MCPServer = adapter
result, err := mcpServer.Initialize(ctx, &mcp.InitializeParams{...})

// Use as MCPAsyncServer
var asyncServer mcp.MCPAsyncServer = adapter
resultChan := asyncServer.CallToolAsync(ctx, &mcp.ToolCallParams{...})
result := <-resultChan
```

### Synchronous Operations

```go
// Initialize
initResult, err := adapter.Initialize(ctx, &mcp.InitializeParams{
    ProtocolVersion: "2025-06-18",
    Capabilities: map[string]any{...},
})

// Call tool
toolResult, err := adapter.CallTool(ctx, &mcp.ToolCallParams{
    Name: "my_tool",
    Arguments: map[string]any{...},
})

// List resources
resources, err := adapter.ListResources(ctx, &mcp.ResourcesListParams{...})
```

### Asynchronous Operations

```go
// Async tool call
resultChan := adapter.CallToolAsync(ctx, &mcp.ToolCallParams{...})
select {
case result := <-resultChan:
    if result.Error != nil {
        // Handle error
    } else {
        // Use result.Result
    }
case <-ctx.Done():
    // Handle cancellation
}

// Batch operations
params := []*mcp.ToolCallParams{...}
resultChan := adapter.CallToolsBatch(ctx, params)
for result := range resultChan {
    // Process result.Index, result.Result, result.Error
}
```

## Protocol Alignment

All methods are explicitly aligned with the MCP specification:

- **Method Names**: Match MCP protocol method names exactly
- **Parameter Types**: Use explicit types matching MCP spec
- **Response Types**: Use explicit result types matching MCP spec
- **Error Handling**: Uses JSON-RPC 2.0 error codes
- **Context Support**: All methods accept `context.Context` for cancellation/timeout

## Async Extensions

The async extensions provide:

1. **Non-blocking Operations**: Operations return channels instead of blocking
2. **Parallel Execution**: Batch operations execute in parallel
3. **Cancellation Support**: Context cancellation works with async operations
4. **Error Handling**: Errors are returned via result channels

## Benefits

1. **Type Safety**: Explicit types prevent runtime errors
2. **Protocol Compliance**: Methods match MCP spec exactly
3. **Async Support**: Extensions provide async where protocol doesn't
4. **Testability**: Interfaces enable easy mocking and testing
5. **Documentation**: Interface serves as protocol documentation

## Implementation Notes

- The `MCPServerAdapter` wraps the existing `Server` to avoid breaking changes
- Existing `Server` methods remain unchanged
- Interface methods delegate to existing handlers
- Async operations use goroutines and channels
- Batch operations use `ParallelExecutor` for concurrent execution
