# MCP Privilege Validation Tests v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Category**: CLI & Interface

## Overview

This document describes the comprehensive test suite that validates privilege-based filtering and ensures only authorized MCP functionality is exposed.

## Test Coverage

### 1. Command-Level Permission Filtering

#### `TestPrivilegeFiltering_ReadOnlyCommands`
**Purpose**: Validates that read-only users only see read commands.

**Validates**:
- ✅ Read-only users can see `object list` (requires `read:*`)
- ✅ Read-only users CANNOT see `object create` (requires `write:backlog_item`)
- ✅ Read-only users CANNOT see `object delete` (requires `delete:backlog_item`)

**Security Context**: `account:viewer` with `roles: ["viewer"]`, `permissions: ["read:*"]`

#### `TestPrivilegeFiltering_WritePermissions`
**Purpose**: Validates that users with write permissions see write commands but not delete commands.

**Validates**:
- ✅ Users with write permissions can see `object list`
- ✅ Users with write permissions can see `object create`
- ✅ Users with write permissions CANNOT see `object delete` (requires delete permission)

**Security Context**: `account:writer` with `roles: ["developer"]`, `permissions: ["read:*", "write:backlog_item"]`

#### `TestPrivilegeFiltering_AdminRole`
**Purpose**: Validates that admin role bypasses all permission checks.

**Validates**:
- ✅ Admin users see ALL commands regardless of permission requirements
- ✅ Admin can see `object list`
- ✅ Admin can see `object create`
- ✅ Admin can see `object delete`

**Security Context**: `account:admin` with `roles: ["admin"]`, `permissions: []` (no specific permissions needed)

#### `TestPrivilegeFiltering_RoleRequirements`
**Purpose**: Validates that commands with role requirements are filtered correctly.

**Validates**:
- ✅ Developers can see regular commands (e.g., `object list`)
- ✅ Developers CANNOT see admin-only commands (e.g., `system admin`)
- ✅ Admins can see admin-only commands

**Security Contexts**:
- Developer: `account:dev` with `roles: ["developer"]`, `permissions: ["read:*", "write:*"]`
- Admin: `account:admin` with `roles: ["admin"]`, `permissions: []`

#### `TestPrivilegeFiltering_WildcardPermissions`
**Purpose**: Validates wildcard permission matching (e.g., `read:*` matches all read operations).

**Validates**:
- ✅ Users with `read:*` can see read commands
- ✅ Users with only `read:*` CANNOT see write commands

**Security Context**: `account:wildcard` with `roles: ["viewer"]`, `permissions: ["read:*"]`

#### `TestPrivilegeFiltering_NoPermissions`
**Purpose**: Validates that users with no permissions see minimal or no commands.

**Validates**:
- ✅ Users with no permissions CANNOT see `object create`
- ✅ Users with no permissions CANNOT see `object delete`

**Security Context**: `account:guest` with `roles: ["guest"]`, `permissions: []`

### 2. MCP Tool Registration Filtering

#### `TestMCPToolRegistration_PrivilegeFiltering`
**Purpose**: Validates that MCP tools are filtered by privileges during registration.

**Validates**:
- ✅ Read-only users can see `cli_object_list` tool
- ✅ Read-only users CANNOT see `cli_object_create` tool
- ✅ Read-only users CANNOT see `cli_object_delete` tool

**Security Context**: `account:viewer` with `roles: ["viewer"]`, `permissions: ["read:*"]`

**Test Flow**:
1. Create server
2. Register CLI tools with read-only security context
3. List registered tools
4. Verify only authorized tools are present

#### `TestMCPToolRegistration_AdminSeesAll`
**Purpose**: Validates that admin users see all MCP tools.

**Validates**:
- ✅ Admin can see `cli_object_list` tool
- ✅ Admin can see `cli_object_create` tool
- ✅ Admin can see `cli_object_delete` tool

**Security Context**: `account:admin` with `roles: ["admin"]`, `permissions: []`

## Test Command Annotations

Tests use commands with annotations to specify permission and role requirements:

### Permission Annotations
```go
Annotations: map[string]string{
    "mcp.permissions": "read:*",           // Read any object
    "mcp.permissions": "write:backlog_item", // Write backlog items
    "mcp.permissions": "delete:backlog_item", // Delete backlog items
}
```

### Role Annotations
```go
Annotations: map[string]string{
    "mcp.roles": "admin",  // Requires admin role
}
```

## Permission Matching Logic

The privilege filtering uses the following logic:

1. **Role Check**:
   - If command requires roles, user must have at least one required role
   - `admin` role bypasses all checks
   - If no roles required, skip role check

2. **Permission Check**:
   - If command requires permissions, user must have at least one required permission
   - Wildcard matching: `read:*` matches `read:backlog_item`
   - Exact matching: `write:backlog_item` matches `write:backlog_item`
   - If no permissions required, skip permission check

3. **Default Behavior**:
   - Commands without annotations are visible to all users
   - This allows backward compatibility

## Running the Tests

```bash
# Run all privilege tests
go test ./pkg/mcp -run TestPrivilegeFiltering -v

# Run MCP tool registration tests
go test ./pkg/mcp -run TestMCPToolRegistration -v

# Run all privilege-related tests
go test ./pkg/mcp -run "TestPrivilegeFiltering|TestMCPToolRegistration" -v
```

## Test Coverage Summary

| Test | Commands Tested | Permission Checks | Role Checks | MCP Tool Checks |
|------|----------------|-------------------|-------------|-----------------|
| `TestPrivilegeFiltering_ReadOnlyCommands` | ✅ | ✅ | ❌ | ❌ |
| `TestPrivilegeFiltering_WritePermissions` | ✅ | ✅ | ❌ | ❌ |
| `TestPrivilegeFiltering_AdminRole` | ✅ | ✅ | ✅ | ❌ |
| `TestPrivilegeFiltering_RoleRequirements` | ✅ | ❌ | ✅ | ❌ |
| `TestPrivilegeFiltering_WildcardPermissions` | ✅ | ✅ | ❌ | ❌ |
| `TestPrivilegeFiltering_NoPermissions` | ✅ | ✅ | ❌ | ❌ |
| `TestMCPToolRegistration_PrivilegeFiltering` | ❌ | ✅ | ❌ | ✅ |
| `TestMCPToolRegistration_AdminSeesAll` | ❌ | ✅ | ✅ | ✅ |

## Security Guarantees

These tests ensure:

1. **Principle of Least Privilege**: Users only see commands they have permission to execute
2. **Role-Based Access Control**: Commands with role requirements are properly filtered
3. **Permission Granularity**: Fine-grained permissions (e.g., `write:backlog_item`) work correctly
4. **Wildcard Support**: Wildcard permissions (e.g., `read:*`) match correctly
5. **Admin Override**: Admin role bypasses all permission checks
6. **MCP Tool Parity**: MCP tools reflect the same privilege filtering as CLI commands

## Future Test Enhancements

1. **Integration Tests**: Test end-to-end MCP tool calls with different security contexts
2. **Edge Cases**: Test complex permission combinations
3. **Performance Tests**: Verify filtering doesn't impact performance
4. **Concurrent Access**: Test privilege filtering under concurrent tool registration
5. **Dynamic Permission Updates**: Test behavior when permissions change during runtime

## Related Documentation

- [MCP Server Package](../../../pkg/mcp/README.md) - Package implementation
- [MCP CLI Bridge Architecture v1.0](./mcp-cli-bridge-v1.0.md) - CLI bridge implementation details

---

**Status**: Active  
**Last Updated**: 2025-01-02

