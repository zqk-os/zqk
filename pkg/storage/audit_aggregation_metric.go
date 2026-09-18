// Extracted from audit_aggregation_helpers.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func (s *AuditAggregationService) createAggregationMetricSync(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	metric map[string]any,
) (string, error) {
	// Enforce per-source, per-window uniqueness before attempting creation:
	// - If an exact window match exists for this source, update that metric (upsert-by-window).
	// - If any overlapping window exists for this source with a different window, reject to avoid double-aggregation.
	windowStart, _ := metric[objects.FieldKeyAggregationWindowStart].(string)
	windowEnd, _ := metric[objects.FieldKeyAggregationWindowEnd].(string)
	source, _ := metric[objects.FieldKeySource].(string)
	if windowStart != emptyValue && windowEnd != emptyValue && source != emptyValue {
		existing, err := s.findExistingMetricByWindow(ctx, secCtx, windowStart, windowEnd)
		if err == nil && existing != nil {
			existingID, _ := existing[objects.FieldKeyID].(string)
			if existingID != emptyValue {
				updates := audit.MetricUpdateFields(metric, zqktime.NowRFC3339UTC(), pkgctx.SystemAccountID)
				if err := s.metrics().Update(ctx, secCtx, existingID, updates); err != nil {
					return "", errfmt.Errorf(ErrMsgUpdateAggMetricWin, windowStart, windowEnd, err)
				}
				return existingID, nil
			}
		}

		// If a different metric overlaps this window for the same source, reject to avoid double-aggregation.
		overlappingID := s.findExistingMetricByOverlappingWindow(ctx, secCtx, windowStart, windowEnd)
		if overlappingID != emptyValue {
			return "", errfmt.Errorf(ErrMsgOverlapAggMetric, source, overlappingID, windowStart, windowEnd)
		}
	}

	// Try to create the metric
	err := s.metrics().Create(ctx, secCtx, metric)
	if err != nil {
		// If object already exists, try to find and update the existing metric
		if err == ErrObjectExists || strings.Contains(err.Error(), ErrMsgAlreadyExists) {
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
				updates := audit.MetricUpdateFields(metric, zqktime.NowRFC3339UTC(), pkgctx.SystemAccountID)

				updateErr := s.metrics().Update(ctx, secCtx, metricID, updates)
				if updateErr != nil {
					// If update fails due to hash mismatch, missing hash file, or stale CAS index entry,
					// skip the problematic metric and create a new one instead.
					// This handles cases where existing metrics have hash mismatches or CAS inconsistencies.
					updateErrStr := updateErr.Error()
					if strings.Contains(updateErrStr, "hash mismatch") ||
						strings.Contains(updateErrStr, ErrMsgReadHashFile) ||
						strings.Contains(updateErrStr, ErrMsgNoExist) ||
						strings.Contains(updateErrStr, ErrMsgBlockingIssues) {
						// Object has hash mismatch or is referenced in index but file doesn't exist
						// Skip this problematic metric and create a new one instead
						// Log the issue for visibility but don't fail the aggregation
						logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
						when.When(func() bool { return strings.Contains(updateErrStr, "hash mismatch") }).Then(func() {
							StorageLog(logger).Warn(LogEventStorageMetricInstanceSkipHashMismatch).
								MetricID(metricID).
								String("error", updateErrStr).
								Log()
						}).OrElse(func() {
							StorageLog(logger).Warn(LogEventStorageMetricInstanceSkipStaleCASIndex).
								MetricID(metricID).
								String("error", updateErrStr).
								Log()
							// Try to clean up stale CAS index entry if it's a file storage issue
							if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
								cas, casErr := fileStorage.getContentAddressableStorage(MetricKindAuditAggregation)
								if casErr == nil {
									// Directly remove from CAS index (this will handle missing hash files gracefully)
									casDeleteErr := cas.Delete(metricID)
									if casDeleteErr != nil {
										// If error is ErrMsgObjectNotFound, that's fine - another goroutine already cleaned it up
										// Only log warnings for other errors
										casDeleteErrStr := casDeleteErr.Error()
										if !strings.Contains(casDeleteErrStr, ErrMsgObjectNotFound) {
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
						}).Run()
						// Remove ID from metric so a new one can be generated
						delete(metric, objects.FieldKeyID)
						createErr := s.metrics().Create(ctx, secCtx, metric)
						if createErr != nil {
							// If create still fails with "object already exists", another goroutine may have created it
							// Try to find and return the existing metric ID
							if createErr == ErrObjectExists || strings.Contains(createErr.Error(), ErrMsgAlreadyExists) {
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
							return "", errfmt.Newf(NoteStaleAggMetric).Wrap(createErr)
						}
						// Return the newly generated ID
						newID, ok := metric[objects.FieldKeyID].(string)
						if !ok {
							return "", errfmt.Errorf(ErrMsgMetricIDNotSet)
						}
						return newID, nil
					}
					return "", errfmt.Newf(ErrMsgUpdateAggMetric).Wrap(updateErr)
				}
				return metricID, nil
			}
			// If we couldn't find the existing metric, return a more helpful error
			return "", errfmt.Errorf(ErrMsgLocateAggMetric,
				metric[objects.FieldKeyAggregationWindowStart], metric[objects.FieldKeyAggregationWindowEnd], err)
		}
		// Other errors - return original error
		return "", err
	}

	// Return the ID that was generated
	id, ok := metric[objects.FieldKeyID].(string)
	if !ok {
		return "", errfmt.Errorf(ErrMsgMetricIDNotSet)
	}

	return id, nil
}

// mergeAggregationMetrics merges two aggregation metrics into one
// Combines counts, ID lists, and other aggregations
