# Mesh COMMS E2E setup & troubleshooting (TPM↔AGY)

Living runbook. **Verified PASS** `2026-08-11T08:46Z` — nonce `CC-20260811T084333Z-5900`  
(`antigravity-1`/`PER-ORCH-ALPHA` ∧ `antigravity-2`/`PER-ORCH-BETA` life+work on the **same** nonce).  
Policies: `POL-AGENT-COMMS-CHECK-001`, `POL-AGENT-MESH-WAKE-001`, `POL-AGENT-ORCH-HOURGLASS-001`, `WFL-TPM-AGY-MESH-001`.

## Pass criteria (objective)

| Layer | Pass signal |
|-------|-------------|
| Transport | `delivery_receipt=true`, `peer_wake_live=true`, `feed doctor` healthy |
| Life | Seat-authored `peer_ack` with **exact challenge nonce** in `--summary`, under that seat’s `agent_id` + intended `persona_ref` (`PER-ORCH-*`, not `PER-DEFAULT-AGENT` alone) |
| Work | Seat-authored steer/status with nonce + `whats-next` evidence bound to same nonce |

Transport alone is **FAIL**. Shared MCP config ≠ dual-seat identity.

## Proper end-to-end setup

### 1. Kernel ↔ IDE skills (any common host)

```bash
bash ./scripts/sync-ide-skills-from-kernel.sh --all --force
bash ./scripts/sync-ide-skills-from-kernel.sh --all --check   # exit 0
```

Symlinks kernel packs into (when present / `--all`):

| Dest | Host |
|------|------|
| `.cursor/skills` | Cursor |
| `.ide/skills` | vendor-neutral / Antigravity project IDE |
| `skills/` | repo twin of `.zqk/skills` |
| `.agent/skills` | Antigravity / Agent marker |
| `.claude/skills` | Claude Code |

Also keep `.ide/mcp.json` aligned with `.cursor/mcp.json` (same ide-adapter → `:8443`).  
Compat wrapper: `scripts/sync-cursor-skills-from-kernel.sh` → same multi-IDE script.  
Reload IDE windows after sync so hosts re-discover packs.

### 2. MCP (tools bus — shared OK)

Same `mcp.json` ide-adapter → `127.0.0.1:8443` for Cursor **and** both AGY instances is fine for tools.  
`mcp_subscribers >= 1` after reconnect proves at least one live IDE/adapter subscription.
A climbing count after reconnect thrash was a **leak** (subscribe without unsubscribe on drop) — fixed by connection-scoped Unsubscribe on disconnect. Prefer a small stable count over growth.

```bash
./bin/zqk mcp ensure --tcp 127.0.0.1:8443
./bin/zqk feed doctor --format json   # want mcp_subscribers >= 1
```

### 3. Peer seats (identity plane)

File: `.zqk/state/mesh/peer_seats.json`

| Seat | `wake` | Required |
|------|--------|----------|
| `antigravity-1` / `antigravity-2` | `agentapi` | **Distinct** `pid` **and** `conversation` (brain GUID) |
| `cursor-composer` | `mcp` | MCP ActionRequired |

Discover live brain GUID:

```bash
lsof -p <agy_pid> | rg 'antigravity-cli/(conversations|brain)/[0-9a-f-]{36}'
```

**Never** assign the same `conversation` to two seats. Shared brain → only one seat answers COMMS.

Refresh PIDs carefully: `feed doctor --refresh-seats` may collapse IDs to `peer-agent-01` (`DefaultPeerSeatIDs`) — prefer manual seat map after discover. TRACK: `TDE-MESH-DEFAULT-SEAT-PEER-AGENT-01-001`.

### 4. Personas + init (Phase A — **before** COMMS)

**Do not skip.** Notify-strip COMMS on a cold seat produces incoherent / `agent_id=human` replies.  
Canonical: [`MMORCH_PEER_SESSION_BOOTSTRAP.md`](./MMORCH_PEER_SESSION_BOOTSTRAP.md).

| Phase | Channel | Rule |
|-------|---------|------|
| **A** | Full chat (`wake-agy --chat`, human opt-in) | Once per seat/session: `scripts/mesh/bootstrap/antigravity-*-session-init.txt` |
| **B** | Notify-only | After `BOOTSTRAP-OK`; short ATTN + AFE; substance on feed |

- Bind ORCH personas (`PER-ORCH-ALPHA` / `PER-ORCH-BETA`) with skills.
- Warm: `PROMPT-PEER-ORCH-INIT-001` via Phase A chat + peer-local `zqk agent prepare-context`.
- Peers must ack/steer with `--agent-id antigravity-*` **and** `--persona-ref PER-ORCH-*` (never `human` / bare `PER-DEFAULT-AGENT` alone).
- Wait for seat-authored `BOOTSTRAP-OK` before step 5.

### 5. COMMS-CHECK challenge (TPM)

