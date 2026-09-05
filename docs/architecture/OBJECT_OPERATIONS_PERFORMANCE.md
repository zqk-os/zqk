# Object operations performance (create / update / delete)

**Last Verified:** 2026-08-31


**Purpose:** Get single-object create, update, and delete into the **millisecond** range—comparable to traditional RDBMS or NoSQL single-row/document ops—so CLI and automation stay responsive. Command metrics and timeouts should reflect fast runs, not multi-minute baselines.

## Target (where we need to get)

- **Single object create/read/update/delete:** **milliseconds** under normal (warm) conditions—e.g. **&lt; 100 ms** per op, with **&lt; 50 ms** as a stretch goal. This aligns with typical RDBMS (single-digit to low double-digit ms) and NoSQL single-doc latency.
- **Batch (e.g. 100 objects):** **&lt; 30 s** total so bulk operations don’t block the CLI.
- **Bulk 5k:** **~1 s** for a batch of 5 000 objects (create or update). This is the target for **BenchmarkCRUDBaselineBulk5kCreate** / **BenchmarkCRUDBaselineBulk5kCreateOnly**.
- **Command timeout:** object commands should not need &gt; 30 s; if baseline grows (e.g. from slow runs), timeout grows and can hit the global cap.

**Note:** Current tests use a **3 s** per-op ceiling as a **regression guardrail** only (CI stability). That is not the performance target; the target is millisecond-level. Once the hot path is optimized, test limits should be tightened to assert the real target.

## Current baseline (actuals)

From **TestCRUDBaselineConsistency** output and `test-scenarios/crud-baseline/baselines/` (one run, file storage, isolated temp env, multiple kinds):

| Operation | Average (ms) | Target (ms) | Gap |
|-----------|-------------|-------------|-----|
| **Create** | ~1 800 | &lt; 100 | ~18× over |
| **Read**   | ~14  | &lt; 100 | ✓ in range |
| **Update** | ~780 | &lt; 100 | ~8× over |
| **Delete** | ~1 340 | &lt; 100 | ~13× over |

- **Read** is already in the target range.
- **Create, Update, Delete** are in the **hundreds of ms to low seconds**; main improvement work is on the write/delete path (validation, cache updates, CAS index, audit).

CLI command metrics (`.zqk/metrics/command_metrics.json`) show similar order of magnitude: e.g. `zqk object create backlog_item --file` baseline ~2.0–3.0 s (nanoseconds in JSON), fastest ~0.6 s, slowest up to ~13 s. Re-run the CRUD baseline test to refresh: `go test -v -count=1 ./pkg/zqkcli/ -run TestCRUDBaselineConsistency`.

## Root causes of slow runs (addressed or to avoid)

1. **Spec cache clear on every create (fixed)**  
   `cmd/zqk/object/create.go` previously called `SpecLoader.ClearCache()` and `InvalidateSpec(...)` on **every** create. That violated PRE_CHANGE_CHECKLIST §2–3 and CLI_PERFORMANCE_AND_CONSISTENCY: no full cache clear on hot path. It forced full spec reload on the next use and contributed to tens-of-second response times. **Fix:** Removed the per-create clear; spec freshness is handled by incremental invalidation or explicit refresh when config/specs change.

2. **Storage constructor cache clear**  
   `NewFileObjectStorage` clears its **instance** spec loader cache once when the storage is constructed. That runs once per process (or per factory instance), not per create, so it is not the main cause of per-request slowness. Avoid adding per-request work there.

3. **Validation and cache handler**  
   Each create/update runs validation and, when context has cache update, the cache operation handler (`UpdateObjectIDCache`, `InvalidateListCacheForKind`, `InvalidateValidationCacheForCacheContext`). These must stay **incremental** and **fast** (no full scans, no LoadFields on hot path). If command metrics show object create/update/delete still slow after the create.go fix, profile validation and cache handler next.

4. **Command metrics and timeout**  
   Timeout is derived from baseline duration (e.g. baseline × 3). If past runs were slow, baseline and timeout grow; once timeout hits the global max, commands can time out. Making object ops fast keeps baselines low and avoids timeout creep.

## Profiling and optimization

