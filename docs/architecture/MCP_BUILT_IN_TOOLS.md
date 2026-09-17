# MCP Built-In Tools

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Document built-in MCP tools and opportunities for expansion

## Overview

Built-in tools are MCP tools that are **hardcoded directly in the MCP server** rather than being discovered from CLI commands. They provide specialized functionality that either:
1. Doesn't map cleanly to CLI commands
2. Requires direct access to internal systems (graph backend, event system, etc.)
3. Provides optimized, purpose-built operations for AI agents

## Current Built-In Tools (13 total)

### 1. Common CLI Tools (5 tools)

These tools wrap commonly used CLI commands and are always available (bypass config whitelisting):

#### `zqk_object_list`
- **Purpose**: List objects with filtering, sorting, and pagination
- **Use Cases**: 
  - Query objects by kind, status, or other properties
  - Find specific objects using filters
  - Most commonly used tool for querying the system
- **Parameters**:
  - `kind`: Object kind to filter by (optional)
  - `filter`: Filter expressions array (optional)
  - `sort_by`: Field to sort by (optional)
  - `sort_asc`: Sort ascending/descending (default: true)
  - `limit`: Maximum number of objects (default: 100)
  - `offset`: Number of objects to skip (default: 0)
  - `format`: Output format "json", "yaml", or "table" (default: "json")

#### `zqk_object_get`
- **Purpose**: Get a single object by ID
- **Use Cases**:
  - Retrieve detailed information about a specific object
  - Get full object data including all fields
- **Parameters**:
  - `id` (required): Object ID (e.g., "BLI-626")
  - `format`: Output format "json" or "yaml" (default: "json")

#### `zqk_object_count`
- **Purpose**: Count objects by kind and optional filters
- **Use Cases**:
  - Quickly get counts without retrieving full object data
  - Count objects matching specific criteria
- **Parameters**:
  - `kind`: Object kind to count (optional)
  - `filter`: Filter expressions array (optional)
  - `format`: Output format "json" or "yaml" (default: "json")

#### `zqk_system_status`
- **Purpose**: Get system status and health information
- **Use Cases**:
  - Check system health before performing operations
  - Monitor overall system status
- **Parameters**:
  - `format`: Output format "json" or "yaml" (default: "json")

#### `zqk_system_check`
- **Purpose**: Check object health and compliance
- **Use Cases**:
  - Validate specific objects
  - Run system-wide health checks
  - Identify tiered violations
- **Parameters**:
  - `id`: Object ID to check (optional, checks all if not provided)
  - `tier`: Tier to check 1-4 (optional, checks all if not specified)
  - `auto_fix`: Automatically fix issues (default: false)
  - `force`: Force fix even if it creates audit events (default: false)
  - `format`: Output format "json" or "yaml" (default: "json")

### 2. Graph Traversal Tools (3 tools)

These tools require the graph backend to be enabled (`ZQK_GRAPH_ENABLED=true`):

#### `zqk_graph_traversal`
- **Purpose**: Perform multi-hop graph traversal starting from a node
- **Use Cases**: 
  - Explore object relationships
  - Find connected items
  - Trace dependencies
  - Discover transitive relationships
- **Parameters**:
  - `start_node_id` (required): Starting node ID (e.g., "BLI-010")
  - `relationship`: Relationship type to traverse (optional)
  - `direction`: "outgoing", "incoming", or "both" (default: "outgoing")
  - `max_depth`: Maximum traversal depth (default: 3)
  - `filter_labels`: Filter nodes by labels
  - `filter_properties`: Filter nodes by properties
  - `limit`: Maximum number of nodes to return (default: 100)

#### `zqk_resolve_references`
- **Purpose**: Resolve object references to actual objects
- **Use Cases**:
  - Expand reference strings (e.g., "goal:GOAL-123") into full object data
  - Understand relationships and dependencies
  - Batch resolve multiple references
- **Parameters**:
  - `references` (required): List of references to resolve
  - `include_related`: Include related objects (default: false)
  - `format`: Output format "json" or "yaml" (default: "json")

#### `zqk_state_aware_query`
- **Purpose**: Perform queries aware of object lifecycle states
- **Use Cases**:
  - Find active items
  - Find blocked items
  - Track dependencies
  - Monitor progress
