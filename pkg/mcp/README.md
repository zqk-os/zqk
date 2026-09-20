# MCP Server Package

**Status**: Active  
**Purpose**: Model Context Protocol (MCP) server implementation for zqk

## Overview

This package provides a complete MCP server implementation that enables AI assistants and other MCP clients to interact with the zqk knowledge kernel through a standardized protocol. The server exposes graph traversal tools that allow programmatic access to the graph backend.

## Architecture

### Components

- **`server.go`**: Core MCP server implementation with JSON-RPC protocol handling
- **`handlers_graph.go`**: Graph traversal tool handlers
- **`graph_connection.go`**: Graph backend connection management
- **`tools.go`**: Tool registration and schema definitions
- **`cli_bridge.go`**: CLI command discovery and MCP tool generation with privilege filtering
- **`handlers_graph_test.go`**: Comprehensive test suite for graph tools
- **`cli_bridge_test.go`**: Test suite for CLI bridge functionality
- **`cli_bridge_privilege_test.go`**: Comprehensive privilege validation tests

**Note**: Metrics tools (PCS, EDD, D&B) are exposed via CLI commands and automatically available through the CLI bridge. No separate metrics bridge is needed.

### Graph Traversal Tools

The MCP server exposes three graph traversal tools:

1. **`graph_traversal`**: Perform multi-hop graph traversal starting from a node
2. **`resolve_references`**: Resolve object references to actual graph nodes
3. **`state_aware_query`**: Perform queries aware of object lifecycle states

### CLI Bridge

The MCP server automatically exposes CLI commands as MCP tools with context-driven, security-aware filtering:

- **Automatic Discovery**: Discovers all CLI commands from the cobra command tree
- **Privilege Filtering**: Filters commands based on security context (roles and permissions)
- **Tool Generation**: Converts cobra commands to MCP tools automatically
- **Security Context**: Initializes from MCP client info during initialization

See [Quickstart / MCP Integration](../../docs/onboarding/QUICKSTART.md) for detailed information.

### Server Shutdown and OS Signal Handling

The MCP server gracefully handles termination to prevent data loss or corrupted states:
- **OS Signals**: The server listens for `SIGINT` and `SIGTERM`. When received, it initiates a graceful shutdown sequence.
- **Shutdown Tool**: Clients can request a graceful shutdown via the `server_shutdown` tool.
- **Graceful Shutdown**: The shutdown sequence flushes remaining messages, stops receiving new events, and safely closes active resource handles.

## Documentation

Detailed MCP integration documentation is available in:
- [docs/onboarding/QUICKSTART.md](../../docs/onboarding/QUICKSTART.md) - MCP server setup, client configuration, and quickstart
- [docs/architecture/README.md](../../docs/architecture/README.md) - Architecture documentation

### Metrics Tools

Metrics tools (PCS, EDD, D&B) are exposed via the CLI bridge:

- **CLI Commands**: Metrics are accessed through CLI commands (e.g., `zqk reports pcs`, `zqk reports edd`, `zqk reports blockers`)
- **Automatic MCP Exposure**: The CLI bridge automatically exposes these commands as MCP tools
- **No Special Bridge Needed**: Metrics tools use the same CLI bridge pattern as all other commands
- **Consistent Architecture**: All functionality is exposed through CLI commands, which are automatically available via MCP

### Server Execution Models: Direct Stdio vs. Proxy Adapter

ZQK supports two distinct deployment models for MCP integration with IDEs (Cursor, VS Code, Windsurf, Claude Desktop):

```
+-----------------------------------------------------------------------------------+
| 1. Direct Stdio (Canonical Default for Core Kernel & All Standalone Projects)     |
|                                                                                   |
|  [ IDE (Cursor / VS Code) ] --(stdio JSON-RPC)--> [ bin/zqk mcp serve ]           |
|                                                                                   |
|  * 100% self-contained within workspace (${workspaceFolder})                      |
|  * Zero background daemons, zero TCP ports, zero loopback collision hazards      |
|  * Multiple projects run in parallel with completely isolated memory & WAL       |
+-----------------------------------------------------------------------------------+

+-----------------------------------------------------------------------------------+
| 2. Proxy Adapter (Specialized for Studio Swarms & Continuous Hot-Rebuilds)        |
|                                                                                   |
|  [ IDE (Cursor) ] --(stdio)--> [ bin/zqk-mcp-ide-adapter ]                       |
|                                       |                                           |
|                               (TCP 127.0.0.1:8443)                                |
|                                       v                                           |
|                           [ zqk mcp daemon --tcp ]                                |
|                                                                                   |
|  * Shields Cursor from turning red when the underlying binary is recompiled       |
|  * Auto-reconnects when the background daemon cycles                              |
|  * Requires central daemon management; potential port clash if multiple repos    |
+-----------------------------------------------------------------------------------+
```

#### When to Use Direct Stdio (`zqk mcp serve`) — **The Core Kernel Default**
- **Default for all project-agnostic repositories:** Whenever you initialize ZQK in a project or work with the core kernel, use direct stdio (`bin/zqk mcp serve`).
- **Why:** 
  - Standard MCP compliance (Anthropic specification).
  - No background daemon lifecycle to supervise or troubleshoot.
  - Zero open TCP ports, avoiding port 8443 clashes when multiple repos are open simultaneously.
  - Managed cleanly by the IDE process tree: starts on workspace open, terminates on workspace close.

#### When to Use Proxy Adapter (`zqk mcp cursor-adapter` / `mcp proxy`)
- **Specialized for Continuous Autonomous Swarms:** Use only in dedicated development environments where autonomous agents or developers are continuously recompiling the CLI binary (`go build -o bin/zqk`) or cycling binaries under an active Cursor session.
- **Why:** Cursor turns its MCP connector red if a direct stdio process dies. The proxy maintains a permanent, resilient stdio channel with Cursor while transparently reconnecting to the background TCP daemon whenever the daemon restarts.

---

## Quick Start

```go
import "github.com/zqk-os/zqk/pkg/mcp"

server := mcp.NewServer()
mcp.RegisterGraphTools(server)
server.SetRootCommand(rootCmd)
server.SetSecurityContext(secCtx)
server.Serve()
```

See [Quickstart / MCP Integration](../../docs/onboarding/QUICKSTART.md) for detailed usage examples and configuration.

### Server Modes and Trimming

- **Full server** (`zqk mcp serve`): Has the full cobra CLI tree; discovers and exposes all leaf commands as tools, plus built-in tools, prompts, and resources.
- **Alias / Curated mode** (`alias_mode: true` in `.zqk/mcp/config.yaml`): Exposes a curated set of high-leverage tools (object CRUD, system status, reports, file I/O) to keep tool counts lean and well under IDE limits.

---

*Last Updated: 2026-09-18*


