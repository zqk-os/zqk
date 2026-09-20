package storage

// POL-CODE-007 wire keys for EventLogger-style APIs ([logging.EventLogger].LogInfo / LogDebug / LogError).

// Scheduler integration ([scheduler_integration]).
const storageSchedulerIntegrationWirePrefix = "storage_scheduler_integration"

const (
	LogEventStorageSchedulerIntegrationCascadeUpdateDirectInfo               = storageSchedulerIntegrationWirePrefix + "_cascade_update_direct"
	LogEventStorageSchedulerIntegrationCacheInvalidationScheduleFallbackWarn = storageSchedulerIntegrationWirePrefix + "_cache_invalidation_schedule_fallback"
	LogEventStorageSchedulerIntegrationCascadeUpdateScheduleFallbackWarn     = storageSchedulerIntegrationWirePrefix + "_cascade_update_schedule_fallback"
	LogEventStorageSchedulerIntegrationCascadeUpdateDirectFailError          = storageSchedulerIntegrationWirePrefix + "_cascade_update_direct_failed"
)

// Object list main path ([List] debug timeline).
const storageListMainWirePrefix = "storage_list_main"

const (
	LogEventStorageListMainOperationStartingDebug       = storageListMainWirePrefix + "_operation_starting"
	LogEventStorageListMainHVTimeWindowCacheSparseDebug = storageListMainWirePrefix + "_hv_timewindow_cache_sparse_use_full_path"
	LogEventStorageListMainOperationDebug               = storageListMainWirePrefix + "_list_operation"
	LogEventStorageListMainKindDirMissingDebug          = storageListMainWirePrefix + "_kind_directory_does_not_exist"
	LogEventStorageListMainCollectingFilePathsDebug     = storageListMainWirePrefix + "_collecting_file_paths"
	LogEventStorageListMainParallelReadStartDebug       = storageListMainWirePrefix + "_starting_parallel_file_read"
	LogEventStorageListMainNoFilePathsDebug             = storageListMainWirePrefix + "_no_file_paths_to_parse"
	LogEventStorageListMainWorkersCompletedDebug        = storageListMainWirePrefix + "_all_list_workers_completed"
	LogEventStorageListMainCollectingResultsDebug       = storageListMainWirePrefix + "_collecting_results"
	LogEventStorageListMainAllResultsCollectedDebug     = storageListMainWirePrefix + "_all_results_collected"
	LogEventStorageListMainCollectedParsedSortDebug     = storageListMainWirePrefix + "_collected_parsed_objects_starting_sort"
	LogEventStorageListMainContextCancelledCollectWarn  = storageListMainWirePrefix + "_context_cancelled_during_result_collection"
	LogEventStorageListMainFailedReadFileWarn           = storageListMainWirePrefix + "_failed_to_read_file"
)

// List collection / bucketing walk ([collectFilePathsUsingStrategy]).
const storageListCollectionWirePrefix = "storage_list_collection"

const (
	LogEventStorageListCollectionWalkingBucketedDateRangeDebug = storageListCollectionWirePrefix + "_walking_bucketed_storage_date_range"
	LogEventStorageListCollectionContextCancelledWalkDebug     = storageListCollectionWirePrefix + "_context_cancelled_during_date_walk"
	LogEventStorageListCollectionFoundFilesDateSubdirDebug     = storageListCollectionWirePrefix + "_found_files_in_date_subdirectory"
	LogEventStorageListCollectionCompletedDateRangeWalkDebug   = storageListCollectionWirePrefix + "_completed_date_range_walk"
	LogEventStorageListCollectionOptimizedDateRangeWalkDebug   = storageListCollectionWirePrefix + "_using_optimized_date_range_walk"
	LogEventStorageListCollectionWalkingAllSubdirsDebug        = storageListCollectionWirePrefix + "_walking_all_subdirectories_no_time_filter"
	LogEventStorageListCollectionWalkErrorSkippingDebug        = storageListCollectionWirePrefix + "_walk_error_skipping_path"
	LogEventStorageListCollectionFoundYAMLFileDebug            = storageListCollectionWirePrefix + "_found_yaml_file"
	LogEventStorageListCollectionCollectedPathsStrategyDebug   = storageListCollectionWirePrefix + "_collected_file_paths_using_strategy"
)

// Queue shutdown coordinator.
const storageQueueShutdownWirePrefix = "storage_queue_shutdown"

