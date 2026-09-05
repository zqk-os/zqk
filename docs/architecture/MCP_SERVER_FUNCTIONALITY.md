# MCP Server Functionality Overview

**Last Verified:** 2026-08-31


**Last Updated**: 2026-01-18  
**Total Tools Available**: 22 tools  
**Server Status**: Active with 15-minute operation timeout

## Overview

The MCP (Model Context Protocol) server provides comprehensive access to the ZQK knowledge kernel through:
- **Tools**: Executable operations (22 total)
- **Resources**: Documentation and guides
- **Prompts**: Help templates and guides
- **Protocol**: JSON-RPC 2.0 over stdio

## Available Tools (22 Total)

### 1. Built-In Tools (13 tools)

These tools are hardcoded in the server and always available (bypass config filtering):

#### Common Object Operations (5 tools)

1. **`zqk-mcp_object_list`** / `zqk_object_list`
   - List objects with filtering, sorting, pagination
   - Parameters: `kind`, `filter[]`, `sort_by`, `sort_asc`, `limit`, `offset`, `format`
   - Most commonly used tool for querying the system

2. **`zqk-mcp_object_get`** / `zqk_object_get`
   - Get a single object by ID
   - Parameters: `id` (required), `format`

3. **`zqk-mcp_object_count`** / `zqk_object_count`
   - Count objects by kind and filters
   - Parameters: `kind`, `filter[]`, `format`

4. **`zqk-mcp_system_status`** / `zqk_system_status`
   - Get system status and health information
   - Parameters: `format`

5. **`zqk-mcp_system_check`** / `zqk_system_check`
   - Check object health and compliance
   - Parameters: `id`, `tier`, `auto_fix`, `force`, `format`

#### Graph Traversal Tools (3 tools)
*Requires graph backend enabled (`ZQK_GRAPH_ENABLED=true`)*

6. **`zqk-mcp_graph_traversal`** / `zqk_graph_traversal`
   - Multi-hop graph traversal from a starting node
   - Parameters: `start_node_id` (required), `relationship`, `direction`, `max_depth`, `filter_labels[]`, `filter_properties`, `limit`

7. **`zqk-mcp_resolve_references`** / `zqk_resolve_references`
   - Resolve object references to actual objects
   - Parameters: `references[]` (required), `include_related`, `format`

8. **`zqk-mcp_state_aware_query`** / `zqk_state_aware_query`
   - State-aware queries (active_items, blocked_items, dependencies, progress)
   - Parameters: `query_type` (required), `filters`, `include_metrics`, `format`

#### Workflow-Aware Tools (4 tools)

9. **`zqk-mcp_get_current_priority_plan`** / `zqk_get_current_priority_plan`
   - Get the current active priority plan (lowest active_order)
   - Parameters: `format`

10. **`zqk-mcp_get_priority_plan_items`** / `zqk_get_priority_plan_items`
    - Get all backlog items for a priority plan, organized by priority tier
    - Parameters: `priority_plan_id` (required), `format`

11. **`zqk-mcp_get_current_backlog_item`** / `zqk_get_current_backlog_item`
    - Get the currently in-progress backlog item (highest priority_tier)
    - Parameters: `format`

12. **`zqk-mcp_get_next_backlog_item`** / `zqk_get_next_backlog_item`
    - Get the next backlog item to work on (planned or exploring)
    - Parameters: `format`

#### Metrics & Observability (2 tools)

13. **`zqk-mcp_get_metrics`** / `zqk_get_metrics`
    - Get comprehensive MCP protocol metrics snapshot
    - Parameters: `format` (json or summary)

14. **`zqk-mcp_get_tool_metrics`** / `zqk_get_tool_metrics`
    - Get metrics for a specific tool
    - Parameters: `tool_name` (required)

#### Interactive Tools (1 tool)

15. **`zqk-mcp_create_object_interactive`** / `zqk_create_object_interactive`
    - Interactively create objects with guided field collection
    - Uses MCP elicitation for step-by-step field collection
    - Parameters: `kind` (required), `session_id` (optional), plus dynamic fields

#### Test/Debug Tools (1 tool)

16. **`zqk-mcp_test_echo`** / `zqk_test_echo`
    - Test tool that echoes a message back
    - Parameters: `message` (required)

### 2. CLI Bridge Tools (Auto-Discovered)

All CLI commands are automatically exposed as MCP tools via the CLI bridge pattern. Tools are prefixed with `zqk-mcp_` or `zqk_`.

