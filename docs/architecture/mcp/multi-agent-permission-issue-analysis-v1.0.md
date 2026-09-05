# Multi-Agent Permission Issue Analysis v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Analysis of multi-agent permission issues when connecting to MCP server

## Issue Summary

**Issue:** Multiple agents unable to get writable privileges when connecting to MCP server  
**Root Cause:** All agents using the same client name (`cursor-vscode`) causing account resolution conflicts

## Problem Summary

When multiple agents connect to the MCP server simultaneously, none of them are able to get writable privileges, even though:
- The account file exists (`account-cursor-vscode.yaml`)
- The account has the `developer` role
- The `developer` role has `read:*` and `write:*` permissions
- The registry is configured correctly

## Root Cause Analysis

### 1. Client Name Collision

All agents are connecting with the same `clientInfo.name: "cursor-vscode"`. This causes:

1. **Registry Lookup Conflict**: All agents match the same registry entry `"account:cursor-vscode"` (line 170-175 in config.yaml)
2. **Account ID Resolution**: All agents get the same `account_id: "account:cursor-vscode"`
3. **Account File Lookup**: When `enforce_account_roles: true` is set, the server looks up `docs/process/accounts/account-cursor-vscode.yaml`
4. **Permission Derivation**: Permissions are derived from the account's roles, which should work correctly

### 2. Configuration Settings

Current config has:
- `enforce_account_roles: true` (line 113) - Enforces that permissions come from account files
- `require_account_id: true` (line 117) - Requires all agents to provide account_id
- Registry entry for `"account:cursor-vscode"` with `developer` role

### 3. What's Actually Happening

From the logs:
- Agents ARE getting `"permissions":"read:*,write:*"` during tool registration
- But client IDs show `viewer_client_...` prefix
- This suggests permissions are set correctly, but client ID generation happens before security context is fully established

## The Real Issue

The problem is that **all agents are using the same client name**, which means:

1. They all resolve to the same account (`account:cursor-vscode`)
2. When `enforce_account_roles: true` is enabled, the server tries to look up the account file
3. If the account lookup succeeds, permissions should be derived from roles correctly
4. **BUT**: If multiple agents are connecting simultaneously, there might be a race condition or the security context might not be properly isolated per connection

## Solutions

### Solution 1: Each Agent Must Provide Unique Account ID (RECOMMENDED)

Each agent should provide a unique `account_id` in their initialize request:

```json
{
  "capabilities": {
    "account_id": "account:coder_agent",  // or "account:test_agent", etc.
    "roles": ["coder_agent"]
  }
}
```

Then update the config to register each agent with their unique account:

```yaml
agent_registry:
  "account:coder_agent":
    account_id: "account:coder_agent"
    roles:
      - "coder_agent"
    profile: "ai-agent"
    required: true
  "account:test_agent":
    account_id: "account:test_agent"
    roles:
      - "test_agent"
    profile: "ai-agent"
    required: true
```

### Solution 2: Disable Account Role Enforcement (QUICK FIX)

If you want to use registry-based permissions instead of account file lookups:

```yaml
security:
  enforce_account_roles: false  # Use registry permissions instead
```

This will use the roles/permissions from the registry directly without looking up account files.

### Solution 3: Use Client ID for Account Resolution

Modify the code to use the unique client ID (which is already generated) instead of client name for account resolution. This would require code changes.

### Solution 4: Create Separate Account Files for Each Agent

Create separate account files for each agent type:
- `account-coder-agent.yaml`
- `account-test-agent.yaml`
- `account-observer-agent.yaml`

Then configure each agent to use their specific account_id.

## Recommended Fix

**Immediate Action**: Each agent must provide a unique `account_id` in their initialize request. The account_id should match an entry in the agent registry.

**Config Changes Needed**:

1. Ensure each agent type has a unique account_id in the registry
2. Each agent should pass their account_id in the initialize request
3. Keep `enforce_account_roles: true` if you want permissions from account files
4. Ensure account files exist for each agent type with appropriate roles

**Example Agent Initialize Request**:

```json
{
  "method": "initialize",
  "params": {
    "capabilities": {
      "account_id": "account:coder_agent",
      "roles": ["coder_agent"]
    },
    "clientInfo": {
      "name": "cursor-vscode",  // This can stay the same
      "version": "1.0.0"
    }
  }
}
```

## Verification

After applying the fix, verify:
1. Each agent gets a unique client ID with the correct role prefix
2. Each agent has `write:*` permissions in the tool registration logs
3. Each agent can successfully call write operations

## Files to Check

- `.zqk/mcp/config.yaml` - Agent registry configuration
- `docs/process/accounts/account-*.yaml` - Account files for each agent type
- `docs/process/roles/ROL-*.yaml` - Role definitions with permissions
- `.zqk/mcp/logs/mcp-trace.log` - Connection and permission logs

## Related Documentation

- [Dynamic Agent Registration Strategy](./dynamic-agent-registration-strategy-v1.0.md)
- [Observer Agent Workflow Clarification](./observer-agent-workflow-clarification-v1.0.md)
- [MCP Multi-Agent Orchestration](../MCP_MULTI_AGENT_ORCHESTRATION.md)
- [MCP Agent Registry](../MCP_AGENT_REGISTRY.md)