const (
	LogEventStorageQueueShutdownRegisteringInfo         = storageQueueShutdownWirePrefix + "_registering_queue_for_coordination"
	LogEventStorageQueueShutdownInitiatingInfo          = storageQueueShutdownWirePrefix + "_initiating_graceful_shutdown_all_queues"
	LogEventStorageQueueShutdownNoQueuesInfo            = storageQueueShutdownWirePrefix + "_no_queues_registered"
	LogEventStorageQueueShutdownDrainingInfo            = storageQueueShutdownWirePrefix + "_draining_all_queues"
	LogEventStorageQueueShutdownAllDrainedSuccessInfo   = storageQueueShutdownWirePrefix + "_all_queues_drained_successfully"
	LogEventStorageQueueShutdownInitiateQueueFailedWarn = storageQueueShutdownWirePrefix + "_initiate_shutdown_for_queue_failed"
	LogEventStorageQueueShutdownDrainCtxCancelledWarn   = storageQueueShutdownWirePrefix + "_drain_context_cancelled_skip_wait"
	LogEventStorageQueueShutdownTimeoutForcingWarn      = storageQueueShutdownWirePrefix + "_shutdown_timeout_exceeded_forcing"
	LogEventStorageQueueShutdownSomeNotDrainedWarn      = storageQueueShutdownWirePrefix + "_some_queues_not_fully_drained"
	LogEventStorageQueueShutdownSomeDrainErrorsWarn     = storageQueueShutdownWirePrefix + "_some_queues_had_drain_errors"
	LogEventStorageQueueShutdownIncompleteOpsWarn       = storageQueueShutdownWirePrefix + "_queue_has_incomplete_operations"
)

// CLI notifier.
const storageCLINotifierWirePrefix = "storage_cli_notifier"

const (
	LogEventStorageCLINotifierOperationFailedErr = storageCLINotifierWirePrefix + "_operation_failed"
)

// Hash migration run ([hash_migration]).
const storageHashMigrationWirePrefix = "storage_hash_migration"

const (
	LogEventStorageHashMigrationFoundFilesInfo        = storageHashMigrationWirePrefix + "_found_hash_files_to_migrate"
	LogEventStorageHashMigrationSkippingIndexedDebug  = storageHashMigrationWirePrefix + "_skipping_hash_file_already_in_index"
	LogEventStorageHashMigrationRegistryUpdatedInfo   = storageHashMigrationWirePrefix + "_created_or_updated_hash_registry"
	LogEventStorageHashMigrationDryRunWouldUpdateInfo = storageHashMigrationWirePrefix + "_dry_run_would_create_or_update_registry"
	LogEventStorageHashMigrationRemovedOldFileInfo    = storageHashMigrationWirePrefix + "_removed_old_hash_file"
)

// WaitGroup observer (debug instrumentation).
const storageWaitGroupObserverWirePrefix = "storage_wait_group_observer"

const (
	LogEventStorageWaitGroupObserverCreatedDebug          = storageWaitGroupObserverWirePrefix + "_wait_group_created"
	LogEventStorageWaitGroupObserverAddDebug              = storageWaitGroupObserverWirePrefix + "_wait_group_add"
	LogEventStorageWaitGroupObserverDoneDebug             = storageWaitGroupObserverWirePrefix + "_wait_group_done"
	LogEventStorageWaitGroupObserverWaitDebug             = storageWaitGroupObserverWirePrefix + "_wait_group_wait"
	LogEventStorageWaitGroupObserverCompletedInfo         = storageWaitGroupObserverWirePrefix + "_wait_group_completed"
	LogEventStorageWaitGroupObserverDoneExceedsAddWarn    = storageWaitGroupObserverWirePrefix + "_done_called_more_than_add"
	LogEventStorageWaitGroupObserverWaitMismatchWarn      = storageWaitGroupObserverWirePrefix + "_wait_mismatched_add_done_counts"
	LogEventStorageWaitGroupObserverCompletedMismatchWarn = storageWaitGroupObserverWirePrefix + "_completed_mismatched_add_done_counts"
	LogEventStorageWaitGroupObserverWaitSlowWarn          = storageWaitGroupObserverWirePrefix + "_wait_unusually_long"
)

// Operation executor workers.
const storageOperationExecutorWirePrefix = "storage_operation_executor"

const (
	LogEventStorageOperationExecutorWorkerIdleShutdownInfo                = storageOperationExecutorWirePrefix + "_worker_shutdown_idle_timeout"
	LogEventStorageOperationExecutorCacheInvalidateFailedErr              = storageOperationExecutorWirePrefix + "_cache_invalidation_failed"
	LogEventStorageOperationExecutorCacheInvalidationScheduleFallbackWarn = storageOperationExecutorWirePrefix + "_cache_invalidation_schedule_fallback"
	LogEventStorageOperationExecutorDeferredHashFilePathWarn              = storageOperationExecutorWirePrefix + "_deferred_hash_file_path_failed"
	LogEventStorageOperationExecutorDeferredHashCompleteWarn              = storageOperationExecutorWirePrefix + "_deferred_hash_complete_failed"
	LogEventStorageOperationExecutorDeferredHashCompleteDeleteWarn        = storageOperationExecutorWirePrefix + "_deferred_hash_complete_failed_delete"
)

// Deferred hash update coordinator.
const storageDeferredHashWirePrefix = "storage_deferred_hash_update"

const (
	LogEventStorageDeferredHashUpdatedIntegrityInfo    = storageDeferredHashWirePrefix + "_updated_integrity_hash_and_cache"
	LogEventStorageDeferredHashUpdateReadyFailedErr    = storageDeferredHashWirePrefix + "_failed_to_update_hash_for_ready_object"
	LogEventStorageDeferredHashIDCacheUpdateFailedWarn = storageDeferredHashWirePrefix + "_id_cache_update_after_hash_failed"
	LogEventStorageDeferredHashForceStaleFailedWarn    = storageDeferredHashWirePrefix + "_force_hash_update_stale_object_failed"
)
