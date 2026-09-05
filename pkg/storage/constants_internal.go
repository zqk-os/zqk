package storage

import (
	"time"

	"github.com/lanceman/zqk/pkg/storage/audit"
)

// Global Constants
const (
	emptyValue = ""
)

// Bucketing Strategy Types and Constants
const (
	StrategyTypeChronological = "chronological"
	StrategyTypeState         = "state"
	StrategyTypeSize          = "size"
	StrategyTypeFirstLetter   = "first_letter"
	StrategyTypeComposite     = "composite"
	BucketSeparator           = "/"
	DefaultStrategyError      = "default (error)"
	DefaultUnknownID          = "unknown"
	ExtYaml                   = ".yaml"
)

// Granularities
const (
	GranularityMonthly    = "monthly"
	GranularityWeekly     = "weekly"
	GranularityDaily      = "daily"
	GranularityHourly     = "hourly"
	GranularityHalfHourly = "half_hourly"
	GranularityQtrHourly  = "qtr_hourly"
	GranularityTenths     = "tenths"
)

// FieldKeys for audit aggregation and buffer
const (
	FieldKeyAggEventCount         = "event_count"
	FieldKeyAggEventTypeCounts    = "event_type_counts"
	FieldKeyAggStatusCounts       = "status_counts"
	FieldKeyAggObjectKindCounts   = "object_kind_counts"
	FieldKeyAggOperationCounts    = "operation_counts"
	FieldKeyAggAggregatedEventIDs = "aggregated_event_ids"
	FieldKeyAggCollectionCount    = "collection_count"
	FieldKeyAggLastSeen           = "last_seen"
	FieldKeyAggErrorEventCount    = "error_event_count"
	FieldKeyAggErrorRate          = "error_rate"
	FieldKeyAggTitle              = "title"
	FieldKeyAggregated            = "aggregated"
	FieldKeyAggCount              = "count"
	FieldKeyCount                 = "count"
	FieldKeyFirstOccurrence       = "first_occurrence"
	FieldKeyAggFirstOccurrence    = "first_occurrence"
	FieldKeyLastOccurrence        = "last_occurrence"
	FieldKeyAggLastOccurrence     = "last_occurrence"
	FieldKeyAggAggregationKey     = "aggregation_key"
	FieldKeyAggGroupCount         = "group_count"
	FieldKeyAggTotalEvents        = "total_events"
	FieldKeyAggGroups             = "groups"
	FieldKeyAggWindowSize         = "window_size"
	FieldKeyAggThreshold          = "threshold"
	FieldKeyAggKey                = "key"
	FieldKeyOriginalEventType     = "original_event_type"
	FieldKeyGranularity           = "granularity"
)

// Runner Names
const (
	RunnerNameBucketStrategyLoader = "bucket_strategy_loader"
)

// Metric Metadata Keys
const (
	MetricMetaEventsCreatedCAS   = "events_created_cas"
	MetricMetaEventsCreatedID    = "events_created_id"
	MetricMetaEventsValidated    = "events_validated"
	MetricMetaEventsSkipped      = "events_skipped"
	MetricMetaEventsDuplicated   = "events_duplicated"
	MetricMetaEventsMerged       = "events_merged"
	MetricMetaMaxCreationTimeMs  = "max_creation_time_ms"
	MetricMetaCasUsageRate       = "cas_usage_rate"
	MetricMetaFailureRate        = "failure_rate"
	MetricMetaValidationSkipRate = "validation_skip_rate"
	MetricMetaCreationEvents     = "creation_events"
	MetricMetaUpdateEvents       = "update_events"
	MetricMetaDeleteEvents       = "delete_events"
	MetricMetaBulkEvents         = "bulk_events"
	MetricMetaSystemEvents       = "system_events"
	MetricMetaOtherEvents        = "other_events"
	MetricMetaMeasurementPeriod  = "measurement_period"
	MetricMetaMetadata           = "metadata"
)

// Statuses and Values
const (
	ValueStatusCompleted      = "completed"
	ValueStatusFailed         = "failed"
	ValueStatusReverted       = "reverted"
	ValueStatusError          = "error"
	ValueStatusArchived       = "archived"
	ValueStatusSuccess        = "success"
	ValueStatusStart          = "start"
	ValueStatusComplete       = "complete"
	ValueStatusProposed       = "proposed"
	ValueUnknown              = "unknown"
	ValueSystem               = "system"
	ValueAuditEventBuffer     = "audit_event_buffer"
	ValueAuditAggregationJob  = "audit_aggregation_job"
	ValueWorker               = "worker"
	ValueAuditEventCollection = "audit_event_collection"
	SeverityLow               = audit.SeverityLow
	SeverityMedium            = audit.SeverityMedium
	SeverityHigh              = audit.SeverityHigh
)

