# CAS mutation shockwave (delete/update ≡ promote/demote)

**Last Verified:** 2026-08-31


**Status:** Design contract (2026-08-19). Implementation is the next unlocked column — do not mint onto `PRI-COMMUNITY-FORK-PREPARE-001` (execution-locked).  
**Parent pipeline:** [`KERNEL_MUTATION_PIPELINE.md`](./KERNEL_MUTATION_PIPELINE.md)  
**Observation taxonomy:** [`KERNEL_COHERENCE_AND_REF_GRAPH.md`](./KERNEL_COHERENCE_AND_REF_GRAPH.md)  
**Archive ≠ erase:** [`CRUD_BUCKETING_ARCHIVING_REQUIREMENTS.md`](./CRUD_BUCKETING_ARCHIVING_REQUIREMENTS.md)  
**Event rhythm:** [`EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md`](./EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md) (causality over clocks)

**TRACK:** `PRI-CAS-MUTATION-SHOCKWAVE-001` (objectify this session). Related: `BLI-1786698242916116000-e4b855ed` (do not delete archived criteria_refs to “heal” complete BLIs), `BLI-1786387465409533000-45bd780c` (GhostRef vs CacheLag).

**Status-transition planes and which listener owns each catalyst:** [LIFECYCLE_SHOCKWAVE_MAP.md](./LIFECYCLE_SHOCKWAVE_MAP.md) (class catalog). **PRI hop tables:** [lifecycle_shockwave/kind_priority_plan.md](./lifecycle_shockwave/kind_priority_plan.md). This CUD contract is the erase/update plane; the catalog is the status-transition plane.

## Why GhostRef is not “cache lag”

`system check` GhostRef means: a referrer names an ID that **Exists is false** (CAS index-first, no directory scan). That is **graph debt**.

The 2026-08-19 cohort was **unpaired git deletes** of criteria/ATK CAS blobs (no matching new-hash add, no `zqk object delete`). Refreshing caches cannot resurrect a missing blob. `heal-dangling --apply` on that cohort would **strip archived CRIT lineage** under complete BLIs — forbidden.

CacheLag remains a real class (target Exists, id-cache miss). Do not collapse the two.

## What already works (promote / demote)

Status hops go through `kernel.cas_object_transition`. On status save, `storage.SetLifecycleHookHandler` (`cmd/zqk/app/root.go`):

1. Appends the transition to the lifecycle WAL.
2. **Synchronously** `lifecycle.ApplyDependencyRefEvents` — one hop up (outbound refs) and one hop down (reverse-index dependents).
3. CLI may `EmitOperationalSync` so a short-lived process cannot exit before the parent edge applies.

That is the integrity-pure pattern: **fire the event, let authorized shockwave run inside the transition’s consistency boundary.** Non-status updates and erase do **not** take this path today (`object_storage_file_update.go` only hooks when status changed; delete is a different CLI membrane). Outbound hops stamp `edge_role` (`membership` \| `composition` \| `associate`) plus `field` so authorized ops can refuse unlinking archived composition lineage.

## Contract (treat CUD as transition-class)

Every logical mutate is a pipeline intent. Shockwave is not a best-effort afterthought.

| Intent | Today | Required |
|--------|--------|----------|
| `kernel.cas_object_transition` | Lifecycle hook + `ApplyDependencyRefEvents` | Keep; this is the template |
| `kernel.cas_object_update` | Status-only hook; field/ref edits skip shockwave | Emit the same dependency-ref event for **old and new** ref sets (fromState/toState may be equal; trigger is `update`) |
| `kernel.cas_object_erase` | CLI fail-closed `--unlink-references` / `--cascade`; git can still unpaired-delete | Erase **is** a transition: DECIDE → linger → shockwave → COMMIT blob GC. Git unpaired delete remains blocked (`check-process-cas-commit-with-work.sh`) |
| `kernel.cas_object_create` | Materialize | Already a membrane; inbound refs to not-yet-visible IDs stay fail-closed at check time, not a silent Exists-true |

**Rule:** if a change would interrupt event propagation or change graph behavior, **still fire the event**. Refuse in DECIDE; do not skip COMMIT’s shockwave. Authorized operations are only those the **context** allows (kind lifecycle, inbound-ref roles, break-glass with reason).

