package callback

import (
	"context"
	"math/bits"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Default telemetry and backpressure watermark constants.
const (
	DefaultWindowDuration              = 10 * time.Second
	DefaultWindowBucketCount           = 10
	DefaultMeshTelemetrySubscriberName = "mesh_telemetry_aggregator"
	DefaultMeshControllerName          = "mesh_telemetry_backpressure_controller"
	ReasonCreditExhausted              = "credit_exhausted"
	MetricKeyTopic                     = "topic"
	MetricKeyBroker                    = "broker"
	MetricKeySubscriber                = "subscriber"
	MetricKeyStatus                    = "status"
)

// Error definitions for telemetry and backpressure boundaries.
var (
	ErrInvalidConfiguration  = errfmt.Errorf("invalid configuration parameters")
	ErrSubscriberNotFound    = errfmt.Errorf("subscriber pool not found")
	ErrInvalidCreditAmount   = errfmt.Errorf("credit amount must be greater than zero")
	ErrCreditExhausted       = errfmt.Errorf("subscriber credit budget exhausted")
	ErrDebtThresholdExceeded = errfmt.Errorf("credit request exceeds permissible debt threshold")
	ErrNilTelemetryEntry     = errfmt.Errorf("telemetry callback entry cannot be nil")
	ErrThrottlerClosed       = errfmt.Errorf("mesh backpressure throttler is closed")
)

// BackpressurePolicy specifies worker queue shedding behavior under load.
type BackpressurePolicy int

const (
	PolicyReject BackpressurePolicy = iota
	PolicyWait
	PolicyDropOldest
)

// WindowMetrics holds aggregated metrics over an active sliding window.
type WindowMetrics struct {
	TotalDispatched  int64
	TotalReceived    int64
	TotalDropped     int64
	ThroughputPerSec float64
	P50              time.Duration
	P95              time.Duration
	P99              time.Duration
}

// DropRate returns the ratio of dropped events to total dispatched events.
func (m WindowMetrics) DropRate() float64 {
	if m.TotalDispatched <= 0 {
		return 0.0
	}
	return float64(m.TotalDropped) / float64(m.TotalDispatched)
}

type windowBucket struct {
	startTime      time.Time
	dispatched     int64
	received       int64
	dropped        int64
	latencyBuckets [LatencyBucketCount]int64
}

// SlidingWindowBuffer accumulates multi-dimensional metrics across fixed sliding sub-buckets.
type SlidingWindowBuffer struct {
	mu             sync.RWMutex
	windowDuration time.Duration
	bucketCount    int
	bucketDuration time.Duration
	clock          func() time.Time
	buckets        []windowBucket
}

// NewSlidingWindowBuffer constructs a sliding window buffer validating input parameters.
func NewSlidingWindowBuffer(window time.Duration, bucketCount int, clock func() time.Time) (*SlidingWindowBuffer, error) {
	if window <= 0 || bucketCount <= 0 {
		return nil, ErrInvalidConfiguration
	}
	if clock == nil {
		clock = time.Now
	}
	bDuration := window / time.Duration(bucketCount)
	if bDuration <= 0 {
		bDuration = time.Millisecond
	}
	return &SlidingWindowBuffer{
		windowDuration: window,
		bucketCount:    bucketCount,
		bucketDuration: bDuration,
		clock:          clock,
		buckets:        make([]windowBucket, bucketCount),
	}, nil
}

// WindowDuration returns the configured sliding window length.
func (b *SlidingWindowBuffer) WindowDuration() time.Duration {
	return b.windowDuration
}

// BucketCount returns the number of discretized sub-buckets.
func (b *SlidingWindowBuffer) BucketCount() int {
	return b.bucketCount
}

func (b *SlidingWindowBuffer) getOrInitBucket(t time.Time) *windowBucket {
	bNanos := b.bucketDuration.Nanoseconds()
	if bNanos <= 0 {
		return &b.buckets[0]
	}
	total := int64(b.bucketCount)
	slot := (t.UnixNano() / bNanos) % total
	if slot < 0 {
		slot += total
	}

	bucketStart := time.Unix(0, (t.UnixNano()/bNanos)*bNanos)
	bucket := &b.buckets[slot]
	if !bucket.startTime.Equal(bucketStart) {
		*bucket = windowBucket{
			startTime: bucketStart,
		}
	}
	return bucket
}

func durationToLatencyBucketIndex(d time.Duration) int {
	ns := uint64(d.Nanoseconds())
	idx := bits.Len64(ns)
	if idx >= LatencyBucketCount {
		return LatencyBucketCount - 1
	}
	return idx
}

func recordLatencyToBucket(buckets *[LatencyBucketCount]int64, d time.Duration) {
	if d <= 0 {
		buckets[0]++
		return
	}
	buckets[durationToLatencyBucketIndex(d)]++
}

// Record records events into the active sub-bucket matching time t.
func (b *SlidingWindowBuffer) Record(t time.Time, dispatched, received, dropped int64, latency time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	bucket := b.getOrInitBucket(t)
	bucket.dispatched += dispatched
	bucket.received += received
	bucket.dropped += dropped
	if latency > 0 {
		recordLatencyToBucket(&bucket.latencyBuckets, latency)
	}
}

// Aggregate computes window metrics across all sub-buckets within the sliding horizon.
func (b *SlidingWindowBuffer) Aggregate(t time.Time) WindowMetrics {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var (
		totalDispatched int64
		totalReceived   int64
		totalDropped    int64
		totalSamples    int64
		mergedBuckets   [LatencyBucketCount]int64
	)

	for i := range b.buckets {
		bucket := &b.buckets[i]
		if bucket.startTime.IsZero() {
			continue
		}
		age := t.Sub(bucket.startTime)
		if age >= 0 && age < b.windowDuration {
			totalDispatched += bucket.dispatched
			totalReceived += bucket.received
			totalDropped += bucket.dropped
			for k := 0; k < LatencyBucketCount; k++ {
				c := bucket.latencyBuckets[k]
				mergedBuckets[k] += c
				totalSamples += c
			}
		}
	}

	var throughput float64
	if b.windowDuration.Seconds() > 0 {
		throughput = float64(totalDispatched) / b.windowDuration.Seconds()
	}

	return WindowMetrics{
		TotalDispatched:  totalDispatched,
		TotalReceived:    totalReceived,
		TotalDropped:     totalDropped,
		ThroughputPerSec: throughput,
		P50:              calculatePercentile(mergedBuckets, totalSamples, 0.50),
		P95:              calculatePercentile(mergedBuckets, totalSamples, 0.95),
		P99:              calculatePercentile(mergedBuckets, totalSamples, 0.99),
	}
}

// MetricDimension stores aggregated telemetry for a single topic, broker, or subscriber.
type MetricDimension struct {
	TotalDispatched int64
	TotalReceived   int64
	TotalDropped    int64
}

// MeshTelemetrySnapshot represents an immutable point-in-time telemetry snapshot.
type MeshTelemetrySnapshot struct {
	Name                   string
	TotalDispatched        int64
	TotalReceived          int64
	TotalDropped           int64
	TotalErrors            int64
	ActiveInFlight         int64
	CreditDebt             int64
	BufferUtilizationRatio float64
	P50                    time.Duration
	P95                    time.Duration
	P99                    time.Duration
	Topics                 map[string]*MetricDimension
	Brokers                map[string]*MetricDimension
	Subscribers            map[string]*MetricDimension
}

// MeshTelemetryConfig specifies configuration options for MeshTelemetryAggregator.
type MeshTelemetryConfig struct {
	Name           string
	Clock          func() time.Time
	WindowDuration time.Duration
	BucketCount    int
}

// MeshTelemetryAggregator collects event mesh metrics and generates dimensional snapshots.
type MeshTelemetryAggregator struct {
	name              string
	clock             func() time.Time
	windowBuffer      *SlidingWindowBuffer
	mu                sync.RWMutex
	totalDispatched   int64
	totalReceived     int64
	totalDropped      int64
	totalErrors       int64
	activeInFlight    int64
	creditDebt        int64
	bufferUtilization float64
	topics            map[string]*MetricDimension
	brokers           map[string]*MetricDimension
	subscribers       map[string]*MetricDimension
}

// NewMeshTelemetryAggregator allocates an initialized telemetry aggregator.
func NewMeshTelemetryAggregator(cfg MeshTelemetryConfig) (*MeshTelemetryAggregator, error) {
	name := cfg.Name
	if name == "" {
		name = DefaultMeshTelemetrySubscriberName
	}
	clk := cfg.Clock
	if clk == nil {
		clk = time.Now
	}
	win := cfg.WindowDuration
	if win <= 0 {
		win = DefaultWindowDuration
	}
	bc := cfg.BucketCount
	if bc <= 0 {
		bc = DefaultWindowBucketCount
	}

	swb, err := NewSlidingWindowBuffer(win, bc, clk)
	if err != nil {
		return nil, err
	}

	return &MeshTelemetryAggregator{
		name:         name,
		clock:        clk,
		windowBuffer: swb,
		topics:       make(map[string]*MetricDimension),
		brokers:      make(map[string]*MetricDimension),
		subscribers:  make(map[string]*MetricDimension),
	}, nil
}

// Name returns the aggregator subscriber identifier.
func (a *MeshTelemetryAggregator) Name() string {
	return a.name
}

// WindowBuffer returns the internal sliding window buffer.
func (a *MeshTelemetryAggregator) WindowBuffer() *SlidingWindowBuffer {
	return a.windowBuffer
}

func getOrInitDimension(m map[string]*MetricDimension, key string) *MetricDimension {
	dim, ok := m[key]
	if !ok {
		dim = &MetricDimension{}
		m[key] = dim
	}
	return dim
}

func (a *MeshTelemetryAggregator) updateDimensions(topic, broker, subscriber string, update func(*MetricDimension)) {
	if topic != "" {
		update(getOrInitDimension(a.topics, topic))
	}
	if broker != "" {
		update(getOrInitDimension(a.brokers, broker))
	}
	if subscriber != "" {
		update(getOrInitDimension(a.subscribers, subscriber))
	}
}

// RecordDispatch tracks an event dispatch across topic, broker, and subscriber dimensions.
func (a *MeshTelemetryAggregator) RecordDispatch(topic, broker, subscriber string, latency time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.totalDispatched++
	a.windowBuffer.Record(a.clock(), 1, 0, 0, latency)
	a.updateDimensions(topic, broker, subscriber, func(d *MetricDimension) { d.TotalDispatched++ })
}

// RecordReceive tracks an event receipt across topic, broker, and subscriber dimensions.
func (a *MeshTelemetryAggregator) RecordReceive(topic, broker, subscriber string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.totalReceived++
	a.windowBuffer.Record(a.clock(), 0, 1, 0, 0)
	a.updateDimensions(topic, broker, subscriber, func(d *MetricDimension) { d.TotalReceived++ })
}

// RecordDrop tracks a dropped event and increments dimensional drop counters.
func (a *MeshTelemetryAggregator) RecordDrop(topic, subscriber, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.totalDropped++
	a.windowBuffer.Record(a.clock(), 0, 0, 1, 0)
	a.updateDimensions(topic, "", subscriber, func(d *MetricDimension) { d.TotalDropped++ })
}

// RecordCreditDebt sets the observed credit debt for a subscriber pool.
func (a *MeshTelemetryAggregator) RecordCreditDebt(subscriber string, debt int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.creditDebt = debt
}

// RecordBufferUtilization updates buffer fill and capacity telemetry.
func (a *MeshTelemetryAggregator) RecordBufferUtilization(current, capacity int64) {
	if capacity <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bufferUtilization = float64(current) / float64(capacity)
}

func cloneDimensions(src map[string]*MetricDimension) map[string]*MetricDimension {
	dst := make(map[string]*MetricDimension, len(src))
	for k, v := range src {
		dst[k] = &MetricDimension{
			TotalDispatched: v.TotalDispatched,
			TotalReceived:   v.TotalReceived,
			TotalDropped:    v.TotalDropped,
		}
	}
	return dst
}

// Snapshot returns an immutable point-in-time view of aggregated telemetry metrics.
func (a *MeshTelemetryAggregator) Snapshot() MeshTelemetrySnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()

	now := a.clock()
	m := a.windowBuffer.Aggregate(now)

	return MeshTelemetrySnapshot{
		Name:                   a.name,
		TotalDispatched:        a.totalDispatched,
		TotalReceived:          a.totalReceived,
		TotalDropped:           a.totalDropped,
		TotalErrors:            a.totalErrors,
		ActiveInFlight:         a.activeInFlight,
		CreditDebt:             a.creditDebt,
		BufferUtilizationRatio: a.bufferUtilization,
		P50:                    m.P50,
		P95:                    m.P95,
		P99:                    m.P99,
		Topics:                 cloneDimensions(a.topics),
		Brokers:                cloneDimensions(a.brokers),
		Subscribers:            cloneDimensions(a.subscribers),
	}
}