// Event Types (canonical values live in pkg/storage/audit)
const (
	EventTypeAggregatedSummary     = audit.EventTypeAggregatedSummary
	EventTypeSchedulerJobStarted   = audit.EventTypeSchedulerJobStarted
	EventTypeSchedulerJobCompleted = audit.EventTypeSchedulerJobCompleted
	EventTypeCacheInvalidation     = audit.EventTypeCacheInvalidation
	EventTypeCacheUpdate           = audit.EventTypeCacheUpdate
	EventTypeCacheBulkInvalidation = audit.EventTypeCacheBulkInvalidation
	EventTypeObjectCreation        = audit.EventTypeObjectCreation
	EventTypeObjectUpdate          = audit.EventTypeObjectUpdate
	EventTypeObjectDeletion        = audit.EventTypeObjectDeletion
	EventTypeBulkOperation         = audit.EventTypeBulkOperation
	EventTypeSystemConfigChange    = audit.EventTypeSystemConfigChange
	EventTypeCacheRefresh          = audit.EventTypeCacheRefresh
	EventTypeCodeQualityBypass     = audit.EventTypeCodeQualityBypass
	KindAuditMetric                = "audit_metric"
)

// Metric and Metadata Keys
const (
	FieldKeyUsedCAS     = "used_cas"
	FieldKeyDurationNS  = "duration_ns"
	FieldKeySuccess     = "success"
	FieldKeyTimestamp   = "timestamp"
	FieldKeyProjectRoot = "project_root"
	FieldKeyLoc         = "loc"
)

// Suffixes and Separators
const (
	SeparatorPathOffset          = "::"
	SuffixTempFile               = ".tmp"
	PrefixAudit                  = audit.Prefix
	PrefixAuditAggregation       = "AAG-"
	PrefixAuditAggregationMetric = "AAM-"
	PrefixCommand                = "CMD-"
	SeparatorRange               = audit.RangeSeparator
)

// Operation and Component Names
const (
	OpNameAuditAggregationAggregateAuditEvents = "storage.audit_aggregation_aggregate_audit_events"
	OpNameAuditAggregationMetricAsync          = "audit_aggregation_metric_async"
	OpNameAuditEventBufferFlushGroup           = "audit_event_buffer_flush_group"
	OpNameAuditBufferFlush                     = "audit_buffer_flush"
	OpNameAuditBufferCoordinatorEvent          = "audit_buffer_coordinator_event"
	OpNameAuditEventBufferPeriodicFlush        = "audit_event_buffer_periodic_flush"
	OpNameAuditMetricsEventEmitter             = "audit_metrics_event_emitter"
	OpNameAuditMetricsCollectorAsync           = "audit_metrics_collector_async"
	OpNameLoadAllStrategies                    = "load_all_strategies"
)

