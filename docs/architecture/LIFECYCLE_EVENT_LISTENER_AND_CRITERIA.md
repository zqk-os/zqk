# Lifecycle Event Listener and Transition Criteria

**Status:** Design + implementation  
**Related:** REQ-014, priority plan completion, `executeLifecycleHook`, object WAL, unbounded-concurrency-fixes

## Goal

Lifecycle transitions (e.g. priority_plan active→complete) must be gated on **events or triggers** that must be confirmed/complete before the transition can proceed. A **lifecycle event listener** processes a WAL of lifecycle events, accumulates satisfied criteria, and when **all criteria** for a transition are met, a **transition updater** triggers the transition (cache first, then disk in background). The system adheres to concurrency guidelines, architecture, and observability best practices.

## Concepts

### 1. Lifecycle transition criteria

- **Criteria** are conditions that must be "satisfied" (confirmed) before an **auto** transition is allowed.
- Criteria can be **event-based**: e.g. "all backlog items for plan X are complete" is represented as a single criterion `all_backlog_items_complete_for_plan` with scope `plan_id=X`.
- Lifecycle specs (YAML) already have `preconditions` (human-readable strings). Optionally, transitions can define **required_criteria** (machine-readable IDs) that the listener uses. If not yet in the spec schema, rules can be configured in code or a small config file (e.g. "when criterion C with scope S is satisfied, trigger transition kind K, id I, to_status T").

### 2. Lifecycle events

- **StatusTransition** – recorded when an object’s status changes (kind, id, from_status, to_status, optional scope). Used for auditing and for deriving higher-level criteria.
- **CriterionSatisfied** – recorded when a discrete criterion is met (criterion_id, scope key-value). The listener uses these to decide when to fire a transition.

Events are appended to a **lifecycle event WAL** (`.zqk/wal/lifecycle_events.wal`) so they are durable and replayable.

### 3. Lifecycle event listener

- **Single reader** per project root (or one global reader that demuxes by project root) to preserve ordering and avoid unbounded goroutines.
- Reads the lifecycle WAL from the last checkpoint (or replays from start). For each record:
  - **StatusTransition**: can be stored for audit or used to derive criteria (e.g. "last backlog_item for plan X completed" → emit or infer CriterionSatisfied).
  - **CriterionSatisfied**: add `(criterion_id, scope_key)` to an in-memory **satisfied set**.
- After processing (per record or per batch), evaluate **transition rules**: for each rule "when criterion C and scope S are satisfied → transition (kind, id, to_status)", if `(C, S)` is in the satisfied set and this rule has not yet fired, **enqueue the transition** to the updater and mark the rule as fired (so we don’t double-fire).
- **Bounded concurrency**: listener runs in one goroutine; transition work is sent to a **channel** consumed by a small, fixed worker pool (e.g. 1–4 workers).

### 4. Transition updater

- Consumes transition requests from the listener (kind, id, to_status; from_status can be inferred or passed).
- **Cache-then-disk**:  
  1. **Cache first**: So that the next Read/List sees the new status immediately. When write-behind is enabled, this is achieved by enqueueing the update to the **object write buffer** + object WAL (same path as normal updates). The buffer is the "cache" that Read merges with disk.  
  2. **Disk in background**: The existing write-behind worker drains the buffer and persists to disk. So the updater calls an API that enqueues the update (e.g. `EnqueueStatusUpdate` on `FileObjectStorage`) rather than calling synchronous `Update()` when write-behind is available.
- **Fallback**: If write-behind is not available (e.g. graph backend or write-behind disabled), the updater calls `storage.Update()` (sync). No unbounded goroutines: transitions are processed by the bounded pool.

### 5. Parallel / sequential / categorical / prioritized handling