func payloadString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

// Notify processes an incoming callback entry, recording dispatch and receive telemetry.
func (a *MeshTelemetryAggregator) Notify(ctx context.Context, entry *CallbackEntry) error {
	if entry == nil {
		return ErrNilTelemetryEntry
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	topic := payloadString(entry.Payload, MetricKeyTopic)
	broker := payloadString(entry.Payload, MetricKeyBroker)
	subscriber := payloadString(entry.Payload, MetricKeySubscriber)

	var elapsed time.Duration
	if !entry.Timestamp.IsZero() {
		elapsed = time.Since(entry.Timestamp)
	}

	a.RecordDispatch(topic, broker, subscriber, elapsed)
	a.RecordReceive(topic, broker, subscriber)
	return nil
}

// SubscriberPoolConfig configures an individual worker pool's credit limits and shedding policy.
type SubscriberPoolConfig struct {
	PoolID         string
	Policy         BackpressurePolicy
	InitialCredits int64
	MaxCredits     int64
	DebtThreshold  int64
	RefillRate     float64
	MinRateLimit   float64
	MaxRateLimit   float64
}

// SubscriberPool represents a registered consumer worker pool with isolated credit accounting.
type SubscriberPool struct {
	mu            sync.Mutex
	poolID        string
	policy        BackpressurePolicy
	credits       int64
	maxCredits    int64
	inFlight      int64
	droppedCount  int64
	debtThreshold int64
	refillRate    float64
	minRateLimit  float64
	maxRateLimit  float64
	rateLimit     float64
	notifyCh      chan struct{}
}

// Credits returns the currently available credits for the pool.
func (p *SubscriberPool) Credits() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.credits
}

