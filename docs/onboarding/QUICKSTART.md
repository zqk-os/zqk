# ZQK Quickstart & MCP Configuration Guide

Your AI agents are coding blind. ZQK gives them project awareness, architecture decisions, and guardrails.

This guide explains how to connect your AI agent (Cursor, Claude Desktop, VS Code, etc.) to your ZQK project so it can verify policies, query the knowledge kernel, and stay aligned with your goals.

## 1. Setup ZQK

Run the quickstart command in your project directory. This guides you through project initialization, kernel inspection, starter policies, and workflow discovery:

```sh
zqk quickstart
```

For automated agent workspace priming and seating:
```sh
zqk system agent-onboard
```
See [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md) for full agent host detection details.

For greenfield initialization in a fresh directory:
```sh
mkdir my-project && cd my-project
zqk init
```

## 2. Connect Your AI Agent

ZQK uses the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to securely expose your project context to AI agents.

### Automated Setup (Recommended)

Run the automated MCP installer from your project root:

```sh
zqk mcp install
```

This automatically writes the proper configuration to `.cursor/mcp.json` or other detected IDE directories. Restart your IDE after installation for changes to take effect.

### Manual Configuration

If you prefer manual configuration, use the snippets below for your AI tool:

#### Cursor

Add this to `.cursor/mcp.json` in your project root:

```json
{
  "mcpServers": {
    "zqk": {
      "command": "zqk",
      "args": ["mcp", "cursor-adapter"]
    }
  }
}
```

#### Claude Desktop

Add this to your Claude Desktop configuration (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "zqk": {
      "command": "zqk",
      "args": ["mcp", "serve"]
    }
  }
}
```

#### VS Code (Continue / Roo Code)

Add this to your extension's MCP configuration:

```json
{
  "experimental": {
    "modelContextProtocolServers": [
      {
        "transport": {
          "type": "stdio",
          "command": "zqk",
          "args": ["mcp", "serve"]
        }
      }
    ]
  }
}
```

## 3. Verify Connection

Once connected, verify the MCP connection from the terminal:

```sh
zqk mcp list-tools
```

Then ask your AI agent a question in your IDE that requires project context, for example:

- *"Check if adding a function without tests complies with project policies."*
- *"What are the active goals for this project?"*
- *"Summarize our project context."*

The agent will automatically invoke the `get_project_context` tool to read your policies and goals and respond accurately.

## 4. Discover, Execute & Mint Work

### Discover Active Tasks
Query the active priority plan and shovel-ready backlog items:
```sh
zqk workflow whats-next
```

### Autonomous Single-Command Execution (`zqk do`)
Claim and execute the next prioritized backlog item end-to-end with AST verification:
```sh
zqk do
```
Or target a specific backlog item:
```sh
zqk do BLI-001
```

### Mint New Goals & Tasks (`zqk new <kind>`)
Mint new objects directly onto the safe Draft Plane:
```sh
zqk new goal --title "Build User Authentication"
zqk new plan --title "Sprint 1: OAuth & JWT"
zqk new req --title "Support GitHub OAuth login"
zqk new bli --title "Implement GitHub OAuth handler"
```

## 5. Run Swarms & Live Telemetry

Execute portable multi-agent swarms with fail-closed membrane isolation:

```sh
# Launch the visual Web Studio (DAG graph & Gantt timeline)
zqk ui -w

# Run a remote or local swarm package
zqk run https://github.com/zqk-os/swarm-starter-kit

# Launch interactive full-screen terminal mission control
zqk ui

# Stream real-time mutations with the visual ANSI seismograph
zqk state stream --dashboard

# Inspect the Knowledge Kernel hierarchy
zqk state tree
```
