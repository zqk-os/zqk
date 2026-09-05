# MCP Agent Registry

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Document the agent registry system for pre-registering agents and enforcing expected roles

## Overview

The agent registry allows you to **pre-register agents** with expected roles and profiles before they connect. This ensures that:

1. **You know what role each agent will use** before they connect
2. **Agents are isolated** by role and profile
3. **Multi-agent scenarios** are properly managed
4. **Access is limited** based on role and profile

## How It Works

### 1. Pre-Registration

Agents are registered in `.zqk/mcp/config.yaml` under `security.agent_registry`:

```yaml
security:
  require_account_id: true  # Require all agents to provide account_id
  agent_registry:
    "account:coder_agent":    # Key: account_id or client_id
      account_id: "account:coder_agent"
      roles:
        - "coder_agent"
      profile: "ai-agent"     # Expected profile
      required: true           # Must be in registry to connect
    "account:test_agent":
      account_id: "account:test_agent"
      roles:
        - "test_agent"
      profile: "ai-agent"
      required: true
```

### 2. Agent Registration Validation

When an agent connects:

1. **Extract identifiers**: System extracts `account_id` and `client_id` from the initialize request
2. **Lookup in registry**: System looks up the agent by `account_id` (preferred) or `client_id`
3. **Validation**:
   - If `require_account_id: true` and no `account_id` provided → **REJECT**
   - If agent not in registry and registry has `required: true` agents → **REJECT**
   - If agent in registry → **ENFORCE** expected roles/profile

### 3. Role Enforcement

If an agent is found in the registry:

- **Roles are enforced** from the registry (overrides agent-provided roles)
- **Account ID is enforced** from the registry (if specified)
- **Profile is determined** from registry or role mapping

### 4. Profile Mapping

Profiles are automatically mapped from roles if not explicitly specified:

- `admin`, `founder` → `mcp` profile (full access)
- `executive`, `owner`, `developer`, `viewer` → `ai-agent` profile
- `observer_agent`, `test_agent`, `coder_agent`, `collective` → `ai-agent` profile

## Configuration

### Basic Setup

```yaml
security:
  require_account_id: true
  agent_registry:
    "account:coder_agent":
      account_id: "account:coder_agent"
      roles:
        - "coder_agent"
      profile: "ai-agent"
      required: true
```

### Multiple Agents

```yaml
security:
  require_account_id: true
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
    "account:observer_agent":
      account_id: "account:observer_agent"
      roles:
        - "observer_agent"
      profile: "ai-agent"
      required: true
```

### Optional Registration

If you want to allow unregistered agents but still enforce roles for registered ones:

```yaml
security:
  require_account_id: false  # Allow agents without account_id
  agent_registry:
    "account:coder_agent":
      account_id: "account:coder_agent"
      roles:
        - "coder_agent"
      profile: "ai-agent"
      required: false  # Not required, but if registered, enforce roles
```

## Enforcement Order

Role enforcement is applied in this order:

1. **Agent Registry** (if agent is registered) - **HIGHEST PRIORITY**
2. **Enforced Role** (if `enforced_role` is set)
3. **Account-Based Enforcement** (if `enforce_account_roles: true`)
4. **Allowed Roles Whitelist** (if `allowed_roles` is set)
5. **Role Validation** (if `validate_roles: true`)

## Multi-Agent Isolation

### Client-Specific Trace Logs

Each agent gets its own trace log file:
- `mcp-trace-<role>_client_<timestamp>_<random>.log`
- Example: `mcp-trace-coder_agent_client_1234567890_abc123.log`

### Role-Based Access Control

Agents are isolated by:
- **Roles**: Each agent can only access commands allowed for their role
- **Permissions**: Each agent has permissions based on their role
- **Profiles**: Each agent uses the appropriate profile (ai-agent, mcp, etc.)

### Account-Based Enforcement

If `enforce_account_roles: true` is also enabled:
- Agent's roles must match the account object's roles
- Registry roles take precedence, but account validation ensures consistency

## Error Messages

### Agent Not Registered

```
Error: Agent registration validation failed: agent not found in registry. 
Please register your agent (account_id: account:unknown, client_id: unknown-client) in the MCP config
```

### Account ID Required

```
Error: Agent registration validation failed: account_id is required but not provided. 
Please provide your account_id in the initialize request
```

## Best Practices

1. **Pre-Register All Agents**: Register all agents before they connect
2. **Use Account Objects**: Create account objects for each agent with appropriate roles
3. **Require Account ID**: Set `require_account_id: true` to enforce explicit identification
4. **Mark Required**: Set `required: true` for agents that must be registered
5. **Use Role Mapping**: Let the system map roles to profiles automatically
6. **Combine with Account Enforcement**: Use `enforce_account_roles: true` for additional validation

## Example Workflow

### Step 1: Create Account Object

```yaml
# docs/process/accounts/account-coder-agent.yaml
id: account:coder_agent
kind: account
roles:
  - coder_agent
status: active
```

### Step 2: Register Agent in Config

```yaml
# .zqk/mcp/config.yaml
security:
  require_account_id: true
  enforce_account_roles: true
  agent_registry:
    "account:coder_agent":
      account_id: "account:coder_agent"
      roles:
        - "coder_agent"
      profile: "ai-agent"
      required: true
```

### Step 3: Agent Connects

Agent sends initialize request:
```json
{
  "method": "initialize",
  "params": {
    "capabilities": {
      "account_id": "account:coder_agent",
      "client_id": "coder-agent-1"
    }
  }
}
```

### Step 4: System Validates

1. ✅ Agent found in registry
2. ✅ Account ID matches registry
3. ✅ Roles enforced from registry: `["coder_agent"]`
4. ✅ Profile determined: `"ai-agent"`
5. ✅ Account object validated (if `enforce_account_roles: true`)

## Integration with Other Features

### Role Elicitation

If an agent is not registered and `require_account_id: false`:
- System will elicit roles/permissions
- Agent can provide roles, which will be validated against `allowed_roles` if set

### Account-Based Enforcement

If `enforce_account_roles: true`:
- Registry roles are enforced first
- Then account object is validated
- Agent's roles must match both registry and account object

### Role Validation

If `validate_roles: true`:
- Registry roles are validated against system role objects
- Ensures roles exist in `docs/process/roles/`

## Conclusion

The agent registry provides a robust mechanism for pre-registering agents and ensuring they use the correct roles and profiles. This is essential for multi-agent scenarios where different agents need different access levels and isolation.

