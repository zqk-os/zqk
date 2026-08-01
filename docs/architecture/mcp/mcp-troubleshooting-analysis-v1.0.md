# MCP Server Troubleshooting Analysis v1.0

**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Troubleshooting analysis for MCP server connection and operational issues

## Current Log Analysis

### Log Files Reviewed
1. **client-metrics.json** - Shows client connection metrics and tool call events
2. **mcp-trace.json** - Shows detailed MCP protocol trace logs
3. **mcp-trace.log** - Shows human-readable trace logs

### Observations from Logs

#### Normal Operation Period (2026-01-01 11:57:03 - 11:59:16)
- Server initialized successfully
- Client connected: `client_1767297423193630000_1767297423`
- 56 tools registered
- 2 tool calls executed:
  - `zqk_test_echo` (denied - not in exposed commands)
  - `zqk_system_status` (successful, 377ms)
- No shutdown messages in logs
- Last log entry: `[2026-01-01 11:59:16.454]` - tools/list call

#### Missing Information
- **No shutdown reason logged** - If server shut down, the reason should appear in `mcp-trace.log` with `[MCP_INFO] Shutdown triggered:` prefix
- **No error messages** - No `[MCP_ERROR]` entries indicating failures
- **No panic traces** - No stack traces or panic recovery logs

## Code Analysis: Potential Crash Points

### 1. Missing Panic Recovery in Async Handler
**Location**: `pkg/mcp/async_handler.go:81-84`

```go
go func() {
    result, err := a.handler.Handle(ctx, method, params)
    resultChan <- asyncResult{result: result, err: err}
}()
```

**Issue**: If `handler.Handle()` panics, the goroutine crashes and the server may hang waiting for a result that never arrives, or the panic propagates and crashes the server.

**Impact**: HIGH - Tool execution panics would crash the server

### 2. Missing Panic Recovery in Serve Loop
**Location**: `pkg/mcp/server.go:572-1066`

**Issue**: The main serve loop has no panic recovery. If any code in the loop panics, the entire server crashes.

**Impact**: CRITICAL - Any panic in message processing crashes the server

### 3. Missing Panic Recovery in Read Goroutine
**Location**: `pkg/mcp/server.go:594-597`

```go
go func() {
    msg, format, err := transport.ReadMessage(reader)
    readChan <- readResult{msg: msg, format: format, err: err}
}()
```

**Issue**: If `transport.ReadMessage()` panics, the goroutine crashes and the server may hang in the select statement.

**Impact**: HIGH - Read failures could hang the server

### 4. Error Handling Gaps
**Location**: Multiple locations in `pkg/mcp/server.go`

- Line 990-996: Marshal errors trigger shutdown
- Line 999-1005: Write errors trigger shutdown
- Line 816-819: Read errors trigger shutdown
- Line 856-867: Parse errors continue (good)

**Observation**: Some errors correctly trigger graceful shutdown, but panics bypass this mechanism entirely.

## Recommended Fixes

### Priority 1: Add Panic Recovery to Async Handler ✅ COMPLETED
Add panic recovery in the async handler goroutine to catch panics from tool execution.

**Implementation**: Added `defer recover()` in `pkg/mcp/async_handler.go:81-84` to catch panics from handler execution and convert them to errors.

### Priority 2: Add Panic Recovery to Serve Loop ✅ COMPLETED
Wrap handler calls in panic recovery to prevent server crashes.

**Implementation**: Added panic recovery around:
- Notification handler calls (`pkg/mcp/server.go:926-945`)
- Request handler calls (`pkg/mcp/server.go:960-975`)

Panics are caught, logged to trace file with stack traces, and converted to errors that are handled normally.

### Priority 3: Add Panic Recovery to Read Goroutine ✅ COMPLETED
Add panic recovery to the read goroutine to handle transport panics.

**Implementation**: Added `defer recover()` in `pkg/mcp/server.go:594-597` to catch panics from `transport.ReadMessage()`.

### Priority 4: Enhanced Logging ✅ COMPLETED
- Log panic details to trace file before recovery
- Include stack traces in panic logs
- Ensure panic logs are written even if client has disconnected

**Implementation**: All panic recovery blocks:
1. Log directly to trace file using `s.traceLogf()` (most reliable)
2. Include full stack traces using `debug.Stack()`
3. Attempt to log via MCP protocol (non-blocking)

## Changes Made

### Files Modified
1. **pkg/mcp/async_handler.go**
   - Added `runtime/debug` import
   - Added panic recovery in handler goroutine

