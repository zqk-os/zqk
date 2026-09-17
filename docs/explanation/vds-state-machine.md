# Explanation: Version-Driven State (VDS) Architecture

How ZQK ensures deterministic, verifiable transitions across system object lifecycles.

---

## 1. Why State Machines Fail in Complex Software
In typical development trackers (Jira, GitHub Issues), status transitions are arbitrary strings updated without programmatic verification. An issue can be marked "Done" even if tests fail, criteria remain unmet, or no commits exist.

---

## 2. The VDS Solution: Precondition Graphs & Auditor Gates
In ZQK, every object kind has a declared lifecycle schema. Transitions are governed by:
1. **Directional Transition Graph:** An object cannot jump arbitrarily; it must traverse defined graph edges (e.g. `planned` → `testing` → `in_progress` → `complete`).
2. **Precondition Validation:** Before a hop is permitted, `zqk object promote` evaluates typed preconditions (e.g., presence of criteria links, valid personas, non-empty commit hashes).
3. **Auditor Gates (`VerifyComplete`):** Transitions to terminal success (`complete`) invoke verification gates that inspect test execution evidence, CAS single-blob invariants, and sign-offs.
