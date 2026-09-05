# ZQK Quickstart & MCP Configuration Guide

Your AI agents are coding blind. ZQK gives them project awareness and guardrails.

This guide explains how to connect your AI agent (Claude, Cursor, VS Code, etc.) to your ZQK project so it can check policies, read architecture decisions, and stay aligned with your goals.

## 1. Setup ZQK

Run the quickstart command in your project directory. This creates starter policies and prepares the MCP server configuration.

```sh
zqk quickstart
```

## 2. Connect Your AI Agent

ZQK uses the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to securely expose your project context to AI agents.

Copy the appropriate configuration snippet below and paste it into your AI tool's MCP configuration file.

### Cursor

Add this to `.cursor/mcp.json` in your project root:

```json
{
  "mcpServers": {
    "zqk": {
      "command": "zqk-mcp",
      "args": ["stdio"]
    }
  }
}
```

### Claude Desktop

Add this to your `claude_desktop_config.json` (located at `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "zqk": {
      "command": "zqk-mcp",
      "args": ["stdio"]
    }
  }
}
```

### VS Code (Continue)

Add this to your `~/.continue/config.json`:

```json
{
  "experimental": {
    "modelContextProtocolServers": [
      {
        "transport": {
          "type": "stdio",
          "command": "zqk-mcp",
          "args": ["stdio"]
        }
      }
    ]
  }
}
```

## 3. Verify Connection

Once connected, ask your AI agent a question that requires project context. For example:

- *"Check if adding a function without tests complies with project policies."*
- *"What are the active goals for this project?"*
- *"Summarize our project context."*

The agent will automatically use the `get_project_context` tool to read your policies and goals and respond accurately.
