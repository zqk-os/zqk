# TraitHarness: Unified List-Manipulating Behavior for All Data

**Last Verified:** 2026-08-31


**Status:** Implemented (harness in pkg/cli; healthchk list wired as first consumer)  
**Related:** INTERNAL_OBJECTS_AS_DEDICATED_WALS.md, pkg/cli/query_flags.go, pkg/cli/harness.go, object list, internal list, healthchk list

## Goal

Provide a **single harness** that implements all list-manipulating behavior (filter, group-by, sort, pagination, format, etc.) for **whatever data is being harnessed**. From an interaction standpoint, all data flavors are just "data"; the harness applies the same flags and behavior regardless of whether the source is object storage, a WAL, a registry, or metrics. This simplifies the codebase and gives a consistent UX across object list, internal list, healthchk list, and future list-style commands.

## Problem today

- **Many object types** declare traits: listable, groupable, filterable, sortable, searchable (see object specs: dozens of kinds with these traits).
- **List behavior is duplicated:** object list, internal list, and other commands each wire up `--filter`, `--group-by`, `--sort-by`, `--limit`, `--offset`, and output formatting in slightly different ways. Shared pieces exist (e.g. `pkg/cli` AddQueryFlags, BuildListFilterData, ParseQueryFlags) but the full pipeline (fetch → filter → sort → group → paginate → format) is not one reusable harness.
- **New data sources** (e.g. WAL-backed change_journal_entry, healthchk monitors) reimplement or skip parts of this behavior, leading to inconsistency.

## Proposal: TraitHarness

A **TraitHarness** (or ListHarness / DataHarness) that:

1. **Exposes a single set of flags** for list-style operations: `--filter`, `--format`, `--group-by`, `--sort-by`, `--sort-asc`, `--limit`, `--offset`, `--count`, `--ids-only`, etc. Same flags everywhere a "list" of items is shown.
2. **Accepts a data provider interface** that returns a slice of items (e.g. `[]map[string]any` or a generic "rows" with field access). The provider is responsible only for **fetching** the raw data (from storage, WAL, registry, etc.); the harness does not care where it came from.
3. **Applies list-manipulating behavior** in a consistent order: filter → sort → group → paginate (offset/limit), then format for output (table, json, yaml). Filter/sort/group can be in-memory for small datasets or delegated to the provider when it supports server-side ops (e.g. storage.List with ListFilter).
4. **Respects traits** for the data source: e.g. if the source is "filterable", apply `--filter`; if "groupable", apply `--group-by`; if not, hide or no-op those flags. So the harness can be configured with a **trait mask** (listable, filterable, groupable, sortable) so that healthchk list doesn’t offer group-by if the registry doesn’t support it, while object list does.
5. **Output format:** Default is not YAML (e.g. table or JSON); YAML only when `--format yaml` is requested. Aligns with INTERNAL_OBJECTS_AS_DEDICATED_WALS: from an interaction standpoint it’s the same "data" regardless of storage format.

So: **one harness, many data flavors**. Each flavor (object storage, internal WAL, healthchk registry, metrics, etc.) implements a small adapter that returns "list of items with fields"; the harness provides all the list-manipulating behavior and flags.

## Traits: Existing System and Harness Mapping

The codebase already has a trait system. The harness interface should align with it so that spec-backed data sources can derive their capability mask from specs, and non–spec-backed sources (healthchk, WAL) use an explicit trait set.

### Where traits are defined

- **Trait definitions:** `docs/process/_internal/traits/` (YAML: base_object_traits, base_auditable_traits, read_only_group, confidential_group, field_* groups).
- **Trait README:** `docs/process/_internal/traits/README.md` — standard traits, base groups, specialized groups.
- **Builders:** `pkg/specbuilder/bldr_trait_v1/` — one builder per trait (listable, groupable, filterable, sortable, searchable, readable, writable, modifiable, removable, formatable, constrainable, snapable, cloneable_configuration not present; manipulatable not present).

### Standard traits (from base_auditable_traits / base_object_traits)

