package storage

// Additional POL-CODE-007 wire keys (split from storage_runtime_log_events.go for readability).

// Shared metric reconciliation (audit_aggregation_metric / change_journal aggregation paths).
const storageMetricInstanceWirePrefix = "storage_metric_instance"

const (
	LogEventStorageMetricInstanceSkipHashMismatch      = storageMetricInstanceWirePrefix + "_skip_metric_hash_mismatch_create_new"
	LogEventStorageMetricInstanceSkipStaleCASIndex     = storageMetricInstanceWirePrefix + "_skip_metric_stale_cas_index_create_new"
	LogEventStorageMetricInstanceCleanupStaleCASFailed = storageMetricInstanceWirePrefix + "_cleanup_stale_cas_index_entry_failed"
)

// Bundled object_spec migration ([MigrateBundledObjectSpecs]).
const storageBundledObjectSpecWirePrefix = "storage_bundled_object_spec_migration"

const (
	LogEventStorageBundledSpecReadFailed        = storageBundledObjectSpecWirePrefix + "_read_failed"
	LogEventStorageBundledSpecBuildFailed       = storageBundledObjectSpecWirePrefix + "_build_failed"
	LogEventStorageBundledSpecReadStorageFailed = storageBundledObjectSpecWirePrefix + "_read_storage_failed"
	LogEventStorageBundledSpecCreateFailed      = storageBundledObjectSpecWirePrefix + "_create_failed"
	LogEventStorageBundledSpecUpdateFailed      = storageBundledObjectSpecWirePrefix + "_update_failed"
	LogEventStorageBundledSpecFinishedInfo      = storageBundledObjectSpecWirePrefix + "_finished"
)

// Metric builders (CAS / file_lock / command_metric).
const storageMetricsWirePrefix = "storage_metrics"

const (
	LogEventStorageMetricsCASBuildFailed        = storageMetricsWirePrefix + "_cas_metric_build_failed"
	LogEventStorageMetricsFileLockBuildFailed   = storageMetricsWirePrefix + "_file_lock_metric_build_failed"
	LogEventStorageMetricsCommandBuildFailed    = storageMetricsWirePrefix + "_command_metric_build_failed"
	LogEventStorageFileLockAsyncStopLockTimeout = storageMetricsWirePrefix + "_file_lock_async_stop_lock_timeout"
)

// Object move / rename ([FileObjectStorage.Move]).
const storageObjectMoveWirePrefix = "storage_object_move"

const (
	LogEventStorageObjectMoveStatusLifecycleInfo               = storageObjectMoveWirePrefix + "_status_changed_for_new_kind_lifecycle"
	LogEventStorageObjectMoveHashRegistryLoadFailed            = storageObjectMoveWirePrefix + "_hash_registry_load_new_dir_failed"
	LogEventStorageObjectMoveRemoveOldFileFailed               = storageObjectMoveWirePrefix + "_remove_old_file_failed"
	LogEventStorageObjectMoveUpdateRefsFailed                  = storageObjectMoveWirePrefix + "_update_references_after_move_failed"
	LogEventStorageObjectMoveCacheFailed                       = storageObjectMoveWirePrefix + "_cache_operation_after_move_failed"
	LogEventStorageObjectMoveUpdateDependentsFailed            = storageObjectMoveWirePrefix + "_update_dependent_references_failed"
	LogEventStorageObjectMoveUpdateDependentsAfterRenameFailed = storageObjectMoveWirePrefix + "_update_dependent_references_after_rename_failed"
	LogEventStorageObjectMoveUpdateRefsAfterRenameFail         = storageObjectMoveWirePrefix + "_update_references_after_rename_failed"
)

// Helpers: persistence tracing, ID generation ([trackPersistenceStep], [generateID]).
const storageObjectHelpersWirePrefix = "storage_object_helpers"

