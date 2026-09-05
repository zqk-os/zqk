# Reboot handoff — 2026-09-03 (pre-reboot)

**Studio root:** `/Users/lanceettl/zqk-restore-clone`  
**Branch:** `integration/pri-cef-remediate-structure-001`  
**Git tip:** `3a4d2441ed` (dirty worktree — see `git status`)

## What was done (this session)

1. **All zqk daemons stopped** — launchd bootout + MCP/scheduler PIDs killed. **No recycle.**
2. **Full rebuild:** `make zqk zqk-admin zqk-community zqk-mcp zqk-mcp-fal generate-spec-builders zqk-neuron zqk-muscle zqk-heart zqk-lung zqk-shim`
3. **Stable promoted** (no daemon cycle): `./scripts/install-zqk-stable.sh --from bin/zqk`
   - **New stable SHA:** `b7a72d77fef9e0b9ca390073d726846ca71ab718de97b3d645b65212095ee70b`
   - **Old stable SHA:** `6711957cc8ea0f968aa324e0b24e3026f77ca1fc9664d88ee3638fc2d131103c`
   - Version: `v2.9.7-kernel-stability-202-g3a4d2441ed-dirty`

## After reboot — run in order

```bash
cd /Users/lanceettl/zqk-restore-clone

# 1) Recycle ALL stable consumers onto the new inode (required after promote)
./scripts/recycle-stable-daemons.sh

# 2) Scheduler run-state
./bin/zqk-stable scheduler config --format json
# If jobs_paused=true and you want timers: fix config, then restart scheduler

# 3) CAP jobs — night-duty must stay disabled; CAP orch enabled
./bin/zqk-stable object get SCH-cap-night-duty --format json --fields id,enabled,status
./bin/zqk-stable object get SCH-cap-orchestrator --format json --fields id,enabled,status,last_run_at,next_run_at

# 4) MCP + IDE
./bin/zqk-stable mcp ensure --tcp 127.0.0.1:8443
./bin/zqk-stable mcp list-tools
python3 scripts/cursor/ide-bridge-request.py --command zqk.mcp.reloadClient || true
./bin/zqk-stable feed doctor --format json
# Expect: mcp_subscribers >= 1, feed_health=healthy
# Do NOT feed doctor --refresh-seats unless restoring antigravity-* ids intentionally

# 5) COMMS-CHECK (optional but recommended before orch)
NONCE="CC-$(date -u +%Y%m%dT%H%M%SZ)-$RANDOM"
./bin/zqk-stable feed steer --agent-id cursor-composer --to-agent-id antigravity-1 \
  --await-peer-ack --message "COMMS-CHECK $NONCE: ack as antigravity-1; echo nonce; whats-next probe"
# Repeat for antigravity-2

# 6) Resume orchestration
./bin/zqk-stable workflow whats-next --format json --skip-measure --agent-id cursor-composer
```

Ref: `docs/onboarding/STABLE_BINARY_MANAGEMENT.md` § *After machine reboot / login*

## Kernel context (orchestration)

| Item | State |
|------|--------|
| **Gantt lead** | `PRI-CEF-R27-ENVELOPE-001` (`in_progress`, `planned=2`) |
| **R27 BLIs** | `BLI-CEF-R27-DUAL-SEAT-REMEASURE-001`, `BLI-CEF-R27-TST-RCV-SEC-001` (planned; archived ATKs are inventory) |
| **STRUCTURE** | Largely complete; do not remint `ORCHESTRATE_PLAN` for STRUCTURE |
| **Red verify** | `pkg/storage` failures in `bundle-make-verify-339.log` — use `scan-tests --package ./pkg/storage`, not foreground `make verify` in AgentX |
| **Open hourglass** (pre-reboot) | ALPHA `AFE-1788429814946497000-11fa97a4`, BETA `AFE-1788429817986696000-cc469fb2` (R27 ORCHESTRATE) |
| **Active CVS** | `CVS-CEF-UNTIL-45-001` (c4_act), `CVS-1787015873043417000-b71a697e` (c1_scope) |
| **Orch throughput CVS** | **Not objectified** — drafts only in `/tmp/zqk-orch-throughput/` |

## LaunchAgents expected after recycle

- `com.zqk.scheduler.*` — scheduler
- `com.zqk.privileged-writer` — object daemon
- `com.zqk.mesh.seat-worker.antigravity-{1,2}` — AGY workers
- `com.zqk.mesh.seat-worker.cursor-composer` — TPM seat-worker
- **`com.zqk.mesh.night-duty`** — leave **disabled**
- Mesh watchdog (`com.zqk.mesh.tpm-agy-watchdog`) — reinstall if needed: `./scripts/mesh/install-mesh-watchdog.sh`

## Pending local work (not committed)

- `scripts/mesh/latest_unacked_agy_steer.py` + test — breaks AFE rebroadcast loop (watchdog still needs wiring in `tpm-agy-watchdog.sh`)
- `scripts/fixtures/change_intents/2026-09-03-break-acked-orch-rebroadcast.json`
- Various unstaged CAS / test artifacts — see `git status`

## Do not

- Re-promote stable unless git tip moved again
- Run `feed doctor --refresh-seats` casually
- Remint `ORCHESTRATE_PLAN` on sealed STRUCTURE hourglass
- Treat "whats-next idle" as permission to ignore red `scan-tests` / bundle failures
