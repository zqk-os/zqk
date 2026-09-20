# ZQK Community Quickstart & MCP

Command examples use the default executable token. `make` writes `./bin/<brand.executable_name>`. Kernel data stays under **`.zqk/`**. There is no standalone `zqk-mcp`.

```sh
./bin/zqk quickstart
./bin/zqk system agent-onboard
./bin/zqk system init --project-name my-project   # greenfield only
```

See [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md) for the full first-run sequence.

## Connect your AI agent (MCP)

```sh
./bin/zqk mcp install
```

That writes `.cursor/mcp.json` (or other detected IDE config). Restart the IDE.

Manual Cursor config:

```json
{
  "mcpServers": {
    "zqk": {
      "command": "/absolute/path/to/bin/zqk",
      "args": ["mcp", "cursor-adapter"]
    }
  }
}
```

Claude Desktop / other stdio hosts:

```json
{
  "mcpServers": {
    "zqk": {
      "command": "/absolute/path/to/bin/zqk",
      "args": ["mcp", "serve"]
    }
  }
}
```

Verify:

```sh
./bin/zqk mcp list-tools
```

Then ask the agent something that requires project context (goals, policies, next work). It should use kernel MCP tools, not chat memory.
