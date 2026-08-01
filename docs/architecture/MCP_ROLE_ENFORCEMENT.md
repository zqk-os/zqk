# MCP Role Enforcement

**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Document role enforcement mechanisms for AI agents

## Overview

Role enforcement ensures that AI agents can only use roles that are allowed or enforced by the system configuration. This prevents agents from claiming unauthorized roles and ensures proper access control.

## Enforcement Mechanisms

### 1. Enforced Role (Highest Priority)

**Config**: `security.enforced_role`

**Behavior**: Forces ALL AI agents to use a specific role, regardless of what they claim.

**Example**:
```yaml
security:
  enforced_role: "viewer"  # All agents forced to use viewer role
```

**Use Case**: When you want all agents to have the same, restricted access level.

**Priority**: Highest - overrides all other enforcement rules.

### 2. Account-Based Role Enforcement

**Config**: `security.enforce_account_roles: true`

**Behavior**: If an `account_id` is provided, the system looks up the account object and enforces that the agent can only use roles assigned to that account.

**Example**:
```yaml
security:
  enforce_account_roles: true
```

**How it works**:
1. Agent provides `account_id` (e.g., `"account:developer"`)
2. System loads account object from `docs/architecture/accounts/account-developer.yaml`
3. System extracts `roles` field from account object
4. Agent's claimed roles are filtered to only those in the account's roles
5. If no matching roles, account's roles are used

**Use Case**: When you want to ensure agents can only use roles assigned to their account.

**Priority**: Medium - applied after enforced_role check, before allowed_roles.

### 3. Allowed Roles Whitelist

**Config**: `security.allowed_roles`

**Behavior**: Only allows agents to use roles from a whitelist. If agent claims a role not in the list, it's rejected.

**Example**:
```yaml
security:
  allowed_roles:
    - "viewer"
    - "developer"
    # Agent cannot claim "admin" or "founder" - will be rejected
```

**Use Case**: When you want to restrict agents to specific roles but allow them to choose from a set.

**Priority**: Low - applied after enforced_role and enforce_account_roles.

### 4. Role Validation

**Config**: `security.validate_roles: true`

**Behavior**: Validates that claimed roles exist in the system by checking against role objects in `docs/architecture/roles/`.

**Example**:
```yaml
security:
  validate_roles: true
```

**How it works**:
1. System loads all role objects from `docs/architecture/roles/*.yaml`
2. Extracts `role_id` from each role object
3. Validates that agent's claimed roles exist in the system
4. Rejects initialization if any role doesn't exist

**Use Case**: When you want to ensure agents can only claim roles that are actually defined in the system.

**Priority**: Applied after all other enforcement rules.

## Enforcement Order

Role enforcement is applied in this order:

1. **Enforced Role** (if set) - Overrides everything
2. **Account-Based Enforcement** (if enabled and account_id provided)
3. **Allowed Roles Whitelist** (if set)
4. **Role Validation** (if enabled)

## Configuration Examples

### Example 1: Force All Agents to Viewer Role

```yaml
security:
  enforced_role: "viewer"
```

**Result**: All agents get `viewer` role, regardless of what they claim.

### Example 2: Restrict to Specific Roles

```yaml
security:
  allowed_roles:
    - "viewer"
    - "developer"
  validate_roles: true
```

**Result**: Agents can only use `viewer` or `developer` roles, and roles must exist in the system.

### Example 3: Enforce Account Roles

```yaml
security:
  enforce_account_roles: true
  validate_roles: true
```

**Result**: 
- If agent provides `account_id`, they can only use roles assigned to that account
- All roles are validated against system role objects

### Example 4: Combined Enforcement

```yaml
security:
  # Don't enforce a single role, but restrict to whitelist
  allowed_roles:
    - "viewer"
    - "developer"
  # Validate roles exist
  validate_roles: true
  # If account provided, enforce account's roles
  enforce_account_roles: true
```

**Result**: 
- Agents can only use `viewer` or `developer`
- Roles must exist in system
- If account_id provided, must match account's roles

## Error Handling

### Role Not Allowed

If an agent claims a role not in `allowed_roles`:

```
Error: Role enforcement failed: none of the provided roles are allowed. Allowed roles: [viewer, developer]
```

### Role Doesn't Exist

If `validate_roles: true` and agent claims a role that doesn't exist:

```
Error: Role validation failed: role 'invalid_role' does not exist in the system
```

### Account Not Found

If `enforce_account_roles: true` and account doesn't exist:

```
Error: Role enforcement failed: account not found: account:unknown
```

## Implementation Details

### Files

- `pkg/mcp/role_enforcement.go` - Role enforcement logic
- `pkg/mcp/server_handlers.go` - Applies enforcement during initialization
- `.zqk/mcp/config.yaml` - Configuration

### Functions

- `enforceRoleEnforcement()` - Applies all enforcement rules
- `getAccountRoles()` - Loads account object and extracts roles
- `validateRolesAgainstSystem()` - Validates roles against system role objects

## Best Practices

1. **Start Restrictive**: Use `enforced_role: "viewer"` initially, then relax as needed
2. **Use Account-Based Enforcement**: If you have account objects, use `enforce_account_roles: true`
3. **Validate Roles**: Always enable `validate_roles: true` to ensure roles exist
4. **Whitelist Approach**: Use `allowed_roles` instead of blacklisting (more secure)

## Migration Path

1. **Phase 1**: Enable `validate_roles: true` (safest, just validates)
2. **Phase 2**: Add `allowed_roles` whitelist (restricts to specific roles)
3. **Phase 3**: Enable `enforce_account_roles: true` (if accounts exist)
4. **Phase 4**: Consider `enforced_role` if you want all agents to have same role

## Security Considerations

- **Enforced Role**: Most secure but least flexible
- **Account-Based**: Good balance of security and flexibility
- **Allowed Roles**: Flexible but requires careful whitelist management
- **Role Validation**: Prevents typos and invalid roles

## Conclusion

Role enforcement provides multiple mechanisms to control which roles AI agents can use, from strict enforcement to flexible whitelisting. Choose the approach that best fits your security requirements.

