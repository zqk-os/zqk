# MCP Output Routing Requirements

## Problem

The zqk CLI has **600+ direct `fmt.Printf` calls** that bypass the logging framework and write directly to `stdout`. In MCP mode, this pollutes the JSON-RPC protocol stream, causing parsing errors.

## Solution

### Immediate Fix (Version 1.0.8+)

Created helper functions in `internal/cli/output.go` that route output through the logging framework in MCP mode:

- `SafePrintf(cmd, format, args...)` - MCP-aware printf
- `SafePrintln(cmd, args...)` - MCP-aware println  
- `SafePrint(cmd, args...)` - MCP-aware print

**Behavior:**
- **In MCP mode** (when `ZQK_MCP_ACCOUNT_ID` is set): Output goes to `stderr` via logger (respects MCP profile)
- **In normal CLI mode**: Output goes directly to `stdout` (backward compatible)

### Migration Path

1. **For new code**: Use `SafePrint*` helpers instead of `fmt.Print*`
2. **For existing code**: Gradually migrate `fmt.Printf` calls to `SafePrintf`
3. **For structured output**: Use `cli.WriteOutput()` with JSON format when `--format json` is set

### Long-term Solution

All commands should use the structured output system:

```go
// Instead of:
fmt.Printf("Validating object: %s\n", objectID)

// Use:
data, _ := json.Marshal(map[string]any{
    "message": "Validating object",
    "object_id": objectID,
})
cli.WriteOutput(cmd, data) // Respects --format json flag
```

## Current Status

- ✅ Command result (WriteOutput/FormatOutput) goes to **stdout** in MCP subprocess so the parent MCP server can capture it; logs and progress go to stderr.
- ✅ Logger fallback fixed to always use stderr in MCP mode
- ✅ Helper functions created for MCP-aware output
- ⚠️ 600+ direct `fmt.Printf` calls still need migration
- ⚠️ Commands should use structured output for JSON format

## Files with Most Direct Output

- `cmd/zqk/system/validate.go` - 11 instances
- `cmd/zqk/system/check_impl.go` - 80+ instances
- `cmd/zqk/system/status.go` - 20+ instances
- `cmd/zqk/object/*.go` - 200+ instances across all object commands

## Detection

To find rogue output in MCP mode:

```bash
# Set MCP mode and run command
ZQK_MCP_ACCOUNT_ID=test ./bin/zqk-stable <command> --format json 2>&1 | grep -v '^{'
```

Any non-JSON output indicates direct stdout writes that need migration.

