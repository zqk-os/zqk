# MCP Format Restrictions

**Last Verified:** 2026-08-31


**Status**: Implemented  
**Version**: 1.0  
**Date**: 2025-01-XX

## Overview

The MCP server supports restricting clients to specific output formats. This provides fine-grained control over what data formats clients can request, enabling security policies that limit data exposure based on format characteristics.

## Use Cases

1. **Security**: Restrict clients to JSON-only output to prevent YAML/table formats that might expose sensitive data in human-readable form
2. **Compatibility**: Ensure clients only use formats they can properly parse
3. **Performance**: Limit clients to streaming formats for real-time data or non-streaming for batch operations
4. **Policy Enforcement**: Enforce organizational policies about data format usage

## Implementation

### Client-Level Restrictions

Clients can specify allowed formats during initialization via capabilities:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "capabilities": {
      "allowed_formats": ["json", "yaml"]
    }
  }
}
```

### Config-Level Restrictions

Format restrictions can also be set in `.zqk/mcp/config.yaml`:

```yaml
mcp_server:
  security:
    allowed_formats:
      - json
      - yaml
```

**Precedence**: Client capabilities override config settings.

### Permission-Based Restrictions

Format restrictions are checked in addition to permission-based restrictions:

1. **Client-level restrictions** (most restrictive) - checked first
2. **Permission-based restrictions** - checked second
3. **Default behavior** - if no restrictions, all formats allowed

## Format Permission Checking

The `CheckFormatPermission()` method validates format access:

1. Checks if user is active
2. Checks client-level format restrictions
3. Checks admin role (admins bypass restrictions)
4. Checks streaming format permissions (json-rpc, stream require special permissions)
5. Defaults to allowing standard formats (table, json, yaml)

## Error Handling

If a client requests a format that's not allowed:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32000,
    "message": "Format not allowed: format 'table' not allowed for this client (allowed: [json, yaml])",
    "data": {
      "format": "table",
      "reason": "format 'table' not allowed for this client (allowed: [json, yaml])",
      "allowed_formats": ["json", "yaml"]
    }
  }
}
```

## Default Behavior

- If no format restrictions are set, all formats are allowed
- Default format for MCP responses is `json`
- If client doesn't specify format and restrictions exist, first allowed format is used

## Examples

### Restrict to JSON Only

```yaml
# .zqk/mcp/config.yaml
mcp_server:
  security:
    allowed_formats:
      - json
```

### Allow Multiple Formats

```yaml
# .zqk/mcp/config.yaml
mcp_server:
  security:
    allowed_formats:
      - json
      - yaml
      - json-rpc
```

### Client-Specific Restrictions

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "capabilities": {
      "allowed_formats": ["json"]
    }
  }
}
```

## Integration Points

1. **Server Initialization** (`pkg/mcp/server_handlers.go`):
   - Extracts `allowed_formats` from client capabilities
   - Falls back to config if client doesn't specify
   - Sets restrictions on server via `SetAllowedFormats()`

2. **Tool Execution** (`pkg/mcp/server.go`):
   - `CheckFormatPermission()` validates format before command execution
   - Returns error if format not allowed
   - Overrides format to first allowed format if none specified

3. **Config Loading** (`pkg/mcp/server.go`):
   - Loads `allowed_formats` from `.zqk/mcp/config.yaml`
   - Applied during server initialization

## Security Considerations

- Format restrictions are enforced **before** command execution (like dry-run)
- Short-circuits unauthorized format access attempts efficiently
- Admin role bypasses format restrictions (for system operations)
- Streaming formats (json-rpc, stream) require additional permissions even if in allowed list

## Future Enhancements

1. **Per-Command Format Restrictions**: Restrict formats per command type
2. **Dynamic Format Restrictions**: Update restrictions at runtime
3. **Format Quotas**: Limit number of requests per format
4. **Format Audit Logging**: Log format usage for compliance

