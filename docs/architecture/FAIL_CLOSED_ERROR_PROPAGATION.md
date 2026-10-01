# Fail-Closed Error Propagation and Elimination of Swallowed Errors

## Overview

Historically, certain critical paths across synchronization loops and persistent index caches assigned error returns to blank identifiers (`_ = sp.Update(...)`) or recorded errors through mechanical logging helpers (`logging.LogSwallowedError(...)`) while allowing execution to proceed.

This failure mode created severe silent corruption risks (`TDE-F-CQ-001`):
1. **Silent Lock Timeout Failures**: In `pkg/storage/reverse_reference_index_persist.go`, lock acquisition timeouts during cache loading or saving were swallowed, leading callers to assume clean persistence when in fact cache updates failed.
2. **Transaction Rollback Loss**: In `cmd/zqk/agent/sync_loop.go`, failure to create audit stamps or apply mutations silently ignored rollback errors.

This specification documents the fail-closed error propagation architecture implemented to eradicate silent failure modes.

---

## Architectural Principles

### 1. Fail-Closed Concurrency Locks
When acquiring read or write locks with timeouts via `concurrency.WithLockTimeout` or `concurrency.WithRLockTimeout`:
- Timeouts MUST return an explicit error wrapped with context.
- Under no circumstance may lock acquisition timeout errors be swallowed or substituted with boolean success flags.

```go
var errLock = concurrency.WithLockTimeout(&r.mu, ctx, nil, func() error {
    // critical section
    return nil
})
if errLock != nil {
    return false, errfmt.Errorf("reverse reference index lock timeout during load: %w", errLock)
}
```

### 2. Explicit Transaction Error Joining
When performing atomic operations within transactional storage scopes:
- Rollback operations on failure MUST be checked.
- If rollback encounters an error, the primary failure and the rollback failure MUST be joined via `errors.Join(primaryErr, rollbackErr)`.

### 3. Structured Observability via Fluent Logging
Where non-fatal events require warning logs, errors MUST be attached via `.WithError(err)` on the fluent logger builder (`logging.FluentEvent(logger).Warn(...).WithError(err).Log()`) rather than swallowing or lossy string formatting.

---

## Verification and Test Coverage

1. `TestReverseReferenceIndex_SaveCache_LockContention_PropagatesError`: Verifies that lock acquisition timeouts in `ReverseReferenceIndex.SaveCache` fail closed and return an explicit error containing `lock timeout`.
2. `TestApplyStateMutation_ErrorPropagation`: Verifies that mutation failures in the agent sync loop fail closed and propagate non-nil errors to the coordinator.
