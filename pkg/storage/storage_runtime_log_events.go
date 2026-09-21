package storage

// Log events for storage runtime paths (POL-CODE-007). Wire prefix + stable suffix per subsystem.

// CAS object recovery ([caspkg.RecoverCASObject], [caspkg.RecoverCASKind]).
const storageCASRecoveryWirePrefix = "storage_cas_recovery"

const (
	LogEventStorageCASRecoveryRecoveredFromIDBased = storageCASRecoveryWirePrefix + "_recovered_from_id_based_file"
	LogEventStorageCASRecoveryRecoveredHashUpdate  = storageCASRecoveryWirePrefix + "_recovered_hash_updated"
	LogEventStorageCASRecoveryRemovedFromIndex     = storageCASRecoveryWirePrefix + "_removed_missing_from_index"
	LogEventStorageCASRecoveryFailed               = storageCASRecoveryWirePrefix + "_recover_failed"
)

// File object storage Create / ensure ID / paths ([FileObjectStorage.Create], helpers).
const storageObjectCreateWirePrefix = "storage_object_create"

const (
	LogEventStorageObjectCreateEmptyProjectRoot                      = storageObjectCreateWirePrefix + "_empty_project_root"
	LogEventStorageObjectCreateValidatePrepareFailed                 = storageObjectCreateWirePrefix + "_validate_prepare_failed"
	LogEventStorageObjectCreateEnsureObjectIDFailed                  = storageObjectCreateWirePrefix + "_ensure_object_id_failed"
	LogEventStorageObjectCreatePreparePathFailed                     = storageObjectCreateWirePrefix + "_prepare_object_path_failed"
	LogEventStorageObjectCreateListingIndexFlushFailed               = storageObjectCreateWirePrefix + "_listing_index_flush_failed"
	LogEventStorageObjectCreateEnsureNoIDGenerating                  = storageObjectCreateWirePrefix + "_ensure_no_id_generating"
	LogEventStorageObjectCreateEnsureRandomComponentFailed           = storageObjectCreateWirePrefix + "_ensure_random_component_failed"
	LogEventStorageObjectCreateEnsureGeneratedCASID                  = storageObjectCreateWirePrefix + "_ensure_generated_cas_id"
	LogEventStorageObjectCreateEnsureGenerateIDFailed                = storageObjectCreateWirePrefix + "_ensure_generate_id_failed"
	LogEventStorageObjectCreateEnsureIDGeneratedOK                   = storageObjectCreateWirePrefix + "_ensure_id_generated_ok"
	LogEventStorageObjectCreateEnsureNormalizedRecursiveSchedulerJob = storageObjectCreateWirePrefix + "_ensure_normalized_recursive_scheduler_job_id"
	LogEventStorageObjectCreatePreparePathMkdirFailed                = storageObjectCreateWirePrefix + "_prepare_path_mkdir_failed"
	LogEventStorageObjectCreatePreparePathGetObjectPathFailed        = storageObjectCreateWirePrefix + "_prepare_path_get_object_file_path_failed"
	LogEventStorageObjectCreateHashRegistrySaveRetryExceeded         = storageObjectCreateWirePrefix + "_hash_registry_save_retry_exceeded_warn"
	LogEventStorageObjectCreateHashRegistryQueueFullRollback         = storageObjectCreateWirePrefix + "_hash_registry_queue_full_rollback"
	LogEventStorageObjectCreateHashRegistryPersistFailed             = storageObjectCreateWirePrefix + "_hash_registry_persist_failed"
	LogEventStorageObjectCreateCacheOperationFailed                  = storageObjectCreateWirePrefix + "_cache_operation_after_create_failed"
	LogEventStorageObjectCreateTouchProcessDirFailed                 = storageObjectCreateWirePrefix + "_touch_process_dir_failed"
)

// File object storage Delete ([FileObjectStorage.Delete], findDependents fallback).
const storageObjectDeleteWirePrefix = "storage_object_delete"

const (
	LogEventStorageObjectDeleteCacheAfterStreamFailed          = storageObjectDeleteWirePrefix + "_cache_operation_after_stream_delete_failed"
	LogEventStorageObjectDeleteCacheAfterDeletionFailed        = storageObjectDeleteWirePrefix + "_cache_operation_after_deletion_failed"
	LogEventStorageObjectDeleteTouchProcessDirFailed           = storageObjectDeleteWirePrefix + "_touch_process_dir_after_deletion_failed"
	LogEventStorageObjectDeleteReverseRefIndexMissFallbackScan = storageObjectDeleteWirePrefix + "_reverse_reference_index_miss_fallback_scan"
)

