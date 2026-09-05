# Family: membrane

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Status:** Active  
**Hub:** [LIFECYCLE_SHOCKWAVE_MAP.md](../LIFECYCLE_SHOCKWAVE_MAP.md)  
**Policy narrative:** [POLICY_LIFECYCLE.md](../POLICY_LIFECYCLE.md)

## Overview

Kinds whose `active` means **enforced / membrane-live**, role `enforced`: **policy**, **role**. They do not occupy a Gantt column. `RoleProgressRank` returns ok=false — do not let plan ranking treat `POL-*` / `ROL-*` `active` as shovel-ready.

Start every campaign with policies. A thin machine is correct; a missing *exam* (“is this policy live?”) on other kinds’ hops is not.

## Occupancy (policy)

| Value | Role | Catalyst |
|-------|------|----------|
| `draft` / `under_review` | `realign` | Origin / submit |
| `active` | `enforced` | Approval or `effective_date` (auto `under_review → active`) |
| `deprecated` / `superseded` | `realign` | Withdraw; supersede is a specialty exam (successor ref) |
| `archived` | `terminal` | Manual |

No `execution_locked`. Policies gate other objects; they do not lock a work column.

## Class hops

- `realign → enforced`: qualifying exam. **A + B**; policy also has an **auto** effective-date hop (must become a recognized token, not fail-open English).
- **No** check valve. **No** `on_dependent_status` compiled target.
- Other kinds consult this family on **their** Plane A exams (“policy X is `enforced`”).

## Shockwave

Exam-only. Plane C may walk a policy’s neighbors and no-op. Do not compile PRI-style child lock onto a policy.

## Kind specialty (Q5 only)

| Kind | Specialty |
|------|-----------|
| `policy` | `related_patterns` / successor on supersede; system-awareness review in POLICY_LIFECYCLE.md. TRACK: `BLI-CEF-R26-POLICY-PRI-EXAM-001`. |
| `role` | Same `enforced` occupancy; thinner graph. Halt/error is repair, not a Gantt pause. |
