package audit

const (
	SeverityLow    = "low"
	SeverityMedium = "medium"
	SeverityHigh   = "high"
)

const (
	EventTypeAggregatedSummary     = "aggregated_summary"
	EventTypeSchedulerJobStarted   = "scheduler_job_started"
	EventTypeSchedulerJobCompleted = "scheduler_job_completed"
	EventTypeCacheInvalidation     = "cache_invalidation"
	EventTypeCacheUpdate           = "cache_update"
	EventTypeCacheBulkInvalidation = "cache_bulk_invalidation"
	EventTypeObjectCreation        = "object_creation"
	EventTypeObjectUpdate          = "object_update"
	EventTypeObjectDeletion        = "object_deletion"
	EventTypeBulkOperation         = "bulk_operation"
	EventTypeSystemConfigChange    = "system_config_change"
	EventTypeCacheRefresh          = "cache_refresh"
	EventTypeCodeQualityBypass     = "code_quality_bypass"
)
