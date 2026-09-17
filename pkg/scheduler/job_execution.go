package scheduler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"github.com/robfig/cron/v3"

	"github.com/lanceman/zqk/pkg/circuitbreaker"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/config"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// runSchedulerJobHandlerWithRetry runs the handler with storage retry and normalizes timeout/cancel errors.
const pipelineKindRunSchedulerJobHandlerWithRetry = "scheduler.run_job_handler_with_retry"
const (
	jobTypeRunWrapper                    = "run_wrapper"
	jobExecutionModeOneTime              = "one_time"
	jobExecReasonDispatchDeadline        = "dispatch_context_expired_before_handler"
	jobExecReasonPackageConcurrencyLimit = "package_concurrency_acquire_failed"
	// jobExecReasonGlobalTestConcurrencyLimit is distinct from the per-package reason so
	// dispatch_pressure.jsonl says which budget was exhausted: one hot package, or the host.
	jobExecReasonGlobalTestConcurrencyLimit = "global_test_concurrency_acquire_failed"
	jobExecReasonConflict                   = "scheduler_conflict" // non-concurrent run_wrapper lost ConflictManager.CanRun
	auditEventSchedulerStarted              = "scheduler_job_started"
	auditEventSchedulerCompleted            = "scheduler_job_completed"
	auditEventSchedulerFailed               = "scheduler_job_failed"
	jobExecStateFailed                      = "failed"
	jobExecStateCompleted                   = "completed"
	jobExecEventFailed                      = "job_execution_failed"
	jobExecEventCompleted                   = "job_execution_completed"
	jobExecEventRequested                   = "job_execution_requested"
	jobExecEventSkipped                     = "job_execution_skipped"
	jobExecPolicyEvaluate                   = "evaluate"
	jobExecEventTypeStarted                 = "started"
	jobExecEventTypeCompleted               = "completed"
	jobExecEventTypeTimeout                 = "timeout"
	jobLogEventTypeOutcome                  = "outcome"
	jobLogEventTypeProgress                 = "progress"
	jobCallbackEventCompletion              = "completion"
	jobCallbackEventError                   = "error"
)

func (s *Scheduler) runSchedulerJobHandlerWithRetry(
	ctx context.Context,
	execCtx context.Context,
	job *ScheduledJob,
	execHandler JobHandler,
	txWrapper *storagepkg.TransactionalStorageWrapper,
	executionStart time.Time,
) error {
	type retryState struct {
		err error
	}

	logger := s.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKindRunSchedulerJobHandlerWithRetry, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(StageExecuteWithRetry, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*retryState](payload)
			if !ok {
				in = &retryState{}
			}

			retryConfig := &storagepkg.RetryConfig{
				MaxAttempts:   3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      4 * time.Second,
				BackoffFactor: 2.0,
			}
			isRecoverableError := func(err error) bool {
				if err == nil {
					return false
				}
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					return false
				}
				errStr := err.Error()
				return strings.Contains(errStr, "hash mismatch") ||
					strings.Contains(errStr, "failed to read hash file") ||
					strings.Contains(errStr, "object already exists") ||
					strings.Contains(errStr, "will retry")
			}

			in.err = storagepkg.ExecuteSimpleRetry(ctx, retryConfig, func() (err error) {
				defer func() {
					if r := recover(); r != nil {
						stack := debug.Stack()
						SchedulerJobExecutionLog(logger).Error(LogEventSchedulerJobExecPanicked,
							errfmt.Errorf("panic: %v", r)).
							JobID(job.ID).
							SchedulerJobType(job.JobType).
							JobCategory(job.Category).
							String("stack", string(stack)).
							Log()
						err = errfmt.Errorf("panic in job execution: %v", r)
					}
				}()

				execErr := execHandler.Execute(execCtx, job)

				if txWrapper != nil {
					if execErr == nil {
						if commitErr := txWrapper.Commit(execCtx); commitErr != nil {
							SchedulerJobExecutionLog(logger).Error(LogEventSchedulerJobExecFailedToCommitTransaction,
								commitErr).
								JobID(job.ID).
								Log()
							return errfmt.Newf("transaction commit failed").Wrap(commitErr)
						}
					} else {
						if rollbackErr := txWrapper.Rollback(execCtx); rollbackErr != nil {
							SchedulerJobExecutionLog(logger).Warn(LogEventSchedulerJobExecFailedToRollbackTransaction).
								JobID(job.ID).
								WithError(rollbackErr).
								Log()
						}
					}
				}

				if execErr != nil && isRecoverableError(execErr) {
					logging.NewEvent(LogEventSchedulerJobExecRecoverableRetry).
						JobID(job.ID).
						JobType(job.JobType).
						Error(logger, execErr)
				}

				return execErr
			}, isRecoverableError)

			return in, nil
		}).
		AddStage(StageNormalizeTimeoutCancel, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*retryState](payload)
			if !ok {
				return &retryState{}, nil
			}

			err := in.err
			if err != nil && execCtx.Err() == context.DeadlineExceeded {
				duration := time.Since(executionStart)
				timeoutFields := []logging.Field{
					logging.JobIDField(job.ID),
					logging.String("job_type", job.JobType),
					logging.String("category", job.Category),
					logging.Int("max_runtime_seconds", job.MaxRuntimeSeconds),
					logging.String("execution_duration", duration.String()),
					logging.String("execution_duration_seconds", fmt.Sprintf("%.2f", duration.Seconds())),
					logging.String("timeout_scope", "execution_only_excludes_trigger_queue_and_package_slot_wait"),
				}
				if job.JobType == jobTypeRunWrapper {
					cmdStr := job.Command
					if len(job.CommandArgs) > 0 {
						cmdStr = fmt.Sprintf("%s %s", cmdStr, strings.Join(job.CommandArgs, " "))
					}
					timeoutFields = append(timeoutFields,
						logging.CommandField(cmdStr),
						logging.String("working_directory", job.WorkingDirectory),
						logging.String("diagnostic_note", "max_runtime_seconds is enforced from when execution starts (after queue/package wait). If tests need longer, raise max_runtime_seconds or reduce bundle parallelism."),
					)
				}
				SchedulerJobExecutionLog(logger).Error(LogEventSchedulerJobExecTimedOutRunning,
					errfmt.Errorf("ran %s; limit is %ds from execution start (not while queued)", duration.Truncate(time.Millisecond), job.MaxRuntimeSeconds)).
					WithFields(timeoutFields...).
					Log()
				err = errfmt.Errorf("job exceeded max_runtime_seconds (%d) after %s of execution (timer starts when the job runs, not while queued)",
					job.MaxRuntimeSeconds, duration.Truncate(time.Millisecond))
			} else if err != nil && execCtx.Err() == context.Canceled {
				err = execCtx.Err()
			}

			in.err = err
			return in, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return payload, nil
		}).
		Build()

	st := &retryState{}
	out, runErr := pl.Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, st)
	if runErr != nil {
		return runErr
	}
	ds, ok := nildecode.DecodeNonNilPayload[*retryState](out)
	if !ok {
		return nil
	}
	return ds.err
}

