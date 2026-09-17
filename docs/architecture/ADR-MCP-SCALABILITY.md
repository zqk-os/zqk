# Architecture Decision Record: MCP Server Scalability

**Last Verified:** 2026-08-31


**ADR ID**: ADR-002  
**Status**: Active (Deferred Decision)  
**Date**: 2025-12-30  
**Decision Makers**: Architecture Team  
**Revisit Date**: 2026-12-30 (1 year from creation)  
**Decision Object**: [`.zqk/process/decisions/ADR-002.yaml`](../decisions/ADR-002.yaml)  
**Related**: [MCP_SCALABILITY.md](./MCP_SCALABILITY.md) - Quick reference  
**Context**: Evaluating scalability options for MCP server response handling

## Context

The MCP server currently uses a synchronous request-response pattern over stdio. A question was raised about implementing async flushing with a backlog queue for better scalability.

## Current Architecture

**Pattern**: Synchronous request-response over stdio
```
Request → Process → Response → Flush → Next Request
```

**Implementation**:
- Uses `bufio.Writer` for efficient buffering
- Flushes immediately after each response
- Processes requests sequentially in a single loop
- Maintains strict response ordering

## Constraints

### Protocol Constraints (Stdio MCP)
1. **Single sequential stream**: stdin/stdout is one bidirectional stream
2. **JSON-RPC ordering requirement**: Responses must match request IDs and arrive in order
3. **Protocol compliance**: MCP over stdio expects sequential, ordered responses
4. **No concurrent requests**: Stdio MCP doesn't support pipelining

### Technical Constraints
1. **Cursor integration**: Uses stdio-based MCP (not SSE/WebSocket)
2. **Response matching**: Each response must match its request ID
3. **Error handling**: Protocol errors must be sent immediately

## Options Considered

### Option 1: Keep Synchronous (Current)
**Approach**: Maintain current synchronous request-response pattern

**Pros**:
- ✅ Protocol compliant (required for stdio MCP)
- ✅ Simple and reliable
- ✅ `bufio.Writer` already provides buffering
- ✅ No ordering issues
- ✅ Works well for typical MCP workloads

**Cons**:
- ❌ Blocks on long-running operations
- ❌ No concurrent request processing
- ❌ Limited scalability for high-throughput scenarios

**Trade-offs**:
- **Reliability vs Performance**: Prioritizes correctness over throughput
- **Simplicity vs Optimization**: Easier to maintain, but less flexible

### Option 2: Async Flushing with Backlog Queue
**Approach**: Queue responses and flush asynchronously

**Pros**:
- ✅ Could improve throughput for multiple rapid requests
- ✅ Allows batching of flushes

**Cons**:
- ❌ **BREAKS PROTOCOL**: Responses could arrive out of order
- ❌ JSON-RPC ID mismatches
- ❌ Cursor would receive wrong responses
- ❌ Complex error handling
- ❌ Not compatible with stdio MCP requirements

**Trade-offs**:
- **Performance vs Correctness**: Would break protocol compliance
- **Not viable for stdio MCP**

### Option 3: Async Processing, Synchronous Responses
**Approach**: Process long operations in goroutines, but respond synchronously

**Pros**:
- ✅ Can handle long-running operations without blocking
- ✅ Maintains protocol compliance
- ✅ Can use MCP progress notifications (if supported)
- ✅ Better resource utilization

**Cons**:
- ❌ Still blocks on response writing
- ❌ Requires careful request ID tracking
- ❌ More complex error handling
- ❌ Need to manage goroutine lifecycle

**Trade-offs**:
- **Complexity vs Performance**: Adds complexity but improves long-operation handling
- **Viable for stdio MCP**

### Option 4: Switch Transport (Future)
**Approach**: Use SSE or WebSocket instead of stdio

**Pros**:
- ✅ True async/parallel processing
- ✅ Better scalability
- ✅ Can handle concurrent requests
- ✅ Standard async patterns

