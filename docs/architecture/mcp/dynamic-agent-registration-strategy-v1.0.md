# Dynamic Agent Registration Strategy v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Detailed explanation of separate account files approach and hybrid strategy for dynamic agent registration

## Option 3: Separate Account Files (Detailed)

### How It Works

Instead of all agents using the same `account:cursor-vscode`, each agent type gets its own account file:

```
docs/process/accounts/
  ├── account-coder-agent.yaml      ✅ Already exists
  ├── account-test-agent.yaml       ✅ Already exists
  ├── account-observer-agent.yaml   ✅ Already exists
  └── account-cursor-vscode.yaml    (for human IDE clients only)
```

### Step-by-Step Implementation

#### 1. **Account Files Already Exist** ✅

The account files are already created:
- `account-coder-agent.yaml` - has `coder_agent` role
- `account-test-agent.yaml` - has `test_agent` role  
- `account-observer-agent.yaml` - has `observer_agent` role

#### 2. **Each Agent Provides Unique Account ID**

When agents connect, they specify their account_id:

```json
// Coder Agent
{
  "capabilities": {
    "account_id": "account:coder_agent"
  }
}

// Test Agent
{
  "capabilities": {
    "account_id": "account:test_agent"
  }
}

// Observer Agent
{
  "capabilities": {
    "account_id": "account:observer_agent"
  }
}
```

#### 3. **Config Registry Maps Account IDs**

The config already has the registry entries (lines 120-137):

```yaml
agent_registry:
  "account:coder_agent":
    account_id: "account:coder_agent"
    roles: ["coder_agent"]
    profile: "ai-agent"
    required: true
  "account:test_agent":
    account_id: "account:test_agent"
    roles: ["test_agent"]
    profile: "ai-agent"
    required: true
  "account:observer_agent":
    account_id: "account:observer_agent"
    roles: ["observer_agent"]
    profile: "ai-agent"
    required: true
```

#### 4. **Account File Lookup**

When `enforce_account_roles: true`:
1. Agent provides `account_id: "account:coder_agent"`
2. System looks up `docs/process/accounts/account-coder-agent.yaml`
3. Reads roles from file: `["coder_agent"]`
4. Looks up role permissions: `read:*`, `write:*` (from `ROL-XXX.yaml`)
5. Applies permissions to agent

### Benefits of Option 3

✅ **Isolation**: Each agent type has its own account  
✅ **Security**: Permissions come from account files (not client claims)  
✅ **Scalability**: Easy to add new agent types  
✅ **Auditability**: Account files are version-controlled  
✅ **Flexibility**: Can modify permissions per agent type  

### Current Status

**Good News**: All the pieces are already in place! The account files exist, the registry is configured, and the roles have the right permissions.

**The Issue**: Agents aren't providing their `account_id` in the initialize request, so they all default to `account:cursor-vscode`.

---

## Dynamic Registration: Observer Agent Workflow

### Vision

Eventually, you want:
1. New agents connect without pre-registration
2. Observer agent detects new agent
3. Observer agent creates account file for new agent
4. Observer agent updates config registry (or uses dynamic registry)
5. New agent gets appropriate permissions

### How It Could Work

#### Phase 1: Observer Agent Creates Account Objects

The observer agent has permissions to:
- `read:*` - Can read existing accounts
- `write:graph_node` - Can create graph nodes
- But currently **cannot** create account objects directly

**Solution**: Give observer agent permission to create account objects:

```yaml
# In observer_agent role (ROL-XXX.yaml)
permissions:
  - read:*
  - write:graph_node
  - write:graph_edge
  - write:account  # NEW: Allow creating account objects
```

#### Phase 2: Observer Agent Workflow

```
1. New Agent Connects
   └─> Provides: client_name, requested_role, capabilities
   
2. Observer Agent Detects New Agent
   └─> Uses: zqk object list --kind account
   └─> Checks: Does account exist for this agent?
   
3. If Account Doesn't Exist:
   └─> Observer Agent Creates Account:
       zqk object create account \
         --file /tmp/new-agent-account.yaml \
         --context ai-agent
   
4. Observer Agent Updates Registry (or uses dynamic lookup)
   └─> Option A: Updates config.yaml (requires file write)
   └─> Option B: Uses account file directly (no config change needed)
   
5. New Agent Re-initializes with account_id
   └─> Gets permissions from account file
   └─> Can now operate with proper permissions
```

#### Phase 3: Dynamic Registry Lookup

Instead of requiring pre-registration in `config.yaml`, the system could:

1. **Check account file first** (if `enforce_account_roles: true`)
2. **If account file exists**, use it (even if not in config registry)
3. **If account file doesn't exist**, fall back to:
   - Registry lookup (if in config)
   - Elicitation (ask agent for credentials)
   - Default to viewer (read-only)

This would allow:
- Pre-registered agents: Use config registry (current behavior)
- Dynamically created accounts: Use account files directly
- Unknown agents: Elicit or default to viewer

---

## Hybrid Approach: Best of Both Worlds

### Immediate Fix (Now)

**Use Option 1 (Unique Account IDs)** - Quick and works immediately:

