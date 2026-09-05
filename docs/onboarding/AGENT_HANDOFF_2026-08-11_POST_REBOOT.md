# Agent handoff — post-reboot 2026-08-10/11

**Audience:** every subsequent agent (TPM, AGY peers, Cursor seats)  
**Human action after this file:** reboot; may delete `~/10-pm-recovery` (safe — delta analyzed).  
**Repo:** `/Users/lanceettl/zqk-restore-clone` · branch `feature/PRI-ENV-SIGNED-LOGIN-001`

## First 60 seconds

```bash
cd /Users/lanceettl/zqk-restore-clone
./bin/zqk workflow whats-next --agent-id <your-seat> --format json
./bin/zqk object get PRI-CAS-MEMBRANE-ENFORCE-001 --fields id,status,active_order,title
./bin/zqk object get PRI-AGENT-IDLENESS-ACCUMULATOR-001 --fields id,status,active_order,title
# Prove objectify restore (must exit 0):
./scripts/verify-objectify-claim.sh scripts/fixtures/objectify_claims/2026-08-11-cas-membrane-enforce-restore.json
./scripts/verify-objectify-claim.sh scripts/fixtures/objectify_claims/2026-08-11-agent-idleness-accumulator-restore.json
```

Seat ids must match `peer_seats.json` (e.g. `antigravity-1`, not `AGY-1`). TPM seat: `cursor-composer`.

## Board order (binding)

| Order | Plan | Role |
|------:|------|------|
| **0** | `PRI-CAS-MEMBRANE-ENFORCE-001` | **P0** — fail-closed CAS membrane (PrivilegedWriter + MCP/path deny + hand-CAS checks) |
| **1** | `PRI-AGENT-IDLENESS-ACCUMULATOR-001` | Behind membrane — idle accumulator + scoreboard + `CVS-AGENT-IDLENESS-REDUCTION-001` |
| — | `PRI-ENV-SIGNED-LOGIN-001` | Already **complete** (env privilege bleeding program) |

Do **not** invent a hollow `WS-AGENT-IDLENESS-ACCUMULATOR-001` — human rejected PRI-twin WS without a real dedicated purpose. Idleness lives on **PRI + CVS + BLIs** only.

## Hard process rules (recent incidents)

1. **Never** `git clean` / `git restore` under `docs/process/` — untracked CAS YAML is kernel state (`.cursor/rules/never-git-clean-process.mdc`).
2. **Never** hand-copy `.zqk/object_drafts/` → `docs/process/` or invent CAS hash filenames — `DEC-NO-HAND-CAS-MATERIALIZE-001` (**active**).
3. Process mutations **only** via `zqk object …` / promote / demote (not editor/MCP `write_file` on CAS).
4. Directed feed steers **require** `--await-peer-ack`; workers use `agent next --on-validation-failure wake` (`POL-AGENT-ORCH-HOURGLASS-001`).
5. COMMS-CHECK = seat-authored **nonce echo + kernel probe** — toast/`delivery_receipt` alone is fail (`POL-AGENT-COMMS-CHECK-001`).
6. **Never** mark BLI `complete` with forged `commit_refs` (e.g. `0000000`) or fake `actual_effort`. Cleared forge residue on archived `REDACTED` (`commit_refs` unset).
7. Draft-sweep apply needs RBAC `delete:object_draft_plane` — not seat-string / env break-glass.

## Root cause still open (membrane)

Same-UID “secure membrane” was a story: `PrivilegedWriter` is **fail-open** (socket miss → direct FS write); MCP `write_file` unrestricted. Implement planned BLIs under `PRI-CAS-MEMBRANE-ENFORCE-001` (start with `BLI-CAS-PW-FAIL-CLOSED-001`, `BLI-CAS-MCP-DENY-PROCESS-001`, `BLI-CAS-HAND-DUP-CHECK-001`, `BLI-CAS-DRAFT-SWEEP-RBAC-AUDIT-001`).

## Scheduler / LaunchAgents after reboot

- Daemon was **intentionally stopped**; plists renamed to `*.disabled-by-user-*` under `~/Library/LaunchAgents/`.
- **Product fix in working tree (uncommitted):** intentional `scheduler stop` disarms host unit; KeepAlive is crash-only (`SuccessfulExit=false`) — see `pkg/scheduler/hostservice/{darwin,linux,registry}.go`, `cmd/zqk/scheduler/{scheduler_core,service_cmd}.go`, `darwin_keepalive_test.go`.
- After reboot, scheduler should **stay down** until human/agent runs install+start intentionally. Do **not** re-enable disabled plists by hand unless that is the goal.
- Prefer `./scripts/install-zqk-stable.sh` for stable binary; never `cp` over a live stable path.

## Backup / recovery note

- Full Acronis backup at **2026-08-10 22:00 PDT**: `/Volumes/My Passport SSD/zqk-restore-clone.tib`.
- Sibling restore was at `~/10-pm-recovery/Users/lanceettl/zqk-restore-clone` — human may delete it.
- **10pm tree did not contain** membrane/idleness objectify (created after backup). Live tree was already ahead on research BLI CAS renames; **do not** overwrite live `docs/process` from that recovery.

## Uncommitted code to preserve

```
M  cmd/zqk/scheduler/scheduler_core.go service_cmd.go
M  pkg/scheduler/hostservice/darwin.go linux.go registry.go
?? pkg/scheduler/hostservice/darwin_keepalive_test.go
?? cmd/zqk/object/draft_sweep_auth.go (+ test)
M  pkg/agentprompt/skills.go
(+ CAS renames under docs/process/backlog; bootstrap archive churn)
```

Commit only when human asks (agent git identity via `scripts/git-commit-as-agent.sh`).

## Active research CVS (still live)

`REDACTED` — multi-plane CAS/ghost/draft RCA. Align membrane work with this session; do not orphan it again by sweeping PRIs without `priority_plan_ref`.

## Transcript

Prior TPM session: [post-clean recover & membrane](27dc87a3-00fd-4061-b097-1626bf53e5a4)

## Draft-plane leftovers (honest)

REQs + `WS-CAS-MEMBRANE-ENFORCE-001` are still on **`.zqk/object_drafts/`** at `planned` — requirement→`active` needs milestone + test_case refs; workstream has no forward hop from `planned` without lifecycle fill. Claims document `expect_plane: draft` for those. **Do not** hand-copy them into `docs/process/`. Materialize only via `zqk object draft promote` once preconditions are met (or recreate with shovel-ready fields).

Board-critical objects already on **CAS**: both PRIs, membrane/idle BLIs, DECs, GLS, CVS, TDE.

## What success looks like next

1. Membrane BLIs implemented with tests; PrivilegedWriter fail-closed proven; draft REQs/WS promoted to CAS properly.
2. Idle accumulator real Go + scoreboard feeding `CVS-AGENT-IDLENESS-REDUCTION-001` (no empty CAP ping theater).
3. Peers stay hourglass-correct; no hand-CAS; no forge completes.
4. Scheduler only runs when intentionally started after disarm fix is committed/promoted.
