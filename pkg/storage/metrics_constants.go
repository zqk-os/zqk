package storage

import "github.com/lanceman/zqk/pkg/objects"

// Storage metric constants (defined here to avoid import cycle with pkg/metrics)
// These constants standardize metric creation across storage package components

// Metric type constants (matching pkg/metrics but avoiding import cycle)
const (
	// StorageMetricTypeSystem is the metric_type value for system metrics
	StorageMetricTypeSystem = "system"
	// StorageMetricTypePerformance is the metric_type value for performance metrics
	StorageMetricTypePerformance = "performance"
)

// Metric source constants for storage components
const (
	// FileLockMetricSource is the source value for file lock system metrics
	FileLockMetricSource = "file_lock_system"
	// AuditAggregationMetricSource is the source value for audit aggregation job metrics
	AuditAggregationMetricSource = "audit_aggregation_job"
	// ChangeJournalAggregationMetricSource is the source value for change journal aggregation job metrics
	ChangeJournalAggregationMetricSource = "change_journal_aggregation_job"
	// AuditSystemMetricSource is the source value for audit system metrics
	AuditSystemMetricSource = "audit_system"
	// CASSystemMetricSource is the source value for CAS system metrics
	CASSystemMetricSource = "cas_system"
)

// Metric tag constants for storage components
const (
	// StorageMetricTagSystem is the "system" tag used in system metrics
	StorageMetricTagSystem = "system"
	// StorageMetricTagAudit is the "audit" tag used in audit-related metrics
	StorageMetricTagAudit = "audit"
	// StorageMetricTagAggregated is the "aggregated" tag used in aggregated metrics
	StorageMetricTagAggregated = "aggregated"
	// StorageMetricTagChangeJournal is the "change_journal" tag used in change journal metrics
	StorageMetricTagChangeJournal = "change_journal"
	// StorageMetricTagEvents is the "events" tag used in event-related metrics
	StorageMetricTagEvents = "events"
	// StorageMetricTagFileLock is the "file_lock" tag used in file lock metrics
	StorageMetricTagFileLock = "file_lock"
	// StorageMetricTagConcurrency is the "concurrency" tag used in concurrency-related metrics
	StorageMetricTagConcurrency = "concurrency"
	// StorageMetricTagCAS is the "cas" tag used in CAS metrics
	StorageMetricTagCAS = "cas"
	// StorageMetricTagStorage is the "storage" tag used in storage metrics
	StorageMetricTagStorage = "storage"
	// StorageMetricTagContentAddressable is the "content_addressable" tag used in CAS metrics
	StorageMetricTagContentAddressable = "content_addressable"
)

// Metric field name constants (shared across storage metrics helpers and collectors).
// These mirror spec field names for metric-like kinds.
const (
	MetricFieldID              = objects.FieldKeyID
	MetricFieldTitle           = "title"
	MetricFieldMetricType      = "metric_type"
	MetricFieldSource          = "source"
	MetricFieldTags            = "tags"
	MetricFieldCollectionCount = "collection_count"
	MetricFieldFirstSeen       = "first_seen"
	MetricFieldLastSeen        = "last_seen"
)

// Metric kind constants for spec-backed storage metrics (ontology strings from pkg/objects).
const (
	MetricKindCommandMetric      = objects.KindCommandMetric
	MetricKindFileLockMetric     = objects.KindFileLockMetric
	MetricKindAuditAggregation   = objects.KindAuditAggregationMetric
	MetricKindChangeJournalEntry = objects.KindChangeJournalEntry
	MetricKindAuditEvent         = objects.KindAuditEvent
)
