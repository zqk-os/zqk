# MCP Proxy Daemon Architecture

## The Problem
The ZQK ecosystem heavily integrates with modern AI IDEs (such as Cursor) via the Model Context Protocol (MCP). By default, MCP operates over a standard input/output (`stdio`) stream.
However, during active development of the ZQK operating system, the underlying binaries (`zqk` or `zqk-mcp`) are frequently recompiled via `make build-all`.
When the binary terminates, the IDE strictly detects the broken `stdio` pipe, assumes a fatal failure, and drops the MCP session entirely. This severely interrupts the developer experience, forcing manual IDE window reloads to re-establish the connection.

## Operator recipe (Cursor — required ship path)

Native Go only — do **not** wrap these in a required shell script.

1. Start the heavy daemon (TCP):
   ```bash
   ./bin/zqk mcp daemon --tcp 127.0.0.1:8443
   ```
2. Point Cursor MCP config at the **proxy** role symlink (stable stdio), not the fat daemon:
   ```json
   {
     "mcpServers": {
       "zqk": {
         "command": "/absolute/path/to/bin/zqk-mcp-proxy",
         "args": ["mcp", "proxy", "--tcp", "127.0.0.1:8443"],
         "cwd": "/absolute/path/to/repo"
       }
     }
   }
   ```
   `bin/zqk-mcp-proxy` and `bin/zqk-mcp-daemon` are symlinks to the CLI so `ps` shows the role, not bare `zqk`. Studio: [`.cursor/mcp.json`](../../../.cursor/mcp.json); created by `mcp ensure` / IDE install.
3. Steer / chat without paste:
   - MCP tool `chat_send`, or
   - `./bin/zqk feed steer --message "…"`, or
   - `./bin/zqk feed emit-status --role tpm --summary "…"`
4. After rebuilds, restart **only** the daemon; leave the proxy (and Cursor session) up. Reconnect + initialize replay is covered by `pkg/mcp/proxy_forwarder_test.go` (including tools/call after reconnect).

## The Proxy Architecture
To achieve autonomous continuity and unblock developer velocity, the ZQK MCP connection utilizes an intermediate proxy layer.

1. **`zqk mcp proxy` (The Shim):**
   The IDE is configured to run `./bin/zqk mcp proxy` rather than connecting directly to the server. This shim is an extremely lightweight, highly stable binary that **never** terminates during a standard rebuild. It binds to the IDE's `stdio` and maintains the persistent parent session.

2. **JSON-RPC Frame Forwarding:**
   The Proxy Forwarder takes incoming JSON-RPC frames from `stdin` and transparently routes them over a resilient TCP socket to the background MCP server (default: `127.0.0.1:8443`).

3. **Background Daemon:**
   The heavy-lifting MCP server runs dynamically in the background. It handles the actual tool invocations, graph database mutations, and context reads.

## The Heartbeat Protocol & State Caching
When the background daemon is rebuilt and restarted, the TCP socket will momentarily drop. The proxy does **not** panic. Instead, it enters a non-blocking reconnect loop.
1. **Initialize Payload Caching:** The proxy intercepts and explicitly caches the original JSON-RPC `initialize` frame sent by the IDE.
2. **Re-Handshake:** The moment the background daemon comes back online, the proxy automatically replays the cached `initialize` frame over the new TCP connection.
3. **Transparent Recovery:** The IDE is completely unaware that the underlying daemon was swapped out. The MCP capabilities continue functioning instantly.

## Testing & Reliability
This critical architecture is objectively verified. The swarm explicitly implemented a robust unit and integration test suite (`pkg/mcp/proxy_test.go`, `pkg/mcp/proxy_forwarder_test.go`, `pkg/proxy/daemon_test.go`) covering:
- Transparent frame routing.
- Safe concurrency utilizing the `concurrency.InterruptChecker`.
- Reconnect resilience and TCP socket closure scenarios.
- Embedded heartbeats that prevent the `stdio` pipe from timing out.
- `tools/call` (e.g. `zqk_chat_send`) forwarded after daemon restart (`TestProxyForwarder_ReconnectForwardsToolsCall`).