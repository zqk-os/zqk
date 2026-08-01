# Backlog items: Cache-first system check architecture

**Purpose:** Work required to bring the desired architecture in `system-check-cache-first-and-async.md` into reality. Aligns with `CLI_PERFORMANCE_AND_CONSISTENCY.md` (1 s response, incremental caches, async for long work). Create these as backlog_item objects via the zqk CLI (e.g. `zqk object create backlog_item --file <yaml>`). Use valid `priority_plan_ref` and `milestone_refs` (or `status: exploring`) per project policy.

---

## 1. System check cache-only — show validation results only

**Title:** System check cache-only - show validation results only

**Description:**
Refactor `zqk system check` so it does not run validation in the CLI process.
- **Main thread:** Load validation cache (and object ID cache for scope). Show cached validation results or a not-ready message (e.g. "outstanding validations: N").
- **Background:** Trigger scan worker (async) to update object counts and enqueue validations.
CLI returns quickly; no blocking on validation. See `docs/architecture/system-check-cache-first-and-async.md`.

**Category:** System

---

## 2. Create/update/delete — async cache updates (three threads)

**Title:** Object create/update/delete - async ID, list, and validation cache updates

**Description:**
On object create, update, or delete, run three async paths and return immediately:
- **Thread 1:** Update/invalidate **object ID cache** — async, with callback when store completed.
- **Thread 2:** Update/invalidate **list cache** — async, with callback when store completed.
- **Thread 3:** **Enqueue validation** — validation queue worker farms out to workers that update the validation cache.
CLI returns after initiating these; no blocking on cache writes or validation. See Flow by thread in `system-check-cache-first-and-async.md`.

**Category:** System

---

## 3. Pre-warm populates all three caches (async)

**Title:** Pre-warm directory scan - populate ID, list, and validation caches async

**Description:**
When pre-warm performs a directory scan (e.g. file backend), it should trigger async population of:
- Object ID cache
- List cache
- Validation cache (by enqueuing validation work; workers write results to validation cache)
All of this must be async; nothing blocks the CLI or scheduler beyond submitting work. Extend scheduler `cache_prewarm` or file-backend pre-warm path. See Pre-warm section in `system-check-cache-first-and-async.md`.

**Category:** System

---

## 4. Object count quick path with mtime diff

**Title:** Object count quick path - mtime diff and async cache/validation updates

**Description:**
- **Quick path:** Object count returns quickly (from cache when possible).
- **When mtime (or equivalent) indicates change:** Diff object cache vs directory scan. From the diff, trigger **async** cache updates and validations only for new/changed objects.
- Expose "objects not yet in cache" and "outstanding validations" so the CLI can inform the user. Count itself returns quickly; heavy work is async with callback. See Object count in `system-check-cache-first-and-async.md`.

**Category:** System

---

## 5. Validation queue worker and scan worker

**Title:** Validation queue worker and scan worker for async validation

**Description:**
- **Validation queue worker:** Accepts enqueued validation tasks (from create/update/delete and from pre-warm/scan). Farms out work to workers that run validation and update the validation cache. Must not block the CLI.
- **Scan worker:** Triggered by system check (async) to update object counts and enqueue validations for objects not yet in cache. Ensures object count and validation cache stay in sync with filesystem without blocking the main thread.
See Flow by thread (system check: thread-1 = scan worker) and Validation in `system-check-cache-first-and-async.md`.

**Category:** System

---

## Creating the items via CLI

Use valid refs for your project (e.g. `priority_plan_ref: PRI-219`, `milestone_refs: [MIL-035]`) or `status: exploring` if policy allows. Example:

```bash
# Example: create from inline data (adjust status/refs per policy)
zqk object create backlog_item --data 'kind: backlog_item
title: System check cache-only - show validation results only
status: exploring
category: System
description: Refactor zqk system check to only read validation cache...
schema_version: "2.0.0"'
```

Or create a YAML file per item and run `zqk object create backlog_item --file <file>`.