// Error Messages and Notes
const (
	ErrMsgInitKindMapper               = "failed to initialize kind mapper"
	ErrMsgProcessEventsBatch           = "failed to process events in batches"
	ErrMsgCreateAggMetric              = "failed to create aggregation metric"
	ErrMsgLoadIDPatterns               = "failed to load ID patterns"
	ErrMsgUpdateAggMetric              = "failed to update existing aggregation metric"
	ErrMsgFormatAggEvent               = "failed to format aggregated event"
	ErrMsgWriteAggEvent                = "failed to write aggregated event or register hash"
	ErrMsgReadConfig                   = "failed to read config file"
	ErrMsgCreateAuditDir               = "failed to create audit directory"
	ErrMsgNextAuditID                  = "failed to find next audit ID"
	ErrMsgCopyBuffer                   = "copy buffer for flush"
	ErrMsgFlushCASIndex                = "Failed to flush CAS index for audit events"
	ErrMsgEmitAuditMetric              = "failed to emit audit metric event"
	ErrMsgRecordAuditMetric            = "failed to record audit metric"
	ErrMsgInvalidMetricMerge           = "invalid metric type for merging"
	ErrMsgInvalidAggMetric             = "invalid aggregation metric type"
	ErrMsgMetricTimeout                = "metric creation timeout after %v"
	ErrMsgUpdateAggMetricWin           = "failed to update existing aggregation metric for window %s to %s: %w"
	ErrMsgOverlapAggMetric             = "overlapping audit_aggregation_metric window for source %s: existing metric %s overlaps [%s, %s]"
	ErrMsgReadHashFile                 = "failed to read hash file"
	ErrMsgBlockingIssues               = "blocking issues detected"
	ErrMsgMetricIDNotSet               = "metric ID not set after creation"
	ErrMsgLocateAggMetric              = "object already exists but could not locate existing metric for time window %s to %s: %w"
	ErrMsgQueryAggEvents               = "failed to query old aggregated events"
	ErrMsgCacheBuildFailed             = "⚠️ [STORAGE-AUDIT] High-volume event cache build failed: %v\n"
	ErrMsgQueryAuditAge                = "failed to query old audit events by age"
	ErrMsgDeleteEventsFmt              = "failed to delete events: %d failed, %d succeeded. First error: %v"
	ErrMsgDeleteEventsNoDet            = "failed to delete events: %d failed, %d succeeded (no error details available)"
	ErrMsgInvalidDurFmt                = "invalid duration format: %s"
	ErrMsgInvalidDurVal                = "invalid duration value: %s"
	ErrMsgPeriodicFlushFail            = "Periodic audit buffer flush failed"
	ErrMsgReadLockFailInd              = "Failed to acquire read lock for individual group flush"
	ErrMsgNoFileStorage                = "audit_event buffer flush requires fileStorage when stream storage is enabled (cannot write to stream without storage)"
	ErrMsgNoExist                      = "no such file or directory"
	ErrMsgBuildCache                   = "failed to build high-volume event cache"
	ErrMsgIDPatternNotFound            = "audit_aggregation_metric ID pattern not found - ensure the spec file exists and has id_template or id_prefixes defined"
	ErrMsgLockGetGlobal                = "lock failed in GetGlobalAuditEventBuffer: %v\n"
	ErrMsgFlushFailRoot                = "flush failed for project root %s: %v\n"
	ErrMsgLockSetEnabled               = "lock failed in SetEnabled: %v\n"
	ErrMsgLockIsEnabled                = "lock failed in IsEnabled: %v\n"
	ErrMsgLockSetFlushErr              = "lock failed in SetFlushErrorCallback: %v\n"
	ErrMsgLockSetFlushCh               = "lock failed in SetFlushProgressChannel: %v\n"
	ErrMsgLockGetProjRoot              = "lock failed in GetProjectRoot: %v\n"
	ErrMsgLockIsDrained                = "lock failed in IsDrained: %v\n"
	ErrMsgLockGetPending               = "lock failed in GetPendingCount: %v\n"
	ErrMsgLockGetBufferStats           = "lock failed in GetBufferStats: %v\n"
	ErrMsgLockSetAuditFlush            = "lock failed in SetAuditBufferFlushEventCallback: %v\n"
	ErrMsgLockGetAuditFlush            = "lock failed in getAuditBufferFlushEventCallback: %v\n"
	ErrMsgEmptyContent                 = "cannot write system object with empty content: %s"
	ErrMsgStreamBacked                 = "kind %q is stream-backed; do not use WriteSystemObjectAndRegisterHash when stream storage is enabled (use storage.Create or writeObjectToStorage to avoid creating YAML under %s)"
	ErrMsgNoCAS                        = "failed to get content-addressable storage for kind %s (CAS required, cannot fall back to direct write): %w"
	ErrMsgCreateCAS                    = "failed to create object in content-addressable storage"
	ErrMsgCreateDir                    = "failed to create directory"
	ErrMsgOpenFileWrite                = "failed to open file for writing"
	ErrMsgWriteFile                    = "failed to write file"
	ErrMsgGetHashReg                   = "failed to get or create hash registry for %s"
	ErrMsgUpdateHashReg                = "failed to update hash registry"
	ErrMsgSaveHashReg                  = "hash registry save failed after %d attempts: %w"
	ErrMsgReloadHashReg                = "failed to reload hash registry for verification"
	ErrMsgHashRegPathEmpty             = "hash registry file path is empty"
	ErrMsgHashRegNoExist               = "hash registry file does not exist after save"
	ErrMsgHashNotFound                 = "hash for %s not found in reloaded registry"
	ErrMsgHashMismatch                 = "hash mismatch for %s: expected %s, got %s"
	ErrMsgAlreadyExists                = "already exists"
	ErrMsgObjectNotFound               = "object not found"
	ErrMsgGetLatestSchemaFmt           = "failed to get latest schema version for command_metric"
	ErrMsgBuildCommandMetric           = "failed to build command_metric instance"
	ErrMsgStorageNotConfigured         = "storage not configured for audit metrics collector"
	ErrMsgCreateMetricObject           = "failed to create audit metric object"
	ErrMsgLockFailedGen                = "lock failed: %v\n"
	ErrMsgSwallowedError               = "swallowed error: %v\n"
	ErrMsgStatHashReg                  = "failed to stat hash registry"
	ErrMsgReadHashRegistry             = "failed to read hash registry"
	ErrMsgParseHashReg                 = "failed to parse hash registry"
	ErrMsgMarshalHashReg               = "failed to marshal hash registry"
	ErrMsgCreateHashRegDir             = "failed to create directory for hash registry"
	ErrMsgOpenTempFile                 = "failed to open temp file for writing"
	ErrMsgWriteHashRegData             = "failed to write hash registry data"
	ErrMsgRemoveTempFile               = "failed to remove temp file: %v\n"
	ErrMsgEnsureDirBeforeRename        = "failed to ensure directory exists before rename"
	ErrMsgSyncDirectory                = "failed to sync directory: %v\n"
	ErrMsgDirNotAccessible             = "directory does not exist or is not accessible"
	ErrMsgEnsureDirHashReg             = "failed to ensure directory exists for hash registry"
	ErrMsgTempFileNotExist             = "temp file does not exist before rename"
	ErrMsgRenameTempFile               = "failed to rename temp file to hash registry file"
	ErrMsgRenameNotFoundRetry          = "failed to rename (file not found, will retry)"
	ErrMsgSaveHashRegRetries           = "failed to save hash registry after retries: %v\n"
	ErrMsgDeleteRequiresCLI            = "delete operations must be performed through CLI (unauthorized direct API call)"
	ErrMsgPermissionDeniedDelete       = "permission denied: %w (delete operations require explicit authorization)"
	ErrMsgProtectedDelete              = "cannot delete SCH-maintenance-wal: maintenance WAL trigger job is protected (required for aggregation and retention)"
	ErrMsgCheckDeps                    = "failed to check dependencies"
	ErrMsgUnlinkRefsFail               = "unlink references before delete failed"
	ErrMsgCheckDepsAfterUnlink         = "failed to check dependencies after unlink"
	ErrMsgCannotDeleteDeps             = "cannot delete object %s: %d dependent object(s) still reference it (use --unlink-references to remove references from dependents, or cascade=true to delete dependents)"
	ErrMsgCascadeDeleteFail            = "failed to cascade delete dependent %s: %w"
	ErrMsgEnqueueDeleteFail            = "failed to enqueue delete for write-behind"
	ErrMsgDeleteCASFail                = "failed to delete object in content-addressable storage"
	ErrMsgDeleteFile                   = "failed to delete file"
	ErrMsgReadProcessDir               = "failed to read process directory"
	ErrMsgFindDependents               = "failed to find dependents"
	ErrMsgUnknownKind                  = "unknown object kind: %s"
	ErrMsgInvalidDirMapping            = "invalid directory mapping for kind %s: %q"
	ErrMsgHashMismatchVerify           = "content hash mismatch: expected %s, got %s (file may be corrupted or tampered)"
	ErrMsgMarshalYAML                  = "failed to marshal YAML"
	ErrMsgEnqueueWriteFail             = "failed to enqueue write operation: %w; direct write failed: %w"
	ErrMsgWriteTimeout                 = "write operation timed out and direct write failed"
	ErrMsgParseYAML                    = "failed to parse YAML"
	ErrMsgEnqueueReadFail              = "failed to enqueue read operation: %w; direct read failed: %w"
	ErrMsgReadTimeout                  = "read operation timed out and direct read failed"
	ErrMsgOpenFile                     = "failed to open file"
	ErrMsgAccountIDRequired            = "account_id is required for keystore_entry"
	ErrMsgInferKindFailed              = "could not infer kind from ID: %s"
	ErrMsgPermUpdateKeystore           = "permission denied: you can only update your own keystore entries"
	ErrMsgPermUpdateCredHash           = "permission denied: only system can update credential_hash"
	ErrMsgPermUpdateSalt               = "permission denied: only system can update salt"
	ErrMsgObjectExistsFmt              = "object with ID %s already exists"
	ErrMsgCheckNewIDExists             = "failed to check if new ID exists"
	ErrMsgValidationFailed             = "validation failed"
	ErrMsgMarshalUpdatedObjWB          = "failed to marshal updated object for write-behind"
	ErrMsgEnqueueUpdateWB              = "failed to enqueue update for write-behind"
	ErrMsgMarshalUpdatedObj            = "failed to marshal updated object"
	ErrMsgWriteRuntimeDelta            = "failed to write runtime-delta current state"
	ErrMsgUpdateCASIDChange            = "failed to update object in content-addressable storage with ID change"
	ErrMsgUpdateCAS                    = "failed to update object in content-addressable storage"
	ErrMsgDeleteCASIDChange            = "failed to delete object in content-addressable storage with ID change"
	ErrMsgReadNewFileForHash           = "failed to read new file back for hash calculation"
	ErrMsgWriteObjNewLoc               = "failed to write object to new location"
	FieldKeyExpectedUpdatedAt          = "expected_updated_at"
	FieldKeyMutationClass              = "mutation_class"
	FieldKeyRequestedFields            = "requested_fields"
	FieldKeyEffectiveFields            = "effective_fields"
	NoteStaleCASIndex                  = "CAS index entry exists but hash file is missing - recovery needed"
	NoteStaleAggMetric                 = "failed to create aggregation metric after update failure (stale CAS index)"
	NoteMetricNoID                     = "metric ID not set after creation"
	NoteNoExistingMetric               = "no existing metric found for time window"
	ErrMsgGetCAS                       = "failed to get content-addressable storage"
	ErrMsgObjectNeedsKind              = "object must have a 'kind' field"
	ErrMsgNoValidIDPrefix              = "no valid ID prefix found for kind %s"
	ErrMsgGenerateID                   = "failed to generate ID"
	ErrMsgSchedulerJobIDLength         = "scheduler_job id must not exceed %d characters (got %d); use a short id to avoid lock file path limits"
	ErrMsgLoadIDPatternsValidation     = "failed to load ID patterns for validation"
	ErrMsgValidateID                   = "failed to validate ID"
	ErrMsgInvalidIDFormat              = "invalid ID format for kind %s: %s"
	ErrMsgMarshalObjCreation           = "failed to marshal object for creation"
	ErrMsgStreamUnmarshalObj           = "stream: unmarshal object"
	ErrMsgValidateNewID                = "failed to validate new ID"
	ErrMsgPersistHashRegRollback       = "failed to persist hash registry for created object %s: %w (object creation rolled back)"
	ErrMsgIdentityCacheHandlerRequired = "object-id-cache handler is required"
	ErrMsgWorkflowConstraintFail       = "workflow constraint validation failed"
	ErrMsgReadFile                     = "failed to read file"
	ErrMsgPermSetCredHash              = "permission denied: only system can set credential_hash"
	ErrMsgPermSetSalt                  = "permission denied: only system can set salt"
	ErrMsgOpenProcessDir               = "failed to open process directory"
	ErrMsgTouchProcessDir              = "failed to touch process directory"
	ErrMsgCountPositive                = "count must be positive, got %d"
	ErrMsgCountExceed                  = "count cannot exceed 500, got %d"
	ErrMsgNoIDPrefix                   = "no ID prefix configured for kind: %s"
	ErrMsgPersistHashRegIDChange       = "failed to persist hash registry after ID change for object %s: %w (update rolled back)"
	ErrMsgReadOptLock                  = "failed to re-read object for optimistic locking"
	ErrMsgTimeoutTestUpdate            = "timeout in test mode update"
	ErrMsgPersistHashRegUpdate         = "failed to persist hash registry for updated object %s: %w (update rolled back)"
	ErrMsgWorkflowConstraintPlan       = "workflow constraint validation failed for priority plan activation"
	ErrMsgFileReadMarshalFallback      = "file read failed: %w; marshal fallback also failed: %w"
	ErrMsgCalcHashUpdate               = "failed to calculate hash for updated object %s: %w (marshal fallback also failed: %w)"
	ErrMsgStreamUpdateUnmarshal        = "stream-backed update: unmarshal"
	ErrMsgStreamUpdateWriteState       = "stream-backed update: write current state"
	ErrMsgUnmarshalUpdateBuf           = "failed to unmarshal updated object from buffer"
	ErrMsgWriteUpdatedObjFile          = "failed to write updated object file"
	ErrMsgCreateStrategyLoader         = "failed to create strategy loader"
	ErrMsgInitStrategyLoader           = "failed to initialize strategy loader"
	ErrMsgLoadStrategiesKind           = "failed to load strategies for kind %s: %w"
	ErrMsgConvertStrategy              = "failed to convert strategy"
	ErrMsgLockDefaultStrategyCheck     = "lock error in getDefaultStrategyForKind check: %v\n"
	ErrMsgLockDefaultStrategyCache     = "lock error in getDefaultStrategyForKind cache: %v\n"
	ErrMsgLockBaseKindCheck            = "lock error in getBaseKind check: %v\n"
	ErrMsgLockBaseKindCache            = "lock error in getBaseKind cache: %v\n"
	ErrMsgStrategyMissingType          = "strategy missing strategy_type"
	ErrMsgChronoNeedsFormat            = "chronological strategy requires format or granularity"
	ErrMsgCreateStorageFactory         = "failed to create storage factory"
	ErrMsgLoadStrategies               = "failed to load strategies"
	ErrMsgTimeoutCheckCache            = "timeout checking cache"
	ErrMsgLockGetStrategy              = "lock failed in GetStrategy: %v\n"
	ErrMsgTimeoutGetStrategies         = "timeout getting strategies for kind"
	ErrMsgLockGetStrategiesKind        = "lock failed in GetStrategiesForKind: %v\n"
	ErrMsgLockGetAllStrategies         = "lock failed in GetAllStrategies: %v\n"
	ErrMsgLockReload                   = "lock failed in Reload: %v\n"
	ErrMsgLoadStrategyFmt              = "failed to load strategy %s: %w"
	ErrMsgListStrategies               = "failed to list strategies"
	ErrMsgStrategyNoID                 = "strategy missing id field"
	ErrMsgSaveStrategy                 = "failed to save strategy"
	ErrMsgDeleteStrategy               = "failed to delete strategy"
	StrategyChronoDefault              = "chrono-default"
	FmtCompositeBucket                 = "%s:composite:%s"
	FmtFirstLetterBucket               = "%s:first_letter:%s"
	FmtAndMore                         = "... and %d more"
	FieldKeyStrategyCount              = "strategy_count"
	FieldKeySchemaVersion              = "schema_version"
	FieldKeyExistingStrategyID         = "existing_strategy_id"
)

