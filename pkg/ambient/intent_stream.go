package ambient

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Ambient Intent Stream Constants & Watermark Defaults.
const (
	// DefaultRingBufferCapacity is the default capacity for the ambient intent ring buffer.
	DefaultRingBufferCapacity = 1024
	// DefaultActiveAgents is the initial default active agent count.
	DefaultActiveAgents = 1
	// DefaultCriticalRunwayDepth is the critical watermark threshold (depth <= 1.0).
	DefaultCriticalRunwayDepth = 1.0
	// DefaultWarningRunwayDepth is the warning watermark threshold (depth <= 2.0).
	DefaultWarningRunwayDepth = 2.0
	// DefaultAlertChannelBufferSize is the default buffer size for subscriber alert channels.
	DefaultAlertChannelBufferSize = 64
	// DefaultMonitorCheckInterval is the fallback ticker duration for the runway depth monitor.
	DefaultMonitorCheckInterval = 50 * time.Millisecond
)

var (
	// ErrIntentEventIDRequired is returned when an intent event has an empty ID.
	ErrIntentEventIDRequired = errfmt.Errorf("ambient intent event id is required")
	// ErrIntentEventIntentRequired is returned when an intent event has an empty intent description.
	ErrIntentEventIntentRequired = errfmt.Errorf("ambient intent event intent is required")
	// ErrIntentEventCorruptPayload is returned when an intent event contains null or control bytes.
	ErrIntentEventCorruptPayload = errfmt.Errorf("ambient intent event payload is corrupt")
	// ErrIntentEventCanceled is returned when an intent event status is canceled.
	ErrIntentEventCanceled = errfmt.Errorf("ambient intent event is canceled")
	// ErrRingBufferEmpty is returned when attempting to pop from an empty ring buffer.
	ErrRingBufferEmpty = errfmt.Errorf("ambient ring buffer is empty")
)

// WatermarkThreshold categorizes the swarm runway state.
type WatermarkThreshold string

const (
	// WatermarkThresholdNominal indicates sufficient runway depth (> warning threshold).
	WatermarkThresholdNominal WatermarkThreshold = "nominal"
	// WatermarkThresholdWarning indicates replenishment needed (depth <= warning threshold).
	WatermarkThresholdWarning WatermarkThreshold = "warning"
	// WatermarkThresholdCritical indicates imminent agent starvation (depth <= critical threshold).
	WatermarkThresholdCritical WatermarkThreshold = "critical"
)