// executeJobAfterHandlerReturns records metrics, clears running state, emits coordination and audit follow-up, and persists the job.
func (s *Scheduler) executeJobAfterHandlerReturns(
	ctx context.Context,
	job *ScheduledJob,
	err error,
	executionStart time.Time,
	executionID string,
	opCallback concurrency.OperationCallback,
	operationID string,
	unregisterCallback func(),
) {
	duration := time.Since(executionStart)

	if s.metrics != nil {
		success := err == nil
		when.When(func() bool { return err != nil }).Then(func() {
			s.metrics.RecordJobExecutionFailed(job.ID, job.JobType, duration, err)
		}).OrElse(func() {
			s.metrics.RecordJobExecutionCompleted(job.ID, job.JobType, duration, success)
		}).Run()
	}

	s.emitClusterStatusAfterJob(job, err)

	if err := concurrency.RunInLockWithLogger(
		&job.RunningMu, LockNameSchedulerJobClearContext, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			job.executionCtx = nil
			job.executionCancel = nil
			job.Running = false
			return nil
		},
	); err != nil {
		SchedulerJobExecutionLog(s.logger).Warn("Failed to clear job execution context under lock").WithError(err).Log()
	}

	unregisterCallback()

	if executionID != emptyValue && s.coordinationChannel != nil && s.stateRegistry != nil {
		completionState := jobExecStateFailed
		completionEventType := jobExecEventFailed
		resultStr := jobExecStateFailed

		if err == nil {
			completionState = jobExecStateCompleted
			completionEventType = jobExecEventCompleted
			resultStr = jobExecStateCompleted
		} else if strings.Contains(err.Error(), TestBundleSuccessWithFailuresPrefix) {
			completionState = jobExecStateCompleted
			completionEventType = jobExecEventCompleted
			resultStr = jobExecStateCompleted
		}

		if errStatus := s.stateRegistry.CompleteExecution(job.ID, executionID, resultStr); errStatus != nil {
			SchedulerJobExecutionLog(s.logger).Warn("Failed to complete job execution in state registry").WithError(errStatus).Log()
		}
		metadata := map[string]any{}
		if completionEventType == jobExecEventFailed {
			metadata["error"] = err.Error()
		}

		if errPub := s.coordinationChannel.PublishEvent(Event{
			Type:           completionEventType,
			Timestamp:      time.Now().UTC(),
			JobID:          job.ID,
			ExecutionID:    executionID,
			ProcessID:      os.Getpid(),
			PolicyDecision: completionState,
			Metadata:       metadata,
		}); errPub != nil {
			SchedulerJobExecutionLog(s.logger).Warn("Failed to publish job completion event").WithError(errPub).Log()
		}
	}

	s.handleJobExecutionOutcome(ctx, job, err, duration, opCallback, operationID)
	s.TouchActivity()

	invokeConfiguredCallback := shouldInvokeConfiguredJobCallback(ctx, job)
	when.When(func() bool {
		return invokeConfiguredCallback && err == nil && job.CallbackOnCompletion != emptyValue
	}).Then(func() {
		InvokeJobCallback(ctx, s.logger, s.asyncRouter, job, jobCallbackEventCompletion, map[string]any{"outcome": "success"})
	}).OrElseWhen(func() bool {
		return invokeConfiguredCallback && err != nil && job.CallbackOnError != emptyValue
	}).Then(func() {
		InvokeJobCallback(ctx, s.logger, s.asyncRouter, job, jobCallbackEventError, map[string]any{"outcome": "error", "error": err.Error()})
	}).Run()

	// SCH-run-* test bundles are one_time + immediate but must stay enabled so scan-tests re-triggers and
	// `zqk scheduler trigger SCH-run-*` can re-run regression bundles; disabling after each green run
	// left jobs stuck disabled with no health.jsonl updates on subsequent triggers.
	if job.ExecutionMode == jobExecutionModeOneTime && !IsTestBundleJob(job.ID) {
		if err == nil {
			SchedulerJobExecutionLog(s.logger).Info(LogEventSchedulerJobExecDisablingOneTimeAfterSuccess).
				JobID(job.ID).
				Log()
		} else {
			SchedulerJobExecutionLog(s.logger).Info(LogEventSchedulerJobExecDisablingOneTimeAfterFailure).
				JobID(job.ID).
				WithError(err).
				Log()
		}
		job.Enabled = false
		job.Status = StatusDisabled
		s.disableJobInStorage(ctx, job)
	}

	s.updateJobInStorage(ctx, job)
}

func shouldInvokeConfiguredJobCallback(ctx context.Context, job *ScheduledJob) bool {
	return TriggerOriginFromContext(ctx) == TriggerOriginPreCommit ||
		(job != nil && job.ExecutionMode == jobExecutionModeOneTime)
}

