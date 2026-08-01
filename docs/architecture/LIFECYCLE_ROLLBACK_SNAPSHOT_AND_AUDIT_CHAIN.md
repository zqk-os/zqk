# Lifecycle Rollback: Status Snapshot and Audit-Chain Reconstruction

**Status:** Implemented  
**Related:** [LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md](./LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md), [change_journal_reconstruction.go](../pkg/storage/change_journal_reconstruction.go), [TIMESTAMP_SNAPSHOT_DESIGN.md](../process/testing/TIMESTAMP_SNAPSHOT_DESIGN.md)

## Goal

Support **rollback** of lifecycle-driven state changes with:

1. **Quick rollback** (within a configurable threshold): restore from a **snapshot of data states for all linked/connected items that have bearing on status**.
2. **Beyond threshold**: rollback via **reconstruction from the audit event chain** (change journal), using existing `ReconstructStateAtTimestamp`-style logic.

This gives **performance** (fast restore from snapshot when within window) and **redundancy** (audit chain always available for reconstruction when snapshot is no longer retained).

## Concepts

### 1. Status-relevant graph

For a given transition (e.g. priority_plan PRI-X → complete), the **linked/connected items that have bearing on status** are the minimal set of objects whose states must be consistent for the transition to be meaningful and reversible:

- **Primary**: The object whose status is changing (e.g. the priority_plan).
- **Connected**: Objects linked by reference that affect or are affected by that status (e.g. all `backlog_item` with `priority_plan_ref = PRI-X` — their statuses define “plan complete” and may need to be reverted together if we roll back the plan).

The snapshot captures **state** (at least id, kind, status; optionally other fields needed for consistency or re-apply) for each object in this set. Definition of “status-relevant graph” can be **per transition rule or per kind** (e.g. for `priority_plan` → complete: plan + all backlog_item with that `priority_plan_ref`).

### 2. Rollback point (snapshot)

A **rollback point** is a single, consistent snapshot of the status-relevant graph **before** (or at) the transition is applied:

- **When**: Captured immediately before the transition updater applies the status change(s). Optionally also after apply (post-transition point) for “roll forward” or comparison.
- **What**: For each object in the status-relevant graph, store minimal restore payload: `id`, `kind`, `status`, and any fields required to re-apply or validate (e.g. `priority_plan_ref`, `active_order`). No need to store full object unless required.
- **Where**: Use the **same infrastructure** as other durable, append-oriented data:
  - **Option A**: Dedicated rollback WAL (e.g. `.zqk/wal/rollback_points` or append-only file) — one record per rollback point, payload = list of `{ kind, id, status, ... }` or a compact blob.
  - **Option B**: A `rollback_point` object kind (or internal record) that holds the snapshot blob and metadata (timestamp, transition_id, rule_id). Retained only within the configured window.

### 3. Configurable threshold

- **Retention**: Keep rollback points for a **configurable** window, e.g.:
  - **Last N transitions** (e.g. last 50 lifecycle-driven transitions), or
  - **Last T time** (e.g. last 24 hours), or both (whichever is more restrictive).
- **Within threshold**: Rollback = **quick path** — load the rollback point for the chosen transition (or timestamp), then **reapply** the saved states to storage (bulk update or one update per object in the snapshot). Same infrastructure (write-behind or Update) as normal writes; no need to read the change journal.
- **Beyond threshold**: Rollback = **reconstruction path** — no snapshot available; use the **audit event chain** (change journal). For each object in the (recomputed) status-relevant graph, use `ReconstructStateAtTimestamp` (or equivalent) to reconstruct state at the target time, then apply. Slower but always available as long as change journal entries are retained.

### 4. Same infrastructure

- **Capture**: When the transition updater is about to apply a transition, it (or a dedicated “rollback capture” step) builds the status-relevant graph, reads current state for each object, and appends a rollback point to the rollback WAL or creates a `rollback_point` record. Same concurrency and durability patterns as lifecycle event WAL (append + optional sync).
- **Restore (quick)**: Read the rollback point, then for each object in the snapshot call `storage.Update` (or enqueue to write-behind) with the saved state. Bounded (e.g. single worker or small pool) to avoid unbounded goroutines.
- **Restore (reconstruction)**: Use existing `ReconstructStateAtTimestamp` and change journal listing; no new storage format.

### 5. Performance and redundancy

| Path              | When              | Mechanism                    | Performance | Redundancy   |
|-------------------|-------------------|------------------------------|------------|--------------|
| **Quick rollback**| Within threshold  | Apply snapshot states        | Fast       | Snapshot only|
| **Reconstruction**| Beyond threshold  | Change journal per object    | Slower     | Audit chain  |

- **Performance**: Snapshot path avoids scanning and reversing the change journal; apply is a small, fixed set of updates.
- **Redundancy**: If snapshots are pruned or missing (e.g. after T time or N transitions), the audit chain (change journal with `previous_state` and `change_type`) still allows reconstruction per object, as already implemented.

## Implementation outline