| Trait         | Object-level | Field-level | Harness relevance |
|---------------|--------------|-------------|--------------------|
| **listable**  | ✓            | ✓           | Can list; field-level → which columns appear in table (and display_length). |
| **readable**  | ✓            | ✓           | Can read single item; not list-specific but required for get. |
| **writable**  | ✓            | ✓           | Create; out of scope for list harness. |
| **modifiable**| ✓            | ✓           | Update; out of scope for list harness. |
| **removable** | ✓            | ✓           | Delete; out of scope for list harness. |
| **formatable**| ✓           | ✓           | Format for display; harness uses for table/json/yaml output. |
| **filterable**| ✓            | ✓           | Enable `--filter`; field-level → allowed filter fields (see GenerateFilterableFields). |
| **sortable**  | ✓            | ✓           | Enable `--sort-by` / `--sort-asc`; field-level → allowed sort fields (GenerateSortableFields). |
| **searchable**| ✓            | ✓           | Search; can map to same as filter or a dedicated search UX. |
| **groupable** | ✓            | ✓           | Enable `--group-by` / `--group-limit`; field-level → allowed group fields (GenerateGroupableFields). |

**Trait groups (bundle traits to keep the interface lean):**

The system uses **trait groups** so specs and call sites don’t enumerate every trait. Groups are expanded by `TraitRegistry.ExpandTraitGroup` / `ExpandTraits` (`pkg/objects/trait_registry.go`). The harness can accept **group names** and expand once to get the list-related capability mask.

- **base_auditable_traits:** listable, readable, writable, modifiable, removable, formatable, filterable, sortable, searchable.
- **base_object_traits:** base_auditable_traits + groupable (typical for objects extending base_object).
- **read_only_group:** listable, readable, formatable, groupable, filterable, sortable, searchable (no writable, modifiable, removable) — e.g. audit events, metrics, healthchk list.
- **confidential_group:** same as base_object_traits; semantic marker for access:confidential.

So a data source can say “I have **read_only_group**” and the harness expands that to know filterable, groupable, sortable, etc. are on, without the caller setting six booleans.

### Domain-specific and spec-only traits

- **constrainable** — object-level; layout/constraint rules; not list-display.
- **manipulatable** — used in specs (backlog_item, goal, milestone, workstream, certificate, prompt_template); no trait definition file or bldr_trait_v1 builder yet. Semantics: object can be manipulated (e.g. bulk update). For a future **write/bulk harness**, this would gate bulk update; list harness only needs list-related traits.
- **cloneable_configuration** — used in specs (brand, library); no trait definition in traits/; semantic: object supports clone-style operations. Not list-specific.

### Traits not in the codebase today

- **bulkable** — not defined. If we want to gate “can be bulk-updated” or “can be bulk-deleted” by trait, we could add it; then TraitSet could include `Bulkable bool` for a future bulk harness.
- **persistable** — not defined. Could indicate “persisted to storage” vs in-memory/registry; could inform whether list is over storage or a cache. Harness can stay agnostic; provider hides persistence.

### TraitSet for the list harness: use groups to keep the interface lean

The harness needs a **capability mask** that drives which flags exist and which ops run. To avoid call sites enumerating many booleans, the harness can accept **trait group names** and resolve them via the existing registry.

**Option A — Pass trait groups (lean):** Caller passes one or more group names; harness expands and derives the mask.

```go
// Trait groups → harness resolves to capability mask
// e.g. TraitGroups: ["read_only_group"] or ["base_object_traits"]
expanded, _ := traitRegistry.ExpandTraits(traitGroups)
// Then set ListTraitSet from presence of listable, filterable, sortable, groupable, searchable, formatable
```

- **Object list (spec-backed):** pass `objectSpec.ResolvedTraits` (often just `["base_object_traits"]`); registry expands; harness derives ListTraitSet and uses GenerateFilterableFields etc. for allowed field names.
- **Healthchk list:** pass `["read_only_group"]` (or a small custom group) → listable, filterable, sortable, groupable, formatable, etc. without hand-building a mask.
- **WAL-backed list:** pass `["read_only_group"]` or a dedicated group (e.g. `["wal_list_group"]`) so the interface stays one line.

**Option B — Resolved mask (when no registry):** When the data source isn’t spec-backed and you don’t want to depend on the trait registry, pass a pre-resolved struct:

