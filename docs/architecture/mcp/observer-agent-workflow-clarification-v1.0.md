# Observer Agent Workflow Clarification v1.0

**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Clarify how observer agent creates accounts for other agents, including sequence and startup workflow

## Current State: What's Implemented vs. What's Not

### ✅ **What's Implemented (Capability Enabled)**

1. **Observer Agent Permission**: 
   - Observer agent has `write:account` permission ✅
   - Can create account objects using `zqk object create account` ✅

2. **Dynamic Account Lookup**:
   - Account files are checked FIRST (even if not in config registry) ✅
   - System can use account files created by observer agent ✅

3. **Account Files Exist**:
   - Pre-registered accounts: `account-coder-agent.yaml`, `account-test-agent.yaml`, `account-observer-agent.yaml` ✅

### ❌ **What's NOT Implemented (Workflow/Sequence)**

1. **Automatic Detection**: 
   - Observer agent doesn't automatically detect new agents connecting ❌
   - No event system to notify observer agent of new connections ❌

2. **Automatic Account Creation**:
   - No automatic workflow to create accounts for new agents ❌
   - Observer agent would need to manually detect and create accounts ❌

3. **Startup Sequence**:
   - No defined startup sequence for observer agent to register others ❌
   - No coordination mechanism between agents ❌

## How It Currently Works (Manual Process)

### Current Startup Sequence

```
1. MCP Server Starts
   └─> Loads config.yaml
   └─> Agent registry configured (coder, test, observer)

2. Observer Agent Connects
   └─> Provides: account_id: "account:observer_agent"
   └─> System looks up: docs/architecture/accounts/account-observer-agent.yaml
   └─> Gets permissions: read:*, write:graph_node, write:graph_edge, write:account ✅
   └─> Observer agent is ready

3. Other Agents Connect
   └─> Coder agent: account_id: "account:coder_agent" ✅ (pre-registered)
   └─> Test agent: account_id: "account:test_agent" ✅ (pre-registered)
   └─> System looks up account files → applies permissions ✅

4. New Agent Connects (Without Account)
   └─> Provides: account_id: "account:new_agent" (or no account_id)
   └─> System looks up: docs/architecture/accounts/account-new-agent.yaml
   └─> Account file NOT FOUND ❌
   └─> Falls back to registry → not found
   └─> Gets fallback permissions (viewer/read-only) ⚠️

5. Observer Agent Manually Creates Account (If Needed)
   └─> Observer agent calls: zqk object create account --file ...
   └─> Creates: docs/architecture/accounts/account-new-agent.yaml ✅
   └─> New agent re-initializes with account_id ✅
   └─> System finds account file → applies permissions ✅
```

## How It Could Work (Future Implementation)

### Proposed Startup Sequence

```
1. MCP Server Starts
   └─> Loads config.yaml
   └─> Agent registry configured

2. Observer Agent Connects First (Bootstrap)
   └─> Provides: account_id: "account:observer_agent"
   └─> Gets permissions including write:account ✅
   └─> Observer agent subscribes to connection events (if implemented)

3. Other Agents Connect
   └─> Agent connects without account_id (or with non-existent account)
   └─> System detects: account not found
   └─> System emits event: "agent_connection_attempted" (if implemented)
   └─> Observer agent receives event
   └─> Observer agent determines appropriate role for new agent
   └─> Observer agent creates account file
   └─> System notifies new agent: "Account created, please re-initialize"
   └─> New agent re-initializes with account_id
   └─> System finds account file → applies permissions ✅
```

## What Needs to Be Built

### Option 1: Event-Driven (Recommended)

**Components Needed:**

1. **Connection Event System**:
   ```go
   // When agent connects without account
   emitEvent("agent_connection_attempted", {
     client_id: "...",
     client_name: "...",
     requested_role: "...",
     account_id: "..."
   })
   ```

2. **Observer Agent Event Subscription**:
   ```go
   // Observer agent subscribes to connection events
   subscribeToEvent("agent_connection_attempted", handleNewAgent)
   ```

3. **Observer Agent Handler**:
   ```go
   func handleNewAgent(event) {
     // Determine appropriate role
     role := determineRole(event.client_name, event.requested_role)
     
     // Create account file
     createAccountFile(event.account_id, role)
     
     // Notify agent
     notifyAgent(event.client_id, "Account created, please re-initialize")
   }
   ```

### Option 2: Polling-Based (Simpler)

**Components Needed:**

1. **Observer Agent Periodic Check**:
   ```go
   // Observer agent periodically checks for unregistered agents
   func checkForNewAgents() {
     // Query MCP server for connected clients
     clients := getConnectedClients()
     
     for each client {
       if client.account_id == "" || accountFileNotFound(client.account_id) {
         // Create account for this client
         createAccountForClient(client)
       }
     }
   }
   ```

### Option 3: Manual/On-Demand (Current State)

**How It Works:**

1. New agent connects without account
2. Gets fallback permissions (viewer)
3. Human or observer agent manually creates account
4. New agent re-initializes with account_id
5. Gets proper permissions

## Recommended Approach

### Phase 1: Manual (Current - Works Now)

- Observer agent has permission ✅
- Manual account creation works ✅
- Agents can be registered on-demand ✅

### Phase 2: Event-Driven (Future)

- Implement connection event system
- Observer agent subscribes to events
- Automatic account creation for new agents
- Full dynamic registration workflow

## Current Configuration

**Observer Agent Config** (in `.zqk/mcp/config.yaml`):
```yaml
"account:observer_agent":
  account_id: "account:observer_agent"
  roles:
    - "observer_agent"
  profile: "ai-agent"
  required: true
```

**Observer Agent Role** (in `docs/architecture/roles/ROL-001.yaml`):
```yaml
permissions:
  - read:*
  - write:graph_node
  - write:graph_edge
  - write:account  # ✅ Can create accounts
```

## Summary

**Your Understanding**: Partially correct!

- ✅ Observer agent CAN create accounts (permission enabled)
- ✅ Account files work dynamically (lookup implemented)
- ❌ Observer agent does NOT automatically create accounts (workflow not implemented)
- ❌ No startup sequence for automatic registration (needs to be built)

**Current State**: Manual process - observer agent can create accounts, but needs to be triggered manually or via future event system.

**Next Steps**: 
1. Use manual process for now (works immediately)
2. Implement event system for automatic registration (future enhancement)

