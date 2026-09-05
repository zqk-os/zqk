# MCP Server Security Enforcement

**Last Verified:** 2026-08-31


**Status**: Implemented  
**Date**: 2025-12-30  
**Purpose**: Document security enforcement mechanisms in the MCP server

## Overview

The MCP server enforces security at multiple levels:
1. **Config-based filtering** (whitelist/blacklist from config file)
2. **Permission-based filtering** (roles/permissions from security context)
3. **Runtime enforcement** (checks before command execution)

## Security Layers

### Layer 1: Config-Based Filtering (Registration Time)

**Location**: `pkg/mcp/config_security.go`

Filters commands when registering tools during initialization:

1. **Blocked Commands** (Blacklist)
   - Commands matching patterns in `blocked_commands` are excluded
   - Supports wildcards: `"system git *"` blocks all `system git` commands
   - Takes precedence over whitelist

2. **Exposed Commands** (Whitelist)
   - If `exposed_commands` is specified, only matching commands are allowed
   - If empty, all commands are allowed (backward compatibility)
   - Supports wildcards: `"object *"` allows all `object` commands

3. **Write Operations**
   - Commands in `write_operations` are explicitly enabled
   - If `require_write_permission` is true, additional checks apply
   - Commands not in `write_operations` but detected as write ops are blocked if `require_write_permission` is true

**Pattern Matching**:
- Exact match: `"object list"` matches only `"object list"`
- Wildcard: `"system git *"` matches `"system git analyze"`, `"system git status"`, etc.
- Prefix: `"object"` matches `"object list"`, `"object get"`, etc.

### Layer 2: Permission-Based Filtering (Registration Time)

**Location**: `pkg/mcp/cli_bridge.go`

Filters commands based on security context (roles/permissions):

1. **Role-Based Access Control (RBAC)**
   - Commands can require specific roles via `mcp.roles` annotation
   - User must have matching role or `admin` role
   - Example: `mcp.roles: "developer"` requires `developer` or `admin` role

2. **Permission-Based Access Control (PBAC)**
   - Commands can require specific permissions via `mcp.permissions` annotation
   - Supports wildcards: `"read:*"` matches `"read:backlog_item"`, `"read:requirement"`, etc.
   - Example: `mcp.permissions: "write:backlog_item"` requires `write:backlog_item` permission

**Default Behavior**:
- If no roles/permissions specified, command is allowed
- If roles/permissions specified, user must match at least one

### Layer 3: Runtime Enforcement (Execution Time)

**Location**: `pkg/mcp/server.go` - `handleToolCallWithContext()`

Double-checks security before executing commands:

1. **Config Validation**
   - Verifies command is not blocked
   - Verifies command is in exposed list (if whitelist exists)
   - Verifies write operation is allowed

2. **Error Response**
   - Returns JSON-RPC error if command is not allowed
   - Includes reason in error message and data

## Configuration Example

```yaml
mcp_server:
  exposed_commands:
    - "object list"
    - "object get"
    - "system status"
  
  blocked_commands:
    - "system sync"
    - "system init"
    - "automation *"
    - "system git *"
  
  write_operations:
    # Currently empty - no write operations enabled
  
  security:
    require_write_permission: true
    default_context: "ai-agent"
    default_format: "json"
```

## Security Flow

```
Command Registration:
  Discover Commands
    ↓
  Filter by Permissions (RBAC/PBAC)
    ↓
  Filter by Config (Whitelist/Blacklist)
    ↓
  Register as MCP Tools

Command Execution:
  Receive Tool Call
    ↓
  Extract Command Path
    ↓
  Check Config Security (Runtime)
    ↓
  Execute Command (if allowed)
    ↓
  Return Result or Error
```

## Testing Security Enforcement

### Test 1: Blocked Commands

**Config**:
```yaml
blocked_commands:
  - "system sync"
```

**Expected**: `system sync` command should not appear in tools list and should return error if called directly.

### Test 2: Exposed Commands Whitelist

**Config**:
```yaml
exposed_commands:
  - "object list"
  - "object get"
```

**Expected**: Only `object list` and `object get` should appear in tools list.

### Test 3: Write Operations

**Config**:
```yaml
write_operations: []
security:
  require_write_permission: true
```

**Expected**: Write operations (create, update, delete) should be blocked.

### Test 4: Permission-Based Filtering

**Security Context**:
```go
roles: ["viewer"]
permissions: ["read:*"]
```

**Command Annotation**:
```go
mcp.roles: "developer"
```

**Expected**: Command should not appear in tools list (viewer role doesn't match developer).

## Implementation Details

### Pattern Matching

The `matchesCommandPattern()` function supports:
- **Exact match**: `"object list"` == `"object list"`
- **Wildcard suffix**: `"system git *"` matches `"system git analyze"`
- **Prefix match**: `"object"` matches `"object list"`, `"object get"`, etc.

### Write Operation Detection

The `isLikelyWriteOperation()` function detects write operations by keywords:
- `create`, `update`, `delete`
- `bulk_create`, `bulk_update`, `bulk_delete`
- `sync`, `init`, `register`, `enable`, `disable`, `flush`

### Error Responses

Security violations return JSON-RPC errors:
```json
{
  "error": {
    "code": -32000,
    "message": "Command not allowed: command 'system sync' is blocked by config (pattern: system sync)",
    "data": {
      "command": "system sync",
      "reason": "command 'system sync' is blocked by config (pattern: system sync)"
    }
  }
}
```

## Related Documents

- [MCP CLI Bridge Architecture](./mcp-cli-bridge-v1.0.md) - CLI bridge details
- [MCP Server Refactor](./MCP_SERVER_REFACTOR.md) - Server architecture
- [MCP Abstraction Layer](./MCP_ABSTRACTION_LAYER.md) - Protocol abstraction
- [Workflow Constraints vs MCP Config](./WORKFLOW_CONSTRAINTS_VS_MCP_CONFIG.md) - Integration strategy for workflow-driven access control

