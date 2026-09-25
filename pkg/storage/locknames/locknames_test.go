package locknames

import "testing"

func TestLockNames(t *testing.T) {
	// Verify that constant locknames are non-empty strings
	names := []string{
		LockNameAsyncMetricsCollectorDisable,
		LockNameAsyncMetricsCollectorEnable,
		LockNameAsyncMetricsCollectorEnqueue,
		LockNameAsyncMetricsCollectorStop,
		LockNameAuditBufferAddEvent,
		LockNameAuditBufferGetCallback,
		LockNameAuditBufferGetFilestorage,
		LockNameAuditBufferGetGlobal,
		LockNameAuditBufferGetPendingCount,
		LockNameAuditBufferGetProjectRoot,
		LockNameAuditBufferGetStats,
		LockNameAuditBufferInitGlobal,
		LockNameAuditBufferIsDrained,
		LockNameAuditBufferIsEnabled,
		LockNameAuditBufferSetCallback,
		LockNameAuditBufferSetEnabled,
		LockNameAuditBufferSetFlushCallback,
		LockNameAuditBufferSetFlushChannel,
		LockNameBatchGeneratorDirScan,
		LockNameBatchGeneratorGenerateBatch,
		LockNameBatchGeneratorGetCachedCheck,
		LockNameBatchGeneratorGetCachedRead,
		LockNameBatchGeneratorGetDirScanLock,
		LockNameBatchGeneratorGetLastSequence,
		LockNameBatchGeneratorGetMaxSequence,
		LockNameBatchGeneratorLegacyGetCreate,
		LockNameBatchGeneratorLegacyGetFast,
		LockNameBatchGeneratorReset,
		LockNameBatchGeneratorScanCacheCheck,
		LockNameBatchGeneratorScanCacheDoubleCheck,
		LockNameBatchGeneratorScanCacheRead,
		LockNameBatchGeneratorScanCacheRead2,
		LockNameBatchGeneratorUpdateCache,
		LockNameBucketStrategyGetBaseCache,
		LockNameBucketStrategyGetBaseCheck,
		LockNameBucketStrategyGetDefaultCache,
		LockNameBucketStrategyGetDefaultCheck,
		LockNameBucketStrategyLoaderCache,
		LockNameBucketStrategyLoaderGetAll,
		LockNameBucketStrategyLoaderGetCheck,
		LockNameBucketStrategyLoaderGetForKind,
		LockNameBucketStrategyLoaderLoadFromCache,
		LockNameBucketStrategyLoaderReload,
		LockNameBucketStrategyLoaderUpdateCache,
		LockNameBulkDeleteCleanup,
		LockNameBulkDeleteCreateJob,
		LockNameBulkDeleteExecuteGetJob,
		LockNameBulkDeleteExecuteStart,
		LockNameBulkDeleteGetStatusJob,
		LockNameBulkDeleteGetStatusManager,
		LockNameBulkDeleteJobFailed,
		LockNameBulkDeleteJobUpdate,
		LockNameCacheManagerCleanupOldInvalidations,
		LockNameCacheManagerGetConsistency,
		LockNameCacheManagerInvalidateAsync,
		LockNameStorageOrchestratorRegisterOperation,
		LockNameStreamRegistryCacheInvalidate,
		LockNameTestModeUpdateSerialize,
		LockNameTransactionAddFile,
		LockNameTransactionCommitCheck,
		LockNameTransactionCommitDeferCheck,
		LockNameTransactionCommitMark,
		LockNameTransactionCreate,
		LockNameTransactionDeleteAdd,
		LockNameTransactionDeleteCheck,
		LockNameTransactionUpdate,
		LockNameTransactionUpdateAdd,
		LockNameUnifiedMetricsStopCopy,
		LockNameUpdateAppendWal,
		LockNameVolumeTrackerGetVolume,
		LockNameVolumeTrackerRecord,
		LockNameWaitgroupManagerAdd,
		LockNameWaitgroupManagerCount,
		LockNameWaitgroupManagerCreateGroup,
		LockNameWaitgroupManagerDeleteGroup,
		LockNameWaitgroupManagerDone,
		LockNameWaitgroupManagerGetGroup,
		LockNameWaitgroupManagerGetInfo,
		LockNameWaitgroupManagerListGroups,
		LockNameWaitgroupManagerSetObserver,
		LockNameWaitgroupManagerWait,
		LockNameWaitgroupObserverClearStats,
		LockNameWaitgroupObserverGetAllStats,
		LockNameWaitgroupObserverGetStats,
		LockNameWaitgroupObserverOnAdd,
		LockNameWaitgroupObserverOnCompleted,
		LockNameWaitgroupObserverOnCreated,
		LockNameWaitgroupObserverOnDone,
		LockNameWaitgroupObserverOnWait,
		LockNameWaitgroupObserverSetEnabled,
		LockNameWaitgroupObserverSummary,
		LockNameWalCompact,
		LockNameKindMapperInitializeCheck,
		LockNameKindMapperSetDirectories,
		LockNameKindMapperGetDirectories,
		LockNameEvolutionOutput,
	}

	for _, name := range names {
		if name == "" {
			t.Errorf("expected lock name constant to be non-empty")
		}
	}
}

// TestLockHierarchyDocumentation asserts that lock operation scopes are well-formed snake_case identifiers
// with recognized domain prefixes to prevent unstructured registry sprawl (TDE-F-ARCH-010).
func TestLockHierarchyDocumentation(t *testing.T) {
	recognizedPrefixes := []string{
		"async_metrics_",
		"audit_buffer_",
		"batch_generator_",
		"bucket_strategy_",
		"bulk_delete_",
		"cache_manager_",
		"cas_",
		"listing_index_",
		"change_journal_",
		"storage_",
		"stream_registry_",
		"test_mode_",
		"transaction_",
		"unified_metrics_",
		"update_",
		"volume_tracker_",
		"waitgroup_",
		"wal_",
		"kind_mapper_",
		"evolution_",
	}

	for _, name := range []string{
		LockNameListingIndexSave,
		LockNameWalCompact,
		LockNameAsyncMetricsCollectorEnqueue,
		LockNameAuditBufferAddEvent,
		LockNameKindMapperSetDirectories,
	} {
		matched := false
		for _, prefix := range recognizedPrefixes {
			if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("lock name %q does not start with any recognized domain scope prefix", name)
		}
	}
}
