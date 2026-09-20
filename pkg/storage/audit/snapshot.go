package audit

import (
	"maps"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// MetricsSnapshot is a point-in-time view of audit creation metrics.
type MetricsSnapshot struct {
	EventsCreated     int64
	EventsCreatedCAS  int64
	EventsCreatedID   int64
	EventsFailed      int64
	EventsValidated   int64
	EventsSkipped     int64
	EventsDuplicated  int64
	EventsMerged      int64
	TotalCreationTime time.Duration
	MaxCreationTime   time.Duration
	CreationEvents    int64
	UpdateEvents      int64
	DeleteEvents      int64
	BulkEvents        int64
	SystemEvents      int64
	OtherEvents       int64
}

// AverageCreationTime is total creation time divided by EventsCreated.
func (s MetricsSnapshot) AverageCreationTime() time.Duration {
	if s.EventsCreated == 0 {
		return 0
	}
	return s.TotalCreationTime / time.Duration(s.EventsCreated)
}

func ratePercent(n, d int64) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d) * 100
}

// CASUsageRate is the percentage of events created via CAS.
func (s MetricsSnapshot) CASUsageRate() float64 {
	return ratePercent(s.EventsCreatedCAS, s.EventsCreated)
}

// FailureRate is the percentage of failed event creations.
func (s MetricsSnapshot) FailureRate() float64 {
	return ratePercent(s.EventsFailed, s.EventsCreated)
}

// ValidationSkipRate is the percentage of events skipped after validation.
func (s MetricsSnapshot) ValidationSkipRate() float64 {
	return ratePercent(s.EventsSkipped, s.EventsValidated)
}

// EventTypeBucket is the metrics counter for an event_type.
type EventTypeBucket int

const (
	BucketOther EventTypeBucket = iota
	BucketCreation
	BucketUpdate
	BucketDelete
	BucketBulk
	BucketSystem
)

// ClassifyEventType maps an event_type onto a metrics bucket.
func ClassifyEventType(eventType string) EventTypeBucket {
	switch eventType {
	case EventTypeObjectCreation:
		return BucketCreation
	case EventTypeObjectUpdate:
		return BucketUpdate
	case EventTypeObjectDeletion:
		return BucketDelete
	case EventTypeBulkOperation:
		return BucketBulk
	case EventTypeSystemConfigChange, EventTypeCacheRefresh, EventTypeCodeQualityBypass:
		return BucketSystem
	default:
		return BucketOther
	}
}

const (
	MetaEventsCreatedCAS   = "events_created_cas"
	MetaEventsCreatedID    = "events_created_id"
	MetaEventsValidated    = "events_validated"
	MetaEventsSkipped      = "events_skipped"
	MetaEventsDuplicated   = "events_duplicated"
	MetaEventsMerged       = "events_merged"
	MetaMaxCreationTimeMs  = "max_creation_time_ms"
	MetaCasUsageRate       = "cas_usage_rate"
	MetaFailureRate        = "failure_rate"
	MetaValidationSkipRate = "validation_skip_rate"
	MetaCreationEvents     = "creation_events"
	MetaUpdateEvents       = "update_events"
	MetaDeleteEvents       = "delete_events"
	MetaBulkEvents         = "bulk_events"
	MetaSystemEvents       = "system_events"
	MetaOtherEvents        = "other_events"
	MetaMeasurementPeriod  = "measurement_period"
)

// SuccessCount is created minus failed.
func (s MetricsSnapshot) SuccessCount() int64 {
	return s.EventsCreated - s.EventsFailed
}

// DurationFields are command_metric duration columns derived from a snapshot.
type DurationFields struct {
	Avg      float64
	Slowest  float64
	Fastest  float64
	Baseline float64
}

// DurationSeconds maps creation times onto command_metric duration fields.
func (s MetricsSnapshot) DurationSeconds() DurationFields {
	avg := s.AverageCreationTime().Seconds()
	slowest := avg
	if s.MaxCreationTime > 0 {
		slowest = s.MaxCreationTime.Seconds()
	}
	return DurationFields{Avg: avg, Slowest: slowest, Fastest: avg, Baseline: avg}
}

// CommandMetricMetadata is the command_metric metadata blob CollectMetrics writes.
func CommandMetricMetadata(s MetricsSnapshot, windowSeconds float64) map[string]any {
	return map[string]any{
		MetaEventsCreatedCAS:   s.EventsCreatedCAS,
		MetaEventsCreatedID:    s.EventsCreatedID,
		MetaEventsValidated:    s.EventsValidated,
		MetaEventsSkipped:      s.EventsSkipped,
		MetaEventsDuplicated:   s.EventsDuplicated,
		MetaEventsMerged:       s.EventsMerged,
		MetaMaxCreationTimeMs:  s.MaxCreationTime.Milliseconds(),
		MetaCasUsageRate:       s.CASUsageRate(),
		MetaFailureRate:        s.FailureRate(),
		MetaValidationSkipRate: s.ValidationSkipRate(),
		MetaCreationEvents:     s.CreationEvents,
		MetaUpdateEvents:       s.UpdateEvents,
		MetaDeleteEvents:       s.DeleteEvents,
		MetaBulkEvents:         s.BulkEvents,
		MetaSystemEvents:       s.SystemEvents,
		MetaOtherEvents:        s.OtherEvents,
		MetaMeasurementPeriod:  windowSeconds,
	}
}

// AttachCommandMetricMetadata merges snapshot metadata onto a command_metric map.
func AttachCommandMetricMetadata(metricObj map[string]any, s MetricsSnapshot, windowSeconds float64) {
	if metricObj == nil {
		return
	}
	meta := CommandMetricMetadata(s, windowSeconds)
	if existing, ok := metricObj[objects.FieldKeyMetadata].(map[string]any); ok && existing != nil {
		maps.Copy(existing, meta)
		return
	}
	metricObj[objects.FieldKeyMetadata] = meta
}
