# MCP Workflow Specialization

**Last Verified:** 2026-08-31


**Status**: Design  
**Date**: 2026-01-05  
**Purpose**: Define MCP workflows as a specialization of the workflow object for MCP server tool exposure control

## Overview

MCP workflows are a specialization of the general `workflow` object that applies MCP-specific context rules and controls which tools are exposed via the MCP server. They reuse all the constraint logic from general workflows while adding MCP-specific tool exposure rules.

## Architecture

### Workflow Object Hierarchy

```
workflow (base)
├── General Workflows (category: development, operations, planning, etc.)
│   └── Apply to priority plans/workstreams
│   └── Control object creation/manipulation constraints
│
└── MCP Workflows (category: mcp)
    └── Apply to MCP server tool registration
    └── Control CLI command/tool exposure
    └── Include mcp_config field for tool-specific rules
```

### MCP Workflow Fields

MCP workflows extend the base workflow object with:

1. **`category: mcp`** - Identifies this as an MCP-specific workflow
2. **`mcp_config`** - MCP-specific configuration:
   - `exposed_tools`: List of CLI command patterns to expose (whitelist)
   - `blocked_tools`: List of CLI command patterns to block (blacklist)
   - `tool_constraints`: Per-tool role requirements/restrictions

3. **Inherited from base workflow**:
   - `applicable_roles`: Which roles can use this MCP workflow
   - `applicable_accounts`: Which accounts can use this MCP workflow
   - `constraints`: Role-based and object-kind constraints (applies to tool execution)
   - `enabled`: Whether workflow is active

## MCP Server Integration

### Tool Registration Flow

```
MCP Server Initialize
  ↓
1. Query MCP Workflows (category: mcp)
   - Filter by applicable_roles (match agent's role)
   - Filter by applicable_accounts (match agent's account)
   - Filter by enabled: true
  ↓
2. Select Active MCP Workflow(s)
   - If multiple match: use most restrictive (intersection)
   - If none match: use default MCP workflow or deny access
  ↓
3. Apply MCP Workflow Constraints
   - exposed_tools: Whitelist of commands to register
   - blocked_tools: Blacklist of commands to exclude
   - tool_constraints: Per-tool role requirements
  ↓
4. Register Tools
   - Only register tools allowed by MCP workflow
   - Apply workflow constraints to tool execution
```

### Example MCP Workflow

```yaml
id: WFL-100
kind: workflow
title: Developer MCP Workflow
category: mcp
enabled: true
applicable_roles:
  - developer
  - coder_agent
applicable_accounts:
  - account:developer
  - account:coder_agent

# MCP-specific tool exposure rules
mcp_config:
  exposed_tools:
    - "object list"
    - "object get"
    - "object create"  # For backlog_item, requirement, test_case
    - "object update"  # For backlog_item, requirement, test_case
    - "system status"
    - "system check"
    - "system validate"
  blocked_tools:
    - "object delete"
    - "system git *"
    - "system sync"
    - "system init"
  tool_constraints:
    "object create":
      required_roles: [developer, coder_agent]
      blocked_roles: []
    "object create milestone":
      blocked_roles: [developer]  # Developers can't create milestones
    "object create priority_plan":
      blocked_roles: [developer]  # Developers can't create priority plans

# General workflow constraints (apply to tool execution)
constraints:
  role_constraints:
    developer:
      allowed_operations: [create, update]
      allowed_kinds: [backlog_item, requirement, test_case]
      blocked_kinds: [priority_plan, milestone]
```

## Benefits

1. **Unified Model**: MCP workflows are just workflows with `category: mcp`
2. **Reuses Constraint Logic**: All workflow constraint validation applies
3. **Context-Aware**: Different MCP workflows for different roles/accounts
4. **Flexible**: Can enable/disable, scope to roles, modify without config changes
5. **Less Coupling**: MCP tool exposure rules not tied to infrastructure config
6. **Dynamic**: Create/modify MCP workflows without restarting MCP server

## MCP Config vs MCP Workflow

### MCP Config (Infrastructure)
- **Purpose**: Server infrastructure settings
- **Scope**: Global (applies to all agents)
- **Examples**: Binary paths, authentication strategies, agent registry
- **When to use**: Infrastructure concerns that don't change per workflow

### MCP Workflow (Business Logic)
- **Purpose**: Tool exposure and access control
- **Scope**: Per-role/per-account (context-specific)
- **Examples**: Which tools to expose, role-based tool restrictions
- **When to use**: Business logic that varies by role/workflow

### Integration

MCP Config provides:
- Infrastructure settings (binary, auth, registry)
- Global command patterns (e.g., always block `system git *`)

MCP Workflow provides:
- Role-specific tool exposure
- Per-tool role requirements
- Workflow-specific constraints

**Both apply**: MCP Config (infrastructure) → MCP Workflow (business logic) → Tool Registration

## Migration Path

### Phase 1: Coexistence
- MCP Config continues to handle `exposed_commands`, `blocked_commands`
- MCP Workflows can override/add additional filtering
- Both systems work together

### Phase 2: MCP Workflow Primary
- MCP Config keeps infrastructure settings only
- MCP Workflows become primary for tool exposure
- MCP Config `exposed_commands` becomes fallback/default

### Phase 3: MCP Workflow Only
- Remove `exposed_commands`, `blocked_commands` from MCP Config
- All tool exposure controlled by MCP Workflows
- Default MCP Workflow provides baseline restrictions

## Implementation Notes

1. **Workflow Query**: MCP server queries workflows with `category: mcp` and matching `applicable_roles`/`applicable_accounts`

2. **Tool Filtering**: Use `mcp_config.exposed_tools` and `mcp_config.blocked_tools` to filter commands during registration

3. **Constraint Application**: Apply `constraints` from MCP workflow to tool execution (same as general workflows)

4. **Default Workflow**: If no MCP workflow matches, use a default workflow or deny access

5. **Multiple Workflows**: If multiple MCP workflows match, use intersection (most restrictive)

## Example: Multiple MCP Workflows

```yaml
# Developer MCP Workflow
id: WFL-100
category: mcp
applicable_roles: [developer]
mcp_config:
  exposed_tools: ["object list", "object get", "object create", "object update"]
  blocked_tools: ["object delete", "system git *"]

# PM MCP Workflow  
id: WFL-101
category: mcp
applicable_roles: [owner, product_manager]
mcp_config:
  exposed_tools: ["object *", "system status", "reports *"]
  blocked_tools: ["system git *"]

# Executive MCP Workflow
id: WFL-102
category: mcp
applicable_roles: [executive]
mcp_config:
  exposed_tools: ["object *", "system *", "reports *"]
  blocked_tools: []  # Executives have broad access
```

## Conclusion

MCP workflows are a clean specialization of the workflow object that:
- Reuses all workflow constraint logic
- Adds MCP-specific tool exposure rules
- Provides context-aware, role-based tool filtering
- Reduces coupling between business logic and infrastructure config

This approach maintains the flexibility and granularity of workflows while providing MCP-specific capabilities.