```go
// ListTraitSet (list / query operations only) — used after expansion or when not using groups
type ListTraitSet struct {
    Listable   bool
    Filterable bool
    Sortable   bool
    Groupable  bool
    Searchable bool
    Formatable bool
    CountOnly  bool
}
```

**Recommendation:** Prefer **trait groups** at the call site; harness (or a small helper) expands via `TraitRegistry.ExpandTraits` and builds the internal ListTraitSet. That keeps the harness interface lean and aligns with how specs already use base_object_traits / read_only_group.

Optional future extensions for a **bulk/write harness** (out of scope for list-only): support groups that include writable, modifiable, removable, or a future bulkable/manipulatable, and derive a write-side capability mask the same way.

## Interface sketch

- **DataProvider** (or ListSource): returns items and optionally supports server-side filter/sort/limit.
  - `List(ctx, opts ListOptions) (items []map[string]any, total int, err error)` or `List(ctx) (items []map[string]any, err error)` with in-memory filter/sort/group/limit in the harness.
- **Capability (lean):** Pass **trait group names** (e.g. `["base_object_traits"]`, `["read_only_group"]`). Harness uses `TraitRegistry.ExpandTraits` to get the full trait list, then derives the list capability mask (which flags to add, which ops to run). For spec-backed data, allowed field names for filter/sort/group come from GenerateFilterableFields etc. Alternatively, pass a pre-resolved **ListTraitSet** when not using the registry.
- **Harness.Run(cmd, provider, traitGroupsOrSet)** (or similar): parses flags from cmd, resolves traits to a ListTraitSet if given groups, calls provider.List (with opts from flags), applies filter/sort/group/limit as allowed by the mask, then writes output (table/json/yaml).

Commands stay lean by using groups:

- **Object list:** `Run(cmd, storageListProvider(kind), objectSpec.ResolvedTraits)` — e.g. `["base_object_traits"]`; registry expands; harness derives mask and field names from spec.
- **Internal list:** `Run(cmd, internalListProvider(kind), ["read_only_group"])`
- **Healthchk list:** `Run(cmd, healthchkRegistryProvider(), ["read_only_group"])`
- **Future WAL-backed list:** `Run(cmd, walScanProvider(kind), ["read_only_group"])` or a custom group name

## Benefits

- **Single place** for flags, filter parsing, sort/group/pagination logic, and output formatting. No duplication across object list, internal list, healthchk list.
- **Consistent UX:** Same `--filter`, `--group-by`, `--format` behavior everywhere.
- **Traits drive capability:** Spec (or config) declares listable/filterable/groupable; harness enables only what the data source supports.
- **New data sources** get full list behavior by implementing one small provider; no reimplementing flags or formatting.
- **Aligns with "data is just data":** Whether the backend is YAML files, WAL, or a registry, the interaction is the same.

## Bulk operations and batching (future)

A **system-wide bulk utility** should be part of the trait harness story so we don’t do more I/O than necessary and bulk ops scale. Two complementary approaches:

- **Use existing caches:** We already cache IDs (e.g. object-id-cache). The bulk path should **prefer cached IDs** where the operation can be satisfied from cache (e.g. “delete these kinds” or “delete IDs matching filter” derived from cache) so we avoid extra list I/O when we already have the IDs.
- **Automatic batching:** Batch based on **configured system limits** (batch size, max payload size). The harness (or a shared bulk runner) batches IDs automatically; callers pass a filter or an ID set and the system chunks into batches.
- **Option A (mark-for-delete + cache update + init filter + background maintainer):** Snapshot IDs for posterity; use an existing property (e.g. `enabled=false`) to mark records for delete; bulk-update to set that state. On disable, **remove those IDs from the object-id-cache** (cache update) so the cache no longer includes them; the WAL handles the disable, then the reaper deletes. During init, the loader **filters out** records in the chosen state so they are never loaded—no extra I/O, no impact on scheduling. A **job maintainer** (e.g. scheduler job retention handler) cleans up disabled/obsoleted records in the background in batches. Faster for the user; metrics and posterity from snapshot; doesn't interfere with system performance.
- **Option B — Queue as event payloads:** Instead of blocking the CLI on 27k deletes, **queue batches as event payloads** (e.g. scheduler or internal queue). Each payload is one batch of IDs (or a compressed blob). Workers process batches; progress and completion can be observed via existing scheduler/event UX. Keeps CLI responsive and leverages existing event-driven infrastructure.
- **Compression:** **Compress batch payloads** (e.g. ID lists, or bulk-update payloads) to minimize memory and wire size. The system can decompress on the worker side and run the actual delete/update in batches. Reduces memory utilization and makes large bulk ops feasible without holding full ID lists in memory.

