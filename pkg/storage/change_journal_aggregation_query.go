// Extracted from change_journal_aggregation.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

func (s *ChangeJournalAggregationService) QueryOldAggregatedEntries(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	cutoffTime time.Time,
) ([]string, error) {
	filter := ListFilter{
		Kind: objects.KindChangeJournalEntry,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$eq": objects.ObjectStatusAggregated, // Entries marked as aggregated
			},
			objects.FieldKeyCreatedAt: map[string]any{
				"$lt": cutoffTime.Format(time.RFC3339), // Older than cutoff
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   0, // No limit - get all old aggregated entries
	}

	result, err := s.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToQueryOldAggregatedEntries).Wrap(err)
	}

	entryIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id := objects.GetString(obj, objects.FieldKeyID); id != emptyValue {
			entryIDs = append(entryIDs, id)
		}
	}

	return entryIDs, nil
}

// QueryOldEntriesByAge returns change_journal_entry IDs with created_at older than cutoff (any status).
// Used for retention cleanup so entries outside the aggregation window still get deleted and the stream does not grow unbounded.
// limit caps how many IDs are returned per call (0 = no limit).
func (s *ChangeJournalAggregationService) QueryOldEntriesByAge(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	cutoffTime time.Time,
	limit int,
) ([]string, error) {
	filter := ListFilter{
		Kind: objects.KindChangeJournalEntry,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$lt": cutoffTime.Format(time.RFC3339),
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   limit,
	}
	result, err := s.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToQueryOldEntriesByAge).Wrap(err)
	}
	entryIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id := objects.GetString(obj, objects.FieldKeyID); id != emptyValue {
			entryIDs = append(entryIDs, id)
		}
	}
	return entryIDs, nil
}

// CleanupAggregatedEntries deletes or archives change journal entries that have been aggregated
func (s *ChangeJournalAggregationService) CleanupAggregatedEntries(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	entryIDs []string,
	archive bool,
) (int, error) {
	if archive {
		// Update entries to archived status
		updates := make([]BulkUpdateItem, 0, len(entryIDs))
		for _, id := range entryIDs {
			updates = append(updates, BulkUpdateItem{
				ID: id,
				Updates: map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusArchived,
				},
			})
		}

		result, err := s.storage.BulkUpdate(ctx, secCtx, updates)
		if err != nil {
			return 0, err
		}
		return result.SuccessCount, nil
	}

	// Delete entries using optimized bulk delete
	expandedIDs := ExpandIDRanges(entryIDs)

	// Use optimized bulk delete if available (FileObjectStorage)
	if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
		result, err := fileStorage.BulkDeleteOptimized(ctx, secCtx, expandedIDs, false, 20)
		if err != nil {
			return 0, err
		}
		return result.SuccessCount, nil
	}

	// Fallback to standard BulkDelete
	result, err := s.storage.BulkDelete(ctx, secCtx, expandedIDs, false)
	if err != nil {
		return 0, err
	}

	return result.SuccessCount, nil
}

// ChangeJournalAggregationResult contains the results of an aggregation operation
type ChangeJournalAggregationResult struct {
	MetricID         string
	EntryCount       int
	MetricsCreated   int
	EntriesProcessed []string
	EntriesUpdated   int
	WindowStart      time.Time
	WindowEnd        time.Time
}