2. **pkg/mcp/server.go**
   - Added `runtime/debug` import
   - Added panic recovery in read goroutine
   - Added panic recovery around notification handler calls
   - Added panic recovery around request handler calls

## Next Steps

1. ✅ Add panic recovery mechanisms to all identified locations - **COMPLETED**
2. ⏳ Test panic scenarios to ensure graceful handling
3. ⏳ Monitor logs after deployment to verify panic recovery works
4. ⏳ Consider adding metrics for panic occurrences

## Testing Recommendations

1. **Test panic in tool execution**: Create a test tool that panics to verify async handler recovery
2. **Test panic in handler**: Verify request/notification handler panic recovery
3. **Test panic in transport**: Verify read goroutine panic recovery

## Why MCP Feels Like It's Hanging (Single-Threaded Message Loop)

**Symptom:** Tool calls take 10–20+ seconds or appear to hang; cancelling in the client has no effect until the tool finally returns.

**Root cause:** The MCP server processes **one JSON-RPC message at a time**. Flow:

1. Serve loop reads a message (e.g. `tools/call`).
2. It calls `ProcessMessage` → handler → **executeCLICommandWithContext** → spawns `zqk` subprocess and **blocks in `cmd.Wait()`** until the CLI exits.
3. The server **does not read the next message** (e.g. `notifications/cancelled`) until the current request finishes and the response is written.
4. So when the client sends "cancel", the server is still blocked in `cmd.Wait()` and never sees the cancel. The CLI subprocess keeps running until it exits or the async operation timeout (90s) fires.

So MCP is a pass-through, but the **message loop is single-threaded**: the server cannot handle cancellation (or any other message) while a tool is running. The "hang" is the CLI taking 10–20s; the "cancel doesn't work" is because the server can't process the cancel until the tool returns.

**Why the CLI is slow:**

- **`system status`**: Runs a full `zqk system check` subprocess (up to 10s timeout) plus priority-plan fetch. With `--context mcp`, a fast path skips the system check so status returns in ~2–3s (see `getSystemHealthDataMCPFast` in `cmd/zqk/system/status_helpers.go`).
- **`object list` (e.g. priority_plan, backlog_item)**: Can take 6–16+ seconds due to storage scan, cold caches, or graph backend.

**Recommendations:**

1. **Keep CLI fast for MCP** – Use the MCP fast path for status; keep workflow tools (e.g. object list with `limit=1`) on the fastest code paths.
2. **Set client expectations** – Document that some tools may take 10–20s; use appropriate client timeouts (e.g. 30–60s) so they don't cancel before the tool returns.
3. **Future improvement** – To honor cancel: read messages in a dedicated goroutine, and when `notifications/cancelled` is received for a request ID, cancel that request's context so `CommandContext` kills the CLI subprocess. That requires associating in-flight requests with IDs and cancelling the right context when a cancel notification arrives.

## Resolved Issues (2026-01-16)

### Panic Recovery
- **Fix**: Panic recovery in `RegisterOnboardingPrompts()` with graceful fallback when onboarding prompts spec file is missing. Server now starts successfully.

### Short Write Errors
- **Fix**: `writeAll()` and `writeAllString()` helpers handle partial writes by retrying until all bytes are written. Transport layer handles partial writes correctly.

### Build Process
- **Fix**: Makefile corrected so `make build-all` builds `bin/zqk-mcp` correctly.

### JSON Parsing Errors (Interleaving)
- **Root cause**: Multiple code paths writing directly to transport writer (sendResponse, SendMessageToClient, SendLogMessage) causing interleaving.
- **Fix**: Route sendResponse through message queue; remove direct write fallbacks. All writes now go through message queue for proper serialization.

### Short Write on Client Disconnect
- **Fix**: `isBrokenPipeError()` helper detects client disconnects gracefully; server no longer shuts down on broken pipe, lets ServeLoop handle disconnect.

### Deadlock During Shutdown
- **Fix**: Shutdown check before `transportMu.RLock()` in `SendLogMessage`; shutdown check in welcome goroutine before `SendMessageToClient`. Prevents lock acquisition during shutdown.

### "Method not found: resources/subscribe"
- Only `events/subscribe` exists; client-side issue. Server correctly returns MethodNotFound.

## Related Documentation

- [MCP Trace Analysis](./mcp-trace-analysis-v1.0.md)
- [Multi-Agent Permission Issue Analysis](./multi-agent-permission-issue-analysis-v1.0.md)
- [MCP Multi-Agent Orchestration](../MCP_MULTI_AGENT_ORCHESTRATION.md)