1. **Define status-relevant graph per rule/kind**  
   For each transition rule (or kind) that can be rolled back, define how to enumerate “linked/connected items that have bearing on status” (e.g. for priority_plan: plan + backlog_item with `priority_plan_ref`).

2. **Rollback point storage**  
   Add a rollback WAL or `rollback_point` store; append one entry per transition (before apply) with payload = list of minimal object states (id, kind, status, …) for the status-relevant graph.

3. **Capture on transition**  
   In the transition updater (or a hook), before applying the transition: compute the status-relevant graph, read current state for each object, write rollback point. Enforce retention (e.g. trim by count or age) so storage stays bounded.

4. **Rollback command / API**  
   - **Input**: Rollback “to” a given transition id or timestamp.
   - **Logic**: If a rollback point exists for that transition (within threshold), **quick rollback**: load snapshot and reapply states. Otherwise, **reconstruction**: for each object in the (recomputed) graph, call `ReconstructStateAtTimestamp` at that time, then apply.

5. **Configuration**  
   - Threshold: e.g. `rollback.retain_count` (last N points), `rollback.retain_duration` (e.g. 24h).  
   - Optional: enable/disable snapshot capture per rule or globally.

## Rollback API (reusable for maintenance)

The same snapshot/rollback infrastructure in `pkg/rollback` is intended for **lifecycle** and for **other internal/maintenance tasks** (migrations, bulk fixes, etc.). Callers can capture a point before making changes and later list or apply it.

### Functions

| Function | Purpose |
|----------|--------|
| **Capture**(projectRoot, scopeType, scopeID, getStates) | If capture is enabled (config), calls getStates(), builds a rollback point with ID like `rb-20060102-150405.000000000`, appends to `.zqk/wal/rollback_points`. Returns point ID or empty if disabled/error. Caller should call **Retain** periodically or after capture. |
| **List**(projectRoot, lastN, withinDuration) | Returns metadata for rollback points (most recent last). Optional limits: last N points and/or within a duration. |
| **Get**(projectRoot, pointID) | Returns the full rollback point (including ObjectStates) by ID. |
| **Apply**(ctx, projectRoot, pointID, provider) | Reapplies all object states from the point to storage via provider.Update (quick rollback path). |
| **ApplyReconstruct**(ctx, projectRoot, targetTimestamp, refs, provider, logger) | Beyond-threshold path: for each ref, reconstruct state at targetTimestamp via `storage.ReconstructStateAtTimestamp` and apply. Refs typically from `lifecycle.RecomputeRefsFromScope(scopeType, scopeID, provider)`. |
| **Retain**(projectRoot) | Trims the rollback store to the configured keep count and duration (DefaultConfig). |

### Scope types

- **ScopeTypeLifecycle** — lifecycle-driven transitions (e.g. priority_plan complete); status-relevant graph is resolved and captured before apply.
- **ScopeTypeMaintenance** — internal/maintenance tasks (e.g. bulk field fix, migration step). Caller supplies the set of (kind, id) to snapshot.
- **ScopeTypeMigration** — migration or one-off scripts; same usage as maintenance.

### Usage for maintenance tasks

1. **Before** making changes: build the list of object refs (kind + id) that will be modified.
2. **Snapshot**: `states, err := rollback.SnapshotObjectStates(ctx, provider, refs)` then `rollback.Capture(projectRoot, rollback.ScopeTypeMaintenance, taskID, func() ([]rollback.ObjectState, error) { return states, nil })`.
3. **Apply changes** via storage as usual.
4. **Optional**: Call `rollback.Retain(projectRoot)` after capture or on a schedule.
5. **Rollback if needed**: `rollback.Apply(ctx, projectRoot, pointID, provider)` (use a point ID from List).

Configuration (retain count/duration, enable/disable capture) is in `pkg/rollback/config.go` and overridable via env: `ZQK_ROLLBACK_RETAIN_COUNT`, `ZQK_ROLLBACK_RETAIN_DURATION`, `ZQK_ROLLBACK_CAPTURE_DISABLED`.

## References

- **Change journal reconstruction**: `pkg/storage/change_journal_reconstruction.go` — `ReconstructStateAtTimestamp` walks change journal entries and reverses updates/deletes using `previous_state`.
- **Change journal entries**: Each update/delete records `object_ref`, `change_type`, `previous_state`, `created_at`; create journal entries support full audit trail.
- **Timestamp snapshot design**: `docs/architecture/README.md` — snapshot timestamp plus reconstruction when object was modified after snapshot.
- **Lifecycle listener**: `docs/architecture/LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md` — transition rules and updater; natural place to hook "capture rollback point before apply".
- **Rollback package**: `pkg/rollback/` — types, store, Capture/List/Get/Apply/ApplyReconstruct/Retain, SnapshotObjectStates. **Lifecycle**: `lifecycle.RecomputeRefsFromScope`; **CLI**: `cmd/zqk/rollback/` (list, apply, reconstruct, retain). `docs/architecture/LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md` — transition rules and updater; natural place to hook “capture rollback point before apply”.
