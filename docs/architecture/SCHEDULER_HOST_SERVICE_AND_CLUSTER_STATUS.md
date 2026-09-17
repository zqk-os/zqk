# Scheduler Host Service and Cluster Status Plane

**Last Verified:** 2026-08-31


**Status:** active architecture (exemplar orchestration program)  
**Umbrella plan:** `[REDACTED-ID]`  
**Related:** [`PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md`](./PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md), [`OPEN_CORE_PUBLIC_RELEASE_HYGIENE.md`](../strategy/open-core/OPEN_CORE_PUBLIC_RELEASE_HYGIENE.md), [`OPEN_CORE_PHASE0_INVENTORY.md`](../strategy/open-core/OPEN_CORE_PHASE0_INVENTORY.md)

## Problem

Per-root PID/keepalive already exist under `<root>/.zqk/scheduler/`, but supervision was an **external cron + shell prosthetic** (`scripts/ensure-scheduler-running.sh`). That path:

- races on restart,
- can **copy over a live daemon binary** (workshop `zqk-stable`),
- can **SIGKILL other roots’ daemons** via host-wide orphan scan when a PID file is missing,
- leaves `no-auto-restart` as a file only cron must understand.

Mesh has `remote_kernel` / lease supervision seeds, but dependents need a first-class **cluster status plane** (API-gateway shaped) so remote job outcomes permeate without SSH into peer process trees.

## Decisions (locked)

| Topic | Decision |
|-------|----------|
| Topology | One **host OS unit per project root** (launchd LaunchAgent / systemd `--user`) |
| Privilege | `zqk scheduler service …` gated by capability (`scheduler_control` / system account) |
| Identity | Stable `root_id` = hash(canonical abs root + brand); paths are mutable via **rebind** |
| Hard cut | **No** ensure-cron dual path; prosthetics deleted when service path ships |
| Platforms | Darwin + Linux first; **Windows designed now, SCM later** (TRACK) |
| Editions | CE: local host service; Studio: full mesh gateway; EE: HA gateway + **Kubernetes** |
| Workshop vs customer | Service units use **product/install binary**, not Studio `zqk-stable` as the public contract |

## Root lifecycle (same host)

| Event | Behavior |
|-------|----------|
| Path rename/move | `service rebind --root-id <id> --new-path <abs>` updates registry + unit WorkingDirectory/env; restart that unit only |
| Project archived / dir gone | `service gc` disables unit, removes plist/unit, clears registry |
| New project | Fresh `install --root <new>` → new `root_id` |
| `zqk use` | Stop/start via **service API**, not ad-hoc SIGTERM-only |

`root_id` stays stable across intentional rebind; archival + new project always allocates a **new** id.

## Layer 1 — Per-root host units

CLI: `zqk scheduler service install|list|start|stop|restart|status|notify|uninstall|gc|rebind`

- **macOS:** `~/Library/LaunchAgents/com.<brand>.scheduler.<rootId>.plist`
- **Linux:** `~/.config/systemd/user/zqk-scheduler@<rootId>.service`
- **Windows (deferred):** same verbs + registry; SCM adapter behind OS interface — TRACK on Phase 2 Windows BLI

**Registry:** `~/.zqk/scheduler-services/registry.json` (brand-prefixed home) — `{root_id, abs_root, unit_label, installed_at, desired_state, binary_ref}`.

**Binary contract:** `ResolveServiceDaemonBinary(root)` prefers configured product binary / `bin/zqk`, **not** requiring `.zqk/bin/zqk-stable`. Workshop may still promote stable **internally**; that must not appear as CE requirement.

## Layer 2 — Failure isolation

1. Orphan recovery only signals PIDs bound to **this** root (cmdline embeds root path or `ZQK_PROJECT_ROOT`).
2. No live-binary clobber from remediator scripts.
3. `desired_state=disabled` + OS disable replaces `no-auto-restart` file semantics for supervision.
   - **macOS:** `scheduler service stop` must `launchctl bootout` (not only `stop`/`kill`). Crash-only `KeepAlive` (`SuccessfulExit=false`) respawns after SIGTERM non-zero exits, which made stop print success while `status` stayed `running`.
   - **Linux:** `stop` + `disable`; `start` re-`enable --now`.
4. Crash loops → OS backoff + emit `scheduler.degraded` on the status plane.
5. `service gc` for missing `abs_root` / `desired_state=absent`.

## Layer 3 — Cluster status plane (API gateway)

Normalized events:

```text
{node_id, root_id, job_id|task_id, phase, progress, error, updated_at}
```

| Concern | Pattern |
|---------|---------|
| Ingress | Scheduler/job wrappers emit |
| Authz | Capability / mesh identity; CE may be local-only |
| Routing | Dependents declare `depends_on` watches |
| TTL | Stale → `degraded` / `awaiting_peer` (fail closed) |
| Industry map | Health/readiness, watch APIs, lease liveness (document analogies; reuse remote_kernel / mesh lease before inventing stores) |

Permeation phases: pull (`mesh status`) → push (feed/MCP) → lease supervision enforcement.

## Layer 4 — Kubernetes prep (enterprise-only)

Map the **same** identity/registry/status contracts onto Deployment/Service + probes. Ship host units first; EE surfaces are gated. Community must not require a cluster.

### EE mapping appendix (non-boil)

| Host concept | K8s analogue |
|--------------|--------------|
| `root_id` + unit | Deployment/StatefulSet name + labels |
| WorkingDirectory / project root | volume mount + env |
| `KeepAlive` / Restart | `restartPolicy` / Deployment rollout |
| Registry | ConfigMap / CRD inventory (future) |
| Status gateway | Service (ClusterIP/headless) + watch API |
| Probes | readiness/liveness aligned to status `phase` |

Edition gate: community binary must not register k8s adapters; Studio may lab; EE ships gated commands/charts.

## Edition & workshop demarkation

| Concern | Community | Studio workshop | Enterprise |
|---------|-----------|-----------------|------------|
| Host `scheduler service` | Yes (core) | Yes + promote-stable tooling | Yes |
| ensure-cron | **Absent** | **Absent** | Absent |
| Multi-node status gateway | Local/minimal | Full mesh | Full + HA |
| Cross-host degrade | Documented CE gap | Full | Full + policy packs |
| Kubernetes | No | Optional lab | **EE-only** |
| Binary | Product/install | May use `zqk-stable` workshop-only | Images/charts |

## Process graph (exemplar)

Umbrella `[REDACTED-ID]` with Phase 0–4 + EE child PRIs, milestones, BLIs, REQs, CRTs, and test cases. Traceability: BLI → REQ → CRT → test. Swarm-parallel Phase 2 adapter BLIs must not collide on the same adapter file.

## Migration (one-way)

1. `zqk scheduler service install --root <path>`
2. Remove ensure-cron / `ensure-scheduler-running.sh` supervisor role
3. Do **not** reinstall cron “for safety”

## Success criteria (program)

- Two roots isolated; kill A → OS restarts A only
- Archive + `gc` leaves no OS residue
- `rebind` preserves `root_id` when intended
- Dependents fail closed on stale peer status
- Zero ensure-cron supervisor path after migration
- CE/Studio/EE boundaries enforced in REQ/edition gates