// executeJob executes a scheduled job with conflict management and timeout
func (s *Scheduler) executeJob(ctx context.Context, job *ScheduledJob, handler JobHandler) {
	// Flush and close buffered job log writer when this run ends (no-op if none was created).
	defer CloseJobLogWriter(s.getProjectRoot(), job.ID)

	// Re-read job from storage to get current enabled status
	// This ensures that disabling a job takes effect immediately without requiring restart
	rawJob, err := s.storage.Read(ctx, s.secCtx, job.ID)
	if err != nil {
		SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecFailedRereadJobUsingCached).
			WithFields(jobLogFieldsByIDAndErr(job.ID, err)...).
			Log()
		// Fall back to cached value if read fails
		if !job.Enabled {
			s.clearImmediateDispatchPending(job.ID)
			return
		}
	} else {
		// Check current enabled status from storage
		if enabled, ok := rawJob[objects.FieldKeyEnabled].(bool); ok && !enabled {
			s.clearImmediateDispatchPending(job.ID)
			SchedulerJobExecutionLog(s.logger).Debug(LogEventSchedulerJobExecSkippingDisabledInStorage).
				JobID(job.ID).
				Log()
			return
		}
		// Update in-memory job with current enabled status
		if enabled, ok := rawJob[objects.FieldKeyEnabled].(bool); ok {
			job.Enabled = enabled
		}
		// Keep environment_variables in sync with storage (e.g. CONVERGENCE_SESSION_ID on test-bundle jobs).
		if s.jobLoader != nil {
			s.jobLoader.RefreshEnvironmentVariablesFromRaw(job, rawJob)
		}
	}

	if s.admissionAlreadyFailed(job.ID) {
		s.clearImmediateDispatchPending(job.ID)
		return
	}

	// Dispatch context (TriggerJob / pool wait) is bounded by getSchedulerDispatchResourceWaitMax(); if the
	// job sat in the pool behind many pkg/storage runs, the context can expire before we acquire the package
	// slot or start the handler. Record + re-enqueue so the loss is visible and retried.
	if err := ctx.Err(); err != nil && errors.Is(err, context.DeadlineExceeded) {
		s.recordDispatchAttemptDropped("execute_job", jobExecReasonDispatchDeadline, job, err)
		logging.NewEvent(LogEventSchedulerJobExecDispatchContextExpiredBeforeStart).
			JobID(job.ID).
			JobType(job.JobType).
			Warn(s.logger.WithFields(logging.Error(err)))
		s.reenqueueTriggerAfterDispatchDrop(job, jobExecReasonDispatchDeadline)
		return
	}

	// One-time jobs can be re-run until disabled or deleted
	// Log if this is a re-execution for visibility
	if job.ExecutionMode == jobExecutionModeOneTime && job.LastRunAt != nil {
		SchedulerJobExecutionLog(s.logger).Info(LogEventSchedulerJobExecReExecutingOneTime).
			JobID(job.ID).
			String("trigger_type", job.TriggerType).
			String("last_run_at", zqktime.FormatRFC3339UTCPtr(job.LastRunAt)).
			String("note", "One-time jobs can be re-run until disabled or deleted").
			Log()
	}

	// Coordination kernel pre-flight (CRIT-9040):
	// - Publish "requested"
	// - Evaluate policy (currently: skip if already in_progress/deferred)
	if s.coordinationChannel != nil && s.policyEngine != nil && s.stateRegistry != nil {
		if errPub := s.coordinationChannel.PublishEvent(Event{
			Type:           jobExecEventRequested,
			Timestamp:      time.Now().UTC(),
			JobID:          job.ID,
			ProcessID:      os.Getpid(),
			PolicyDecision: jobExecPolicyEvaluate,
			Metadata: map[string]any{
				objects.FieldKeyJobType:     job.JobType,
				objects.FieldKeyTriggerType: job.TriggerType,
				objects.FieldKeyCategory:    job.Category,
			},
		}); errPub != nil {
			SchedulerJobExecutionLog(s.logger).Warn("Failed to publish job requested event").WithError(errPub).Log()
		}

		decision, derr := s.policyEngine.Evaluate(job.ID, job)
		if derr != nil {
			SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecCoordinationPolicyEvalFailed).
				WithError(derr).
				Log()
		} else if decision != nil {
			switch decision.Action {
			case decisionSkip:
				if errPub := s.coordinationChannel.PublishEvent(Event{
					Type:           jobExecEventSkipped,
					Timestamp:      time.Now().UTC(),
					JobID:          job.ID,
					ProcessID:      os.Getpid(),
					PolicyDecision: decision.Action,
					Metadata:       map[string]any{objects.FieldKeyReason: decision.Reason},
				}); errPub != nil {
					SchedulerJobExecutionLog(s.logger).Warn("Failed to publish job skip event").WithError(errPub).Log()
				}
				return
			case decisionDefer, decisionScheduledDefer:
				// Phase-1 PolicyEngine doesn't currently emit defer decisions,
				// but keep the control flow for forwards compatibility.
				var deferUntil *time.Time
				if decision.DeferUntil != nil {
					deferUntil = decision.DeferUntil
				}
				if errDefer := s.stateRegistry.DeferExecution(job.ID, decision.Reason, deferUntil); errDefer != nil {
					SchedulerJobExecutionLog(s.logger).Warn("Failed to defer job execution in state registry").WithError(errDefer).Log()
				}
				return
			}
		}
	}

	// Check for conflicts
	canRun := s.conflictMgr.CanRun(job)
	if s.metrics != nil {
		s.metrics.RecordConflictCheck(job.ID, canRun)
	}
	if !canRun {
		if s.metrics != nil {
			s.metrics.RecordConflictDetected(job.ID, job.JobType)
		}
		// Non-concurrent run_wrappers (e.g. maintenance scripts) serialize globally; timer fires that
		// lose the slot used to drop silently with no retry. Record dispatch pressure and re-enqueue
		// like package-concurrency / dispatch-deadline drops so the trigger queue retries.
		s.recordDispatchAttemptDropped("execute_job", jobExecReasonConflict, job)
		s.reenqueueTriggerAfterDispatchDrop(job, jobExecReasonConflict)
		SchedulerJobExecutionLog(s.logger).Debug(LogEventSchedulerJobExecSkippingConflict).
			JobID(job.ID).
			SchedulerJobType(job.JobType).
			JobCategory(job.Category).
			Log()
		return
	}

	// Skip jobs that have repeatedly failed to create/acquire lock (stops log spam and process flow).
	if s.shouldSkipJobDueToLockFailures(job.ID) {
		SchedulerJobExecutionLog(s.logger).Debug(LogEventSchedulerJobExecSkippingRepeatedLockFailures).
			JobID(job.ID).
			SchedulerJobType(job.JobType).
			Log()
		return
	}

	// Acquire distributed lock to prevent duplicate execution across processes
	jobLock, lockErr := NewJobLock(job.ID, s.projectRoot)
	if lockErr != nil {
		count, shouldLog, shouldDisable := s.recordLockFailure(job.ID)
		if shouldLog {
			if count == 1 {
				SchedulerJobExecutionLog(s.logger).Error(LogEventSchedulerJobExecFailedCreateJobLock,
					lockErr).
					JobID(job.ID).
					SchedulerJobType(job.JobType).
					JobCategory(job.Category).
					Log()
			} else {
				SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecRepeatedLockCreateFailures).
					JobID(job.ID).
					SchedulerJobType(job.JobType).
					Int("failure_count", count).
					Log()
			}
		}
		if shouldDisable {
			s.disableJobInStorage(ctx, job)
		}
		return
	}
	defer func() {
		if closeErr := jobLock.Close(); closeErr != nil {
			SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecFailedCloseJobLock).
				JobID(job.ID).
				WithError(closeErr).
				Log()
		}
	}()

	// Try to acquire lock (non-blocking first attempt)
	acquired, lockErr := jobLock.TryAcquire()
	if lockErr != nil {
		count, shouldLog, shouldDisable := s.recordLockFailure(job.ID)
		if shouldLog {
			if count == 1 {
				SchedulerJobExecutionLog(s.logger).Error(LogEventSchedulerJobExecFailedTryAcquireJobLock,
					lockErr).
					JobID(job.ID).
					SchedulerJobType(job.JobType).
					JobCategory(job.Category).
					Log()
			} else {
				SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecRepeatedLockAcquireFailures).
					JobID(job.ID).
					SchedulerJobType(job.JobType).
					Int("failure_count", count).
					Log()
			}
		}
		if shouldDisable {
			s.disableJobInStorage(ctx, job)
		}
		return
	}

	// If not acquired, try with timeout
	if !acquired {
		SchedulerJobExecutionLog(s.logger).Debug(LogEventSchedulerJobExecJobLockHeldWaiting).
			JobID(job.ID).
			Log()
		acquireErr := jobLock.Acquire()
		if acquireErr != nil {
			count, shouldLog, shouldDisable := s.recordLockFailure(job.ID)
			if shouldLog {
				if count == 1 {
					SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecFailedAcquireLockTimeout).
						JobID(job.ID).
						SchedulerJobType(job.JobType).
						JobCategory(job.Category).
						WithError(acquireErr).
						Log()
				} else {
					SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecRepeatedLockAcquireTimeoutFailures).
						JobID(job.ID).
						SchedulerJobType(job.JobType).
						Int("failure_count", count).
						Log()
				}
			}
			if shouldDisable {
				s.disableJobInStorage(ctx, job)
			}
			return
		}
	}
	// Lock acquired successfully; clear any prior failure count so re-enabled jobs can run again
	s.clearLockFailureCount(job.ID)

	// Limit concurrent run_wrapper jobs per package path (e.g. at most 1 pkg/storage at a time)
	// to avoid timeouts and contention when scan-tests submits many bundles at once.
	if job.JobType == jobTypeRunWrapper && s.packageConcurrencyLimiter != nil {
		packagePath := circuitbreaker.ExtractPackagePathFromRunWrapperCommand(job.Command, job.CommandArgs)
		if s.packageConcurrencyLimiter.ShouldLimit(packagePath) {
			if err := s.packageConcurrencyLimiter.Acquire(ctx, packagePath); err != nil {
				s.recordDispatchAttemptDropped("execute_job", jobExecReasonPackageConcurrencyLimit, job, err)
				SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecFailedAcquirePackageConcurrencySlot).
					JobID(job.ID).
					String("package_path", packagePath).
					WithError(err).
					Log()
				s.reenqueueTriggerAfterDispatchDrop(job, jobExecReasonPackageConcurrencyLimit)
				return
			}
			defer s.packageConcurrencyLimiter.Release(packagePath)
		}

		// Then a slot from the global test budget, so total concurrency is bounded by the host and
		// not by how many distinct packages happen to be in flight.
		//
		// Ordered after the per-package slot deliberately. Acquiring the global slot first would let
		// several jobs queued on one hot package sit on global slots while other packages starve;
		// this way a job waiting for the global budget holds only its own package's slot, which was
		// already limited to one.
		if err := s.acquireGlobalTestJobSlot(ctx); err != nil {
			s.recordDispatchAttemptDropped("execute_job", jobExecReasonGlobalTestConcurrencyLimit, job, err)
			SchedulerJobExecutionLog(s.logger).Warn("scheduler: failed to acquire global test concurrency slot").
				JobID(job.ID).
				WithError(err).
				Log()
			s.reenqueueTriggerAfterDispatchDrop(job, jobExecReasonGlobalTestConcurrencyLimit)
			return
		}
		defer s.releaseGlobalTestJobSlot()
	}

	// Lock and concurrency slots acquired successfully; clear any dispatch drop retries
	s.clearDispatchDropRetryCount(job.ID)

	// Mark as running
	if err := concurrency.RunInLockWithLogger(
		&job.RunningMu, LockNameSchedulerJobMarkRunning, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			job.Running = true
			return nil
		},
	); err != nil {
		SchedulerJobExecutionLog(s.logger).Warn("Failed to mark job as running under lock").WithError(err).Log()
	}

	// Register with conflict manager
	s.conflictMgr.RegisterRunning(job)
	// Use callback (not defer) for predictable execution order
	unregisterCallback := func() {
		s.conflictMgr.UnregisterRunning(job)
	}

	// Execution clock starts here (after trigger-queue wait, conflicts, job lock, and package slot).
	// max_runtime_seconds applies only to this phase, not time spent queued for a worker or package limiter.
	executionStart := time.Now()

	// Use WithoutCancel(ctx) so the dispatch context's long queue/ceiling safety deadline does not
	// cap execution; only max_runtime_seconds applies while the handler runs.
	var execCtx context.Context
	var cancel context.CancelFunc
	if job.MaxRuntimeSeconds > 0 {
		execCtx, cancel = context.WithTimeout(
			context.WithoutCancel(ctx),
			time.Duration(job.MaxRuntimeSeconds)*time.Second,
		)
	} else {
		execCtx, cancel = context.WithCancel(context.WithoutCancel(ctx))
	}
	defer cancel()

	// Store execution context for potential cancellation
	_ = concurrency.RunInLockWithLogger(
		&job.RunningMu, LockNameSchedulerJobStoreContext, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			job.executionCtx = execCtx
			job.executionCancel = cancel
			return nil
		},
	)

	// Coordination kernel (CRIT-9040): register execution + publish "started".
	var executionID string
	if s.coordinationChannel != nil && s.stateRegistry != nil {
		executionID = fmt.Sprintf("exec-%d-%d", executionStart.Unix(), os.Getpid())
		if err := s.stateRegistry.RegisterExecution(job.ID, executionID, os.Getpid()); err != nil {
			SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecCoordinationRegisterFailed).
				WithError(err).
				Log()
			executionID = ""
		} else {
			_ = s.coordinationChannel.PublishEvent(Event{
				Type:           "job_execution_started",
				Timestamp:      time.Now().UTC(),
				JobID:          job.ID,
				ExecutionID:    executionID,
				ProcessID:      os.Getpid(),
				PolicyDecision: "execute",
				Metadata: map[string]any{
					objects.FieldKeyJobType:  job.JobType,
					objects.FieldKeyCategory: job.Category,
				},
			})
		}
	}

	// Pool workers (goroutinelabels) recover panics without re-panicking, so a panic in the
	// handler would otherwise skip CompleteExecution and leave job_execution in_progress on disk,
	// causing timer jobs (e.g. SCH-cache-prewarm cache_prewarm) to be skipped forever.
	var operationID string
	var opCallback concurrency.OperationCallback

	defer func() {
		if r := recover(); r != nil {
			panicErr := errfmt.Errorf("panic in executeJob: %v", r)
			if executionID != emptyValue && s.coordinationChannel != nil && s.stateRegistry != nil {
				_ = s.stateRegistry.CompleteExecution(job.ID, executionID, "failed")
				_ = s.coordinationChannel.PublishEvent(Event{
					Type:           "job_execution_failed",
					Timestamp:      time.Now().UTC(),
					JobID:          job.ID,
					ExecutionID:    executionID,
					ProcessID:      os.Getpid(),
					PolicyDecision: "failed",
					Metadata:       map[string]any{"error": panicErr.Error()},
				})
			}
			s.executeJobAfterHandlerReturns(ctx, job, panicErr, executionStart, executionID, opCallback, operationID, unregisterCallback)
		}
	}()

	if s.metrics != nil {
		s.metrics.RecordJobExecutionStarted(job.ID, job.JobType)
	}

	// Update last run time
	now := time.Now()
	job.LastRunAt = &now
	s.clearImmediateDispatchPending(job.ID)

	// Create high-level operation callback for this job execution
	// This provides a canonical lifecycle view on top of the more granular
	// coordinator events already emitted via createJobAuditEvent
	operationID = fmt.Sprintf("scheduler_job_%s_%d", job.ID, executionStart.UnixNano())

	if ctxOp := ctx.Value(operationCallbackContextKey{}); ctxOp != nil {
		if cb, ok := ctxOp.(concurrency.OperationCallback); ok {
			opCallback = cb
		}
	}
	when.When(func() bool { return opCallback == nil }).Then(func() {
		projectRoot := s.getProjectRoot()
		when.When(func() bool { return projectRoot != emptyValue && s.storage != nil }).Then(func() {
			opCallback = coordination.NewCoordinatorOperationCallback(
				ctx,
				projectRoot,
				s.storage,
				"scheduler_job_execution",
				string(pkgctx.ProfileSystem),
			)
		}).OrElse(func() {
			opCallback = &concurrency.NoOpOperationCallback{}
		}).Run()
	}).Run()
	// Emit start event with job metadata (noop-safe)
	opCallback.OnStart(operationID, map[string]any{
		"operation_type": "scheduler_job_execution",
		KeyJobID:         job.ID,
		KeyJobType:       job.JobType,
		KeyCategory:      job.Category,
		KeyJobTitle:      job.Title,
	})

	// Create audit event for job start (BLI-TDE-AUDIT-FAILCLOSED-001: fail-closed if audit persist fails)
	if err := s.createJobAuditEvent(ctx, auditEventSchedulerStarted, job.ID, job.JobType, job.Category, true, 0, nil); err != nil {
		auditErr := fmt.Errorf("fail-closed: job start audit persist failed: %w", err)
		if executionID != emptyValue && s.coordinationChannel != nil && s.stateRegistry != nil {
			_ = s.stateRegistry.CompleteExecution(job.ID, executionID, "failed")
			_ = s.coordinationChannel.PublishEvent(Event{
				Type:           "job_execution_failed",
				Timestamp:      time.Now().UTC(),
				JobID:          job.ID,
				ExecutionID:    executionID,
				ProcessID:      os.Getpid(),
				PolicyDecision: "failed",
				Metadata:       map[string]any{"error": auditErr.Error()},
			})
		}
		s.executeJobAfterHandlerReturns(ctx, job, auditErr, executionStart, executionID, opCallback, operationID, unregisterCallback)
		return
	}

	// Append category index entry so maintenance/testing/etc. jobs are easy to locate by category.
	appendCategoryLogEntry(s.getProjectRoot(), job, "started", 0, nil)

	// Write per-job events for non-run_wrapper jobs (run_wrapper writes its own detailed events to the same path)
	// Location: JobEventsFilePath (per-job dir, churn-runs/, or test-bundles per job_log_paths.go).
	if job.JobType != jobTypeRunWrapper {
		writeJobLogEntry(s.getProjectRoot(), job.ID, map[string]any{
			KeyTimestamp: zqktime.FormatRFC3339UTC(executionStart),
			KeyJobID:     job.ID,
			KeyJobType:   job.JobType,
			KeyEventType: jobExecEventTypeStarted,
		})
	}

	// If transactional mode is enabled, wrap storage operations in a transaction
	var txWrapper *storagepkg.TransactionalStorageWrapper
	var transactionalHandler JobHandler
	if job.Transactional {
		SchedulerJobExecutionLog(s.logger).Debug(LogEventSchedulerJobExecTransactionalMode).
			WithFields(jobLogFields(job)...).
			Log()

		// Create transaction-aware storage wrapper
		wrapper, txErr := storagepkg.NewTransactionalStorageWrapper(execCtx, s.storage)
		when.When(func() bool { return txErr != nil }).Then(func() {
			SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecFailedCreateTransactionalWrapper).
				JobID(job.ID).
				WithError(txErr).
				Log()
		}).OrElse(func() {
			txWrapper = wrapper
			transactionalHandler = s.createHandlerWithStorage(job, wrapper)
		}).Run()
	}

	// Use transactional handler if available, otherwise use original handler
	execHandler := handler
	if transactionalHandler != nil {
		execHandler = transactionalHandler
	}

	// CRITICAL: Execute job with retry logic for 99.999% reliability (see runSchedulerJobHandlerWithRetry).
	err = s.runSchedulerJobHandlerWithRetry(ctx, execCtx, job, execHandler, txWrapper, executionStart)

	s.executeJobAfterHandlerReturns(ctx, job, err, executionStart, executionID, opCallback, operationID, unregisterCallback)
}

