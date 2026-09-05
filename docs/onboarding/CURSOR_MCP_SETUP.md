# Cursor MCP Configuration

This guide explains how to configure Cursor to use the zqk MCP server with the stable binary and specialized config.

## Configuration Location

Cursor 2026 MCP controls live under **Customize** (sidebar) → MCP — not the old Settings → MCP panel.

Config files (both are loaded; project key wins on conflict):

| Scope | Path |
|-------|------|
| Project | `.cursor/mcp.json` |
| Global (Customize default write target) | `~/.cursor/mcp.json` |

`zqk mcp ensure` writes **both**, with `"type": "stdio"` (required by current Cursor stdio schema). An empty global `mcpServers: {}` after the UI move leaves the connector red even when the project file is valid.

**Note**: Cursor uses `.cursor/mcp.json` instead of a repo-root `mcp.json`.

## Configuration Format

```json
{
  "mcpServers": {
    "zqk": {
      "type": "stdio",
      "command": "/ABS/PATH/TO/zqk/bin/zqk-mcp-ide-adapter",
      "args": ["mcp", "ide-adapter", "--tcp", "127.0.0.1:8443"],
      "cwd": "${workspaceFolder}",
      "env": {
        "ZQK_MCP_CONFIG_PATH": "${workspaceFolder}/.zqk/mcp/config.yaml",
        "ZQK_PROJECT_ROOT": "${workspaceFolder}"
      }
    }
  }
}
```

## Setup Steps

### 1. Build Stable Binary

```bash
mkdir -p bin
go build -o bin/zqk-stable ./cmd/zqk
```

### 2. Verify Binary

```bash
./bin/zqk-stable --help
```

### 3. Create Cursor Configuration

The `.cursor/mcp.json` file is already created. It uses a wrapper script (`scripts/mcp-server.sh`) that:
- Uses the stable binary (`./bin/zqk-stable`)
- Loads the MCP config from `.zqk/mcp/config.yaml`
- Sets proper environment variables

### 4. Reload Cursor

After creating or updating `.cursor/mcp.json`:
1. **Reload Cursor window**: Cmd+Shift+P → "Developer: Reload Window"
2. Or **restart Cursor completely**

## How It Works

1. **Wrapper Script**: `scripts/mcp-server.sh` handles binary discovery and environment setup
2. **Stable Binary**: Uses `./bin/zqk-stable` instead of the development binary
3. **Config File**: The MCP server reads `.zqk/mcp/config.yaml` for command exposure settings
4. **Selective Exposure**: Only commands listed in the config are exposed via MCP
5. **Environment Variables**: 
   - `ZQK_STABLE_BINARY_PATH`: Points to stable binary
   - `ZQK_MCP_CONFIG_PATH`: Points to MCP config file
   - `ZQK_PROJECT_ROOT`: Project root directory

## Verifying Configuration

### Check MCP Server Status

In Cursor:
1. Open **Customize** in the sidebar → **MCP** (or Command Palette → MCP)
2. Confirm **zqk** is listed and the status dot is green
3. Toggle off/on once after editing `mcp.json` so Cursor re-probes
4. Output panel → **MCP Logs** if spawn fails (`ENOENT` = missing `bin/zqk-mcp-ide-adapter`; run `zqk mcp ensure`)

### Test MCP Tools

Once connected, you should see these MCP tools available:
- `cli_object_list` - List objects
- `cli_object_get` - Get object details
- `cli_object_count` - Count objects
- `cli_system_status` - System status
- `cli_system_check` - System health check
- `cli_system_validate` - Validate objects
- `cli_reports_pcs` - PCS report
- `cli_reports_edd` - EDD report
- `cli_reports_blockers` - Blockers report
- `cli_docman_list` - List documentation

These match the `exposed_commands` in `.zqk/mcp/config.yaml`.

## Troubleshooting

### Binary Not Found

If you get "binary not found" errors:
1. Verify binary exists: `test -f ./bin/zqk-stable`
2. Check binary is executable: `chmod +x ./bin/zqk-stable`
3. Verify wrapper script: `test -f ./scripts/mcp-server.sh && chmod +x ./scripts/mcp-server.sh`

### Config Not Found

If MCP server can't find config:
1. Verify config exists: `test -f .zqk/mcp/config.yaml`
2. Check `ZQK_MCP_CONFIG_PATH` in `.cursor/mcp.json`
3. Verify path is relative to workspace root