- **CRUD test with CPU/heap profile:** Set **ZQK_CRUD_PROFILE=1** (and optionally **ZQK_CRUD_HEAP_PROFILE=1**) when running **TestCRUDBaselineConsistency**. Profiles are written to **test-scenarios/crud-baseline/baselines/** (`crud_cpu_<timestamp>.pprof`, `crud_mem_<timestamp>.pprof`). Use `go tool pprof -http=:8080 <file>` to inspect and find hot spots (validation, cache updates, CAS index, audit).
- **Bulk benchmark profiling:** Run **BenchmarkCRUDBaselineBulk5kCreateOnly** with **-cpuprofile=bulk5k_create.pprof -benchtime=1x**. Default bulk size is **100** until the hot path is optimized; set **ZQK_BULK_BENCH_SIZE=5000** for a full 5k run.
- See **test-scenarios/crud-baseline/README.md** for full profiling and bulk benchmark commands.

## Profile-based recommendations (CRUD)

CPU profiles from **ZQK_CRUD_PROFILE=1** runs (TestCRUDBaselineConsistency) show the following hot paths and suggested improvements. Profile duration ~23 s, sampled CPU ~1.6 s (much of the rest is I/O wait).

### 1. Spec loading (~26% of sampled CPU)

- **Hot:** `SpecLoader.LoadSpecWithInheritance`, `loadSpecWithInheritanceRecursive`, `os.ReadFile` / `os.Open`.
- **Cause:** Specs are loaded from disk (and inheritance chain) when resolving kind→directory, bucket strategy, or schema version. Multiple call sites (bucket strategy loader, list filtering, validation) can trigger loads.
- **Recommendations:**
  - **Cache spec by kind:** Cache the result of `LoadSpecWithInheritance(kind+".yaml")` per kind (and project root / spec dir) so the second request for the same kind does not read disk again. Invalidate on spec file change if needed.
  - **Pre-warm at storage init:** When `NewFileObjectStorage` (or first Create) runs, pre-load specs for the kinds that will be used (e.g. from config or a small “common kinds” set) so the first Create does not pay full spec load cost.
  - **Avoid spec load in hot path:** Ensure `getSchemaVersionForKind` and any “get strategy/spec for kind” path use a cache; do not call `LoadSpecWithInheritance` on every Create.

### 2. Field registry and normalization (~14% of sampled CPU)

- **Hot:** `FieldRegistry.GetFieldsForKind`, `FieldRegistry.LoadFields`, `normalizeObjectValues`.
- **Cause:** First `GetFieldsForKind(kind)` triggers `LoadFields()` (loads **all** specs and builds the full field cache). Used by `normalizeObjectValues` on every Create/Update.
- **Recommendations:**
  - **Pre-warm field registry:** Call `FieldRegistry.LoadFields()` once at storage init (or scheduler/CLI init), not on first Create. Ensures the first Create does not pay full LoadFields cost.
  - **Per-kind cache:** `GetFieldsForKind` already uses a cache after LoadFields; ensure no code path clears it on create/update (PRE_CHANGE_CHECKLIST: no LoadFields on hot path).
  - **Lazy normalization:** If normalization is only needed for a subset of kinds or for persistence, consider doing it only when required or batching for bulk writes.

### 3. Kind mapper and bucket strategy (~12% of sampled CPU)

- **Hot:** `DynamicKindMapper.Initialize`, `discoverKindForDirectory`, `DefaultBucketStrategyRegistry.GetStrategyForKind`, `getBaseKind`, `getDefaultStrategyForKind`.
- **Cause:** First `GetStrategyForKind(ctx, kind)` (e.g. during Create) can call `kindMapper.Initialize()`, which discovers kinds by scanning directories/specs. `getDefaultStrategyForKind` then caches per kind.
- **Recommendations:**
  - **Initialize once:** Run `kindMapper.Initialize()` once at storage or app startup (or when the bucket strategy registry is built), not inside `getBaseKind` on first use. Lazy init is fine, but ensure it runs only once per process (or per project root).
  - **Cache strategy and base kind:** Ensure `defaultsCache` and any “base kind” result are cached so repeated `GetStrategyForKind(kind)` for the same kind are cheap.
  - **Avoid directory scan on every new kind:** If discovery is expensive, consider building the kind→directory map from spec filenames or config instead of scanning the filesystem on first use.

### 4. File I/O and write path (~17% write, ~10% stat)

- **Hot:** `FileObjectStorage.writeObjectToCAS` / `writeObjectToStorage`, `os.OpenFile`, `os.Stat`, `syscall.Open` / `Stat`.
- **Cause:** Each Create/Update writes a file; CAS and directory layout may trigger multiple opens or stats.
- **Recommendations:**
  - **Batch or buffer writes:** For bulk create, consider batching small writes or buffering in memory and flushing once per batch (with care for durability and crash consistency).
  - **Reduce stat/open calls:** Reuse directory existence checks or in-memory state where safe; avoid repeated `os.Stat` for the same path in a single request.
  - **CAS index:** Ensure CAS index updates are incremental and do not rescan the whole kind directory on each write.

### 5. Audit and async goroutines (~12% goroutine start, ~7% audit build)

- **Hot:** `GoroutineBuilder.StartSimple`, `CreateAuditEventWithBuilder`.
- **Cause:** Each Create (and possibly Update/Delete) may start async work (e.g. CAS index queue, audit router) and build an audit event.
- **Recommendations:**
  - **Batch audit events:** For bulk create/update, consider batching audit events (e.g. one “bulk create” event or a buffer that flushes every N ops) instead of one event per object.
  - **Defer or pool goroutines:** Avoid starting a new goroutine per object; use a shared worker pool or channel so many creates reuse the same workers.
  - **Lighter audit payload:** Build minimal audit payloads on the hot path; defer heavy serialization or extra metadata to a background step.

### 6. YAML and GC (~9% YAML parse, ~19% GC)

- **Hot:** `gopkg.in/yaml.v3` (unmarshal, parser, fetch tokens), `runtime.gcDrain`, `runtime.mallocgc`.
- **Cause:** Spec and object YAML parsing/allocation increase GC pressure and CPU.
- **Recommendations:**
  - **Cache parsed specs:** Store parsed spec structs (or at least the inheritance-resolved result) in the spec cache so we do not re-parse the same YAML on every request.
  - **Reduce allocations on create:** Reuse buffers or structs where possible in the create/update path; avoid large temporary maps or strings that trigger GC.
  - **Faster YAML (optional):** If spec parsing remains hot after caching, consider a faster YAML library or a small binary/cached representation for the hot path.

### Priority order for implementation

1. **Pre-warm and cache:** Field registry LoadFields and kind mapper Initialize at startup; cache spec per kind and bucket strategy per kind. Highest impact for “first touch” and repeated ops.
2. **Avoid per-op spec load:** Ensure no Create/Update calls `LoadSpecWithInheritance` or full LoadFields on the hot path; use only cached data.
3. **Audit and async:** Batch or defer audit; use a fixed worker pool instead of one goroutine per op.
4. **I/O and CAS:** Reduce redundant stat/open; keep CAS index updates incremental.
5. **Allocation and YAML:** Cache parsed specs; reduce allocations in the write path.

Re-profile after each change with `ZQK_CRUD_PROFILE=1` and compare baseline JSON and pprof top/cum to confirm improvements.

### Why create and delete are still slow (profile findings)

- **CAS index flush:** Every CAS Create calls `FlushKind(kind, 5s)` so that List sees the new object. FlushKind waits for the index queue to drain and for the worker to persist the batch. Fixed sleeps were reduced (no 50ms when queue exists; shorter post-empty stabilization); remaining cost is waiting for queue empty + batch timeout.
- **Delete (CAS):** `ContentAddressableStorage.Delete` enqueues an index remove, waits on the done channel (up to 5s), then calls `FlushKind` again. So each delete pays queue wait + flush.
- **Disk I/O:** Profile shows ~60%+ in `syscall.syscall`; `os.OpenFile`, `os.Stat`, `ReadFile`, and CAS `writeFileWithSync` dominate. Actual file write and sync are unavoidable for durability.
- **Audit and async:** `CreateAuditEventWithBuilder` and `GoroutineBuilder.StartSimple` (index queue worker, hash registry worker) show up in CPU; goroutine start and channel ops add some cost even when work is async.
- **Hash registry:** `HashRegistry.processSave` and `IDIndex.saveMappingsNoLock` run in workers; Create/Delete block only on FlushKind (index), not on hash registry, but the workers still do I/O that can contend.

Further gains would require: making FlushKind optional or deferred for a short window (weaker consistency), batching audit events, or reducing sync/stat in the write path where safe.

### Options to reach target performance (&lt; 100 ms create/delete)

| Option | Idea | Guarantee | Tradeoff / effort |
|--------|------|-----------|-------------------|
| **A. Defer FlushKind** | Stop blocking Create/Delete on `FlushKind`. Enqueue index update and return; same process already sees in-memory index. | Same-process List/Read see writes immediately; disk index and cross-process visibility are eventually consistent (worker persists in background). | Easiest: one or two call-site changes. Risk: new process or List that reads from disk before worker runs might miss the object until next flush. Mitigation: flush on timer or on next List for that kind. |
| **B. Layered cache + background I/O** | Accept write into an in-memory layer (and optionally an append-only WAL); return immediately. Background worker(s) persist to CAS files, index, hash registry, audit. | **Write eventually happens**: durability after a bounded delay (e.g. within N ms or on process exit / explicit flush). | Same-process Read/List merge in-memory layer with on-disk; cross-process/restart see state after persist. Requires: write buffer or WAL, merge logic for Read/List, shutdown/graceful drain so no writes are lost. More invasive but gives full control. |
| **C. Sync file, async everything else** | Keep syncing the object file to disk in Create (so content is durable quickly), but do not wait for index flush or audit. In-memory index already updated; worker persists index/audit in background. | Content durable on return (or after one sync); index/audit eventually consistent. | Middle ground: we already have async index queue; we only remove the `FlushKind` wait (same as A). Optionally make the file write itself async (buffer + worker sync) to get Create under 100 ms even on slow disk. |
| **D. Batch writes** | Buffer multiple creates in memory; flush in batches (by count or time). Reduces number of syncs and index flushes per object. | Durability at batch boundaries. | Good for bulk; single-object latency still one batch interval unless we add immediate path for single op. |

**Recommendation:** To get to &lt; 100 ms without a large redesign, **Option A (defer FlushKind)** is the smallest change: remove or make optional the `FlushKind` call after Create (and for Delete, either stop waiting on the index-remove done channel or make FlushKind non-blocking). Same-process semantics stay correct (in-memory index is already updated). For **Option B (layered cache + background I/O)**, the design would be:

- **Write path:** Create/Update/Delete enqueue to a **write buffer** (per kind or global). Optionally append to a **WAL** (e.g. one file per kind or single log) so crash recovery can replay. Return success to caller once the op is in the buffer (and optionally after WAL append + sync if we want “committed” guarantee).
- **Background worker:** Drains buffer; for each op: write CAS file (or update/delete), update index, hash registry, audit. One worker or pool; ordering per kind if needed.
- **Read/List:** Merge in-memory “pending” state with on-disk state (e.g. Read checks buffer for id, then disk; List merges buffer IDs with index then reads).
- **Guarantee:** “Write eventually happens” = every buffered op is persisted before process exit (drain on shutdown) and optionally within N ms in steady state. Crash: if we have WAL, replay on startup; else we lose ops after last flush (acceptable only if we document it).

### Option B with WAL: design (implemented)

This section defines the concrete design for Option B so the system operates like a performant data store: writes are accepted into a layer and persisted in the background with crash recovery via WAL.

- **WAL (write-ahead log)**
  - **Location:** Under project root, e.g. `.zqk/wal/object.wal` (single log file per project root; compaction removes applied entries).
  - **Record format:** Length-prefixed or JSONL. Each record: `op` (create | update | delete), `kind`, `id`, optional `data` (base64 or raw bytes for create/update). Optional `seq` for ordering. Append-only; sync after append (or batch sync) for durability.
  - **Checkpoint:** `.zqk/wal/object.wal.checkpoint` stores last applied seq so replay only reads unapplied tail.

- **In-memory write buffer**
  - **Purpose:** Hold ops that are in WAL but not yet applied; enables fast return for Create/Update/Delete and merge for Read/List.
  - **Structure:** Ordered queue of ops (FIFO for worker) plus index by `(kind, id)` for lookup: latest op per key (create/update/delete) so Read/List see correct pending state.
  - **Enqueue:** Append record to WAL (and optionally sync), then add to buffer; return success to caller.

- **Background persister (worker)**
  - **Input:** Drains buffer in order (or reads WAL from checkpoint and applies, advancing checkpoint). Single worker preserves ordering; one goroutine per project root.
  - **Apply:** For each create/update: call existing durable path (write CAS file, update CAS index, hash registry, audit, cache callback). For each delete: remove from index, remove file, hash registry, audit. Reuse `writeObjectToCAS`, `finalizeObjectCreation`, and CAS delete logic so behavior matches current semantics.
  - **After apply:** Remove op from buffer; advance checkpoint so replay does not re-apply after restart.

- **Read/List merge**
  - **Read:** If pending has create/update for (kind, id), return that payload (and infer kind from id if needed). If pending has delete for (kind, id), return not found. Else read from CAS or file as today.
  - **List:** For each kind, merge: IDs from CAS index (and on-disk list) plus IDs from pending creates for that kind, minus pending deletes. Then resolve each ID (from pending or disk) for full objects if needed.

- **Shutdown**
  - **Drain:** On process shutdown, signal worker to stop accepting new ops and drain buffer (apply all remaining). Block until buffer empty or timeout. No writes lost on graceful exit.

- **Startup / crash recovery**
  - **Replay:** On storage init (or first use), open WAL, read from checkpoint to end; for each record, re-insert into buffer (or apply directly and advance checkpoint). Then start worker. Ensures ops that were in WAL but not applied before crash are applied after restart.

- **Guarantees**
  - **Same-process:** Read/List see all buffered writes immediately (pending merge).
  - **Durability:** After WAL append+sync, write is durable; after worker apply, state is on disk (CAS + index).
  - **Crash:** Replay WAL from checkpoint recovers unapplied ops.

## Tests

- **pkg/zqkcli/performance_test.go**
  - **TestObjectOperationsLatency:** Asserts a single Create, Update, and Delete each complete within **3 s** (regression ceiling only; **target is &lt; 100 ms** per op). Ensures regressions (e.g. reintroducing ClearCache on create) are caught. `targetSingleOpDuration` (100 ms) is the goal; tighten assertion when hot path meets it.
  - **TestPerformanceComparison** (Create/Update batches): Asserts 100 creates and 100 updates each complete within **4 min** (target 30 s; limit relaxed until hot path is optimized). Tighten back to 30 s when batch path is fast.
- **pkg/zqkcli/crud_baseline_test.go**
  - **TestCRUDBaselineConsistency:** Runs full CRUD for multiple kinds in an **isolated temp environment**. Each single operation must complete within **3 s** (regression ceiling; **target is millisecond-level**). Optionally writes a baseline JSON to **test-scenarios/crud-baseline/baselines/** when run from repo root. With **ZQK_CRUD_PROFILE=1**, writes CPU (and optionally heap) profile for the CRUD loop only.
  - **BenchmarkCRUDBaselineBulk5kCreate** / **BenchmarkCRUDBaselineBulk5kCreateOnly:** Bulk create (and create+delete) benchmarks; target **5k objects in ~1 s**. Use **ZQK_BULK_BENCH_SIZE=100** for quick profiling runs.
- Run: `go test -v -count=1 ./pkg/zqkcli/ -run 'TestObjectOperationsLatency|TestPerformanceComparison|TestCRUDBaselineConsistency'`
- CRUD baseline scenario: see **test-scenarios/crud-baseline/README.md** for how to run, profile, and interpret baselines.

## References

- **PRE_CHANGE_CHECKLIST.md** §2–3 (no LoadFields/ClearCache on hot path; incremental cache maintenance).
- **CLI_PERFORMANCE_AND_CONSISTENCY.md** (response time &lt; 1 s; no full cache clear on every operation).
- **.zqk/metrics/command_metrics.json** (inspect `baseline_duration` / `avg_duration` for object create/update/delete; durations are in nanoseconds).
