# MCP CLI Bridge Architecture v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Category**: CLI & Interface

## Overview

This document describes the context-driven, security-aware CLI bridge that automatically exposes CLI commands as MCP tools. The bridge provides automatic parity between CLI commands and MCP tools without requiring code changes, with privilege-based filtering based on security context.

## Architecture

The CLI bridge provides automatic parity between CLI commands and MCP tools without requiring code changes. It:

1. **Discovers CLI commands** from the cobra command tree
2. **Filters by permissions** based on security context (roles and permissions)
3. **Converts to MCP tools** automatically
4. **Executes commands** with proper context and security

### Components

1. **Command Discovery** (`DiscoverCLICommands`)
   - Walks the cobra command tree
   - Extracts metadata (permissions, roles) from command annotations
   - Builds a flat list of discoverable commands

2. **Permission Filtering** (`FilterCommandsByPermissions`)
   - Filters commands based on security context
   - Checks roles and permissions
   - Only exposes commands the user can execute

3. **Tool Generation** (`ConvertCommandToMCPTool`)
   - Converts cobra commands to MCP tool definitions
   - Extracts flags and converts to JSON schema
   - Generates tool names and descriptions

4. **Command Execution** (`ExecuteCLICommandViaMCP`)
   - Executes CLI commands via the zqk binary
   - Passes security context via environment variables
   - Returns JSON-formatted results

5. **Security Context Initialization** (`InitializeSecurityContextFromMCP`)
   - Extracts security context from MCP client info
   - Supports account ID, roles, and permissions
   - Falls back to system context if not provided

## Usage

### Basic Setup

```go
import (
    "github.com/lanceman/zqk/pkg/mcp"
    "github.com/spf13/cobra"
    pkgctx "github.com/lanceman/zqk/pkg/context"
)

// Create MCP server
server := mcp.NewServer()

// Set root command (from cmd/zqk)
rootCmd := getRootCommand() // Your root cobra command
server.SetRootCommand(rootCmd)

// Set security context (or let it be initialized from client info)
secCtx := pkgctx.NewSecurityContext("account:user", []string{"admin"}, []string{"read:*", "write:*"})
server.SetSecurityContext(secCtx)

// Set project root
server.SetProjectRoot("/path/to/project")

// Start server
server.Serve()
```

### Automatic Bootstrap

During MCP initialization, if `rootCommand` and `secCtx` are set, CLI tools are automatically bootstrapped:

1. Client sends `initialize` request with client info
2. Server extracts security context from client info (if not already set)
3. Server discovers all CLI commands
4. Server filters commands by permissions
5. Server registers filtered commands as MCP tools

### Manual Registration

You can also manually register CLI tools:

```go
import (
    "github.com/lanceman/zqk/pkg/mcp"
    "github.com/spf13/cobra"
    pkgctx "github.com/lanceman/zqk/pkg/context"
)

rootCmd := getRootCommand()
secCtx := pkgctx.NewSecurityContext("account:user", []string{"admin"}, []string{"read:*", "write:*"})

// Register CLI tools manually
err := mcp.RegisterCLIToolsWithRootCommand(server, rootCmd, secCtx, "/path/to/project")
```

## Command Annotations

Commands can specify required permissions and roles via annotations:

```go
cmd := &cobra.Command{
    Use:   "delete",
    Short: "Delete an object",
    Annotations: map[string]string{
        "mcp.permissions": "delete:backlog_item",
        "mcp.roles": "admin",
    },
}
```

### Permission Format

- `read:*` - Read any object
- `write:backlog_item` - Write backlog items
- `delete:backlog_item` - Delete backlog items

### Role Format

- `admin` - Administrator (has all permissions)
- `developer` - Developer role
- `viewer` - Read-only access

## Security Context from Client

MCP clients can provide security context during initialization:

```json
{
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "client_id": "account:ai-agent-1",
      "roles": ["developer"],
      "permissions": ["read:*", "write:backlog_item"]
    },
    "clientInfo": {
      "name": "ai-agent",
      "version": "1.0.0"
    }
  }
}
```

The server will:
1. Extract `client_id` as `account_id`
2. Extract `roles` array
3. Extract `permissions` array
4. Create security context
5. Filter CLI tools based on permissions

## Tool Naming

CLI commands are converted to MCP tools with the prefix `cli_`:

- `zqk object list` → `cli_object_list`
- `zqk object create` → `cli_object_create`

## Command Execution

When a CLI tool is called via MCP:

1. Tool name is converted back to command path
2. Arguments are converted to CLI flags
3. Command is executed via `zqk` binary
4. Security context is passed via environment variables:
   - `ZQK_MCP_ACCOUNT_ID`
   - `ZQK_MCP_ROLES`
   - `ZQK_MCP_PERMISSIONS`
5. Output is captured and returned as JSON

## Example

### Client Request

```json
{
  "method": "tools/call",
  "params": {
    "name": "cli_object_list",
    "arguments": {
      "kind": "backlog_item",
      "filter": "status=exploring"
    }
  }
}
```

### Server Execution

1. Converts `cli_object_list` → `object list`
2. Executes: `zqk object list backlog_item --filter status=exploring --format json --context ai-agent`
3. Returns JSON result

## Benefits

1. **No Code Changes**: New CLI commands automatically become MCP tools
2. **Security-Aware**: Commands are filtered by permissions
3. **Context-Driven**: Security context is initialized from client info
4. **Consistent**: Same commands work in CLI and MCP
5. **Maintainable**: Single source of truth (CLI commands)

## Limitations

1. **Import Cycles**: Root command must be passed explicitly (can't import from cmd/zqk)
2. **Binary Execution**: Commands are executed via subprocess (not in-process)
3. **Flag Conversion**: Complex flag types may need manual mapping

## Future Enhancements

1. In-process command execution (avoid subprocess overhead)
2. Direct function calls for better performance
3. Streaming output for long-running commands
4. Command result caching
5. Permission caching for better performance

## Related Documentation

- [MCP Server Package](../../../pkg/mcp/README.md) - Package implementation
- [MCP Privilege Tests](./mcp-privilege-tests-v1.0.md) - Test coverage for privilege validation
- [CLI Ontology Specification v1.0](./cli-ontology-v1.0.md) - CLI command structure

---

**Status**: Active  
**Last Updated**: 2025-01-02