### Commands Not Available

If expected commands aren't available:
1. Check `.zqk/mcp/config.yaml` `exposed_commands` list
2. Verify command is not in `blocked_commands`
3. Check binary includes the command: `./bin/zqk-stable <command> --help`

### Server Not Starting

If the MCP server doesn't start:
1. Check wrapper script is executable: `chmod +x scripts/mcp-server.sh`
2. Test wrapper script manually: `./scripts/mcp-server.sh` (should wait for stdin)
3. Check Cursor's MCP logs/console for errors

### Server Shutdown and Restart

If the server hangs or needs to be safely restarted:
1. You can stop the server cleanly by executing the `server_shutdown` tool in Cursor.
2. Alternatively, you can send an interrupt signal to the wrapper script (e.g., `Ctrl+C` if running manually) or kill the process in your OS. The server is designed to intercept OS signals (`SIGINT`, `SIGTERM`) and gracefully flush final responses before shutting down to prevent corrupted sessions.
3. Once stopped, Cursor will automatically attempt to restart the server on the next interaction.

## Updating Configuration

### Adding a New Command

1. Edit `.zqk/mcp/config.yaml`
2. Add command to `exposed_commands` list
3. Reload Cursor window
4. Verify command appears in MCP tools

### Updating Stable Binary

1. Build new binary: `go build -o bin/zqk-stable ./cmd/zqk`
2. Update version in `.zqk/mcp/config.yaml` if needed
3. Reload Cursor window
4. Test exposed commands

## Why `.cursor/mcp.json` Instead of `mcp.json`?

Cursor supports project-specific MCP configuration via `.cursor/mcp.json`. This allows:
- **Project-Specific Config**: Each project can have its own MCP setup
- **Version Control**: Configuration is tracked in git
- **Team Consistency**: All team members use the same MCP configuration
- **Isolation**: Different projects can use different MCP servers/configs

## Configuring Authentication

The zqk MCP server supports keystore-based authentication for secure access. Cursor is configured to use keystore authentication automatically.

### Keystore Authentication (Recommended)

The `.cursor/mcp.json` file is configured with `ZQK_MCP_KEYSTORE_KEY_ID` set to `KEY-002`, which corresponds to the `account:cursor-vscode` account.

**How it works:**
1. The MCP server reads `ZQK_MCP_KEYSTORE_KEY_ID` from the environment
2. It loads the keystore entry (e.g., `KEY-002`)
3. Validates the key is not revoked or expired
4. Resolves the account (`account:cursor-vscode`)
5. Extracts roles and permissions from the account object
6. Creates an authenticated session

**Current Configuration:**
```json
{
  "mcpServers": {
    "zqk": {
      "env": {
        "ZQK_MCP_KEYSTORE_KEY_ID": "KEY-002"
      }
    }
  }
}
```

**To use a different account:**
1. Create a keystore entry: `zqk keystore create --account-id account:your-account --key-type api_key --credential "your-key" --title "Your Key"`
2. Note the key ID (e.g., `KEY-003`)
3. Update `.cursor/mcp.json` to use the new key ID

### Alternative: Role Elicitation

If keystore authentication is not configured, the server will use **role elicitation**:

1. **Allow the connection** (cursor-vscode is recognized as a human IDE client)
2. **Elicit roles/permissions** - The server will ask you to provide:
   - `roles`: Your role(s) (e.g., `["developer"]`, `["admin"]`)
   - `account_id`: Your account ID (e.g., `"account:cursor-vscode"`)
   - `permissions`: Optional custom permissions

### Pre-Registered Account

The `account:cursor-vscode` account is pre-registered:

1. **The account exists**: `docs/process/accounts/account-cursor-vscode.yaml`
2. **It's in the registry**: `.zqk/mcp/config.yaml` has `account:cursor-vscode` configured
3. **It has the `developer` role** with appropriate permissions
4. **Keystore key exists**: `KEY-002` is configured for this account

## Related Documentation

- [MCP Configuration Guide](./MCP_CONFIGURATION.md)
- [MCP Binary Stability Architecture](../process/architecture/mcp-binary-stability-v1.0.md)
- [POL-ARCH-003](../process/policies/POL-ARCH-003.yaml): MCP Binary Stability Policy
- [MCP Role Elicitation](../process/architecture/MCP_ROLE_ELICITATION.md)
