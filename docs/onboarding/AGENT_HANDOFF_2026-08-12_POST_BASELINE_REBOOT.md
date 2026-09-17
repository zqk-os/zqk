# Agent handoff — post-merge baseline 2026-08-12 (reboot)

**Audience:** next TPM / Cursor seat (and AGY peers after TPM seats the board)  
**Human:** rebooting after syncing `main`, rebuild, and **v2.8.3** release of PR **#1520**  
**Repo:** `/Users/lanceettl/zqk-restore-clone`  
**Baseline:** `origin/main` @ `65dea310d7` (Merge #1520) · tag **`v2.8.3`** · system check **`total_issues=0`** (handoff snapshot)

This is a **stable baseline to build on**. Do not reopen the integrity firefight unless evidence regresses.

---

## TPM charter (non-negotiable priority)

You are the **ZQK kernel steward + process administrator**, not a random feature coder.

| Duty | Meaning |
|------|---------|
| **Kernel steward** | Keep CAS / draft / membrane / check planes honest; PrivilegedWriter up; no hand-CAS; no test pollution into `zqk:kernel`; pristine `system check` trends hold. |
| **Advocate** | Defend fail-closed membrane, VDS done-gates, hourglass mesh, CLI-only process data — push back on shortcuts that re-rot the kernel. |
| **Process admin** | Seat the board: `active_order`, shovel-ready BLIs, swarm ATKs aligned to **strategic plans / goals / workstreams**, not vibes. Peers get **meaningful prioritized work**. |
| **Altitude** | Stay above implementation theater: groom → claim → orchestrate → verify with artifacts. Use `zqk workflow whats-next`, `system align`, VDS — not chat memory. |

**Binding board intent (as of handoff):**

| Order | Plan | Status (handoff) | Role |
|------:|------|------------------|------|
| **0** | `PRI-CAS-MEMBRANE-ENFORCE-001` | **`grooming`** (P0) | Membrane program — must not stay buried behind idle plan in `whats-next` |
| **1** | `PRI-AGENT-IDLENESS-ACCUMULATOR-001` | **`active`** (still whats-next default) | Idle scoreboard / CVS — **after** membrane board is seated correctly |

**First TPM move after reboot:** refresh alignment / `active_order` so stewards and peers see **membrane first**. Do not let swarm grind idleness theater while membrane stays `grooming` without a clear next shovel-ready tranche.

```bash
cd /Users/lanceettl/zqk-restore-clone
git checkout main && git pull --ff-only
./bin/zqk-stable workflow whats-next --agent-id cursor-composer --format json
./bin/zqk-stable object get PRI-CAS-MEMBRANE-ENFORCE-001
./bin/zqk-stable object get PRI-AGENT-IDLENESS-ACCUMULATOR-001
./bin/zqk-stable system align --format json   # or project align command of record
./bin/zqk-stable system check --format json   # expect issues≈0
```

---

## First 60 seconds (ops)

```bash
# Stable consumers
./scripts/install-zqk-stable.sh --stop-scheduler --from bin/zqk   # if tip rebuilt
# or: make promote-stable   # now stops MCP holders + recycles scheduler/MCP
./bin/zqk-stable scheduler start
./bin/zqk-stable mcp ensure --tcp 127.0.0.1:8443
python3 scripts/cursor/ide-bridge-request.py --command zqk.mcp.reloadClient
./bin/zqk-stable feed doctor --format json   # mcp_subscribers >= 1

# PrivilegedWriter (CAS mutations fail-closed without it)
ls -la /tmp/zqk-privileged-writer.sock || nohup ./bin/zqk object daemon >> .zqk/logs/privileged-writer-daemon.log 2>&1 &
```

## MMORCH peer prep (before COMMS / plan handoff)

**Canonical:** [`MMORCH_PEER_SESSION_BOOTSTRAP.md`](./MMORCH_PEER_SESSION_BOOTSTRAP.md) — two-phase only (no partial adherence).

1. **Phase A:** human-opt-in `wake-agy --chat` with `scripts/mesh/bootstrap/antigravity-*-session-init.txt` (persona + seat id). Wait for `BOOTSTRAP-OK` (reject `agent_id=human`).
2. **Phase B:** COMMS-CHECK life∧work → `zqk agent orchestrate <PRI>` (whole plan). Steady wakes = notify-only.

Do **not** open cold peers with notify-strip nonce COMMS — that produces incoherent / `human`-stamped replies.

**Promote lesson:** MCP runs as `bin/zqk-mcp-daemon` (symlink → stable). `make promote-stable` / `install-zqk-stable.sh --stop-scheduler` now clears those holders. Never `cp` onto a live stable path.

---

## What just landed (do not re-derive)

- **PR #1520** merged → `main` / **`v2.8.3`**
- Kernel stewardship: fixture purge, CacheLag HV exclusion, CAS fallthrough gated on `TEST_ROOT`, SCH-evag multi-MB diagnostics resilience, PR review cleanup, promote-stable MCP holder fix
- Pristine check posture at handoff: **0 issues / 0 blocking** (~6670 objects)
- Objectify claims (re-verify if board looks hollow):
  - `./scripts/verify-objectify-claim.sh scripts/fixtures/objectify_claims/2026-08-11-cas-membrane-enforce-restore.json`
  - `./scripts/verify-objectify-claim.sh scripts/fixtures/objectify_claims/2026-08-11-agent-idleness-accumulator-restore.json`

---

## Hard process rules (still)

1. **Never** `git clean` / `git restore` under `.zqk/process/` (CAS kernel state).
2. **Never** hand-materialize CAS / edit instance YAML — `zqk object …` only (`DEC-NO-HAND-CAS-MATERIALIZE-001`).
3. Directed steers: `--await-peer-ack`; workers: `agent next --on-validation-failure wake`.
4. COMMS-CHECK = seat nonce echo + kernel probe (`POL-AGENT-COMMS-CHECK-001`).
5. No hollow `complete` / forged `commit_refs` — VDS before done claims.
6. Tests must not mint into live `zqk:kernel` — `TEST_ROOT` before fallthrough (`ApplyIsolatedStorageEnv`).

---

## Git hygiene (do this session — before more feature work)

### A. Orphan branch: `integration/pri-cas-membrane-enforce-001`

**Problem:** Pushed with **no merge-base vs `main`** (detached / orphan history). Tip `75f12dd50f`.

**Likely outcome:** most membrane intent already on `main` via #1520 / `6526fdeb0f` (“CAS membrane enforce + hand-CAS/dup-id checks on main base”). Key current files (`privileged_writer_membrane.go`, `prototype_rejection.go`) are **missing on that tip**; Makefile / install-stable on that tip are **behind** `main`.

**Required procedure:**

```bash
git fetch --all --prune
# Inventory unique commits (orphan — no merge-base; use log + file diffs, not merge)
git log --oneline origin/integration/pri-cas-membrane-enforce-001 | head -40
# For any commit that looks unique, try cherry-pick onto a throwaway branch from main:
git checkout -b tmp/membrane-orphan-audit origin/main
git cherry-pick -n <sha>   # or cherry-pick and resolve; abort if noise
# Prefer content review of unique paths over blind cherry-pick of whole tip.

# When satisfied nothing unique remains:
git push origin --delete integration/pri-cas-membrane-enforce-001
git branch -D integration/pri-cas-membrane-enforce-001
git branch -D tmp/membrane-orphan-audit 2>/dev/null || true
```

**Do not** merge that branch into `main` (no shared history).

### B. Local branches — lingering / prune

At handoff, **~41 local branches are already `--merged` into `origin/main`** (safe delete candidates), including the old feature branch for #1520 once you are on `main`.

**Not merged (local) — audit before delete (~7):**

| Branch | Note |
|--------|------|
| `integration/pri-cas-membrane-enforce-001` | Orphan — §A |
| `feature/cvs-fix-tests` | tip mentions hand-CAS blocking — compare to main before drop |
| `feature/PRI-ENV-SIGNED-LOGIN-001` | program claimed complete historically — confirm no unique commits |
| `feature/pri-1786084814786868000-df9be3b3` | audit |
| `feature/swarm-continuity-runway-mesh-cap` | audit |
| `feature/sweep-draft-plane-bench` | ahead of remote — check |
| `fix/ghost-cvs-tick-templates-disabled` | may overlap `feat/disk-usage-atk-merge-gate` |

**Suggested cleanup (after §A cherry audit):**

```bash
git checkout main
git pull --ff-only

# Merged locals (review list first!)
git branch --merged origin/main | grep -v '^\*' | grep -v '^  main$' | xargs -n1 git branch -d

# Stale remotes already deleted upstream
git remote prune origin

# Merged remotes (careful — only delete when PR merged / human OK)
# List: git branch -r --merged origin/main | rg -v 'origin/main$' | head
# Delete example: git push origin --delete <branch>
```

**Also:** leave `feature/BLI-CAS-HAND-DUP-CHECK-001` after switching to `main` if still checked out; delete local+remote once confirmed merged (`#1520`).

### C. Worktree noise

- `internal/bootstrap/archive/bootstrap.tar.gz` often dirties on rebuild — do **not** treat as process CAS; commit only if intentional package delta.
- Never tidy by deleting `.zqk/process/**` untracked YAML.

---

## Product / board follow-ups (after git clean)

1. **Seat membrane:** promote `PRI-CAS-MEMBRANE-ENFORCE-001` from `grooming` only when shovel-ready (REQ/CRIT/WS linkage + VDS); refresh `active_order` / align so swarm sees it first.
2. **Close hollow completes** if any membrane BLIs still claim `complete` without REQ/CRIT proof (prior adversarial finding).
3. **Idleness plan** stays second: real scoreboard → `CVS-AGENT-IDLENESS-REDUCTION-001`, not empty CAP pings.
4. Keep **PW daemon** + **mcp ensure** in the post-reboot ritual; SCH-evag should stay green (multi-MB diagnostics fixed).

---

## Success criteria for next session

1. On `main` @ `v2.8.3` (or newer ff); worktree clean of stale branches.
2. `integration/pri-cas-membrane-enforce-001` gone local+remote after cherry audit.
3. TPM board: membrane P0 visibly first in align/whats-next; swarm has claimed, shovel-ready work.
4. `system check` still ~0 issues; no new `Test User` / fixture CAS in kernel.
5. Mesh: `mcp_subscribers >= 1`, hourglass COMMS green before orchestration.

---

## Transcripts / prior context

- Stewardship + pristine + promote + SCH-evag + Makefile holders: [kernel steward baseline](0f4aaea4-0e37-4bbf-b3fd-e0624282b870)
- Older post-reboot membrane note (partially superseded by #1520): `docs/onboarding/AGENT_HANDOFF_2026-08-11_POST_REBOOT.md`

---

## Human reboot note

Safe to reboot. After boot: `git checkout main && git pull`, rebuild (`make build-all` or tip+`make promote-stable`), start PW + scheduler + MCP, then execute **Git hygiene §A–B** before seating swarm work.

## Overnight duty (2026-08-12 late)

Human sleeping; TPM mandate **no idle**.

- Lead: `PRI-AGENT-IDLENESS-ACCUMULATOR-001` (SCOREBOARD+STORE complete; BASELINE then TICK/CVS-WIRE)
- Next column: `PRI-COMMS-CURSOR-TPM-LIVE-WAKE-001` (prioritizing ao=1)
- Wake mode: `--chat` until notify deterministic; stopgap `ring-tpm-live` + inbox-ringer
- Controls: `com.zqk.mesh.night-duty`, `tpm-agy-watchdog`, PW sock, 12m `AGENT_LOOP_TICK_overnight_tpm`
- Claims: `2026-08-12-tpm-idleness-roadmap-groom.json`, `2026-08-12-overnight-tpm-arm.json`
