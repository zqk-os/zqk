package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/audit"
)

var (
	pendingAuditEventsEnqueuedTotal atomic.Int64
	pendingAuditEventsFlushedTotal  atomic.Int64
)

// GetPendingAuditEventStats returns lifetime counters for enqueued and flushed pending audit events.
func GetPendingAuditEventStats() (enqueued, flushed int64) {
	return pendingAuditEventsEnqueuedTotal.Load(), pendingAuditEventsFlushedTotal.Load()
}

// WithDeferAuditEvents marks the context so create*AuditEvent enqueue instead of creating immediately.
// Caller must call FlushPendingAuditEvents at the end of the bulk operation.
func WithDeferAuditEvents(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	return audit.WithDeferEvents(ctx)
}

// IsDeferAuditEvents returns true when ctx was marked with WithDeferAuditEvents.
func IsDeferAuditEvents(ctx context.Context) bool {
	return audit.HasDeferEvents(ctx)
}

// PendingAuditItem holds enough data to build one audit event at flush time.
type PendingAuditItem struct {
	ProjectRoot string
	SecCtx      *pkgctx.SecurityContext
	FileStorage *FileObjectStorage
	Options     *AuditEventOptions
}

var (
	pendingAuditMu   sync.Mutex
	pendingAuditList []PendingAuditItem
)

// EnqueuePendingAuditEvent appends an audit event to the pending queue (call when IsDeferAuditEvents).
func EnqueuePendingAuditEvent(item PendingAuditItem) {
	if item.Options == nil {
		return
	}
	pendingAuditEventsEnqueuedTotal.Add(1)
	pendingAuditMu.Lock()
	defer pendingAuditMu.Unlock()
	pendingAuditList = append(pendingAuditList, item)
}

// FlushPendingAuditEvents drains the pending queue for projectRoot, builds audit event instances,
// and BulkCreates them with deferred CAS flush. Safe to call with no pending events.
func FlushPendingAuditEvents(ctx context.Context, projectRoot string, storageProvider ObjectStorageProvider, secCtx *pkgctx.SecurityContext) {
	if storageProvider == nil || projectRoot == emptyValue {
		return
	}
	pendingAuditMu.Lock()
	// Drain items for this project
	var batch []PendingAuditItem
	var rest []PendingAuditItem
	for _, item := range pendingAuditList {
		if item.ProjectRoot == projectRoot {
			batch = append(batch, item)
		} else {
			rest = append(rest, item)
		}
	}
	pendingAuditList = rest
	pendingAuditMu.Unlock()

	if len(batch) == 0 {
		return
	}
	pendingAuditEventsFlushedTotal.Add(int64(len(batch)))

	// Build instances and BulkCreate (best effort)
	instances := buildPendingAuditInstances(ctx, projectRoot, batch, storageProvider)
	if len(instances) == 0 {
		return
	}
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	bulkCtx := WithBulkCreateDeferFlush(ctx)
	if _, err := storageProvider.BulkCreate(bulkCtx, secCtx, instances); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBulkCreatePendingFailed).
			Int(FieldKeyCount, len(instances)).
			WithError(err).
			Log()
		return
	}
	// Flush CAS index for audit_event so they are visible
	queue := caspkg.GetListingIndexWriteQueueForProjectRoot(projectRoot)
	const flushTimeout = 5 * time.Second
	if err := queue.FlushKind(objects.KindAuditEvent, flushTimeout); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(ErrMsgFlushCASIndex).
			String(FieldKeyProjectRoot, projectRoot).
			WithError(err).
			Log()
	}
}

// buildPendingAuditInstances builds one audit event instance per item; returns only successful builds.
func buildPendingAuditInstances(ctx context.Context, _ string, batch []PendingAuditItem, _ ObjectStorageProvider) []map[string]any {
	var instances []map[string]any
	for i := range batch {
		item := &batch[i]
		inst, err := BuildAuditEventInstance(ctx, item.ProjectRoot, item.SecCtx, item.Options, item.FileStorage)
		if err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Debug(LogEventStorageAuditBulkBuildPendingFailedDebug).
				Int("index", i).
				WithError(err).
				Log()
			continue
		}
		if inst != nil {
			instances = append(instances, inst)
		}
	}
	return instances
}
