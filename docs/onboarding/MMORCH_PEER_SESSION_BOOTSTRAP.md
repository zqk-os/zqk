# MMORCH peer session bootstrap (two-phase — mandatory)

**Purpose:** Stop “notify strip = full prompt” failure. Peers look incompetent when TPM only rings the doorbell; they are coherent when given a real chat-turn onboard.

**Identity layers (do not conflate):**

| Layer | What it is | Example |
|-------|------------|---------|
| Logical seat | Stable mailbox / routing address in `peer_seats.json` | `peer-agent-1` |
| Runtime session | One `zqk_session` per live seat-worker process (`session_type=agent_worker`) | `ZQK-…` stamped as `session_id` on feed/heartbeat |
| Persona | Cognition / role binding (`--persona-ref`) | `PER-ORCH-ALPHA` |
| Model / provider | Serving LLM recorded on the worker session | `provider` + `model_id` on session object |
| Wake transport | Membrane only (`mcp` / `agentapi` / stamp) | notify vs rare `--chat` bootstrap |

Vendor product names (e.g. Antigravity/`agy`) belong in adapter discovery only — not as the identity of who executed work.

**Binding objects:** `PROMPT-PEER-ORCH-INIT-001`, `PROMPT-TPM-ORCH-INIT-001`, `WFL-MMORCH-OPERATIONAL-RUNBOOK`, `WFL-TPM-AGY-MESH-001`, `POL-AGENT-MESH-WAKE-001`, `POL-AGENT-COMMS-CHECK-001`, `POL-AGENT-ORCH-HOURGLASS-001`, `WFL-SUBAGENT-DISPATCH`, `REQ-COMMS-RUNTIME-SESSION-001`, `POL-CODE-CAS-TPM-GET-001`, `POL-WORKFLOW-CEF-RECO-COMPLETE-001`, `POL-WORKFLOW-VDS`.

**Payload files (repo):** `scripts/mesh/bootstrap/antigravity-*-session-init.txt` (legacy AGY adapter bootstrap; seat ids in live `peer_seats.json` may still use historical names until remapped)

---

## Non-negotiable rule

| Phase | Channel | What peers receive | When |
|------:|---------|--------------------|------|
| **A — Session bootstrap** | **Full chat turn** (`wake-agy --chat` / explicit human paste) | Persona + seat id + wake/substance contract + role + `PROMPT-PEER-ORCH-INIT-001` + prepare-context mandate | **Once per seat per session** (or after brain GUID / process restart) |
| **B — Steady-state** | **Notify-only** (`feed steer` wake / `--notify-only`) | Short ATTN + AFE id (“substance on feed”) | After Phase A PASS; all routine steers / COMMS / ATK handoffs |

**Forbidden:** Phase B (notify strip / nonce-only COMMS) as the **first** message of a cold peer session.  
**Forbidden:** Treating agentapi inbox strip as a transcript user message.  
**Forbidden:** Plan handoff (`agent orchestrate <PRI>`) before **COMMS life∧work PASS** for every required seat.

Human opt-in: mesh policy defaults to notify-only. Phase A `--chat` requires **explicit human opt-in that turn** (this doc + TPM charter).

---

## Repeatable bring-up (`zqk agent swarm-init`)

Do **not** improvise notify strips. Cold seats run the MMORCH pipeline; already-bootstrapped seats use the Phase B recipe (no chat). `--refresh-seats` is forbidden in every stage (`TDE-MESH-DEFAULT-SEAT-PEER-AGENT-01-001`).

```bash
# Cold start (optional chat_bootstrap only with --allow-chat)
./bin/zqk agent swarm-init --pipeline PIP-SWARM-INIT-MMORCH-001 --plan-id PRI-…
./bin/zqk agent swarm-init --workflow WFL-TPM-AGY-MESH-001 --allow-chat

# Steady-state / public-shaped (conversation + heartbeat must already pass)
./bin/zqk agent swarm-init --pipeline PIP-SWARM-INIT-PHASE-B-001 --dry-run
./bin/zqk agent swarm-init --pipeline PIP-SWARM-INIT-PHASE-B-001 --from-stage comms_check
```

`--chat` remains one optional executor (`chat_bootstrap`), not the default control plane.

## Phase A — TPM checklist (do all)

1. **Control plane (post-reboot first):** follow `docs/onboarding/STABLE_BINARY_MANAGEMENT.md` § **After machine reboot / login** — recycle daemons, `jobs_paused`, **disable `SCH-cap-night-duty`**, keep `SCH-cap-orchestrator` enabled, `mcp ensure` until `mcp_subscribers >= 1`. Prefer **`zqk agent swarm-init --pipeline PIP-SWARM-INIT-MMORCH-001`** (or `--workflow WFL-TPM-AGY-MESH-001`) so bind/seat-worker/COMMS/plan dispatch are staged with pass predicates. Confirm `pid` **and** `conversation` in `.zqk/state/mesh/peer_seats.json` and seat-worker `.alive.json`. **Do not** default to `feed doctor --refresh-seats` — it can collapse seats to `peer-agent-01` (TRACK `TDE-MESH-DEFAULT-SEAT-PEER-AGENT-01-001`). Restore the canonical seat map by hand if refresh ran.
2. **Skills sync (hosts):** `bash ./scripts/sync-ide-skills-from-kernel.sh --all --check`
3. **Board:** shovel-ready `priority_plan` seated (`active_order`); no hollow completes.
4. **Build / refresh bootstrap payloads** (or use committed templates) when `--allow-chat` will run `chat_bootstrap`:
   - `scripts/mesh/bootstrap/antigravity-1-session-init.txt` → seat `antigravity-1`, persona `PER-ORCH-ALPHA` (plan primary)
   - `scripts/mesh/bootstrap/antigravity-2-session-init.txt` → seat `antigravity-2`, persona `PER-ORCH-BETA` (kernel health / fast-response)
