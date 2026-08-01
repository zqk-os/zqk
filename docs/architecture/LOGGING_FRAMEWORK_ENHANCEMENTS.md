# Logging Framework Enhancements for Complex Output Patterns

**Status**: Design Proposal  
**Version**: 1.0  
**Date**: 2026-01-05  
**Purpose**: Enhance the logging framework to support complex output patterns (progress bars, real-time updates, streaming) while maintaining compliance with POL-CODE-007

## Problem Statement

The current logging framework (`pkg/logging/`) is designed for structured log entries but doesn't handle:
1. **Progress bars** - Need `\r` for line overwriting
2. **Real-time streaming** - Continuous updates during long operations
3. **Interactive output** - User-facing status updates vs. structured logs
4. **Format-aware output** - Different formats for humans vs. agents/systems
5. **Multiple output channels** - Logs vs. progress vs. command results

Current workaround: `cli_notifier.go` uses direct `fmt.Fprintf(os.Stderr, ...)` which violates POL-CODE-007.

## Proposed Solution

### 1. Extend Logger Interface with Progress/Interactive Methods

Add new methods to the `Logger` interface for interactive output:

```go
type Logger interface {
    // Existing methods
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, err error, fields ...Field)
    Fatal(msg string, err error, fields ...Field)
    
    // New methods for interactive output
    Progress(operationID string, progress int, message string, fields ...Field)
    Status(operationID string, oldStatus, newStatus string, fields ...Field)
    Stream(ctx context.Context, eventType string, data interface{}, fields ...Field) error
}
```

### 2. Add Progress Formatter

Create a new formatter type for progress/interactive output:

```go
type ProgressFormatter interface {
    // Format progress update (supports line overwriting)
    FormatProgress(operationID string, progress int, message string, fields map[string]any) ([]byte, error)
    
    // Format status change
    FormatStatus(operationID string, oldStatus, newStatus string, fields map[string]any) ([]byte, error)
    
    // Format streaming event
    FormatStreamEvent(eventType string, data interface{}, fields map[string]any) ([]byte, error)
    
    // Supports line overwriting (for progress bars)
    SupportsOverwrite() bool
}
```

### 3. Implement Progress Formatters

#### TextProgressFormatter (for human output)
- Uses `\r` for line overwriting
- Human-readable progress bars
- Status messages with arrows (e.g., "Status: pending → in_progress")

#### JSONProgressFormatter (for agents/systems)
- Structured JSON events
- No line overwriting (each update is a separate JSON object)
- Machine-parseable progress data
- Compatible with MCP event system

#### CompactProgressFormatter (for debug)
- Compact single-line updates
- Supports overwriting
- Minimal overhead

### 4. Output Channel Separation

Separate output channels:
- **Logs** → Structured log entries (existing behavior)
- **Progress** → Interactive updates (new)
- **Command Results** → Final output (existing via `cli.WriteOutput`)

Each channel can have different:
- Formatters (text for humans, JSON for agents)
- Destinations (stdout, stderr, files)
- Formatting rules (overwrite vs. append)

### 5. Context-Aware Progress Output

Progress output respects context:
- **Human profile**: Text progress bars with overwriting
- **AI Agent profile**: JSON structured events (no overwriting)
- **MCP mode**: JSON events via stderr (stdout reserved for JSON-RPC)
- **Debug profile**: Compact progress with overwriting

### 6. Streaming Support

Add streaming capabilities for real-time updates:

```go
type StreamLogger interface {
    // Start a stream for an operation
    StartStream(ctx context.Context, operationID string, eventTypes []string) (StreamWriter, error)
    
    // Write streaming event
    WriteStreamEvent(streamID string, eventType string, data interface{}) error
    
    // Close stream
    CloseStream(streamID string) error
}
```

## Implementation Plan

### Phase 1: Core Progress Interface
1. Add `Progress()` and `Status()` methods to `Logger` interface
2. Create `ProgressFormatter` interface
3. Implement `TextProgressFormatter` and `JSONProgressFormatter`
4. Update `logger` implementation to support progress methods

### Phase 2: Integration
1. Update `LogRouter` to handle progress output separately from logs
2. Add progress destination configuration
3. Integrate with existing format handlers

### Phase 3: Migration
1. Migrate `cli_notifier.go` to use new progress methods
2. Update all progress-related code to use logging framework
3. Remove direct `fmt.Fprintf` calls for progress

### Phase 4: Streaming
1. Add `StreamLogger` interface
2. Implement streaming formatters
3. Integrate with MCP event system

## Example Usage

### Before (Violates POL-CODE-007)
```go
// cli_notifier.go
fmt.Fprintf(os.Stderr, "\r%s %s %s", bar, op.ObjectID, message)
```

### After (Compliant)
```go
// Using new progress methods
logger := logging.GetLoggerFromProfile(ctx.Profile)
logger.Progress(op.ID, progress, message,
    logging.String("object_id", op.ObjectID),
    logging.String("operation_type", string(op.Type)))
```

### Format-Aware Behavior

**Human profile** (text):
```
[====================] BLI-001 Processing... 100%
```

**AI Agent profile** (JSON):
```json
{"type":"progress","operation_id":"op-123","progress":100,"message":"Processing...","object_id":"BLI-001","timestamp":"2026-01-05T19:45:00Z"}
```

**MCP mode** (JSON via stderr):
- Same as AI Agent profile
- Automatically routed to stderr
- Compatible with MCP event system

## Benefits

1. **Policy Compliance**: All output goes through logging framework (POL-CODE-007)
2. **Format Flexibility**: Different formats for different consumers
3. **Agent-Friendly**: Structured JSON events for programmatic consumption
4. **Human-Friendly**: Readable progress bars for interactive use
5. **MCP-Compatible**: Works seamlessly in MCP mode
6. **Extensible**: Easy to add new progress formatters
7. **Testable**: Progress output can be captured and verified in tests

## Architecture Alignment

- **CLI Bridge Pattern**: Progress output respects CLI context and format flags
- **Context-Driven**: Progress formatter selected based on context profile
- **Storage Provider Abstraction**: Progress output doesn't depend on storage backend
- **Single Context Principle**: Progress uses same context as logging

## Related Documentation

- [POL-CODE-007](../policies/POL-CODE-007.yaml) - Logging Architecture Policy
- [MCP Event Format](./MCP_EVENT_FORMAT.md) - Event streaming format
- [MCP Output Routing](./MCP_OUTPUT_ROUTING.md) - Output routing in MCP mode
- [CLI Format Handlers](../../internal/cli/format_handler.go) - Format handler system

