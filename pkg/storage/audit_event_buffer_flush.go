package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// Keys for aggregated rollup / metadata shapes that do not have pkg/objects field key constants.
const (
	auditBufferRollupKeyCount           = FieldKeyAggCount
	auditBufferRollupKeyFirstOccurrence = FieldKeyAggFirstOccurrence
	auditBufferRollupKeyLastOccurrence  = FieldKeyAggLastOccurrence
	auditBufferAggMetaKeyAggregationKey = FieldKeyAggAggregationKey
	auditBufferAggMetaSourceAuditBuffer = ValueAuditEventBuffer
)

// periodicFlush periodically flushes aggregated events
func (b *AuditEventBuffer) periodicFlush() {
	// This method is now called from StartWithContext, so ctx parameter is available
	// But we keep the method signature for backward compatibility
	// The actual context is passed via closure in StartWithContext
	for {
		select {
		case <-b.ctx.Done():
			// Context cancelled - exit gracefully
			return
		case <-b.stopChan:
			// Stop signal received - exit gracefully
			return
		case <-b.flushTicker.C:
			if err := b.Flush(); err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Debug(ErrMsgPeriodicFlushFail).WithError(err).Log()
			}
		}
	}
}

// Flush flushes all buffered events as aggregated summaries
func (b *AuditEventBuffer) Flush() error {
	var keys []string
	var groups map[string]*AggregationGroup
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	// Use bounded timeout to prevent deadlocks
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()
	err := concurrency.RunInRLock(&b.mu, func() error {
		if len(b.buffer) == 0 {
			return nil
		}
		keys = make([]string, 0, len(b.buffer))
		groups = make(map[string]*AggregationGroup)
		for key, group := range b.buffer {
			keys = append(keys, key)
			groupCopy := *group
			groups[key] = &groupCopy
		}
		return nil
	})
	if err != nil {
		return errfmt.Newf(ErrMsgCopyBuffer).Wrap(err)
	}
	if len(keys) == 0 {
		return nil
	}

	// Flush groups (NO LOCK HELD - prevents deadlocks during I/O)
	for _, key := range keys {
		group := groups[key]
		// Emit flush start event via coordinator
		if group != nil {
			b.emitFlushEvent(StatusStart, key, group, 0, nil)
		}

		groupStartTime := time.Now()
		var groupToFlush *AggregationGroup
		_ = concurrency.RunInLock(&b.mu, func() error {
			if g, exists := b.buffer[key]; exists && g != nil {
				copy := *g
				groupToFlush = &copy
				delete(b.buffer, key)
			}
			return nil
		})

		if groupToFlush == nil || groupToFlush.Count == 0 {
			continue
		}

		flushErr := b.flushExtractedGroup(ctx, key, groupToFlush)
		if flushErr != nil {
			// Log error but continue with other groups
			StorageLog(logger).Warn(LogEventStorageAuditBufferFlushGroupFailedWarn).
				String("key", key).
				WithError(flushErr).
				Log()
			// Emit error event
			if group != nil {
				b.emitFlushEvent(StatusError, key, group, time.Since(groupStartTime), flushErr)
			}
		}
	}

	return nil
}

// flushGroup flushes a specific group (called from AddEvent when threshold exceeded)
// Captures callback/channel references before starting work to report errors.
// Uses a snapshot of the group so emitFlushEvent and post-flush logic never touch
// the live *AggregationGroup after releasing the lock (avoids data races with AddEvent).
func (b *AuditEventBuffer) flushGroup(key string) {
	// Capture callback, channel, and extract the group under lock
	var callback FlushErrorCallback
	var progressChan chan FlushProgress
	var groupToFlush *AggregationGroup

	err := concurrency.RunInLock(&b.mu, func() error {
		callback = b.flushErrorCallback
		progressChan = b.flushProgressChan
		if g, exists := b.buffer[key]; exists && g != nil {
			copy := *g
			groupToFlush = &copy
			// Remove from buffer immediately so we don't hold the lock during slow I/O
			delete(b.buffer, key)
		}
		return nil
	})
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(ErrMsgReadLockFailInd).WithError(err).Log()
	}

	if groupToFlush == nil || groupToFlush.Count == 0 {
		return // Nothing to flush
	}

	// Emit flush start event via coordinator
	b.emitFlushEvent(StatusStart, key, groupToFlush, 0, nil)

	// Transactional flush: run without holding the buffer lock!
	startTime := time.Now()
	flushCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	err = b.flushExtractedGroup(flushCtx, key, groupToFlush)
	duration := time.Since(startTime)

	// Emit flush completion event via coordinator (only if error, since success is emitted in flushGroupLocked)
	if err != nil && groupToFlush != nil {
		b.emitFlushEvent(StatusError, key, groupToFlush, duration, err)
	}

	// Report result (non-blocking, no locks held)
	if err != nil {
		if callback != nil {
			callback(key, err)
		}
		if progressChan != nil {
			select {
			case progressChan <- FlushProgress{Key: key, Status: StatusError, Error: err}:
			default:
				// Channel full, drop - don't block flush goroutine
			}
		}
	} else {
		if callback != nil {
			callback(key, nil)
		}
		if progressChan != nil {
			select {
			case progressChan <- FlushProgress{Key: key, Status: StatusSuccess}:
			default:
				// Channel full, drop - don't block flush goroutine
			}
		}
	}
}

