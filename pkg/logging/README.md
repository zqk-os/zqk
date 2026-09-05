# Logging Package

This package provides structured logging with context-aware routing, MCP protocol protection, and multi-destination support.

## Overview

The `pkg/logging` package implements a comprehensive logging system that:
- **Routes logs to multiple destinations** (files, stdout, stderr) with different formatters
- **Protects MCP protocol streams** by suppressing stdio output during MCP operations
- **Supports context-aware logging** with profile-based configuration
- **Provides structured JSON logging** with field support
- **Handles formatting failures gracefully** with emergency fallback output

## Package Structure

```
pkg/logging/
├── logger.go              # Core logger implementation
├── event_logger.go        # EventLogger with structured fields
├── router.go              # LogRouter for multi-destination routing
├── context_logger.go      # Context-aware logger creation
├── decision_context_logger.go  # LoggingDecisionContext integration
├── formatters.go          # JSON and text formatters
├── progress_logger.go     # Progress logging for CLI operations
├── buffered_writer.go     # Buffered I/O for file destinations
├── rolling_writer.go      # Rolling file writer
└── README.md              # This file
```

## Core Components

### EventLogger

Structured logger with field support and context awareness.

```go
import "github.com/lanceman/zqk/pkg/logging"

// Create logger from context
logger := logging.GetLoggerFromContext(ctx)

// Log with fields
logger.LogInfo("Operation started",
    logging.String("operation_id", "op-123"),
    logging.Int("total_items", 100),
)

// Log errors
logger.LogError("Operation failed", err,
    logging.String("operation_id", "op-123"),
)
```

### LogRouter

Routes logs to multiple destinations with different formatters and levels.

```go
import "github.com/lanceman/zqk/pkg/logging"

router := logging.NewLogRouter()

// Add file destination
file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
router.AddDestination("file", file, logging.InfoLevel, logging.NewJSONFormatter(ctx))

// Add stdout destination
router.AddDestination("stdout", os.Stdout, logging.InfoLevel, logging.NewTextFormatter(ctx))

// Route log entry
router.Route(ctx, logging.InfoLevel, "Message", nil, fields...)
```

### Context-Aware Logging

Loggers can be created from various context types:

```go
// From Go context (extracts LoggingContext)
logger := logging.GetLoggerFromContext(ctx)

// From LoggingContext directly
logger := logging.GetLoggerFromLoggingContext(ctx, loggingCtx)

// From LoggingDecisionContext (most context-aware)
logger := logging.GetLoggerFromDecisionContext(decisionCtx, projectRoot)
```

## MCP Protocol Protection

The logging system automatically protects MCP JSON-RPC protocol streams:

- **Suppresses stdio output** when MCP server is actively serving
- **Redirects debug logs** to stderr instead of stdout
- **Preserves file destinations** - only stdio is protected
- **Handles subprocess mode** - detects MCP subprocess environment

This ensures that logging output never pollutes the JSON-RPC protocol stream on stdout.

## Formatters

### JSONFormatter

Formats logs as compact JSON (JSONL format - one JSON object per line):

```json
{"timestamp":"2024-01-23T10:30:00Z","level":"info","message":"Operation started","operation_id":"op-123"}
```

### TextFormatter

Formats logs as human-readable text:

```
[2024-01-23T10:30:00Z] INFO: Operation started operation_id=op-123
```

## Architecture Decisions

### Context Propagation

All logging operations use `pkgctx.NewSystemContext()` for:
- Lock operations (timeout-protected)
- Default context creation
- Fallback contexts when no parent context is available

This ensures proper context propagation throughout the logging system.

### Emergency Fallback

When formatters fail, the system uses emergency fallback output:
- Uses `fmt.Sprintf` + `Write` instead of `fmt.Fprintf` to comply with POL-CODE-007
- Writes to stderr (never stdout) to protect MCP protocol
- Includes formatting error details for debugging
- Suppresses output during MCP operations to protect protocol stream

### Buffered I/O

File destinations use buffered writers to minimize I/O context switches:
- Automatic level-aware flushing (immediate flush on error/fatal)
- Configurable buffer sizes
- Thread-safe writes with mutex protection

## Integration

The logging package integrates with:
- **`pkg/context`** - Context-aware logger creation
- **`pkg/coordination`** - Event coordination for logging channel
- **`pkg/mcp`** - MCP protocol protection
- **`pkg/storage`** - Audit event integration

## Best Practices

1. **Always use context-aware loggers** - Use `GetLoggerFromContext()` instead of creating loggers directly
2. **Use structured fields** - Prefer `logging.String()`, `logging.Int()` over string formatting
3. **Respect MCP protocol** - Never write to stdout during MCP operations
4. **Use appropriate log levels** - Info for operations, Error for failures, Debug for detailed tracing
5. **Include operation context** - Always include operation IDs, object IDs, etc. in log fields

## Related Documentation

- `docs/process/architecture/` - Architecture documentation
- `pkg/context/` - Context package for context-aware logging
- `pkg/coordination/` - Event coordination system