- **Parameters**:
  - `query_type` (required): "active_items", "blocked_items", "dependencies", or "progress"
  - `filters`: Additional filters (status, kind, date_range, etc.)
  - `include_metrics`: Include calculated metrics (default: false)
  - `format`: Output format "json", "yaml", or "markdown" (default: "json")

### 3. Workflow-Aware Tools (4 tools)

These tools are context-aware and optimized for agent workflows. They understand priority plans, current work, and next items.

#### `zqk_get_current_priority_plan`
- **Purpose**: Get the current active priority plan
- **Use Cases**:
  - Determine what priority plan agents should be working on
  - Get the plan with the lowest `active_order` (or first active plan)
- **Parameters**:
  - `format`: Output format "json", "yaml", or "table" (default: "json")

#### `zqk_get_priority_plan_items`
- **Purpose**: Get all backlog items for a specific priority plan, organized by priority tier
- **Use Cases**:
  - See what items are in a priority plan
  - Understand the scope of work for a plan
- **Parameters**:
  - `priority_plan_id` (required): Priority plan ID (e.g., "PRI-210")
  - `format`: Output format "json", "yaml", or "table" (default: "json")

#### `zqk_get_current_backlog_item`
- **Purpose**: Get the currently in-progress backlog item
- **Use Cases**:
  - See what the agent is currently working on
  - Get the item with highest priority_tier that's in progress
- **Parameters**:
  - `format`: Output format "json", "yaml", or "table" (default: "json")

#### `zqk_get_next_backlog_item`
- **Purpose**: Get the next immediate backlog item to work on
- **Use Cases**:
  - See what the agent should work on next
  - Get the highest priority planned item (or exploring if no planned items)
- **Parameters**:
  - `format`: Output format "json", "yaml", or "table" (default: "json")

### 4. Test/Debug Tools (1 tool)

#### `zqk_test_echo`
- **Purpose**: Test tool that echoes a message back
- **Use Cases**:
  - Validate MCP communication
  - Test MCP server responsiveness
  - Debug connection issues
- **Parameters**:
  - `message` (required): Message to echo back

## Built-In vs CLI Tools

### Key Differences

| Aspect | Built-In Tools | CLI Tools |
|--------|----------------|----------|
| **Discovery** | Hardcoded in `RegisterAllTools()` | Auto-discovered from Cobra command tree |
| **Filtering** | Always available (not filtered by config) | Filtered by `exposed_commands`, `blocked_commands`, RBAC |
| **Security** | Not subject to `exposed_commands` whitelist | Subject to config security and RBAC |
| **Dependencies** | May require special backends (graph, events) | Standard CLI execution |
| **Performance** | Optimized for specific use cases | General-purpose CLI execution |
| **Maintenance** | Manual registration in code | Automatic via CLI command discovery |

### When to Use Built-In Tools

**Use built-in tools when:**
- ✅ You need direct access to graph backend
- ✅ You need optimized, purpose-built operations
- ✅ The operation doesn't map cleanly to a CLI command
- ✅ You need real-time event or state information
- ✅ You need batch operations that are more efficient than CLI
- ✅ You need workflow-aware operations (current priority plan, current item, next item)

**Use CLI tools when:**
- ✅ The operation maps directly to a CLI command
- ✅ You need standard CRUD operations
- ✅ You want to leverage existing CLI functionality
- ✅ You need operations that are already well-tested in CLI

## Opportunities for Expansion

### 1. Event System Tools

**Potential Tools:**
- `zqk_subscribe_events`: Subscribe to real-time system events
- `zqk_get_event_history`: Get historical event data
- `zqk_emit_event`: Emit custom events (for testing/integration)

**Benefits:**
- Real-time awareness of system changes
- Event-driven workflows
- Better observability

**Implementation Notes:**
- Requires event emitter system (already exists in `pkg/mcp/server.go`)
- Could leverage existing `EventEmitter` infrastructure

### 2. Batch Operations Tools

**Potential Tools:**
- `zqk_bulk_resolve`: Resolve multiple references in one call
- `zqk_bulk_traverse`: Traverse multiple starting nodes
- `zqk_batch_query`: Execute multiple queries in parallel

**Benefits:**
- More efficient than multiple CLI calls
- Reduced network overhead
- Better performance for bulk operations

**Implementation Notes:**
- Could wrap existing CLI `bulk` commands
- Or provide optimized implementations

### 3. System Health Tools

**Potential Tools:**
- `zqk_system_health`: Get comprehensive system health status
- `zqk_check_integrity`: Check hash registry integrity
- `zqk_get_metrics`: Get system metrics (PCS, EDD, D&B)