// Component and Process Descriptions
const (
	DescAuditAggMetricAsync      = "creating audit aggregation metric asynchronously"
	DescAuditAggTitleFmt         = audit.TitleFmt
	DescAuditMetricsTitleFmt     = "Audit Event Metrics: %d events from %s to %s"
	DescFlushAggGroup            = "flush aggregated group: %s"
	DescEmitFlushEvent           = "emitting flush event for %s"
	DescAggregatedEventsFmt      = "Aggregated %d %s events"
	DescPeriodicFlush            = "periodic flush of aggregated audit events"
	DescDeletedObjectFmt         = "Deleted object %s"
	DescDeletedCascadeFmt        = "Deleted object %s and %d dependent object(s) (cascade)"
	DescDeletedDependentFmt      = "Deleted object %s (had %d dependent(s) that were not deleted)"
	DescCacheRefreshReq          = "Cache refresh requested - all caches cleared for revalidation against latest specs"
	DescCacheRefreshFmt          = "Cache refresh: %s"
	DescFlushAggGroupFmt         = "flush aggregated group: %s"
	DescAuditMetricsEventEmitter = "emitting audit event creation metric asynchronously"
	DescAuditMetricsAsync        = "creating audit metric asynchronously"
	DescHashRegWorker            = "hash_registry_%s_worker"
	DescHashRegSaveWorker        = "hash_registry_save_worker"
	DescProcessBatchHashReg      = "processing batched hash registry save operations"
	DescHashRegCtxCancelled      = "hash registry context cancelled"
	DescHashRegShutdownInit      = "hash registry shutdown initiated"
	DescHashRegCtxCancelNoSave   = "hash registry context cancelled, cannot save"
	DescShutdownInProgressNoSave = "shutdown in progress, cannot save hash registry"
	DescCopyingHashRegData       = "copying hash registry data"
	DescHashRegCtxCancelEnqueue  = "hash registry context cancelled during enqueue"
	DescSaveQueueFull            = "save queue is full"
	DescHashRegCtxCancelWait     = "hash registry context cancelled while waiting for save"
	DescHashRegSaveNotComplete   = "hash registry save did not complete within %v"
	DescHashRegSaveNotCompleteBg = "hash registry save did not complete within %v (worker may still complete in background)"
	DescHashRegCoordEvent        = "hash_registry_coordinator_event"
	DescEmitBatchProcessEvent    = "emitting batch processing event for %s"
	DescHashRegDrainWait         = "hash_registry_drain_wait"
	DescWaitHashRegWorker        = "waiting for hash registry worker: %s"
	DescHashRegDrainWaitInner    = "hash_registry_drain_wait_inner"
	DescWaitWaitGroup            = "waiting for wait group %s"
	DescHashRegName              = "hash_registry_%s"
)