// handleJobExecutionOutcome logs result, writes job log, emits audit/op callbacks, reports issues, and sends notifications.
// When the job ran successfully but had internal issues (e.g. test bundle ran but some tests failed), we do not create
// error-level entries: we log a warning, append to .zqk/scheduler/issues.json (maintenance), and emit completed so logs stay clean.
func (s *Scheduler) handleJobExecutionOutcome(ctx context.Context, job *ScheduledJob, err error, duration time.Duration, opCallback concurrency.OperationCallback, operationID string) {
	if err != nil {
		// Job ran successfully (e.g. test bundle executed) but had internal issues (e.g. test failures). Treat as success for
		// logging/audit so we don't create error entries; log warning and append to issues.json for maintenance.
		if strings.Contains(err.Error(), TestBundleSuccessWithFailuresPrefix) {
			fields := append(jobLogFieldsWithCategoryAndDuration(job, duration), logging.String(KeyError, err.Error()))
			SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecCompletedWithInternalIssues).
				WithFields(fields...).
				Log()
			if opCallback != nil {
				opCallback.OnComplete(operationID, map[string]any{
					KeyJobID:   job.ID,
					KeyJobType: job.JobType,
					KeySuccess: true,
					KeyError:   err.Error(),
				}, duration)
			}
			if auditErr := s.createJobAuditEvent(ctx, auditEventSchedulerCompleted, job.ID, job.JobType, job.Category, true, duration, nil); auditErr != nil {
				SchedulerJobExecutionLog(s.logger).Error("fail-closed: job completed audit persist failed", auditErr).
					WithFields(jobLogFieldsWithCategoryAndDuration(job, duration)...).
					Log()
			}
			ReportIssue(s.getProjectRoot(), job.ID, job.JobType, err.Error())
			return
		}

		eventType := jobExecStateFailed
		if strings.Contains(err.Error(), "timed out") {
			eventType = jobExecEventTypeTimeout
		}
		if job.JobType != jobTypeRunWrapper {
			writeJobLogEntry(s.getProjectRoot(), job.ID, map[string]any{
				KeyTimestamp: zqktime.NowRFC3339UTC(),
				KeyJobID:     job.ID,
				KeyJobType:   job.JobType,
				KeyEventType: eventType,
				KeyDuration:  duration.String(),
				KeyError:     err.Error(),
			})
		}
		// Category index: failed/timeout entries for quick scanning by category.
		appendCategoryLogEntry(s.getProjectRoot(), job, eventType, duration, err)
		logger := s.logger
		if logger == nil {
			logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		}
		SchedulerJobExecutionLog(logger).Error(LogEventSchedulerJobExecFailed,
			err).
			WithFields(jobLogFieldsWithCategoryAndDuration(job, duration)...).
			Log()
		if opCallback != nil {
			opCallback.OnError(operationID, err)
		}
		if auditErr := s.createJobAuditEvent(ctx, auditEventSchedulerFailed, job.ID, job.JobType, job.Category, false, duration, err); auditErr != nil {
			SchedulerJobExecutionLog(logger).Error("fail-closed: job failed audit persist failed", auditErr).
				WithFields(jobLogFieldsWithCategoryAndDuration(job, duration)...).
				Log()
		}
		ReportIssue(s.getProjectRoot(), job.ID, job.JobType, err.Error())
		if job.JobType != jobTypeRunWrapper && s.notificationContext != nil {
			notif := CreateJobNotification(
				job.ID,
				job.JobType,
				job.Category,
				"failed",
				PriorityHigh,
				duration,
				err,
				map[string]any{
					KeyJobTitle:       job.Title,
					KeyJobDescription: job.Description,
				},
			)
			s.notificationContext.Notify(notif)
		}
		return
	}
	if job.JobType != jobTypeRunWrapper {
		writeJobLogEntry(s.getProjectRoot(), job.ID, map[string]any{
			KeyTimestamp: zqktime.NowRFC3339UTC(),
			KeyJobID:     job.ID,
			KeyJobType:   job.JobType,
			KeyEventType: jobExecEventTypeCompleted,
			KeyDuration:  duration.String(),
		})
	}
	// Category index: completed entry so frequency and outcome per category are visible.
	appendCategoryLogEntry(s.getProjectRoot(), job, "completed", duration, nil)
	SchedulerJobExecutionLog(s.logger).Info(LogEventSchedulerJobExecCompleted).
		WithFields(jobLogFieldsWithCategoryAndDuration(job, duration)...).
		Log()
	if opCallback != nil {
		opCallback.OnComplete(operationID, map[string]any{
			KeyJobID:   job.ID,
			KeyJobType: job.JobType,
			KeySuccess: true,
		}, duration)
	}
	if auditErr := s.createJobAuditEvent(ctx, auditEventSchedulerCompleted, job.ID, job.JobType, job.Category, true, duration, nil); auditErr != nil {
		SchedulerJobExecutionLog(s.logger).Error("fail-closed: job completed audit persist failed", auditErr).
			WithFields(jobLogFieldsWithCategoryAndDuration(job, duration)...).
			Log()
	}
	if job.JobType != jobTypeRunWrapper && s.notificationContext != nil {
		notif := CreateJobNotification(
			job.ID,
			job.JobType,
			job.Category,
			"completed",
			PriorityMedium,
			duration,
			nil,
			map[string]any{
				KeyJobTitle:       job.Title,
				KeyJobDescription: job.Description,
			},
		)
		s.notificationContext.Notify(notif)
	}
}

