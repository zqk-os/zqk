# MCP Spec-Based Access Control

**Status**: Implemented  
**Date**: 2025-12-30  
**Purpose**: Document spec-based access control with field-level restrictions and cascading access

## Overview

The MCP server now uses **spec-based access control** instead of object-level tags. Access restrictions are defined in object specifications (YAML files), providing centralized management and field-level granularity.

## Key Benefits

1. **Centralized Management**: Access rules defined in specs, not per object
2. **Field-Level Control**: Restrict access to specific fields within objects
3. **Cascading Access**: Access to referenced objects through relationships
4. **Maintainable**: Update access rules by editing spec files

## Architecture

### Spec-Based Access Control

Access control is defined in object specification files:

```yaml
fields:
  confidential_field:
    type: string
    semantic_type: statement
    traits: [readable, writable]
    access:
      read:
        roles: ["admin", "executive"]
        permissions: ["read:confidential"]
        requires: ["access:confidential"]
      write:
        roles: ["admin"]
        permissions: ["write:confidential"]
    checklist:
      security: "sensitive - contains confidential information"
```

### Field-Level Access

Each field can specify:
- **Read Roles**: Roles that can read the field
- **Read Permissions**: Permissions required to read the field
- **Write Roles**: Roles that can write the field
- **Write Permissions**: Permissions required to write the field
- **Requires Access**: Additional access requirements (e.g., team membership)

### Cascading Access

When a user has access to object A, and A references object B:
- User can see object B (cascading access)
- Field-level restrictions still apply to B
- User doesn't need direct access to B

**Example**:
- User has access to `BLI-001` (backlog item)
- `BLI-001` references `REQ-001` (requirement)
- User can see `REQ-001` even without direct access
- But sensitive fields in `REQ-001` are still filtered

## Implementation

### SpecAccessControl (`pkg/mcp/spec_access_control.go`)

Provides:
- `HasFieldAccess(field, obj, secCtx, operation)` - Checks field-level access
- `FilterObjectFields(obj, secCtx, operation)` - Filters object to accessible fields
- `HasCascadingAccess(referencedObj, referencingObj, secCtx)` - Checks cascading access
- `HasObjectAccess(obj, secCtx)` - Checks kind-level access

### Access Control Flow

1. **Object-Level Check**: Verify user has access to object kind
2. **Field-Level Filtering**: Filter object to only accessible fields
3. **Cascading Access**: For referenced objects, grant limited access if user has access to referencing object

### Integration

- **CLI Command Results**: `applyObjectAccessControl()` uses spec-based filtering
- **Object get/update (CLI)**: `zqk object get` and `zqk object update` apply field-level read/write checks when a project spec loader is available (BLI-642); uses `NewSpecAccessControlFromObjectsLoader` and `FilterObjectFields` / `HasFieldAccess`.
- **Reference Traversal**: Cascading access applied when following references
- **Field Filtering**: Sensitive fields automatically filtered based on spec

## Spec Format

### Field Access Control

```yaml
fields:
  field_name:
    type: string
    semantic_type: statement
    traits: [readable, writable]
    access:
      read:
        roles: ["admin", "developer"]
        permissions: ["read:field_name"]
        requires: ["access:team-alpha"]
      write:
        roles: ["admin"]
        permissions: ["write:field_name"]
    checklist:
      security: "sensitive"  # Also indicates sensitivity
```

### Security Metadata

The `checklist.security` field can indicate sensitivity:
- `"non-sensitive"` - No restrictions
- `"sensitive"` - Requires `read:sensitive` permission
- `"confidential"` - Requires explicit confidential access
- `"may contain sensitive details"` - Treated as sensitive

## Cascading Access Rules

1. **Direct Access**: User has kind-level permission → full object access (field-level restrictions apply)
2. **Cascading Access**: User has access to referencing object → limited access to referenced object
3. **Field Restrictions**: Cascading access still respects field-level restrictions

### Example

```
User has: read:backlog_item
Object A: BLI-001 (backlog_item) - user has access
Object B: REQ-001 (requirement) - referenced by BLI-001
  - User can see REQ-001 (cascading access)
  - But sensitive fields in REQ-001 are filtered
  - User doesn't need read:requirement permission
```

## Migration from Tag-Based

The system supports both approaches:
- **Spec-based** (preferred): Access control in spec files
- **Tag-based** (fallback): Access control via object tags

If spec loader is unavailable, falls back to tag-based access control for backward compatibility.

## Benefits Over Tag-Based

1. **Centralized**: All access rules in one place (spec files)
2. **Field-Level**: Granular control per field
3. **Maintainable**: Update rules by editing specs
4. **Type-Safe**: Access rules validated with spec validation
5. **Cascading**: Automatic access through references

## Future Enhancements

1. **Query-Level Filtering**: Push access restrictions to database queries
2. **Access Logging**: Log when fields are filtered
3. **Access Metrics**: Track access patterns
4. **Hierarchical Access**: Support nested team structures
5. **Time-Based Access**: Support time-limited access

## Related Documents

- [MCP Object Access Control](./MCP_OBJECT_ACCESS_CONTROL.md) - Original tag-based approach
- [MCP Security Enforcement](./MCP_SECURITY_ENFORCEMENT.md) - Command-level security

