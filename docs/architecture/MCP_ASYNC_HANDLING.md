# MCP Server Async Handling for Variable-Length Operations

**Last Verified:** 2026-08-31


**Status**: Implemented  
**Date**: 2025-12-30  
**Purpose**: Handle variable-length operations and prevent system starvation with multiple agents

## Overview

The MCP server now supports **async processing with synchronous responses** to handle variable-length operation durations while maintaining protocol compliance. This prevents system starvation when multiple agents use the server concurrently.

## Problem Statement

### Original Issues

1. **Blocking Operations**: Long-running CLI commands blocked the entire server
2. **No Concurrency Control**: Multiple agents could overwhelm the server
3. **No Timeouts**: Operations could run indefinitely
4. **No Cancellation**: Operations couldn't be cancelled mid-execution
5. **System Starvation**: One slow operation blocked all other requests

### MCP Protocol Constraints

- **Stdio MCP**: Single sequential stream (stdin/stdout)
- **Response Ordering**: Responses must match request IDs and arrive in order
- **Protocol Compliance**: Cannot reorder responses (breaks JSON-RPC)

## Solution: Async Processing with Synchronous Responses

### Architecture

```
Request → AsyncHandler → Goroutine → Wait for Result → Response
         ↓
    Concurrency Limit
    Timeout Protection
    Cancellation Support
```

**Key Principle**: Process asynchronously, respond synchronously

- Operations run in goroutines (non-blocking)
- Server loop waits for completion (maintains ordering)
- Responses sent in request order (protocol compliant)

## Implementation

### 1. AsyncHandler (`async_handler.go`)

Wraps handlers to execute them asynchronously while maintaining synchronous response ordering.

**Features**:
- **Concurrency Limit**: Maximum concurrent operations (default: 10)
- **Timeout Protection**: Maximum operation duration (default: 5 minutes)
- **Context Cancellation**: Operations can be cancelled
- **Capacity Checking**: Rejects requests when at capacity

**Configuration**:
```go
config := AsyncHandlerConfig{
    MaxConcurrent: 10,              // Limit concurrent operations
    Timeout: 5 * time.Minute,      // Maximum operation duration
}
```

### 2. OperationTracker (`async_handler.go`)

Tracks active operations for monitoring and cancellation.

**Features**:
- Track all active operations
- Cancel specific operations by ID
- Monitor operation count
- Get operation details (method, start time, etc.)

### 3. Context Propagation

Context flows through the entire request handling chain:

```
Server.Serve() 
  → AsyncHandler.Handle() 
    → MethodRouter.Handle() 
      → Server.handleToolsCall() 
        → Server.handleToolCallWithContext() 
          → CLI Bridge (with context)
            → exec.CommandContext() (cancellable)
```

**Benefits**:
- Operations can be cancelled at any level
- Timeouts enforced at handler level
- CLI commands respect context cancellation

## Configuration

### Default Settings

```go
AsyncHandlerConfig{
    MaxConcurrent: 10,              // 10 concurrent operations max
    Timeout: 5 * time.Minute,       // 5 minute timeout
}
```

### Customization

```go
server := NewServer()
server.SetAsyncConfig(AsyncHandlerConfig{
    MaxConcurrent: 20,              // Allow more concurrency
    Timeout: 10 * time.Minute,      // Longer timeout for complex operations
})
```

## Behavior

### Normal Operation

1. Request arrives → AsyncHandler checks capacity
2. If capacity available → Start operation in goroutine
3. Server loop waits for result (non-blocking)
4. Operation completes → Response sent in order
5. Next request processed

### At Capacity

1. Request arrives → AsyncHandler checks capacity
2. If at capacity → Return error immediately:
   ```json
   {
     "error": {
       "code": -32000,
       "message": "Server at capacity (10 concurrent operations)",
       "data": {"max_concurrent": 10}
     }
   }
   ```
3. Client can retry or wait

### Timeout

1. Operation starts → Context with timeout created
2. Operation exceeds timeout → Context cancelled
3. Error returned:
   ```json
   {
     "error": {
       "code": -32000,
       "message": "Operation timed out after 5m0s",
       "data": {"timeout": "5m0s"}
     }
   }
   ```

### Cancellation

1. Client sends `notifications/cancelled` → OperationTracker cancels operation
2. Context cancelled → Operation stops
3. Error returned:
   ```json
   {
     "error": {
       "code": -32000,
       "message": "Operation was cancelled",
       "data": {"reason": "context canceled"}
     }
   }
   ```

## Multiple Agents Support

### Concurrency Limits

- **Default**: 10 concurrent operations
- **Per-Agent**: No explicit per-agent limit (shared pool)
- **Fairness**: First-come-first-served

### Preventing Starvation

1. **Capacity Limits**: Reject requests when at capacity
2. **Timeouts**: Operations can't run indefinitely
3. **Cancellation**: Long operations can be cancelled
4. **Non-Blocking**: Server loop never blocks on operations

### Example Scenario

**Multiple Agents**:
- Agent A: 3 long-running operations
- Agent B: 5 quick operations
- Agent C: 4 medium operations

**Server Behavior**:
- Accepts first 10 operations (mixed from all agents)
- Rejects 11th operation with capacity error
- Operations complete → Capacity freed → Next request accepted
- No agent starves (fair scheduling)

## MCP Protocol Compliance

### Response Ordering

✅ **Maintained**: Responses sent in request order
- Operations may complete out of order
- Responses queued and sent in request order
- JSON-RPC ID matching preserved

### Protocol Requirements

✅ **Compliant**: All MCP stdio requirements met
- Sequential request processing
- Ordered response delivery
- Proper error handling
- Notification support

## Benefits

1. **Variable-Length Operations**: Handle operations of any duration
2. **No Blocking**: Server never blocks on long operations
3. **Concurrency Control**: Prevent system overload
4. **Timeout Protection**: Operations can't run indefinitely
5. **Cancellation Support**: Operations can be cancelled
6. **Multiple Agents**: Support multiple concurrent clients
7. **Protocol Compliant**: Maintains MCP stdio requirements

## Limitations

1. **Stdio Constraint**: Still limited by stdio transport (single stream)
2. **No True Parallelism**: Responses must be sent sequentially
3. **Shared Capacity**: All agents share the same concurrency pool
4. **No Priority**: No request prioritization (FIFO)

## Future Enhancements

1. **Per-Agent Limits**: Separate concurrency limits per agent
2. **Priority Queue**: Prioritize certain operations
3. **Progress Notifications**: Send progress updates (if MCP supports)
4. **Adaptive Timeouts**: Adjust timeouts based on operation type
5. **Metrics**: Track operation durations and success rates

## Related Documents

- [ADR-MCP-SCALABILITY.md](./ADR-MCP-SCALABILITY.md) - Scalability decision record
- [MCP_SCALABILITY.md](./MCP_SCALABILITY.md) - Quick reference
- [MCP_ABSTRACTION_LAYER.md](./MCP_ABSTRACTION_LAYER.md) - Architecture overview
- [MCP_SERVER_REFACTOR.md](./MCP_SERVER_REFACTOR.md) - Server refactoring details