// updateJobInStorage updates the job's last_run_at and next_run_at in storage
func (s *Scheduler) updateJobInStorage(ctx context.Context, job *ScheduledJob) {
	// Get the job entry from cron to calculate next run
	entry := s.cron.Entry(job.CronEntryID)
	if entry.Valid() {
		nextRun := entry.Schedule.Next(time.Now())
		job.NextRunAt = &nextRun
	} else if job.TriggerType == "timer" && job.ScheduleExpr != "" {
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		if schedule, err := parser.Parse(job.ScheduleExpr); err == nil {
			nextRun := schedule.Next(time.Now())
			job.NextRunAt = &nextRun
		}
	}

	// Update the scheduler_job object
	// Format timestamps as UTC with Z suffix (YYYY-MM-DDTHH:MM:SSZ) to match validation pattern
	updateData := map[string]any{}
	if job.LastRunAt != nil {
		updateData[objects.FieldKeyLastRunAt] = zqktime.FormatRFC3339UTCPtr(job.LastRunAt)
	}
	if job.NextRunAt != nil {
		updateData[objects.FieldKeyNextRunAt] = zqktime.FormatRFC3339UTCPtr(job.NextRunAt)
	}
	if !job.Enabled {
		updateData[objects.FieldKeyEnabled] = false
		if job.Status == StatusDisabled {
			updateData[objects.FieldKeyStatus] = StatusDisabled
		}
	}

	// Actually update the job object in storage
	// Use system security context since this is an internal scheduler operation
	lastRunAtStr := ""
	if job.LastRunAt != nil {
		lastRunAtStr = zqktime.FormatRFC3339UTCPtr(job.LastRunAt)
	}
	err := s.storage.Update(ctx, s.secCtx, job.ID, updateData)
	when.When(func() bool { return err != nil }).Then(func() {
		SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecFailedUpdateMetadata).
			WithFields(jobLogFieldsByIDAndErr(job.ID, err)...).
			Log()
		SchedulerJobExecutionLog(s.logger).Debug(LogEventSchedulerJobExecMetadataUpdateAttempted).
			JobID(job.ID).
			String("last_run_at", lastRunAtStr).
			String("enabled", fmt.Sprintf("%v", job.Enabled)).
			Log()
	}).OrElse(func() {
		SchedulerJobExecutionLog(s.logger).Debug(LogEventSchedulerJobExecMetadataUpdated).
			JobID(job.ID).
			String("last_run_at", lastRunAtStr).
			String("enabled", fmt.Sprintf("%v", job.Enabled)).
			Log()
		_ = caspkg.GetListingIndexWriteQueueForProjectRoot(s.getProjectRoot()).FlushKind(objects.KindSchedulerJob, 2*time.Second) //nolint:errcheck // best-effort
	}).Run()
}

