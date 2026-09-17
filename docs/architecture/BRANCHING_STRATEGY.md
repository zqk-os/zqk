# ZQK Git Branching & Integration Strategy

## 1. Overview
This specification defines a rigorous, trace-driven Git branching model for ZQK. It maps branches directly to the Knowledge Kernel ontology (Priority Plans -> Workstreams -> Backlog Items -> Agent Tasks) and is optimized for concurrent, single-host Swarm development utilizing `git worktree`.

## 1a. Mint from updated `main` (non-negotiable)

After a PR merges to `main`, **fetch `origin/main`** before creating the next branch. The integration branch (`integration/pri-{ID}` or a TPM-named collaboration branch) is minted **only** from that tip. Worker / peer branches are **git worktrees** off that integration branch — not `git checkout` in the studio working tree (shared index, mixed commits, missing CAS pairs).

Solo studio work follows the same mint rule: `git fetch origin main && git switch -c <branch> --no-track origin/main` (or equivalent). Do not branch from `HEAD` on a previous feature.

Push gate: `scripts/check-branch-contains-origin-main.sh` (`origin/main` must be an ancestor of the commit being pushed). Kernel: `POL-CODE-1784784370706305000-e3e7245a`.

## 2. Naming Conventions (Traceability Mandate)
All branches must strictly map to a valid ZQK object ID. Arbitrary or conversational names (e.g., `integration/v1.x`, `subagent-Swarm-XXX`) are prohibited.

| Object Type | Branch Naming Convention | Description |
| :--- | :--- | :--- |
| **Priority Plan** | `integration/pri-{ID}` | The primary integration target for all workstreams within a priority plan. |
| **Workstream** | `workstream/wks-{ID}` | Optional intermediate branch if a priority plan is massive. Branches off the `pri-{ID}`. |
| **Backlog Item** | `feature/bli-{ID}` or `fix/bli-{ID}`| The core development branch for a specific backlog item. Branches off the target `pri` or `wks`. |
| **Agent Task** | `task/tsk-{ID}` | The granular, localized branch created by a Swarm Worker executing an `agent_task`. |

**Live anchor:** `priority_plan.branch_ref` is the integration target the swarm must use. Default mint is `integration/pri-{ID}` when `branch_ref` is unset. A TPM-named collaboration branch may host several in-flight plans that share one bake-in trunk (example: `integration/cef-r27-failclosed-structure` for the CEF R27 / fail-closed / structure column). Do not mint a second `integration/pri-*` trunk for a plan whose `branch_ref` already names the collaboration branch.

## 3. Swarm Worker Workflow & Concurrency (`git worktree`)
To allow multiple Swarm Workers to act concurrently on a single host without Git index collisions, agents must utilize `git worktree` rather than `git checkout`.

### 3.1. Branching (Initialization) & Dynamic Anchoring
To scale massively, the Swarm orchestrator must support multiple, isolated integration branches anchored to fundamentally unique system objects (like Priority Plans). The branch provisioning logic must dynamically determine the correct base branch for a new task.

1. **Discover Target:** Worker queries its `agent_task` object (e.g., `tsk-123`) to identify the parent `backlog_item` (`bli-456`).
2. **Resolve Integration Anchor (Graph Traversal):** The orchestrator logic must dynamically climb the object graph (`Task` -> `Backlog Item` -> `Workstream` -> `Priority Plan`) to resolve the base anchor branch. 
   - If the task traces back to an active Priority Plan, the base branch is the plan's integration branch (e.g. `integration/pri-789`).
   - If the task is an isolated hotfix or ad-hoc goal with no Priority Plan, the base branch defaults to `main`.
3. **Resolve Parent Branch:** The orchestrator determines the immediate parent branch for the task: `feature/bli-456` (which itself was branched from the resolved anchor).
4. **Create Worktree:** The worker executes a branch and worktree checkout into a sibling directory, keeping agents grouped by their integration branch:
   ```bash
   git worktree add ../zqk-tsk-123 -b task/tsk-123 feature/bli-456
   ```
4. **Execution:** The agent performs all modifications, builds (`make zqk`), and tests within `../zqk-tsk-123`.

### 3.2. Merging & Verification Lock
1. **Pre-merge Verification:** The Swarm Worker fulfills the `acceptance_criteria` of its task prompt within the worktree.
2. **Commit:** Commits must reference the task and backlog item: `feat(tsk-123): implement X for bli-456`.
3. **Transition State:** Worker transitions `agent_task` to `pending_verification`.
4. **Merge Back:** Instead of direct pushes, the merge back to `feature/bli-{ID}` is handled programmatically via a ZQK verification hook/pipeline. Once verified, the branch is merged.

## 4. Protecting `main`
- **Strict Lockdown:** Direct pushes to `main` are strictly prohibited (PR-Only Development).
- **Integration Flow:** 
  1. `task/tsk-{ID}` merges into `feature/bli-{ID}`.
  2. `feature/bli-{ID}` merges into `integration/pri-{ID}` upon Backlog Item completion.
  3. `integration/pri-{ID}` is only merged into `main` after the entire Priority Plan undergoes an automated Architect Review and passes the global `make verify` and `make build-all` checks.
- **Verification Hook:** PRs to `main` require a programmatic approval from the ZQK Pipeline confirming that all associated `goal_refs` and `requirement_refs` have validated acceptance criteria.

## 5. Handling Orphaned or Abandoned Branches (The Quarantine Protocol)
Swarm workers are ephemeral and prone to sudden termination or failure. Furthermore, ZQK system objects may occasionally be transitioned to `abandoned` or `completed` errantly without human authorization. 

To prevent branch clutter while **absolutely guaranteeing zero code loss**, ZQK uses a **Git Archive Namespace (Soft-Delete)**:

1. **State-Driven Garbage Collection:** 
   Abandoned branches are managed by querying the ZQK Kernel for `agent_task` or `backlog_item` objects in `failed`, `abandoned`, or `completed` states.
2. **Worktree Teardown:** 
   The ZQK daemon safely unmounts the isolated worktree (`git worktree remove --force ../zqk-tsk-{ID}`).
3. **The Soft-Delete (Quarantine):** 
   Instead of running `git branch -D`, the daemon moves the branch pointer into a hidden Git namespace (`refs/archive/`):
   ```bash
   git update-ref refs/archive/task/tsk-{ID} task/tsk-{ID}
   git branch -D task/tsk-{ID}
   ```
4. **Result:** 
   The branch vanishes from your `git branch` list (keeping your local environment pristine), but the code is permanently preserved and immune to Git garbage collection. 
5. **Resurrection:**
   If you (or an agent) decide to resurrect a prematurely closed item, the code is restored instantly:
   ```bash
   git checkout -b task/tsk-{ID} refs/archive/task/tsk-{ID}
   ```
