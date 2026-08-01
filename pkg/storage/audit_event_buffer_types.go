package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
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

// AggregationGroup represents a group of events that will be aggregated together
type AggregationGroup struct {
	Key          string
	EventType    string
	TargetKind   string
	Severity     string
	Count        int
	FirstSeen    time.Time
	LastSeen     time.Time
	SampleEvents []map[string]any // Keep N samples for forensics
	Events       []map[string]any // All events in this group (for aggregation)
}

// AggregationRule defines which events should be aggregated
type AggregationRule struct {
	EventTypes      []string      `yaml:"event_types"`
	Severities      []string      `yaml:"severities"`
	GroupBy         []string      `yaml:"group_by"` // ["event_type", "target_kind"]
	Window          time.Duration `yaml:"window"`
	Threshold       int           `yaml:"threshold"`
	PreserveSamples int           `yaml:"preserve_samples"`
}

// DefaultAggregationRules returns default aggregation rules per design document
func DefaultAggregationRules() []AggregationRule {
	return []AggregationRule{
		{
			EventTypes:      []string{EventTypeCacheInvalidation, EventTypeCacheUpdate},
			Severities:      []string{SeverityLow},
			GroupBy:         []string{objects.FieldKeyEventType, objects.FieldKeyTargetKind},
			Window:          time.Hour,
			Threshold:       10,
			PreserveSamples: 5,
		},
		{
			EventTypes:      []string{EventTypeCacheBulkInvalidation},
			Severities:      []string{SeverityLow, SeverityMedium},
			GroupBy:         []string{objects.FieldKeyEventType, objects.FieldKeyTargetKind},
			Window:          time.Hour,
			Threshold:       5,
			PreserveSamples: 3,
		},
		{
			EventTypes:      []string{EventTypeSchedulerJobStarted, EventTypeSchedulerJobCompleted},
			Severities:      []string{SeverityLow},
			GroupBy:         []string{objects.FieldKeyEventType, objects.FieldKeyTargetKind, objects.FieldKeyJobType},
			Window:          5 * time.Minute, // Flush after 5 minutes max (reduces latency)
			Threshold:       20,              // Aggregate after 20 events (reduces I/O for high-frequency jobs)
			PreserveSamples: 5,
		},
	}
}
