# Stable Binary Management Guide

This guide explains how to manage the stable MCP / scheduler binary and when to update it.

## Footgun: never overwrite a running stable binary in place

**Incident (2026-08-04):** `cp bin/zqk .zqk/bin/zqk-stable` (or `go build -o .zqk/bin/zqk-stable`) while the scheduler daemon was still executing that path produced **`zsh: killed`** / exit **137** on later launches. Truncating a Mach-O inode that is mapped for execution corrupts the image.

**Always use the guarded installer:**

```bash
./scripts/install-zqk-stable.sh --build
# or, with tip already built:
./scripts/install-zqk-stable.sh --from bin/zqk
# if the daemon holds the file (Makefile promote-stable always does this):
./scripts/install-zqk-stable.sh --stop-scheduler --from bin/zqk
make promote-stable   # stops scheduler+MCP holders, installs, recycles daemons
```

The script refuses when `lsof`/`pgrep` show holders, writes via **temp + `mv`** (new inode), and installs both `bin/zqk-stable` and `.zqk/bin/zqk-stable` by default. TRACK: `TDE-1785808957221945000-fcd15e47`.

Do **not**:
- `cp -f` / `go build -o` directly onto `.zqk/bin/zqk-stable` or `bin/zqk-stable` while the daemon/MCP is running
- Rely on `rm` + `cp` ad hoc from agents without the holder check

## Quick Reference

### When to Update

**Must Update**:
- MCP protocol changes
- Bug fixes in MCP or exposed commands
- Security fixes
- New exposed commands added

**Should Update**:
- Performance improvements
- Logging improvements
- Compatibility improvements

**No Update Needed**:
- Non-MCP code changes
- Unexposed command changes
- Development-only features

### Update Process

```bash
# Preferred (non-interactive safe install)
./scripts/install-zqk-stable.sh --build --stop-scheduler

# Interactive versioned MCP update flow
./scripts/update-stable-binary.sh

# Make (promote-stable stops holders + recycles scheduler/MCP)
make zqk && make promote-stable
# or full tree:
make build-all
```

After install, **cycle every long-lived consumer of stable** so they exec the new inode. Scheduler alone is **not** enough — MCP TCP daemon, PrivilegedWriter (`object daemon`), mesh seat-workers, and IDE adapters keep the **old inode** until restarted/kickstarted.

`mcp ensure` is **idempotent**: if something is already listening on `:8443`, it will **not** restart that process. Always kill-then-ensure (or use the recycle script).

### Mandatory post-promote daemon cycle (all consumers)

Preferred (what `make promote-stable` runs):

```bash
./scripts/recycle-stable-daemons.sh
# Confirm: ./bin/zqk-stable feed doctor --format json → mcp_subscribers >= 1
# Confirm etimes are fresh for scheduler, mcp-daemon, object daemon, seat-workers:
#   ps -eo etime,command | rg 'zqk-stable|zqk-mcp-daemon|seat-worker|object daemon'
```

Manual equivalent:

```bash
# 1) Scheduler
./bin/zqk-stable scheduler stop --wait && ./bin/zqk-stable scheduler start

# 2) MCP — kill first; ensure alone keeps a stale listener
pkill -TERM -f 'mcp daemon --tcp' || true; sleep 1
./bin/zqk-stable mcp ensure --tcp 127.0.0.1:8443
./bin/zqk-stable mcp list-tools

# 3) PrivilegedWriter + seat-workers (launchd KeepAlive)
launchctl kickstart -k "gui/$(id -u)/com.zqk.privileged-writer"
launchctl kickstart -k "gui/$(id -u)/com.zqk.mesh.seat-worker.antigravity-1"
launchctl kickstart -k "gui/$(id -u)/com.zqk.mesh.seat-worker.antigravity-2"

# 4) IDE reconnect
python3 scripts/cursor/ide-bridge-request.py --command zqk.mcp.reloadClient

# Optional ops snapshot
./scripts/mesh/ops-background-status.sh
```

**Neglect pattern (2026-08-10):** promote + scheduler restart left `127.0.0.1:8443` down → mesh wake / COMMS-CHECK transport yellow until `mcp ensure`. Treat MCP as a **required** sibling of the scheduler whenever stable is replaced.

