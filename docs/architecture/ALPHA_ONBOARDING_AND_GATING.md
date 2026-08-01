# Alpha: Onboarding, Gating, and Progressive Access

**Version**: 1.0.0  
**Status**: Design (alpha preparation)  
**Purpose**: Ensure bootstrap/init is organized and clear; gate system manipulation until users/agents are certified; support per-account onboarding duplication, archival, and a "my desktop" space; phase tutorials by complexity; and grant full CLI access only after certification.

## Goals

- **Init clarity**: New onboardees understand how the system works from the start. Init and first-run experience are unambiguous.
- **Gating**: Prevent manipulating the system or other code until certified with basic understanding and expectations.
- **Per-account onboarding**: Tutorials are per-account (not shared project state). Duplicate curriculum for each new team member; associate with their account and privileges; archive and issue certificate on completion to avoid stale data.
- **Layered tutorials**: Curriculum can evolve and be phased in layers of increasing complexity without overwhelming new resources.
- **"My desktop"**: User-scoped side objectives/missions that are separate from core system mission but supportive of it; specific to the user account.
- **Progressive CLI access**: Do not grant full CLI access until a certain level of proficiency or certification; certifications increase scope of access.

---

## 1. Bootstrap / Init Phase

### 1.1 Current State

- Init supports greenfield, legacy, snapshot; creates `.zqk/`, `docs/architecture/`, extracts bootstrap, writes config; optional `--discover`, `--with-maintenance-jobs`.
- No post-init "welcome" or explicit pointer to onboarding. No gating of commands.

### 1.2 Alpha Improvements

**A. Init output and first-run clarity**

- After successful init, print a short **welcome block** that:
  - Confirms project name and project root.
  - Points to "Start Here" for new users/agents: e.g. run `zqk system start-here` (or show the Start Here policy ID and `zqk object get <id>`).
  - States that **mutating commands** (object create/update/delete, internal, etc.) are restricted until onboarding is complete (when gating is enabled).
- Option: **first-run file** (e.g. `.zqk/state/first_run_done`) so the welcome is shown only once per project (or always on init, depending on product choice).

**B. Optional: `zqk system start-here`**

- Command that prints the Start Here tutorial steps and relevant object IDs (Operational Philosophy policy, onboarding plan/workstream, `zqk object list backlog_item --filter priority_plan_ref=PRIO-onboarding`). No new object kinds; uses existing policies and onboarding roadmap.

**C. Init and onboarding roadmap**

- Init does **not** create onboarding workstream/plan/items by default (they are project-level templates). Optionally, init could ensure the **template** onboarding objects exist (from `scripts/onboarding_roadmap/`) when a flag like `--with-onboarding-templates` is set, so the project is ready to duplicate for new accounts.

---

## 2. Gating: No Manipulation Until Certified

### 2.1 Principle

- **Uncertified** users/agents (no `onboarding_certified` certification or equivalent role) must not:
  - Create, update, or delete project objects (object create/update/delete, bulk, move, etc.).
  - Use `internal` (admin) or other privileged commands.
  - Run destructive system commands (e.g. init --force, snapshot --wipe) or repair commands that mutate shared state.
- **Certified** users/agents can use the full CLI (subject to existing MCP config and role-based filtering when running via MCP).

### 2.2 Command Tiers (Progressive Access)

| Tier | Who | Commands (examples) |
|------|-----|----------------------|
| **0** | Everyone | `help`, `version`, `system whoami`, `system start-here` (if implemented), `auth login/logout` |
| **1** | Uncertified (onboarding) | Tier 0 + read-only: `object list`, `object get`, `system check` (read), `system retention-status`, `object list policy`, etc. Only commands needed to complete the onboarding curriculum. |
| **2** | Certified (onboarding_certified) | Tier 0 + Tier 1 + object create/update/delete (project objects), bulk, move; scheduler submit (non-admin); utility; reports; quick; etc. |
| **3** | Admin / elevated | Tier 2 + `internal`, system repair/destructive, init (when gating applies to init), etc. |

- **CLI**: In alpha, gating can be implemented in **root or group PersistentPreRunE**: resolve current account (from session/env/keystore); if account has no certification/role `onboarding_certified`, check command path against an allowlist (tier 0 + tier 1). If not allowed, return a clear error: "This command requires onboarding certification. Run `zqk system start-here` to begin."
- **MCP**: Already has **FilterCommandsByPermissions** and config-based **exposed_commands** / **blocked_commands**. Map certification to roles/permissions: e.g. uncertified → role `onboarding` with permissions only for tier 1 commands; certified → role `onboarding_certified` or existing `coder_agent` etc. with broader permissions. Use existing MCP security context so tool list is filtered by certification-derived role.

