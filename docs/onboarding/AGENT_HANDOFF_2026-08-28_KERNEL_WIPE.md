# Agent handoff — kernel wipe + orchestration reboot (2026-08-28)

**Audience:** next Cursor/TPM (and AGY seats after COMMS) spawned after the human reboots  
**Repo:** `/Users/lanceettl/zqk-restore-clone`  
**Remote:** `git@github.com:lanceman/zqk.git` (personal — not public `zqk-os`)  
**Paste-short form:** `scripts/mesh/REBOOT_HANDOFF_20260828.md`

If this file disagrees with live `zqk object get` / `whats-next`, **trust the kernel**.

**Hard stop:** do not start swarm, `ORCHESTRATE_PLAN`, CAP dispatch, or `system check --auto-fix` until **Gate 0** (`sh ./scripts/kernel-health-snapshot-check.sh`) reports **blocking=0**. Ticking CAP on a hollow object store is how the last session spent hours in `cap_stage_grooming` with `planned=0`.

---

## Why this reboot exists

Overnight 2026-08-27→28: swarm used the **studio clone** as an ATK worktree (branch thrash). A human `git clean` plus a racing `zqk system check all --auto-fix` (PID 83932, killed overnight) **wiped instance CAS** and ephemeral `.zqk` state. TPM orchestration then kept “aligning” an empty runway instead of restoring the kernel or shipping the completed column.

The human’s critique is accepted as the operating diagnosis: grooming runway gone, kernel stewardship only when prompted, swarm branch hygiene absent, hourglass hibernation, no autonomous strategy sessions, CAP ticks without dispatch. This handoff is the spawn contract so the next seats do not repeat that pattern.

---

## Identity and hard rules

| Seat | Role |
|------|------|
| `cursor-composer` / `PER-DEFAULT-OPERATOR` | TPM — Gantt, CAS restore, hourglass, ship-exit. Do **not** claim orch-bound ATKs. |
| `antigravity-1`, `antigravity-2` | Workers — **re-seat after Gate 0**. Last `peer_seats.json` only had auto-seeded `peer-agent-01`. |

- Wake: **notify/MCP only**. Directed `feed steer` **requires** `--await-peer-ack`.
- Process YAML: **CLI only**. Never hand-edit `.zqk/process/` instance files.
- **Never** `git clean` / `git stash -u` / `rm -rf` under `.zqk/process/`.
- Never checkout `main` in a **linked** worktree. Primary clone may hold `main`.
- Never `heal-dangling` GhostRefs that are wiped files (restore blobs first).
- Never remint `ORCHESTRATE_PLAN PRI-CEF-R23-PKG-LAYOUT-001` without a live `object get` of planned fuel.
- Never overwrite live `zqk-stable` inode (`scripts/install-zqk-stable.sh`).
- Public push: human `public_push_ack.json` + payload check. Do not invent ACK.

---

## Git at pause (verify after reboot)

Last observed before this handoff:

- Branch: `integration/pri-cef-r24-local-models-001`
- Tip: `38c0c79fe1` (`docs: add telemetry for local models bakeoff`) — **1 ahead of** `origin/main` `3a2846634a`
- Dirty: tracked CAS deletes + large untracked `.zqk/process/**/*.yaml` (kernel state, not noise)

```bash
cd /Users/lanceettl/zqk-restore-clone
git log -1 --oneline
git rev-parse --abbrev-ref HEAD
# Avoid a full git status on a huge dirty tree if the agent-exec wrapper hangs (FD3).
```

**Rescue is already off `/tmp`:** `/Users/lanceettl/zqk-kernel-rescue-20260828/zqk-health-rescue-20260828T035652/` (13,831 files). Time Machine local snapshots: **none**. Live `.zqk/process` YAML **15,682** vs rescue **13,634** — overlay **missing ids only**. Two missing plans were copied back (hash-verified). `.zqk/object_drafts` was recreated empty. **Full** missing-id overlay + `kernel-health-snapshot-check.sh` is still Gate 0 after reboot.

---

