# Path-cache migration scan

**Last Verified:** 2026-08-31


**Purpose:** Identify locations that should use path-cache aliases instead of hardcoded paths, and document markdown conventions (alias + valid links).

**See also:** [PATH_ALIAS_RESOLUTION.md](./PATH_ALIAS_RESOLUTION.md) for scheme and cache behavior.

---

## 1. doc_entry path attribute

- **Spec:** `doc_entry.path` is "File system path or URL to the document" and "Path must exist or be resolvable via resolver."
- **Requirement:** The path attribute MUST use a path-cache alias for programmatic resolution (e.g. `prefix:docs/architecture/PATH_ALIAS_RESOLUTION.md`).
- **Implementation:**
  - **Write (create/update):** Store `prefix:` + relative path so the value is a path ref. When creating a doc_entry, merge that alias into the path cache so it resolves immediately. Pre-warm merges all existing doc_entry path values into the cache after building default+stream aliases.
  - **Read (GetExistingEntries, view, etc.):** Normalize stored value when building key sets (strip `prefix:` so `file.RelPath` matches). When resolving to an absolute file path, use `paths.ResolvePathStrict(projectRoot, pathRef)` or a helper that accepts both `prefix:...` and legacy bare relative paths.
- **Backward compatibility:** Consumers that resolve doc_entry.path to a file path should support both forms: path-cache ref (`prefix:...`) and bare relative path (legacy), e.g. via `paths.ResolvePath(projectRoot, path)` (best-effort) or a dedicated `ResolveDocEntryPath(projectRoot, pathRef)` helper.

---

## 2. Go call sites: hardcoded paths that should use path-cache

These locations build paths with `filepath.Join(projectRoot, "docs", "process")`, `filepath.Join(projectRoot, paths.ProjectDataDir, ...)`, or similar. Prefer resolving via path-cache so behavior stays consistent and data can move.

| Location | Current pattern | Recommended |
|----------|-----------------|-------------|
| `cmd/zqk/system/check_async_baseline.go` | ~~`filepath.Join(projectRoot, paths.ProcessDir)`~~ | **Done:** `resolveProcessDirForProject` → `paths.ResolvePathStrict(..., "prefix:process")` (after `storage.BuildPathAliasCacheForProject`) |
| `cmd/zqk/system/async_check_helpers.go` | ~~`filepath.Join(checkCtx.ProjectRoot, paths.ProcessDir)`~~ | **Done:** same helper for discovery and cache path candidates |
| `pkg/scheduler/handlers_cache_prewarm.go` | ~~`filepath.Join(projectRoot, "docs", "process")`~~ (prewarmReverseReferenceIndex) | **Done:** `paths.ResolvePathStrict(projectRoot, "prefix:process")` |
| `pkg/scheduler/config.go` | ~~`filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, ...)`~~ | **Done:** `paths.ResolvePathFromCacheOrConstant(projectRoot, "scheduler", …)`; default alias `scheduler` → `.zqk/scheduler` |
| `pkg/storage/stream_delta_config.go` | ~~`filepath.Join(projectRoot, ...ProcessInternalObjectSpecsDir)` / `paths.ProcessDir` + `_internal/configs`~~ | **Done:** `paths.ResolvePathFromCacheOrConstant` for `process_internal` + `object_specs`, and `process` + `_internal/configs` |
| `pkg/storage/change_journal_helper.go` | ~~`filepath.Join(projectRoot, "docs", "process", "change_journal", month)`~~ | **Done:** `paths.ResolvePath(projectRoot, "prefix:process")` then join change_journal/month |
| `pkg/validation/paths_config.go` | Various `filepath.Join(wd, relPath, ...)` with ProcessInternalConfigsDir etc. | Prefer path-cache aliases where project root is known |
| `cmd/zqk/system/check_cache.go` (AddEntriesFromBuild) | ~~`filepath.Join(projectRoot, "docs", "process")`~~ | **Done:** `paths.ResolvePath(projectRoot, "prefix:process")` |
| `cmd/zqk/system/check_cache.go` (getCacheFilePath) | ~~`filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, ...)`~~ | **Done:** `paths.ResolvePath(projectRoot, "prefix:cache")` + filename |
| `pkg/docman/discoverer.go` | ~~`filepath.Join(d.rootPath, paths.DocsDir)`~~ | **Done:** `paths.ResolvePath(d.rootPath, "prefix:docs")` |
| `pkg/storage/runtime_delta_config.go` | ~~`filepath.Join(projectRoot, paths.ProcessDir, "_internal", "configs", …)`~~ | **Done:** `paths.ResolvePathFromCacheOrConstant(..., "process", paths.ProcessDir)` + `_internal/configs` |
| `cmd/zqk/system/check_impl.go` (blocking check) | ~~`Join(projectRoot, paths.ProcessDir)`~~ | **Done:** `resolveProcessDirForProject` + fallback |
| `cmd/zqk/system/status_helpers.go` | ~~`Join(projectRoot, paths.ProcessDir)`~~ | **Done:** `resolveProcessDirForProject` for status, audit activity, storage info; incomplete message uses `GetPathAlias(..., "process")` when cache built |
| `cmd/zqk/system/repair_yaml.go` (`findObjectFileByID`) | ~~`Join(projectRoot, paths.ProcessDir)`~~ | **Done:** `resolveProcessDirForProject` + fallback |
| `pkg/storage/object_storage_file.go` (`NewFileObjectStorage`) | ~~`Join(projectRoot, paths.ProcessDir)`~~ | **Done:** `paths.ResolvePathFromCacheOrConstant(..., "process", paths.ProcessDir)` (before `EnsurePathAliasCacheReady`) |

