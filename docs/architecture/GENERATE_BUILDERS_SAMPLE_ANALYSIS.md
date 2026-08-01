# generate-builders Process Sample Analysis (PID 7937)

**Date:** 2026-02-15  
**Sample:** `build-sample.txt` (macOS `sample` of zqk system generate-builders --overwrite (PRUNED))  
**Process:** zqk [7937], parent go [7889]

## Summary

| Metric | Value |
|--------|--------|
| **Physical footprint** | 14.4 GB |
| **Samples** | 2229 (1 ms sampling) |
| **Main thread** | ~100% in `_pthread_cond_wait` (blocked) |
| **Other thread** | 2219/2229 samples in `usleep`/`nanosleep` (sleeping) |
| **Hot thread (1911380)** | 65 samples in a **deep, repeating call chain** (same ~15 PCs in a loop) |

## Findings

### 1. Main thread is blocked

The main goroutine is almost entirely in `_pthread_cond_wait` / `__psynch_cvwait`. So the process is not CPU-bound on the main thread; it is waiting (e.g. on a channel or mutex).

### 2. Very deep recursion on one thread

Thread 1911380 shows a **repeating sequence** of the same zqk frame offsets, e.g.:

- `0x7dc4d4` → `0x7aa744` → `0x7a8c44` → `0x7a90cc` → `0x7b5048` → `0x8f2f0` → `0x7b50cc` → `0x762938` → `0x76438c` → `0x767a60` → `0x7e52bc` → `0x7e5328` → `0x7a0170` → `0x7db09c` → `0x7dbf10` → then back to `0x7dc4d4`

That pattern repeats for many levels (sample counts go from 65 down to 3 as the stack deepens). So one goroutine is in a **deep or unbounded recursion**, which fits with:

- **14.4 GB** (stack + allocations in the recursive path)
- **Long runtime** (lots of work per level or many levels)

The binary was sampled without symbols, so these offsets need to be resolved (e.g. with `go tool nm` or a similar build) to get function names.

### 3. Likely code paths for recursion

From the codebase, the main recursive paths in the generate-builders flow are:

1. **Spec loading with inheritance**  
   `LoadSpecWithInheritance` → `loadSpecWithInheritanceRecursive` → load parent → recurse.  
   Cycle detection exists via a `visited` map; depth is bounded by the inheritance DAG (e.g. base_object ← auditable ← …). Unlikely to be 50+ levels unless there is a cycle or a bug in `visited`.

2. **Resolving inheritance from builders**  
   `resolveSpecInheritance` can recurse when resolving parent specs (from registry or YAML). Same `visited` and DAG depth argument as above.

3. **JSON schema validation**  
   For each spec, `loadSpecWithInheritanceRecursive` can call `validator.ValidateYAML(specPath, schemaRef)`. The `santhosh-tekuri/jsonschema` library compiles and validates; validation walks the document and schema. Deeply nested YAML (e.g. many fields with nested `checklist`/`validation`) can lead to deep call stacks during validation. This is a plausible candidate for 50+ levels if some spec files are very large or deeply nested.

4. **CreateConstants**  
   Uses `GetGlobalSpecLoader().LoadSpecWithInheritance(...)` for the current spec and parent. So the recursion could be coming from the **global** SpecLoader (e.g. wrong or empty `specsDir` leading to repeated failed loads and retries, or a path that triggers more loading than expected).

## Fixes applied (Feb 2026)

- **Use local SpecLoader in generate-builders:** `ConstantsFactory` now accepts an optional `*objects.SpecLoader` via `NewConstantsFactoryWithSpecLoader`. The `generate-builders` command passes its local `specLoader` (with correct `specsDir`) so `CreateConstants` no longer uses `GetGlobalSpecLoader()`, avoiding wrong/empty specsDir and redundant loading.
- **Skip schema validation in generate-builders:** When `ZQK_SKIP_SPEC_SCHEMA_VALIDATION=1`, spec loading (`pkg/objects/spec_loader.go`) and codegen (`pkg/specbuilder/builders/codegen.go`) skip JSON schema validation. The `generate-builders` command sets this env at start when unset. The **Makefile** exports `ZQK_SKIP_SPEC_SCHEMA_VALIDATION=1` for `generate-spec-builders` so every `go run` sees it. If you run **without** make (e.g. `go run ./cmd/zqk system generate-builders` (PRUNED)), set it yourself: `ZQK_SKIP_SPEC_SCHEMA_VALIDATION=1 go run ./cmd/zqk system generate-builders --overwrite` (PRUNED).
- **Recursion depth limit:** `loadSpecWithInheritanceRecursive` and `resolveSpecInheritance` now enforce `maxSpecInheritanceDepth` (30). If depth is exceeded (e.g. cycle or bug), they return an error instead of recursing indefinitely, preventing unbounded stack growth and process hang.

