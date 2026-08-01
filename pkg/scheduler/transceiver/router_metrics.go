package transceiver

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/when"
)

// RouterMetrics tracks router performance and health
type RouterMetrics struct {
	mu sync.RWMutex

	// Message routing metrics
	MessagesRouted  int64
	MessagesMatched int64
	MessagesFailed  int64
	MessagesDropped int64 // Messages dropped due to queue full

	// Rule matching metrics
	RulesEvaluated int64
	RulesMatched   int64

	// Action execution metrics
	ActionsExecuted  int64
	ActionsSucceeded int64
	ActionsFailed    int64

	// Timing metrics
	TotalRoutingTime time.Duration
	MaxRoutingTime   time.Duration
	MinRoutingTime   time.Duration
	RoutingCount     int64

	// Queue metrics (for async router)
	QueueDepth        int64
	QueueFullCount    int64
	QueueDroppedCount int64

	// Adapter metrics
	AdapterCalls    map[string]int64
	AdapterFailures map[string]int64
	adapterMu       sync.RWMutex

	startTime time.Time
}

// NewRouterMetrics creates a new router metrics tracker
func NewRouterMetrics() *RouterMetrics {
	return &RouterMetrics{
		MinRoutingTime:  time.Hour, // Initialize to large value
		AdapterCalls:    make(map[string]int64),
		AdapterFailures: make(map[string]int64),
		startTime:       time.Now(),
	}
}

// RecordMessageRouted records a message routing attempt
func (rm *RouterMetrics) RecordMessageRouted() {
	atomic.AddInt64(&rm.MessagesRouted, 1)
}

// RecordMessageMatched records that a message matched a rule
func (rm *RouterMetrics) RecordMessageMatched() {
	atomic.AddInt64(&rm.MessagesMatched, 1)
}

// RecordMessageFailed records a failed message routing
func (rm *RouterMetrics) RecordMessageFailed() {
	atomic.AddInt64(&rm.MessagesFailed, 1)
}

// RecordMessageDropped records a dropped message (queue full)
func (rm *RouterMetrics) RecordMessageDropped() {
	atomic.AddInt64(&rm.MessagesDropped, 1)
}

// RecordRuleEvaluated records a rule evaluation
func (rm *RouterMetrics) RecordRuleEvaluated() {
	atomic.AddInt64(&rm.RulesEvaluated, 1)
}

// RecordRuleMatched records a rule match
func (rm *RouterMetrics) RecordRuleMatched() {
	atomic.AddInt64(&rm.RulesMatched, 1)
}

// RecordActionExecuted records an action execution
func (rm *RouterMetrics) RecordActionExecuted(adapterName string, success bool) {
	atomic.AddInt64(&rm.ActionsExecuted, 1)
	when.When(func() bool { return success }).Then(func() {
		atomic.AddInt64(&rm.ActionsSucceeded, 1)
	}).OrElse(func() {
		atomic.AddInt64(&rm.ActionsFailed, 1)
	}).Run()

	// Record adapter-specific metrics
	_ = concurrency.RunInLockWithLogger(
		&rm.adapterMu,
		LockNameRouterMetricsRecordAdapter,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			rm.AdapterCalls[adapterName]++
			if !success {
				rm.AdapterFailures[adapterName]++
			}
			return nil
		},
	)
}

// RecordRoutingTime records routing duration
func (rm *RouterMetrics) RecordRoutingTime(duration time.Duration) {
	_ = concurrency.RunInLockWithLogger(
		&rm.mu,
		LockNameRouterMetricsRecordRoutingTime,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			rm.TotalRoutingTime += duration
			rm.RoutingCount++

			if duration > rm.MaxRoutingTime {
				rm.MaxRoutingTime = duration
			}
			if duration < rm.MinRoutingTime {
				rm.MinRoutingTime = duration
			}
			return nil
		},
	)
}

// UpdateQueueDepth updates the current queue depth
func (rm *RouterMetrics) UpdateQueueDepth(depth int64) {
	atomic.StoreInt64(&rm.QueueDepth, depth)
}

// RecordQueueFull records that the queue was full
func (rm *RouterMetrics) RecordQueueFull() {
	atomic.AddInt64(&rm.QueueFullCount, 1)
}

