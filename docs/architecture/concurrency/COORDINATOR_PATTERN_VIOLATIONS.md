# Coordinator Pattern Violations

**Last Verified:** 2026-08-31


This document lists all areas in production and test code that should be using the coordinator pattern but are currently using direct logging or output.

## Policy

**All goroutine lifecycle events, operation start/complete events, and user-facing progress messages MUST use the coordinator pattern.**

See: `docs/process/architecture/GOROUTINE_ARCHITECTURE_POLICY.md`

## Production Code Violations

### Critical: Direct fmt.Fprintf for Progress/Status Messages

#### cmd/zqk/system/async_check_helpers.go
- **Line 82-84**: `fmt.Fprintf(os.Stderr, "Validation cache cleared...")` - Cache clear status
  - **Should use**: Coordinator event for cache operations
- **Line 344**: `fmt.Fprintf(os.Stderr, "Enqueued %d objects...\r")` - Progress update
  - **Should use**: Coordinator progress event
- **Line 354-355**: `fmt.Fprintf(os.Stderr, "\nEnqueued %d objects total...")` - Completion summary
  - **Should use**: Coordinator completion event

#### cmd/zqk/system/show_validation_progress_helpers.go
- **Line 315**: `fmt.Fprintf(os.Stderr, "\nValidation timeout after %v\n")` - Timeout message
  - **Status**: Has coordinator event via `emitCheckTimeoutEvent()` but also direct output
  - **Should use**: Only coordinator event, remove direct output
- **Line 406-407**: `fmt.Fprintf(os.Stderr, "\nValidation completed: %d/%d...")` - Completion message
  - **Status**: Has coordinator event but also direct output
  - **Should use**: Only coordinator event
- **Line 415**: `fmt.Fprintf(os.Stderr, "\n%s\n", vpc.Metrics.String())` - Metrics output
  - **Should use**: Coordinator event with metrics data
- **Line 721**: `fmt.Fprintf(os.Stderr, "\rProgress: [%s] %.1f%%...")` - Progress bar
  - **Should use**: Coordinator progress event
- **Line 941-942**: `fmt.Fprintf(os.Stderr, "\nValidation completed: %d/%d...")` - Completion message
  - **Status**: Has coordinator event but also direct output
  - **Should use**: Only coordinator event
- **Line 949**: `fmt.Fprintf(os.Stderr, "\n%s\n", vpc.Metrics.String())` - Metrics output
  - **Should use**: Coordinator event with metrics data

#### cmd/zqk/scheduler/submit.go
- **Line 88**: `fmt.Fprintf(os.Stderr, "Warning: Failed to capture diagnostics...")` - Warning message
  - **Should use**: Coordinator warning event
- **Line 267-275**: Multiple `fmt.Fprintf(os.Stdout, ...)` - Job submission output
  - **Should use**: Coordinator events for job lifecycle
- **Line 314-328**: Multiple `fmt.Fprintf(os.Stdout, ...)` - Job submission output
  - **Should use**: Coordinator events for job lifecycle

#### cmd/zqk/scheduler/scheduler.go
- **Line 1066**: `fmt.Fprintf(os.Stderr, "ERROR: Failed to check scheduler status...")` - Error message
  - **Should use**: Coordinator error event
- **Line 1073**: `fmt.Fprintf(os.Stderr, "ERROR: Failed to create storage factory...")` - Error message
  - **Should use**: Coordinator error event
- **Line 1094**: `fmt.Fprintf(os.Stderr, "ERROR: Failed to query scheduler jobs...")` - Error message
  - **Should use**: Coordinator error event
- **Line 1107-1112**: Multiple `fmt.Fprintf(os.Stderr, ...)` - Scheduler daemon status messages
  - **Should use**: Coordinator operational events
- **Line 1128-1129**: `fmt.Fprintf(os.Stderr, ...)` - Keep-alive stale warnings
  - **Should use**: Coordinator warning events

#### cmd/zqk/scheduler/show_history_helpers.go
- **Line 141**: `fmt.Fprintf(os.Stderr, "Querying up to %d audit events...\n")` - Progress message
  - **Should use**: Coordinator progress event
- **Line 162**: `fmt.Fprintf(os.Stderr, "Found %d audit events, filtering...\n")` - Progress message
  - **Should use**: Coordinator progress event

#### cmd/zqk/root.go
- **Line 586-594**: Multiple `fmt.Fprintf(os.Stderr, ...)` - Scheduler daemon warning
  - **Should use**: Coordinator warning event

### Critical: Direct Logger Calls for Lifecycle Events