5. **Deliver Phase A** (opt-in chat). Prefer the pipeline executor:

```bash
./bin/zqk agent swarm-init --pipeline PIP-SWARM-INIT-MMORCH-001 --allow-chat --plan-id PRI-…
```

Manual equivalent (human opt-in only):

```bash
./scripts/wake-agy.sh --chat --pid <agy1_pid> --conversation <guid1> \
  "$(cat scripts/mesh/bootstrap/antigravity-1-session-init.txt)"
./scripts/wake-agy.sh --chat --pid <agy2_pid> --conversation <guid2> \
  "$(cat scripts/mesh/bootstrap/antigravity-2-session-init.txt)"
```

6. **Also stamp feed** (durable copy) with `--await-peer-ack` pointing at the same bootstrap contract (short pointer + path is OK if body is huge). Swarm-init stamps the run id via `feed emit-status`.
7. **Wait for** seat-authored `BOOTSTRAP-OK <seat> persona=…; whats-next.priority_plan.id=…` under the **correct** `agent_id` (never `human`).

### Peer must internalize (Phase A content)

- `--agent-id` = seat in `peer_seats.json` (`antigravity-1` / `antigravity-2`)
- `--persona-ref` = `PER-ORCH-ALPHA` / `PER-ORCH-BETA`
- Notify = doorbell; substance = feed + `whats-next`
- Subagents only via `zqk agent prepare-context` then orchestrate (`WFL-SUBAGENT-DISPATCH`)
- Hourglass on every directed steer / `agent next --on-validation-failure wake`
- **Done-claim gate:** before any COMPLETE steer, `zqk object get` the BLI **and** every `criteria_ref`. Do not stamp if any CRIT is not `complete` or any recommendation sentence lacks a code path + test. `validated` ≠ fulfilled. Feed COMPLETE is not kernel complete (`POL-CODE-CAS-TPM-GET-001`). Anti-idle does not license a 100% stamp. Until `object get <BLI> --view backlog-completion-report` exists, this N-get join **is** the gate (TRACK `REDACTED`).

---

## Phase B — after BOOTSTRAP-OK

1. **COMMS-CHECK** (`POL-AGENT-COMMS-CHECK-001` / `REQ-COMMS-SEAT-WORKER-001`) — life ∧ work, same nonce, correct `agent_id`. Prefer **`zqk agent swarm-init --pipeline PIP-SWARM-INIT-PHASE-B-001`** (or `--from-stage comms_check`) so conversation/heartbeat predicates fail closed before dispatch. Seat-workers must run with `--execute-non-comms` when the recipe says so; AGY/Cursor UIs are optional observers of Mesh Feed.
2. **Plan handoff (whole PRI):** first line **`ORCHESTRATE_PLAN PRI-…`** via the `orchestrate_plan` executor (not TPM `zqk agent orchestrate` in the studio tree). Peers orchestrate subagents.
3. **Steady wakes:** notify-only / `feed steer --await-peer-ack` with short ATTN + AFE. Substance stays on feed; seat-worker consumes inbox.
4. **Mid-session COMMS break:** stop ATK push; re-seat; restart seat-worker; re-run COMMS; only then resume. If brain GUID changed → **re-run Phase A** (human opt-in chat once).

Phase A `--chat` remains **human opt-in once** per cold seat. Phase B never depends on paste or strip interpretation for COMMS PASS.

---

## Role split (default MMORCH evening)

| Seat | Persona | Lane |
|------|---------|------|
| `antigravity-1` | `PER-ORCH-ALPHA` | Optional AGY observer + seat-worker bind — plan primary when orchestrating |
| `antigravity-2` | `PER-ORCH-BETA` | Optional AGY observer + seat-worker bind — kernel health / interrupt sponge |
| `zqk-worker-*` | ORCH persona | ZQK-native COMMS/ATK executor (see `scripts/mesh/peer_seats.example.json`) |
| `cursor-composer` | TPM persona | Board, COMMS gate, hourglass, PR-to-main — **no** peer BLI coding |

---

## Failure signatures → fix

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Peer “barely coherent” / nonce-only replies | Phase A skipped; only notify strips | Run Phase A `--chat` bootstrap **or** start `zqk agent seat-worker` |
| Replies as `agent_id=human` | Seat identity never onboarded | Phase A + reject until stamped as seat |
| `delivery_receipt` but COMMS FAIL | Transport ≠ life∧work; seat-worker down | Start seat-worker; demand seat ack + WORK; fail closed if `.alive.json` missing |
| Hollow “complete” on feed | No VDS; skipped tests; invented TRACK ids; CRIT status flipped without the recommendation sentences | REJECT; demote BLI; do not hand more work. Re-load `PROMPT-PEER-ORCH-INIT-001` done-claim gate |
| Cold `prepare-context` never run | TPM generated file locally only | Peer must run `prepare-context` in **their** shell |

---

## Related

- E2E COMMS: `docs/onboarding/MESH_COMMS_E2E_SETUP.md`
- Mesh ops: `scripts/mesh/README.md`
- Correspondence standard: `docs/architecture/AGENT_CORRESPONDENCE_FEED_STANDARD.md`
- Post-baseline handoff: `docs/onboarding/AGENT_HANDOFF_2026-08-12_POST_BASELINE_REBOOT.md`
