# Family: record

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)

## Overview

Linear append-mostly records: audit_event, audit_event_aggregation, audit_aggregation_metric, change_journal_entry, code_quality_metric, import_tracking, qa_success, component, display.

Typical machine: `pending`/`proposed` → `completed`/`failed`/`reverted` → `archived`, plus system `error`. There is often **no `active`**. Occupancy is “this blob’s outcome,” not a Gantt column.

## Occupancy

| Pattern | Roles | Planes |
|---------|-------|--------|
| pending → completed \| failed | `realign` → `terminal` | **A + B**; instantiate is the exam |
| `error` | `halted` | Recover to origin or fail closed |
| `archived` | `terminal` + archive | **B + CAS linger** |

No check valve. No compiled Plane C target. Auto-only edges here are outcome stamps (aggregation completed), not child-status shockwave.

## Kind specialty (Q5 only)

| Kind | Specialty |
|------|-----------|
| `audit_event` | Integrity event; failed/reverted are terminal outcomes, not halt resume. |
| `change_journal_entry` | Runtime-delta cousin; do not treat as `stream_current` SSOT for scheduler_job. |
| `*_metric` | Instance builders + allowed statuses only (`completed`/`archived`/`error` families). |
| `component` / `display` | Extensible UI records; lifecycle is publish/archive, not execution lock. |
| `qa_success` | Signed QA-auditor PASS token (`item_id` + ECDSA P-256). Spec exists with `fields: {}` / `extends: null` — not a missing file. Unique vs unsigned `audit_event`. Keep until fields are declared or a signed `audit_event` path exists. Do not bulk-delete ~778 `QAS-*`. TRACK: `BLI-CEF-QA-SUCCESS-SPEC-001`. |

Do not add `on_dependent_status` so that an audit_event completes a priority_plan.