// Metric Names
const (
	MetricNameAuditEventCreation   = "audit_event_creation"
	MetricNameAuditEventValidation = "audit_event_validation"
	MetricNameAuditEventDuplicate  = "audit_event_duplicate"
	MetricNameAuditEventMerged     = "audit_event_merged"
	MetricNameAuditEventCollection = "audit_event_collection"
)

// Metric Tags
const (
	TagAudit         = "audit"
	TagEvent         = "event"
	TagCreation      = "creation"
	TagDuplicate     = "duplicate"
	TagMerged        = "merged"
	TagValidation    = "validation"
	TagWorkerRunning = "worker_running"
	TagFileSizeBytes = "file_size_bytes"
)

// Default Values and Units
const (
	DefaultWindowSize           = "1h"
	DefaultAggregationThreshold = 10
	DefaultPreserveSamples      = 5
	DefaultAuditMetricTimeout   = 30 * time.Second
	UnitHour                    = "h"
	UnitDay                     = "d"
	UnitWeek                    = "w"
	UnitMonth                   = "m"
)

// Log Messages and Formats
const (
	LogMsgAuditAggregation    = "⚠️ [STORAGE-AUDIT] Failed to list CAS IDs during pre-aggregate check: %v\n"
	LogMsgCasIndexPopulated   = "⚠️ [STORAGE-AUDIT] Failed to ensure CAS index populated: %v\n"
	LogMsgCacheBuildFailed    = "⚠️ [STORAGE-AUDIT] High-volume event cache build failed: %v\n"
	LogMsgLintBypassed        = "Lint checks bypassed with --no-verify flag"
	LogFmtLintBypassed        = "Lint checks bypassed: %s"
	LogFmtCreatedObject       = "Created object %s"
	LogFmtUpdatedObject       = "Updated object %s"
	LogFmtUpdatedObjectDetail = "Updated object %s: %s"
	LogFmtUpdatedObjectMore   = "Updated object %s: %s and %d more field(s)"
	LogFmtCreateAuditDirFail  = "failed to create audit directory %s: %v\n"
)