// disableJobInStorage disables a job in storage (for one-time jobs).
// Passes WithCacheInvalidate so the cache handler removes this ID from the object-id-cache
// and invalidates list cache; WAL handles the update, then the reaper deletes. Init does not
// load disabled jobs (LoadJobs filters them; cache no longer contains the ID).
// DisableJobInStorage disables a job in storage (for one-time jobs).
func (s *Scheduler) DisableJobInStorage(ctx context.Context, job *ScheduledJob) {
	s.disableJobInStorage(ctx, job)
}

func (s *Scheduler) disableJobInStorage(ctx context.Context, job *ScheduledJob) {
	if s == nil || job == nil {
		return
	}
	job.Enabled = false
	job.Status = StatusDisabled
	_ = concurrency.RunInLockWithLogger(
		&s.jobsMu, "scheduler.disable_job", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if j, exists := s.jobs[job.ID]; exists && j != nil {
				j.Enabled = false
				j.Status = StatusDisabled
			}
			return nil
		},
	)

	// No permission check needed - this is an internal operation by the scheduler
	// Update the scheduler_job object to disable it
	updateData := map[string]any{
		objects.FieldKeyEnabled: false,
		objects.FieldKeyStatus:  StatusDisabled,
	}
	if job.LastRunAt != nil {
		// Format timestamp as UTC with Z suffix (YYYY-MM-DDTHH:MM:SSZ) to match validation pattern
		updateData[objects.FieldKeyLastRunAt] = zqktime.FormatRFC3339UTCPtr(job.LastRunAt)
	}

	// Cache update: remove this ID from object-id-cache so init/load doesn't see it; handler also invalidates list cache.
	ctx = pkgctx.WithCacheInvalidate(ctx, job.ID)
	// Actually update the job object in storage (WAL handles disable; reaper deletes later)
	err := s.storage.Update(ctx, s.secCtx, job.ID, updateData)
	when.When(func() bool { return err != nil }).Then(func() {
		SchedulerJobExecutionLog(s.logger).Warn(LogEventSchedulerJobExecFailedDisableInStorage).
			WithFields(append(jobLogFieldsWithErr(job, err), logging.String("execution_mode", job.ExecutionMode))...).
			Log()
	}).OrElse(func() {
		SchedulerJobExecutionLog(s.logger).Debug(LogEventSchedulerJobExecJobDisabledInStorage).
			JobID(job.ID).
			String("execution_mode", job.ExecutionMode).
			Log()
		_ = caspkg.GetListingIndexWriteQueueForProjectRoot(s.getProjectRoot()).FlushKind(objects.KindSchedulerJob, 2*time.Second) //nolint:errcheck // best-effort
	}).Run()
}

