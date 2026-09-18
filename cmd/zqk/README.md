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