## Linger (annotate, then prove, then drop)

Hard-delete must not make Exists flip false while dependents and caches are still converging.

1. **DECIDE** computes an erase/update plan (unlink, refuse, or break-glass).
2. **Linger:** annotate the index (and optionally the object) `mutation_pending` / `erase_pending` with the plan + event id. **Exists stays true.** Check classifies this window as **CacheLag / pending mutation**, not GhostRef.
3. **Shockwave** runs authorized ops in-graph (unlink live inbound refs, or refuse if live refs remain).
4. **FINALIZE / `kernel.cas_blob_gc`:** drop the blob and clear the linger **only after** reverse-index dependents are closed **or** remaining inbound refs are themselves delete-worthy (below).

This is the “object lingers while the system validates the operation” idea. Prefer an **index tombstone** first (no spec field required); object-field annotation is a later overlay if operators need `object get` to show pending erase.

Do **not** invent a second object store. Linger is the same CAS id in a pending-erase epoch.

## Delete-worthiness is decided at `archived`

Archive is the antithesis of GhostRef: the blob **must remain** so complete parents can keep historical `criteria_refs` (`go_validator.go` — archived-only refs satisfy the complete-BLI CRIT gate).

**Delete-worthy** (computed when the object **arrives at** `archived`, stored as derived/annotated fact, recomputed if inbound graph changes):

- Kind is allowed to erase after archive (critical kinds stay fail-closed per CRUD policy unless break-glass).
- **No live inbound refs.** Live = referrer role is not `terminal` (and not itself `erase_pending` with a closed plan).
- Every remaining inbound ref is **also** `archived` (or other terminal+archive) **and** delete-worthy (bounded recursion; cycle → refuse).
- Unlink plan is empty **or** already applied.

`zqk object delete` on a non-delete-worthy id **refuses** even with `--unlink-references` if unlink would steal lineage from a non-archived parent (the 2026-08-19 heal-dangling footgun). `--cascade` is the same graph walk, not a silent rm.

Git delete of a hash-named instance YAML is **not** a substitute for this hop. The unpaired-delete commit gate is the VCS membrane; this contract is the kernel membrane.

## Authorized shockwave by context (examples)

| Trigger | Authorized ops | Forbidden |
|---------|----------------|-----------|
| BLI → `complete` | Last-child PRI ledger shrink; plan may auto-complete; **work-envelope autofill** (`started_at`/`completed_at`/`actual_effort`) for `effort_aware` kinds — [WORK_ENVELOPE_AND_EFFORT_FACETS.md](./WORK_ENVELOPE_AND_EFFORT_FACETS.md) | Unlink archived CRITs |
| Object → `archived` | Recompute delete-worthiness; optionally enqueue blob-gc if worthy | Immediate Exists-false |
| Erase DECIDE `erase_unlink` | Unlink **live** inbound refs only | Unlink archived lineage under complete parents |
| Field update of `*_refs` | Dependency-ref events for removed and added IDs | Persist new refs to IDs that do not Exist (unless linger create) |
| Git unpaired CAS D | Pre-commit refuse | `git add -A .zqk/process/` as “cleanup” |

## What this is not

- Not “refresh the cache more.” GhostRef after unpaired delete is absent bytes.
- Not `heal-dangling --apply` as default remediation for archived CRITs.
- Not a new pipeline kind. Intents already exist; **FINALIZE must actually run the shockwave** for update/erase the way transition already does.
- Not stuffing the locked community-fork column.

## Verification (when implementing)

- Promote/demote tests in `pkg/lifecycle/dependency_propagation_test.go` stay the behavioral template.
- Add erase/update cases: linger Exists-true during pending; GhostRef only after FINALIZE with closed graph; refuse erase of referenced archived CRIT under complete BLI.
- Keep `scripts/check-process-cas-commit-with-work.sh` unpaired-delete fail-closed. `ZQK_ALLOW_UNPAIRED_PROCESS_CAS_DELETE=1` still fails when deleted IDs remain in on-disk `*_refs` (`scripts/cas_unpaired_delete_inbound_refs.py`).
- Package gate: `zqk scheduler scan-tests --package ./pkg/lifecycle` (and `./pkg/kernelcas` / `./cmd/zqk/object` as touched).