**Note:** Migrating every call site is follow-up work. Path cache must be built before any resolution (pre-warm or `zqk system path-cache` (PRUNED)). Tests that don’t run pre-warm should call `storage.BuildPathAliasCacheForProject(projectRoot)` (and optionally merge doc_entry paths) in setup.

### 2.1 Package triage (for migration + test-bundle coherence)

Work **one package (or small subtree) per pass** so `scan-tests` / pre-commit bundles point at a single failure domain. Order:

| Tier | Packages / areas | Rationale |
|------|------------------|-----------|
| **P0** | `cmd/zqk/system` (check, cache, async, status, repair, init hot paths) | User-facing system check and caches; highest blast radius. Remaining: e.g. `init_impl`, detect/repair helpers still on literal `paths.ProcessDir` where listed below. |
| **P1** | `pkg/storage` (object paths, CAS, snapshots, validation strategies), `pkg/scheduler` (jobs, handlers, config) | Durable I/O and scheduler layout; aliases `scheduler`, `pre_commit`, `drafts` support consistency. |
| **P2** | `pkg/validation`, `pkg/objects` prewarm/loaders, `pkg/migration`, `pkg/operational` | Spec and validation paths; migrate when `projectRoot` is known. |
| **P3** | Tests and `cmd/zqk/utility` scenario builders | Keep **literal** `paths.ProcessDir` under `t.TempDir()` when only building a fake tree; call `BuildPathAliasCacheForProject` when the test asserts resolution. |

**CVS anchor (integration pillar):** `integration/fixtures/convergence_session/path_cache_migration_by_package.yaml` (`CVS-ITEST-PATH-CACHE-MIGRATION-PACKAGES-001`) — use as the durable contract when driving multi-step migration; it does not live under `docs/process/`.

---

## 3. Markdown files: path-cache alias and links

- **No markdown resolver override:** There is no project-specific markdown link resolver that rewrites `[text](path)` at render time. Standard markdown tooling (GitHub, VS Code, etc.) resolves links as relative or absolute file paths.
- **Recommendation:** Use both:
  1. **Valid markdown links** so readers and standard tools get working navigation, e.g. `[PATH_ALIAS_RESOLUTION](./PATH_ALIAS_RESOLUTION.md)` or `[Doc](docs/architecture/foo.md)`.
  2. **Path-cache alias for programmatic use** where tooling needs to resolve paths (e.g. doc_entry.path, scripts, or automation). In YAML or code that references a doc path, use the alias form `prefix:docs/architecture/foo.md` so resolution goes through the path cache.
- **In doc_entry objects:** The `path` attribute holds the path-cache alias (e.g. `prefix:docs/architecture/foo.md`). That is for programmatic resolution; it is not the markdown link text. Doc content (markdown body) should still use normal relative links for readability and editor/CI link checks.
- **If a markdown resolver is added later:** If the project adds a resolver that rewrites links (e.g. `prefix:docs/...` → absolute or relative path at render time), then markdown could use only the alias form and the resolver would produce the link; until then, keep both alias (for tooling) and valid links (for humans and tools).

---

## 4. Test and bootstrap paths

- Many tests use `filepath.Join(tmpDir, "docs", "process", ...)` to build a minimal tree. Those can continue to use literal paths for isolation, or use path-cache after calling `BuildPathAliasCacheForProject(tmpDir)` (and optionally merging doc paths) so behavior matches production.
- **`pkg/storage/path_alias_routing_test.go`** (`TestBuildPathAliasCacheForProject_CustomAliasesRemap`): isolated test that writes `zqk-settings.yaml` with custom `process` and `streams` aliases, calls `BuildPathAliasCacheForProject` twice, and asserts `ResolvePathStrict` / `GetStreamSegmentDir` track the remap.
- **object_storage_dynamic_test_helper.go**, **comprehensive_test_helper.go**, **scenario_builder_build.go:** When constructing doc_entry objects for tests, set `path` to the path-cache form (e.g. `prefix:docs/test.md`) and ensure path cache is built in test setup if resolution is exercised.

---

## 5. Summary

| Area | Action |
|------|--------|
| doc_entry.path | Store and resolve via path-cache alias; merge doc_entry paths into cache at pre-warm and on create. |
| Go call sites | Prefer `paths.ResolvePathStrict(projectRoot, "prefix:<alias>")` over `filepath.Join(projectRoot, ...)`; migrate incrementally. |
| Markdown | Keep valid markdown links for navigation; use path-cache alias in YAML/code/automation. No resolver override yet. |