const (
	LogEventStorageObjectHelpersReloadRegistryAfterVerifyFailed = storageObjectHelpersWirePrefix + "_reload_original_registry_after_verify_failed"
	LogEventStorageObjectHelpersPersistenceFailedIdempotent     = storageObjectHelpersWirePrefix + "_persistence_step_failed_idempotent"
	LogEventStorageObjectHelpersPersistenceFailed               = storageObjectHelpersWirePrefix + "_persistence_step_failed"
	LogEventStorageObjectHelpersPersistenceStepDebug            = storageObjectHelpersWirePrefix + "_persistence_step"
	LogEventStorageObjectHelpersPersistenceSlowInfo             = storageObjectHelpersWirePrefix + "_persistence_step_slow"
	LogEventStorageObjectHelpersPersistenceOneTimeInitDebug     = storageObjectHelpersWirePrefix + "_persistence_step_one_time_init"
	LogEventStorageObjectHelpersPersistenceCompletedDebug       = storageObjectHelpersWirePrefix + "_persistence_step_completed"
	LogEventStorageObjectHelpersGenerateIDStartingInfo          = storageObjectHelpersWirePrefix + "_generate_id_starting"
	LogEventStorageObjectHelpersGenerateIDCreatedGeneratorInfo  = storageObjectHelpersWirePrefix + "_generate_id_created_generator"
	LogEventStorageObjectHelpersGenerateIDDefaultStrategyInfo   = storageObjectHelpersWirePrefix + "_generate_id_got_default_strategy_config"
	LogEventStorageObjectHelpersGenerateIDBeforeNextIDInfo      = storageObjectHelpersWirePrefix + "_generate_id_about_to_call_generate_next_id"
	LogEventStorageObjectHelpersGenerateIDNextFailed            = storageObjectHelpersWirePrefix + "_generate_id_generate_next_id_failed"
	LogEventStorageObjectHelpersGenerateIDNextOKInfo            = storageObjectHelpersWirePrefix + "_generate_id_generate_next_id_succeeded"
)

// Documentation policy validator.
const storageDocumentationPolicyWirePrefix = "storage_documentation_policy"

const (
	LogEventStorageDocumentationPolicyWalkDirFailed = storageDocumentationPolicyWirePrefix + "_walk_directory_tree_failed"
)

// Audit aggregation service (non-shared messages).
const storageAuditAggregationWirePrefix = "storage_audit_aggregation"

const (
	LogEventStorageAuditAggregationSchemaDefaultWarn        = storageAuditAggregationWirePrefix + "_latest_schema_version_default"
	LogEventStorageAuditAggregationBuildInstanceFailedWarn  = storageAuditAggregationWirePrefix + "_build_instance_failed"
	LogEventStorageAuditAggregationFilterArchivedFailedWarn = storageAuditAggregationWirePrefix + "_filter_archived_from_cache_failed"
)

// Object update path ([FileObjectStorage.Update], write-behind visibility).
const storageObjectUpdateWirePrefix = "storage_object_update"

const (
	LogEventStorageObjectUpdateMutationClassifiedDebug          = storageObjectUpdateWirePrefix + "_mutation_classified"
	LogEventStorageObjectUpdateWBStreamCurrentFailed            = storageObjectUpdateWirePrefix + "_write_behind_stream_current_visibility_failed"
	LogEventStorageObjectUpdateWBRuntimeDeltaFailed             = storageObjectUpdateWirePrefix + "_write_behind_runtime_delta_visibility_failed"
	LogEventStorageObjectUpdateWBCASApplyFailed                 = storageObjectUpdateWirePrefix + "_write_behind_cas_apply_visibility_failed"
	LogEventStorageObjectUpdateHashQueueFullIDChangeRollback    = storageObjectUpdateWirePrefix + "_hash_registry_queue_full_id_change_rollback"
	LogEventStorageObjectUpdateHashPersistAfterIDChangeFailed   = storageObjectUpdateWirePrefix + "_hash_registry_persist_after_id_change_failed"
	LogEventStorageObjectUpdateRemoveOldFileAfterIDChangeFailed = storageObjectUpdateWirePrefix + "_remove_old_file_after_id_change_failed"
	LogEventStorageObjectUpdateCacheAfterIDChangeFailed         = storageObjectUpdateWirePrefix + "_cache_operation_after_id_change_failed"
	LogEventStorageObjectUpdateTestModeSerializationWarn        = storageObjectUpdateWirePrefix + "_test_mode_update_serialization_failed"
	LogEventStorageObjectUpdateCalcHashFailedWarn               = storageObjectUpdateWirePrefix + "_calculate_hash_for_updated_object_failed"
	LogEventStorageObjectUpdateHashQueueFullTestModeRollback    = storageObjectUpdateWirePrefix + "_hash_registry_queue_full_update_rollback_test_mode"
	LogEventStorageObjectUpdateHashPersistAfterUpdateTestFailed = storageObjectUpdateWirePrefix + "_hash_registry_persist_after_update_test_mode_failed"
	LogEventStorageObjectUpdateCacheAfterUpdateFailed           = storageObjectUpdateWirePrefix + "_cache_operation_after_update_failed"
	LogEventStorageObjectUpdateHashQueueFullOptimisticRollback  = storageObjectUpdateWirePrefix + "_hash_registry_queue_full_update_rollback_optimistic_locking"
	LogEventStorageObjectUpdateHashPersistAfterUpdateOptFailed  = storageObjectUpdateWirePrefix + "_hash_registry_persist_after_update_optimistic_locking_failed"
	LogEventStorageObjectUpdateCalcHashFailedErr                = storageObjectUpdateWirePrefix + "_calculate_hash_for_updated_object_error"
	LogEventStorageObjectUpdateHashQueueFullRollback            = storageObjectUpdateWirePrefix + "_hash_registry_queue_full_update_rollback"
	LogEventStorageObjectUpdateHashPersistAfterUpdateFailed     = storageObjectUpdateWirePrefix + "_hash_registry_persist_after_update_failed"
	LogEventStorageObjectUpdateTouchProcessDirFailedDebug       = storageObjectUpdateWirePrefix + "_touch_process_dir_after_update_failed"
)

