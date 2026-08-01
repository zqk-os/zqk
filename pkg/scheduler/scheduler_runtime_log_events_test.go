package scheduler

import (
	"strings"
	"testing"
)

func requireWirePrefix(t *testing.T, wirePrefix string, events []string) {
	t.Helper()
	want := wirePrefix + "_"
	for _, evt := range events {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestSchedulerRuntimeLogEvents_wirePrefixes(t *testing.T) {
	t.Parallel()
	requireWirePrefix(t, schedulerDaemonWirePrefix, []string{
		LogEventSchedulerDaemonMetricsSamplerInitFailed,
		LogEventSchedulerDaemonAsyncRouterFromProfileFailed,
		LogEventSchedulerDaemonMaintenanceWalCompactStartupFailed,
		LogEventSchedulerDaemonStarting,
		LogEventSchedulerDaemonPidWriteFailed,
		LogEventSchedulerDaemonLegacyStateMigrationFinishedWithError,
		LogEventSchedulerDaemonLegacyStateFilesMigrated,
		LogEventSchedulerDaemonStateBucketDirsMigrated,
		LogEventSchedulerDaemonCachePrewarmEnqueueStartFailed,
		LogEventSchedulerDaemonRoutingRulesLoadFailed,
		LogEventSchedulerDaemonRoutingRulesValidationErrorsFound,
		LogEventSchedulerDaemonRoutingRuleValidationRow,
		LogEventSchedulerDaemonRoutingRulesLoaded,
		LogEventSchedulerDaemonAsyncRouterStartFailed,
		LogEventSchedulerDaemonPermissionDeniedStop,
		LogEventSchedulerDaemonStopping,
		LogEventSchedulerDaemonKeepAliveRemoveFailed,
		LogEventSchedulerDaemonCronStopTimedOut,
		LogEventSchedulerDaemonTriggeredPoolStopTimedOut,
		LogEventSchedulerDaemonAsyncRouterStopFailed,
		LogEventSchedulerDaemonProcessGroupShutdownIssues,
		LogEventSchedulerDaemonStorageShutdownInitiateFailed,
		LogEventSchedulerDaemonQueueShutdownDrainIncomplete,
		LogEventSchedulerDaemonStorageShutdownFailed,
		LogEventSchedulerDaemonPidRemoveFailed,
		LogEventSchedulerDaemonStopped,
	})
	requireWirePrefix(t, schedulerLifecycleWirePrefix, []string{
		LogEventSchedulerLifecycleSchedulerJobCASReconcileReloadFailed,
		LogEventSchedulerLifecycleReloadJobsFailed,
		LogEventSchedulerLifecycleConfigReloadCLIRequested,
		LogEventSchedulerLifecycleLoadConfigWatchFailed,
		LogEventSchedulerLifecycleConfigUpdated,
		LogEventSchedulerLifecycleRescheduleAfterUnpauseFailed,
		LogEventSchedulerLifecycleTimerJobsRescheduledFlowing,
		LogEventSchedulerLifecycleKeepAliveWriteFailed,
		LogEventSchedulerLifecycleHighGoroutineCount,
		LogEventSchedulerLifecycleHighHeapMemory,
		LogEventSchedulerLifecycleCronNilRestarting,
		LogEventSchedulerLifecycleReloadJobsAfterCronRestartFailed,
		LogEventSchedulerLifecycleCronRestartedJobsReloaded,
		LogEventSchedulerLifecycleCriticalDaemonNotRunningTimerJobs,
		LogEventSchedulerLifecycleQueryAuditEventsHealthCheckFailed,
		LogEventSchedulerLifecycleMissedJobTriggerRecovery,
		LogEventSchedulerLifecycleMissedJobRecoveryDispatchCeiling,
		LogEventSchedulerLifecycleHealthMetricBuildFailed,
		LogEventSchedulerLifecycleHealthMetricRecordFailed,
		LogEventSchedulerLifecycleHealthMetricRecorded,
		LogEventSchedulerLifecycleCASReconcileSecondReloadFailed,
		LogEventSchedulerLifecycleManualTriggerJob,
	})
	requireWirePrefix(t, schedulerJobMgmtWirePrefix, []string{
		LogEventSchedulerJobMgmtJobsPausedOnlyManual,
		LogEventSchedulerJobMgmtFailedToScheduleJob,
		LogEventSchedulerJobMgmtScheduledJob,
		LogEventSchedulerJobMgmtFailedToHydrateJob,
		LogEventSchedulerJobMgmtCronExecutionPanicked,
		LogEventSchedulerJobMgmtCronDispatchSkippedCeiling,
		LogEventSchedulerJobMgmtCronDispatchSkippedPoolTimeout,
		LogEventSchedulerJobMgmtSkippingOneTimeImmediateAlreadyRun,
		LogEventSchedulerJobMgmtSchedulingOneTimeImmediate,
		LogEventSchedulerJobMgmtReExecutingImmediate,
		LogEventSchedulerJobMgmtRegisteredTriggeredJob,
		LogEventSchedulerJobMgmtInitialLoadSummary,
		LogEventSchedulerJobMgmtStartupBootstrapSubmitted,
		LogEventSchedulerJobMgmtStartupBootstrapSubmitFailed,
		LogEventSchedulerJobMgmtImmediateLoadChunking,
	})
	requireWirePrefix(t, schedulerJobExecWirePrefix, []string{
		LogEventSchedulerJobExecPanicked,
		LogEventSchedulerJobExecFailedToCommitTransaction,
		LogEventSchedulerJobExecFailedToRollbackTransaction,
		LogEventSchedulerJobExecRecoverableRetry,
		LogEventSchedulerJobExecDispatchContextExpiredBeforeStart,
		LogEventSchedulerJobExecTimedOutRunning,
		LogEventSchedulerJobExecDisablingOneTimeAfterSuccess,
		LogEventSchedulerJobExecFailedRereadJobUsingCached,
		LogEventSchedulerJobExecSkippingDisabledInStorage,
		LogEventSchedulerJobExecReExecutingOneTime,
		LogEventSchedulerJobExecCoordinationPolicyEvalFailed,
		LogEventSchedulerJobExecSkippingConflict,
		LogEventSchedulerJobExecSkippingRepeatedLockFailures,
		LogEventSchedulerJobExecFailedCreateJobLock,
		LogEventSchedulerJobExecRepeatedLockCreateFailures,
		LogEventSchedulerJobExecFailedCloseJobLock,
		LogEventSchedulerJobExecFailedTryAcquireJobLock,
		LogEventSchedulerJobExecRepeatedLockAcquireFailures,
		LogEventSchedulerJobExecJobLockHeldWaiting,
		LogEventSchedulerJobExecFailedAcquireLockTimeout,
		LogEventSchedulerJobExecRepeatedLockAcquireTimeoutFailures,
		LogEventSchedulerJobExecFailedAcquirePackageConcurrencySlot,
		LogEventSchedulerJobExecCoordinationRegisterFailed,
		LogEventSchedulerJobExecTransactionalMode,
		LogEventSchedulerJobExecFailedCreateTransactionalWrapper,
		LogEventSchedulerJobExecCompletedWithInternalIssues,
		LogEventSchedulerJobExecFailed,
		LogEventSchedulerJobExecCompleted,
		LogEventSchedulerJobExecFailedUpdateMetadata,
		LogEventSchedulerJobExecMetadataUpdateAttempted,
		LogEventSchedulerJobExecMetadataUpdated,
		LogEventSchedulerJobExecFailedDisableInStorage,
		LogEventSchedulerJobExecJobDisabledInStorage,
	})
	requireWirePrefix(t, schedulerTriggerQueueWirePrefix, []string{
		LogEventSchedulerTriggerQueueSkippedDuplicateEnqueue,
		LogEventSchedulerTriggerQueueRequestEnqueued,
		LogEventSchedulerTriggerQueueRequestsEnqueuedBatch,
		LogEventSchedulerTriggerQueueFailedClearAfterRead,
		LogEventSchedulerTriggerQueueDequeuedSummary,
		LogEventSchedulerTriggerQueueFailedDequeue,
		LogEventSchedulerTriggerQueueCASReconcileBeforeBatchFailed,
		LogEventSchedulerTriggerQueueReloadBeforeBatchFailed,
		LogEventSchedulerTriggerQueueSkippedDuplicateInBatch,
		LogEventSchedulerTriggerQueueProcessing,
		LogEventSchedulerTriggerQueueReloadRetry,
		LogEventSchedulerTriggerQueueFailedReloadBeforeRetry,
		LogEventSchedulerTriggerQueueRequeuedTestBundleRetry,
		LogEventSchedulerTriggerQueueFailedReenqueueTestBundle,
		LogEventSchedulerTriggerQueueSkippedStaleTestBundle,
		LogEventSchedulerTriggerQueueSkippedStaleMaintenance,
		LogEventSchedulerTriggerQueueSkippedNotInCache,
		LogEventSchedulerTriggerQueueFailedReenqueueNotFoundRetry,
		LogEventSchedulerTriggerQueueFailedReenqueuePoolFull,
		LogEventSchedulerTriggerQueuePoolFullRequeuedForPoll,
		LogEventSchedulerTriggerQueueFailedTriggerFromQueue,
		LogEventSchedulerTriggerQueueExcessiveMissingTestBundles,
	})
	requireWirePrefix(t, schedulerDispatchWirePrefix, []string{
		LogEventSchedulerDispatchReenqueueAfterDropFailed,
		LogEventSchedulerDispatchReenqueuedAfterDrop,
		LogEventSchedulerDispatchTestBundleReenqueuedAfterDrop,
	})
	requireWirePrefix(t, schedulerPolicyWirePrefix, []string{
		LogEventSchedulerPolicyReconcilingStaleInProgress,
	})
	requireWirePrefix(t, cvsPipelineTickSyncWirePrefix, []string{
		LogEventCVSPipelineTickSyncNoReplacementActive,
		LogEventCVSPipelineTickSyncRepointAfterCompletedFailed,
		LogEventCVSPipelineTickSyncRepointedAfterCompleted,
		LogEventCVSPipelineTickSyncSetTargetOnActivateFailed,
		LogEventCVSPipelineTickSyncSetTargetOnActivate,
		LogEventCVSPipelineTickSyncRepointMissingPriorFailed,
		LogEventCVSPipelineTickSyncRepointedPriorMissing,
		LogEventCVSPipelineTickSyncRepointTerminalPriorFailed,
		LogEventCVSPipelineTickSyncRepointedTerminalPrior,
	})
	requireWirePrefix(t, convergenceRoutingWirePrefix, []string{
		LogEventConvergenceRoutingReadSessionFailed,
		LogEventConvergenceRoutingKindNotSession,
	})
	requireWirePrefix(t, changeJournalPipelineWirePrefix, []string{
		LogEventChangeJournalPipelinePreExecHealthFailed,
		LogEventChangeJournalPipelineObjectCountAggressiveCleanup,
		LogEventChangeJournalPipelineAggressiveCleanupCompleted,
	})
	requireWirePrefix(t, schedulerJobLoaderWirePrefix, []string{
		LogEventSchedulerJobLoaderSkippingOneTimeAlreadyRun,
		LogEventSchedulerJobLoaderInvalidLogLevel,
		LogEventSchedulerJobLoaderInvalidPriority,
	})
	requireWirePrefix(t, schedulerNotificationsWirePrefix, []string{
		LogEventSchedulerNotificationsDesktopDisplayFailed,
	})
	requireWirePrefix(t, schedulerJobTypeViewCacheWirePrefix, []string{
		LogEventSchedulerJobTypeViewCacheMissingHandlers,
	})
	requireWirePrefix(t, schedulerTriggerParseWirePrefix, []string{
		LogEventSchedulerTriggerParseUnknownType,
	})
	requireWirePrefix(t, schedulerAuditEventsWirePrefix, []string{
		LogEventSchedulerAuditSkippedNoProjectRoot,
		LogEventSchedulerAuditSkippedNoStorage,
	})
}