### 2.3 Certification and Roles

- **Certification object** (see [ONBOARDING_ROADMAP_AND_CERTIFICATION.md](ONBOARDING_ROADMAP_AND_CERTIFICATION.md)): `certification_type: onboarding`, `status: awarded` for an account.
- **Role**: When certification is awarded, add role `onboarding_certified` (or equivalent) to the account so that:
  - CLI gating allows tier 2 (and optionally tier 3 for admin).
  - MCP permission cache and FilterCommandsByPermissions see the updated role and expose the right tools.

---

## 3. Per-Account Onboarding: Duplicate, Associate, Archive

### 3.1 Template vs Instance

- **Template**: Canonical onboarding curriculum — one workstream, one priority plan, N backlog items (see `scripts/onboarding_roadmap/`). Stored as normal project objects; IDs fixed or known (e.g. `PRIO-onboarding`, workstream ID, backlog item IDs).
- **Instance**: A **copy** of the curriculum for a specific account. Each new team member gets their own workstream (or plan) and their own backlog items so that:
  - Completion is per-account (they mark *their* items complete).
  - Progress does not affect other users.
  - Data can be archived per-account when done.

### 3.2 Duplication Flow

- **Duplicate for new team member**: One-shot or command, e.g. `zqk onboarding duplicate-for-account --account-id account:new_agent` (or via admin/internal).
  - Creates: copy of onboarding workstream (e.g. title "Onboarding – account:new_agent"); copy of priority plan (e.g. "PRIO-onboarding-&lt;account_suffix&gt;"); copies of each backlog item, with `priority_plan_ref` and `workstream_refs` pointing to the new plan and workstream.
  - Associates: link the new workstream/plan to the account (e.g. **account_onboarding** object: `account_ref`, `workstream_ref`, `priority_plan_ref`, `status: in_progress`). This gives a single place to look up "this account's onboarding instance."
- **Quick duplicate**: Script or CLI that reads template IDs from config or from the canonical onboarding objects and runs the duplicate flow so adding a new team member is a single invocation.

### 3.3 Archival and Certificate on Completion

- When the account has completed all items in their onboarding instance (or an admin marks complete):
  - **Award certification**: Create/update certification object for this account, type `onboarding`, status `awarded`; add role `onboarding_certified` to account.
  - **Archive instance**: Move or mark the account's onboarding workstream/plan/items as archived (or export to a compressed/archived store per account) so they do not clutter the main project index. Option: **archive** lifecycle status on workstream and plan; or move to a dedicated namespace/directory (e.g. `docs/architecture/onboarding_archive/<account_id>/`) and optionally compress.
  - **Certificate issued**: Certification object is the "certificate of completion"; optionally issue X.509 later (see ONBOARDING_ROADMAP_AND_CERTIFICATION.md). Certificate (object or X.509) is the component of user identification/credential that grants elevated privileges.

---

## 4. "My Desktop" (User-Scoped Objectives)

### 4.1 Concept

- A **"my desktop"** area: side objectives, missions, or work that are **specific to the user account** and separate from the core project mission, but supportive of it (e.g. personal learning goals, agent-specific experiments).
- Distinguishes:
  - **Project-wide**: Shared workstreams, priority plans, backlog (core mission and goals).
  - **User-scoped**: Workstreams/plans/backlog that belong to an account and are namespaced so they do not mix with project-wide data.

### 4.2 Design Options

**A. Namespace or attribute on existing objects**

- Add **owner_ref** or **scope** (e.g. `project` vs `account`) to workstream, priority_plan, backlog_item. When `scope: account` and `owner_ref: account:alice`, only that account (and admin) can see or mutate them. CLI: `zqk object list backlog_item --filter scope=account --filter owner_ref=account:alice` (or resolve from current session).

**B. Dedicated top-level command: `zqk desktop` (or `zqk my`)**

- **`zqk desktop`** (or **`zqk my`**): Subcommands such as `list`, `create`, `object list`-style queries but scoped to the current account. Under the hood: same object kinds (workstream, priority_plan, backlog_item) with a filter or namespace that restricts to "this account's desktop." Implementation can be a thin wrapper: e.g. `zqk desktop backlog list` → `zqk object list backlog_item` with `owner_ref=<current_account>` and optionally a "desktop" workstream tag.
- Benefits: Clear mental model ("my desktop" vs "project"); easy to show only user-scoped items in UI/CLI.