#### cmd/zqk/system/async_check_helpers.go
- **Line 273-276**: `logger.Info("Deduplicated files by object ID...")` - Operation status
  - **Should use**: Coordinator event for discovery completion

#### cmd/zqk/system/check_all_helpers.go
- **Line 217**: `logger.Info("Cache refresh completed...")` - Cache operation completion
  - **Should use**: Coordinator event for cache operations
- **Line 219**: `logger.Info("Cache refresh completed...")` - Cache operation completion
  - **Should use**: Coordinator event for cache operations

#### cmd/zqk/system/check_impl.go
- **Line 754**: `logger.Warn("Cache rebuild completed but cache is empty...")` - Cache operation warning
  - **Should use**: Coordinator warning event
- **Line 756**: `logger.Debug("Cache rebuild completed successfully...")` - Cache operation completion
  - **Should use**: Coordinator completion event

#### cmd/zqk/system/init_impl.go
- **Line 307**: `logger.Info("Snapshot initialization completed successfully...")` - Operation completion
  - **Should use**: Coordinator completion event
- **Line 441**: `logger.Info("Object restoration completed...")` - Operation completion
  - **Should use**: Coordinator completion event

#### cmd/zqk/system/check_async_baseline.go
- **Line 86**: `logger.Info("Starting async validator baseline comparison...")` - Operation start
  - **Should use**: Coordinator start event
- **Line 199**: `logger.Warn("Timeout waiting for async validation...")` - Timeout event
  - **Status**: Has coordinator event via `emitCheckTimeoutEvent()` but also direct logging
  - **Should use**: Only coordinator event
- **Line 281**: `logger.Info("Async validation completed...")` - Operation completion
  - **Should use**: Coordinator completion event

#### cmd/zqk/system/check_baseline.go
- **Line 52**: `logger.Info("Starting synchronous check baseline collection...")` - Operation start
  - **Should use**: Coordinator start event

#### cmd/zqk/system/migrate_lifecycles.go
- **Line 158**: `logger.Info("Lifecycle migration completed...")` - Operation completion
  - **Should use**: Coordinator completion event

#### cmd/zqk/system/show_validation_progress_helpers.go
- **Line 242**: `logger.Debug("Starting progress drain goroutine...")` - Goroutine start
  - **Should use**: Coordinator event for goroutine lifecycle
- **Line 260**: `logger.Debug("Drain goroutine: timeout...")` - Goroutine timeout
  - **Should use**: Coordinator timeout event
- **Line 320**: `logger.Warn("Failed to stop validator on timeout...")` - Error event
  - **Should use**: Coordinator error event
- **Line 329**: `logger.Warn("Failed to output results on timeout...")` - Error event
  - **Should use**: Coordinator error event
- **Line 815**: `logger.Debug("Queue became empty, starting fallback timeout...")` - Operation status
  - **Should use**: Coordinator event
- **Line 833**: `logger.Debug("Validation goroutines still running after timeout...")` - Operation status
  - **Should use**: Coordinator event
- **Line 903**: `logger.Warn("Timeout waiting for drain goroutine...")` - Timeout event
  - **Should use**: Coordinator timeout event
- **Line 1039**: `logger.Debug("Validation goroutines still running after timeout...")` - Operation status
  - **Should use**: Coordinator event
- **Line 1051**: `logger.Warn("Queue empty for extended period...")` - Warning event
  - **Should use**: Coordinator warning event
- **Line 1059**: `logger.Warn("Failed to stop validator on fallback timeout...")` - Error event
  - **Should use**: Coordinator error event

#### cmd/zqk/system/validate.go
- **Line 235**: `logger.Error("Validation completed with issues...")` - Operation completion with errors
  - **Should use**: Coordinator completion event with error

#### cmd/zqk/system/expand_check_snapshot.go
- **Line 28**: `logger.Info("Started check snapshot expansion in background...")` - Operation start
  - **Should use**: Coordinator start event

#### cmd/zqk/system/spec_writer_field_ops.go
- **Line 111**: `logger.Info("Field operation completed...")` - Operation completion
  - **Should use**: Coordinator completion event

#### cmd/zqk/system/auto_fix_batch_cmd.go
- **Line 105**: `logger.Info("Auto-fix batch completed...")` - Operation completion
  - **Should use**: Coordinator completion event

#### cmd/zqk/system/check_snapshot.go
- **Line 46**: `logger.Info("Started check snapshot save in background...")` - Operation start
  - **Should use**: Coordinator start event

