# MCP Event Format as Output Format

**Status**: Design Proposal  
**Version**: 1.0  
**Date**: 2025-01-XX

## Overview

The MCP event subscription system can be conceptualized as another output format option, similar to `--format json` or `--format yaml`. This unifies the output system and makes events a first-class format option.

## Current Format System

The CLI currently supports:
- `--format table` - Human-readable table output
- `--format json` - Single JSON response
- `--format yaml` - Single YAML response

## Proposed Extension

Add streaming formats:
- `--format json-rpc` - Stream JSON-RPC notifications (events)
- `--format stream` - Alias for json-rpc

## Architecture

### Unified Format System

```
Command Execution
    ↓
Format Selection (--format flag)
    ↓
┌─────────────────────────────────────┐
│  Format Handler                      │
├─────────────────────────────────────┤
│  • table → TableFormatter            │
│  • json → JSONFormatter (single)     │
│  • yaml → YAMLFormatter (single)     │
│  • json-rpc → JSONRPCFormatter       │
│    (streaming events)                 │
└─────────────────────────────────────┘
    ↓
Output (stdout/stderr or events)
```

### Format Handlers

1. **TableFormatter**: Formats data as human-readable tables
2. **JSONFormatter**: Formats data as single JSON object
3. **YAMLFormatter**: Formats data as single YAML document
4. **JSONRPCFormatter**: Streams data as JSON-RPC notifications (events)

### Integration Points

#### CLI Commands

When a command is executed with `--format json-rpc`:

```go
format := cli.GetFormat(cmd)
if format == cli.FormatJSONRPC {
    // Instead of returning single result, subscribe to events
    // and stream them as JSON-RPC notifications
    return streamEventsAsJSONRPC(cmd, result)
}
```

#### MCP Server

In MCP mode, `--format json-rpc` automatically:
1. Subscribes to relevant events for the command
2. Streams events as JSON-RPC notifications
3. Returns final result when command completes

## Benefits

1. **Unified API**: Events are just another format option
2. **Consistent Interface**: Same `--format` flag for all output types
3. **Backward Compatible**: Existing formats unchanged
4. **Discoverable**: Format options visible in `--help`
5. **Flexible**: Can combine with other flags (e.g., `--format json-rpc --verbose`)

## Implementation Strategy

### Phase 1: Format Registration

Extend `OutputFormat` enum to include streaming formats:

```go
const (
    FormatTable   OutputFormat = "table"
    FormatJSON    OutputFormat = "json"
    FormatYAML    OutputFormat = "yaml"
    FormatJSONRPC OutputFormat = "json-rpc"
    FormatStream  OutputFormat = "stream" // Alias
)
```

### Phase 2: Format Handler Interface

Create a `FormatHandler` interface:

```go
type FormatHandler interface {
    Format(data interface{}) ([]byte, error)
    IsStreaming() bool
    Stream(ctx context.Context, data interface{}, writer io.Writer) error
}
```

### Phase 3: Event Integration

For streaming formats, integrate with event emitter:

```go
if format == FormatJSONRPC {
    // Subscribe to events
    subscriber := eventEmitter.Subscribe(eventTypes...)
    defer eventEmitter.Unsubscribe(subscriber.ID())
    
    // Stream events as JSON-RPC notifications
    return streamFormatHandler.Stream(ctx, result, writer)
}
```

## Use Cases

### 1. Real-time Command Monitoring

```bash
zqk object list --format json-rpc
# Streams: object.created, object.updated events as they happen
```

### 2. Long-running Operations

```bash
zqk system check --format json-rpc
# Streams: check.started, check.progress, check.completed events
```

### 3. MCP Integration

In MCP mode, `--format json-rpc` automatically:
- Subscribes to relevant events
- Streams them to the client
- Returns final result when done

## Comparison: Current vs. Proposed

### Current (Separate API)

```json
// Step 1: Subscribe to events
{"jsonrpc": "2.0", "id": 1, "method": "events/subscribe", "params": {...}}

// Step 2: Execute command
{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {...}}

// Step 3: Receive events (notifications)
{"jsonrpc": "2.0", "method": "notifications/event", "params": {...}}
```

### Proposed (Unified Format)

```json
// Single command with format option
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "cli_object_list",
    "arguments": {
      "format": "json-rpc"  // ← Format option
    }
  }
}

// Automatically streams events as notifications
{"jsonrpc": "2.0", "method": "notifications/event", "params": {...}}
```

## Migration Path

1. **Add format constants** - Extend `OutputFormat` enum
2. **Update validation** - Include new formats in `ValidateFormat`
3. **Create format handlers** - Implement streaming format handlers
4. **Integrate with events** - Wire up event emitter for streaming formats
5. **Update MCP bridge** - Pass format through to commands
6. **Documentation** - Update help text and docs

## Future Enhancements

1. **Format-specific event filtering**: `--format json-rpc --events log.error,permission.denied`
2. **Format composition**: `--format json-rpc --format json` (stream events + return JSON result)
3. **Format profiles**: `--context streaming` (pre-configured for streaming formats)

## Related Documentation

- [MCP Event Subscription](./MCP_EVENT_SUBSCRIPTION.md)
- [CLI Output Format System](../../internal/cli/README.md)
- [MCP Output Routing](./MCP_OUTPUT_ROUTING.md)

