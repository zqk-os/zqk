# MCP Role Elicitation

**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Document the role/credentials elicitation during MCP server initialization

## Overview

When an agent connects to the MCP server without specifying roles or permissions, the server will **elicit** (ask) the agent to provide their role or credentials. This ensures that agents explicitly specify their access level and receive the appropriate permissions.

## When Elicitation Occurs

Elicitation is triggered during the `initialize` method when:
- ✅ No `roles` are provided in client capabilities
- ✅ No `permissions` are provided in client capabilities
- ✅ The account is NOT a system account (system accounts bypass elicitation)

## Elicitation Parameters

When elicitation is triggered, the server asks for:

### 1. `roles` (Required)
- **Type**: `array` of strings
- **Description**: Your role(s) in the system
- **Choices**: 
  - `admin` - Full access to all operations
  - `developer` - Code, test, and backlog access
  - `viewer` - Read-only access
  - `founder` - Executive access (same as admin)
  - `executive` - Executive access
  - `owner` - Project owner access
  - `observer_agent` - Observer agent (graph operations)
  - `test_agent` - Test agent
  - `coder_agent` - Coder agent
- **Format**: Can be provided as an array `["admin"]` or comma-separated string `"admin,developer"`

### 2. `permissions` (Optional)
- **Type**: `array` of strings
- **Description**: Specific permissions if you need custom access
- **Format**: `operation:resource` (e.g., `"read:*"`, `"write:backlog_item"`)
- **Note**: If not provided, permissions are automatically determined from your role(s)

### 3. `account_id` (Optional)
- **Type**: `string`
- **Description**: Your account ID (e.g., `"account:developer"`, `"account:viewer"`)
- **Note**: If not provided, will be generated from your role

## Example Elicitation Response

When the server elicits role/credentials, the agent should respond by re-initializing with the required information:

```json
{
  "method": "initialize",
  "params": {
    "capabilities": {
      "roles": ["developer"],
      "permissions": ["read:*", "write:backlog_item", "write:test_case"]
    },
    "clientInfo": {
      "name": "my-agent",
      "version": "1.0.0"
    }
  }
}
```

Or with a single role:

```json
{
  "method": "initialize",
  "params": {
    "capabilities": {
      "roles": ["admin"]
    },
    "clientInfo": {
      "name": "my-agent",
      "version": "1.0.0"
    }
  }
}
```

## Role-Based Permissions

### Admin Role
- **Permissions**: `read:*`, `write:*`, `delete:*`
- **Access**: Full access to all operations
- **Bypasses**: Config whitelisting (sees all CLI tools)

### Developer Role
- **Permissions**: `read:*`, `write:code`, `write:test_case`, `write:backlog_item`
- **Access**: Code development, testing, backlog updates

### Viewer Role
- **Permissions**: `read:*`
- **Access**: Read-only access to all objects

### Founder/Executive Roles
- **Permissions**: `read:*`, `write:*`, `delete:*`
- **Access**: Same as admin, but distinct role for executive authority

## Default Behavior

If an agent does not respond to the elicitation and re-initializes without roles/permissions:
- **System accounts**: Automatically get system context (full access)
- **Other accounts**: Default to `viewer` role with `read:*` permissions

## Benefits

1. **Explicit Access Control**: Agents must explicitly specify their role
2. **Security**: Prevents accidental access with wrong permissions
3. **Audit Trail**: Clear record of which agent has which role
4. **Flexibility**: Supports custom permissions if needed

## Implementation Details

The elicitation is implemented in `pkg/mcp/server_handlers.go` in the `handleInitialize` function. It checks if roles/permissions are missing and returns an `ElicitationError` with the required parameters.

The elicitation error is converted to a JSON-RPC error with elicitation data, which the client can use to prompt the agent for the required information.

