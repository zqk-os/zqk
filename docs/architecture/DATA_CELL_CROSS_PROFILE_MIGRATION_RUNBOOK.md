> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Cross-profile data cell migration — operator runbook

**Last Verified:** 2026-08-31


**Audience:** Operators changing a kind’s **`storage_profile`** along the escalation chain (see [DATA_CELL_MODEL.md](./DATA_CELL_MODEL.md)).  
**Scope:** Planning, backups, eligibility checks, spec-plane flips, and rollback — **not** a fully automated relocator for every CAS/stream layout.

**Tracked:** BLI **`[REDACTED-ID]`**; criterion **`CRIT-DATACELL-002`** (transport and policy at the membrane).

---

## 1. Canonical profile chain

Code: [`pkg/datacell.OperatorProfileMigrationChain`](../../pkg/datacell/profile_migration_chain.go) matches profile contracts ([`profile_contracts.go`](../../pkg/datacell/profile_contracts.go)): **light_file → cas_entity → stream**.

Each profile has distinct **`migration_mode`**, **`membrane_transport`**, and **`membrane_policy`** strings (surfaced as JSON fields on **`zqk system data-cells --json` (PRUNED)** — CRIT-DATACELL-002).

---

## 2. filesystem layout guardrails

Do **not** invent unbounded flat directories when exporting or staging migration artifacts. Follow [**FILESYSTEM_DATA_LAYOUT.md**](./FILESYSTEM_DATA_LAYOUT.md) (≤100 top-level entries per populated directory; hash/date/job bucketing). CAS shards under **`docs/process/`** already use content-addressed paths; preserves that structure.

---

## 3. Phased migration (M4-style sequence)

Treat these as **idempotent blocks**: each step should be safely repeatable from backup after a rollback.

| Phase | Goal | Actions |
|-------|------|---------|
| **P0 Inventory** | Know current cell identity | **`zqk system data-cells` (PRUNED)** (table / `--json`); note **`kind`**, **`storage_profile`**, **`primary_path`**, **`operational_envelope`**. Optional: **`zqk system data-cells --json --json-envelope` (PRUNED)** for drift (`stream_stewardship_drift`), test-bundle health summary. |
| **P1 Backup** | Recoverable baseline | Snapshot repo branch or archive **`docs/process/`**, **`.zqk/`** materialized indexes (`spec_index.json`), and stream/CAS dirs your kind touches (see STREAM_STORAGE / CAS docs). Record git SHA. |
| **P2 Dry eligibility** | Deploy gates green | **`zqk system generate-spec-index` (PRUNED)** / **`zqk system validate`** patterns your pipeline uses; resolve **high_volume_kinds** ↔ **spec_index** drift before flipping a stream profile (`ValidateHighVolumeStreamKindsMatchSpecIndex`). |
| **P3 Physical prep** | Target layout ready | Allocate stream segments / CAS shards per [FILESYSTEM_DATA_LAYOUT.md](./FILESYSTEM_DATA_LAYOUT.md); **do not** dump unlimited files into one folder. Steward/rebuild steps stay in stream + storage layers (see STREAM_STORAGE.md). |
| **P4 Spec-plane flip** | Declare new profile | Edit **`object_specs`** (or **`zqk system update-specs`** field workflow) so **`storage_profile`** matches target; regenerate **`pkg/specbuilder`** artifacts as required by your pipeline; **`generate-spec-index`** strict mode. |
| **P5 Validate** | Prove stewardship matches | **`zqk system validate`**, **`zqk scheduler scan-tests`** on impacted packages (`pkg/datacell`, **`pkg/storage`**, **`cmd/zqk/system`** as appropriate — no foreground `./...` sweeps per workspace posture). **`zqk scheduler scan-tests --load-bundles data-cell-stream-organism,data-cell-stream-organism-pkg-storage`** when datacell / stream stewardship paths change. |
| **Rollback** | Undo bad flip | Restore backed-up specs + **`spec_index.json`**; revert git; re-run **`generate-spec-index`** / **`validate`**. |

**Bulk deletes:** Use **`zqk object bulk`** patterns when removing many instance IDs — do not loop per-delete in scripts (Cursor rule **`object-bulk-delete-not-loop.mdc`**).

---

## 4. Automated checks (fixture / unit)

Logical chain and membrane fields are pinned in **`pkg/datacell`** (`TestProfileContract_migrationChain_*`, **`TestContractForProfile_KnownProfiles`** for CRIT-DATACELL-002). Same chain as [**`OperatorProfileMigrationChain`**](../../pkg/datacell/profile_migration_chain.go).

**Runnable probes (foreground, bounded timeouts):**

| Probe | Command |
|-------|---------|
| Profile escalation + distinct migration/write modes | `go test ./pkg/datacell -timeout 60s -count=1 -run 'TestProfileContract_migrationChain'` |
| Membrane contract strings non-empty (**CRIT-DATACELL-002**) | `go test ./pkg/datacell -timeout 60s -count=1 -run 'TestContractForProfile_KnownProfiles'` |
| Stream → CAS migrate command (fixture / in-process) | `go test ./cmd/zqk/system -timeout 90s -count=1 -run 'TestMigrateLegacyToStream_(dryRun|applyRemoveLegacy)_inProcess'` |

**Discovery:** **`zqk system data-cells --json` (PRUNED)** rows include **`membrane_transport`** and **`membrane_policy`** from [`profile_contracts.go`](../../pkg/datacell/profile_contracts.go).

---

## 5. Deferred / future work

Full **automatic** CAS→stream movers, coordinator-enqueue migration jobs, and admin binaries remain **follow-on** BLIs unless explicitly scheduled; this runbook satisfies **operator-facing** phased migration documentation for BLI closure.
