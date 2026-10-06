package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
		keys, groups = audit.CloneGroups(b.buffer)
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
			groupToFlush = audit.ExtractGroup(b.buffer, key)
			return nil
		})

		if !audit.ShouldFlushGroup(groupToFlush) {
			continue
		}

		flushErr := b.flushExtractedGroup(ctx, key, groupToFlush)
		if flushErr != nil {
			// Log error but continue with other groups
			StorageLog(logger).Warn(LogEventStorageAuditBufferFlushGroupFailedWarn).
				String("group_key", key).
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
		groupToFlush = audit.ExtractGroup(b.buffer, key)
		return nil
	})
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(ErrMsgReadLockFailInd).WithError(err).Log()
	}

	if !audit.ShouldFlushGroup(groupToFlush) {
		return
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
	windowStr := audit.WindowString(group)

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
	auditDir := audit.MonthlyDir(b.projectRoot, now)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateAuditDir).Wrap(err)
	}

	validateCtx, cancel := b.getContext()
	defer cancel()
	plan, validated, written, err := audit.FlushRun(
		ctx,
		validateCtx,
		auditDir,
		func(allocCtx context.Context, dir string) audit.IDAllocator {
			var storageProvider ObjectStorageProvider
			if b.fileStorage != nil {
				storageProvider = b.fileStorage
			}
			return GetAuditIDGenerator(allocCtx, dir, storageProvider)
		},
		auditFlushSink{tsdb: b.tsdb, fileStorage: b.fileStorage},
		GetGlobalAuditMetricsCollector(),
		audit.FlushActorFromSec(b.secCtx),
		key,
		group,
		now,
	)
	if err != nil {
		return errfmt.Newf(ErrMsgNextAuditID).Wrap(err)
	}
	auditID := plan.ID
	if validated.Err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferFlushValidateFailedWarn).
			String("audit_id", auditID).
			WithError(validated.Err).
			Log()
	} else if !validated.OK {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferFlushValidationFailedWarn).
			String("audit_id", auditID).
			String("errors", strings.Join(validated.Messages, "; ")).
			Log()
	}

	if written.Err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferFlushWriteOrHashFailedWarn).
			String("audit_id", auditID).
			WithError(written.Err).
			Log()
		return errfmt.Newf(ErrMsgWriteAggEvent).Wrap(written.Err)
	}

	// Remove group from buffer
	delete(b.buffer, key)

	// Emit flush completion event via coordinator
	b.emitFlushEvent(StatusComplete, key, group, time.Since(now), nil)
	b.eventsFlushedTotal.Add(int64(group.Count))

	return nil
}
