package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

// AuditBufferFlushEventCallback is a callback for emitting events via coordinator
// This avoids import cycles by using dependency injection
type AuditBufferFlushEventCallback func(
	ctx context.Context,
	projectRoot string,
	storage ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	groupKey string,
	eventType string,
	targetKind string,
	eventCount int,
	aggregationWindow string,
	duration time.Duration,
	err error,
)

var (
	globalAuditBufferFlushEventCallback atomic.Pointer[AuditBufferFlushEventCallback]
)

// SetAuditBufferFlushEventCallback sets the global callback for emitting events via coordinator
// This should be called during system initialization to wire up coordinator integration
func SetAuditBufferFlushEventCallback(callback AuditBufferFlushEventCallback) {
	if callback == nil {
		globalAuditBufferFlushEventCallback.Store(nil)
		return
	}
	ptr := new(AuditBufferFlushEventCallback)
	*ptr = callback
	globalAuditBufferFlushEventCallback.Store(ptr)
}

func getAuditBufferFlushEventCallback() AuditBufferFlushEventCallback {
	ptr := globalAuditBufferFlushEventCallback.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// FlushProgress represents the progress/result of a flush operation
type FlushProgress struct {
	Key    string
	Status string // "success", "error"
	Error  error
}

// FlushErrorCallback is called when a flush operation completes (success or failure)
type FlushErrorCallback func(key string, err error)

// AuditEventBuffer handles in-memory buffering of low-severity audit events
// for aggregation before writing to disk (Hybrid Approach from design doc)
type AuditEventBuffer struct {
	mu                 sync.RWMutex
	buffer             map[string]*AggregationGroup
	windowSize         time.Duration
	threshold          int
	flushTicker        *time.Ticker
	stopChan           chan struct{}
	ctx                context.Context    // Context for goroutine lifecycle management
	cancel             context.CancelFunc // Cancel function for context
	projectRoot        string
	secCtx             *pkgctx.SecurityContext
	enabled            bool
	preserveSamples    int
	rules              []AggregationRule
	fileStorage        *FileObjectStorage // Optional fileStorage for CAS routing
	tsdb               TSDBProvider       // Optional TSDB provider for time-series persistence
	flushErrorCallback FlushErrorCallback // Optional callback for flush errors
	flushProgressChan  chan FlushProgress // Optional channel for flush progress/errors
	eventsAddedTotal   atomic.Int64
	eventsFlushedTotal atomic.Int64
}

// AggregationGroup is the storage-root alias for audit.Group.
type AggregationGroup = audit.Group

// AggregationRule is the storage-root alias for audit.Rule.
type AggregationRule = audit.Rule

// DefaultAggregationRules returns default aggregation rules per design document.
func DefaultAggregationRules() []AggregationRule {
	return audit.DefaultRules()
}
