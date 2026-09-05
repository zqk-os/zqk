# Priority roadmap (current focus)

**Last Verified:** 2026-08-31


**Purpose:** Ordered focus areas after wrapping up spec-index/CLI completion work. Role-aware CLI ops are explicitly deferred until after the current priority plan and multi-agent work.

---

## 1. Object count disparities

**Goal:** Per `docs/architecture/OBJECT_COUNT_SELF_MAINTENANCE.md` — the system must maintain its own object count; report must be trustworthy without `--no-cache`; single maintenance path.

**Status: in place.** Verified:
- **Single delete:** CLI passes `WithCacheInvalidate(ctx, id)`; storage `Delete` calls `executeCacheOperation`; handler runs `InvalidateObjectIDCache(OldID)` and `InvalidateListCache()` (root.go).
- **Bulk delete:** `cmd/zqk/object/bulk_delete.go` calls `BulkInvalidateObjectIDCache(successIDs, projectRoot)` after success; that invalidates list cache and saves cache to disk.
- **Report self-correction:** `cmd/zqk/system/object_count_report.go` when `total_object > total_disk` runs `CleanStaleCacheEntries(projectRoot)` and re-runs congruence with storage counts so the written report is accurate without `--no-cache`.

**Refs:** OBJECT_COUNT_SELF_MAINTENANCE.md, OBJECT_COUNT_MANAGEMENT.md, retention_tolerance.yaml.

---

## 2. Audit stream location (canonical)

**Canonical location:** `.zqk/streams/audit_event/` (segment files e.g. `YYYY-MM-DD_stream.json`). Resolved via path-cache alias `streams/audit_event` → `ProjectDataDir/StreamsDir/audit_event` (see `pkg/storage/stream_path_resolver.go`, `GetStreamSegmentDir`).

**Legacy:** `.zqk/audit_streams/` — doc reference in `docs/architecture/AUDIT_STREAM_FORMAT.md` (migration: `mv .zqk/audit_streams .zqk/streams/audit_event`). Constant `paths.AuditStreamsDir` kept for migration/external reference only.

**Findings:**
- **Code:** New writes use `GetStreamSegmentDir(projectRoot, "audit_event")` (path-cache); no Go code writes to `AuditStreamsDir` for I/O.
- **Runtime state:** Existing `.zqk/state/stream_registry_audit_event.jsonl` may still contain `loc` values pointing at `.zqk/audit_streams/...` if written before path-cache pointed at `.zqk/streams/audit_event`. 
- **Work:** (1) Ensure path-cache is built before any stream write (scheduler pre-warm / CLI). (2) Data migration: run **`zqk system migrate (PRUNED)-audit-stream`** to move segment files from `.zqk/audit_streams/` to `.zqk/streams/audit_event/` (with filename mapping to `YYYY-MM-DD_stream.json`) and rewrite the stream registry; idempotent if legacy dir is missing.

---

## 3. Path-cache usage audit

**Policy:** All resolution of project structure (streams, process_internal, config, etc.) should use the path alias cache (`paths.ResolvePathStrict`, `FindPath`, or storage’s `GetStreamSegmentDir`) so locations are consistent and movable.

**Done:** Replaced hardcoded `.zqk` and subdir strings with `paths` constants and `filepath.Join(projectRoot, paths.ProjectDataDir, paths.Xxx)` in:
- `cmd/zqk/system/bootstrap_extractor.go` — `paths.CLISpecsDir`
- `cmd/zqk/system/cleanup_duplicates_helpers.go`, `quarantine_report.go`, `cleanup_quarantine_helpers.go`, `repair_cas_corruption.go`, `pkg/storage/cas_corruption_repair.go` — `paths.SystemHealthDir`, `paths.QuarantineDir`
- `cmd/zqk/callback/processor.go` — `paths.CallbackDir`
- `cmd/zqk/system/auto_fix_scheduler_batch.go` — `paths.AutofixDir`
- `cmd/zqk/scheduler/scan_tests_setup.go` — `paths.LockDir`
- `pkg/mcp/prompts.go` — `paths.ProjectDataDir`, `paths.MCPDir` for MCP spec paths

**New constants in `pkg/paths/constants.go`:** `CLISpecsDir`, `SystemHealthDir`, `QuarantineDir`, `AutofixDir`, `LockDir`.

**Note:** Tests that build known tmp dirs may keep literal paths for isolation.

---

## 4. Object cleanup

**Scope:** Orphans, stale refs, retention alignment — ensure they are covered by existing mechanisms and docs.

**Existing mechanisms:**
- **Cache stale entries:** `CleanStaleCacheEntries(projectRoot)` (object-count-report self-correction and on-demand) removes object ID cache entries for missing files; invalidates list cache and saves cache.
- **CAS orphans:** CAS orphan cleanup queue (enqueues hash-file cleanup on replace/move); `RunStaleCASCleanupForResults` runs hash-duplicates cleanup for kinds with "Stale CAS version" issues (check --auto-fix).
- **Retention:** `retention_tolerance.yaml`, `zqk system retention-tolerance` (PRUNED), scheduler jobs (`job_type: retention_tolerance`, `audit_event_aggregation`). See OBJECT_COUNT_MANAGEMENT.md and ensure_retention_jobs.
- **Scripts (targeted):** `cleanup_scheduler_jobs_and_logs.sh` (orphan job YAML + log dirs), `cleanup_orphaned_cas_files.sh`, `delete_unmanaged_audit_yaml.py` (one-time when jobs aren’t enough — OBJECT_COUNT_MANAGEMENT.md).

**Alignment:** Single maintenance path is retention + aggregation jobs + object-count-report (which self-corrects when total_object > total_disk). No new generic "object cleanup" command required; use retention-tolerance, aggregate-audit, and report as documented.

---

## 5. Resolve failing tests

- Address any currently failing tests (e.g. in pkg/zqkcli, cmd/zqk/object, or elsewhere) so CI and local runs are green.

---

## 6. Complete PRI-218

- Finish remaining work for priority plan PRI-218 (process & platform) per backlog and plan.

---

## 7. Role-aware CLI ops (later)

- **Deferred** until after current priority plan and multi-agent work.
- When enabled: filter suggested kinds/fields (and value completions) by current account/role using existing security context and permission checks; doc already in place in `docs/architecture/CLI_PERFORMANCE_AND_CONSISTENCY.md` §3.

---

## 8. Spec origin plane (unified revision + derived indexes)

**Why:** Specs define the persisted data model; operational paths (CRUD, list/search, scenario bundle create order, future data cells) must not drift across ad hoc spec walks and separate caches.

**Architecture:** [SPEC_ORIGIN_PLANE.md](./SPEC_ORIGIN_PLANE.md) — detail layer (`SpecLoader`) vs thin materialized indexes, invalidation, first-foray scope.

**Process:** **P1** backlog **`[REDACTED-ID]`** on priority plan **[REDACTED-ID]** (Object maintenance redesign). Agents see reminders in **PRE_CHANGE_CHECKLIST §3a** and generated **AGENT_CONTEXT_REFRESH.md** (checklist item 9 — spec plane; item 12 — **glossary**).

**Loss if dropped:** Inconsistent validation vs tooling, stale spec indexes, and harder evolution toward graph-backed or remote specs.

---

## References

- OBJECT_COUNT_SELF_MAINTENANCE.md, OBJECT_COUNT_MANAGEMENT.md
- AUDIT_STREAM_FORMAT.md, STREAM_STORAGE.md
- CLI_PERFORMANCE_AND_CONSISTENCY.md
- PRE_CHANGE_CHECKLIST.md
