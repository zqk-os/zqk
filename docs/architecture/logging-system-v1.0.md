# Logging System Architecture

**Last Verified:** 2026-08-31


**Version**: 1.0  
**Status**: Design  
**Date**: 2025-12-25

## Problem Statement

The zqk system needs a formal logging infrastructure before further development. Current code uses `fmt.Printf` and similar ad-hoc logging, which:
- Lacks structured data
- No log levels
- No context integration
- No output format control (JSON for AI agents, human-readable for humans)
- Difficult to filter, search, and analyze

## Requirements

### Core Requirements

1. **Structured Logging**: All logs must be structured with fields (timestamp, level, message, context, etc.)
2. **Log Levels**: Support standard levels (DEBUG, INFO, WARN, ERROR, FATAL)
3. **Context Integration**: Integrate with CLI context system (ai-agent, human, debug)
4. **Output Formats**: Support JSON (for AI agents) and human-readable (for humans)
5. **Performance**: Minimal overhead, async where possible
6. **Traceability**: Link logs to system objects (backlog items, requirements, etc.)

### Context-Aware Output

- **ai-agent context**: JSON output, all log levels
- **human context**: Human-readable output, INFO and above
- **debug context**: Human-readable output, all log levels with stack traces

## Architecture

### Logger Interface

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, err error, fields ...Field)
    Fatal(msg string, err error, fields ...Field)
    
    WithFields(fields ...Field) Logger
    WithContext(ctx context.Context) Logger
    WithObjectRef(kind, id string) Logger
}
```

### Field System

```go
type Field struct {
    Key   string
    Value interface{}
}

// Helpers
func String(key, value string) Field
func Int(key string, value int) Field
func Error(err error) Field
func ObjectRef(kind, id string) Field
```

### Output Formatters

1. **JSONFormatter**: For AI agents, structured JSON
2. **TextFormatter**: For humans, readable text
3. **CompactFormatter**: For debug, compact with stack traces

### Context Integration

- Logger reads from `internal/cli/context` to determine output format
- Logs include context metadata (command, user, etc.)
- Supports correlation IDs for request tracing

## Implementation Plan

### Phase 1: Core Logger

1. Create `pkg/logging` package
2. Implement `Logger` interface
3. Implement field system
4. Basic output formatters (JSON, Text)
5. Context integration

### Phase 2: Advanced Features

1. Async logging
2. Log rotation
3. File output
4. Structured error logging
5. Performance metrics

### Phase 3: Integration

1. Replace all `fmt.Printf` with logger
2. Add logging to critical paths
3. CLI command logging
4. MCP server logging

## Requirements

- REQ-023: Structured Logging System
- REQ-024: Context-Aware Log Output
- REQ-025: Log Traceability to System Objects

## Criteria

- CRIT-8208: All logs are structured with fields
- CRIT-8209: Log levels are properly used
- CRIT-8210: Context determines output format
- CRIT-8211: Logs can be traced to system objects
- CRIT-8212: Performance overhead is minimal