**Preferred executor:** `zqk agent seat-worker` (REQ-COMMS-SEAT-WORKER-001). agentapi notify is doorbell-only; do not depend on AGY interpreting the ATTN stub for PASS.

```bash
# Bind workers (one terminal per seat, or --once for one-shot)
./bin/zqk agent seat-worker --agent-id antigravity-1 --persona-ref PER-ORCH-ALPHA &
./bin/zqk agent seat-worker --agent-id antigravity-2 --persona-ref PER-ORCH-BETA &

NONCE="CC-$(date -u +%Y%m%dT%H%M%SZ)-$RANDOM"
./bin/zqk feed steer --agent-id cursor-composer --to-agent-id antigravity-1 \
  --await-peer-ack --message "COMMS-CHECK $NONCE: life∧work via seat-worker"
# same for antigravity-2
```

Manual peer shape (still valid if seat-worker is down — COMMS must FAIL closed until worker returns):

```bash
./bin/zqk feed ack --in-reply-to AFE-… --agent-id antigravity-N \
  --persona-ref PER-ORCH-… --summary "COMMS-CHECK $NONCE LIFE"
./bin/zqk workflow whats-next --format json --skip-measure --agent-id antigravity-N
./bin/zqk feed steer --agent-id antigravity-N --to-agent-id cursor-composer \
  --await-peer-ack --persona-ref PER-ORCH-… \
  --message "COMMS-CHECK-REPLY $NONCE WORK priority=<id> inbox=<n>"
```

Score from `.zqk/logs/ide-hooks/agent_chat_channel.jsonl` (not narrative). Heartbeat: `.zqk/state/mesh/seat_workers/<seat>.alive.json`. Persist `.zqk/state/ambient/comms-check-latest.json`.

### 6. Tip binary for wake stubs

Wake notify text includes `--agent-id <destination seat>` (fixed in `pkg/agentfeed/wake_paste.go`). Rebuild tip:

```bash
go build -o bin/zqk ./cmd/zqk
```

Do not rely on stubs that still say `peer-agent-01`.

## Troubleshooting matrix

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `mcp_subscribers=0` | IDE/adapter not connected | Reload Window; `mcp ensure`; check `.cursor/mcp.json` |
| `peer_seat_stale_pid` | agy restarted | Update `pid` (+ confirm `conversation`) in `peer_seats.json` |
| agy2 silent / 0 events | Shared conversation GUID | New `agy` brain; set distinct `conversation` |
| Auto `peer_ack` + `PER-DEFAULT-AGENT`, no nonce | Session not running ORCH init / COMMS script | Re-steer bootstrap + exact ack `--summary` with nonce |
| Wake log shows `peer-agent-01` | Stale tip / old stub | Rebuild `bin/zqk`; verify wake line uses seat id |
| Life PASS, work FAIL | Ack only, no whats-next + WORK steer | Peer must run steps 2–3 |
| IDE lacks zqk skills | Host skills dir not synced from kernel | `sync-ide-skills-from-kernel.sh --all --force` + reload |
| `feed doctor --refresh-seats` wipes seats | Default seat id `peer-agent-01` | Restore canonical `antigravity-*` map; TRACK TDE |
| Night-duty ticking after reboot | `SCH-cap-night-duty.enabled=true` even if LaunchAgent disabled | `zqk object update SCH-cap-night-duty --field enabled=false`; CAP loop is `SCH-cap-orchestrator` |
| `:8443` open but mesh dead | `mcp_subscribers=0` (IDE not subscribed) | `mcp ensure` + IDE MCP reload; reboot checklist in `STABLE_BINARY_MANAGEMENT.md` |

## Evidence log (this incident)

- Shared MCP OK; AGY wake = `agentapi` not MCP seat routing.
- Dual silent until distinct GUID `0af091f2-8be4-4e0e-add0-e127a1fa7e0b` for `antigravity-2` (agy1 kept `a3ac21b7-…`).
- Multi-IDE skills synced via `scripts/sync-ide-skills-from-kernel.sh --all` (20 packs → `.cursor` / `.ide` / `skills/` / `.agent` / `.claude`).
- Wake stub previously hard-coded `peer-agent-01` → tip `pkg/agentfeed/wake_paste.go` uses `ToAgentID`.
- Staggered proofs first; same-nonce dual PASS after agy1 WORK nudge on `CC-20260811T084333Z-5900`.
- Verdict artifact: `.zqk/state/ambient/comms-check-latest.json` (`overall=PASS`).

## Related objects / scripts

- `TDE-IDE-KERNEL-SKILL-SYNC-001`, `TDE-MESH-DEFAULT-SEAT-PEER-AGENT-01-001`
- `scripts/sync-ide-skills-from-kernel.sh` (compat: `sync-cursor-skills-from-kernel.sh`)
- `scripts/mesh/README.md` (mesh overview)
- Verdict: `.zqk/state/ambient/comms-check-latest.json`
