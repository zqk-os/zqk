# MCP Client Pattern for Programmatic Use

**Last Verified:** 2026-08-31


**Status**: Active  
**Created**: 2026-06-24  
**Purpose**: Document the Go-native MCP Client execution pattern for programmatically invoking MCP tools.

## Overview

The Model Context Protocol (MCP) is typically used by desktop clients (like Claude Desktop) to communicate with local tools. However, in the ZQK ecosystem, we often need to invoke MCP tools programmatically via Go, especially within autonomous agents and the swarm orchestration layer. 

The `pkg/swarm/mcp_client.go` package provides the `MCPExecutor` which handles starting a native MCP server process and routing LLM tool calls to it seamlessly.

## Architecture

The `MCPExecutor` encapsulates the lifecycle of an MCP server binary:
1. **Process Management**: It starts the specified MCP binary (e.g., `zqk-mcp`), piping `stdin` and `stdout` to communicate over the standard standard I/O transport.
2. **Client Initialization**: It creates an `mcp.Client`, connects it to the standard I/O pipes, and sends the `Initialize` request with the required protocol version.
3. **Tool Discovery**: It queries the server for available tools (`ListTools`) and maps their schemas to `llm.ToolDefinition` structures.
4. **Tool Execution**: It routes specific `llm.ToolCall` requests to the MCP server via `CallTool` and returns the string result.

## Usage Pattern

The standard pattern for using the `MCPExecutor` involves initializing it with a context and the path to the MCP server binary, fetching the tools, executing tool calls as needed, and deferring the `Close()` method to clean up the process.

### Full Code Example

```go
package example

import (
	"context"
	"fmt"
	"log"

	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/swarm"
)

func RunProgrammaticMCP() {
	ctx := context.Background()

	// 1. Initialize the MCP Executor
	// Ensure the path points to your built MCP server binary.
	mcpPath := "./bin/zqk-mcp" 
	executor, err := swarm.NewMCPExecutor(ctx, mcpPath)
	if err != nil {
		log.Fatalf("Failed to initialize MCP executor: %v", err)
	}
	// Always defer Close to ensure the subprocess is terminated.
	defer executor.Close()

	// 2. Discover Available Tools
	tools, err := executor.GetTools(ctx)
	if err != nil {
		log.Fatalf("Failed to get tools: %v", err)
	}

	fmt.Printf("Discovered %d tools from MCP server.\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - Tool: %s\n", t.Name)
	}

	// 3. Execute a Tool Call
	// In practice, this ToolCall object usually comes from the LLM's response.
	call := llm.ToolCall{
		Name: "zqk_object_list",
		Arguments: `{"kind":"policy"}`, // JSON string of arguments
	}

	fmt.Printf("Executing tool: %s\n", call.Name)
	result, err := executor.ExecuteToolCall(ctx, call)
	if err != nil {
		log.Fatalf("Tool execution failed: %v", err)
	}

	fmt.Printf("Tool Result:\n%s\n", result)
}
```

## Best Practices

1. **Lifecycle Management**: Always `defer executor.Close()` immediately after successful initialization to prevent orphan processes.
2. **Context Cancellation**: Pass a `context.Context` to handle timeouts, especially since tool execution duration can vary. The executor handles process cancellation if the context expires.
3. **Arguments Formatting**: Ensure that `call.Arguments` is a valid JSON string that matches the tool's expected input schema. The `ExecuteToolCall` method will automatically unmarshal it into the `map[string]any` structure expected by the MCP protocol layer.
4. **Error Handling**: Network/Pipe errors or initialization failures will return wrapped errors. Always check errors from `NewMCPExecutor`, `GetTools`, and `ExecuteToolCall`.
