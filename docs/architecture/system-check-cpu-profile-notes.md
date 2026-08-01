# System check CPU profiling and "Warming CAS indexes" slowness

## Quick run (output to file)

```bash
# Run once; all output to .zqk/logs/
./scripts/run_system_check_with_cpu_profile.sh [timeout e.g. 60m]
# Or manually:
./zqk system check --verbose --cpu-profile .zqk/logs/system-check-cpu.pprof --timeout 60m \
  > .zqk/logs/system-check_$(date +%Y%m%d-%H%M%S).log 2>&1
```

## Analyze the profile (after run exits)

```bash
# Top CPU consumers
go tool pprof -top -nodecount=40 .zqk/logs/system-check-cpu.pprof

# Cumulative text (shows call chains)
go tool pprof -text -cum .zqk/logs/system-check-cpu.pprof

# Interactive
go tool pprof .zqk/logs/system-check-cpu.pprof
```

## Why "Warming CAS indexes" used to be slow (fixed)

- **Where it runs**: `warmCASIndexesFromCache` in `cmd/zqk/system/check_cache.go`, called from `BuildCache` after loading/building the object ID cache.
- **Current behavior**: Warming uses **only the existing object ID cache** (id→path per kind). We call `EnsureCASIndexFromPaths(kind, idToPath)` for each kind—no directory scans or per-file reads. Cache load rejects any cache with entries missing `FilePath`, so we rebuild once and then always have paths; warm is just merging cache data into CAS index files.
- **Removed**: The previous path that called `EnsureCASIndexPopulatedFromScan(kind)` (scan dir + read every file to get id) is no longer used for check warm. That had been the bottleneck (O(files) reads per kind).

### Kind mapper serialization (fixed)

- **Problem:** During warm, worker goroutines called `GetDirectoryFromKind` → **DynamicKindMapper.Initialize()** (spec load, YAML parse, dir scan) under lock, serializing all workers and blowing up warm phase time.
- **Fix:** Pre-initialize the kind mapper (in parallel where applicable, then wait) **before** starting the warm workers. Emit progress messages (e.g. "Merging CAS indexes for N kinds…", "CAS indexes warmed") so the phase is observable.

## Increasing the cache loader timeout (for profiling or large repos)

In `.zqk/config/config.yaml` (create if missing), under `component_loaders`:

```yaml
component_loaders:
  object_id_cache:
    timeout_seconds: 900   # 15 minutes for large repos / profiling
    # or: load_operation_seconds: 900
```

Then re-run the check; the profile and log will reflect the longer run.
