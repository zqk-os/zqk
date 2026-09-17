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

See [MCP CLI Bridge Architecture v1.0](../../docs/architecture/mcp-cli-bridge-v1.0.md) for detailed information.

### Server Shutdown and OS Signal Handling

The MCP server gracefully handles termination to prevent data loss or corrupted states:
- **OS Signals**: The server listens for `SIGINT` and `SIGTERM`. When received, it initiates a graceful shutdown sequence.
- **Shutdown Tool**: Clients can request a graceful shutdown via the `server_shutdown` tool.
- **Graceful Shutdown**: The shutdown sequence flushes remaining messages, stops receiving new events, and safely closes active resource handles.

## Documentation

Detailed MCP documentation is available in:
- [docs/architecture/mcp/](../../docs/architecture/mcp/) - Active architecture documentation
- [docs/archive/mcp/](../../docs/archive/mcp/) - Archived implementation and refactoring documents

### Metrics Tools

Metrics tools (PCS, EDD, D&B) are exposed via the CLI bridge:

- **CLI Commands**: Metrics are accessed through CLI commands (e.g., `zqk reports pcs`, `zqk reports edd`, `zqk reports blockers`)
- **Automatic MCP Exposure**: The CLI bridge automatically exposes these commands as MCP tools
- **No Special Bridge Needed**: Metrics tools use the same CLI bridge pattern as all other commands
- **Consistent Architecture**: All functionality is exposed through CLI commands, which are automatically available via MCP

## Quick Start

```go
import "github.com/lanceman/zqk/pkg/mcp"

server := mcp.NewServer()
mcp.RegisterGraphTools(server)
server.SetRootCommand(rootCmd)
server.SetSecurityContext(secCtx)
server.Serve()
```

See [MCP CLI Bridge Architecture v1.0](../../docs/architecture/mcp-cli-bridge-v1.0.md) for detailed usage examples and configuration.

## Documentation

Architecture documentation for the MCP server is located in the project documentation tree:

- **[MCP CLI Bridge Architecture v1.0](../../docs/architecture/mcp-cli-bridge-v1.0.md)**: Context-driven CLI command exposure via MCP with security filtering
- **[MCP Privilege Tests v1.0](../../docs/architecture/mcp-privilege-tests-v1.0.md)**: Comprehensive test suite validating privilege-based access control

### Server Modes and Trimming

- **Full server** (`zqk mcp serve`): Has the full cobra CLI tree; discovers and exposes all leaf commands as tools, plus built-in tools, prompts, and resources. This can exceed client limits (e.g. Cursor’s 40-tool warning / 80-tool max).
- **mcp-simple** (`bin/zqk-mcp`): No CLI tree; only built-in tools, prompts, and resources are registered. Tool execution runs the full `zqk` binary in a subprocess per call.
- **Plan**: Move back to the full server once tools, prompts, and resources are trimmed/organized (e.g. allowlists or profiles) so we stay within client limits. See **[MCP Exposure and Trimming](docs/MCP_EXPOSURE_AND_TRIMMING.md)** for the audit, limits, and curation sketch.

### Related Documentation

- [Graph Provider Interfaces](../../graph/provider/README.md)
- [MemGraph Implementation](../../graph/memgraph/README.md)
- [BLI-622: Expand MCP Server with Graph Traversal Tools](../../.zqk/process/backlog/BLI-622.yaml)
- [Security Context Package](../../context/README.md)
- [MCP Exposure and Trimming](docs/MCP_EXPOSURE_AND_TRIMMING.md) — audit, client limits, and trimming approach

---

*Last Updated: 2025-01-02*