## Gate 0 — restore kernel CAS (blocking)

Do **not** `system check --auto-fix`. Do **not** `heal-dangling`.

```bash
# 1) Tracked deletes from THIS branch HEAD (not origin/main if HEAD has more CAS)
git ls-files -d .zqk/process > /tmp/cas-deleted.txt
# Review, then restore:
# git restore --source=HEAD -- $(cat /tmp/cas-deleted.txt)

# 2) Overlay rescue if present (missing-id only; never blindly clobber a healthier tree)
ls -ld /Users/lanceettl/zqk-kernel-rescue-20260828 /tmp/zqk-health-rescue-*

# 3) Human Time Machine: restore .zqk/process from a snapshot taken BEFORE the git-clean.

# 4) Recreate empty draft plane (gitignored). Missing dir is expected after git clean -fdx.
mkdir -p .zqk/object_drafts

# 5) Health gate — FULL check, no --auto-fix, no --fast
sh ./scripts/kernel-health-snapshot-check.sh
```

When blocking=0, commit CAS as a restore snapshot (`scripts/README.md` — Kernel health snapshot).

### Live kernel truth at handoff write (2026-08-28 ~16:45Z)

| Object | Result |
|--------|--------|
| `PRI-CEF-R24-LOCAL-MODELS-001` | **exists** `active` `active_order=10`. `branch_ref` unset in get. |
| `PRI-CEF-R20-BRANCH-PROVENANCE-001` | **exists** `grooming` `active_order=1`. |
| `PRI-CEF-R23-PKG-LAYOUT-001` | **exists** `grooming`. BLIs SRC-TRASH + DIR-CENSUS **planned**. |
| `PRI-CEF-R24-KERNEL-INTEGRITY-001` | **restored from rescue** — `active` `active_order=1` (collides with R20 ao=1). Not a new lead until children are planned and local-models ship-exits. |
| `PRI-CEF-R25-ENVELOPE-REMEASURE-001` | **restored from rescue** — `grooming` `active_order=5` |
| `BLI-REDACTED` | MCP get missed; **live YAML exists** as `complete` (`4c089f…`). Rescue is older `validated` (`07a74e…`) — do not overlay. |
| `BLI-REDACTED` / `…6e1ae4f0` | **complete**, still `priority_plan_ref=PRI-CEF-R24-LOCAL-MODELS-001` |
| `BLI-KERNEL-UNPAIRED-DELETE-INBOUND-001` | **validated**, plan ref gone (parent plan missing) |
| `object count backlog_item` | **1017** |
| `object count priority_plan` | **81** (grooming count=2, active count=1) |
| `object list backlog_item --filter priority_plan_ref=PRI-CEF-R20-…` | **0 objects** (get of known IDs still works) |
| `object list priority_plan --filter status=grooming` | **0 objects** while count says 2 |

Treat **count vs list vs get** disagreement as a **membership/index defect**, not as “empty backlog.” Restore CAS + rebuild indexes before trusting list/CAP.

---

## Gantt weighting (the question that started this)

**`grooming` + `active_order=1` (R20) does not outrank `active` + `active_order=10` (R24).**

Whats-next (`pkg/workflow/whatsnext`, `pkg/objects/lifecycle_whatsnext_rank.go`):

1. **Execution-facing filter:** `active` / `in_progress` / `paused` are execution-facing. **`grooming` is not.** R20 is skipped when selecting the execution lead.
2. **Status bonus:** `in_progress` (execution_locked) ≫ `active` (shovel_ready, +400k) ≫ `grooming` (**+0**).
3. **Lower `active_order` wins only among shovel-ready/active plans.** Unset order on `active` is a huge penalty. `in_progress` is treated as order 0 even if the field is stale.
4. Therefore the **lead is R24 `active@10`**, not R20 `grooming@1`. R20’s leftover `active_order=1` is **Gantt dirt** (should have been cleared on demote to grooming). It does not mean “R20 is next.”

---

## Why a plan can be `active` when all children are `complete`

