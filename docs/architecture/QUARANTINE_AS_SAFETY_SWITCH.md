# Quarantine as Safety Switch

**Last Verified:** 2026-08-31


**Status:** Design  
**Purpose:** Define quarantine as a safety and audit mechanism (not a staging area for deletion), and add a targeted auto-fix option that applies resolutions only to quarantined items to reduce re-validation overhead.

---

## 1. Purpose of Quarantine

Quarantine is a **safety switch**: it isolates objects that have known violations or integrity issues so that:

- **Re-validation overhead is reduced** – Items that have already been validated and have not changed since passing validation are not re-validated. Only quarantined items (known failures) need to be considered for fix.
- **Data is protected** – The actual object is isolated (and possibly moved) so lifecycle or validation logic cannot corrupt or incorrectly modify it until a fix has been applied and verified.
- **Audit trail is preserved** – Like an auto-fix report, but persisted: we store the original violations, the remedy (e.g. auto-fix command), the outcome of running the fix, and metrics/cleanup so we have a full record.

Quarantine is **not** used for:

- Disposable duplicates (e.g. old hash-named CAS files). Those are **deleted** (or renamed to `.tmp` then deleted). See INTEGRITY_RESOLUTION_PLAN and the orphan-cleanup design; no quarantine bloat for hash-duplicates.

---

## 2. What We Store (Audit Trail)

For each quarantined item we maintain:

| Data | Description |
|------|-------------|
| **Original violations** | The validation/integrity issues that caused the object to be quarantined. |
| **Remedy** | The auto-fix command (or other remedy) that was run to address the violation. |
| **Outcome** | Result of running the remedy (success, failure, partial). |
| **Metrics / cleanup** | Counts, timestamps, and any cleanup actions (e.g. files removed, index updated). |

This gives a full forensic trail and supports “fix only what’s in quarantine” without re-scanning the whole tree.

---

## 3. When to Quarantine

- **Lifecycle or validation issues** where we are not confident to auto-fix immediately – e.g. reference/lifecycle violations that need a deliberate fix. The object is moved aside so it is not re-validated as a normal object and so bad modifications are prevented until a fix is applied.
- **Never for system-generated objects** – see Invariant below.

Scope and exact triggers (which issue types lead to quarantine vs. delete vs. in-place fix) are to be defined in implementation; this doc sets the principle.

---

## 4. Invariant: System-Generated Objects Never Have Violations

**It must never be possible for system-generated objects to have a violation or integrity issue.**

- **Implication:** System-generated kinds are never quarantined. They are produced by trusted paths (scheduler, retention, aggregation, CLI system commands) and must satisfy integrity by construction (e.g. instance builders, CAS index consistency, bypass kinds in blocking check config).
- **Enforcement:**
  - **Bypass kinds** (from blocking check config) are excluded from blocking checks and from flows that would quarantine (e.g. Stale CAS cleanup **deletes** duplicates for these kinds; they are not moved to quarantine).
  - **Generation path:** All creation/update of system-generated objects goes through builders and storage paths that maintain consistency (no direct YAML edits under `docs/process/`).
- If a “violation” or “integrity issue” ever appears for a system-generated object, that is a **bug in the generator or checker**, not a case for quarantine. The fix is to correct the generator or the validation logic, not to quarantine the object.

---

## 5. Targeted / Advanced Auto-Fix (Quarantine-Only)

To further reduce the need to re-validate items that have already been validated and have not changed:

- **Option:** A **targeted** (or **advanced**) auto-fix mode that runs resolutions **only for items currently in quarantine**.
- **Behavior:**
  - Input: the set of objects (or references) that are in quarantine, plus their stored violations and remedies.
  - Action: Run the stored remedy (e.g. fix command) for each quarantined item; record outcome and update the audit trail.
  - No full-tree validation is required for this pass – we only re-run fixes for known-failed items. Items that passed validation and have not changed are not re-validated.
- **CLI / UX:** To be designed (e.g. `zqk system check --auto-fix --quarantine-only`, or a dedicated `zqk system fix-quarantine` command). The important point is that there is an explicit path for “fix only quarantined” so that normal check runs do not re-validate the entire tree when the user only wants to resolve quarantined items.

---

## 6. Location and Schema (High Level)

- **Default root:** `<project-root>/.zqk/system-health/quarantine/`
- **Per-item (or per-entry):** A quarantined object is isolated (e.g. moved under a quarantine subfolder by kind/object ID or by violation type). Alongside it (or in a manifest), we store the audit record: original violations, remedy, outcome, metrics.
- **Detailed schema** (folder layout, manifest format, and how “quarantine-only” auto-fix discovers entries) is left to implementation; this doc defines the intent and the invariant.

---

## 7. Relationship to Existing Behavior

| Area | Current / recent behavior | Alignment with this design |
|-----|---------------------------|----------------------------|
| **Hash-duplicates / Stale CAS** | Duplicates are **deleted** (no quarantine). Orphan CAS files are renamed to `.tmp` then deleted. | Correct: we do not use quarantine as a staging area for deletion. |
| **Quarantine folder** | Today `.zqk/system-health/quarantine/` may still contain legacy hash-duplicate moves; repair-cas-corruption can move corrupt files there. | Going forward, quarantine is used only as the safety switch above (audit trail, isolate, fix-only-quarantine option). Legacy contents can be cleaned via `cleanup-quarantine` or manual delete. |
| **System-generated kinds** | Bypass kinds; hash-duplicate cleanup deletes (never quarantines) for these kinds. | Aligns with invariant: system-generated objects must not have violations; they are never quarantined. |
| **Auto-fix** | `check --auto-fix` runs in-place fixes and Stale CAS cleanup. | Targeted auto-fix adds a **quarantine-only** mode so resolutions can be applied only to quarantined items without re-validating the full tree. |

---

## 8. References

- [INTEGRITY_RESOLUTION_PLAN.md](../../cmd/zqk/system/INTEGRITY_RESOLUTION_PLAN.md) – Stale CAS cleanup, auto-fix wiring, resolution summary.
- [QUARANTINE_AND_ANALYSIS.md](../process/system-health/QUARANTINE_AND_ANALYSIS.md) – Quarantine folder location, report, cleanup, and dashboard data.
- [AUTO_FIX_PATTERN.md](../process/architecture/AUTO_FIX_PATTERN.md) – Auto-fix layers and fix-command execution.
- [BYPASS_KIND_STORAGE.md](./BYPASS_KIND_STORAGE.md) – For bypass kinds, avoid CAS overhead; use aggregate storage (same file for multiple events/objects) for efficiency.
- Blocking check config / bypass kinds: `pkg/storage/blocking_check_config.go`.