// emitFlushEvent emits a flush operation event via coordinator
func (b *AuditEventBuffer) emitFlushEvent(
	status string,
	key string,
	group *AggregationGroup,
	duration time.Duration,
	err error,
) {
	callback := getAuditBufferFlushEventCallback()
	if callback == nil || b.projectRoot == emptyValue || b.fileStorage == nil {
		// Coordinator not available - skip
		return
	}

	// Build aggregation window string
	windowStr := fmt.Sprintf("%s to %s",
		zqktime.FormatRFC3339UTC(group.FirstSeen),
		zqktime.FormatRFC3339UTC(group.LastSeen))

	// Use system context for background event emission
	ctx := pkgctx.NewSystemContext()
	operationID := fmt.Sprintf("%s_%s_%d", OpNameAuditBufferFlush, key, time.Now().UnixNano())
	operationType := OpNameAuditBufferFlush

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine(OpNameAuditBufferCoordinatorEvent, fmt.Sprintf(DescEmitFlushEvent, key)).
		StartSimple(func() {
			callback(
				ctx,
				b.projectRoot,
				b.fileStorage,
				operationID,
				operationType,
				status,
				key,
				group.EventType,
				group.TargetKind,
				group.Count,
				windowStr,
				duration,
				err,
			)
		})
}