Your instinct (`in_progress` at minimum) is what the **documented ladder** says. The **persisted machine** does not keep that status.

1. **`active` is shovel-ready (ready to act), not parked.** Shockwave `active → in_progress` is a check valve: exits are pause / blocked / complete / cancelled. No demotion to `active` or `grooming`. Halt resume is `paused|blocked → in_progress`, not `→ active`. Complete children on `active` mean closeout failed (promote to `complete`). Rubric: `docs/architecture/LIFECYCLE_STATE_MACHINE_RUBRIC.md`.
2. **Auto `active → complete` exists** (last child terminal) but is gated on:
   - all linked BLIs terminal
   - **`branch_ref` is an ancestor of trunk**
   R24’s `object get` returned **no `branch_ref`**. That precondition fails → plan stays `active` forever even with a complete column.
3. **`grooming → complete` is manual only** (escape hatch when children are already terminal). R20 sits in grooming with complete children and leftover `active_order=1` until TPM promotes it.

**Correct closeout:** package/PR (`packaging_cue` / POL-WORKFLOW-002), then `promote` to `complete` (or set `branch_ref` so auto-complete can fire). **Do not restuff** a sealed complete column.

State-machine work after restore (do not start until Gate 0):

- Stop mapping `in_progress` onto `active`, **or** stop advertising `in_progress` as a real status.
- Auto-complete must not depend on missing `branch_ref` (fail visible, or complete when children terminal + packaging_cue).
- Clear `active_order` on demote to grooming.
- CAP: `cap_stage_grooming` + `planned=0` must **fail closed** after N attempts (surface ship-exit / next column), not tick 37 times.
- Membership index: `list --filter priority_plan_ref=` returning 0 while `get` works is a P0 after restore.

---

## `.zqk/object_drafts` missing

The path is **gitignored** (`.gitignore`: `.zqk/object_drafts`). It is an ephemeral draft plane, recreated on first draft-plane write. It is **not** kernel CAS.

It disappears after:

- `git clean -fdx` (ignored files)
- crash / studio checkout `-f`
- racing `system check --auto-fix`

Empty draft-plane totals (`total=0`) after a wipe are consistent with a missing directory. Recreate with `mkdir -p .zqk/object_drafts`. **Do not** treat this folder as the restore source for `.zqk/process` hash YAML.

---

## Gate 1 — daemons (only after Gate 0)

```bash
./scripts/mesh/ops-background-status.sh
./bin/zqk-stable mcp ensure --tcp 127.0.0.1:8443
./bin/zqk-stable feed doctor --format json   # mcp_subscribers >= 1
```

If LaunchAgents were disabled for a clean boot (see `docs/onboarding/AGENT_HANDOFF_2026-08-15_POST_COMMS_REBOOT.md`), re-enable **PrivilegedWriter first**, then scheduler, then MCP. CAS writes fail closed without the writer.

Rebind night-duty: last observed `SCH-cap-night-duty` used `NIGHT_DUTY_PLAN_ID=PRI-CEF-S1-SURFACE`, **not** the Gantt lead. Bind to **whats-next lead** or disable until Gate 0.

Do not `go run ./cmd/zqk` on this tree (`.zqk` write-guard panic). Use `./bin/zqk-stable` / `./bin/zqk`.

---

## Gate 2 — Gantt after a healthy kernel

| Plan | Last truth | Action |
|------|------------|--------|
| `PRI-CEF-R24-LOCAL-MODELS-001` | `active` ao=10, remaining visible BLIs complete; one BLI **missing** | **Ship-exit** (package/PR). No mint. No demote to planned. |
| `PRI-CEF-R20-BRANCH-PROVENANCE-001` | `grooming` ao=1, known BLIs complete | Clear leftover `active_order` or `grooming→complete`. Not the execution lead. |
| `PRI-CEF-R23-PKG-LAYOUT-001` | `grooming`, SRC-TRASH + DIR-CENSUS **planned** | Next unlocked intake **if** restore still shows planned fuel. Do not remint orch if already dispatched. |
| R24 kernel / R25 remesure | **missing plans** | Restore YAML or recreate via CLI after Gate 0. Kernel BLI `BLI-KERNEL-UNPAIRED-DELETE-INBOUND-001` still exists as **validated** unbound. |

