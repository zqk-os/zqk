# Agent handoff — post-COMMS merge reboot (2026-08-15)

**Audience:** next Cursor/TPM agent after the human finishes reboot and local cleanup
**Repo:** `/Users/lanceettl/zqk-restore-clone`
**Git baseline:** `main` @ `0ae64f559ef5` (`Merge pull request #1570 from lanceman/integration/pri-mcp-comms-reliability-001`)
**Stable SHA-256:** `a8f4f914c33cffc17bd86d9cf933e7cb7e619c0193f4ea64119dd16cd039b4fe`
**Shutdown snapshot:** `2026-08-15 10:14 PDT`

The human is rebooting to clean logs and ephemeral buildup. **Do not start the
scheduler, MCP daemon, IDE adapter, seat workers, night duty, or orchestration
until the human says cleanup is complete.**

## Intentional shutdown state

- Scheduler stopped gracefully, then its LaunchAgent was booted out.
- MCP TCP daemon stopped; `127.0.0.1:8443` is closed.
- Cursor IDE MCP adapter/proxy stopped. **Cursor respawns it automatically**
  while the IDE is open. That is expected and harmless with 8443 closed; only
  quitting Cursor or disabling the zqk MCP server in Cursor settings keeps it
  down.
- `antigravity-1` seat worker stopped because it held the stable binary.
- PrivilegedWriter (`./bin/zqk object daemon`) stopped;
  `/tmp/zqk-privileged-writer.sock` removed. It had been started from a Cursor
  shell wrapper on 2026-08-14, not from launchd.
- `SCH-cap-night-duty.enabled=false`.
- Stable was rebuilt from synced `main` and promoted atomically to all three
  paths. Hashes matched for `bin/zqk`, `bin/zqk-stable`, and
  `.zqk/bin/zqk-stable`. No long-lived consumer was restarted afterward.

### LaunchAgents are disabled on purpose

All zqk LaunchAgents were disabled at the human's direction so the machine
boots clean. **Nothing zqk-related will start at login.** An empty
`launchctl list | rg zqk` after reboot is the intended state, not a fault.

| Label | State |
|-------|-------|
| `com.zqk.scheduler.6fccea8f521def1d` | booted out + disabled |
| `com.zqk.scheduler` | disabled |
| `com.zqk.scheduler.9eb20fd56eb50b03` | disabled (pre-existing) |
| `com.zqk.privileged-writer` | disabled |
| `com.zqk.mesh.tpm-agy-watchdog` | booted out + disabled |
| `com.zqk.mesh.night-duty` | disabled (pre-existing) |

Re-enable only what the human approves, one at a time:

```bash
launchctl enable gui/$(id -u)/com.zqk.privileged-writer
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.zqk.privileged-writer.plist
# repeat per label; legacy fallback if bootstrap errors with I/O error:
#   launchctl load -w ~/Library/LaunchAgents/<label>.plist
```

Leave `com.zqk.mesh.night-duty` disabled. Do not re-enable the mesh watchdog
without deciding the wake policy first.

## First 60 seconds after cleanup

```bash
cd /Users/lanceettl/zqk-restore-clone

# Confirm the preserved baseline. Do not clean .zqk/process.
git status -sb
git log -1 --oneline
shasum -a 256 bin/zqk-stable .zqk/bin/zqk-stable

# Inspect stopped services before starting anything.
./scripts/mesh/ops-background-status.sh
ps -axo pid,ppid,command |
  rg -i 'zqk-stable.*scheduler|zqk-mcp-daemon|zqk-mcp-ide-adapter|agent seat-worker'
```

Only after the human clears bring-up. CAS mutations fail closed without
PrivilegedWriter, so start it first:

```bash
# PrivilegedWriter: prefer the LaunchAgent over an ad-hoc shell child, which is
# how the previous instance ended up owned by a Cursor terminal.
launchctl enable gui/$(id -u)/com.zqk.privileged-writer
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.zqk.privileged-writer.plist
ls -la /tmp/zqk-privileged-writer.sock

./bin/zqk-stable scheduler start
./bin/zqk-stable mcp ensure --tcp 127.0.0.1:8443
./bin/zqk-stable mcp list-tools
python3 scripts/cursor/ide-bridge-request.py --command zqk.mcp.reloadClient
./bin/zqk-stable feed doctor --format json
```

Do not re-enable night duty as part of ordinary bring-up. It was a stopgap, not
the production CAP mechanism.

## Git and filesystem state to preserve

