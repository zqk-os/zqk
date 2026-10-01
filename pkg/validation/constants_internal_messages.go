package validation

const ErrMsgSwallowedError = "swallowed error: %v\n"

// Validation Metrics Constants
const (
	ErrMsgLockGetObjectsPerSec  = "lock failed in GetObjectsPerSecond: %v\n"
	ErrMsgLockRecordWait        = "lock failed in RecordLockWait: %v\n"
	ErrMsgLockGetContentionRate = "lock failed in GetContentionRate: %v\n"
	ErrMsgLockGetTimeoutRate    = "lock failed in GetTimeoutRate: %v\n"
	ErrMsgLockGetMaxWaitTime    = "lock failed in GetMaxWaitTime: %v\n"
	ErrMsgLockRecordTimeout     = "lock failed in RecordLockTimeout: %v\n"
	ErrMsgLockRecordHRCacheWait = "lock failed in RecordHashRegistryCacheLockWait: %v\n"
	ErrMsgLockRecordHRCacheHold = "lock failed in RecordHashRegistryCacheLockHold: %v\n"
	ErrMsgLockRecordHRLoad      = "lock failed in RecordHashRegistryLoad: %v\n"
	ErrMsgLockIncHRAccess       = "lock failed in IncrementHashRegistryConcurrentAccess: %v\n"
	ErrMsgLockRecordHROps       = "lock failed in RecordHashRegistryOperations: %v\n"
	ErrMsgLockRecordHRBucket    = "lock failed in RecordHashRegistryBucketedProcessing: %v\n"
	ErrMsgLockRecordHRNonBucket = "lock failed in RecordHashRegistryNonBucketedProcessing: %v\n"
	ErrMsgLockRecordEnqueue     = "lock failed in RecordEnqueue: %v\n"
	ErrMsgLockRecordValidation  = "lock failed in RecordValidation: %v\n"
	ErrMsgLockRecordCollection  = "lock failed in RecordCollection: %v\n"
	ErrMsgLockIncValidated      = "lock failed in IncrementValidated: %v\n"
	ErrMsgLockIncFailed         = "lock failed in IncrementFailed: %v\n"
	ErrMsgLockIncCacheHit       = "lock failed in IncrementCacheHit: %v\n"
	ErrMsgLockIncCacheMiss      = "lock failed in IncrementCacheMiss: %v\n"
	ErrMsgLockIncRetry          = "lock failed in IncrementRetry: %v\n"
	ErrMsgLockSetTotalObjects   = "lock failed in SetTotalObjects: %v\n"
	ErrMsgLockSetWorkerCount    = "lock failed in SetWorkerCount: %v\n"
	ErrMsgLockUpdateQueueSize   = "lock failed in UpdateQueueSize: %v\n"
	ErrMsgLockRecordTierIssue   = "lock failed in RecordTierIssue: %v\n"
	ErrMsgLockFinalize          = "lock failed in Finalize: %v\n"
	ErrMsgMarshalMetrics        = "failed to marshal metrics"
	ErrMsgLockSave              = "lock failed in Save: %v\n"
	ErrMsgWriteMetricsFile      = "failed to write metrics file"
	ErrMsgLockString            = "lock failed in String: %v\n"
	MetricNameAsyncValidator    = "async_validator"
	MetricNameWorkerShutdown    = "worker_shutdown"
	DescWorkerStopped           = "Worker stopped"
	DescWorkerStarted           = "Worker started"
	MetricNameSemaphoreFull     = "semaphore_full"
	MetricNameSemaphoreCapacity = "semaphore_capacity"
	MetricNameFileReadError     = "file_read_error"
	FmtAppliesTo                = "applies_to[%d]"
	FieldKeyTierProgression     = "tier_progression"
	FieldKeySchemaVersion       = "schema_version"
	FieldKeyAggWindowStart      = "aggregation_window_start"
	FieldKeyAggWindowEnd        = "aggregation_window_end"
	FieldKeyAggWindowTime       = "aggregation_window_time"
	FieldKeySemanticTypes       = "semantic_types"
	FieldKeyDisplayLength       = "display_length"
	FieldKeyWorkstreamRef       = "workstream_ref"
	ErrMsgFileNotFound          = "file_not_found"
	KindChangeJournal           = "change_journal"
	FormatTwoWordAbbrev         = "two_word_abbrev"
	SuffixOrganizational        = ":organizational"
)

// Message strings composition roots still alias.
const (
)

// Extracted field and log strings composition roots still alias.
const (
)
