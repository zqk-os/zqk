# Fail-Closed Error Propagation & Transaction Resilience

**Document ID:** `DOC-FAIL-CLOSED-ERROR-PROPAGATION-001`  
**Status:** Approved / Enforced  
**Technical Debt Reference:** `TDE-F-CQ-001`  
**Priority Plan:** `PRI-SYNC-LOOP-ERROR-PROPAGATION`  

---

## 1. Overview

Silent error swallowing via blank identifiers (`_ = err`) and mechanical logging (`logging.LogSwallowedError`) compromises data consistency, masks concurrency deadlocks, and allows corrupted state mutations to proceed.

This standard establishes strict invariants for transaction commits, state transitions, and concurrency locks across the Knowledge Kernel and CLI surfaces.

---

## 2. Core Invariants & Rules

### Rule 1: Fail-Closed Transaction Rollbacks
- All database and storage transactions (`sp.BeginTransaction(ctx)`) must be guarded by explicit error handling.
- If creating idempotency audit stamps or mutating nodes fails:
  1. The transaction must immediately execute `tx.Rollback(ctx)`.
  2. Any error returned by `tx.Rollback` must be joined with the primary failure via `errors.Join(primaryErr, rollbackErr)`.
  3. The combined error must be propagated to the caller without proceeding to subsequent steps.

### Rule 2: Synchronous Status Transition Integrity
- Status transitions on tasks and kernel entities must never discard mutation errors.
- If updating task status to `in_progress`, `blocked`, `complete`, or `failed` fails, the execution loop must abort or surface the error, rather than continuing as if the transition succeeded.

### Rule 3: Storage Lock Timeouts Must Fail Closed
- When acquiring shared or exclusive locks (e.g. `concurrency.WithLockTimeout`, `concurrency.WithRLockTimeout`), lock timeouts must return explicit errors.
- Cache loading or saving operations must never return `true, nil` or silent success when lock acquisition times out.

---

## 3. Verification

Verified by automated test suites:
- `cmd/zqk/agent/sync_loop_test.go` (`TestApplyStateMutation_ErrorPropagation`, `TestSyncLoop_MaxVerificationAttempts`)
- `pkg/storage/reverse_reference_index_test.go` (`TestReverseReferenceIndex_SaveCache_LockContention_PropagatesError`, `TestReverseReferenceIndex_GetDependentsWithError_Contention`)
