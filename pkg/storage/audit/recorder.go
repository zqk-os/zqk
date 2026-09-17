package audit

import (
	"context"
	"time"
)

// CreationRecorder is the metrics slice audit persistence needs. Storage's
// AuditMetricsCollector implements this; the collector stays out of this package.
type CreationRecorder interface {
	RecordAuditEventCreation(ctx context.Context, eventType string, usedCAS bool, duration time.Duration, success bool)
	RecordAuditEventValidation(success bool)
	RecordAuditEventDuplicate()
}

// RecordBuffered records a successfully buffered event (no CAS, no duration).
func RecordBuffered(ctx context.Context, rec CreationRecorder, eventType string) {
	if rec == nil {
		return
	}
	rec.RecordAuditEventCreation(ctx, eventType, false, 0, true)
}

// RecordPersistOutcome records create/retry metrics. Duplicate is counted only
// when the store still reports already-exists after the persist attempt.
func RecordPersistOutcome(ctx context.Context, rec CreationRecorder, eventType string, usedCAS bool, duration time.Duration, result PersistResult) {
	if rec == nil {
		return
	}
	success := result.Err == nil || result.AlreadyExists
	rec.RecordAuditEventCreation(ctx, eventType, usedCAS, duration, success)
	rec.RecordAuditEventValidation(success)
	if result.Err != nil && result.AlreadyExists {
		rec.RecordAuditEventDuplicate()
	}
}