`main` is synchronized with `origin/main`. The following were intentionally not
discarded:

- `stash@{0}` named
  `reboot-preserve-testscan-cache-2026-08-15` contains only the tracked
  `.zqk/testscan_cache.json` delta from the integration branch. It is optional
  ephemeral state; inspect before applying or dropping.
- `.zqk/process/personas/91e91cb9973299bac52514abbbf168b48c3db49a3a6f42cdf2c36b6476f55a6f.yaml`
  is untracked CAS/process state. **Never delete it with cleanup commands.**
  Resolve through `zqk object ...` / system checks, not `rm`, `git clean`, or
  `git restore`.
- Root artifacts `activity.yaml`, `context.yaml`, `patch.json`,
  `patch_verify.json`, `temp.json`, and `verify_atk.yaml` remain untracked.
  They may be cleanup candidates, but inspect ownership/content first.

## What landed

PR #1570 contains the MCP/COMMS tranche and the policy-gate repairs, including:

- authoritative MCP spec/kernelization work and COMMS transport/seat-worker
  changes;
- MCP resource-registration race fix and regression coverage;
- pre-commit timer scope repair: empty staging can no longer self-certify a
  blocking pass;
- stale background verdicts now expire rather than latching green forever;
- git-hook synchronization repairs missing executable bits and provides
  `--list`, `--disable`, and `--enable`;
- `paths.FilePerm644` restored to `0o644`, with file I/O routed through
  `pkg/utils/fileutil` on the touched hook/bootstrap paths.

Targeted tests and the policy/lint gates passed before merge.

## Kernel state and honest next work

### MCP/COMMS plan

`PRI-MCP-COMMS-RELIABILITY-001` is still `in_progress`. It has 11 linked BLIs:
9 `complete`, 2 still `in_progress`:

1. `BLI-COMMS-SEAT-WORKER-001` — P0 deterministic COMMS + AgentX seat worker.
2. `BLI-MCP-COMMS-E2E-CLOSURE-001` — P0 restart-safe verification matrix.

`CVS-COMMS-SWARM-OVERNIGHT-001` is `escalated`, phase `c1_scope`. Its recorded
next action is to finish seat-worker/fail-closed verification, then same-nonce
dual-seat and E2E closure. Because #1570 merged, the next agent must first
reconcile existing evidence against those two remaining BLIs and the CVS
desired end state; do not blindly redo implementation or mark them complete
from chat history.

### `whats-next` anomaly / stopgap

The shutdown-time `workflow whats-next --skip-measure` selected
`PRI-OVERNIGHT-PERPETUAL-ITERATION-001` (`in_progress`, `active_order=10`).
That plan represents the night-duty/CAP keep-alive stopgap. The human has
explicitly said night duty is not production quality and should remain off.
Do not resume that plan merely because `whats-next` selected it; reassess board
ordering and the real CAP product path first.

### Policy-gate productization (formalized, not current priority)

The following are `validated` and intentionally have no priority-plan
assignment yet:

- `BLI-NATIVE-POLICY-GATES-001`
- `BLI-NATIVE-FILESCOPE-001`
- `BLI-NATIVE-CHECKER-REGISTRY-001`
- `BLI-NATIVE-DEBT-RECONCILE-001`

They capture the native-Go/uniform-control-surface plan. Do not turn this into
an ad-hoc linting initiative. It must be prioritized and scheduled formally.
Claim proof:

```bash
./scripts/verify-objectify-claim.sh \
  scripts/fixtures/objectify_claims/2026-08-15-native-policy-gates-plan.json
```

## Next-agent sequence

1. Wait for human cleanup clearance.
2. Verify git/CAS state without deleting untracked process YAML.
3. Re-enable and bring up PrivilegedWriter, scheduler, MCP, IDE reload, and
   feed health in that order. Remember every LaunchAgent is disabled by
   design; nothing will be running when you arrive.
4. Run `workflow whats-next --format json` with the actual seat ID, but reject
   the overnight stopgap as automatic priority.
5. Reconcile #1570 evidence against the two remaining MCP/COMMS P0 BLIs and the
   escalated CVS contract.
6. Decide with the human whether the MCP/COMMS plan can close or needs one
   bounded restart-safe verification pass.
7. Leave native policy-gate productization parked until it receives a real
   priority plan.

## Prior conversation

Full context: [MCP/COMMS reliability and policy-gate repair](33af18a6-eb96-48ed-9b79-79dd23812dd8)