POL-AGENT-TPM-GROOM-AHEAD-001: stay **3 full priority plans** ahead. Align-cache score is **not** a strategy session and **not** a third column.

---

## Gate 3 — mesh (after kernel healthy)

```bash
./bin/zqk feed doctor --refresh-seats --format json
# COMMS-CHECK with nonce; life ∧ work. POL-AGENT-COMMS-CHECK-001.
```

Workers: own ATK worktree under `$TMPDIR/zqk-worktrees/...` (or project worktree helper). **Never** checkout the plan branch in the studio clone.

Ack SLA: inbox unacked → `feed ack` then next `--await-peer-ack` steer **in the same turn**. Empty inbox + complete lead = ship-exit or unlock next column, not another align.

---

## Orchestration pivot (spawn contract)

The last TPM seat was a single-agent loop: empty inbox → idle; CAP grooming → persist align cache; swarm waiting for ack. That is hibernation. After reboot the TPM seat **is** the orchestration process, not a chat responder.

1. **Restore-before-advance.** No CAP dispatch / orch / scan-tests-as-progress until snapshot-check blocking=0.
2. **3-plan runway is TPM work.** If fewer than 3 execution-or-intake columns with real BLIs exist, the job is objectify from undelivered REQ/CRIT — not `align-latest.json`.
3. **CAP fail-closed.** `cap_stage_grooming` + `planned=0` after N attempts → packaging_cue / next grooming with `planned>0`. Do not leave silent ticks.
4. **Ack SLA.** Directed steer always `--await-peer-ack`. Peer ping that sits hours is a TPM defect.
5. **Stratplan is a scheduled session**, not a vibe. First healthy session after Gate 0: run the strategic-planning team configuration (kernel objects + hourglass to strategy personas). Record the session object. Align cache ≠ session.
6. **Branch hygiene is TPM-enforced.** Studio stays on the plan-integration or `main` as the **human** parked it. Swarm only in linked worktrees. Detach-from-trunk diffs are a stop-the-line event, not a PR.
7. **Ambient telemetry.** Scheduler `health.jsonl`, CAP issues, GhostRef counts, `feed doctor` — TPM reads these every turn that is not a restore/fix, without waiting for a human prompt.
8. **No `--auto-fix` during recovery.**

---

## Immediate “do not”

- `git clean` / `stash -u` on `.zqk/process`
- `heal-dangling` on post-wipe GhostRefs
- Remint R23 orch without `object get` proof of planned fuel
- Restuff R24 local-models
- `go run ./cmd/zqk` on this tree
- Overwrite `zqk-stable` while scheduler/MCP hold the inode
- Public push / `zqk-os` without ACK
- Treat MCP `object list` returning 0 as “no work” when `object get` of known IDs succeeds

---

## Bring-up order after human says cleanup done

1. Gate 0 restore + snapshot-check  
2. PrivilegedWriter → scheduler → MCP  
3. `whats-next --format json --persona-id PER-DEFAULT-OPERATOR --agent-id cursor-composer`  
4. Ship-exit R24 **or** unlock next column with planned fuel  
5. Re-seat AGY + COMMS-CHECK  
6. CAP only when the lead can dispatch (`planned>0`) or stage is allowed to leave grooming  
7. First autonomous stratplan session (hourglass, not “I’ll get to it”)

---

## Related

- `scripts/mesh/REBOOT_HANDOFF_20260828.md` — short paste
- `docs/onboarding/AGENT_HANDOFF_2026-08-15_POST_COMMS_REBOOT.md` — LaunchAgent table
- `scripts/README.md` — kernel health snapshot / incident recovery
- Lifecycle: `.zqk/specs/lifecycles/priority_plan_lifecycle.yaml`
- Rank: `pkg/objects/lifecycle_whatsnext_rank.go`
