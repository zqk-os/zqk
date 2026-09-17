# MCP Event Subscription Quick Reference

**Last Verified:** 2026-08-31


**Purpose**: Quick reference for finding and managing event subscriptions

## Where Subscriptions Are Stored

### Server-Side Storage

**Location**: `pkg/mcp/event_emitter.go`

```go
type EventEmitter struct {
    subscribers map[string]EventSubscriber // subscription ID → subscriber
    typeIndex   map[EventType][]string      // event type → subscriber IDs
}
```

**Access Methods**:
- `GetSubscriberCount()` - Total active subscribers
- `GetSubscriberCountByType(eventType)` - Subscribers for specific event type
- `Subscribe(subscriber)` - Add subscription
- `Unsubscribe(subscriberID)` - Remove subscription

### Server Instance

**Location**: `pkg/mcp/server.go`

```go
type Server struct {
    eventEmitter *EventEmitter  // All subscriptions managed here
    // ...
}
```

## Client API Endpoints

### 1. Subscribe to Events

**Method**: `events/subscribe`  
**Handler**: `pkg/mcp/server_handlers.go:handleEventsSubscribe`

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "events/subscribe",
  "params": {
    "eventTypes": ["log.error", "tool.failed"],
    "clientId": "my-client"
  }
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "subscriptionId": "sub_1234567890",
    "eventTypes": ["log.error", "tool.failed"]
  }
}
```

### 2. Unsubscribe from Events

**Method**: `events/unsubscribe`  
**Handler**: `pkg/mcp/server_handlers.go:handleEventsUnsubscribe`

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

### 3. List Available Events

**Method**: `events/list`  
**Handler**: `pkg/mcp/server_handlers.go:handleEventsList`

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "events/list"
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "eventTypes": ["log.debug", "log.info", ...],
    "subscriberCount": 2
  }
}
```

## Finding Subscriptions in Code

### 1. Check Server's EventEmitter

```go
// In pkg/mcp/server.go
server.eventEmitter.GetSubscriberCount()
server.eventEmitter.GetSubscriberCountByType(EventTypeLogError)
```

### 2. Check Handler Registration

```go
// In pkg/mcp/server_handlers.go:setupHandlers()
router.RegisterFunc("events/subscribe", s.handleEventsSubscribe)
router.RegisterFunc("events/unsubscribe", s.handleEventsUnsubscribe)
router.RegisterFunc("events/list", s.handleEventsList)
```

### 3. Check Subscriber Implementation

```go
// In pkg/mcp/mcp_event_subscriber.go
// MCPEventSubscriber implements EventSubscriber interface
// Created in handleEventsSubscribe and stored in EventEmitter
```

## Subscription Lifecycle

1. **Creation**: Client calls `events/subscribe` → `handleEventsSubscribe` creates `MCPEventSubscriber` → stored in `EventEmitter.subscribers`

2. **Active**: Subscriber receives events via `SendEvent()` → events sent as JSON-RPC notifications

3. **Cleanup**: 
   - Manual: Client calls `events/unsubscribe`
   - Automatic: Idle timeout (5 minutes) or write failure → subscriber marked inactive → cleaned up by `EventEmitter.Emit()`

## Key Files

- **API Handlers**: `pkg/mcp/server_handlers.go` (lines 290-409)
- **Storage**: `pkg/mcp/event_emitter.go` (lines 65-210)
- **Subscriber**: `pkg/mcp/mcp_event_subscriber.go`
- **Documentation**: `docs/architecture/MCP_EVENT_SUBSCRIPTION.md`

## Testing

**Test File**: `pkg/mcp/event_subscription_test.go`

Tests cover:
- Subscription/unsubscription
- Event delivery
- Multiple subscribers
- Cleanup mechanisms
- Thread safety