// Change journal entry creation ([CreateChangeJournalEntryWithBuilder]).
const storageChangeJournalWirePrefix = "storage_change_journal"

const (
	LogEventStorageChangeJournalProviderCASRoutingWarn = storageChangeJournalWirePrefix + "_storage_provider_missing_cas_routing_skipped"
)

// Change journal reconstruction from snapshot.
const storageChangeJournalReconstructWirePrefix = "storage_change_journal_reconstruct"

const (
	LogEventStorageChangeJournalReconstructNoEntriesAfterSnapshotDebug = storageChangeJournalReconstructWirePrefix + "_no_entries_after_snapshot_using_current"
	LogEventStorageChangeJournalReconstructSkipBeforeSnapshotDebug     = storageChangeJournalReconstructWirePrefix + "_skip_entry_before_snapshot_timestamp"
	LogEventStorageChangeJournalReconstructReverseUpdateDebug          = storageChangeJournalReconstructWirePrefix + "_applied_reverse_update"
	LogEventStorageChangeJournalReconstructMissingPrevStateUpdateWarn  = storageChangeJournalReconstructWirePrefix + "_missing_previous_state_reverse_update"
	LogEventStorageChangeJournalReconstructReverseDeleteDebug          = storageChangeJournalReconstructWirePrefix + "_applied_reverse_delete"
	LogEventStorageChangeJournalReconstructMissingPrevStateDeleteWarn  = storageChangeJournalReconstructWirePrefix + "_missing_previous_state_reverse_delete"
	LogEventStorageChangeJournalReconstructObjectCreatedAfterSnapshot  = storageChangeJournalReconstructWirePrefix + "_object_created_after_snapshot_cannot_reconstruct"
	LogEventStorageChangeJournalReconstructImportEntryDebug            = storageChangeJournalReconstructWirePrefix + "_found_import_entry"
	LogEventStorageChangeJournalReconstructUnknownChangeTypeWarn       = storageChangeJournalReconstructWirePrefix + "_unknown_change_type"
	LogEventStorageChangeJournalReconstructStateAtTimestampInfo        = storageChangeJournalReconstructWirePrefix + "_reconstructed_state_at_timestamp"
)

// Change journal compaction batch job.
const storageChangeJournalCompactionWirePrefix = "storage_change_journal_compaction"

