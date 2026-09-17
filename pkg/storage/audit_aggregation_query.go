// Extracted from pkg/storage/audit_aggregation_helpers.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/audit"
)

// ExpandIDRange expands an ID range like "AUD-1..AUD-165" into individual IDs.
func ExpandIDRange(rangeStr string) []string {
	return audit.ExpandIDRange(PrefixAudit, SeparatorRange, rangeStr)
}

// ExpandIDRanges expands a list of ID ranges and individual IDs into a flat list of individual IDs.
func ExpandIDRanges(idRanges []string) []string {
	return audit.ExpandIDRanges(PrefixAudit, SeparatorRange, idRanges)
}

// QueryOldAggregatedEvents queries for all archived/aggregated events older than cutoff time
func (s *AuditAggregationService) QueryOldAggregatedEvents(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	cutoffTime time.Time,
) ([]string, error) {
	objs, err := s.query().List(ctx, secCtx, storageCtx, audit.EventsOldestFirst(
		MetricKindAuditEvent,
		audit.MergeFilters(audit.StatusEq(ValueStatusArchived), audit.CreatedAtBefore(cutoffTime)),
		0,
	))
	if err != nil {
		return nil, errfmt.Newf(ErrMsgQueryAggEvents).Wrap(err)
	}
	return audit.CollectIDs(objs), nil
}

// QueryOldAuditEventsByAge returns audit_event IDs older than cutoff (by created_at).
// Used for catch-up cleanup when aggregation has been failing and event count is high.
// limit caps the number of IDs returned per call (0 = no limit; use 1000-2000 for batching).
// Uses high-volume event cache for fast queries when available.
func (s *AuditAggregationService) QueryOldAuditEventsByAge(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	cutoffTime time.Time,
	limit int,
) ([]string, error) {

	cache := GetGlobalHighVolumeEventCache()
	if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
		projectRoot := fileStorage.GetProjectRoot()
		if cache.IsPopulatedForProject(projectRoot) {

			eventIDs := cache.QueryOlderThan(cutoffTime, limit)
			if len(eventIDs) > 0 {

				filteredIDs := s.filterOutArchivedEvents(ctx, secCtx, storageCtx, eventIDs)
				if len(filteredIDs) > 0 {
					return filteredIDs, nil
				}

			}

		} else {

			buildCtx, buildCancel := context.WithTimeout(ctx, 5*time.Second)
			defer buildCancel()
			if errBuild := EnsureHighVolumeEventCacheReady(buildCtx, projectRoot, s.storage, false); errBuild != nil {
				StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(LogMsgCacheBuildFailed).WithError(errBuild).Log()
			}

			if cache.IsPopulatedForProject(projectRoot) {
				eventIDs := cache.QueryOlderThan(cutoffTime, limit)

				if len(eventIDs) > 0 {

					filteredIDs := s.filterOutArchivedEvents(ctx, secCtx, storageCtx, eventIDs)
					if len(filteredIDs) > 0 {
						return filteredIDs, nil
					}

				}
			}
		}
	}

	queryCtx := ctx
	var queryCancel context.CancelFunc
	if ctx != nil && ctx.Err() == nil {
		queryCtx, queryCancel = context.WithTimeout(ctx, 300*time.Second)
		defer func() {
			if queryCancel != nil {
				queryCancel()
			}
		}()
	}

	objs, err := s.query().List(queryCtx, secCtx, storageCtx, audit.EventsOldestFirst(
		MetricKindAuditEvent,
		audit.MergeFilters(audit.CreatedAtBefore(cutoffTime), audit.StatusNe(ValueStatusArchived)),
		limit,
	))
	if err != nil {

		if queryCtx != nil && queryCtx.Err() == context.DeadlineExceeded {
			return []string{}, nil
		}
		return nil, errfmt.Newf(ErrMsgQueryAuditAge).Wrap(err)
	}
	return audit.CollectIDs(objs), nil
}

// filterOutArchivedEvents filters out event IDs that are already archived
// This is needed when using the cache (which doesn't store status) to avoid redundant updates
func (s *AuditAggregationService) filterOutArchivedEvents(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	eventIDs []string,
) []string {
	if len(eventIDs) == 0 {
		return eventIDs
	}

	filterCtx := ctx
	var filterCancel context.CancelFunc
	if ctx != nil && ctx.Err() == nil {
		filterCtx, filterCancel = context.WithTimeout(ctx, 10*time.Second)
		defer func() {
			if filterCancel != nil {
				filterCancel()
			}
		}()
	}

	result, err := s.query().List(filterCtx, secCtx, storageCtx, audit.Limited(
		MetricKindAuditEvent,
		audit.MergeFilters(audit.IDsIn(eventIDs), audit.StatusNe(ValueStatusArchived)),
		len(eventIDs),
	))
	if err != nil {

		if filterCtx != nil && filterCtx.Err() == context.DeadlineExceeded {
			return []string{}
		}

		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditAggregationFilterArchivedFailedWarn).
			WithError(err).
			Int("event_count", len(eventIDs)).
			Log()
		return []string{}
	}

	return audit.CollectIDs(result)
}

// CleanupAggregatedEvents deletes or archives audit events that have been aggregated
// Skips events that are already archived to avoid redundant WAL entries.
func (s *AuditAggregationService) CleanupAggregatedEvents(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	eventIDs []string,
	archive bool,
) (int, error) {
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if archive {

		storageCtx := pkgctx.GetStorageContext()
		nonArchivedIDs := s.filterOutArchivedEvents(ctx, secCtx, storageCtx, eventIDs)
		if len(nonArchivedIDs) == 0 {

			return 0, nil
		}

		if ctx != nil && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return s.statuses().UpdateStatus(ctx, secCtx, nonArchivedIDs, objects.ObjectStatusArchived)
	}

	expandedIDs := ExpandIDRanges(eventIDs)

	updateHighVolumeEventCacheOnBulkDelete(expandedIDs)

	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}

	result, err := s.deleter().DeleteIDs(ctx, secCtx, expandedIDs)
	if err != nil {
		return 0, err
	}

	if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
		if cache := GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(fileStorage.GetProjectRoot())
		}
	}

	if result.WrapAllFailed() {
		if result.FirstMessage != "" {
			return 0, errfmt.Errorf(ErrMsgDeleteEventsFmt,
				result.FailureCount, result.SuccessCount, result.FirstMessage)
		}
		return 0, errfmt.Errorf(ErrMsgDeleteEventsNoDet,
			result.FailureCount, result.SuccessCount)
	}

	return result.SuccessCount, nil
}
