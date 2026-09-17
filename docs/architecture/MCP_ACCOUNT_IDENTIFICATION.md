# MCP Account Identification and Role Enforcement

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-01-02  
**Status:** Active  
**Purpose:** Document how agents are prompted for account_id (account identification) and how role enforcement ensures agents only have access to their assigned roles

## Overview

The MCP server now proactively prompts agents to provide their `account_id` when `require_account_id: true` is configured. This account identification process ensures each agent uses the correct account and receives only the roles and permissions assigned to that account.

## Account Identification

### When Account Identification Occurs

Account identification is triggered when:
- ✅ `require_account_id: true` is set in `.zqk/mcp/config.yaml`
- ✅ Agent registry is configured (has entries)
- ✅ No `account_id` is provided in the initialize request
- ✅ Registry lookup by client name fails
- ✅ Client is NOT a human IDE (human clients can use credentials instead)

### Account Identification Process

1. **Registry Lookup First**: The server attempts to find the account by matching the client name (e.g., `cursor-vscode` → `account:cursor-vscode`)

2. **If Lookup Fails**: The server prompts for `account_id` with:
   - **Required parameter**: `account_id` (string)
   - **Available choices**: List of all account IDs from the agent registry
   - **Description**: Explains that account_id determines role and permissions

3. **Alternative Options**: Credentials (keystore key, username/password) are also offered as alternative authentication methods

### Example Account Identification Response

When the server prompts for account_id, the agent should respond by re-initializing with:

```json
{
  "method": "initialize",
  "params": {
    "capabilities": {
      "account_id": "account:coder_agent"
    },
    "clientInfo": {
      "name": "my-agent",
      "version": "1.0.0"
    }
  }
}
```

### Available Account IDs

The server provides a list of available account IDs from the agent registry. Common accounts include:
- `account:coder_agent` - Code generation and modification
- `account:test_agent` - Test execution and validation
- `account:observer_agent` - Monitoring and observation
- `account:cursor-vscode` - Cursor IDE client (developer role)
- `account:developer` - Developer account
- `account:viewer` - Read-only access

## Role Enforcement

### How It Works

When `enforce_account_roles: true` is enabled, the system ensures agents can **only** use roles assigned to their account:

1. **Account File Lookup**: System looks up the account file at `.zqk/process/accounts/account-{username}.yaml`

2. **Role Filtering**: 
   - If agent claims roles, only roles that match the account's assigned roles are allowed
   - If agent claims roles NOT in the account, those roles are **filtered out**
   - If no matching roles, the account's roles are used instead

3. **Permission Derivation**:
   - Permissions are derived from roles (via role objects in `.zqk/process/roles/`)
   - Agents **cannot** claim permissions directly - permissions come from roles
   - If account has roles, permissions are derived from those roles
   - If account has no roles, explicit account permissions are required

### Security Guarantees

✅ **Agents cannot claim roles they don't have**: Role filtering ensures only account-assigned roles are used

✅ **Agents cannot claim permissions directly**: Permissions are always derived from roles or account, never from agent claims

✅ **Account files are authoritative**: Account files in `.zqk/process/accounts/` are the source of truth for roles

✅ **Dynamic registration supported**: Observer agent can create account files, and they work immediately without config updates

### Example: Role Enforcement in Action

**Scenario**: Agent tries to use `account:coder_agent` but claims `roles: ["admin", "coder_agent"]`

**Account File** (`account-coder-agent.yaml`):
```yaml
roles:
  - coder_agent
```

**Result**:
- ✅ `coder_agent` role is allowed (matches account)
- ❌ `admin` role is filtered out (not in account)
- ✅ Final roles: `["coder_agent"]`
- ✅ Permissions: Derived from `coder_agent` role object (not admin permissions)

## Configuration

### Required Settings

In `.zqk/mcp/config.yaml`:

```yaml
security:
  require_account_id: true              # Require all agents to provide account_id
  enforce_account_roles: true          # Enforce roles match account's assigned roles
  agent_registry:                       # Map of account IDs to expected configuration
    "account:coder_agent":
      account_id: "account:coder_agent"
      roles:
        - "coder_agent"
      profile: "ai-agent"
      required: true
```

### Account Files

Each account must have a file at `.zqk/process/accounts/account-{username}.yaml`:

```yaml
id: account:coder_agent
kind: account
roles:
  - coder_agent
status: active
```

### Role Files

Each role must have a file at `.zqk/process/roles/ROL-XXX.yaml`:

```yaml
id: ROL-001
kind: role
role_id: coder_agent
permissions:
  - read:*
  - write:code
  - write:test_case
status: active
```

## Benefits

1. **Explicit Account Identity**: Agents must explicitly identify themselves
2. **Strict Role Enforcement**: Agents can only use roles assigned to their account
3. **Clear Permission Model**: Permissions come from roles, not agent claims
4. **Dynamic Registration**: New accounts can be created without config updates
5. **Audit Trail**: Clear record of which agent has which role

## Implementation Details

- **Elicitation**: Implemented in `pkg/mcp/server_handlers.go` in `handleInitialize` function
- **Role Enforcement**: Implemented in `pkg/mcp/role_enforcement.go` in `enforceRoleEnforcement` function
- **Account Lookup**: Implemented in `pkg/mcp/role_enforcement.go` in `getAccountRolesAndPermissions` function

## Troubleshooting

### Agent Not Getting Developer Role

**Issue**: Agent shows as `viewer` instead of `developer`

**Causes**:
1. Account ID not provided in initialize request
2. Registry lookup by client name failed
3. Account file not found or doesn't have `developer` role

**Solutions**:
1. Provide `account_id` in initialize request capabilities
2. Check that client name matches registry entry
3. Verify account file exists and has correct roles
4. Check MCP server logs for account lookup failures

### Account Lookup Failed

**Issue**: Server sends `account_lookup_failed` notification

**Solutions**:
1. Create account file: `.zqk/process/accounts/account-{username}.yaml`
2. Ensure account file has correct roles assigned
3. Re-initialize with the account_id after creating the file

## Related Documentation

- [MCP Role Elicitation](./MCP_ROLE_ELICITATION.md) - Role/credentials elicitation (different from account identification)
- [MCP Agent Registry](./MCP_AGENT_REGISTRY.md) - Agent registry configuration
- [MCP Built-In Tools](./MCP_BUILT_IN_TOOLS.md) - Available MCP tools

## Terminology

- **Account Identification**: The process of identifying which account an agent wants to use (via `account_id`)
- **Authentication**: The process of proving you are that account (via credentials like keystore keys, username/password)
- **Authorization**: The process of determining what you can do (via roles and permissions)

Account identification is distinct from authentication - it's about selecting an account, not proving identity.