// flushExtractedGroup flushes a specific group that has been extracted from the buffer (no locks held)
func (b *AuditEventBuffer) flushExtractedGroup(ctx context.Context, key string, group *AggregationGroup) error {

	// Create aggregated_summary audit event
	now := time.Now().UTC()
	month := now.Format("2006-01")
	auditDir := filepath.Join(b.projectRoot, paths.ProcessAuditDir, month)
	if err := os.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateAuditDir).Wrap(err)
	}

	// Find next sequence number
	// Use thread-safe batch ID generator with CAS support (if fileStorage is available)
	var storageProvider ObjectStorageProvider
	if b.fileStorage != nil {
		storageProvider = b.fileStorage
	}
	// Use context from Flush() method (already defined with timeout)
	generator := GetAuditIDGenerator(ctx, auditDir, storageProvider)
	auditID, err := generator.GenerateNextID()
	if err != nil {
		return errfmt.Newf(ErrMsgNextAuditID).Wrap(err)
	}

	// Build aggregated_events array
	aggregatedEvents := []map[string]any{
		{
			objects.FieldKeyEventType:           group.EventType,
			auditBufferRollupKeyCount:           group.Count,
			auditBufferRollupKeyFirstOccurrence: zqktime.FormatRFC3339UTC(group.FirstSeen),
			auditBufferRollupKeyLastOccurrence:  zqktime.FormatRFC3339UTC(group.LastSeen),
		},
	}
	if group.TargetKind != emptyValue {
		aggregatedEvents[0][objects.FieldKeyTargetKind] = group.TargetKind
	}

	// Build operation description
	operation := fmt.Sprintf(DescAggregatedEventsFmt, group.Count, group.EventType)
	if group.TargetKind != emptyValue {
		operation += fmt.Sprintf(" for %s", group.TargetKind)
	}

	// Build aggregation window string
	windowStr := fmt.Sprintf("%s to %s",
		zqktime.FormatRFC3339UTC(group.FirstSeen),
		zqktime.FormatRFC3339UTC(group.LastSeen))

	// Get actor
	actor := ValueSystem
	if b.secCtx != nil && b.secCtx.AccountID != emptyValue {
		actor = b.secCtx.AccountID
	}

	// Create aggregated_summary audit event
	aggregatedEvent := map[string]any{
		objects.FieldKeyID:                auditID,
		objects.FieldKeyKind:              objects.KindAuditEvent,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:         now.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:         actor,
		objects.FieldKeyUpdatedAt:         now.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:         actor,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyStatus:            StatusCompleted,
		objects.FieldKeyEventType:         EventTypeAggregatedSummary,
		objects.FieldKeyOperation:         operation,
		objects.FieldKeyTargetKind:        group.TargetKind,
		objects.FieldKeySeverity:          group.Severity,
		objects.FieldKeyAggregationWindow: windowStr,
		objects.FieldKeyAggregatedCount:   group.Count,
		objects.FieldKeyAggregatedEvents:  aggregatedEvents,
		objects.FieldKeyMetadata: map[string]any{
			objects.FieldKeySource:              auditBufferAggMetaSourceAuditBuffer,
			auditBufferAggMetaKeyAggregationKey: key,
			objects.FieldKeyPreservedSamples:    len(group.SampleEvents),
		},
	}

	// Add preserved samples if any
	if len(group.SampleEvents) > 0 {
		aggregatedEvent[objects.FieldKeyPreservedSamples] = group.SampleEvents
	}

	// Validate aggregated event
	validator := validation.NewGoValidator()
	// Use context with timeout for validation (background flush operation)
	ctx, cancel := b.getContext()
	defer cancel()
	validationOptions := &validation.ValidationOptions{
		CurrentState:          ValueStatusCompleted,
		ValidateLifecycle:     true,
		ValidateSemanticTypes: false,
	}

	result, err := validator.Validate(ctx, aggregatedEvent, objects.KindAuditEvent, validationOptions)
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferFlushValidateFailedWarn).
			String("audit_id", auditID).
			WithError(err).
			Log()
	} else if !result.IsValid {
		var errorMessages []string
		for _, validationErr := range result.Errors {
			errorMessages = append(errorMessages, fmt.Sprintf("%s: %s", validationErr.Field, validationErr.Message))
		}
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferFlushValidationFailedWarn).
			String("audit_id", auditID).
			String("errors", strings.Join(errorMessages, "; ")).
			Log()
	}

	// Write aggregated event: if TSDB is available, use it instead of CAS/Stream YAMLs.
	// This avoids creating high-frequency stream segments for aggregated background events.
	fileStorage := b.fileStorage
	startTime := time.Now()
	var writeErr error
	var usedCAS bool

	if b.tsdb != nil {
		// TSDB mode: emit point instead of file storage
		usedCAS = false
		pt := TSDBPoint{
			Measurement: "audit_metric", // Or "audit_event_summary"
			Tags: map[string]string{
				objects.FieldKeyEventType:  group.EventType,
				objects.FieldKeyTargetKind: group.TargetKind,
				objects.FieldKeySeverity:   group.Severity,
			},
			Fields:    aggregatedEvent,
			Timestamp: time.Now().UTC(),
		}
		writeErr = b.tsdb.WritePoint(ctx, pt)
	} else {
		// Fallback to legacy file-based persistence (Stream or CAS)
		auditFilePath := filepath.Join(auditDir, fmt.Sprintf("%s.yaml", auditID))
		var data []byte
		data, writeErr = FormatMultiLineYAML(aggregatedEvent)
		if writeErr == nil {
			secCtx := pkgctx.NewSystemSecurityContext()
			if StreamStorageEnabledForKind(objects.KindAuditEvent) {
				if fileStorage == nil {
					writeErr = errfmt.Errorf(ErrMsgNoFileStorage)
				} else {
					usedCAS = false
					writeErr = fileStorage.writeObjectToStorage(ctx, auditID, objects.KindAuditEvent, "", data, secCtx)
				}
			} else {
				usedCAS = fileStorage != nil && fileStorage.usesContentAddressableStorage(objects.KindAuditEvent)
				baseAuditDir := filepath.Dir(auditDir)
				writeErr = WriteSystemObjectAndRegisterHash(auditFilePath, data, objects.KindAuditEvent, baseAuditDir, auditID, fileStorage)
			}
		} else {
			writeErr = errfmt.Newf(ErrMsgFormatAggEvent).Wrap(writeErr)
		}
	}
	duration := time.Since(startTime)
	success := writeErr == nil

	// Record metrics (route through proper metrics handler, not leaky pipe)
	metrics := GetGlobalAuditMetricsCollector()
	metrics.RecordAuditEventCreation(ctx, EventTypeAggregatedSummary, usedCAS, duration, success)
	metrics.RecordAuditEventValidation(result.IsValid)

	if writeErr != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferFlushWriteOrHashFailedWarn).
			String("audit_id", auditID).
			WithError(writeErr).
			Log()
		return errfmt.Newf(ErrMsgWriteAggEvent).Wrap(writeErr)
	}

	// Remove group from buffer
	delete(b.buffer, key)

	// Emit flush completion event via coordinator
	b.emitFlushEvent(StatusComplete, key, group, time.Since(now), nil)
	b.eventsFlushedTotal.Add(int64(group.Count))

	return nil
}
