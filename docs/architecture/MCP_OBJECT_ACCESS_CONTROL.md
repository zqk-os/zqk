# MCP Object-Level Access Control via Semantic Tags

**Status**: Implemented  
**Date**: 2025-12-30  
**Purpose**: Document object-level access control using semantic tags

## Overview

The MCP server now enforces object-level access restrictions based on semantic tags. Objects can have access restriction tags that control who can view or access them, independent of command-level permissions.

## How It Works

### Semantic Tags for Access Control

Objects can have semantic tags in the `tags` field that indicate access restrictions:

- **`access:restricted`** - Requires explicit access permission
- **`access:admin-only`** - Requires admin role
- **`access:team-<teamname>`** - Requires team membership (e.g., `access:team-alpha`)
- **`access:<permission>`** - Requires specific permission (e.g., `access:confidential`)

### Security Context Access Permissions

The security context can have access permissions that indicate what the user can access:

- **`access:*`** - Can access all objects (bypasses restrictions)
- **`access:team-<teamname>`** - Can access objects for that team
- **`access:<permission>`** - Can access objects with that permission

### Access Control Logic

1. **No Tags = Accessible**: Objects without access restriction tags are accessible to everyone (with appropriate kind-level permissions)

2. **Admin Bypass**: Users with `admin` role can access all objects, regardless of tags

3. **Permission Matching**: 
   - User must have an access permission that matches the object's access tag
   - Wildcard `access:*` matches all access tags
   - Team tags match exactly (e.g., `access:team-alpha` matches `access:team-alpha`)

4. **Deny by Default**: If an object has access restriction tags and the user doesn't have matching permissions, access is denied

## Implementation

### ObjectAccessControl (`pkg/mcp/object_access_control.go`)

Provides:
- `HasAccess(obj, secCtx)` - Checks if security context has access to an object
- `FilterObjectsByAccess(objects, secCtx)` - Filters a list of objects
- `AddAccessFilterToQuery(secCtx, filters)` - Adds query-level filters (future optimization)

### Integration Points

1. **CLI Command Results** (`pkg/mcp/cli_bridge.go`)
   - `applyObjectAccessControl()` filters command results before returning to MCP client
   - Handles multiple result formats:
     - `{"objects": [...]}` - Array of objects
     - `{"object": {...}}` - Single object
     - Direct object (has `id` field)
     - Direct array

2. **Result Format Support**
   - Automatically detects result format
   - Updates counts when filtering arrays
   - Returns error for single objects when access denied

## Example Usage

### Object with Access Restriction

```yaml
id: BLI-001
kind: backlog_item
title: "Confidential Feature"
tags:
  - "feature"
  - "access:team-alpha"  # Only team-alpha can access
  - "access:confidential"  # Requires confidential permission
```

### Security Context with Access Permissions

```go
secCtx := &SecurityContext{
    AccountID: "account:user1",
    Roles: []string{"developer"},
    Permissions: []string{
        "read:backlog_item",
        "access:team-alpha",      // Can access team-alpha objects
        "access:confidential",    // Can access confidential objects
    },
}
```

### Result

- User can access `BLI-001` because they have both `access:team-alpha` and `access:confidential` permissions
- Objects with only `access:team-alpha` tag are accessible
- Objects with only `access:confidential` tag are accessible
- Objects with `access:team-beta` tag are NOT accessible (no matching permission)

## Security Layers

The MCP server now has three layers of security:

1. **Command-Level** (Config-based)
   - `exposed_commands` whitelist
   - `blocked_commands` blacklist
   - `write_operations` restrictions

2. **Kind-Level** (Permission-based)
   - `read:backlog_item` - Can read backlog items
   - `write:backlog_item` - Can write backlog items
   - `delete:backlog_item` - Can delete backlog items

3. **Object-Level** (Tag-based) ← **NEW**
   - `access:team-alpha` - Can access team-alpha objects
   - `access:confidential` - Can access confidential objects
   - `access:admin-only` - Admin-only objects

## Benefits

1. **Granular Control**: Restrict access to specific objects without changing command or kind permissions
2. **Team-Based Access**: Control access by team membership
3. **Confidentiality**: Mark objects as confidential and restrict access
4. **Flexible**: Use semantic tags for any access control pattern
5. **Transparent**: Objects are filtered automatically - no changes needed to commands

## Future Enhancements

1. **Query-Level Filtering**: Push access restrictions to database queries for efficiency
2. **Access Logging**: Log when objects are filtered due to access restrictions
3. **Access Metrics**: Track access patterns and restrictions
4. **Hierarchical Access**: Support nested team structures (e.g., `access:team-alpha:subteam-1`)
5. **Time-Based Access**: Support time-limited access permissions

## Related Documents

- [MCP Security Enforcement](./MCP_SECURITY_ENFORCEMENT.md) - Command-level security
- [MCP CLI Bridge Architecture](./mcp-cli-bridge-v1.0.md) - CLI bridge details