// Command Strings
const (
	CommandRefreshCache = " system check --refresh-cache"
)

// Files and Specs
const (
	SpecFileAuditEvent = "audit_event.yaml"
)

// Global Constants
const (
	StatusArchived     = ValueStatusArchived
	StatusStart        = ValueStatusStart
	StatusError        = ValueStatusError
	StatusComplete     = ValueStatusComplete
	StatusSuccess      = ValueStatusSuccess
	StatusCompletedVal = ValueStatusCompleted
)

// Internal temporary constants to fix compilation
const (
	aggEventTypeCountKeyAggregated = audit.EventTypeCountKeyAggregated
	aggMergeKeyEventCount          = audit.MergeKeyEventCount
	aggMergeKeyEventTypeCounts     = audit.MergeKeyEventTypeCounts
	aggMergeKeyStatusCounts        = audit.MergeKeyStatusCounts
	aggMergeKeyObjectKindCounts    = audit.MergeKeyObjectKindCounts
	aggMergeKeyOperationCounts     = audit.MergeKeyOperationCounts
	aggMergeKeyAggregatedEventIDs  = audit.MergeKeyAggregatedEventIDs
	aggMergeKeyCollectionCount     = audit.MergeKeyCollectionCount
	aggMergeKeyLastSeen            = audit.MergeKeyLastSeen
	aggMergeKeyErrorEventCount     = audit.MergeKeyErrorEventCount
	aggMergeKeyErrorRate           = audit.MergeKeyErrorRate
	aggMergeKeyTitle               = audit.MergeKeyTitle
)