// InFlight returns the count of active in-flight executions for the pool.
func (p *SubscriberPool) InFlight() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inFlight
}

// DroppedCount returns the count of dropped messages for the pool.
func (p *SubscriberPool) DroppedCount() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.droppedCount
}

// RateLimit returns the current dynamic rate limit for the pool.
func (p *SubscriberPool) RateLimit() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rateLimit
}

func (p *SubscriberPool) addCreditsLocked(amount int64) {
	p.credits += amount
	if p.maxCredits > 0 && p.credits > p.maxCredits {
		p.credits = p.maxCredits
	}
}

func (p *SubscriberPool) broadcastLocked() {
	close(p.notifyCh)
	p.notifyCh = make(chan struct{})
}

func (p *SubscriberPool) replenishAndBroadcast(amount int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.addCreditsLocked(amount)
	p.broadcastLocked()
}

func (p *SubscriberPool) acknowledgeAndReplenish(credits int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.inFlight > 0 {
		p.inFlight--
	}
	if credits > 0 {
		p.addCreditsLocked(credits)
	}
	p.broadcastLocked()
}

// ThrottlerConfig configures backpressure thresholds, default shedding policy, and AIMD parameters.
type ThrottlerConfig struct {
	DefaultPolicy          BackpressurePolicy
	Aggregator             *MeshTelemetryAggregator
	HighWatermark          float64
	LowWatermark           float64
	AdditiveIncrease       float64
	MultiplicativeDecrease float64
}

