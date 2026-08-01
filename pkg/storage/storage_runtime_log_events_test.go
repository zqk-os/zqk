package storage

import (
	"strings"
	"testing"
)

func TestStorageRuntimeLogEvents_casRecoveryWirePrefix(t *testing.T) {
	want := storageCASRecoveryWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageCASRecoveryRecoveredFromIDBased,
		LogEventStorageCASRecoveryRecoveredHashUpdate,
		LogEventStorageCASRecoveryRemovedFromIndex,
		LogEventStorageCASRecoveryFailed,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_objectCreateWirePrefix(t *testing.T) {
	want := storageObjectCreateWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageObjectCreateEmptyProjectRoot,
		LogEventStorageObjectCreateValidatePrepareFailed,
		LogEventStorageObjectCreateEnsureObjectIDFailed,
		LogEventStorageObjectCreatePreparePathFailed,
		LogEventStorageObjectCreateListingIndexFlushFailed,
		LogEventStorageObjectCreateEnsureNoIDGenerating,
		LogEventStorageObjectCreateEnsureRandomComponentFailed,
		LogEventStorageObjectCreateEnsureGeneratedCASID,
		LogEventStorageObjectCreateEnsureGenerateIDFailed,
		LogEventStorageObjectCreateEnsureIDGeneratedOK,
		LogEventStorageObjectCreateEnsureNormalizedRecursiveSchedulerJob,
		LogEventStorageObjectCreatePreparePathMkdirFailed,
		LogEventStorageObjectCreatePreparePathGetObjectPathFailed,
		LogEventStorageObjectCreateHashRegistrySaveRetryExceeded,
		LogEventStorageObjectCreateHashRegistryQueueFullRollback,
		LogEventStorageObjectCreateHashRegistryPersistFailed,
		LogEventStorageObjectCreateCacheOperationFailed,
		LogEventStorageObjectCreateTouchProcessDirFailed,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_objectDeleteWirePrefix(t *testing.T) {
	want := storageObjectDeleteWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageObjectDeleteCacheAfterStreamFailed,
		LogEventStorageObjectDeleteCacheAfterDeletionFailed,
		LogEventStorageObjectDeleteTouchProcessDirFailed,
		LogEventStorageObjectDeleteReverseRefIndexMissFallbackScan,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_objectTransactionWirePrefix(t *testing.T) {
	want := storageObjectTransactionWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageObjectTransactionDeleteFailedDuringCommit,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_auditBulkWirePrefix(t *testing.T) {
	want := storageAuditBulkWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageAuditBulkCreatePendingFailed,
		LogEventStorageAuditBulkBuildPendingFailedDebug,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_factoryWirePrefix(t *testing.T) {
	want := storageFactoryWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageFactoryGraphConnectionFailedFallbackFile,
		LogEventStorageFactoryUsingGraphBackendDebug,
		LogEventStorageFactoryUsingFileBackendDebug,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_bucketingWirePrefix(t *testing.T) {
	want := storageBucketingWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageBucketingGetStrategyForKindFailedWarn,
		LogEventStorageBucketingEnsuredAllKindsInfo,
		LogEventStorageBucketingInitRegistryFailedWarn,
		LogEventStorageBucketingLoadLegacyConfigFailedWarn,
		LogEventStorageBucketingLoaderInitializingDebug,
		LogEventStorageBucketingLoaderValidationFailedSkip,
		LogEventStorageBucketingLoaderMissingIDSkip,
		LogEventStorageBucketingLoaderConflictSkipDuplicate,
		LogEventStorageBucketingLoaderPartialIndexConflict,
		LogEventStorageBucketingLoaderInitializedDebug,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_writeBehindWirePrefix(t *testing.T) {
	want := storageWriteBehindWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageWriteBehindWalReplayCancelled,
		LogEventStorageWriteBehindWalReplayFailed,
		LogEventStorageWriteBehindWalStartupReplayCompleted,
		LogEventStorageWriteBehindWalPostStartupCompactionFailed,
		LogEventStorageWriteBehindWalPostStartupCompactionCompleted,
		LogEventStorageWriteBehindBacklogStatus,
		LogEventStorageWriteBehindWalCompactionFailed,
		LogEventStorageWriteBehindWalCompactionCompleted,
		LogEventStorageWriteBehindApplyRetryableDropped,
		LogEventStorageWriteBehindApplyNonRetryableHeld,
		LogEventStorageWriteBehindWriteCheckpointFailed,
		LogEventStorageWriteBehindBacklogReplayFailed,
		LogEventStorageWriteBehindBufferAboveThreshold,
		LogEventStorageWriteBehindShutdownDrainApplyFailed,
		LogEventStorageWriteBehindShutdownDrainTimeout,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_auditEventsWirePrefix(t *testing.T) {
	want := storageAuditEventsWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageAuditAllowedTypesSpecLoadFailed,
		LogEventStorageAuditAllowedTypesMissingFieldDef,
		LogEventStorageAuditAllowedTypesMissingValidation,
		LogEventStorageAuditAllowedTypesEnumNotFound,
		LogEventStorageAuditStorageProviderUnavailable,
		LogEventStorageAuditIDGenerateFailed,
		LogEventStorageAuditBuilderUnavailable,
		LogEventStorageAuditBuildForBufferFailed,
		LogEventStorageAuditBufferAddFailedImmediate,
		LogEventStorageAuditBuildImmediateFailed,
		LogEventStorageAuditDuplicateMerged,
		LogEventStorageAuditDuplicateIdempotent,
		LogEventStorageAuditDuplicateMergeSkippedPerformance,
		LogEventStorageAuditCreateViaProviderFailed,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}

func TestStorageRuntimeLogEvents_casOrphanCleanupWirePrefix(t *testing.T) {
	want := storageCASOrphanCleanupWirePrefix + "_"
	for _, evt := range []string{
		LogEventStorageCASOrphanWakeWorker,
		LogEventStorageCASOrphanWorkerStarting,
		LogEventStorageCASOrphanWorkerStoppedIdle,
		LogEventStorageCASOrphanWorkerIdleShutdown,
		LogEventStorageCASOrphanRequeueFailed,
		LogEventStorageCASOrphanRequeuedForRetry,
		LogEventStorageCASOrphanWorkerStoppedOK,
		LogEventStorageCASOrphanShutdownWaitTimeout,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}
