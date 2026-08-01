# Observer Agent Privileges Update v1.0

**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Document the addition of `write:account` permission to observer agent role to enable dynamic agent registration

## Change Summary

**Change:** Added `write:account` permission to observer agent role  
**Impact:** Enables dynamic agent registration

## Changes Made

### File: `docs/architecture/roles/ROL-001.yaml`

**Added Permission:**
- `write:account` - Allows observer agent to create account objects

**Updated Description:**
- Added mention of "dynamic agent registration (creating account objects for new agents)"

**Updated Timestamp:**
- `updated_at: "2026-01-01T00:00:00Z"`

## New Permissions List

The observer agent now has these permissions:

1. `read:*` - Read access to all objects
2. `read:code` - Read code files
3. `read:ast` - Read AST (Abstract Syntax Tree) data
4. `write:graph_node` - Create/update graph nodes
5. `write:graph_edge` - Create/update graph edges
6. `read:graph_node` - Read graph nodes
7. `read:graph_edge` - Read graph edges
8. `write:account` - **NEW:** Create account objects for dynamic registration

## Dynamic Registration Workflow

Now that observer agent has `write:account` permission, it can:

### Step 1: Detect New Agent
Observer agent detects when a new agent connects without an account.

### Step 2: Create Account Object
Observer agent creates an account file:
```bash
zqk object create account \
  --file docs/architecture/accounts/account-new-agent.yaml \
  --context ai-agent
```

Example account file structure:
```yaml
id: account:new_agent
kind: account
roles:
  - appropriate_role
status: active
```

### Step 3: Agent Re-initializes
New agent re-initializes with the account_id:
```json
{
  "capabilities": {
    "account_id": "account:new_agent"
  }
}
```

### Step 4: System Applies Permissions
- System looks up `docs/architecture/accounts/account-new-agent.yaml`
- Finds account file (created by observer agent)
- Derives permissions from account's roles
- Applies permissions to agent
- Agent can now operate with proper permissions ✅

## Benefits

✅ **Dynamic Registration**: New agents can be registered without manual config updates  
✅ **Automated Onboarding**: Observer agent handles agent registration automatically  
✅ **No Config Changes**: Account files work immediately (no config.yaml updates needed)  
✅ **Secure**: Permissions still come from role objects (not client claims)  

## Security Considerations

- Observer agent can only create account objects (not modify existing ones without additional permissions)
- Permissions are still derived from role objects (secure source of truth)
- Account files are version-controlled (audit trail)
- Role enforcement still applies (account roles must match system roles)

## Next Steps

1. **Test Dynamic Registration**:
   - Have observer agent create an account for a new agent
   - Verify new agent can connect and get proper permissions

2. **Monitor Account Creation**:
   - Track account objects created by observer agent
   - Ensure proper role assignment

3. **Consider Additional Permissions** (if needed):
   - `update:account` - If observer agent needs to modify existing accounts
   - `delete:account` - If observer agent needs to remove accounts (unlikely)

## Related Files

- `docs/architecture/roles/ROL-001.yaml` - Observer agent role (updated)
- `docs/architecture/accounts/account-observer-agent.yaml` - Observer agent account
- `.zqk/mcp/config.yaml` - MCP server configuration
- `pkg/mcp/role_enforcement.go` - Role enforcement logic (supports dynamic lookup)

