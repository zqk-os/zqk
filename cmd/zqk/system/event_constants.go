package system

const (
	eventKeyPhase           = "phase"
	eventKeyCacheStatus     = "cache_status"
	eventKeyMessage         = "message"
	eventKeyEventType       = "event_type"
	eventKeyOperation       = "operation"
	eventKeySeverity        = "severity"
	eventKeyTargetKind      = "target_kind"
	eventKeyTargetID        = "target_id"
	eventKeyWorkerID        = "worker_id"
	eventKeyWorkerCount     = "worker_count"
	eventKeyProcessedCount  = "processed_count"
	eventKeyFailedCount     = "failed_count"
	eventKeyDurationSeconds = "duration_seconds"
	eventKeyDurationNS      = "duration_ns"
	eventKeyStatus          = "status"
	eventKeySource          = "source"
	eventKeyProjectRoot     = "project_root"
	eventKeyEntryCount      = "entry_count"
	eventKeyForceRebuild    = "force_rebuild"
	eventKeyBuildDuration   = "build_duration"
	eventKeySaveDuration    = "save_duration"
	eventKeyCacheType       = "cache_type"
	eventKeyAvailable       = "available"
	eventKeyCacheAvailable  = "cache_available"
	eventKeyCacheEvent      = "cache_event"
	eventKeyCacheOperation  = "cache_operation"
	eventKeyServiceName     = "service_name"
	eventKeyOperationType   = "operation_type"
	eventKeyChangeType      = "change_type"
	eventKeyObjectRef       = "object_ref"
	eventKeyKind            = "kind"
	eventKeyObjectID        = "object_id"
	eventKeyBatchSize       = "batch_size"
	eventKeyHashCount       = "hash_count"
	eventKeyError           = "error"
	eventKeyQueueName       = "queue_name"
	eventKeyPendingCount    = "pending_count"
	eventKeyIsCritical      = "is_critical"
	eventKeyDurationMS      = "duration_ms"
	eventKeyShutdownEvent   = "shutdown_event"
	eventKeyHasError        = "has_error"
	eventKeySuccessCount    = "success_count"
	eventKeyFailureCount    = "failure_count"
	eventKeyFailedFileCount = "failed_file_count"
)

const (
	eventTypeSystemCheck       = "system_check"
	eventTypeCacheOperation    = "cache_operation"
	eventTypeCacheAvailability = "cache_availability"
	eventTypeCacheSidecars     = "cache_sidecars"
	eventTypeAsyncValidation   = "async_validation"
	eventTypeOperationExecutor = "operation_executor"
	eventTypeHashRegistryBatch = "hash_registry_batch"
	eventTypeAsyncRouter       = "async_router"
)

const (
	eventStatusProgress = "progress"
	eventStatusStart    = "start"
	eventStatusComplete = "complete"
	eventStatusWarning  = "warning"
	eventStatusError    = "error"
	eventStatusLoading  = "loading"
	eventStatusInProg   = "in_progress"
)

const (
	severityLow    = "low"
	severityMedium = "medium"
	severityHigh   = "high"
)

const (
	targetKindCache   = "cache"
	targetKindService = "service"
)

const (
	sourceCacheBuild        = "cache_build"
	sourceCacheSave         = "cache_save"
	sourceCacheAvailability = "cache_availability"
	sourceBackgroundWorker  = "background_worker"
)

const (
	operationTypeOperationExecutor = "operation_executor"
	operationTypeAsyncRouter       = "async_router"
	eventTypeSystemConfigChange    = "system_config_change"
	operationIDIDQueueManager      = "id_queue_manager"
	operationIDQueueShutdown       = "queue_shutdown"
)

const (
	cacheTypeObjectID = "object_id_cache"
)