// WriteJobOutcome appends an outcome entry to the job's events file so aggregation and retention runs show desired results.
// Handlers call this with job-specific metrics (e.g. events_processed, metrics_created for aggregation; total_archived, total_deleted for retention).
// Best-effort: errors are ignored so job execution is not affected.
func WriteJobOutcome(projectRoot, jobID, jobType string, outcome map[string]any) {
	if projectRoot == emptyValue || jobID == emptyValue || outcome == nil {
		return
	}
	entry := make(map[string]any)
	entry[KeyEventType] = jobLogEventTypeOutcome
	entry[KeyJobID] = jobID
	entry[objects.FieldKeyJobType] = jobType
	entry[KeyTimestamp] = zqktime.NowRFC3339UTC()
	for k, v := range outcome {
		entry[k] = v
	}
	writeJobLogEntry(projectRoot, jobID, entry)
}

// WriteJobProgress appends a progress entry to the job's events file. Use from long-running handlers so that on timeout
// we have a record of what was already done (e.g. per-kind archived/deleted counts for retention_tolerance).
// Best-effort: errors are ignored so job execution is not affected.
func WriteJobProgress(projectRoot, jobID string, entry map[string]any) {
	if projectRoot == emptyValue || jobID == emptyValue || entry == nil {
		return
	}
	if entry[KeyEventType] == nil {
		entry[KeyEventType] = jobLogEventTypeProgress
	}
	if entry[KeyTimestamp] == nil {
		entry[KeyTimestamp] = zqktime.NowRFC3339UTC()
	}
	writeJobLogEntry(projectRoot, jobID, entry)
}

