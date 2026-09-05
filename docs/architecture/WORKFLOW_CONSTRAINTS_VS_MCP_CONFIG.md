# Workflow Constraints vs MCP Config

**Last Verified:** 2026-08-31


**Status**: Design  
**Date**: 2026-01-05  
**Last Updated**: 2026-01-05  
**Purpose**: Clarify the relationship between workflow constraints and MCP config, and define integration strategy

**Note**: See [MCP Workflow Specialization](./MCP_WORKFLOW_SPECIALIZATION.md) for details on MCP workflows as a specialization of the workflow object.

## Overview

Workflow constraints provide a more flexible, granular, and business-logic-focused approach to role-based access control compared to MCP config's role enforcement. However, they serve different purposes and should be integrated rather than one replacing the other entirely.

## Comparison

### MCP Config (Infrastructure-Level)

**Purpose**: MCP server infrastructure and protocol-level access control

**Responsibilities**:
1. **Command Exposure** (Infrastructure)
   - `exposed_commands`: Which CLI commands are available via MCP
   - `blocked_commands`: Which commands are explicitly blocked
   - `write_operations`: Which write commands are enabled
   - **Scope**: Protocol-level (affects all MCP clients)

2. **Agent Registry** (Infrastructure)
   - `agent_registry`: Pre-registered agents with expected roles/profiles
   - `require_account_id`: Require agents to provide account_id
   - **Scope**: Agent connection management

3. **Authentication** (Infrastructure)
   - `auth_strategies`: Which authentication methods are enabled
   - **Scope**: Authentication flow

4. **Role Enforcement** (Can be replaced by workflows)
   - `enforced_role`: Force all agents to use a specific role
   - `allowed_roles`: Whitelist of allowed roles
   - `validate_roles`: Validate roles against system role objects
   - `enforce_account_roles`: Enforce roles match account's assigned roles
   - **Scope**: Global role restrictions (applies to all MCP clients)

5. **Infrastructure Settings**
   - Binary paths, versions, compatibility
   - Format restrictions, idle timeout, trace logging
   - **Scope**: Server configuration

### Workflow Constraints (Business Logic-Level)

**Purpose**: Workflow-specific, granular, object-kind-level access control

**Responsibilities**:
1. **Role-Based Constraints** (Business Logic)
   - `role_constraints`: Per-role restrictions (allowed/blocked operations and kinds)
   - **Scope**: Workflow-specific (can differ per priority plan/workstream)

2. **Object-Kind Constraints** (Business Logic)
   - `object_constraints`: Per-object-kind restrictions (required/blocked roles)
   - **Scope**: Workflow-specific (can differ per priority plan/workstream)

3. **Operation-Level Constraints** (Business Logic)
   - `allowed_operations`: Which operations (create, update, delete) are allowed
   - `blocked_kinds`: Which object kinds are blocked for a role
   - **Scope**: Workflow-specific (can differ per priority plan/workstream)

4. **Workflow Scoping**
   - `applicable_roles`: Which roles can use this workflow
   - `applicable_accounts`: Which accounts can use this workflow
   - `enabled`: Whether workflow is active
   - **Scope**: Workflow-specific

## Integration Strategy

### Phase 1: Coexistence (Current State)

**MCP Config** handles:
- Command exposure (infrastructure)
- Agent registry (infrastructure)
- Authentication (infrastructure)
- Global role enforcement (legacy, can be deprecated)

**Workflow Constraints** handle:
- Workflow-specific role restrictions
- Object-kind-level restrictions
- Operation-level restrictions
- Priority plan/workstream scoping

### Phase 2: Integration (Recommended)

**MCP Config** should:
- Keep infrastructure concerns (command exposure, agent registry, authentication)
- **Remove** global role enforcement (`enforced_role`, `allowed_roles`)
- **Add** workflow-aware command filtering

**Workflow Constraints** should:
- Continue handling workflow-specific restrictions
- **Extend** to influence MCP command exposure (if workflow is active for the agent)
- **Extend** to provide default constraints when no workflow is active

