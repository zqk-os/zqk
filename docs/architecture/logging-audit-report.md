# Logging System Audit Report

**Last Verified:** 2026-08-31


**Date**: 2025-01-XX  
**Status**: Audit Complete - Migration Required

## Executive Summary

The zqk codebase has a **comprehensive logging framework** in `pkg/logging/` that supports multiple output formats and context-aware logging. However, **most non-test code is NOT using it** and instead uses direct `fmt.Print*` calls.

## Logging Framework Status

### ✅ Framework Capabilities

The logging framework (`pkg/logging/`) provides:

1. **Multiple Output Formats**:
   - `JSONFormatter`: Structured JSON output for AI agents
   - `TextFormatter`: Human-readable text output
   - `CompactFormatter`: Compact format for debug mode

2. **Context-Aware Logging**:
   - `GetLoggerFromContext(ctx)`: Extracts profile from CLI context
   - `GetLoggerFromProfile(profile)`: Creates logger based on profile
   - Profiles: `"ai-agent"` → JSON, `"human"` → Text, `"debug"` → Compact

3. **Structured Logging**:
   - Log levels: Debug, Info, Warn, Error, Fatal
   - Field-based structured data
   - Context integration
   - Object reference tracking

4. **EventLogger**:
   - High-level event logging methods
   - Automatic context extraction
   - Specialized methods for common events

### ❌ Current Usage

**Only 1 file uses the logging system:**
- `cmd/zqk/system/check_impl.go` ✅

**Files using direct fmt.Print* (non-test):**
- `cmd/zqk/object/count.go` - 22 instances
- `cmd/zqk/object/list.go` - 28 instances
- `cmd/zqk/object/create.go` - 14 instances
- `cmd/zqk/object/update.go` - 16 instances
- `cmd/zqk/object/delete.go` - 10 instances
- `cmd/zqk/object/get.go` - 8 instances
- `cmd/zqk/object/fields.go` - 108 instances
- `cmd/zqk/root.go` - 2 instances
- `cmd/zqk/utility/version.go` - 4 instances
- `cmd/zqk/utility/migrate.go` - 6 instances
- `pkg/storage/hash_migration.go` - 5 instances (fmt.Fprintf to stderr)
- Various pkg files (365 instances across 41 files)

## Output Format Support Verification

### ✅ Confirmed Support

The framework supports three output formats based on context profile:

1. **JSON Format** (`ai-agent` profile):
   ```json
   {
     "timestamp": "2025-01-XXT...",
     "level": "info",
     "message": "Operation completed",
     "field1": "value1",
     "field2": 123
   }
   ```

2. **Text Format** (`human` profile):
   ```
   [info] 2025-01-XXT... Operation completed field1=value1 field2=123
   ```

3. **Compact Format** (`debug` profile):
   ```
   15:04:05.000 [I] Operation completed field1=value1 field2=123
   ```

### Implementation Details

- `GetLoggerFromProfile()` automatically selects formatter based on profile
- `NewEventLogger(ctx)` extracts profile from context and creates appropriate logger
- Formatters implement the `Formatter` interface
- All formatters support structured fields

## Migration Requirements

### High Priority (CLI Commands)

1. **cmd/zqk/object/*.go** - All object CRUD commands
   - Replace `fmt.Printf/Println` with logger.Info/Debug
   - Use `GetLoggerFromContext(cmd.Context())` to get logger
   - For user-facing output, use appropriate log levels:
     - Info: Normal operation messages
     - Debug: Detailed diagnostic info
     - Error: Error messages (already using fmt.Errorf for returns)

2. **cmd/zqk/root.go** - Root command error handling
   - Replace error output with logger.Error

3. **cmd/zqk/utility/*.go** - Utility commands
   - Migrate to logging system

### Medium Priority (Package Code)

1. **pkg/storage/hash_migration.go**
   - Replace `fmt.Fprintf(os.Stderr, ...)` with logger.Warn/Info
   - Pass context through migration functions

2. **pkg/graph/memgraph/*.go**
   - Add logging for connection, transaction, and pool operations
   - Use Debug level for detailed operations

3. **pkg/objects/*.go**
   - Add logging for spec loading, validation, field discovery
   - Use EventLogger methods where appropriate

## Recommended Migration Pattern

For CLI commands:

```go
func runCommand(cmd *cobra.Command, args []string) error {
    ctx := cli.GetContext(cmd)
    logger := logging.GetLoggerFromContext(cmd.Context())
    
    // Instead of: fmt.Printf("Processing...\n")
    logger.Info("Processing operation", 
        logging.String("kind", kind),
        logging.Int("count", count))
    
    // Instead of: fmt.Println("Done")
    logger.Info("Operation completed")
    
    // For errors, still return fmt.Errorf, but also log:
    if err != nil {
        logger.Error("Operation failed", err,
            logging.String("operation", "create"),
            logging.String("kind", kind))
        return fmt.Errorf("failed: %w", err)
    }
    
    return nil
}
```

For package code:

```go
func SomeFunction(ctx context.Context, ...) error {
    logger := logging.NewEventLogger(ctx)
    
    // Instead of: fmt.Printf("Loading spec: %s\n", specFile)
    logger.LogSpecLoad(specFile, nil)
    
    // Or for custom events:
    logger.logger.Info("Custom event",
        logging.String("event", "custom"),
        logging.String("field", value))
    
    return nil
}
```

## Output Format Testing

✅ **VERIFIED**: All three output formats work correctly:

```bash
# Test results:
=== JSON Format (ai-agent) ===
{"field1":"value1","field2":123,"level":"info","message":"Test message","timestamp":"2025-12-26T12:02:34-08:00"}

=== Text Format (human) ===
[info] 2025-12-26T12:02:34-08:00 Test message field1=value1 field2=123

=== Compact Format (debug) ===
12:02:34.260 [i] Test message field1=value1 field2=123
```

To use in CLI commands:

```bash
# JSON format (ai-agent context)
zqk --context ai-agent count backlog_item

# Text format (human context, default)
zqk count backlog_item

# Compact format (debug context)
zqk --context debug count backlog_item
```

## Conclusion

✅ **Logging framework supports multiple output formats** (JSON, Text, Compact) - **VERIFIED**  
✅ **Context-aware formatter selection works** - **VERIFIED**  
❌ **Most code is NOT using the logging framework** - **Migration Required**  
📋 **Migration required** to replace `fmt.Print*` with structured logging

The framework is ready and capable, but needs to be integrated throughout the codebase.

