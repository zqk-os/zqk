package crud

import (
	"context"

	"github.com/zqk-os/zqk/pkg/objects"
)

// bulkCreateCtxKey is the context key for deferring listing-index flush during bulk job creation.
type bulkCreateCtxKey struct{}

// WithBulkCreateDeferFlush marks the context so Create() skips per-object listing-index FlushKind for
// scheduler_job and audit_event. Caller must call FlushKind("scheduler_job") and
// FlushKind("audit_event") once after the bulk create loop (e.g. in GenerateJobs).
func WithBulkCreateDeferFlush(ctx context.Context) context.Context {
	return context.WithValue(ctx, bulkCreateCtxKey{}, true)
}

func DeferListingIndexFlushForBulkCreate(ctx context.Context, kind string) bool {
	v, ok := ctx.Value(bulkCreateCtxKey{}).(bool)
	if !ok || !v {
		return false
	}
	return kind == objects.KindSchedulerJob || kind == objects.KindAuditEvent
}

// syncCreateForSchedulerJobKey is the context key for synchronous create of scheduler_job
// so the new job is visible to LoadJobs in the same process (avoids write-behind race).
type syncCreateForSchedulerJobKey struct{}

// WithSyncCreateForSchedulerJob marks the context so Create() for kind scheduler_job
// skips write-behind and applies to storage immediately. Used by the scheduler when
// ensuring SCH-maintenance-wal and other critical jobs so LoadJobs sees them on first load.
func WithSyncCreateForSchedulerJob(ctx context.Context) context.Context {
	return context.WithValue(ctx, syncCreateForSchedulerJobKey{}, true)
}

// syncCreateForKindKey is the context key for synchronous create of a specific kind (used by migrate-legacy-to-stream).
type syncCreateForKindKey struct{}

// WithSyncCreateForKind marks the context so Create() for the given kind skips write-behind and applies to storage immediately.
// Used when migrating legacy YAML in .zqk/process/<dir> to stream so each object is visible after create.
func WithSyncCreateForKind(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, syncCreateForKindKey{}, kind)
}