// File object storage transactions ([FileObjectTransaction.commitStageFinalize]).
const storageObjectTransactionWirePrefix = "storage_object_transaction"

const (
	LogEventStorageObjectTransactionDeleteFailedDuringCommit = storageObjectTransactionWirePrefix + "_delete_failed_during_transaction_commit"
)

// Deferred audit bulk flush ([FlushPendingAuditEvents], [buildPendingAuditInstances]).
const storageAuditBulkWirePrefix = "storage_audit_bulk"

const (
	LogEventStorageAuditBulkCreatePendingFailed     = storageAuditBulkWirePrefix + "_bulk_create_pending_failed"
	LogEventStorageAuditBulkBuildPendingFailedDebug = storageAuditBulkWirePrefix + "_build_pending_instance_failed"
)

// Storage factory backend selection ([NewStorageFactory], [createFileBackend]).
const storageFactoryWirePrefix = "storage_factory"

const (
	LogEventStorageFactoryGraphConnectionFailedFallbackFile = storageFactoryWirePrefix + "_graph_connection_failed_fallback_file"
	LogEventStorageFactoryUsingGraphBackendDebug            = storageFactoryWirePrefix + "_using_graph_storage_backend"
	LogEventStorageFactoryUsingFileBackendDebug             = storageFactoryWirePrefix + "_using_file_storage_backend"
)

// Bucketing strategies ([DefaultBucketStrategyRegistry], [FileObjectStorage.getBucketStrategyRegistry]).
const storageBucketingWirePrefix = "storage_bucketing"

const (
	LogEventStorageBucketingGetStrategyForKindFailedWarn  = storageBucketingWirePrefix + "_get_strategy_for_kind_failed_use_default"
	LogEventStorageBucketingEnsuredAllKindsInfo           = storageBucketingWirePrefix + "_ensured_all_system_kinds_have_strategies"
	LogEventStorageBucketingInitRegistryFailedWarn        = storageBucketingWirePrefix + "_init_bucket_strategy_registry_failed_use_defaults"
	LogEventStorageBucketingLoaderInitializingDebug       = storageBucketingWirePrefix + "_strategy_loader_initializing"
	LogEventStorageBucketingLoaderValidationFailedSkip    = storageBucketingWirePrefix + "_strategy_validation_failed_skipping"
	LogEventStorageBucketingLoaderMissingIDSkip           = storageBucketingWirePrefix + "_strategy_missing_id_skipping"
	LogEventStorageBucketingLoaderConflictSkipDuplicate   = storageBucketingWirePrefix + "_strategy_conflict_skipping_duplicate"
	LogEventStorageBucketingLoaderPartialIndexConflict    = storageBucketingWirePrefix + "_strategy_partial_index_due_to_conflicts"
	LogEventStorageBucketingLoaderInitializedDebug        = storageBucketingWirePrefix + "_strategy_loader_initialized"
	LogEventStorageBucketingStorageListSlowDebug          = storageBucketingWirePrefix + "_bucket_strategy_storage_list_slow"
	LogEventStorageBucketingStorageListTotalDurationDebug = storageBucketingWirePrefix + "_bucket_strategy_storage_list_total_duration"
)

// Write-behind WAL worker ([ObjectWriteBehindWorker]).
const storageWriteBehindWirePrefix = "storage_write_behind"

const (
	LogEventStorageWriteBehindWalReplayCancelled                = storageWriteBehindWirePrefix + "_wal_replay_cancelled"
	LogEventStorageWriteBehindWalReplayFailed                   = storageWriteBehindWirePrefix + "_wal_replay_failed"
	LogEventStorageWriteBehindWalStartupReplayCompleted         = storageWriteBehindWirePrefix + "_wal_startup_replay_completed"
	LogEventStorageWriteBehindWalPostStartupCompactionFailed    = storageWriteBehindWirePrefix + "_wal_post_startup_compaction_failed"
	LogEventStorageWriteBehindWalPostStartupCompactionCompleted = storageWriteBehindWirePrefix + "_wal_post_startup_compaction_completed"
	LogEventStorageWriteBehindBacklogStatus                     = storageWriteBehindWirePrefix + "_backlog_status"
	LogEventStorageWriteBehindWalCompactionFailed               = storageWriteBehindWirePrefix + "_wal_compaction_failed"
	LogEventStorageWriteBehindWalCompactionCompleted            = storageWriteBehindWirePrefix + "_wal_compaction_completed"
	LogEventStorageWriteBehindOwnerSkipped                      = storageWriteBehindWirePrefix + "_owner_skipped"
	LogEventStorageWriteBehindApplyRetryableDropped             = storageWriteBehindWirePrefix + "_apply_retryable_dropped"
	LogEventStorageWriteBehindApplyNonRetryableHeld             = storageWriteBehindWirePrefix + "_apply_non_retryable_held"
	LogEventStorageWriteBehindWriteCheckpointFailed             = storageWriteBehindWirePrefix + "_write_checkpoint_failed"
	LogEventStorageWriteBehindBacklogReplayFailed               = storageWriteBehindWirePrefix + "_backlog_replay_failed"
	LogEventStorageWriteBehindBufferAboveThreshold              = storageWriteBehindWirePrefix + "_buffer_above_threshold"
	LogEventStorageWriteBehindShutdownDrainApplyFailed          = storageWriteBehindWirePrefix + "_shutdown_drain_apply_failed"
	LogEventStorageWriteBehindShutdownDrainTimeout              = storageWriteBehindWirePrefix + "_shutdown_drain_timeout"
)