### Phase 3: Workflow-Driven (Future)

**MCP Config** becomes minimal:
- Only infrastructure concerns (command exposure patterns, agent registry, authentication)
- No role enforcement (delegated to workflows)

**Workflow Constraints** become primary:
- All role-based restrictions defined in workflows
- MCP server queries active workflows for the agent's role
- Command exposure influenced by workflow constraints
- Default workflow provides baseline restrictions

## Benefits of Workflow-Driven Approach

1. **Granularity**: Constraints can differ per workflow (e.g., "Feature Development Workflow" vs "Bug Fix Workflow")
2. **Flexibility**: Workflows can be enabled/disabled, scoped to roles/accounts
3. **Business Logic**: Constraints are defined where they're used (workflows), not in infrastructure config
4. **Less Coupling**: No tight coupling between MCP config and business rules
5. **Dynamic**: Workflows can be created/modified without changing MCP config
6. **Context-Aware**: Constraints apply based on active priority plan/workstream

## Migration Path

### Step 1: Keep MCP Config for Infrastructure
- Maintain command exposure, agent registry, authentication
- Keep global role enforcement as fallback

### Step 2: Add Workflow Integration to MCP
- When agent connects, determine active workflow(s) based on:
  - Agent's role (from `applicable_roles`)
  - Agent's account (from `applicable_accounts`)
  - Active priority plans (if agent is working on a plan)
- Apply workflow constraints to MCP command filtering

### Step 3: Deprecate MCP Role Enforcement
- Mark `enforced_role`, `allowed_roles` as deprecated
- Use workflow constraints instead
- Provide migration guide

### Step 4: Make Workflows Primary
- Remove MCP role enforcement entirely
- All role restrictions come from workflows
- Default workflow provides baseline restrictions

## Example: Workflow-Driven MCP Access

```yaml
# Workflow defines constraints
id: WFL-001
kind: workflow
title: Feature Development Workflow
applicable_roles: [developer, owner]
constraints:
  role_constraints:
    developer:
      allowed_operations: [create, update]
      allowed_kinds: [backlog_item, requirement, test_case]
      blocked_kinds: [priority_plan, milestone]
    owner:
      allowed_operations: [create, update, delete]
      allowed_kinds: [backlog_item, requirement, milestone, priority_plan]

# MCP Config becomes minimal
mcp_server:
  exposed_commands:
    - "object *"  # All object commands (filtered by workflow)
    - "system status"
  blocked_commands:
    - "system git *"  # Infrastructure-level blocks
  security:
    # No role enforcement - workflows handle it
    validate_roles: true  # Still validate roles exist
    enforce_account_roles: true  # Still enforce account roles match
```

## Implementation Notes

1. **MCP Server Integration**:
   - When agent connects, query workflows for `applicable_roles` matching agent's role
   - If multiple workflows match, use most restrictive constraints (intersection)
   - If no workflow matches, use default workflow or deny access

2. **Command Filtering**:
   - MCP config still defines which commands are exposed (infrastructure)
   - Workflow constraints filter which commands the agent can actually use (business logic)
   - Example: `object create` is exposed, but workflow blocks `create` for `milestone` objects

3. **Backward Compatibility**:
   - Keep MCP config role enforcement as fallback
   - If workflow constraints are not available, use MCP config
   - Gradually migrate to workflow-driven approach

## Conclusion

**Workflow constraints should replace MCP config's role enforcement**, but MCP config should retain infrastructure concerns. The integration provides:

- **Better separation of concerns**: Infrastructure (MCP config) vs Business Logic (workflows)
- **More flexibility**: Workflow-specific constraints without config changes
- **Less coupling**: Business rules not tied to infrastructure config
- **Better maintainability**: Constraints defined where they're used

The recommended approach is **Phase 2: Integration**, where workflows influence MCP access control while MCP config handles infrastructure.