- The listener can be **unified** (one WAL, one reader) or **bifurcated**:
  - **Parallel**: Multiple WALs or partitions (e.g. by kind or category), each with its own reader goroutine (bounded total).
  - **Sequential**: One WAL, one reader (current minimal design).
  - **Categorical**: Route events by category into different queues; each queue has a bounded worker.
  - **Prioritized**: Transition queue is a priority queue (e.g. by plan tier or deadline); workers pull highest priority first.
- Initial implementation uses **one WAL, one reader, one transition channel, one updater worker** to satisfy concurrency guidelines. Extension points are documented so parallel/categorical/prioritized handling can be added later.

## Concurrency and best practices

- **No unbounded goroutines**: Listener = 1 goroutine per project root (or 1 global). Updater = fixed N workers (e.g. 1) pulling from a channel. See [unbounded-concurrency-fixes.md](./unbounded-concurrency-fixes.md).
- **Hot path**: Appending a lifecycle event must be fast (append to WAL + optional sync). No heavy work on the status-change path; listener runs asynchronously.
- **Cache**: No full cache clear; transition only updates the single object (buffer or direct update).
- **Observability**: Logging (e.g. `logging.NewEventLogger(ctx)` / `LogInfo`/`LogDebug`) for criteria satisfied, rules fired, and transitions applied. Optional metrics: lifecycle_events_appended_total, lifecycle_transitions_triggered_total, lifecycle_listener_lag_seconds.

## Rollback (snapshot + audit chain)

Rollback of lifecycle-driven transitions is designed to use:

- **Within a configurable threshold**: A **snapshot of data states for all linked/connected items that have bearing on status** (e.g. plan + all its backlog items’ statuses), stored using the same WAL/infrastructure, enabling **quick rollback** by reapplying those states.
- **Beyond threshold**: **Reconstruction via the audit event chain** (change journal and `ReconstructStateAtTimestamp`) so rollback remains possible when snapshots are pruned.

See [LIFECYCLE_ROLLBACK_SNAPSHOT_AND_AUDIT_CHAIN.md](./LIFECYCLE_ROLLBACK_SNAPSHOT_AND_AUDIT_CHAIN.md) for the full design (performance + redundancy).

---

## Implementation layout

- **`pkg/lifecycle/`**
  - `event_wal.go` – LifecycleEvent, LifecycleEventWAL (append, replay from checkpoint).
  - `listener.go` – Listener that reads WAL, accumulates satisfied criteria, evaluates rules, sends transitions to channel.
  - `rules.go` – Transition rules (in-code or loaded from config): criterion_id + scope → (kind, id, to_status).
  - `updater.go` – Updater that consumes transition channel; calls storage (EnqueueStatusUpdate when available, else Update).
- **Wire**: When storage reports a status change, append a `StatusTransition` event to the lifecycle WAL (if listener is enabled). This happens both on synchronous updates (CLI/API) and when the **write-behind worker** applies updates from the object WAL (`applyUpdateFromBuffer`), so status transitions from the daemon are also recorded. Listener is started when storage/scheduler is ready (e.g. same process that holds storage); it opens the WAL and runs the read loop.

## Cache-then-disk detail

- **With write-behind**: `FileObjectStorage` exposes `EnqueueStatusUpdate(ctx, secCtx, id, updates map[string]any)`: read current object (with buffer merge), apply updates, marshal to YAML, `AppendToWALAndBuffer("update", kind, id, data, true)`, `Notify()` worker. Result: update is visible in buffer immediately (Read merges buffer); worker persists to disk in background.
- **Without write-behind**: Updater calls `storage.Update(ctx, secCtx, id, map[string]any{"status": to_status})` so transition is applied synchronously. No separate "cache" layer; consistency is still maintained.

## Lifecycle spec extension (optional)

- Transition definition can add optional `required_criteria: ["criterion_id_1", "criterion_id_2"]` and optional `scope_binding` (e.g. `plan_id: object_id`) so the listener can derive which object(s) to transition from the scope of the satisfied criteria. If not in the schema yet, rules are maintained in code or a small config file.
