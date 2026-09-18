// Extracted from change_journal_aggregation.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"errors"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func (s *ChangeJournalAggregationService) createAggregationMetricSync(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	metric map[string]any,
) (string, error) {
	// Try to create the metric
	err := s.storage.Create(ctx, secCtx, metric)
	if err != nil {
		// If object already exists, try to find and update the existing metric
		if errors.Is(err, ErrObjectExists) || strings.Contains(err.Error(), ConstAuditAlreadyExists) {
			var metricID string

			// First, check if metric has an ID (it might have been set during Create attempt)
			if id, hasID := metric[objects.FieldKeyID].(string); hasID && id != emptyValue {
				metricID = id
			} else {
				// No ID set - we need to find existing metric by time window
				// Query for existing metrics with matching time window
				windowStart, _ := metric[objects.FieldKeyAggregationWindowStart].(string)
				windowEnd, _ := metric[objects.FieldKeyAggregationWindowEnd].(string)
				if windowStart != emptyValue && windowEnd != emptyValue {
					existingMetric, findErr := s.findExistingMetricByWindow(ctx, secCtx, windowStart, windowEnd)
					if findErr == nil && existingMetric != nil {
						if id := objects.GetString(existingMetric, objects.FieldKeyID); id != emptyValue {
							metricID = id
						}
					}
					// If findExistingMetricByWindow failed, try a more lenient search
					// by querying for metrics with overlapping time windows
					if metricID == emptyValue {
						metricID = s.findExistingMetricByOverlappingWindow(ctx, secCtx, windowStart, windowEnd)
					}
				}
			}

			if metricID != emptyValue {
				// Update existing metric instead of creating new one
				// Remove fields that shouldn't be updated
				updates := make(map[string]any)
				for k, v := range metric {
					// Skip ID and timestamps that should be preserved
					if k != objects.FieldKeyID && k != objects.FieldKeyCreatedAt && k != objects.FieldKeyCreatedBy {
						updates[k] = v
					}
				}
				// Always update updated_at and updated_by
				updates[objects.FieldKeyUpdatedAt] = zqktime.NowRFC3339UTC()
				updates[objects.FieldKeyUpdatedBy] = pkgctx.SystemAccountID

				updateErr := s.storage.Update(ctx, secCtx, metricID, updates)
				if updateErr != nil {
					// If update fails due to hash mismatch, missing hash file, or stale CAS index entry,
					// skip the problematic metric and create a new one instead.
					// This handles cases where existing metrics have hash mismatches or CAS inconsistencies.
					updateErrStr := updateErr.Error()
					if strings.Contains(updateErrStr, "hash mismatch") ||
						strings.Contains(updateErrStr, ConstAuditFailedToReadHashFile) ||
						strings.Contains(updateErrStr, ConstAuditNoSuchFileOrDirectory) ||
						strings.Contains(updateErrStr, ConstAuditBlockingIssuesDetected) {
						// Object has hash mismatch or is referenced in index but file doesn't exist
						// Skip this problematic metric and create a new one instead
						// Log the issue for visibility but don't fail the aggregation
						logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
						if strings.Contains(updateErrStr, "hash mismatch") {
							StorageLog(logger).Warn(LogEventStorageMetricInstanceSkipHashMismatch).
								MetricID(metricID).
								String("error", updateErrStr).
								Log()
						} else {
							StorageLog(logger).Warn(LogEventStorageMetricInstanceSkipStaleCASIndex).
								MetricID(metricID).
								String("error", updateErrStr).
								Log()
							// Try to clean up stale CAS index entry if it's a file storage issue
							if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
								cas, casErr := fileStorage.getContentAddressableStorage(objects.KindAuditAggregationMetric)
								if casErr == nil {
									// Directly remove from CAS index (this will handle missing hash files gracefully)
									casDeleteErr := cas.Delete(metricID)
									if casDeleteErr != nil {
										// If error is "object not found", that's fine - another goroutine already cleaned it up
										// Only log warnings for other errors
										casDeleteErrStr := casDeleteErr.Error()
										if !strings.Contains(casDeleteErrStr, ConstAuditObjectNotFound) {
											StorageLog(logger).Warn(LogEventStorageMetricInstanceCleanupStaleCASFailed).
												MetricID(metricID).
												WithError(casDeleteErr).
												Log()
										}
									} else {
										updateReverseReferenceIndexOnDelete(metricID)
									}
								}
							}
						}
						// Remove ID from metric so a new one can be generated
						delete(metric, objects.FieldKeyID)
						createErr := s.storage.Create(ctx, secCtx, metric)
						if createErr != nil {
							// If create still fails with "object already exists", another goroutine may have created it
							// Try to find and return the existing metric ID
							if errors.Is(createErr, ErrObjectExists) || strings.Contains(createErr.Error(), ConstAuditAlreadyExists) {
								// Try to find existing metric by time window one more time
								windowStart, _ := metric[objects.FieldKeyAggregationWindowStart].(string)
								windowEnd, _ := metric[objects.FieldKeyAggregationWindowEnd].(string)
								if windowStart != emptyValue && windowEnd != emptyValue {
									existingMetric, findErr := s.findExistingMetricByWindow(ctx, secCtx, windowStart, windowEnd)
									if findErr == nil && existingMetric != nil {
										if id := objects.GetString(existingMetric, objects.FieldKeyID); id != emptyValue {
											return id, nil
										}
									}
									// Try overlapping window search
									if existingID := s.findExistingMetricByOverlappingWindow(ctx, secCtx, windowStart, windowEnd); existingID != emptyValue {
										return existingID, nil
									}
								}
							}
							return "", errfmt.Newf(ConstAuditFailedToCreateAggregationMetricAfterUpdateFailureStaleCasIndex).Wrap(createErr)
						}
						// Return the newly generated ID
						newID, ok := metric[objects.FieldKeyID].(string)
						if !ok {
							return "", errfmt.Errorf(ConstAuditMetricIdNotSetAfterCreation)
						}
						return newID, nil
					}
					return "", errfmt.Newf(ConstAuditFailedToUpdateExistingAggregationMetric).Wrap(updateErr)
				}
				return metricID, nil
			}
			// If we couldn't find the existing metric, return a more helpful error
			return "", errfmt.Errorf(ConstAuditObjectAlreadyExistsButCouldNotLocateExistingMetricForTimeWindowTo,
				metric[objects.FieldKeyAggregationWindowStart], metric[objects.FieldKeyAggregationWindowEnd], err)
		}
		// Other errors - return original error
		return "", errfmt.Newf(ConstAuditFailedToCreateAggregationMetric).Wrap(err)
	}

	// Return the ID that was generated
	id, ok := metric[objects.FieldKeyID].(string)
	if !ok {
		return "", errfmt.Errorf(ConstAuditMetricIdNotSetAfterCreation)
	}

	return id, nil
}

// mergeAggregationMetrics merges two aggregation metrics into one
// Combines counts, ID lists, and other aggregations