// RecordQueueDropped records a dropped message due to queue full
func (rm *RouterMetrics) RecordQueueDropped() {
	atomic.AddInt64(&rm.QueueDroppedCount, 1)
}

// GetSnapshot returns a snapshot of current metrics
func (rm *RouterMetrics) GetSnapshot() RouterMetricsSnapshot {
	var snapshot RouterMetricsSnapshot
	_ = concurrency.RunInRLockWithLogger(
		&rm.mu,
		LockNameRouterMetricsGetSnapshotMain,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var adapterCallsLocal, adapterFailuresLocal map[string]int64
			_ = concurrency.RunInRLockWithLogger(
				&rm.adapterMu,
				LockNameRouterMetricsGetSnapshotAdapter,
				logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					// Copy adapter maps
					adapterCallsLocal = make(map[string]int64)
					adapterFailuresLocal = make(map[string]int64)
					for k, v := range rm.AdapterCalls {
						adapterCallsLocal[k] = v
					}
					for k, v := range rm.AdapterFailures {
						adapterFailuresLocal[k] = v
					}
					return nil
				},
			)

			var avgRoutingTime time.Duration
			if rm.RoutingCount > 0 {
				avgRoutingTime = rm.TotalRoutingTime / time.Duration(rm.RoutingCount)
			}

			snapshot = RouterMetricsSnapshot{
				Timestamp:         time.Now(),
				Uptime:            time.Since(rm.startTime),
				MessagesRouted:    atomic.LoadInt64(&rm.MessagesRouted),
				MessagesMatched:   atomic.LoadInt64(&rm.MessagesMatched),
				MessagesFailed:    atomic.LoadInt64(&rm.MessagesFailed),
				MessagesDropped:   atomic.LoadInt64(&rm.MessagesDropped),
				RulesEvaluated:    atomic.LoadInt64(&rm.RulesEvaluated),
				RulesMatched:      atomic.LoadInt64(&rm.RulesMatched),
				ActionsExecuted:   atomic.LoadInt64(&rm.ActionsExecuted),
				ActionsSucceeded:  atomic.LoadInt64(&rm.ActionsSucceeded),
				ActionsFailed:     atomic.LoadInt64(&rm.ActionsFailed),
				AvgRoutingTime:    avgRoutingTime,
				MaxRoutingTime:    rm.MaxRoutingTime,
				MinRoutingTime:    rm.MinRoutingTime,
				QueueDepth:        atomic.LoadInt64(&rm.QueueDepth),
				QueueFullCount:    atomic.LoadInt64(&rm.QueueFullCount),
				QueueDroppedCount: atomic.LoadInt64(&rm.QueueDroppedCount),
				AdapterCalls:      adapterCallsLocal,
				AdapterFailures:   adapterFailuresLocal,
			}
			return nil
		},
	)
	return snapshot
}

// RouterMetricsSnapshot provides a point-in-time view of router metrics
type RouterMetricsSnapshot struct {
	Timestamp         time.Time
	Uptime            time.Duration
	MessagesRouted    int64
	MessagesMatched   int64
	MessagesFailed    int64
	MessagesDropped   int64
	RulesEvaluated    int64
	RulesMatched      int64
	ActionsExecuted   int64
	ActionsSucceeded  int64
	ActionsFailed     int64
	AvgRoutingTime    time.Duration
	MaxRoutingTime    time.Duration
	MinRoutingTime    time.Duration
	QueueDepth        int64
	QueueFullCount    int64
	QueueDroppedCount int64
	AdapterCalls      map[string]int64
	AdapterFailures   map[string]int64
}

// GetSuccessRate returns the success rate as a percentage (0-100)
//
//nolint:gocritic // Value receiver used for metric snapshot; copying acceptable
func (snapshot RouterMetricsSnapshot) GetSuccessRate() float64 {
	if snapshot.ActionsExecuted == 0 {
		return 100.0
	}
	return float64(snapshot.ActionsSucceeded) / float64(snapshot.ActionsExecuted) * 100.0
}

// GetMatchRate returns the rule match rate as a percentage (0-100)
//
//nolint:gocritic // Value receiver used for metric snapshot; copying acceptable
func (snapshot RouterMetricsSnapshot) GetMatchRate() float64 {
	if snapshot.MessagesRouted == 0 {
		return 0.0
	}
	return float64(snapshot.MessagesMatched) / float64(snapshot.MessagesRouted) * 100.0
}
