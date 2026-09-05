# MCP Event Subscription System

**Last Verified:** 2026-08-31


**Status**: Active  
**Version**: 1.0  
**Date**: 2025-01-XX

## Overview

The MCP Event Subscription System provides a pub/sub mechanism for MCP clients to receive real-time notifications about system events. This enables reactive workflows, better observability, and prompt-based interactions where clients can subscribe to relevant events and respond accordingly.

## Architecture

### Components

1. **EventEmitter** (`pkg/mcp/event_emitter.go`):
   - Manages event subscriptions and emission
   - Thread-safe pub/sub pattern
   - Automatic cleanup of inactive subscribers
   - Type-based event filtering

2. **MCPEventSubscriber** (`pkg/mcp/mcp_event_subscriber.go`):
   - Implements `EventSubscriber` interface
   - Sends events as JSON-RPC notifications to MCP clients
   - Handles connection failures and idle timeouts
   - Converts events to MCP notification format

3. **LoggerEventAdapter** (`pkg/mcp/logger_event_adapter.go`):
   - Bridges logging system to event emission
   - Automatically converts log messages to events
   - Can be enabled/disabled per logger instance

4. **MCP Methods** (`pkg/mcp/server_handlers.go`):
   - `events/subscribe` - Subscribe to event types
   - `events/unsubscribe` - Unsubscribe from events
   - `events/list` - List available event types and subscriber count

## Event Types

### Log Events
- `log.debug` - Debug-level log messages
- `log.info` - Info-level log messages
- `log.warn` - Warning-level log messages
- `log.error` - Error-level log messages

### Permission Events
- `permission.denied` - Access denied to a resource
- `permission.granted` - Access granted to a resource
- `user.deactivated` - User account deactivated
- `user.activated` - User account activated

### Tool Execution Events
- `tool.started` - Tool execution started
- `tool.completed` - Tool execution completed successfully
- `tool.failed` - Tool execution failed

### System Events
- `system.warning` - System warning
- `system.error` - System error

### Prompt Events
- `prompt.available` - A prompt is available for the client
- `action.required` - An action is required from the client

## Usage

### Subscribing to Events

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "events/subscribe",
  "params": {
    "eventTypes": ["log.error", "permission.denied", "tool.failed"],
    "clientId": "my-client"
  }
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "subscriptionId": "sub_1234567890",
    "eventTypes": ["log.error", "permission.denied", "tool.failed"]
  }
}
```

### Receiving Events

Events are delivered as JSON-RPC notifications:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/event",
  "params": {
    "event": {
      "type": "permission.denied",
      "timestamp": "2025-01-XXT12:00:00Z",
      "message": "Permission denied for field access",
      "fields": {
        "user": "account:user",
        "field": "email",
        "kind": "account",
        "operation": "read"
      },
      "severity": "warn"
    }
  }
}
```

### Unsubscribing

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "events/unsubscribe",
  "params": {
    "subscriptionId": "sub_1234567890"
  }
}
```

### Listing Available Events

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "events/list"
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "eventTypes": [
      "log.debug",
      "log.info",
      "log.warn",
      "log.error",
      "permission.denied",
      "permission.granted",
      "user.deactivated",
      "user.activated",
      "tool.started",
      "tool.completed",
      "tool.failed",
      "system.warning",
      "system.error",
      "prompt.available",
      "action.required"
    ],
    "subscriberCount": 2
  }
}
```

### IDE Notification & Local Subscriber Stub

If Cursor or a similar IDE client lacks a native hook to resume from idle via MCP `notifications/event`, you can use the local subscriber stub. When `zqk feed steer` is executed and no active IDE MCP subscribers are present (i.e. `subscriberCount == 0`), the CLI fails loud with `peer_wake_unrepaired: true` and a `mcp_no_subscriber` reason.

A Python-based fallback script is provided at `.zqk/mcp/subscriber-stub.py` (which mirrors `scripts/mesh/mcp-tpm-subscriber.py`).
This stub daemonizes an active connection to `127.0.0.1:8443`, subscribes to `action.required` events, and directly executes the `tpm-correspondence-tick.sh` (or a `feed pending` ring) when the IDE is otherwise unresumable.

## Integration Points

### Logging System

The logging system automatically emits events when using `LoggerEventAdapter`:

```go
logger := logging.GetLoggerFromProfile("system")
eventLogger := mcp.NewLoggerEventAdapter(logger, server.eventEmitter)
permissionCache.SetLogger(eventLogger)
```

### Permission System

Permission checks automatically emit events:

```go
specAccessControl.SetEventEmitter(server.eventEmitter)
```

### Tool Execution

Tool execution automatically emits start/completion events in `handleToolCallWithContext`.

## Implementation Details

### Transport Integration

Subscribers use the server's transport layer to send notifications. The transport context is stored in the server and accessed by subscribers via a closure:

```go
writeFunc := func(data []byte) error {
    s.transportMu.RLock()
    writer := s.transportWriter
    format := s.transportFormat
    s.transportMu.RUnlock()
    
    transport := NewDefaultTransport()
    return transport.WriteMessage(writer, data, format)
}
```

### Idle Timeout

Subscribers automatically become inactive after 5 minutes of inactivity. This prevents resource leaks from disconnected clients.

### Event Filtering

Subscribers can filter events by type. If no event types are specified, the subscriber receives all events.

## Future Enhancements

1. **Persistent Subscriptions**: Store subscriptions across server restarts
2. **Event Batching**: Batch multiple events into single notifications
3. **Event History**: Provide access to recent event history
4. **Priority Queuing**: Prioritize critical events
5. **Rate Limiting**: Limit event emission rate per subscriber
6. **Event Transformation**: Allow clients to transform events before delivery

## Security Considerations

- Subscribers only receive events they're authorized to see
- Permission events respect the same access control as the underlying operations
- Event emission respects the logging suppression during active serving

## Related Documentation

- [MCP Server Architecture](./MCP_ABSTRACTION_LAYER.md)
- [MCP Security Enforcement](./MCP_SECURITY_ENFORCEMENT.md)
- [MCP Spec Access Control](./MCP_SPEC_ACCESS_CONTROL.md)

