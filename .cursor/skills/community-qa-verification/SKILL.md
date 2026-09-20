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
2. **Automated Test Execution**:
   - Run targeted test suites specified by test cases.
   - Confirm 100% pass rate and verify that tests genuinely evaluate criteria conditions.
3. **Done-Gate Attestation**:
   - Prevent premature state promotion; verify that `InvariantGate` assertions clear before transitioning from `PlaneDraft` to `PlanePromoted`.

