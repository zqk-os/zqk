# Configurable Brand Prefix for Tool Names

## Overview

All MCP tool names now use a configurable brand prefix derived from the executable name, enabling white-labeling without hardcoded brand references.

## Implementation

### Brand Prefix Functions

Located in `pkg/mcp/brand_prefix.go`:

- **`GetExecutableName()`**: Returns the current executable name
- **`GetBrandPrefix()`**: Returns the brand prefix (executable name with common suffixes removed)
- **`GetToolPrefix()`**: Returns the tool prefix (brand prefix + "_")
- **`GetToolName(toolSuffix)`**: Creates a full tool name with brand prefix

### Examples

If the executable is named:
- `zqk` → tool prefix: `zqk_` → tools: `zqk_object_list`, `zqk_graph_traversal`
- (legacy) `zqk` → tool prefix: `zqk_` → tools: `zqk_object_list`, `zqk_graph_traversal`
- `mybrand` → tool prefix: `mybrand_` → tools: `mybrand_object_list`, `mybrand_graph_traversal`
- `mybrand-stable` → tool prefix: `mybrand_` (suffix removed) → tools: `mybrand_object_list`

### Suffix Removal

Common suffixes are automatically removed from executable names:
- `-stable`
- `-dev`
- `-beta`
- `-alpha`
- `-rc`

This ensures `mybrand-stable` and `mybrand-dev` both produce `mybrand_` prefix.

## Usage

### In Tool Registration

**Before (hardcoded)**:
```go
server.RegisterTool(
    "zqk_object_list",
    "List objects...",
    ...
)
```

**After (configurable)**:
```go
server.RegisterTool(
    GetToolName("object_list"),
    "List objects...",
    ...
)
```

### In Tool Descriptions

Tool descriptions should also use `GetToolName()` for examples:

```go
server.RegisterTool(
    GetToolName("object_list"),
    "List objects with filtering. Example: " + GetToolName("object_list") + " with kind='backlog_item'.",
    ...
)
```

## Updated Files

All tool registration files have been updated:
- `pkg/mcp/tools.go` - Graph tools
- `pkg/mcp/tools_common.go` - Common tools (object_list, object_get, system_status, etc.)
- `pkg/mcp/tools_interactive.go` - Interactive tools
- `pkg/mcp/tools_workflow.go` - Workflow tools
- `pkg/mcp/tools_echo.go` - Test tools
- `pkg/mcp/server_handlers.go` - Welcome messages and documentation

## Tool Name Mapping

All built-in tools now use the configurable prefix:

| Tool Suffix | Full Tool Name (if executable is "zqk") |
|------------|------------------------------------------|
| `object_list` | `zqk_object_list` |
| `object_get` | `zqk_object_get` |
| `object_count` | `zqk_object_count` |
| `system_status` | `zqk_system_status` |
| `system_check` | `zqk_system_check` |
| `graph_traversal` | `zqk_graph_traversal` |
| `resolve_references` | `zqk_resolve_references` |
| `state_aware_query` | `zqk_state_aware_query` |
| `create_object_interactive` | `zqk_create_object_interactive` |
| `get_current_priority_plan` | `zqk_get_current_priority_plan` |
| `get_priority_plan_items` | `zqk_get_priority_plan_items` |
| `get_current_backlog_item` | `zqk_get_current_backlog_item` |
| `get_next_backlog_item` | `zqk_get_next_backlog_item` |
| `test_echo` | `zqk_test_echo` |

## Benefits

1. **White-Labeling**: No hardcoded brand references
2. **Automatic**: Prefix derived from executable name at runtime
3. **Consistent**: All tools use the same prefix
4. **Flexible**: Works with any executable name
5. **Backward Compatible**: Existing tool handlers work with any prefix

## Tool Handler Compatibility

Tool handlers in `server_handlers.go` use pattern matching that works with any prefix:

```go
// Matches any tool with the configured prefix
if strings.HasPrefix(name, GetToolPrefix()) {
    // Handle built-in tool
}
```

This ensures handlers work regardless of the brand prefix.

## Future Enhancements

Potential future improvements:
1. **Configurable Override**: Allow prefix override via config file or environment variable
2. **Prefix Validation**: Validate prefix format (alphanumeric, underscores)
3. **Migration Tool**: Tool to update existing tool references in documentation