const (
	LogEventStorageChangeJournalCompactionStartingInfo          = storageChangeJournalCompactionWirePrefix + "_starting_for_window"
	LogEventStorageChangeJournalCompactionNoEntriesInfo         = storageChangeJournalCompactionWirePrefix + "_no_entries_in_window_skipped"
	LogEventStorageChangeJournalCompactionFoundEntriesInfo      = storageChangeJournalCompactionWirePrefix + "_found_entries_to_compact"
	LogEventStorageChangeJournalCompactionCompactedInfo         = storageChangeJournalCompactionWirePrefix + "_compacted_entries_to_artifact"
	LogEventStorageChangeJournalCompactionDeletingOrigInfo      = storageChangeJournalCompactionWirePrefix + "_deleting_original_entries"
	LogEventStorageChangeJournalCompactionDeleteOrigFailed      = storageChangeJournalCompactionWirePrefix + "_delete_original_entries_after_compaction_failed"
	LogEventStorageChangeJournalCompactionSomeDeletesFailedWarn = storageChangeJournalCompactionWirePrefix + "_some_original_entries_failed_to_delete"
)

// Change journal helper (builder create path).
const storageChangeJournalHelperWirePrefix = "storage_change_journal_helper"

const (
	LogEventStorageChangeJournalHelperFactoryFailedWarn      = storageChangeJournalHelperWirePrefix + "_storage_factory_failed_skipped"
	LogEventStorageChangeJournalHelperNilProviderWarn        = storageChangeJournalHelperWirePrefix + "_storage_provider_nil_skipped"
	LogEventStorageChangeJournalHelperBuilderUnavailableWarn = storageChangeJournalHelperWirePrefix + "_instance_builder_unavailable_skipped"
	LogEventStorageChangeJournalHelperBuildFailedWarn        = storageChangeJournalHelperWirePrefix + "_build_instance_failed_skipped"
	LogEventStorageChangeJournalHelperCreateFailedWarn       = storageChangeJournalHelperWirePrefix + "_create_via_provider_failed"
)

// Snapshot capture / proxy / replay queue.
const storageSnapshotWirePrefix = "storage_snapshot"

const (
	LogEventStorageSnapshotInitiatedInfo              = storageSnapshotWirePrefix + "_initiated"
	LogEventStorageSnapshotCapturingMetadataInfo      = storageSnapshotWirePrefix + "_capturing_metadata"
	LogEventStorageSnapshotReadObjectMetadataWarn     = storageSnapshotWirePrefix + "_read_object_for_metadata_failed"
	LogEventStorageSnapshotHashFromRegistryDebug      = storageSnapshotWirePrefix + "_hash_from_registry"
	LogEventStorageSnapshotHashFromContentDebug       = storageSnapshotWirePrefix + "_hash_calculated_from_content"
	LogEventStorageSnapshotGetHashWarn                = storageSnapshotWirePrefix + "_get_hash_failed"
	LogEventStorageSnapshotMetadataCapturedInfo       = storageSnapshotWirePrefix + "_metadata_captured"
	LogEventStorageSnapshotHashMismatchUsingCalcWarn  = storageSnapshotWirePrefix + "_hash_mismatch_using_calculated_hash"
	LogEventStorageSnapshotEndingInfo                 = storageSnapshotWirePrefix + "_ending"
	LogEventStorageSnapshotReplaySomeFailedWarn       = storageSnapshotWirePrefix + "_some_replay_operations_failed"
	LogEventStorageSnapshotCompletedInfo              = storageSnapshotWirePrefix + "_completed"
	LogEventStorageSnapshotProxyActivatedInfo         = storageSnapshotWirePrefix + "_proxy_activated_queued"
	LogEventStorageSnapshotProxyDeactivatedInfo       = storageSnapshotWirePrefix + "_proxy_deactivated_normal"
	LogEventStorageSnapshotProxyReadPendingWriteDebug = storageSnapshotWirePrefix + "_read_returning_pending_write_data"
	LogEventStorageSnapshotOpQueuedDebug              = storageSnapshotWirePrefix + "_operation_queued"
	LogEventStorageSnapshotOpReplayNoneDebug          = storageSnapshotWirePrefix + "_no_operations_to_replay"
	LogEventStorageSnapshotOpReplayStartingInfo       = storageSnapshotWirePrefix + "_replaying_queued_operations"
	LogEventStorageSnapshotOpReplayFailedWarn         = storageSnapshotWirePrefix + "_operation_replay_failed"
	LogEventStorageSnapshotOpReplayCompletedInfo      = storageSnapshotWirePrefix + "_operation_replay_completed"
)

