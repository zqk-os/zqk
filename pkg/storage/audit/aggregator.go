package audit

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// Aggregator is the audit subpackage contract. Implementations live in package
// storage (AuditAggregationService) so this package does not import storage.
type Aggregator interface {
	GetAuditAggregationStats() (aggregations, eventsAggregated int64)
	CleanupAggregatedEvents(ctx context.Context, secCtx *pkgctx.SecurityContext, eventIDs []string, archive bool) (int, error)
	QueryOldAggregatedEvents(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, cutoffTime time.Time) ([]string, error)
	QueryOldAuditEventsByAge(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, cutoffTime time.Time, limit int) ([]string, error)
	AggregateAuditEvents(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, windowStart, windowEnd time.Time) (*AggregationResult, error)
}