**Cons**:
- ❌ Requires Cursor to support alternative transport
- ❌ Significant architecture change
- ❌ May not be compatible with current Cursor MCP implementation
- ❌ More complex deployment

**Trade-offs**:
- **Performance vs Compatibility**: Better performance but may break Cursor integration
- **Future consideration**

## Decision

**Defer decision** - Current synchronous approach is appropriate for:
1. Current workload characteristics (low-to-moderate request rate)
2. Protocol compliance requirements (stdio MCP)
3. Simplicity and reliability needs
4. Cursor integration constraints

## When to Revisit

Revisit this decision when one or more of the following conditions are met:

### Performance Indicators
- [ ] Request processing time becomes a bottleneck
- [ ] Response latency exceeds acceptable thresholds (>1s for typical operations)
- [ ] Throughput requirements exceed current capacity
- [ ] Long-running operations (>5s) become common

### Strategic Indicators
- [ ] Cursor adds support for SSE/WebSocket MCP transport
- [ ] Alternative MCP clients with different transport requirements emerge
- [ ] Scale requirements change significantly (e.g., multiple concurrent users)
- [ ] New use cases require concurrent request handling

### Tactical Indicators
- [ ] Current architecture becomes a maintenance burden
- [ ] Performance optimizations in other areas are exhausted
- [ ] Team capacity allows for architectural refactoring
- [ ] Clear ROI for async processing is demonstrated

## Evaluation Criteria (When Revisiting)

When revisiting, evaluate against:

1. **Protocol Compliance**: Must maintain MCP protocol correctness
2. **Cursor Compatibility**: Must work with Cursor's MCP implementation
3. **Complexity vs Benefit**: Is the added complexity worth the performance gain?
4. **Maintenance Burden**: Can the team maintain the more complex solution?
5. **User Experience**: Will users notice the improvement?
6. **Strategic Alignment**: Does it align with long-term architecture goals?

## Implementation Notes (If Revisiting)

### If Choosing Option 3 (Async Processing)
- Use goroutines for long-running CLI commands
- Maintain request ID tracking
- Implement proper cancellation/timeout handling
- Consider MCP progress notifications for user feedback
- Keep response writing synchronous

### If Choosing Option 4 (Transport Switch)
- Evaluate Cursor compatibility first
- Design transport abstraction layer
- Plan migration strategy
- Consider backward compatibility
- Test thoroughly with Cursor

## Related Documents

- [MCP_SCALABILITY.md](./MCP_SCALABILITY.md) - Quick reference guide
- [MCP_OUTPUT_ROUTING.md](./MCP_OUTPUT_ROUTING.md) - Output routing strategy
- [POL-CODE-007.yaml](../policies/POL-CODE-007.yaml) - Logging architecture policy
- [ARCHITECTURE_DECISION_RECORDS.md](./ARCHITECTURE_DECISION_RECORDS.md) - ADR format guide
- [DECISION_LIFECYCLE.md](./DECISION_LIFECYCLE.md) - Decision lifecycle process
- MCP server changelog: see docs/architecture/mcp/ or git history

## Implementation Notes

- Current `bufio.Writer` with immediate flush is the correct approach for stdio MCP
- Async flushing would break protocol compliance
- Future scalability should focus on async processing (not async flushing)
- Transport upgrade (SSE/WebSocket) is a strategic decision requiring Cursor support

## Lifecycle Tracking

This ADR should be:
- **Revisited** when any of the "When to Revisit" indicators are met
- **Updated** if strategic or tactical context changes significantly
- **Reviewed** as part of quarterly architecture reviews
- **Archived** if superseded by a new ADR or if constraints fundamentally change

**Decision Object**: This ADR is tracked as a system object at [`.zqk/process/decisions/ADR-002.yaml`](../decisions/ADR-002.yaml)

To query this decision:
```bash
# Get the decision object
zqk object get ADR-002

# List all ADRs
zqk object list decision --filter "title~ADR-"
```