// Audit event helper ([CreateAuditEventWithBuilder], allowlist load).
const storageAuditEventsWirePrefix = "storage_audit_events"

const (
	LogEventStorageAuditAllowedTypesSpecLoadFailed       = storageAuditEventsWirePrefix + "_allowed_types_spec_load_failed"
	LogEventStorageAuditAllowedTypesMissingFieldDef      = storageAuditEventsWirePrefix + "_allowed_types_missing_event_type_field"
	LogEventStorageAuditAllowedTypesMissingValidation    = storageAuditEventsWirePrefix + "_allowed_types_missing_validation"
	LogEventStorageAuditAllowedTypesEnumNotFound         = storageAuditEventsWirePrefix + "_allowed_types_enum_not_found"
	LogEventStorageAuditStorageProviderUnavailable       = storageAuditEventsWirePrefix + "_storage_provider_unavailable"
	LogEventStorageAuditIDGenerateFailed                 = storageAuditEventsWirePrefix + "_audit_id_generate_failed"
	LogEventStorageAuditBuilderUnavailable               = storageAuditEventsWirePrefix + "_instance_builder_unavailable"
	LogEventStorageAuditBuildForBufferFailed             = storageAuditEventsWirePrefix + "_build_for_buffer_failed"
	LogEventStorageAuditBufferAddFailedImmediate         = storageAuditEventsWirePrefix + "_buffer_add_failed_write_immediate"
	LogEventStorageAuditBuildImmediateFailed             = storageAuditEventsWirePrefix + "_build_immediate_failed"
	LogEventStorageAuditDuplicateMerged                  = storageAuditEventsWirePrefix + "_duplicate_merged"
	LogEventStorageAuditDuplicateIdempotent              = storageAuditEventsWirePrefix + "_duplicate_idempotent_no_merge"
	LogEventStorageAuditDuplicateMergeSkippedPerformance = storageAuditEventsWirePrefix + "_duplicate_merge_skipped_performance"
	LogEventStorageAuditCreateViaProviderFailed          = storageAuditEventsWirePrefix + "_create_via_provider_failed"
	LogEventStorageAuditBufferInitFailedContinueDefaults = storageAuditEventsWirePrefix + "_buffer_init_failed_continue_defaults"
)

// CAS orphan cleanup queue worker ([caspkg.CASOrphanCleanupQueue]).
const storageCASOrphanCleanupWirePrefix = "storage_cas_orphan_cleanup" //nolint:gosec

const (
	LogEventStorageCASOrphanWakeWorker          = storageCASOrphanCleanupWirePrefix + "_wake_worker"
	LogEventStorageCASOrphanWorkerStarting      = storageCASOrphanCleanupWirePrefix + "_worker_starting"
	LogEventStorageCASOrphanWorkerStoppedIdle   = storageCASOrphanCleanupWirePrefix + "_worker_stopped_idle"
	LogEventStorageCASOrphanWorkerIdleShutdown  = storageCASOrphanCleanupWirePrefix + "_worker_idle_shutdown"
	LogEventStorageCASOrphanRequeueFailed       = storageCASOrphanCleanupWirePrefix + "_requeue_failed"
	LogEventStorageCASOrphanRequeuedForRetry    = storageCASOrphanCleanupWirePrefix + "_requeued_for_retry"
	LogEventStorageCASOrphanWorkerStoppedOK     = storageCASOrphanCleanupWirePrefix + "_worker_stopped_success"
	LogEventStorageCASOrphanShutdownWaitTimeout = storageCASOrphanCleanupWirePrefix + "_shutdown_wait_timeout"
	LogEventStorageCASOrphanRefusedSoleSurvivor = storageCASOrphanCleanupWirePrefix + "_refused_sole_survivor"
)
