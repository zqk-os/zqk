# Scheduler diagnostics findings and fixes

**Last Verified:** 2026-08-31


**Source:** Review of `.zqk/scheduler/diagnostics/` dumps (goroutines, processes, threads, heap).  
**Reference:** Conversation that identified HashRegistry.Save blocking, HighVolumeEventCache.BuildCache blocking 78+ min, high memory, and goroutine creation.

---

## 1. HashRegistry.Save blocking (fixed)

**Problem:** Multiple goroutines blocked in `(*HashRegistry).Save` at `hash_registry.go:572` for 4–6+ minutes; `startSaveWorker` goroutines in chan receive 64–78 min. One worker per registry serializes saves; for audit_event (250k+ hashes) `processSave` does json.Marshal + write + Sync + rename + Chtimes + dir Sync and takes minutes, so callers hit the 5‑minute timeout.

**Fixes applied:**
- **Shorter timeout for large registries:** When `len(dataCopy) > 50000`, `Save()` uses `hashRegistryLargeSaveMaxWait` (90s) instead of 5 min so callers fail fast and can retry or WAL replay.
- **processSave optimization:** For `len(hashes) > hashRegistryLargeThreshold` (50k), skip Chtimes and the extra parent-dir Sync to reduce I/O and time in `processSave`.

**Constants:** `hashRegistryLargeThreshold`, `hashRegistryLargeSaveMaxWait` in `pkg/storage/hash_registry.go`.

---

## 2. HighVolumeEventCache.BuildCache blocking (fixed)

**Problem:** `BuildCache` blocked 73–78 minutes; aggregation handler calls `EnsureHighVolumeEventCacheReady` with a 10s timeout but `BuildCache` waited on `<-done` with no `ctx.Done()` check, so the timeout never took effect. With 250k+ audit_event IDs, `buildCacheFromIndex` does 250k CAS reads (64 workers) and can run for hours.

**Fixes applied:**
- **Respect ctx in BuildCache:** Wait with `select { case err := <-done; case <-ctx.Done(): return ctx.Err() }` so the handler’s 10s timeout actually stops the wait; aggregation then falls back to storage queries.
- **Cap work when deadline is near:** In `buildCacheFromIndex`, if ctx has a deadline &lt; 2 min and `len(ids) > 100000`, cap at 100k IDs so a partial cache can be built within the timeout.

---

## 3. Memory (heap profile)

**Profile:** `scheduler_daemon_20260301-235819_heap.prof` (alloc_space).

**Top allocators (summary):**
- `os.(*File).readdir` — ~33 GB (readdir buffers)
- `(*indexQueue).processBatch.func3` — ~26 GB
- `encoding/json.MarshalIndent` — ~17 GB (prefer `Marshal` for large blobs)
- `(*AsyncCacheValidationStrategy).scanDirectory` / `ValidateMappings` — ~15+13 GB
- `bytes.growSlice`, `yaml_emitter_*`, `os.Stat`, `encoding/json.Marshal` — significant

**Recommendations:**
- Prefer `json.Marshal` over `MarshalIndent` for large caches/serialization.
- Review `indexQueue.processBatch` and async cache validation for allocation hot spots and streaming or chunking.
- For ongoing profiling: `go tool pprof -http=:8080 <heap.prof>`.

---

## 4. Goroutine creation

**Observation:** Goroutine IDs in dumps reached 2.2M+; only ~121–140 were live at capture. So the process has created millions of short-lived goroutines over its lifetime.

**Code review:** Scheduler and storage use bounded concurrency:
- `goroutinelabels.Pool` for job execution and worker pools.
- `buildCacheFromIndex` uses a fixed 64 workers (StartSimple), not one per ID.
- HashRegistry uses one worker per registry and a buffered queue.

**Conclusion:** No unbounded “one goroutine per event” in the paths reviewed. High ID count is likely from many short-lived job/operation goroutines over long daemon uptime. If goroutine count grows without bound in future, add metrics and inspect job execution and storage paths for new goroutines per item.

---

## 5. Dump events in scheduler-events.json (2026-03-02)

**Observation:** `scheduler_command` `operation=dump` events were logged without a `status` field, so it was not possible to see `status=started` vs `status=complete` vs `status=error` in the events file. The daemon does emit started (on SIGUSR1), then complete or error when capture finishes.

**Change:** `emitSchedulerDumpEventViaCoordinator` now adds `status` to the event fields so each line in scheduler-events.json includes `"status":"started"`, `"status":"complete"`, or `"status":"error"`. You can grep for `operation=dump` and then check the same or later lines for `status=complete` or `status=error` (capture can take a minute with many goroutines).

---

## 6. Recent dumps review (2026-03-02 01:24:38)

**Files:** `scheduler_daemon_20260302-012437_threads.txt` (130 goroutines), `scheduler_start_20260302-012437_threads.txt` (125), `cli_20260302-012437_threads.txt` (126). All three are from the same scheduler process; the dump at 01:24:38 is the most recent (`scheduler_dump_1772443478377820000`). Only one event appears in scheduler-events.json for that dump (no `status` key), so we cannot tell from the file whether capture completed; after the code change above, future dumps will show `status=complete` or `status=error` when capture finishes.

**Findings from stack dumps:**

- **HashRegistry.Save** — Several goroutines blocked in `(*HashRegistry).Save` at `hash_registry.go:572` (from `saveHashRegistry` → `writeObjectToCAS` or `Update`). Save remains a bottleneck under load.
- **HashRegistry.startSaveWorker** — Multiple goroutines in `chan receive` for **150–164 minutes** at `hash_registry.go:273`. Workers are waiting on the save queue; serialization of large registries (e.g. audit_event) still causes long waits.
- **HighVolumeEventCache.BuildCache / buildCacheFromIndex** — Goroutines in `chan receive` for **159–164 minutes** at `high_volume_event_cache.go:569` and `.731`. BuildCache (and its worker `buildCacheFromIndex`) are still blocking for hours when building from 250k+ IDs; the ctx.Done() and 100k cap fixes should reduce this once deployed and after the next aggregation cycle.
- **CAS reconciliation worker** — One goroutine blocked ~4 minutes at `object_storage_file_discovery.go:40`.
- **Validation cache flusher** — One goroutine blocked ~3 minutes at `state_cache_shared.go:93`.
- **Goroutine IDs** — High IDs (e.g. 2.3M+, 2.1M+, 1.9M+) indicate many short-lived goroutines over daemon uptime; live count (~125–130) is bounded. No unbounded leak observed in these dumps.

---

## How to capture diagnostics again

- Scheduler daemon writes dumps on a schedule or signal (see `pkg/diagnostics` and `cmd/zqk/scheduler`).
- Heap: `go tool pprof -http=:8080 .zqk/scheduler/diagnostics/scheduler_daemon_<timestamp>_heap.prof`
- Goroutines: same with `_goroutines.pb.gz` or the `.txt` stack dump.
- In scheduler-events.json: `operation=dump` with `status=started` (immediate); after capture, look for `status=complete` or `status=error`.
