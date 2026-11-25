---
name: community-qa-verification
description: REQ/CRIT/BLI traceability and VDS done-gates for community.
---

# QA Audit and Done-Gate Verification

## Objective
Ensure that all claimed completions are backed by deterministic automated test evidence and clear invariant evaluations.

## Verification Protocol
1. **Traceability Verification**:
   - Confirm that every BacklogItem maps to at least one Requirement and one Acceptance Criterion.
   - Use `zqk test discover` and `zqk test bind` (plus `zqk test dashboard`) to verify deterministic `TST-* -> CRIT-* -> REQ-*` lineage.
2. **Automated Test Execution & Rubrics**:
   - Run targeted test suites via `zqk test run --all` and declarative rubrics via `zqk eval validate|run|matrix`.
   - Confirm 100% pass rate and verify that tests genuinely evaluate criteria conditions.
3. **Done-Gate Attestation**:
   - Use native `zqk gate stage|review|approve|reject` for SHA-256 human-in-the-loop sign-off and tamper-evident receipts.
   - Prevent premature state promotion; verify that `InvariantGate` assertions clear before transitioning from `PlaneDraft` to `PlanePromoted`.