// Stream stewardship periodic maintenance.
const storageStreamStewardshipWirePrefix = "storage_stream_stewardship"

const (
	LogEventStorageStreamStewardshipSpecIndexUnavailableWarn  = storageStreamStewardshipWirePrefix + "_spec_index_unavailable"
	LogEventStorageStreamStewardshipSpecDriftWarn             = storageStreamStewardshipWirePrefix + "_spec_config_drift"
	LogEventStorageStreamStewardshipRegistryCompactWarn       = storageStreamStewardshipWirePrefix + "_registry_compaction_failed_non_fatal"
	LogEventStorageStreamStewardshipSegmentGCInfo             = storageStreamStewardshipWirePrefix + "_segment_gc"
	LogEventStorageStreamStewardshipSegmentGCSummaryInfo      = storageStreamStewardshipWirePrefix + "_segment_gc_summary"
	LogEventStorageStreamStewardshipRuntimeDeltaBackfillInfo  = storageStreamStewardshipWirePrefix + "_runtime_delta_backfill"
	LogEventStorageStreamStewardshipRuntimeDeltaOverlayGCInfo = storageStreamStewardshipWirePrefix + "_runtime_delta_overlay_gc"
)

// CAS index reads and CRUD ([content_addressable_storage_types], [content_addressable_storage_crud]).
const storageCASIndexWirePrefix = "storage_cas_index"

const (
	LogEventStorageCASIndexReadListFailedEmptyWarn      = storageCASIndexWirePrefix + "_read_index_list_failed_empty"
	LogEventStorageCASIndexReadMappingsTimeoutEmptyWarn = storageCASIndexWirePrefix + "_read_mappings_timeout_or_error_empty_map"
	LogEventStorageCASIndexPersistMappingFailedWarn     = storageCASIndexWirePrefix + "_persist_mapping_failed_memory_preserved"
	LogEventStorageCASIndexUpdateNewObjectTimeoutWarn   = storageCASIndexWirePrefix + "_update_new_object_timeout"
	LogEventStorageCASIndexUpdateTimeoutCongestedWarn   = storageCASIndexWirePrefix + "_update_timeout_queue_congested"
	LogEventStorageCASIndexPostWriteRepairedWarn        = storageCASIndexWirePrefix + "_post_write_invariant_repaired"
	LogEventStorageCASIndexPostWriteRepairFailedWarn    = storageCASIndexWirePrefix + "_post_write_invariant_repair_failed"
)

// High-volume event cache.
const storageHighVolumeCacheWirePrefix = "storage_high_volume_cache"

const (
	LogEventStorageHighVolumeCacheLoadedDebug                  = storageHighVolumeCacheWirePrefix + "_loaded_from_disk"
	LogEventStorageHighVolumeCacheInvalidatedProjectDebug      = storageHighVolumeCacheWirePrefix + "_invalidated_for_project_rebuild"
	LogEventStorageHighVolumeCacheBuildingInfo                 = storageHighVolumeCacheWirePrefix + "_building"
	LogEventStorageHighVolumeCacheSaveFailedWarn               = storageHighVolumeCacheWirePrefix + "_save_failed"
	LogEventStorageHighVolumeCacheBuiltInfo                    = storageHighVolumeCacheWirePrefix + "_built"
	LogEventStorageHighVolumeCacheSkipKindNoCASEmptyDebug      = storageHighVolumeCacheWirePrefix + "_skip_kind_build_no_cas_or_empty"
	LogEventStorageHighVolumeCacheBuiltAllKindsInfo            = storageHighVolumeCacheWirePrefix + "_built_from_index_stream_all_kinds"
	LogEventStorageHighVolumeCacheListFailedWarn               = storageHighVolumeCacheWirePrefix + "_list_events_for_build_failed"
	LogEventStorageHighVolumeCacheBuildingFromListInfo         = storageHighVolumeCacheWirePrefix + "_building_from_list"
	LogEventStorageHighVolumeCacheBuildingCappedTimeoutInfo    = storageHighVolumeCacheWirePrefix + "_building_capped_for_timeout"
	LogEventStorageHighVolumeCacheBuildingFromStreamCappedInfo = storageHighVolumeCacheWirePrefix + "_building_from_stream_capped_ids"
	LogEventStorageHighVolumeCacheSkipStreamRecordDebug        = storageHighVolumeCacheWirePrefix + "_skip_stream_record_for_build"
)

