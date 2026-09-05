> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Path alias resolution (any path, prefix scheme)

**Last Verified:** 2026-08-31


**Status:** Implemented  
**Purpose:** All path resolution uses a path alias cache at runtime so paths are not hardcoded and data can be moved with minimal risk. Path references use scheme prefixes to distinguish project-relative aliases, absolute/file URIs, and web URLs.

---

## 1. Scheme prefixes

Path references use these prefixes (see `pkg/paths` constants):

| Prefix   | Meaning | Example | Resolved to |
|----------|---------|---------|-------------|
| `prefix:` | Project-relative path from alias cache | `prefix:docs` | `<project-root>/docs` |
| `prefix:streams/audit_event` | Stream segment dir from cache | — | `<project-root>/.zqk/streams/audit_event` |
| `abs:`   | Absolute path or file URI (passthrough) | `abs:file:///tmp/foo` or `abs:/local/path` | Value after prefix |
| `web:`   | Web URL (passthrough) | `web:https://example.com/resource` | Value after prefix |

- **prefix:** The cache maps alias → path relative to project root. Built at scheduler cache pre-warm (Tier 0), or by `zqk system path-cache` (PRUNED) / `zqk use <path>`. Use `paths.ResolvePathStrict` so a missing alias returns `ErrPathAliasNotInCache` (no silent fallback); `ResolvePath` still has a best-effort fallback for legacy callers.
- **abs:** and **web:** No cache lookup; the resolver returns the remainder after the prefix for use by callers (e.g. remote file or HTTP client).
- References with **no scheme** are treated as `prefix:<value>` (alias or relative path).

---

## 2. Path alias cache

- **Built at:** Scheduler cache pre-warm **Tier 0**; or `zqk system path-cache` (PRUNED) (build if missing, or start background refresh if stale); or `zqk use <path>` (refresh for the new root).
- **Staleness:** `paths.IsPathCacheBuilt(projectRoot)` and `paths.IsPathCacheStale(projectRoot, checkDirs)` use last-built time vs. mtime of critical dirs (e.g. `.zqk`, `.zqk/config`). Refresh when stale so path resolution stays correct.
- **Background refresh and swap:** The cache uses an immutable snapshot per project root. Refreshes run async and atomically swap in the new snapshot. Use `storage.RefreshPathCacheForProject(ctx, projectRoot, provider, opts)` for context cancellation, optional heartbeat, and completion notification. Options: `OnHeartbeat(message)` called periodically while building (e.g. for progress when the user waits); `OnComplete(err)` called when done (for fire-and-forget notification); `HeartbeatInterval` (default 500ms). The function returns a channel that receives one value (nil or error) when done. To wait with heartbeat: pass `OnHeartbeat` and block on the channel. To run without waiting: pass `OnComplete` and return; the callback runs when the refresh finishes. `storage.RefreshPathCacheForProjectAsync(projectRoot, provider)` is fire-and-forget with no callbacks. When cache is already built but stale, `zqk system path-cache` (PRUNED) starts a refresh; use `--wait` to block with heartbeat updates, or omit for background refresh with OnComplete notification.
- **Incremental updates:** When a path is added (e.g. new doc_entry or folder), call `paths.AddPathAlias(projectRoot, alias, relPath)`. When removed, call `paths.RemovePathAlias(projectRoot, alias)`. These copy the current snapshot, apply the change, and swap so resolution stays correct without a full rebuild.
- **Contents:** For each project root, a map **alias → path relative to project root**:
  - Default project structure: `docs`, `process`, `architecture`, `zqk`, `.zqk`, `streams`, `config`, `cache`, `logs`, `state`, `metrics`, `scheduler` (`.zqk/scheduler`), `pre_commit` (`.zqk/pre-commit`), `drafts` (`.zqk/drafts`), `process_internal`, `process_policies`, `process_goals`, datacell runtime keys (`datacell_feature_flags`, `datacell_cli_hook_profile`, `datacell_tray_yaml`, `datacell_runtime_manifest`, `datacell_agent_chat_channel_config`, `datacell_agent_chat_channel_events`, `datacell_steward_enqueue`), etc. (see `paths.DefaultPathAliases()`).
  - Stream segment dirs: `streams/<kind>` for each stream-enabled kind (e.g. `streams/audit_event` → `.zqk/streams/audit_event`).
- **API:** `paths.ReplacePathCache(projectRoot, aliases)` replaces the cache (atomic swap). `paths.BuildPathAliasCache(projectRoot, aliases)` merges aliases into the current snapshot and swaps. `paths.AddPathAlias` / `paths.RemovePathAlias` for single-alias updates. `storage.BuildPathAliasCacheForProject(projectRoot)` clones `DefaultPathAliases`, merges brand `paths.aliases` from `zqk-settings.yaml` when present, adds stream segment aliases, then replaces. `storage.RefreshPathCacheForProjectAsync(projectRoot, provider)` builds in background (with doc_entry paths if provider non-nil) and swaps. `paths.ResolvePath(projectRoot, pathRef)` resolves; `paths.GetPathAlias(projectRoot, alias)` returns the cached relative path.

---

## 3. Usage