**Benefits:**
- Quick health checks without full CLI execution
- Optimized for monitoring/alerting
- Real-time system status

**Implementation Notes:**
- Could leverage existing `zqk system check` logic
- Or provide direct access to metrics systems

### 4. Search and Discovery Tools

**Potential Tools:**
- `zqk_semantic_search`: Semantic search across objects
- `zqk_find_similar`: Find similar objects
- `zqk_discover_patterns`: Discover patterns in relationships

**Benefits:**
- Advanced search capabilities
- AI-friendly discovery operations
- Pattern recognition

**Implementation Notes:**
- Would require semantic search backend
- Could leverage graph traversal with filters

### 5. Workflow and Automation Tools

**Potential Tools:**
- `zqk_execute_workflow`: Execute predefined workflows
- `zqk_get_workflow_status`: Get workflow execution status
- `zqk_trigger_automation`: Trigger automation rules

**Benefits:**
- Workflow orchestration
- Automation integration
- Process automation

**Implementation Notes:**
- Would require workflow engine
- Could leverage existing automation system

### 6. Context and State Tools

**Potential Tools:**
- `zqk_get_context`: Get current execution context
- `zqk_save_context`: Save context for later use
- `zqk_restore_context`: Restore saved context

**Benefits:**
- Context persistence across sessions
- Better state management
- Improved agent continuity

**Implementation Notes:**
- Would require context storage system
- Could leverage existing context infrastructure

## Implementation Guidelines

### Adding a New Built-In Tool

1. **Create Tool Registration Function**
   ```go
   func RegisterMyNewTool(server *Server) {
       server.RegisterTool(
           "zqk_my_new_tool",
           "Description of what the tool does...",
           map[string]any{
               "type": "object",
               "properties": map[string]any{
                   // Define parameters
               },
               "required": []string{"param1"},
           },
           nil, // Handler called via switch statement
       )
   }
   ```

2. **Add Handler Function**
   ```go
   func HandleMyNewTool(args map[string]any) (any, error) {
       // Extract parameters
       // Perform operation
       // Return result
   }
   ```

3. **Register in `RegisterAllTools()`**
   ```go
   func RegisterAllTools(server *Server) {
       RegisterGraphTools(server)
       RegisterEchoTool(server)
       RegisterMyNewTool(server) // Add here
       // ...
   }
   ```

4. **Add Handler Case in `handleToolCallWithContext()`**
   ```go
   switch name {
   case "zqk_my_new_tool":
       return HandleMyNewTool(args)
   // ...
   }
   ```

5. **Update Tool Counting Logic** (if needed)
   - Update `builtInTools` map in `server_handlers.go` for accurate counting

### Best Practices

1. **Naming Convention**: Use `zqk_` prefix for all built-in tools
2. **Documentation**: Include clear descriptions and examples
3. **Error Handling**: Use `NewElicitationError()` for missing required parameters
4. **Testing**: Create comprehensive tests in `*_test.go` files
5. **Security**: Consider RBAC even for built-in tools (if applicable)
6. **Performance**: Optimize for common use cases
7. **Backward Compatibility**: Support both `zqk_` and non-prefixed names

## Current Limitations

1. **Graph Backend Dependency**: Graph tools require `ZQK_GRAPH_ENABLED=true`
2. **Limited Tool Count**: Only 4 built-in tools currently (3 graph + 1 echo)
3. **No Event Tools**: Event system exists but no tools expose it
4. **No Batch Operations**: No optimized batch operation tools
5. **No Health Tools**: System health requires full CLI execution

## Recommendations

1. **Short Term** (Next Sprint):
   - Add `zqk_system_health` tool for quick health checks
   - Add `zqk_bulk_resolve` for efficient reference resolution

2. **Medium Term** (Next Quarter):
   - Add event subscription tools
   - Add batch operation tools
   - Add semantic search tools

3. **Long Term** (Future):
   - Add workflow orchestration tools
   - Add context persistence tools
   - Add pattern discovery tools

## Related Documentation

- [MCP Server Package README](../../pkg/mcp/README.md)
- [MCP CLI Bridge Architecture](./mcp-cli-bridge-v1.0.md)
- [Graph Traversal Tools](../../pkg/mcp/handlers_graph.go)
- [Tool Registration](../../pkg/mcp/tools.go)