// Hash registry coordinated save.
const storageHashRegistryWirePrefix = "storage_hash_registry"

const (
	LogEventStorageHashRegistrySaveTimeoutErr = storageHashRegistryWirePrefix + "_save_timeout"
)

// Hash mismatch autofix strategies.
const storageHashMismatchFixWirePrefix = "storage_hash_mismatch_fix"

const (
	LogEventStorageHashMismatchFixNotInIndexDebug           = storageHashMismatchFixWirePrefix + "_object_not_in_index_nothing_to_remove"
	LogEventStorageHashMismatchFixRemovedStaleInfo          = storageHashMismatchFixWirePrefix + "_removed_stale_index_entry"
	LogEventStorageHashMismatchFixRemovedStaleRecomputeInfo = storageHashMismatchFixWirePrefix + "_removed_stale_entry_recompute_strategy"
	LogEventStorageHashMismatchFixCannotInferKindWarn       = storageHashMismatchFixWirePrefix + "_cannot_infer_kind_from_object_id"
)

// Semantic snapshot brand validation.
const storageSemanticSnapshotWirePrefix = "storage_semantic_snapshot"

const (
	LogEventStorageSemanticSnapshotBrandIncompleteWarn = storageSemanticSnapshotWirePrefix + "_brand_definition_missing_name_or_token"
)

// WaitGroup manager (listing index coordination).
const storageWaitGroupWirePrefix = "storage_wait_group"

const (
	LogEventStorageWaitGroupCreateLockTimeoutWarn  = storageWaitGroupWirePrefix + "_create_failed_lock_timeout"
	LogEventStorageWaitGroupAddMissingSkipWarn     = storageWaitGroupWirePrefix + "_add_group_missing_skip"
	LogEventStorageWaitGroupDoneMissingSkipWarn    = storageWaitGroupWirePrefix + "_done_group_missing_skip"
	LogEventStorageWaitGroupGetInfoLockTimeoutWarn = storageWaitGroupWirePrefix + "_get_info_lock_timeout"
)

// Bulk delete optimized graph path.
const storageBulkDeleteWirePrefix = "storage_bulk_delete"

const (
	LogEventStorageBulkDeleteIDPatternsFailedFullGraphWarn = storageBulkDeleteWirePrefix + "_id_patterns_failed_full_dependency_graph"
	LogEventStorageBulkDeleteSkipGraphAllLeavesDebug       = storageBulkDeleteWirePrefix + "_skipping_dependency_graph_all_leaf_nodes"
	LogEventStorageBulkDeleteBuildingGraphInfo             = storageBulkDeleteWirePrefix + "_building_dependency_graph"
	LogEventStorageBulkDeleteFindDependentsFailedWarn      = storageBulkDeleteWirePrefix + "_find_dependents_failed"
	LogEventStorageBulkDeleteOrderDeterminedInfo           = storageBulkDeleteWirePrefix + "_deletion_order_determined"
	LogEventStorageBulkDeleteCompletedFastLeafInfo         = storageBulkDeleteWirePrefix + "_completed_fast_path_leaf_nodes"
	LogEventStorageBulkDeleteCompletedInfo                 = storageBulkDeleteWirePrefix + "_completed"
	LogEventStorageBulkDeleteRollbackFailedErr             = storageBulkDeleteWirePrefix + "_rollback_failed"
	LogEventStorageBulkDeleteAddNodeFailedWarn             = storageBulkDeleteWirePrefix + "_add_node_failed"
	LogEventStorageBulkDeleteAddDependencyFailedWarn       = storageBulkDeleteWirePrefix + "_add_dependency_failed"
	LogEventStorageBulkDeleteGetOrderFailedErr             = storageBulkDeleteWirePrefix + "_get_order_failed"
)

// Async bulk delete job manager.
const storageBulkDeleteAsyncWirePrefix = "storage_bulk_delete_async"