// Hash Registry Magic Strings
const (
	WorkerNameHashRegistryFmt  = "hash_registry_%s_worker"
	WorkerNameHashRegistrySave = "hash_registry_save_worker"
	DescHashRegistrySaveBatch  = "processing batched hash registry save operations"
	ErrHashRegistryCtxCancel   = "hash registry context cancelled"
	ErrHashRegistryShutdown    = "hash registry shutdown initiated"
	ErrStatHashRegistry        = "failed to stat hash registry"
	ErrReadHashRegistry        = "failed to read hash registry"
	ErrParseHashRegistry       = "failed to parse hash registry"
	ErrHashRegCtxCancelSave    = "hash registry context cancelled, cannot save"
	ErrHashRegShutdownSave     = "shutdown in progress, cannot save hash registry"
	DescCopyHashRegistry       = "copying hash registry data"
	ErrHashRegCtxCancelEnqueue = "hash registry context cancelled during enqueue"
	ErrSaveQueueFull           = "save queue is full"
	ErrHashRegCtxCancelWait    = "hash registry context cancelled while waiting for save"
	ErrHashRegSaveTimeoutFmt   = "hash registry save did not complete within %v"
	ErrHashRegSaveTimeoutBgFmt = "hash registry save did not complete within %v (worker may still complete in background)"
	FieldWorkerRunning         = "worker_running"
	FieldFileSizeBytes         = "file_size_bytes"
	ErrMarshalHashRegistry     = "failed to marshal hash registry"
	ErrCreateDirHashRegistry   = "failed to create directory for hash registry"
	ErrOpenTempFileWrite       = "failed to open temp file for writing"
	ErrWriteHashRegistryData   = "failed to write hash registry data"
	ErrEnsureDirRename         = "failed to ensure directory exists before rename"
	ErrDirNotExist             = "directory does not exist or is not accessible"
	ErrEnsureDirHashRegistry   = "failed to ensure directory exists for hash registry"
	ErrTempFileNotExist        = "temp file does not exist before rename"
	ErrRenameTempHashRegistry  = "failed to rename temp file to hash registry file"
	ErrRenameRetry             = "failed to rename (file not found, will retry)"
	ErrSaveHashRegRetries      = "failed to save hash registry after retries"
	EventHashRegCoordinator    = "hash_registry_coordinator_event"
	DescEmitBatchProcessFmt    = "emitting batch processing event for %s"
	EventHashRegDrainWait      = "hash_registry_drain_wait"
	DescWaitHashRegWorkerFmt   = "waiting for hash registry worker: %s"
	EventHashRegDrainWaitInner = "hash_registry_drain_wait_inner"
	DescWaitWaitgroupFmt       = "waiting for wait group %s"
	HashRegistryNameFmt        = "hash_registry_%s"
)

