# ZQK Community Quickstart & MCP

Command examples use the default executable token. `make` writes `./bin/<brand.executable_name>`. Kernel data stays under **`.zqk/`**. There is no standalone `zqk-mcp`.

```sh
./bin/zcom quickstart
./bin/zcom system agent-onboard
./bin/zcom system init --project-name my-project   # greenfield only
```

See [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md) for the full first-run sequence.

## Connect your AI agent (MCP)

```sh
./bin/zcom mcp install
```

That writes `.cursor/mcp.json` (or other detected IDE config). Restart the IDE.

Manual Cursor config:

```json
{
  "mcpServers": {
    "zcom": {
      "command": "/absolute/path/to/bin/zcom",
      "args": ["mcp", "cursor-adapter"]
    }
  }
}
```

Claude Desktop / other stdio hosts:

```json
{
  "mcpServers": {
    "zcom": {
      "command": "/absolute/path/to/bin/zcom",
      "args": ["mcp", "serve"]
    }
  }
}
```

Verify:

```sh
./bin/zcom mcp list-tools
```

Then ask the agent something that requires project context (goals, policies, next work). It should use kernel MCP tools, not chat memory.
