# MCP Server Scalability Considerations

> **Note**: This document provides a quick reference. For detailed decision rationale and evaluation criteria, see [ADR-MCP-SCALABILITY.md](./ADR-MCP-SCALABILITY.md).

## Current Architecture: Synchronous Request-Response

The MCP server uses a **synchronous request-response pattern** over stdio:

```
Request → Process → Response → Flush → Next Request
```

This is **required** for stdio-based MCP because:
1. **Single stream**: stdin/stdout is a single sequential stream
2. **JSON-RPC ordering**: Responses must match request IDs and arrive in order
3. **Protocol compliance**: MCP over stdio expects sequential, ordered responses

## Why Async Flushing Doesn't Work for Stdio MCP

**Problem**: If we flush asynchronously, responses could arrive out of order:
```
Request 1 → Process (slow) → Response 1 (queued)
Request 2 → Process (fast) → Response 2 (flushed first) ❌ BREAKS PROTOCOL
```

**Result**: Cursor would receive Response 2 before Response 1, causing JSON-RPC ID mismatches and protocol violations.

## Current Optimizations

The current implementation already provides buffering benefits:

1. **`bufio.Writer`**: Automatically buffers writes, reducing system calls
2. **Immediate flush after response**: Ensures Cursor receives responses promptly
3. **Synchronous processing**: Guarantees response ordering

## Decision Status

**Status**: Deferred - See [ADR-MCP-SCALABILITY.md](./ADR-MCP-SCALABILITY.md) for full decision record.

**Current Approach**: Keep synchronous (Option 1) - appropriate for current workload and protocol requirements.

**When to Revisit**: See ADR for performance, strategic, and tactical indicators.

## Quick Reference

- **Async flushing**: ❌ Not viable - breaks protocol compliance
- **Async processing**: ✅ Viable - can process long operations in goroutines while maintaining synchronous responses
- **Transport upgrade**: 🔮 Future - requires Cursor support for SSE/WebSocket

For detailed analysis, trade-offs, and evaluation criteria, see [ADR-MCP-SCALABILITY.md](./ADR-MCP-SCALABILITY.md).

