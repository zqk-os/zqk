package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/audit"
)

// auditStore adapts ObjectStorageProvider onto audit ObjectQuery, MetricStore,
// BulkStatusWriter, and BulkDeleter without the audit package importing ListFilter.
type auditStore struct {
	p ObjectStorageProvider
}

func (a auditStore) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, q audit.ListQuery) ([]map[string]any, error) {
	result, err := a.p.List(ctx, secCtx, storageCtx, ListFilter{
		Kind:    q.Kind,
		Filters: q.Filters,
		SortBy:  q.SortBy,
		SortAsc: q.SortAsc,
		Limit:   q.Limit,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.Objects, nil
}

func (a auditStore) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return a.p.Create(ctx, secCtx, obj)
}

func (a auditStore) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return a.p.Update(ctx, secCtx, id, updates)
}

func (a auditStore) UpdateStatus(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, status string) (int, error) {
	updates := make([]BulkUpdateItem, 0, len(ids))
	for _, id := range ids {
		updates = append(updates, BulkUpdateItem{
			ID: id,
			Updates: map[string]any{
				objects.FieldKeyStatus: status,
			},
		})
	}
	result, err := a.p.BulkUpdate(ctx, secCtx, updates)
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, nil
	}
	return result.SuccessCount, nil
}

const auditBulkDeleteWorkers = 20

func (a auditStore) DeleteIDs(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (audit.BulkDeleteOutcome, error) {
	if fs, ok := a.p.(*FileObjectStorage); ok {
		result, err := fs.BulkDeleteOptimized(ctx, secCtx, ids, false, auditBulkDeleteWorkers)
		if err != nil {
			return audit.BulkDeleteOutcome{}, err
		}
		return bulkDeleteOutcomeFromOptimized(result), nil
	}
	result, err := a.p.BulkDelete(ctx, secCtx, ids, false)
	if err != nil {
		return audit.BulkDeleteOutcome{}, err
	}
	return bulkDeleteOutcomeFromBulk(result), nil
}

func bulkDeleteOutcomeFromOptimized(result *BulkDeleteResult) audit.BulkDeleteOutcome {
	if result == nil {
		return audit.BulkDeleteOutcome{Optimized: true}
	}
	return bulkDeleteOutcome(result.SuccessCount, result.FailureCount, result.Errors, true)
}

func bulkDeleteOutcomeFromBulk(result *BulkResult) audit.BulkDeleteOutcome {
	if result == nil {
		return audit.BulkDeleteOutcome{}
	}
	return bulkDeleteOutcome(result.SuccessCount, result.FailureCount, result.Errors, false)
}

func bulkDeleteOutcome(success, fail int, errs []BulkOperationError, optimized bool) audit.BulkDeleteOutcome {
	out := audit.BulkDeleteOutcome{SuccessCount: success, FailureCount: fail, Optimized: optimized}
	if len(errs) > 0 {
		if errs[0].Error != nil {
			out.FirstMessage = errs[0].Error.Error()
		} else {
			out.FirstMessage = errs[0].Message
		}
	}
	return out
}

func (s *AuditAggregationService) query() audit.ObjectQuery {
	return auditStore{s.storage}
}

func (s *AuditAggregationService) metrics() audit.MetricStore {
	return auditStore{s.storage}
}

func (s *AuditAggregationService) statuses() audit.BulkStatusWriter {
	return auditStore{s.storage}
}

func (s *AuditAggregationService) deleter() audit.BulkDeleter {
	return auditStore{s.storage}
}

func (c *AuditMetricsCollector) metrics() audit.MetricStore {
	return auditStore{c.storage}
}

var (
	_ audit.ObjectQuery      = auditStore{}
	_ audit.MetricStore      = auditStore{}
	_ audit.BulkStatusWriter = auditStore{}
	_ audit.BulkDeleter      = auditStore{}
	_ audit.MetricStore      = (*FileObjectStorage)(nil)
)
