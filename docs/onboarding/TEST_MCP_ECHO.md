# Testing the MCP Echo Tool

## Quick Test in Cursor

Since you've restarted the MCP server, the easiest way to test is directly in Cursor:

### Method 1: Ask Cursor to Use the Tool

Simply ask Cursor:
```
"Call the echo tool with the message 'Hello, MCP!'"
```

Or:
```
"Use the MCP echo tool to echo back 'Testing 123'"
```

### Method 2: Check Available Tools

Ask Cursor:
```
"What MCP tools are available?"
```

You should see `echo` in the list.

### Method 3: Test Event Subscription

1. Ask Cursor to subscribe to events:
   ```
   "Subscribe to MCP events for tool.started and tool.completed"
   ```

2. Then call the echo tool:
   ```
   "Call the echo tool with message 'Test events'"
   ```

3. Check if you received event notifications

## Manual Testing (Advanced)

If you want to test manually via command line:

```bash
# Start the server and send requests
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./zqk mcp serve
```

Or use the simple test script:
```bash
./scripts/test_echo_simple.sh | ./zqk mcp serve
```

## What to Expect

### Successful Echo Tool Call

**Request**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "echo",
    "arguments": {
      "message": "Hello, MCP!"
    }
  }
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"echo\":\"Hello, MCP!\",\"status\":\"success\",\"message\":\"Echo: Hello, MCP!\"}"
      }
    ]
  }
}
```

### Event Notifications

After subscribing and calling the echo tool, you should receive:

1. `tool.started` notification
2. `tool.completed` notification

## Troubleshooting

### Tool Not Found

If the echo tool doesn't appear:
- Verify the server was restarted after building
- Check that `RegisterAllTools()` is called in `cmd/zqk/mcp/mcp.go`
- Look at trace logs: `.zqk/mcp/logs/mcp-trace.log`

### No Events Received

- Ensure you subscribed before calling the tool
- Check that the server's `eventEmitter` is initialized
- Verify notifications are being read (they come as JSON-RPC notifications, not responses)

## Next Steps

Once basic communication works:
1. Test with multiple event types
2. Test subscription/unsubscription
3. Test with multiple tools
4. Verify event filtering works correctly