// MeshBackpressureThrottler coordinates multi-tenant credit-based backpressure and AIMD rate limiting.
type MeshBackpressureThrottler struct {
	mu                     sync.RWMutex
	defaultPolicy          BackpressurePolicy
	aggregator             *MeshTelemetryAggregator
	highWatermark          float64
	lowWatermark           float64
	additiveIncrease       float64
	multiplicativeDecrease float64
	pools                  map[string]*SubscriberPool
	closed                 bool
	closeCh                chan struct{}
}

// NewMeshBackpressureThrottler creates a backpressure throttler with configured watermark thresholds.
func NewMeshBackpressureThrottler(cfg ThrottlerConfig) (*MeshBackpressureThrottler, error) {
	return &MeshBackpressureThrottler{
		defaultPolicy:          cfg.DefaultPolicy,
		aggregator:             cfg.Aggregator,
		highWatermark:          cfg.HighWatermark,
		lowWatermark:           cfg.LowWatermark,
		additiveIncrease:       cfg.AdditiveIncrease,
		multiplicativeDecrease: cfg.MultiplicativeDecrease,
		pools:                  make(map[string]*SubscriberPool),
		closeCh:                make(chan struct{}),
	}, nil
}

// RegisterPool adds a new subscriber pool to the throttler.
func (t *MeshBackpressureThrottler) RegisterPool(cfg SubscriberPoolConfig) (*SubscriberPool, error) {
	if cfg.PoolID == "" {
		return nil, ErrInvalidConfiguration
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, ErrThrottlerClosed
	}

	rate := cfg.RefillRate
	if rate <= 0 && cfg.MinRateLimit > 0 {
		rate = cfg.MinRateLimit
	}

	pool := &SubscriberPool{
		poolID:        cfg.PoolID,
		policy:        cfg.Policy,
		credits:       cfg.InitialCredits,
		maxCredits:    cfg.MaxCredits,
		debtThreshold: cfg.DebtThreshold,
		refillRate:    cfg.RefillRate,
		minRateLimit:  cfg.MinRateLimit,
		maxRateLimit:  cfg.MaxRateLimit,
		rateLimit:     rate,
		notifyCh:      make(chan struct{}),
	}
	t.pools[cfg.PoolID] = pool
	return pool, nil
}

