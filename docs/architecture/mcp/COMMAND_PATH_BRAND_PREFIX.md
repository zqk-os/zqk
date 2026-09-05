# Command Path Brand Prefix Update

**Last Verified:** 2026-08-31


## Overview

All hardcoded command path references have been updated to use the configurable brand prefix derived from the executable name.

## New Functions

### `GetCommandPath(commandPath string)`
Creates a command path with the executable name prefix.

**Example**:
- `GetCommandPath("object list")` → `"zqk object list"` (if executable is "zqk")
- `GetCommandPath("object list")` → `"{executable} object list"` (e.g. "zqk object list" for legacy executable name)

### `NormalizeCommandPath(commandPath string)`
Removes the executable name prefix from a command path.

**Example**:
- `NormalizeCommandPath("zqk object list")` → `"object list"`
- `NormalizeCommandPath("zqk object list")` → `"object list"` (legacy support)

Also handles legacy hardcoded prefixes (`zqk `, `zqk `) for backward compatibility.

### `findExecutableBinary()`
Finds the executable binary using the current executable name (e.g. "zqk") instead of a hardcoded name.

**Behavior**:
1. Checks `ZQK_STABLE_BINARY_PATH` environment variable
2. Uses current executable (`os.Args[0]`)
3. Tries current directory: `./{executable}`, `./{executable}-stable`
4. Tries PATH: `{executable}`, `{executable}-stable`
5. Falls back to legacy names (`zqk`, `zqk`) for backward compatibility

## Updated Files

### 1. `pkg/mcp/tools_workflow.go`

**Before**:
```go
cliArgs := map[string]any{
    "_command_path": "zqk object list priority_plan",
    ...
}
```

**After**:
```go
cliArgs := map[string]any{
    "_command_path": GetCommandPath("object list priority_plan"),
    ...
}
```

**Updated locations**:
- `HandleGetCurrentPriorityPlan`: `"zqk object list priority_plan"` → `GetCommandPath("object list priority_plan")`
- `HandleGetPriorityPlanItems`: `"zqk object list backlog_item"` → `GetCommandPath("object list backlog_item")`
- `HandleGetCurrentBacklogItem`: `"zqk object list backlog_item"` → `GetCommandPath("object list backlog_item")`
- `HandleGetNextBacklogItem`: `"zqk object list backlog_item"` → `GetCommandPath("object list backlog_item")`
- `handleGetNextBacklogItemExploring`: `"zqk object list backlog_item"` → `GetCommandPath("object list backlog_item")`

### 2. `pkg/mcp/cli_bridge.go`

**Before**:
```go
// Remove executable name prefix from command path if present (e.g. "zqk ", "zqk ")
if strings.HasPrefix(commandPath, "zqk ") {
    commandPath = strings.TrimPrefix(commandPath, "zqk ")
}
```

**After**:
```go
// Remove executable name prefix from command path if present
commandPath = NormalizeCommandPath(commandPath)
```

**Binary Finding**:
- `findExecutableBinary()` (replaces brand-specific binary finder)
- Uses `GetExecutableName()` (e.g. "zqk") instead of hardcoded executable name
- Searches for executable name and `{executable}-stable`
- Falls back to legacy names for backward compatibility

### 3. `pkg/mcp/config_security.go`

**Before**:
```go
func normalizeCommandPath(path string) string {
    rootPrefixes := []string{"zqk ", "cli "}
    for _, prefix := range rootPrefixes {
        if strings.HasPrefix(path, prefix) {
            return strings.TrimPrefix(path, prefix)
        }
    }
    return path
}
```

**After**:
```go
func normalizeCommandPath(path string) string {
    return NormalizeCommandPath(path)
}
```

### 4. `pkg/mcp/server.go`

**Before**:
```go
if !strings.HasPrefix(existingPath, "zqk ") && !strings.HasPrefix(existingPath, commandPath) {
    args["_command_path"] = commandPath
}
```

**After**:
```go
normalizedExisting := NormalizeCommandPath(existingPath)
normalizedCommand := NormalizeCommandPath(commandPath)
if !strings.HasPrefix(normalizedExisting, normalizedCommand) {
    args["_command_path"] = GetCommandPath(commandPath)
}
```

### 5. `pkg/mcp/tools_interactive_handlers.go`

**Before**:
```go
tmpFile, err := os.CreateTemp("", "zqk-interactive-*.yaml")
```

**After**:
```go
execName := GetBrandPrefix()
tmpFile, err := os.CreateTemp("", execName+"-interactive-*.yaml")
```

## Command Path Examples

### Workflow Tools

| Function | Before | After (if executable is "zqk") |
|----------|--------|--------------------------------|
| `HandleGetCurrentPriorityPlan` | `"zqk object list priority_plan"` | `"zqk object list priority_plan"` |
| `HandleGetPriorityPlanItems` | `"zqk object list backlog_item"` | `"zqk object list backlog_item"` |
| `HandleGetCurrentBacklogItem` | `"zqk object list backlog_item"` | `"zqk object list backlog_item"` |
| `HandleGetNextBacklogItem` | `"zqk object list backlog_item"` | `"zqk object list backlog_item"` |

### Common Tools

Common tools (`HandleObjectList`, `HandleObjectGet`, etc.) already use unprefixed paths like `"object list"`, which is correct. The CLI bridge will add the brand prefix automatically via `GetCommandPath()` when needed.

## Backward Compatibility

### Command Path Normalization

`NormalizeCommandPath()` supports:
1. **Brand prefix**: `"{executable} object list"` → `"object list"`
2. **Legacy prefixes**: `"zqk object list"` → `"object list"` (backward compatibility)
3. **Unprefixed**: `"object list"` → `"object list"` (no change)

### Binary Finding

`findExecutableBinary()` tries:
1. Current executable name
2. `{executable}-stable`
3. Legacy names: `zqk`, `zqk-stable`, `zqk`, `zqk-stable` (backward compatibility)

## Benefits

1. **White-Labeling**: Command paths use executable name, not hardcoded brand name (e.g. "zqk")
2. **Consistency**: All command paths use the same brand prefix system
3. **Flexibility**: Works with any executable name
4. **Backward Compatible**: Legacy paths still work
5. **Automatic**: No manual updates needed when rebranding

## Status

✅ **Complete**: All hardcoded command path references updated
✅ **Backward Compatible**: Legacy paths still supported
✅ **White-Label Ready**: No hardcoded brand references in command paths
