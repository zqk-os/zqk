# Testing the Echo Tool

**Status**: Ready to Test  
**Purpose**: Verify that the MCP echo tool is discoverable and callable by AI agents.

## Quick Test

The echo tool is registered and should appear in the tools list. To verify it works:

### 1. Check Tools List

Ask Cursor:
```
What MCP tools are available?
```

You should see `echo` in the list.

### 2. Call the Echo Tool

Ask Cursor:
```
Call the echo tool with message "Hello, MCP!"
```

Or:
```
Use the MCP echo tool to echo back "test message"
```

### 3. Expected Response

The echo tool should return:
```json
{
  "echo": "Hello, MCP!",
  "status": "success",
  "message": "Echo: Hello, MCP!"
}
```

## How MCP Tools Work

1. **Discovery**: AI agents call `tools/list` to see available tools
2. **Description**: Each tool has a `description` that tells the agent what it does
3. **Schema**: The `inputSchema` tells the agent what parameters are needed
4. **Execution**: The agent calls `tools/call` with the tool name and arguments

**No prompts or resources needed** - the tool's description and schema are sufficient for the agent to understand and use it.

## Troubleshooting

- **Echo tool not found**: Ensure the server was restarted after building the binary
- **Tool call fails**: Check the MCP trace log (`.zqk/mcp/logs/mcp-trace.log`) for errors
- **Agent doesn't see tool**: The agent may need to refresh its tools cache - try asking it to list tools again

## Manual Test Script

Run the test script:
```bash
./scripts/test_echo_simple.sh
```

This will:
1. Initialize the MCP server
2. List all available tools (verify echo is present)
3. Call the echo tool
4. Display the server's response