**Neglect pattern (2026-08-17):** promote recycled scheduler only; `mcp ensure` no-op’d on a live stale daemon; PrivilegedWriter + seat-workers kept ~2h-old inodes → tip/stable split-brain. Use `recycle-stable-daemons.sh`.

Protocol reminder: `docs/enforcement/AGENT_PROTOCOL_PROCESS.md` § **Post-promote / daemon cycle**; posture table in `.cursor/rules/agent-working-posture.mdc`.

## After machine reboot / login (not the same as promote)

**Incident (2026-08-19):** stable SHA was already correct, but after reboot MCP had no IDE subscribers, `jobs_paused` stayed true until a human flipped it, `SCH-cap-night-duty` resumed every 2 minutes, and COMMS-CHECK was skipped. Promote docs do not cover this path.

Do **not** rebuild or re-promote stable unless `git rev-parse HEAD` / `zqk-stable version` moved.

```bash
cd "$(git rev-parse --show-toplevel)"

# 1) Control plane (LaunchAgents that should be on)
launchctl print "gui/$(id -u)" | rg 'com\.zqk\.(scheduler|privileged-writer|mesh.seat-worker)' 
# Want: scheduler + privileged-writer + seat-worker.antigravity-{1,2} enabled
# Want: com.zqk.mesh.night-duty DISABLED (leave disabled — CAP orch is the loop)

# 2) Recycle long-lived consumers onto the existing stable inode
./scripts/recycle-stable-daemons.sh

# 3) Scheduler run-state
./bin/zqk-stable scheduler config --format json
# If jobs_paused=true and the human wants timers: --no-jobs-paused (then restart scheduler)
./bin/zqk object get SCH-cap-night-duty --format json --fields id,enabled,status
./bin/zqk object get SCH-cap-orchestrator --format json --fields id,enabled,status,last_run_at,next_run_at
# Night-duty must be enabled=false. CAP orch must stay enabled.
# Disable night-duty (do not snap status): zqk object update SCH-cap-night-duty --field enabled=false

# 4) MCP + IDE subscriber
./bin/zqk-stable mcp ensure --tcp 127.0.0.1:8443
python3 scripts/cursor/ide-bridge-request.py --command zqk.mcp.reloadClient || true
./bin/zqk feed doctor --format json
# Success: mcp_subscribers >= 1, feed_health=healthy
# Do NOT run feed doctor --refresh-seats unless you will restore antigravity-* ids
# (refresh can collapse seats to peer-agent-01). TRACK: TDE-MESH-DEFAULT-SEAT-PEER-AGENT-01-001

# 5) COMMS-CHECK (life ∧ work) — transport/toast is not a pass
# Canonical: docs/onboarding/MESH_COMMS_E2E_SETUP.md §5
NONCE="CC-$(date -u +%Y%m%dT%H%M%SZ)-$RANDOM"
./bin/zqk feed steer --agent-id cursor-composer --to-agent-id antigravity-1 \
  --await-peer-ack --message "COMMS-CHECK $NONCE: ack as antigravity-1; echo nonce; whats-next probe"
# Repeat for antigravity-2. PASS = seat-authored peer_ack + nonce + WORK steer.
```

Night-duty LaunchAgent (`com.zqk.mesh.night-duty`) staying disabled is **not** enough — the **scheduler job** `SCH-cap-night-duty` can still fire every 2 minutes after reboot if `enabled` is true. CAP loop is `SCH-cap-orchestrator`.

## Decision Framework

See [MCP Binary Stability Decision Framework](../process/architecture/mcp-binary-stability-decision-framework.md) for detailed guidance.

## Version Management

Versions are stored in:
- `.zqk/mcp/config.yaml` - `binary_version` field
- Change history: see docs/process/architecture/mcp/ or git history

## Testing

After updating:
1. Run the **Mandatory post-promote daemon cycle** (scheduler + `mcp ensure` + IDE reload)
2. Confirm `.zqk/bin/zqk-stable version` works (not `zsh: killed`)
3. `mcp list-tools` and JSON-RPC via Cursor MCP
4. `feed doctor --format json` → `mcp_subscribers >= 1`, `peer_wake_live` as expected
5. **Verify Spec Provenance**: `zqk system status` to ensure `kernel_ambience` lists valid specs and the daemon isn't running blind.
6. Monitor for issues

## Rollback

If issues occur:
1. Restore backup binary (created by update script)
2. Update config to previous version
3. Restart MCP server / scheduler
4. Investigate issue
