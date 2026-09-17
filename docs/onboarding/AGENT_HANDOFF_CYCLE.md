# ZQK AI Agent Hand-Off Guide

**Date:** 2026-08-07
**Target Audience:** Next cycle of Antigravity / Agentic Workers

## 1. System State & Context

The system recently underwent a series of fixes to repair CLI command timeouts and failing CAS fingerprints during bundle convergence testing. The next active priority plan is currently prioritized for MMORCH leak remediation.

*   **Current Active Plan:** `PRI-1786121461227090000-f8c05f3c` (MMORCH Leak Remediation — Aug 7 overnight control-plane cracks)
*   **Backlog State:** 9 planned items remaining in the active plan.

## 2. Recent Accomplishments (Current Cycle)

*   **CLI Stabilization:** Addressed high-failure commands and frequent timeouts (`system state-restore`, `agent validate`, `agent sync-loop`, `object delete`).
*   **Test Fingerprint Convergence:** 
    *   Fixed a bug in `cmd/zqk/utility` where scenario dependency sorting was failing due to improperly generating `ACC-*` IDs for `account` objects.
    *   Fixed `pkg/storage` CRUD pagination tests (`TestAllKindsCRUD/workstream`) which were failing reference validation due to an outdated reference to `account:testuser` instead of `ACC-TESTUSER`.
    *   Both test suites now pass completely, clearing out the `e90b38e64908f8e4c5410e31` and `5047c4e5f466fc62a26b76dc` CAS failing fingerprints.
*   **State Promotion:** Advanced and completed `BLI-1785899470673804000-a8ada88e` (Re-align MMORCH CVS after timeout) with committed fixes.

## 3. Pending Inbox & Watch Items

The `antigravity-1` inbox currently has **6 unacknowledged messages** from `cursor-composer` requesting follow-ups on ATKs that are sitting in `pending_verification`.

These are the pending ATKs you must follow up on:
*   `ATK-1786101103932765000-891b381b`
*   `ATK-1786090644619546000-c935f6f1`
*   `ATK-1786096841578125000-9225ffbc`
*   `ATK-1786091468566449000-eab9e756`
*   `ATK-1786093303954388000-def10a7a`
*   `ATK-1786100112051042000-47753e85`

## 4. Next Actions for Incoming Agents

1.  **Acknowledge Inbox:** The immediate workflow hint is `ack_then_continue`. Start by acknowledging the pending messages in the inbox.
2.  **Verify ATKs:** Check the status of the 6 ATKs listed above. Ensure that the scheduler has processed them and they have passed their verification steps. If validation failed, wake the appropriate agent or fix the underlying issues.
3.  **Resume Active Plan:** Once the ATKs are cleared, claim the next planned backlog items under `PRI-1786121461227090000-f8c05f3c` (MMORCH Leak Remediation).
4.  **MMORCH Subagent Dogfooding:** Remember the project directive to *dogfood the MMORCH operation*. Do not hesitate to spawn subagents (`invoke_subagent` / `zqk agent orchestrate`) to parallelize work where appropriate, while keeping the hourglass interrupters on (`--on-validation-failure wake`).