// GetPool looks up a registered subscriber pool by ID.
func (t *MeshBackpressureThrottler) GetPool(poolID string) (*SubscriberPool, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	pool, ok := t.pools[poolID]
	if !ok {
		return nil, ErrSubscriberNotFound
	}
	return pool, nil
}

func (t *MeshBackpressureThrottler) resolvePoolForCredits(poolID string, credits int64) (*SubscriberPool, error) {
	if credits <= 0 {
		return nil, ErrInvalidCreditAmount
	}
	return t.GetPool(poolID)
}

// Acquire requests credits for a subscriber pool, blocking or shedding according to policy.
func (t *MeshBackpressureThrottler) Acquire(ctx context.Context, poolID string, credits int64) error {
	pool, err := t.resolvePoolForCredits(poolID, credits)
	if err != nil {
		return err
	}

	policy := pool.policy
	if policy == 0 && t.defaultPolicy != 0 {
		policy = t.defaultPolicy
	}

	for {
		t.mu.RLock()
		closed := t.closed
		t.mu.RUnlock()
		if closed {
			return ErrThrottlerClosed
		}

		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}

		pool.mu.Lock()
		if pool.credits >= credits {
			pool.credits -= credits
			pool.inFlight++
			pool.mu.Unlock()
			return nil
		}

		switch policy {
		case PolicyReject:
			debt := credits - pool.credits
			pool.mu.Unlock()
			if pool.debtThreshold > 0 && debt > pool.debtThreshold {
				return ErrDebtThresholdExceeded
			}
			return ErrCreditExhausted

		case PolicyDropOldest:
			pool.droppedCount++
			pool.mu.Unlock()
			if t.aggregator != nil {
				t.aggregator.RecordDrop("", poolID, ReasonCreditExhausted)
			}
			return nil

		case PolicyWait:
			ch := pool.notifyCh
			pool.mu.Unlock()

			select {
			case <-t.closeCh:
				return ErrThrottlerClosed
			case <-ctx.Done():
				return ctx.Err()
			case <-ch:
			}
		default:
			pool.mu.Unlock()
			return ErrCreditExhausted
		}
	}
}

// Replenish adds credits to a subscriber pool and notifies blocked waiters.
func (t *MeshBackpressureThrottler) Replenish(poolID string, credits int64) error {
	pool, err := t.resolvePoolForCredits(poolID, credits)
	if err != nil {
		return err
	}

	pool.replenishAndBroadcast(credits)
	return nil
}

// Acknowledge marks completed work, decrements in-flight, and replenishes credits.
func (t *MeshBackpressureThrottler) Acknowledge(poolID string, credits int64, latency time.Duration) error {
	pool, err := t.GetPool(poolID)
	if err != nil {
		return err
	}

	pool.acknowledgeAndReplenish(credits)
	return nil
}