## Recommended next steps (if issues persist)

1. **Resolve symbols**  
   Build with symbols and re-run `sample`, or run under Delve and capture a stack trace when memory is high, to see the exact function names in the repeating chain.

2. **Reduce or disable schema validation in this path**  
   In `pkg/objects/spec_loader.go`, `loadSpecWithInheritanceRecursive` calls `validator.ValidateYAML` for every spec file. Temporarily skip this call when the loader is used from generate-builders (or when an env var is set), and see if memory and runtime drop. If they do, schema validation is a major contributor (e.g. deep validation stacks or heavy compilation).

3. **Avoid using the global SpecLoader in generate-builders**  
   In `cmd/zqk/system/generate_builders.go`, the local `specLoader` (with correct `specsDir`) is used for `GetLoadOrder()` only. `GenerateBuilderFromYAML` uses `GetGlobalSpecLoader()` and then `CreateConstants` uses that for `LoadSpecWithInheritance`. Pass the local loader (or a loader built with the same `specsDir`) into the codegen path so that spec loading uses the same, correct dir and does not rely on the global loader’s (possibly empty) `specsDir`.

4. **Add a recursion / depth limit**  
   In `loadSpecWithInheritanceRecursive` and/or in the schema validator path, add a maximum depth (e.g. 20–30) and return a clear error if exceeded. That will prevent runaway recursion and make the cause obvious.

5. **Reproduce with a memory profile**  
   Run:  
   `go run ./cmd/zqk system generate-builders --overwrite -memprofile= (PRUNED).zqk/genbuild-mem.prof`  
   Then inspect with:  
   `go tool pprof -alloc_space -top .zqk/genbuild-mem.prof`  
   or `go tool pprof -http=:8080 .zqk/genbuild-mem.prof`  
   to see where allocations and size come from.

## Later sample (2026-02-18, PID 89866)

A second sample (`gen-builders-sample.txt`) of `zqk-build` (Makefile staging binary running `generate-builders` and `generate-instance-builders`) showed:

- **Main thread:** 2177 samples in `pthread_cond_wait` (blocked).
- **Other thread:** 2170 samples in `runtime.usleep_trampoline` (1470) and `pthread_cond_wait` (700).
- **Other threads:** Many samples in `open` (file I/O) and cond_wait.

So the process was still **waiting** (cond_wait, usleep), not CPU-bound. Likely causes: global SpecLoader init/lock contention, or heavy I/O (many spec file opens).

**Additional fix applied:** In `pkg/specbuilder/builders/codegen.go`, `GenerateBuilderFromYAML` no longer calls `GetGlobalSpecLoader()` when `constantsFactory != nil`. The generate-builders command passes a local SpecLoader via `NewConstantsFactoryWithSpecLoader`, so the global loader is not needed and avoiding it reduces init contention and possible hang in this path.

## Fix: Skip session for system generate-* (2026-02-22)

Sample of hung `zqk-build` (PID 97277) showed main thread in `pthread_cond_wait` and another thread in `indexQueue.startWorker` → `processBatch` → `RunInLock` → `loadLocked`. For `system generate-builders`, PreRunE was still starting a zqk session (and thus creating storage on demand), which triggered CAS index queue workers. To avoid any interaction between generate-builders and the index queue/storage path, **session and storage creation are now skipped for all `system generate-*` commands** in `cmd/zqk/root.go` (PersistentPreRunE). So `zqk system generate-builders` (PRUNED) (and other generate-* subcommands) no longer call `GetObjectStorageForCommand` or start a session; the CAS index write queue is never created or used for those commands.

## References

- Sample file: `~/Desktop/build-sample.txt` (PID 7937); `~/Desktop/gen-builders-sample.txt` (PID 89866); `~/Desktop/hung-build-gen-spec-sample.txt` (PID 97277)
- Spec loader: `pkg/objects/spec_loader.go` (`loadSpecWithInheritanceRecursive`, `resolveSpecInheritance`)
- Codegen: `pkg/specbuilder/builders/codegen.go` (`GenerateBuilderFromYAML`; when constantsFactory is set, global SpecLoader is not used)
- Schema validation: `pkg/specbuilder/yaml/schema_validator.go` (used from spec_loader and codegen)
- Root PreRunE: `cmd/zqk/root.go` (skip session when `isSystemGenerate`: `system generate-*`)
