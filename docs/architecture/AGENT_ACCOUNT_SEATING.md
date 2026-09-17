# Agent account seating (ACC + role + persona)

**Last Verified:** 2026-08-31


**Policy:** `POL-AGENT-ACCOUNT-LOGIN-001` (`POL-CODE-1785905132115717000-81ab632e`)  
**Criterion:** `CRI-ACCOUNT-RBAC-READY` (`CRIT-1785905131008420000-d90a0622`)  
**TRACK:** `BLI-1785905136581480000-1f317f44`

## Rules

1. Every agent session authenticates as an **`ACC-*`** kernel account (not `account:*`).
2. The account must carry **roles** and a **persona** (`persona_ref` / `persona_refs` / `persona`) before CLI/MCP dispatch.
3. System account (`SystemAccountID`) and test harness remain break-glass paths inside `AuthMiddleware`.
4. **No** `ZQK_ALLOW_UNBOUND_ACCOUNT` opt-out — unbound ACC is unauthorized (BLI-ENV-BREAKGLASS-REMOVE-001).

## Identity vs grants

**ACC** is the audit identity (who acted across sessions). **Roles** are the group-shaped grant (what the seat may write). **Persona** is the interaction lens (TPM vs coder prompts/policies). Do not infer write power from chat title or `--agent-id cursor-composer`.

Live snapshot (not CAS): `.zqk/state/identity_status.json` (`zqk_identity_status_v1`), written by AuthMiddleware and `zqk system whoami`. Status bars read that file (`scripts/cursor-statusline-identity.sh`). `zqk system status` includes an `identity` object.

Planner vs doer is decided from **role objects** (`permissions` + `aliases`) and **account bindings** (`persona_ref`, `roles`) via `authcred.SeatDirectory`. A ref is planner only when it matches a role that grants `agent:orchestrate` (or an account bound to such a role). Substring tokens (`tpm`, `orch`) are not a signal. Planner refs that match a planner role but have **no** seated ACC must **not** fall back to `DefaultSwarmWorkerAccount` (empty = seating miss).

## TPM / planner seat

| Human title | Kernel binding |
|-------------|----------------|
| Cursor TPM (this studio) | `ACC-1787804696944532000-f7fb7d6c` (`username: cursor_tpm`, `roles: [cap_orchestrator]` → `ROL-AGENT-ORCH`, `persona_ref: PER-1781253460190606080-f6ad5d4a`) |
| Swarm worker pool | `ACC-1785920548450214011-dabd3692` (`roles: [swarm_worker]`) — doer only |

Project `.zqk/credentials` must be the TPM ACC for this IDE checkout. Shared `~/.zqk/credentials` pointing at the swarm ACC is how a TPM chat silently lost kernel writes.

## Swarm worker pool pattern

| Human title | Kernel id | Binding |
|-------------|-----------|---------|
| `swarm_worker_1` | `ACC-<ts>-<hex>` | `roles` + `persona_ref` |
| `swarm_worker_N` | distinct `ACC-*` | same |

Titles may be descriptive; **ids stay ACC-***. Seating prefers an **issued** opaque
`ZQK_API_KEY` (`zqk_ak_…`) from `zqk keystore issue --account-id ACC-…` (fingerprint in
keystore + `account.tokens`; local seating file under `.zqk/seating/credentials/`).
AuthMiddleware resolves issued secrets via keystore hash (POL-AGENT-API-KEY-001).
Transitional fallback: `ZQK_API_KEY=<ACC-id>` still works until every seat is issued.

Admin / codegen binary: `ZQK_ADMIN_API_KEY=<ACC-*>` or an issued secret for that ACC
(do not use `account:system`).

## Auth entry

`cmd/zqk/app/auth_middleware.go` → session / opaque key / ACC id resolution, then
`resolveSecurityContext` enforces ACC form and role+persona readiness.

**Issue keys:** `zqk keystore issue --account-id ACC-… --title swarm_worker_N`  
**Resolve seat for orchestrate:** `authcred.ResolveSeatAccount` maps `ACC-*` / `PER-*` / role labels → seated ACC (default swarm worker). Spawn injects via `APIKeyForSeat` + `WithSeatAPIKeyEnv` (POL-AGENT-API-KEY-001).

## Planner vs doer (POL-AGENT-PLANNER-DOER-001)

**TRACK:** `BLI-1785905540598640000-12d5118e` · criterion `CRI-PLANNER-DOER-SPLIT`

| Lane | Typical ACC roles | May | Must not |
|------|-------------------|-----|----------|
| Doer | `swarm_worker` (aliases to role_id `agent-swarm-worker`), `coder_agent` | `write:agent_task`, `write:code` | Strategic kernel writes; `agent orchestrate`; directed `feed steer --to-agent-id` |
| Planner | `agent-cap-orchestrator`, `owner`, `executive` | Strategic kinds + peer orchestration (`agent:orchestrate`) | `write:code` on canonical planner agent roles |

ACC `roles` labels are matched to immutable `role.role_id` via `authcred.RoleMatches` / aliases (e.g. `swarm_worker` → `agent-swarm-worker`). Helpers: `pkg/authcred/planner_doer.go`. Negative tests: `TestINV_RBAC_PD_001_*` (`TST-1785905543229739000-341d327c`).

## Object discovery membrane (list / count / fields --list-kinds)

**TRACK:** `BLI-1785908739114727000-9a7cc2bd` · POL-AGENT-PLANNER-DOER-001 discoverability

Bare `object list`, `object count`, and `object fields --list-kinds` default to a **seat-scoped kind catalog** (not the full ontology):

| Lane | Who | Default kinds |
|------|-----|----------------|
| Doer | swarm / coder (no orchestrate) | `agent_task` + agreed doer set (`agent_skill`, `criteria`, `test_case`, …) |
| Planner | CAP / orchestrate / strategic write | planning + strategic kernel + assignment kinds |
| Full | admin / owner / `write:*` / system | entire registry |

Break-glass: **`--all-kinds`**. Explicit `object <kind> …` is unchanged. Helpers: `pkg/authcred/discovery_membrane.go`.

## Resolved link hydration sidecar

**TRACK:** `BLI-1785909672838827000-9fca84f5`

Default `object get` is raw CAS. Opt-in hydration can write joinable metadata under **`.zqk/resolved/<kind>/<2hex>/<id>.json`** (`--write-resolved-sidecar` or `--resolved-sidecar-only`) so overlays never enter hashed CAS bytes. See `pkg/objectget/resolved_sidecar.go`.
