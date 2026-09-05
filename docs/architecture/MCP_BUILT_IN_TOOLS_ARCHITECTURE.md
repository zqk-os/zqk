# MCP Built-In Tools Architecture

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Define the architecture and principles for built-in vs registered tools

## Current Architecture (Needs Refinement)

### Current State
- **Built-in tools**: Always available, bypass config whitelisting
- **Registered CLI tools**: Filtered by RBAC + config whitelisting

### Problem
The current architecture doesn't distinguish between:
- **Workflow/context-aware tools** (priority plan, current item, next item)
- **General operations** (list, get, create, update)

## Proposed Architecture

### Built-In Tools = Workflow/Context-Aware Tools

**Purpose**: Provide workflow-specific, context-aware operations that are optimized for agent use cases.

**Characteristics**:
- Workflow-aware (know about priority plans, current work, next items)
- Context-aware (understand current state, relationships)
- Role-based (can be filtered by role, but always available to appropriate roles)
- Optimized for agent workflows
- Don't map cleanly to single CLI commands

**Examples**:
- `zqk_get_current_priority_plan` - Get current active priority plan
- `zqk_get_current_backlog_item` - Get currently in-progress backlog item
- `zqk_get_next_backlog_item` - Get next immediate backlog item to work on
- `zqk_get_priority_plan_items` - Get all items for a priority plan
- `zqk_graph_traversal` - Graph traversal (workflow-aware)
- `zqk_state_aware_query` - State-aware queries (workflow-aware)

### Registered CLI Tools = General Operations

**Purpose**: Provide general CRUD and query operations that map directly to CLI commands.

**Characteristics**:
- Map directly to CLI commands
- General-purpose operations
- Filtered by RBAC (roles and permissions)
- Filtered by config (exposed_commands, blocked_commands)
- Standard CRUD operations

**Examples**:
- `zqk_object_list` - General list operation
- `zqk_object_get` - General get operation
- `zqk_object_create` - General create operation
- `zqk_object_update` - General update operation
- `zqk_system_check` - General system check

## Key Distinctions

| Aspect | Built-In Tools | Registered CLI Tools |
|--------|----------------|---------------------|
| **Purpose** | Workflow/context-aware | General operations |
| **Mapping** | Don't map to single CLI command | Map directly to CLI commands |
| **Filtering** | Role-based (can filter by role) | RBAC + config whitelisting |
| **Availability** | Always available to appropriate roles | Filtered by config + permissions |
| **Optimization** | Optimized for agent workflows | General-purpose |
| **Examples** | Priority plan, current item, next item | List, get, create, update |

## Implementation Strategy

### Phase 1: Add Workflow-Aware Built-In Tools

1. **Priority Plan Tools**:
   - `zqk_get_current_priority_plan` - Wraps `object pplan current` logic
   - `zqk_get_priority_plan_items` - Gets items for a plan
   - `zqk_get_next_priority_plan` - Gets next plan in sequence

2. **Backlog Item Tools**:
   - `zqk_get_current_backlog_item` - Gets in-progress item
   - `zqk_get_next_backlog_item` - Gets next item to work on
   - `zqk_get_backlog_item_by_priority` - Gets items by priority tier

### Phase 2: Make Built-In Tools Role-Based

1. **Add role filtering to built-in tools**:
   - Some tools available to all roles (read operations)
   - Some tools require specific roles (write operations, planning)
   - Some tools require admin role (system operations)

2. **Update registration**:
   - `RegisterWorkflowTools(server, secCtx)` - Role-aware registration
   - Filter tools based on security context

### Phase 3: Refactor Common Tools

1. **Move common CLI tools back to registered**:
   - `zqk_object_list`, `zqk_object_get`, etc. should be registered CLI tools
   - They're general operations, not workflow-specific

2. **Keep only workflow tools as built-in**:
   - Priority plan tools
   - Current/next item tools
   - Graph traversal tools
   - State-aware query tools

## Benefits

1. **Clear Separation**: Workflow tools vs general operations
2. **Role-Based Access**: Built-in tools can be role-aware
3. **Better Agent Experience**: Workflow tools optimized for agent use cases
4. **Reduced Context**: Agents know workflow tools are always available (if they have the role)

## Migration Plan

1. **Add workflow tools** as built-in (priority plan, current item, next item)
2. **Make built-in tools role-based** (filter by security context)
3. **Move common CLI tools** back to registered (they're general operations)
4. **Update documentation** to reflect new architecture

