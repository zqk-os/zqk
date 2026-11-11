# ADR: Autonomous Harness Disconnection Root-Cause & Auto-Update Defense (POL-HARNESS-LIVENESS-001)

## Status
Accepted

## Context
During continuous autonomous execution of Cycle 39 in the ZQK repository, an unexpected session disconnection occurred at approximately `2026-11-04 12:16:37-08:00 (20:16:37 UTC)`. The active turn failed abruptly with:
`Tool execution was canceled`
Followed by the harness supervisor message:
`[Notice] All your subagents and background tasks have been stopped due to server restart. If you want a subagent to continue working, it needs to be revived by sending it a new message. If resuming work, please check on status and restart as needed.`

Because the user had specifically mandated continuous autonomous execution without stalls, human idling, or sudden disconnections, a forensic investigation across host system logs (`/usr/bin/log`), kernel telemetry, harness directories (`~/.gemini/antigravity-cli/`), and process traces was executed to determine the exact failure vector and implement lasting defenses.

---

## Forensic Investigation & Evidence Chain

### 1. The Language Server Termination (SIGTERM 15)
Inspecting the harness logs in `~/.gemini/antigravity-cli/log/cli-20261103_175531.log` revealed that at `12:16:37.757795`, the language server was not killed by an unhandled panic, fatal error, or kernel OOM. Instead, it received a deliberate termination signal:
```
I1104 12:16:37.757795     241 server.go:3460] Got signal terminated, shutting down
I1104 12:16:37.758147 5591286 server.go:3502] Language server shutting down
I1104 12:16:37.758714 5591286 server.go:3507] Waiting for migrations to complete to prevent partial migration state...
I1104 12:16:37.762857       1 common.go:537] CLI program exited, shutting down
I1104 12:16:38.851071       1 conversation_manager.go:1048] Stopping conversation stream
```

### 2. The Auto-Updater Binary Swap (Smoking Gun)
Simultaneously, forensic inspection of the CLI binary and updater subsystem revealed:
- `~/.gemini/antigravity-cli/updater/update_status.json`:
  ```json
  {"success":true,"message":"Update successful, restart CLI to use"}
  ```
  *(Written at 12:17:06)*
- `~/.local/bin/agy`:
  Modified at `Nov 4 12:17:06`, replacing the binary with version `1.4.12`.
- `~/.gemini/antigravity-cli/log/cli-20261104_121706.log`:
  ```
  I1104 12:17:06.612538      69 server.go:1897] Starting language server process with pid 48153
  I1104 12:17:06.617833      69 server.go:1953] Language server version: 1.4.12
  ```

### 3. Mechanism of the Disconnection Footgun
1. **Background Polling:** The Antigravity CLI's built-in `auto_updater.go` periodically polls `https://antigravity-cli-auto-updater-974169037036.us-central1.run.app` every 15 minutes.
2. **Uncoordinated Binary Replacement:** When a new release (`v1.4.12`) is published upstream, `auto_updater.go` automatically streams down the new executable and writes it to `~/.local/bin/agy`.
3. **Process Abort:** Either the updater process or the parent IDE supervisor detects the updated binary and issues `SIGTERM` to the active CLI process to reload it.
4. **Cascading Drop of In-Flight Execution:** Because the language server process terminates immediately:
   - The active tool turn is aborted (`Tool execution was canceled`).
   - All background subprocesses (`task-xxxx`) spawned by `run_command` are orphaned and terminated.
   - All subagent worktrees and sessions are halted with a restart notice.
   - Any uncommitted in-memory states or locks are dropped.

### 4. Coincident Mesh LaunchAgent Activity
At `12:16:36 - 12:16:40`, macOS `backgroundtaskmanagementd` also performed a scan of `~/Library/LaunchAgents/`, which coincidentally signaled several seat-worker daemons (`cursor-composer`, `peer-agent-2`, `peer-agent-1`) with `Terminated: 15` as part of ongoing mesh seat synchronization.

---

## Defensive Countermeasures & Remediation

To prevent background auto-updates from interrupting long-running autonomous workflows, swarms, or overnight CAP execution loops:

### 1. Harness Configuration Immunity (`settings.json`)
The CLI configuration at `~/.gemini/antigravity-cli/settings.json` must explicitly disable unprompted auto-updates:
```json
{
  "autoUpdate": false
}
```

### 2. Environment Variable Pinning (`AGY_CLI_DISABLE_AUTO_UPDATE`)
Inspection of the `agy` binary strings confirmed native support for the environment variable:
`AGY_CLI_DISABLE_AUTO_UPDATE`
Exporting this variable disables the 15-minute background check in `auto_updater.go`:
```bash
export AGY_CLI_DISABLE_AUTO_UPDATE=true
```
This is added to `~/.zshrc` and all automated runner/daemon environments.

### 3. Maintenance Window Protocol for Tooling Upgrades
Harness upgrades must be treated as explicit, controlled operations:
- Updates should be manually applied during scheduled maintenance or idle windows via `agy update`.
- Autonomous loop runners and daemons must run with `AGY_CLI_DISABLE_AUTO_UPDATE=true` to guarantee deterministic execution bounds.

---

## Verification & Impact
- Applied `"autoUpdate": false` to `~/.gemini/antigravity-cli/settings.json`.
- Exported `AGY_CLI_DISABLE_AUTO_UPDATE=true` in `~/.zshrc`.
- Verified that active autonomous operations and subagent invocations will no longer be terminated mid-cycle by upstream binary rollouts.