// RecordDrain records buffer drain telemetry and applies AIMD rate adjustments.
func (t *MeshBackpressureThrottler) RecordDrain(poolID string, drainedCount int64, currentQueueSize, queueCapacity int64) error {
	if queueCapacity <= 0 || currentQueueSize < 0 {
		return ErrInvalidConfiguration
	}

	pool, err := t.GetPool(poolID)
	if err != nil {
		return err
	}

	utilization := float64(currentQueueSize) / float64(queueCapacity)

	t.mu.RLock()
	high := t.highWatermark
	low := t.lowWatermark
	add := t.additiveIncrease
	mult := t.multiplicativeDecrease
	agg := t.aggregator
	t.mu.RUnlock()

	if agg != nil {
		agg.RecordBufferUtilization(currentQueueSize, queueCapacity)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if high > 0 && mult > 0 && utilization >= high {
		pool.rateLimit = clampRateLimit(pool.rateLimit*mult, pool.minRateLimit, pool.maxRateLimit)
	} else if low > 0 && add > 0 && utilization <= low {
		pool.rateLimit = clampRateLimit(pool.rateLimit+add, pool.minRateLimit, pool.maxRateLimit)
	}

	return nil
}

func clampRateLimit(rate, minRate, maxRate float64) float64 {
	if minRate > 0 && rate < minRate {
		return minRate
	}
	if maxRate > 0 && rate > maxRate {
		return maxRate
	}
	return rate
}

// Notify routes a callback entry through the backpressure throttler.
func (t *MeshBackpressureThrottler) Notify(ctx context.Context, entry *CallbackEntry) error {
	if entry == nil {
		return ErrNilTelemetryEntry
	}
	if t.aggregator != nil {
		return t.aggregator.Notify(ctx, entry)
	}
	return nil
}

// Close gracefully terminates the throttler and unblocks all waiting acquires.
func (t *MeshBackpressureThrottler) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.closeCh)

	pools := make([]*SubscriberPool, 0, len(t.pools))
	for _, p := range t.pools {
		pools = append(pools, p)
	}
	t.mu.Unlock()

	for _, p := range pools {
		p.mu.Lock()
		close(p.notifyCh)
		p.notifyCh = make(chan struct{})
		p.mu.Unlock()
	}
	return nil
}

// MeshTelemetryBackpressureController unifies telemetry collection and backpressure throttling.
type MeshTelemetryBackpressureController struct {
	name       string
	aggregator *MeshTelemetryAggregator
	throttler  *MeshBackpressureThrottler
}

// NewMeshTelemetryBackpressureController constructs a unified controller.
func NewMeshTelemetryBackpressureController(
	agg *MeshTelemetryAggregator,
	throttler *MeshBackpressureThrottler,
) *MeshTelemetryBackpressureController {
	return &MeshTelemetryBackpressureController{
		name:       DefaultMeshControllerName,
		aggregator: agg,
		throttler:  throttler,
	}
}

// Name returns the controller subscriber identifier.
func (c *MeshTelemetryBackpressureController) Name() string {
	return c.name
}

// Aggregator returns the underlying telemetry aggregator.
func (c *MeshTelemetryBackpressureController) Aggregator() *MeshTelemetryAggregator {
	return c.aggregator
}

// Throttler returns the underlying backpressure throttler.
func (c *MeshTelemetryBackpressureController) Throttler() *MeshBackpressureThrottler {
	return c.throttler
}

// Notify forwards entries to the telemetry aggregator.
func (c *MeshTelemetryBackpressureController) Notify(ctx context.Context, entry *CallbackEntry) error {
	if c.aggregator != nil {
		return c.aggregator.Notify(ctx, entry)
	}
	return nil
}

// Close closes the underlying throttler.
func (c *MeshTelemetryBackpressureController) Close() error {
	if c.throttler != nil {
		return c.throttler.Close()
	}
	return nil
}

// Aliases satisfying CRIT-1 and CRIT-2 naming conventions.
type EventMeshTelemetryAggregator = MeshTelemetryAggregator
type CreditBasedBackpressureThrottler = MeshBackpressureThrottler
type BackpressureSubscriber = MeshTelemetryBackpressureController

var (
	_ CallbackSubscriber = (*MeshTelemetryAggregator)(nil)
	_ CallbackSubscriber = (*MeshTelemetryBackpressureController)(nil)
	_ Subscriber         = (*MeshTelemetryAggregator)(nil)
	_ Subscriber         = (*MeshTelemetryBackpressureController)(nil)
)