**C. Separate object store or directory**

- Store desktop objects under `.zqk/desktop/<account_id>/` or under `docs/architecture/desktop/<account_id>/` so they are physically separate. Requires loader/storage to support multiple roots or a "desktop" namespace. Heavier than A/B.

**Recommendation for alpha**: Option B (dedicated `zqk desktop` command) with backend implemented via **scope/owner_ref** on existing kinds (option A) so that desktop items are first-class objects but filtered by account. No separate store initially.

### 4.3 Desktop and Onboarding

- Onboarding **instance** (per-account copy) could be considered the first "desktop" workstream for that account. After certification and archival, the desktop can still be used for ongoing user-scoped objectives (new workstreams/plans/items with `scope: account`).

---

## 5. Layered Tutorials (Phased Complexity)

- Curriculum in `scripts/onboarding_roadmap/` (and any policy body) can be **versioned and phased**:
  - **Layer 1**: Minimal (read philosophy, run start-here, list goals, run retention-status and check). Few backlog items; quick to complete.
  - **Layer 2**: Add items (e.g. run a non-destructive system command, list policies by category, explain priority plan).
  - **Layer 3**: Deeper (e.g. create a test backlog item in a sandbox, run a read-only report).
- **Implementation**: Multiple priority plans (e.g. `PRIO-onboarding-layer1`, `PRIO-onboarding-layer2`) or a single plan with priority_tier (P0 = layer 1, P1 = layer 2, P2 = layer 3). Certification can be awarded after layer 1 for "basic" access and after layer 2 for "full" access, or a single certification after the desired layer. Phasing is data-driven (backlog items and plan structure); no code change required to add a layer.

---

## 6. Implementation Order (Alpha)

1. **Init welcome and start-here**
   - Add post-init welcome message (and optional `zqk system start-here`) pointing to Start Here policy and onboarding roadmap. No gating yet.
2. **Certification object and award flow**
   - Implement certification object (and optionally completion records); CLI to create/list/get; award flow (manual or automated when all onboarding items for an account are done). On award: set account role `onboarding_certified`.
3. **CLI gating**
   - In root or group PersistentPreRunE: resolve account (e.g. from env/session; default to system for backward compatibility when no account). If account is not system and does not have role `onboarding_certified`, allow only tier 0 + tier 1 commands; else allow full. Clear error message when blocked.
4. **MCP alignment**
   - Ensure uncertified accounts get a security context with roles/permissions that restrict tools to tier 1; certified accounts get broader permissions. Reuse FilterCommandsByPermissions and config.
5. **Per-account onboarding duplication**
   - Implement "duplicate onboarding for account" (script or CLI): copy workstream, plan, backlog items; create account_onboarding (or similar) linking account to the new instance. Document in ONBOARDING_ROADMAP_AND_CERTIFICATION.md.
6. **Archival on completion**
   - When certification is awarded, archive the account's onboarding workstream/plan/items (lifecycle status or move to archive area); keep certification object and account role as the long-lived record.
7. **My desktop**
   - Add `zqk desktop` (or `zqk my`) with subcommands; scope workstream/backlog by owner_ref or scope when implemented. Optional for alpha if time is short.

---

## 7. Related

- [ONBOARDING_ROADMAP_AND_CERTIFICATION.md](ONBOARDING_ROADMAP_AND_CERTIFICATION.md) – certification object, X.509, roadmap structure
- [OBJECT_FIRST_ALIGNMENT.md](../enforcement/OBJECT_FIRST_ALIGNMENT.md) – philosophy and Start Here
- [scripts/onboarding_roadmap/](../../scripts/onboarding_roadmap/README.md) – template YAMLs, creation order, **advanced-tutorial reference pattern**
- [onboarding_roadmap_seed.yaml](../../scripts/scheduler_jobs/onboarding_roadmap_seed.yaml) – one-shot scheduler job (`init --with-onboarding-roadmap` + start scheduler)
- MCP: `pkg/mcp/cli_bridge_permissions.go`, `pkg/mcp/config_security.go` – permission filtering and config-based allowlist/blocklist
- Init: `cmd/zqk/system/init.go`, `init_impl.go`