// Object Storage File Create Magic Strings
const (
	ErrObjectNoKind              = "object must have a 'kind' field"
	ErrLoadIDPatterns            = "failed to load ID patterns"
	ErrNoValidIDPrefixFmt        = "no valid ID prefix found for kind %s"
	ErrGenerateID                = "failed to generate ID"
	ErrSchedulerJobIDLengthFmt   = "scheduler_job id must not exceed %d characters (got %d); use a short id to avoid lock file path limits"
	ErrLoadIDPatternsValidation  = "failed to load ID patterns for validation"
	ErrValidateID                = "failed to validate ID"
	ErrInvalidIDFmt              = "invalid ID format for kind %s: %s"
	ErrCreateDir                 = "failed to create directory"
	ErrMarshalObjectCreate       = "failed to marshal object for creation"
	ErrCreateObjectEmptyFmt      = "cannot create object with empty content: %s"
	DescStreamUnmarshal          = "stream: unmarshal object"
	ErrGetCAS                    = "failed to get content-addressable storage"
	ErrCreateObjectCAS           = "failed to create object in content-addressable storage"
	ErrPersistHashRegRollbackFmt = "failed to persist hash registry for created object %s: %w (object creation rolled back)"
	ErrWorkflowConstraintFail    = "workflow constraint validation failed"
)

// Bulk Delete
const (
	ErrMsgLockCreateJob            = "failed to create job in lock"
	ErrMsgLockGetJob               = "failed to get job in lock"
	ErrMsgJobNotFound              = "job not found: %s"
	ErrMsgJobNotPending            = "job is not in pending state: %s"
	ErrMsgLockGetJobStatus         = "failed to get job status in lock"
	ErrMsgLockCopyJobStatus        = "failed to copy job status in lock"
	ErrMsgNoFailedDeletionsCleanup = "no failed deletions to cleanup"
	ErrMsgLockCheckLeafNode        = "failed to check if leaf node in lock: %v\n"
	ErrMsgBeginTxBulkDelete        = "failed to begin transaction for bulk delete"
	ErrMsgBulkDeleteReqFileObjTx   = "bulk delete leaf path requires FileObjectTransaction"
	ErrMsgBulkDeleteEnqueue        = "bulk delete enqueue %s: %w"
	ErrMsgBulkDeleteCommit         = "bulk delete commit"
	ErrMsgBeginTx                  = "failed to begin transaction"
	ErrMsgDeleteObjFail            = "failed to delete object %s: %v"
	ErrMsgCommitBulkDelTx          = "failed to commit bulk delete transaction"

	FmtBulkDeleteJobName       = "bulk-delete-%d"
	DescBulkDeleteAsyncExec    = "bulk_delete_async_executor"
	DescExecBulkDeleteJob      = "executing bulk delete job %s"
	FmtBulkDeleteWorkerName    = "bulk_delete_worker_%d"
	FmtBulkDeleteWorkerDesc    = "deleting objects (worker %d of %d)"
	DescBulkDeleteJobSender    = "bulk_delete_job_sender"
	DescSendDelJobsWorkers     = "sending deletion jobs to workers"
	DescBulkDeleteResCollector = "bulk_delete_result_collector"
	DescCollectDelRes          = "collecting deletion results"
)
