# zqk CLI Commands

This directory contains the zqk CLI command implementations, organized by topical groups.


## Command Organization

Commands are organized into four topical groups matching the CLI ontology:

### Object Commands (`object/`)
CRUD operations for all object types:
- `create` - Create new objects
- `list` - List objects with filtering
- `get` - Get specific object by ID
- `update` - Update existing objects
- `delete` - Delete objects

### Domain Commands (`domain/`)
Domain-specific operations:
- `backlog` - Backlog item management
- `goal` - Goal operations
- `milestone` - Milestone operations
- `workstream` - Workstream operations
- `priority-plan` - Priority plan operations

### System Commands (`system/`)
System-level operations:
- `init` - Initialize a new zqk project
- `status` - Show system status
- `validate` - Validate objects
- `sync` - Sync with remote
- `check` - Check object health and integrity

### Utility Commands (`utility/`)
Helper operations:
- `version` - Show version information
- `migrate` - Migrate data between backends
- `docman` - Documentation management

## Adding New Commands

1. Place the command file in the appropriate subfolder
2. Export a `New*Cmd()` function that returns `*cobra.Command`
3. Register the command in `root.go`'s `registerCommands()` function

## Example

```go
// cmd/zqk/system/status.go
package system

import "github.com/spf13/cobra"

func NewStatusCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "status",
        Short: "Show system status",
        RunE:  runStatus,
    }
}
```

## MCP Integration (AI Tool Interop)

`zqk` ships a full **MCP JSON-RPC 2.0** server.

### Connect to IDE

Add to `~/.cursor/mcp.json` (or `.cursor/mcp.json` in your project):

```json
{
  "mcpServers": {
    "zqk": {
      "command": "zqk",
      "args": ["mcp", "serve", "--stdio"]
    }
  }
}
```

### Connect to Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "zqk": {
      "command": "zqk",
      "args": ["mcp", "serve", "--stdio"]
    }
  }
}
```

### Connect via local HTTP (any MCP client)

```sh
zqk mcp proxy --tcp 0.0.0.0:7777
# MCP endpoint: http://localhost:7777
```

### Smoke test (verify ≥1 tool exposed)

```sh
zqk mcp list-tools   # should print tool names
```

### Available MCP subcommands

| Subcommand | Description |
|------------|-------------|
| `serve` | Start MCP server (stdio, JSON-RPC 2.0) |
| `proxy` | Start MCP proxy shim (stdio ↔ TCP bridge) |
| `list-tools` | Print all registered tool names |
| `install` | Auto-configure supported IDEs (IDE, VS Code) |
| `daemon` | Start MCP server as background daemon |