const (
	LogEventStorageBulkDeleteAsyncJobCreatedInfo   = storageBulkDeleteAsyncWirePrefix + "_job_created"
	LogEventStorageBulkDeleteAsyncJobCompletedInfo = storageBulkDeleteAsyncWirePrefix + "_job_completed"
	LogEventStorageBulkDeleteAsyncJobFailedErr     = storageBulkDeleteAsyncWirePrefix + "_job_failed"
	LogEventStorageBulkDeleteAsyncCleanupFailedErr = storageBulkDeleteAsyncWirePrefix + "_cleanup_failed"
	LogEventStorageBulkDeleteAsyncLockFailedErr    = storageBulkDeleteAsyncWirePrefix + "_lock_failed"
)

// Audit event buffer flush (aggregation).
const storageAuditBufferFlushWirePrefix = "storage_audit_buffer_flush"

const (
	LogEventStorageAuditBufferFlushGroupFailedWarn       = storageAuditBufferFlushWirePrefix + "_flush_aggregation_group_failed"
	LogEventStorageAuditBufferFlushValidateFailedWarn    = storageAuditBufferFlushWirePrefix + "_validate_aggregated_event_failed"
	LogEventStorageAuditBufferFlushValidationFailedWarn  = storageAuditBufferFlushWirePrefix + "_aggregated_event_validation_failed"
	LogEventStorageAuditBufferFlushWriteOrHashFailedWarn = storageAuditBufferFlushWirePrefix + "_write_or_register_hash_failed"
)

// Reverse reference index cache.
const storageReverseRefIndexWirePrefix = "storage_reverse_reference_index"

const (
	LogEventStorageReverseRefIndexLoadedDebug        = storageReverseRefIndexWirePrefix + "_loaded_from_disk"
	LogEventStorageReverseRefIndexSavedDebug         = storageReverseRefIndexWirePrefix + "_saved_to_disk"
	LogEventStorageReverseRefIndexSkipKindDirDebug   = storageReverseRefIndexWirePrefix + "_skipping_kind_dir_non_fatal"
	LogEventStorageReverseRefIndexBuiltFromScanDebug = storageReverseRefIndexWirePrefix + "_built_from_scan"
)

// Object WAL file ([object.wal] — replay/compaction helpers in object_wal.go).
const storageObjectWALWirePrefix = "storage_object_wal"

const (
	LogEventStorageObjectWALReplaySkipLineWarn           = storageObjectWALWirePrefix + "_replay_skip_line_invalid_format"
	LogEventStorageObjectWALReplayCompletedInfo          = storageObjectWALWirePrefix + "_replay_completed"
	LogEventStorageObjectWALCompactionSkipLineWarn       = storageObjectWALWirePrefix + "_compaction_skip_line_invalid"
	LogEventStorageObjectWALCompactionAppliedSeqZeroWarn = storageObjectWALWirePrefix + "_compaction_applied_seq_zero_checkpoint_missing"
	LogEventStorageObjectWALCompactionNoRemovableInfo    = storageObjectWALWirePrefix + "_compaction_no_entries_to_remove_all_unapplied"
	LogEventStorageObjectWALCompactionCompletedInfo      = storageObjectWALWirePrefix + "_compaction_completed"
)

// File object storage core ([FileObjectStorage] lifecycle, cache checker, WAL init).
const storageObjectFileWirePrefix = "storage_object_file"

const (
	LogEventStorageObjectFileSetCacheCheckerNilInfo        = storageObjectFileWirePrefix + "_set_cache_checker_nil_clearing"
	LogEventStorageObjectFileSetCacheCheckerSetInfo        = storageObjectFileWirePrefix + "_set_cache_checker_setting"
	LogEventStorageObjectFileCacheCheckerNowNilInfo        = storageObjectFileWirePrefix + "_cache_checker_now_nil"
	LogEventStorageObjectFileCacheCheckerNowSetInfo        = storageObjectFileWirePrefix + "_cache_checker_now_set"
	LogEventStorageObjectFileChangeNotifyHandlerErrDebug   = storageObjectFileWirePrefix + "_change_notification_handler_error"
	LogEventStorageObjectFileSpecEnsureReadyWarnDebug      = storageObjectFileWirePrefix + "_spec_loader_ensure_ready_warning"
	LogEventStorageObjectFileAuditBufferReadFailedWarn     = storageObjectFileWirePrefix + "_audit_buffer_file_storage_read_failed"
	LogEventStorageObjectFileWALInitFailedSyncFallbackWarn = storageObjectFileWirePrefix + "_wal_init_failed_sync_create_delete"
)