// IntentEvent represents an agent intent event ingested into the ambient stream.
type IntentEvent struct {
	ID        string            `json:"id"`
	AgentID   string            `json:"agent_id"`
	Intent    string            `json:"intent"`
	Payload   string            `json:"payload,omitempty"`
	Status    string            `json:"status,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// Validate checks for structural integrity and fail-closes on corrupt or canceled payloads.
func (e IntentEvent) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return ErrIntentEventIDRequired
	}
	if strings.TrimSpace(e.Intent) == "" {
		return ErrIntentEventIntentRequired
	}
	if containsCorruptPayload(e.Intent) || containsCorruptPayload(e.Payload) {
		return ErrIntentEventCorruptPayload
	}
	status := strings.ToLower(strings.TrimSpace(e.Status))
	if status == "canceled" || status == "cancelled" {
		return ErrIntentEventCanceled
	}
	return nil
}

// SwarmRunwaySnapshot captures a point-in-time calculation of swarm runway depth.
type SwarmRunwaySnapshot struct {
	PendingIntents int                `json:"pending_intents"`
	ActiveAgents   int                `json:"active_agents"`
	RunwayDepth    float64            `json:"runway_depth"`
	Threshold      WatermarkThreshold `json:"threshold"`
	Timestamp      time.Time          `json:"timestamp"`
}

// SwarmRunwayAlert represents an automated alert emitted when runway depth drops below watermarks.
type SwarmRunwayAlert struct {
	Threshold   WatermarkThreshold  `json:"threshold"`
	RunwayDepth float64             `json:"runway_depth"`
	Message     string              `json:"message"`
	Snapshot    SwarmRunwaySnapshot `json:"snapshot"`
	Timestamp   time.Time           `json:"timestamp"`
}

// RingBuffer is a thread-safe circular ring buffer for IntentEvents with graceful eviction on overflow.
type RingBuffer struct {
	mu       sync.RWMutex
	capacity int
	buffer   []IntentEvent
	head     int // next write index
	tail     int // next read index
	count    int // current number of stored elements
	evicted  int64
}

// NewRingBuffer creates a new RingBuffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultRingBufferCapacity
	}
	return &RingBuffer{
		capacity: capacity,
		buffer:   make([]IntentEvent, capacity),
	}
}

// Push adds an event to the ring buffer. If the buffer is full, the oldest event is gracefully evicted.
// Returns true if an item was evicted, along with any validation error.
func (r *RingBuffer) Push(event IntentEvent) (bool, error) {
	if err := event.Validate(); err != nil {
		return false, err
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	evicted := false
	if r.count == r.capacity {
		// Evict oldest element
		r.tail = (r.tail + 1) % r.capacity
		r.evicted++
		evicted = true
	} else {
		r.count++
	}

	r.buffer[r.head] = event
	r.head = (r.head + 1) % r.capacity
	return evicted, nil
}

// PushBatch adds multiple events to the ring buffer in order. Returns the count of evicted items.
func (r *RingBuffer) PushBatch(events []IntentEvent) (int, error) {
	for _, event := range events {
		if err := event.Validate(); err != nil {
			return 0, err
		}
	}

	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()

	evictedCount := 0
	for _, event := range events {
		if event.Timestamp.IsZero() {
			event.Timestamp = now
		}
		if r.count == r.capacity {
			r.tail = (r.tail + 1) % r.capacity
			r.evicted++
			evictedCount++
		} else {
			r.count++
		}
		r.buffer[r.head] = event
		r.head = (r.head + 1) % r.capacity
	}
	return evictedCount, nil
}

// Pop removes and returns the oldest event. Returns false if empty.
func (r *RingBuffer) Pop() (IntentEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return IntentEvent{}, false
	}
	event := r.buffer[r.tail]
	r.buffer[r.tail] = IntentEvent{} // zero out for GC
	r.tail = (r.tail + 1) % r.capacity
	r.count--
	if r.count == 0 {
		r.head = 0
		r.tail = 0
	}
	return event, true
}

// Peek returns the oldest event without removing it. Returns false if empty.
func (r *RingBuffer) Peek() (IntentEvent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.count == 0 {
		return IntentEvent{}, false
	}
	return r.buffer[r.tail], true
}

func (r *RingBuffer) resetIndicesLocked() {
	r.head = 0
	r.tail = 0
	r.count = 0
}

// Drain removes and returns all stored events in FIFO order.
func (r *RingBuffer) Drain() []IntentEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return nil
	}
	res := make([]IntentEvent, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.tail + i) % r.capacity
		res[i] = r.buffer[idx]
		r.buffer[idx] = IntentEvent{}
	}
	r.resetIndicesLocked()
	return res
}

// DrainN removes and returns up to n events in FIFO order.
func (r *RingBuffer) DrainN(n int) []IntentEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 || n <= 0 {
		return nil
	}
	toDrain := n
	if toDrain > r.count {
		toDrain = r.count
	}
	res := make([]IntentEvent, toDrain)
	for i := 0; i < toDrain; i++ {
		idx := (r.tail + i) % r.capacity
		res[i] = r.buffer[idx]
		r.buffer[idx] = IntentEvent{}
	}
	r.tail = (r.tail + toDrain) % r.capacity
	r.count -= toDrain
	if r.count == 0 {
		r.head = 0
		r.tail = 0
	}
	return res
}

// Size returns the number of currently stored events.
func (r *RingBuffer) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

// Capacity returns the maximum buffer capacity.
func (r *RingBuffer) Capacity() int {
	return r.capacity
}

// EvictedCount returns total number of evicted events since creation.
func (r *RingBuffer) EvictedCount() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.evicted
}

// IsEmpty returns true if no events are buffered.
func (r *RingBuffer) IsEmpty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count == 0
}

// IsFull returns true if buffer has reached capacity.
func (r *RingBuffer) IsFull() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count == r.capacity
}

// Clear flushes all events without returning them.
func (r *RingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.buffer {
		r.buffer[i] = IntentEvent{}
	}
	r.resetIndicesLocked()
}

// AmbientIntentStreamConfig holds configuration options for AmbientIntentStream.
type AmbientIntentStreamConfig struct {
	Capacity          int
	ActiveAgents      int
	WarningThreshold  float64
	CriticalThreshold float64
	AlertBufferSize   int
}

// DefaultAmbientIntentStreamConfig returns standard default configuration.
func DefaultAmbientIntentStreamConfig() AmbientIntentStreamConfig {
	return AmbientIntentStreamConfig{
		Capacity:          DefaultRingBufferCapacity,
		ActiveAgents:      DefaultActiveAgents,
		WarningThreshold:  DefaultWarningRunwayDepth,
		CriticalThreshold: DefaultCriticalRunwayDepth,
		AlertBufferSize:   DefaultAlertChannelBufferSize,
	}
}

// AmbientIntentStream manages real-time ingestion of agent intent events into an ambient ring buffer,
// continuously evaluates swarm runway depth against replenishment watermarks, and broadcasts alerts.
// Implements REQ-1793597212444417000-c9f17c58 (Ambient Intent Stream Aggregator & Continuous Swarm Runway Depth Arbiter).
type AmbientIntentStream struct {
	mu                sync.RWMutex
	ring              *RingBuffer
	activeAgents      int
	warningThreshold  float64
	criticalThreshold float64

	subMu        sync.RWMutex
	subscribers  map[chan SwarmRunwayAlert]struct{}
	alertBufSize int
}

// IntentStreamAggregator is a type alias for AmbientIntentStream.
type IntentStreamAggregator = AmbientIntentStream

// IntentStreamConfig is a type alias for AmbientIntentStreamConfig.
type IntentStreamConfig = AmbientIntentStreamConfig

// NewAmbientIntentStream creates an AmbientIntentStream instance with optional configuration overrides.
func NewAmbientIntentStream(cfgs ...AmbientIntentStreamConfig) *AmbientIntentStream {
	cfg := DefaultAmbientIntentStreamConfig()
	if len(cfgs) > 0 {
		userCfg := cfgs[0]
		if userCfg.Capacity > 0 {
			cfg.Capacity = userCfg.Capacity
		}
		if userCfg.ActiveAgents > 0 {
			cfg.ActiveAgents = userCfg.ActiveAgents
		}
		if userCfg.WarningThreshold > 0 {
			cfg.WarningThreshold = userCfg.WarningThreshold
		}
		if userCfg.CriticalThreshold > 0 {
			cfg.CriticalThreshold = userCfg.CriticalThreshold
		}
		if userCfg.AlertBufferSize > 0 {
			cfg.AlertBufferSize = userCfg.AlertBufferSize
		}
	}
	return &AmbientIntentStream{
		ring:              NewRingBuffer(cfg.Capacity),
		activeAgents:      cfg.ActiveAgents,
		warningThreshold:  cfg.WarningThreshold,
		criticalThreshold: cfg.CriticalThreshold,
		subscribers:       make(map[chan SwarmRunwayAlert]struct{}),
		alertBufSize:      cfg.AlertBufferSize,
	}
}

// NewIntentStreamAggregator creates an IntentStreamAggregator instance.
func NewIntentStreamAggregator(cfgs ...AmbientIntentStreamConfig) *AmbientIntentStream {
	return NewAmbientIntentStream(cfgs...)
}

// Push adds an event to the ambient ring buffer.
func (s *AmbientIntentStream) Push(event IntentEvent) error {
	_, err := s.ring.Push(event)
	return err
}

// PushBatch adds a slice of events into the ambient ring buffer.
func (s *AmbientIntentStream) PushBatch(events []IntentEvent) error {
	_, err := s.ring.PushBatch(events)
	return err
}

// Drain flushes and returns all pending events in FIFO order.
func (s *AmbientIntentStream) Drain() []IntentEvent {
	return s.ring.Drain()
}

// DrainN flushes and returns up to n pending events in FIFO order.
func (s *AmbientIntentStream) DrainN(n int) []IntentEvent {
	return s.ring.DrainN(n)
}

// Size returns the count of buffered intent events.
func (s *AmbientIntentStream) Size() int {
	return s.ring.Size()
}

// EvictedCount returns the count of evicted events due to buffer saturation.
func (s *AmbientIntentStream) EvictedCount() int64 {
	return s.ring.EvictedCount()
}

// SetActiveAgents updates the active agent count used for runway depth calculation.
func (s *AmbientIntentStream) SetActiveAgents(agents int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if agents < 1 {
		agents = 1
	}
	s.activeAgents = agents
}

// ActiveAgents returns the current active agent count.
func (s *AmbientIntentStream) ActiveAgents() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeAgents
}

// EvaluateRunwayDepth calculates depth = (pendingIntents / activeAgents),
// and determines the WatermarkThreshold (critical <= 1.0, warning <= 2.0, nominal > 2.0).
func (s *AmbientIntentStream) EvaluateRunwayDepth() (float64, WatermarkThreshold) {
	s.mu.RLock()
	agents := s.activeAgents
	crit := s.criticalThreshold
	warn := s.warningThreshold
	s.mu.RUnlock()

	if agents <= 0 {
		agents = 1
	}

	pending := s.ring.Size()
	depth := float64(pending) / float64(agents)

	var threshold WatermarkThreshold
	if depth <= crit {
		threshold = WatermarkThresholdCritical
	} else if depth <= warn {
		threshold = WatermarkThresholdWarning
	} else {
		threshold = WatermarkThresholdNominal
	}

	return depth, threshold
}

// Snapshot returns a point-in-time SwarmRunwaySnapshot.
func (s *AmbientIntentStream) Snapshot() SwarmRunwaySnapshot {
	depth, threshold := s.EvaluateRunwayDepth()
	return SwarmRunwaySnapshot{
		PendingIntents: s.ring.Size(),
		ActiveAgents:   s.ActiveAgents(),
		RunwayDepth:    depth,
		Threshold:      threshold,
		Timestamp:      time.Now().UTC(),
	}
}

// SubscribeAlerts registers a channel to receive SwarmRunwayAlert notifications.
// Returns the receive-only channel and an unsubscribe cancellation function.
func (s *AmbientIntentStream) SubscribeAlerts() (<-chan SwarmRunwayAlert, func()) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	ch := make(chan SwarmRunwayAlert, s.alertBufSize)
	s.subscribers[ch] = struct{}{}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			s.subMu.Lock()
			delete(s.subscribers, ch)
			close(ch)
			s.subMu.Unlock()
		})
	}

	return ch, unsubscribe
}

// EmitAlert broadcasts a SwarmRunwayAlert to all active subscribers without blocking.
func (s *AmbientIntentStream) EmitAlert(alert SwarmRunwayAlert) {
	s.subMu.RLock()
	defer s.subMu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- alert:
		default:
			// Non-blocking drop on slow or saturated subscriber to prevent backpressure
		}
	}
}

// CheckAndEmitAlert evaluates current runway depth and emits an alert if at warning or critical threshold.
// Returns the generated alert and true if an alert was emitted.
func (s *AmbientIntentStream) CheckAndEmitAlert() (SwarmRunwayAlert, bool) {
	depth, threshold := s.EvaluateRunwayDepth()
	if threshold == WatermarkThresholdCritical || threshold == WatermarkThresholdWarning {
		snap := s.Snapshot()
		alert := SwarmRunwayAlert{
			Threshold:   threshold,
			RunwayDepth: depth,
			Message:     fmt.Sprintf("swarm runway depth %.2f fell below watermark threshold: %s", depth, threshold),
			Snapshot:    snap,
			Timestamp:   time.Now().UTC(),
		}
		s.EmitAlert(alert)
		return alert, true
	}
	return SwarmRunwayAlert{}, false
}

// StartRunwayMonitor launches a background periodic runway depth evaluator using goroutinelabels.
// Terminates gracefully when ctx is cancelled.
func (s *AmbientIntentStream) StartRunwayMonitor(ctx context.Context, checkInterval time.Duration) {
	if checkInterval <= 0 {
		checkInterval = DefaultMonitorCheckInterval
	}

	goroutinelabels.NewGoroutine("runway_monitor", "periodic runway depth check").StartSimple(func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		// Run immediate check
		s.CheckAndEmitAlert()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.CheckAndEmitAlert()
			}
		}
	})
}