// writeJobLogEntry appends a single JSONL event entry to the job's events file.
// Test-bundle jobs (SCH-run-*): append to single shared .zqk/logs/scheduler/cvs/test-bundles/events.jsonl (each line includes job_id).
// Maintenance log jobs (maintenance-aggregate, maintenance-retention): always use direct write so each cycle is visible immediately (runner never closes the writer, so buffered writer would not flush for many cycles).
// Other jobs: use buffered writer per job (or fallback open-write-close). Best-effort; errors are ignored.
func writeJobLogEntry(projectRoot, jobID string, entry map[string]any) {
	if projectRoot == emptyValue || jobID == emptyValue {
		return
	}
	if IsTestBundleJob(jobID) {
		AppendTestBundleEvent(projectRoot, jobID, entry)
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	// Maintenance runner removed in V1.0 Hardening.
	useDirectWrite := false
	if !useDirectWrite {
		w, err := GetOrCreateJobLogWriter(projectRoot, jobID)
		if err == nil && w != nil {
			_ = w.WriteLine(data)
			return
		}
	}
	// Fallback / maintenance: direct write so each line is visible on disk
	logDir := JobLogDir(projectRoot, jobID)
	eventsFilePath := JobEventsFilePath(projectRoot, jobID)
	if mkErr := fileutil.MkdirAll(logDir, paths.DirPerm755); mkErr != nil {
		return
	}
	f, openErr := fileutil.OpenFile(eventsFilePath, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm644)
	if openErr != nil {
		return
	}
	_, _ = f.Write(data)
	_, _ = f.WriteString("\n")
	_ = f.Close()
	maxLines := config.GetMaxJobLogLines(projectRoot)
	trimJobLogFileIfNeeded(eventsFilePath, maxLines)
}

// trimJobLogFileIfNeeded keeps only the last maxLines lines in the JSONL file (rolling retention).
// Best-effort: errors are ignored.
func trimJobLogFileIfNeeded(logFilePath string, maxLines int) {
	if maxLines <= 0 {
		return
	}
	f, err := fileutil.Open(logFilePath)
	if err != nil {
		return
	}
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) != emptyValue {
			lines = append(lines, line)
		}
	}
	_ = f.Close()
	if sc.Err() != nil || len(lines) <= maxLines {
		return
	}
	keep := lines[len(lines)-maxLines:]
	w, err := fileutil.Create(logFilePath)
	if err != nil {
		return
	}
	defer w.Close()
	for _, line := range keep {
		_, _ = w.WriteString(line)
		_, _ = w.WriteString("\n")
	}
}

// dispatchContextForScheduledJob returns the context stored on triggeredJobWork for pool dispatch.
// When max_runtime_seconds > 0, this context uses an outer deadline (see getSchedulerDispatchResourceWaitMax,
// env ZQK_SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX, default 2h) so trigger-queue wait, pool backlog, and
// WaitUnderGoroutineCeiling do not inherit the execution budget; executeJob applies max_runtime_seconds
// from actual execution start via WithoutCancel + WithTimeout.
// When max_runtime_seconds == 0, a 30-minute deadline preserves a safety cap for legacy "unbounded" jobs.
func dispatchContextForScheduledJob(job *ScheduledJob) (context.Context, context.CancelFunc) {
	if job.MaxRuntimeSeconds <= 0 {
		return context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Minute)
	}
	return context.WithTimeout(pkgctx.NewSystemContext(), getSchedulerDispatchResourceWaitMax())
}