// Validation path ([validate before persistence]).
const storageObjectValidationWirePrefix = "storage_object_validation"

const (
	LogEventStorageObjectValidationBlockingCheckFailedWarn = storageObjectValidationWirePrefix + "_blocking_issues_check_failed"
	LogEventStorageObjectValidationErrorDetailDebug        = storageObjectValidationWirePrefix + "_validation_error_detail"
	LogEventStorageObjectValidationNonBlockingWorkflowWarn = storageObjectValidationWirePrefix + "_non_blocking_validation_errors_save_allowed"
	LogEventStorageObjectValidationBatchModeInfo           = storageObjectValidationWirePrefix + "_batch_mode_detection"
	LogEventStorageObjectValidationBatchRefWarn            = storageObjectValidationWirePrefix + "_reference_validation_batch_non_blocking"
	LogEventStorageObjectValidationNonBlockingRefWarn      = storageObjectValidationWirePrefix + "_reference_validation_non_blocking_save_allowed"
	LogEventStorageObjectValidationBeforePersistWarn       = storageObjectValidationWirePrefix + "_validation_error_before_persistence"
	LogEventStorageObjectValidationSkipInferRefKindWarn    = storageObjectValidationWirePrefix + "_skipping_reference_validation_cannot_infer_kind"
	LogEventStorageObjectValidationCacheCheckerResultDebug = storageObjectValidationWirePrefix + "_cache_checker_result"
)

// Additional audit event file/stream helpers (same wire prefix family as [storage_audit_events]).
const (
	LogEventStorageAuditSyncAfterWriteFailedWarn       = storageAuditEventsWirePrefix + "_sync_file_after_write_failed"
	LogEventStorageAuditCacheRefreshProviderFailedWarn = storageAuditEventsWirePrefix + "_cache_refresh_provider_unavailable"
	LogEventStorageAuditLintBypassProviderFailedWarn   = storageAuditEventsWirePrefix + "_lint_bypass_provider_unavailable"
)

// CAS object file ops ([apply CAS], post-sync cache).
const storageObjectCASWirePrefix = "storage_object_cas"

const (
	LogEventStorageObjectCASEnqueueOrphanCleanupFailedWarn  = storageObjectCASWirePrefix + "_enqueue_orphan_cleanup_failed_sync_removed"
	LogEventStorageObjectCASOrphanCleanupSyncFailedWarn     = storageObjectCASWirePrefix + "_sync_orphan_cleanup_failed_after_enqueue_failure"
	LogEventStorageObjectCASRegisterCachePostSyncFailedWarn = storageObjectCASWirePrefix + "_register_cache_post_sync_callback_failed"
)

// CAS corruption repair CLI/tooling.
const storageCASCorruptionRepairWirePrefix = "storage_cas_corruption_repair"

const (
	LogEventStorageCASCorruptionRepairedInfo = storageCASCorruptionRepairWirePrefix + "_repaired"
)

// Test teardown helper ([project_test_teardown] — still wire-keyed for log aggregation).
const storageProjectTestWirePrefix = "storage_project_test"

const (
	LogEventStorageProjectTestTeardownSkipGitWorktreeWarn = storageProjectTestWirePrefix + "_skip_destructive_teardown_git_worktree"
)

// Object-id-cache pending journal ([object_id_cache_pending]).
// TRACK: BLI-CEF-R2-REL-OIDCACHE-SWALLOW — load/persist I/O must not be silent.
const storageObjectIDCachePendingWirePrefix = "storage_object_id_cache_pending"

const (
	LogEventStorageObjectIDCachePendingLoadFailedWarn            = storageObjectIDCachePendingWirePrefix + "_load_failed"
	LogEventStorageObjectIDCachePendingPersistFailedWarn         = storageObjectIDCachePendingWirePrefix + "_persist_failed"
	LogEventStorageObjectIDCachePendingPersistSkippedLoadErrWarn = storageObjectIDCachePendingWirePrefix + "_persist_skipped_load_error"
)