**Examples from trace log:**
- `zqk-mcp_object_list` (also available as built-in)
- `custom_analysis` (if available via CLI)

**Configuration**: 
- `register_cli_tools: true` (enabled)
- `exposed_commands: []` (empty = expose all commands)
- `blocked_commands: []` (none blocked)

**Note**: CLI tools are filtered by:
- Security context (roles and permissions)
- `exposed_commands` whitelist (if configured)
- `blocked_commands` blacklist
- Command-level permission requirements

## Available Prompts

Access via `prompts/get` with prompt name:

1. **`getting_started`** - Comprehensive getting started guide
2. **`query_help`** - How to query objects with filters
3. **`create_object_guide`** - Guide for creating objects (requires `kind` argument)
4. **`common_tasks`** - Examples of common tasks
5. **`filter_syntax`** - Filter syntax guide
6. **`role_based_access`** - Understanding role-based access
7. **`welcome`** - Welcome prompt for new sessions

## Available Resources

Access via `resources/get` with URI:

### Critical Resources (if spec file exists)
- Lifecycles guide (object state transitions)
- System health monitoring guide
- Workstreams guide
- Architecture decision records
- Policy and decision lifecycle guides

### Auto-Discovered Resources
- All markdown files in `docs/process/architecture/`
- Automatically registered when files exist

**URI Format**: `file://docs/path/to/file.md`

**Note:** `resources/subscribe` is **not implemented**. Clients receive `success: false` for that method. Use `resources/list` and `resources/get` to read documentation; avoid retrying subscribe.

## Configuration

Current MCP server configuration (`.zqk/mcp/config.yaml`):

```yaml
mcp_server:
  trace:
    enabled: true
    file: ".zqk/mcp/logs/mcp-trace.log"
  idle_timeout: "10m"
  register_cli_tools: true
  exposed_commands: []  # Empty = expose all
  blocked_commands: []
  write_operations: []
  async:
    max_concurrent: 10
    timeout: "15m"  # Increased from default 5m
```

## Protocol Features

### Async Operations
- **Max Concurrent**: 10 operations
- **Timeout**: 15 minutes (configurable)
- **Non-blocking**: Operations run in goroutines but responses maintain order

### Security
- **Role-Based Access Control (RBAC)**: Tools filtered by roles/permissions
- **Config-Based Filtering**: `exposed_commands` and `blocked_commands`
- **Write Operations**: Controlled via `write_operations` config

### Observability
- **Trace Logging**: Enabled at `.zqk/mcp/logs/mcp-trace.log`
- **Metrics**: Available via `get_metrics` tool
- **Per-Tool Metrics**: Available via `get_tool_metrics` tool

## Usage Examples

### List Available Tools
```json
{
  "method": "tools/list",
  "params": {}
}
```

### Call a Tool
```json
{
  "method": "tools/call",
  "params": {
    "name": "zqk-mcp_object_list",
    "arguments": {
      "kind": "backlog_item",
      "filter": ["status=in_progress"],
      "format": "json"
    }
  }
}
```

### Get a Prompt
```json
{
  "method": "prompts/get",
  "params": {
    "name": "getting_started"
  }
}
```

### Get a Resource
```json
{
  "method": "resources/get",
  "params": {
    "uri": "file://docs/onboarding/AI_AGENT_ONBOARDING.md"
  }
}
```

## Quick Reference

### Most Common Tools
1. `zqk-mcp_object_list` - Query objects
2. `zqk-mcp_object_get` - Get specific object
3. `zqk-mcp_system_status` - Check system health
4. `zqk-mcp_get_current_priority_plan` - Get current work context
5. `zqk-mcp_get_next_backlog_item` - Get next work item

### Workflow Tools
- `zqk-mcp_get_current_priority_plan` - Current priority plan
- `zqk-mcp_get_priority_plan_items` - Items in a plan
- `zqk-mcp_get_current_backlog_item` - Current work item
- `zqk-mcp_get_next_backlog_item` - Next work item

### Graph Tools (if enabled)
- `zqk-mcp_graph_traversal` - Explore relationships
- `zqk-mcp_resolve_references` - Resolve object references
- `zqk-mcp_state_aware_query` - State-aware queries

## Notes

- **Tool Names**: Tools may have multiple names (with/without prefixes) for backward compatibility
- **Timeout**: Operations have 15-minute timeout (configurable)
- **Concurrency**: Maximum 10 concurrent operations
- **Security**: All tools respect RBAC and config-based filtering
- **CLI Bridge**: All CLI commands automatically available as tools (unless blocked)