#### cmd/zqk/system/fix_command_executor.go
- **Line 196**: `logger.Info("Fix command executor started...")` - Operation start
  - **Should use**: Coordinator start event
- **Line 228**: `logger.Warn("Timeout waiting for workers to stop...")` - Timeout event
  - **Should use**: Coordinator timeout event

#### cmd/zqk/system/service.go
- **Line 244**: `logger.Info("Service started successfully...")` - Service lifecycle event
  - **Should use**: Coordinator operational event

#### cmd/zqk/system/migrate_audit_buckets.go
- **Line 64**: `logger.Info("Starting audit event migration to buckets...")` - Operation start
  - **Should use**: Coordinator start event

#### cmd/zqk/system/recover_cas.go
- **Line 148**: `logger.Info("CAS recovery completed...")` - Operation completion
  - **Should use**: Coordinator completion event
- **Line 181**: `logger.Info("CAS recovery completed for kind...")` - Operation completion
  - **Should use**: Coordinator completion event

### Missing Coordinator Events for Goroutine Lifecycle

#### cmd/zqk/system/snapshot_expand.go
- **Line 197**: `logger.Debug("Starting parallel write workers...")` - Goroutine start
  - **Should use**: Coordinator event for worker pool start
- **Line 213**: `logger.Debug("Worker cancelled...")` - Goroutine cancellation
  - **Should use**: Coordinator cancellation event

#### cmd/zqk/system/async_check.go
- **Line 490**: `logger.Warn("Discovery timeout...")` - Goroutine timeout
  - **Status**: Has coordinator event via `emitDiscoveryCompletionEventViaCoordinator()` but also direct logging
  - **Should use**: Only coordinator event
- **Line 494**: `logger.Debug("Discovery cancelled...")` - Goroutine cancellation
  - **Status**: Has coordinator event via `emitDiscoveryCancellationEventViaCoordinator()` but also direct logging
  - **Should use**: Only coordinator event

#### cmd/zqk/system/discover_objects_helpers.go
- **Line 30**: `logger.Debug("Discovery cancelled before starting...")` - Goroutine cancellation
  - **Status**: Has coordinator event but also direct logging
  - **Should use**: Only coordinator event
- **Line 61**: `logger.Debug("Discovery cancelled before sending results...")` - Goroutine cancellation
  - **Status**: Has coordinator event but also direct logging
  - **Should use**: Only coordinator event

## Test Code Violations

Test code has many direct logging calls, but these are less critical. However, test code should still use coordinator events for:
- Test operation lifecycle (start, complete, timeout)
- Test progress reporting
- Test error reporting

### Test Files with Direct Output

- `cmd/zqk/scheduler/scan_tests_setup.go` - Multiple `fmt.Fprintf(os.Stdout, ...)` calls
- `cmd/zqk/scheduler/scan_tests_helpers.go` - Multiple `fmt.Fprintf(os.Stdout, ...)` calls
- Many test files use direct logging for test status

## Summary Statistics

- **Production Code Violations**: ~50+ instances
  - Direct `fmt.Fprintf` for progress/status: ~20 instances
  - Direct logger calls for lifecycle events: ~30 instances
  - Missing coordinator events for goroutine lifecycle: ~10 instances
- **Test Code Violations**: ~30+ instances (lower priority)

## Recommended Fixes

1. **Replace all `fmt.Fprintf(os.Stderr/Stdout, ...)` with coordinator events**
   - Progress messages → `emitProgressEventViaCoordinator()`
   - Completion messages → `emitCompletionEventViaCoordinator()`
   - Error messages → `emitErrorEventViaCoordinator()`
   - Warning messages → `emitWarningEventViaCoordinator()`

2. **Replace direct logger calls for lifecycle events with coordinator events**
   - Operation start → `emitOperationStartEventViaCoordinator()`
   - Operation complete → `emitOperationCompletionEventViaCoordinator()`
   - Operation timeout → `emitOperationTimeoutEventViaCoordinator()`
   - Operation error → `emitOperationErrorEventViaCoordinator()`

3. **Add coordinator events for all goroutine lifecycle events**
   - Goroutine start → Coordinator event
   - Goroutine completion → Coordinator event
   - Goroutine cancellation → Coordinator event (already exists for discovery)
   - Goroutine timeout → Coordinator event

4. **Remove duplicate logging** - If coordinator event exists, remove direct logging

## Implementation Priority

1. **High Priority**: Direct `fmt.Fprintf` for user-facing messages (blocks commit per policy)
2. **Medium Priority**: Direct logger calls for lifecycle events (should use coordinator)
3. **Low Priority**: Test code violations (can be addressed incrementally)