- **PathResolver (injectable façade):** `paths.NewPathResolver(projectRoot)` implements `paths.PathResolver` with `ResolveStrict` and `ResolveFromCacheOrConstant` delegating to the functions below. From CLI code, use `ctx.PathResolver()` on `*cli.Context` (see `internal/cli/context.go`). `PathResolver` is wired through **`withInnerContext`** so nil wrapper/embedded context returns a safe zero resolver—see **[WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md](./WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md)** (glossary `GLS-1775965558831066000-25c5083a`). Tests can inject a mock with `ctx.WithPathResolver(mock)` (inner: `(*context.Context).WithPathResolver`) so resolution does not depend on the global alias cache for that context.
- **Resolve any path:** Prefer `paths.ResolvePathStrict(projectRoot, pathRef)` so cache miss returns `ErrPathAliasNotInCache`; callers can run `zqk system path-cache` (PRUNED) or ensure pre-warm. Legacy: `paths.ResolvePath` has a fallback (no error).
- **Stream segment dirs:** `storage.GetStreamSegmentDir(projectRoot, kind)` returns `(string, error)`; error when cache not built.
- **CLI:** Run `zqk system path-cache` (PRUNED) to build or refresh the cache for the current project root (returns quickly). Use `zqk system path-cache --show-paths` (PRUNED) after the cache step to print resolved absolute paths for the datacell runtime files (and effective protocol version from optional `datacell_runtime.json`). Run `zqk use <path>` to set project root and refresh the path cache for that root.
- **Pre-warm:** Scheduler Tier 0 runs `prewarmPathAliasCache`, which calls `storage.BuildPathAliasCacheForProject(projectRoot)`.

---

## 4. doc_entry and other path attributes

- **doc_entry.path** must use a path-cache alias (e.g. `prefix:docs/architecture/foo.md`). Stored when creating/updating; pre-warm and create merge doc paths into the cache. Use `paths.PathRefFromRelPath(relPath)` when writing; `paths.NormalizeDocEntryPathForKey(path)` when building key sets; `paths.ResolveDocEntryPath(projectRoot, pathRef)` when resolving to an absolute path. See [PATH_CACHE_MIGRATION_SCAN.md](./PATH_CACHE_MIGRATION_SCAN.md).

---

## 5. References

- **Glossary:** **Data stream summary** (`GLS-1774504419525806000-78a9c2e4`) ties logical storage contracts to path aliases and observability; **Data cell** (`GLS-1774504405009664000-c1c04629`) describes coordinator-gated access to stream/CAS internals. Requirements **REQ-DATASTREAM-001**, **REQ-DATACELL-001**.
- [PATH_CACHE_MIGRATION_SCAN.md](./PATH_CACHE_MIGRATION_SCAN.md) — Call sites to migrate, markdown guidance, doc_entry path
- [STREAM_STORAGE.md](./STREAM_STORAGE.md) — Stream storage and path resolution
- `pkg/paths/constants.go` — PathSchemePrefix, PathSchemeAbs, PathSchemeWeb
- `pkg/paths/resolver.go` — BuildPathAliasCache, ResolvePath, ResolvePathStrict, GetPathAlias, IsPathCacheBuilt, IsPathCacheStale, DefaultStalenessCheckDirs
- `pkg/paths/path_resolver.go` — PathResolver interface, NewPathResolver; CLI: `ctx.PathResolver()` on `*cli.Context`
- `pkg/storage/stream_path_resolver.go` — BuildPathAliasCacheForProject, GetStreamSegmentDir
- Process policy and requirement for path resolution from cache (POL / REQ stream path cache)

---

## 6. Brand settings (implemented)

- **{brand}-settings.yaml:** The CLI orients itself using a brand-named settings file (e.g. `zqk-settings.yaml`) at the project root. Schema: `.zqk/cli/specs/schemas/brand_settings.schema.json` (paths.project_root, paths.staleness_check_dirs, paths.aliases, cli.default_context, **kernel_state.snapshot_backup_dir** / **snapshot_backup_keep** for state-commit prior tips).
  - **`paths.aliases`:** Merged **on top of** `paths.DefaultPathAliases()` (not a full replace). Partial overrides (e.g. only `process` and `streams`) keep all other defaults, including datacell runtime paths and `prefix:docs`.
  - **`zqk use <settings-path>`** takes a settings file path (or a directory containing `zqk-settings.yaml`), loads it, and derives the project root from `paths.project_root` (or the file’s directory). It persists that project root in `.zqk/current_root` and aligns the scheduler. The settings file is the definitive resource for orienting the CLI with its data; `use` does **not** auto-create it.
  - **Scheduler and services:** Without a valid brand settings file for the resolved project root, the scheduler (and any service that requires orientation) does not start and returns a clear error that points back to `zqk use <settings-path>`.
- **Env vars:** Resolution order remains ZQK_PROJECT_ROOT → ZQK_TEST_ROOT → .zqk/current_root → CWD; brand settings supply path options and defaults once a root is chosen. **Do not** set `ZQK_PROJECT_ROOT` to an ATK git worktree — that skips `paths.project_root` and forks the kernel (`POL-AGENT-KERNEL-ROOT-BINDING-001`). Off-tree worktrees bind with `paths.project_root` **and** unset worktree env, or they stay git-only while daemons keep studio env.
- **Follow-up (not yet):** Cache-loaded events; migration utility for moving data.
