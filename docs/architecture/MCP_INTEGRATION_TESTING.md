# MCP Integration Testing Guide

**Last Verified:** 2026-08-31


**Purpose**: Guide for testing MCP event subscription system with actual agents/clients

## Overview

This guide provides instructions for testing the MCP event subscription system end-to-end, validating that:
1. MCP server can receive and process requests
2. Event subscriptions work correctly
3. Events are emitted and delivered to subscribers
4. Communication channel is functioning properly

## Test Tools

### Echo Tool

A simple test tool (`echo`) has been added to validate MCP communication:

**Tool Name**: `echo`  
**Description**: Echo a message back - useful for testing MCP communication  
**Parameters**:
- `message` (string, required): Message to echo back

**Example Request**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "echo",
    "arguments": {
      "message": "Hello, MCP!"
    }
  }
}
```

**Example Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"echo\":\"Hello, MCP!\",\"status\":\"success\",\"message\":\"Echo: Hello, MCP!\"}"
      }
    ]
  }
}
```

## Test Scripts

### 1. Python Test Script (Recommended)

**Location**: `scripts/test_mcp_echo.py`

**Features**:
- Interactive testing with real-time output
- Tests initialization, subscription, echo tool, and event reception
- Handles notifications asynchronously
- Comprehensive test coverage

**Usage**:
```bash
# Build the binary first
go build -o zqk ./cmd/zqk

# Run the test script
python3 scripts/test_mcp_echo.py
```

**What it tests**:
1. ✅ Initialize MCP server
2. ✅ Subscribe to events (tool.started, tool.completed, tool.failed)
3. ✅ List available events
4. ✅ List available tools (verify echo tool is registered)
5. ✅ Call echo tool
6. ✅ Receive event notifications (if emitted)

### 2. Bash Test Script

**Location**: `scripts/test_mcp_events.sh`

**Features**:
- Simple sequential test execution
- Good for CI/CD pipelines
- Validates basic functionality

**Usage**:
```bash
# Build the binary first
go build -o zqk ./cmd/zqk

# Run the test script
./scripts/test_mcp_events.sh
```

## Manual Testing with Cursor

### Step 1: Start MCP Server

The MCP server is configured in Cursor's settings. Ensure the server is running and connected.

### Step 2: Subscribe to Events

Use Cursor's MCP interface or send a request:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "events/subscribe",
  "params": {
    "eventTypes": ["tool.started", "tool.completed"],
    "clientId": "cursor-client"
  }
}
```

### Step 3: Call Echo Tool

Request the echo tool:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "echo",
    "arguments": {
      "message": "Hello from Cursor!"
    }
  }
}
```

### Step 4: Verify Events

Check for event notifications:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/event",
  "params": {
    "event": {
      "type": "tool.started",
      "message": "Tool execution started: echo",
      "fields": {
        "tool": "echo"
      }
    }
  }
}
```

## Expected Behavior

### Successful Test Flow

1. **Initialize**: Server responds with protocol version and capabilities
2. **Subscribe**: Server returns a subscription ID
3. **List Events**: Server returns available event types and subscriber count
4. **List Tools**: Server returns available tools including `echo`
5. **Call Echo**: Server executes tool and returns echo result
6. **Receive Events**: Client receives `tool.started` and `tool.completed` notifications

### Event Notifications

When the echo tool is called, you should receive:

1. **tool.started** event:
```json
{
  "jsonrpc": "2.0",
  "method": "notifications/event",
  "params": {
    "event": {
      "type": "tool.started",
      "timestamp": "2025-01-XXT...",
      "message": "Tool execution started: echo",
      "fields": {
        "tool": "echo"
      },
      "severity": "info"
    }
  }
}
```

2. **tool.completed** event:
```json
{
  "jsonrpc": "2.0",
  "method": "notifications/event",
  "params": {
    "event": {
      "type": "tool.completed",
      "timestamp": "2025-01-XXT...",
      "message": "Tool execution completed: echo",
      "fields": {
        "tool": "echo"
      },
      "severity": "info"
    }
  }
}
```

## Troubleshooting

### Echo Tool Not Found

**Problem**: `tools/list` doesn't show the echo tool

**Solution**: Ensure `RegisterAllTools()` is called during server initialization. Check `cmd/zqk/mcp/mcp.go`.

### Events Not Received

**Problem**: Subscribed but not receiving event notifications

**Possible Causes**:
1. Events are emitted but notifications aren't being read
2. Subscription ID mismatch
3. Transport layer issue (stdio vs Content-Length)

**Solution**: 
- Check trace log: `.zqk/mcp/logs/mcp-trace.log`
- Verify subscription ID matches
- Ensure client is reading from stdout continuously

### Connection Issues

**Problem**: Server doesn't respond

**Solution**:
- Verify binary is built: `go build -o zqk ./cmd/zqk`
- Check MCP server configuration in Cursor
- Review trace logs for errors

## Next Steps

After validating basic communication:

1. **Test with Multiple Subscribers**: Subscribe from multiple clients
2. **Test Event Filtering**: Subscribe to specific event types only
3. **Test Permission Events**: Trigger permission checks and verify events
4. **Test Logger Events**: Verify log messages are converted to events
5. **Test Idle Timeout**: Verify subscribers are cleaned up after inactivity

## Related Documentation

- [MCP Event Subscription System](./MCP_EVENT_SUBSCRIPTION.md)
- [MCP Event Subscription Quick Reference](./MCP_EVENT_SUBSCRIPTION_QUICK_REF.md)
- [MCP Multi-Agent Orchestration](./MCP_MULTI_AGENT_ORCHESTRATION.md)