Option A is often faster: mark + optional snapshot; init ignores marked records (e.g. LoadJobs filters out one_time + enabled=false); maintainer job deletes in background—no user blocking, metrics from snapshot. Option B (queue + compression) suits explicit batch deletes when immediate removal is required. The trait harness can expose bulk delete/update as capabilities and support either or both.

## Scope

- **In scope:** List-style commands (object list, internal list, healthchk list, and future list-over-WAL, list-over-metrics). Count can be a variant (count-only path) or use the same harness with `--count`. **Future:** bulk operations (bulk delete, bulk update) as part of the same harness, using the bulk utility above.
- **Fields in the harness:** Fields are part of registering a spec: load fields and constraints into a cache on init (or manually forced), and use that **static cache** to serve data to the trait harness. One place for list behavior plus field metadata → fluid, consistent, performant, responsive. Well-managed accumulable and finite data sets.
- **Out of scope (for this doc):** Full implementation of the bulk utility and event-queue integration; exact package name and placement to be decided in implementation.

## Summary

A **TraitHarness** provides one place for all list-manipulating behavior (filter, group-by, sort, pagination, format) and a single set of flags. Any data source plugs in via a small provider; from an interaction standpoint they are all just "data." **Trait groups** (base_object_traits, read_only_group, etc.) keep the interface lean: call sites pass group names, and the harness expands them via the trait registry to get the capability mask. This simplifies the codebase and keeps list UX consistent across object types and data flavors.

## Implementation

- **pkg/cli/harness.go:** `TraitExpander`, `ListTraitSet`, `ListTraitSetFromExpanded`, `ListSource`, `ListConfig`, `AddListFlags`, `Run`. In-memory filter/sort/group/pagination and table/json/yaml output.
- **First consumer:** `cmd/zqk/healthchk/list` uses `AddListFlags`, a `healthchkListSource` implementing `ListSource`, and `Run` with `TraitGroups: ["read_only_group"]` and `objects.NewTraitRegistry()` as `TraitExpander`.
- **Tests:** `pkg/cli/harness_test.go` (run with `go test ./pkg/cli/... -timeout 30s`).

### Verification before migrating more commands

Before migrating list/count (and related) commands to the trait harness, run the verification script so rollback, lifecycle, and rollback CLI behavior are confirmed:

- **Script:** `scripts/verify-rollback-before-trait-harness.sh`
- **What it runs:** Unit tests for `pkg/rollback`, `pkg/lifecycle`; integration tests for rollback CLI (`TestRollback*` in `cmd/zqk`). All must pass before proceeding with trait-harness migration.

## References

- **Trait definitions:** `docs/process/_internal/traits/README.md`, `docs/process/_internal/traits/*.yaml`
- **Base trait groups:** `docs/process/testing/BASE_TRAIT_GROUPS.md`
- **Trait registry / expansion:** `pkg/objects/trait_registry.go` (ExpandTraitGroup, ExpandTraits)
- **Field-level trait usage:** `pkg/objects/field_help.go` (GenerateFilterableFields, GenerateSortableFields, GenerateGroupableFields)
- **Query flags today:** `pkg/cli/query_flags.go` (AddQueryFlags, ParseQueryFlags, BuildListFilterData)
- **ID cache (bulk / cache-first):** `pkg/storage` object-id-cache, cache-first list path; `pkg/scheduler` ObjectIDCacheBuilder, cache_prewarm handler.
- **Mark-for-delete + maintainer (Option A):** `pkg/scheduler/handlers_scheduler_job_retention.go` — SchedulerJobRetentionHandler deletes one_time + enabled=false (or status=disabled) scheduler jobs in batches; `JobLoader.LoadJobs` in `job_loader.go` currently loads all jobs (add filter at load to ignore marked records for faster init).
