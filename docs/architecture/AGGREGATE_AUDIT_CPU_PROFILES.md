# Aggregate-audit CPU profile findings

**Last Verified:** 2026-08-31


**Source:** CPU profiles from `zqk system aggregate-audit` (PRUNED) (and cleanup runs with `--cpu-profile`).  
**Profiles:** 136 `*.out` files in project root (e.g. `aggregate-audit-cpu.out`, `aggregate-audit-clean-metrics.out`, `cpu-AAM-*.out`). All are gzip-compressed pprof format; `go tool pprof -top <file>.out` works as-is (`.out` is valid; renaming to `.prof` is optional).

---

## Profile summary

| Profile | Duration | Total samples | Notes |
|--------|----------|---------------|--------|
| aggregate-audit-cpu.out | 109.54s | 168.98s (154%) | Full aggregation run |
| aggregate-audit-clean-metrics.out | 4.54s | 9.35s (206%) | Short cleanup run |
| cpu-AAM-*.out (sample) | ~2s | ~3.6s | Single metric cleanup |

---

## Where CPU time goes (aggregate-audit-cpu.out)

### By category (flat %)

- **syscall.rawsyscalln** ~27% — kernel I/O (read/write, fs ops).
- **encoding/json** (appendIndent, stateInString, checkValid, object, mapEncoder, unquoteBytes, etc.) — large share; JSON marshal/unmarshal for CAS index and related data.
- **runtime** (tryDeferToSpanScan, mapassign_faststr, scanObject, madvise, pthread_cond_signal, gcDrain) — GC, maps, concurrency.
- **slices.partitionCmpFunc** — sorting (JSON key sort and fs entry sort).

### Application hot spots (cumulative %)

| Function | Cum % | Cum time | Location |
|----------|-------|----------|----------|
| (*AsyncCacheValidationStrategy).scanDirectory | 31.03% | 52.44s | pkg/storage/validation_strategy_async.go |
| (*IDIndex).SetMapping.func2 | 25.52% | 43.12s | pkg/storage/content_addressable_storage_index.go (closure: loadLocked + merge + copy) |
| (*IDIndex).loadLocked | 19.51% | 32.97s | pkg/storage/content_addressable_storage_index.go |
| encoding/json (various) | — | — | Under loadLocked and saveMappingsLocked |

So during aggregate-audit:

1. **CAS index I/O and JSON** — Each `SetMapping` does: load full index from disk (loadLocked → JSON decode), merge one entry, then save (JSON encode + write). With many metrics/cleanups this is O(n) index reads/writes and dominates via loadLocked + save-side JSON.
2. **Directory scanning** — `AsyncCacheValidationStrategy.scanDirectory` recursively walks the kind dir (e.g. audit_event), using `os.ReadDir` and sorting; cost shows up as syscalls and slices.partitionCmpFunc.
3. **JSON** — appendIndent, checkValid, mapEncoder, etc. are consistent with reading/writing the index file (and any object JSON/YAML in the path).

---

## Recommendations

1. **Batch index updates** — Prefer `SetMappings` over many `SetMapping` calls where possible (e.g. when warming from cache or applying a batch of cleanup results) so the index is loaded once and saved once per batch.
2. **Reduce validation scan cost** — If ValidateMappings runs often during aggregation, consider caching or narrowing the scan (e.g. by bucket or time window) so scanDirectory is not repeatedly walking the full tree.
3. **Index format/size** — If the CAS index is very large, JSON decode/encode and I/O will remain hot; consider structured format or sharding if the index grows further (see also SCHEDULER_DIAGNOSTICS_FINDINGS.md for HashRegistry large-save behavior).

---

## How to re-run analysis

```bash
# Top 20 nodes by flat time
go tool pprof -top -nodecount=20 aggregate-audit-cpu.out

# Top 20 by cumulative (find our code)
go tool pprof -cum -nodecount=20 aggregate-audit-cpu.out

# Filter to zqk
go tool pprof -top -nodecount=60 aggregate-audit-cpu.out | grep -E "zqk|storage|audit"

# Interactive / HTTP
go tool pprof -http=:8080 aggregate-audit-cpu.out
```

No need to rename `.out` to `.prof`; pprof accepts any filename.