1. Each agent provides `account_id` in initialize request
2. Account files already exist ✅
3. Registry already configured ✅
4. Permissions work correctly ✅

**Implementation**:
- Update your agent configurations to pass `account_id`
- No code changes needed
- No config changes needed (already set up)

### Future Enhancement (Later)

**Add Dynamic Registration Support** - Enables observer agent workflow:

1. **Extend Observer Agent Permissions**:
   ```yaml
   # In ROL-XXX.yaml for observer_agent
   permissions:
     - write:account  # Allow creating account objects
   ```

2. **Add Dynamic Registry Lookup**:
   - If account file exists → use it (even if not in config)
   - If account file doesn't exist → check registry → elicit → default

3. **Observer Agent Can Create Accounts**:
   ```bash
   # Observer agent creates account for new agent
   zqk object create account \
     --file docs/process/accounts/account-new-agent.yaml \
     --context ai-agent
   ```

4. **New Agent Re-initializes**:
   - Provides `account_id: "account:new_agent"`
   - System finds account file
   - Gets permissions from account file
   - Works immediately (no config update needed)

### Code Changes Needed for Dynamic Registration

#### 1. Update Role Enforcement Logic

```go
// In pkg/mcp/role_enforcement.go
func enforceRoleEnforcement(...) {
  // Current: Only checks config registry
  // New: Check account file first, then registry
  
  if config.MCPServer.Security.EnforceAccountRoles && accountID != "" {
    // Try account file lookup first
    accountRoles, accountPerms, err := getAccountRolesAndPermissions(accountID, projectRoot)
    if err == nil {
      // Account file exists - use it (even if not in config registry)
      // ... apply permissions ...
      return clientInfo, nil
    }
    // Account file doesn't exist - continue with registry lookup
  }
}
```

#### 2. Add Observer Agent Permission

```yaml
# In docs/process/roles/ROL-XXX.yaml (observer_agent role)
permissions:
  - read:*
  - write:graph_node
  - write:graph_edge
  - write:account  # NEW
```

#### 3. Optional: Dynamic Config Updates

If you want observer agent to update config.yaml:

```go
// New function: Update agent registry in config
func updateAgentRegistry(configPath string, accountID string, roles []string) error {
  // Read config
  // Add/update registry entry
  // Write config
  // Reload server config (or require restart)
}
```

**Note**: This is more complex and may not be necessary if you use account file lookup directly.

---

## Recommended Path Forward

### Step 1: Immediate Fix (Today)

**Use unique account IDs** - Update your agent configs:

```yaml
# In each agent's MCP client config
capabilities:
  account_id: "account:coder_agent"  # or test_agent, observer_agent
```

This solves the immediate problem with zero code changes.

### Step 2: Enable Dynamic Lookup (Next)

**Modify role enforcement** to check account files even if not in config registry:

- Allows observer agent to create accounts
- New agents can use account files directly
- No config.yaml updates needed for new agents

### Step 3: Observer Agent Registration (Future)

**Give observer agent permission to create accounts**:

- Observer agent can create account objects
- Observer agent can register new agents
- Full dynamic registration workflow

---

## Technical Details

### Current Account File Structure

```yaml
# account-coder-agent.yaml
id: account:coder_agent
kind: account
roles:
  - coder_agent
status: active
```

### Current Role Structure

```yaml
# ROL-XXX.yaml (coder_agent role)
role_id: coder_agent
permissions:
  - read:*
  - write:*
```

### Permission Resolution Flow

```
1. Agent provides: account_id: "account:coder_agent"
2. System looks up: docs/process/accounts/account-coder-agent.yaml
3. Reads roles: ["coder_agent"]
4. Looks up role: docs/process/roles/ROL-XXX.yaml
5. Gets permissions: ["read:*", "write:*"]
6. Applies to agent
```

### With Dynamic Registration

```
1. New agent connects: no account_id
2. Observer agent detects: new agent
3. Observer agent creates: account file
4. New agent re-initializes: with account_id
5. System looks up: account file (exists now)
6. Gets permissions: from account file
```

---

## Summary

**Option 3 (Separate Account Files)** is already implemented! The account files exist, the registry is configured, and the roles have permissions.

**The immediate fix** is simple: Each agent needs to provide its `account_id` in the initialize request.

**The future enhancement** (dynamic registration) requires:
1. Extending observer agent permissions
2. Modifying role enforcement to check account files first
3. Observer agent workflow to create accounts

**Recommended approach**: Start with unique account IDs (immediate fix), then add dynamic lookup support (enables future dynamic registration without breaking current setup).

## Related Documentation

- [Multi-Agent Permission Issue Analysis](./multi-agent-permission-issue-analysis-v1.0.md)
- [Observer Agent Workflow Clarification](./observer-agent-workflow-clarification-v1.0.md)
- [Observer Agent Privileges Update](./observer-agent-privileges-update-v1.0.md)
- [MCP Multi-Agent Orchestration](../MCP_MULTI_AGENT_ORCHESTRATION.md)
- [MCP Agent Registry](../MCP_AGENT_REGISTRY.md)
- [Automatic Agent Registration Planning](../../planning/automatic-agent-registration-planning-v1.0.md)

