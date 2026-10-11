package callback

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

const (
	// DefaultMinBackoffDelay is the default floor backoff duration.
	DefaultMinBackoffDelay = 50 * time.Millisecond
	// DefaultMaxBackoffDelay is the default ceiling backoff duration.
	DefaultMaxBackoffDelay = 5 * time.Second
	// DefaultMaxRetries is the default maximum retry attempt count.
	DefaultMaxRetries = 5
	// DefaultBackoffMultiplier is the default decorrelated jitter multiplication factor.
	DefaultBackoffMultiplier = 3.0

	// DefaultQuorumRateLimit is the default allowed events per second.
	DefaultQuorumRateLimit = 50.0
	// DefaultQuorumBurstCapacity is the default token bucket burst capacity.
	DefaultQuorumBurstCapacity = 100
	// DefaultQuorumWindowSize is the default sliding window rate evaluation duration.
	DefaultQuorumWindowSize = 1 * time.Second
	// DefaultQuorumMaxTopics is the default upper bound for distinct tracked topics.
	DefaultQuorumMaxTopics = 1000
	// DefaultQuorumTopicTTL is the default idle topic bucket eviction duration.
	DefaultQuorumTopicTTL = 10 * time.Minute

	// DefaultTopicKey is the fallback topic identifier when no explicit topic is found.
	DefaultTopicKey = "default"
)

var (
	// ErrQuorumDampingActive indicates event intake rate has exceeded the quorum damping threshold.
	ErrQuorumDampingActive = errors.New("quorum damping active: event intake rate exceeded threshold")
	// ErrMaxRetriesExceeded indicates that the maximum retry attempts were exhausted.
	ErrMaxRetriesExceeded = errors.New("maximum retry attempts exceeded")
)

var (
	_ Subscriber         = (*DampedSubscriber)(nil)
	_ CallbackSubscriber = (*DampedSubscriber)(nil)
)

// AdaptiveBackoffConfig configures the decorrelated jitter backoff calculation.
type AdaptiveBackoffConfig struct {
	MinDelay   time.Duration
	MaxDelay   time.Duration
	MaxRetries int
	Multiplier float64
}

// DefaultAdaptiveBackoffConfig returns safe production defaults for backoff calculations.
func DefaultAdaptiveBackoffConfig() AdaptiveBackoffConfig {
	return AdaptiveBackoffConfig{
		MinDelay:   DefaultMinBackoffDelay,
		MaxDelay:   DefaultMaxBackoffDelay,
		MaxRetries: DefaultMaxRetries,
		Multiplier: DefaultBackoffMultiplier,
	}
}

// AdaptiveBackoffCalculator computes bounded exponential backoff with full jitter
// using decorrelated jitter mechanics: t = min(maxDelay, rand(baseDelay, sleep * 3)).
type AdaptiveBackoffCalculator struct {
	cfg AdaptiveBackoffConfig
}

// NewAdaptiveBackoffCalculator constructs a validated, sanitized backoff calculator.
func NewAdaptiveBackoffCalculator(cfg AdaptiveBackoffConfig) *AdaptiveBackoffCalculator {
	if cfg.MinDelay <= 0 {
		cfg.MinDelay = DefaultMinBackoffDelay
	}
	if cfg.MaxDelay < cfg.MinDelay {
		cfg.MaxDelay = cfg.MinDelay
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.Multiplier < 1.0 {
		cfg.Multiplier = DefaultBackoffMultiplier
	}
	return &AdaptiveBackoffCalculator{cfg: cfg}
}

// Config returns a copy of the active backoff configuration.
func (c *AdaptiveBackoffCalculator) Config() AdaptiveBackoffConfig {
	return c.cfg
}

// MinDelay returns the configured floor delay.
func (c *AdaptiveBackoffCalculator) MinDelay() time.Duration {
	return c.cfg.MinDelay
}

// MaxDelay returns the configured ceiling delay.
func (c *AdaptiveBackoffCalculator) MaxDelay() time.Duration {
	return c.cfg.MaxDelay
}

// MaxRetries returns the configured maximum retry count.
func (c *AdaptiveBackoffCalculator) MaxRetries() int {
	return c.cfg.MaxRetries
}

// CanRetry returns whether the given attempt index is permitted to retry.
func (c *AdaptiveBackoffCalculator) CanRetry(attempt int) bool {
	if c.cfg.MaxRetries <= 0 {
		return false
	}
	return attempt < c.cfg.MaxRetries
}

// Ceiling returns the monotonic upper ceiling for a given retry attempt count.
// It is guaranteed to be non-decreasing with respect to attempt and capped at MaxDelay.
func (c *AdaptiveBackoffCalculator) Ceiling(attempt int) time.Duration {
	if attempt <= 0 {
		return c.cfg.MinDelay
	}
	if attempt > 30 {
		return c.cfg.MaxDelay
	}
	ceiling := c.cfg.MinDelay << uint(attempt)
	if ceiling < c.cfg.MinDelay || ceiling > c.cfg.MaxDelay {
		return c.cfg.MaxDelay
	}
	return ceiling
}

// Calculate computes the next sleep duration using Decorrelated Jitter:
//
//	t = min(maxDelay, rand(baseDelay, sleep * multiplier))
//
// Guaranteed invariants:
// 1. MinDelay <= t <= MaxDelay.
// 2. Safe against integer overflow on extreme inputs.
// 3. Zero heap allocations.
func (c *AdaptiveBackoffCalculator) Calculate(attempt int, prevSleep time.Duration) time.Duration {
	sleep := prevSleep
	if sleep < c.cfg.MinDelay {
		sleep = c.cfg.MinDelay
	}

	// Guard against integer overflow on multiplier multiplication
	var high time.Duration
	if c.cfg.Multiplier > 1.0 && float64(sleep) > float64(math.MaxInt64)/c.cfg.Multiplier {
		high = c.cfg.MaxDelay
	} else {
		high = time.Duration(float64(sleep) * c.cfg.Multiplier)
	}

	if high < c.cfg.MinDelay {
		high = c.cfg.MinDelay
	}
	if high > c.cfg.MaxDelay {
		high = c.cfg.MaxDelay
	}

	delta := high - c.cfg.MinDelay
	var delay time.Duration
	if delta <= 0 {
		delay = c.cfg.MinDelay
	} else {
		delay = c.cfg.MinDelay + time.Duration(rand.Int64N(int64(delta)+1))
	}

	if delay > c.cfg.MaxDelay {
		delay = c.cfg.MaxDelay
	}
	if delay < c.cfg.MinDelay {
		delay = c.cfg.MinDelay
	}
	return delay
}

// Next calculates the next backoff duration assuming attempt 0 starting from prevSleep.
func (c *AdaptiveBackoffCalculator) Next(prevSleep time.Duration) time.Duration {
	return c.Calculate(0, prevSleep)
}

// DampingAction defines the suppression policy when event intake exceeds quorum threshold.
type DampingAction int

const (
	// DampingActionReject returns ErrQuorumDampingActive when rate threshold is exceeded.
	DampingActionReject DampingAction = iota
	// DampingActionCollapse collapses/deduplicates redundant events during the damping window.
	DampingActionCollapse
	// DampingActionDrop silently sheds redundant events without returning an error.
	DampingActionDrop
)

// QuorumDampenerConfig defines the rate limiting, memory bounding, and action policy.
type QuorumDampenerConfig struct {
	RateLimit      float64
	BurstCapacity  int
	WindowSize     time.Duration
	MaxTopics      int
	TopicTTL       time.Duration
	Action         DampingAction
	TopicExtractor func(entry *CallbackEntry) string
	Clock          func() time.Time
}

// DefaultQuorumDampenerConfig returns production defaults for quorum event storm damping.
func DefaultQuorumDampenerConfig() QuorumDampenerConfig {
	return QuorumDampenerConfig{
		RateLimit:      DefaultQuorumRateLimit,
		BurstCapacity:  DefaultQuorumBurstCapacity,
		WindowSize:     DefaultQuorumWindowSize,
		MaxTopics:      DefaultQuorumMaxTopics,
		TopicTTL:       DefaultQuorumTopicTTL,
		Action:         DampingActionReject,
		TopicExtractor: DefaultTopicExtractor,
		Clock:          time.Now,
	}
}

// DefaultTopicExtractor extracts a canonical topic name from a callback entry.
func DefaultTopicExtractor(entry *CallbackEntry) string {
	if entry == nil {
		return DefaultTopicKey
	}
	if entry.Payload != nil {
		for _, key := range []string{"topic", "event_type", "action", "type", "operation"} {
			if v, ok := entry.Payload[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	if entry.JobID != "" {
		return entry.JobID
	}
	return DefaultTopicKey
}

type topicBucket struct {
	capacity   float64
	tokens     float64
	refillRate float64
	lastRefill time.Time
	lastSeen   time.Time
	collapsed  int64
	total      int64
}

// QuorumDampener tracks event storm frequency using a sliding-window token bucket algorithm.
// It limits intake rate per topic, collapses redundant events, and prevents downstream saturation.
type QuorumDampener struct {
	cfg     QuorumDampenerConfig
	mu      sync.RWMutex
	buckets map[string]*topicBucket

	totalAllowed atomic.Int64
	totalDamped  atomic.Int64
	totalDropped atomic.Int64

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewQuorumDampener constructs a thread-safe, bounded memory QuorumDampener.
func NewQuorumDampener(cfg QuorumDampenerConfig) *QuorumDampener {
	if cfg.RateLimit <= 0 {
		cfg.RateLimit = DefaultQuorumRateLimit
	}
	if cfg.BurstCapacity <= 0 {
		cfg.BurstCapacity = DefaultQuorumBurstCapacity
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = DefaultQuorumWindowSize
	}
	if cfg.MaxTopics <= 0 {
		cfg.MaxTopics = DefaultQuorumMaxTopics
	}
	if cfg.TopicTTL <= 0 {
		cfg.TopicTTL = DefaultQuorumTopicTTL
	}
	if cfg.TopicExtractor == nil {
		cfg.TopicExtractor = DefaultTopicExtractor
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}

	return &QuorumDampener{
		cfg:     cfg,
		buckets: make(map[string]*topicBucket),
	}
}

// Config returns a copy of the active QuorumDampenerConfig.
func (q *QuorumDampener) Config() QuorumDampenerConfig {
	return q.cfg
}

// Allow evaluates whether an incoming event for the specified topic is permitted.
// If tokens are available, deducts one token and returns (true, nil).
// If rate threshold is exceeded, executes the configured DampingAction:
// - DampingActionReject: returns (false, ErrQuorumDampingActive)
// - DampingActionCollapse: increments collapsed count and returns (false, nil)
// - DampingActionDrop: increments dropped count and returns (false, nil)
func (q *QuorumDampener) Allow(topic string) (bool, error) {
	if topic == "" {
		topic = DefaultTopicKey
	}

	now := q.cfg.Clock()

	q.mu.Lock()
	defer q.mu.Unlock()

	bucket, exists := q.buckets[topic]
	if !exists {
		// Enforce memory bounds
		if len(q.buckets) >= q.cfg.MaxTopics {
			q.evictLocked(now)
		}

		refillRate := q.cfg.RateLimit / q.cfg.WindowSize.Seconds()
		if refillRate <= 0 {
			refillRate = q.cfg.RateLimit
		}

		bucket = &topicBucket{
			capacity:   float64(q.cfg.BurstCapacity),
			tokens:     float64(q.cfg.BurstCapacity),
			refillRate: refillRate,
			lastRefill: now,
			lastSeen:   now,
		}
		q.buckets[topic] = bucket
	}

	// Refill tokens based on elapsed duration
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	if elapsed > 0 {
		bucket.tokens += elapsed * bucket.refillRate
		if bucket.tokens > bucket.capacity {
			bucket.tokens = bucket.capacity
		}
		bucket.lastRefill = now
	}
	bucket.lastSeen = now
	bucket.total++

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		q.totalAllowed.Add(1)
		return true, nil
	}

	// Threshold exceeded: storm active
	q.totalDamped.Add(1)
	switch q.cfg.Action {
	case DampingActionCollapse:
		bucket.collapsed++
		return false, nil
	case DampingActionDrop:
		q.totalDropped.Add(1)
		return false, nil
	case DampingActionReject:
		fallthrough
	default:
		return false, ErrQuorumDampingActive
	}
}

// Check evaluates whether the topic is allowed, returning ErrQuorumDampingActive if damped.
func (q *QuorumDampener) Check(topic string) error {
	allowed, err := q.Allow(topic)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrQuorumDampingActive
	}
	return nil
}

// AllowEntry evaluates an incoming CallbackEntry using the configured TopicExtractor.
func (q *QuorumDampener) AllowEntry(ctx context.Context, entry *CallbackEntry) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	topic := q.cfg.TopicExtractor(entry)
	return q.Allow(topic)
}

// TopicStats returns point-in-time statistics for a specific topic.
func (q *QuorumDampener) TopicStats(topic string) (tokens float64, collapsed int64, total int64, found bool) {
	if topic == "" {
		topic = DefaultTopicKey
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	b, ok := q.buckets[topic]
	if !ok {
		return 0, 0, 0, false
	}
	return b.tokens, b.collapsed, b.total, true
}

// Metrics returns aggregate allowed, damped, and dropped counts.
func (q *QuorumDampener) Metrics() (allowed, damped, dropped int64) {
	return q.totalAllowed.Load(), q.totalDamped.Load(), q.totalDropped.Load()
}

// ActiveTopicCount returns the count of currently tracked topic buckets.
func (q *QuorumDampener) ActiveTopicCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.buckets)
}

// Reset clears the bucket for a single topic.
func (q *QuorumDampener) Reset(topic string) {
	if topic == "" {
		topic = DefaultTopicKey
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.buckets, topic)
}

// ResetAll clears all tracked topic buckets.
func (q *QuorumDampener) ResetAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.buckets = make(map[string]*topicBucket)
}

// evictLocked evicts idle or oldest topics to keep memory strictly bounded.
func (q *QuorumDampener) evictLocked(now time.Time) {
	// First pass: evict topics older than TopicTTL
	for topic, b := range q.buckets {
		if now.Sub(b.lastSeen) > q.cfg.TopicTTL {
			delete(q.buckets, topic)
		}
	}

	// Second pass: if still at or over capacity, evict oldest lastSeen
	if len(q.buckets) >= q.cfg.MaxTopics {
		var oldestTopic string
		var oldestTime time.Time
		first := true
		for topic, b := range q.buckets {
			if first || b.lastSeen.Before(oldestTime) {
				oldestTopic = topic
				oldestTime = b.lastSeen
				first = false
			}
		}
		if !first {
			delete(q.buckets, oldestTopic)
		}
	}
}

// Start launches a periodic cleanup worker to purge expired topic buckets.
func (q *QuorumDampener) Start(ctx context.Context) {
	q.mu.Lock()
	if q.stopCh != nil {
		q.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	q.stopCh = stopCh
	q.wg.Add(1)
	q.mu.Unlock()

	goroutinelabels.NewGoroutine("quorum_damper_evict", "periodic sliding window eviction").StartSimple(func() {
		defer q.wg.Done()
		ticker := time.NewTicker(q.cfg.WindowSize)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				q.mu.Lock()
				q.evictLocked(q.cfg.Clock())
				q.mu.Unlock()
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	})
}

// Stop cleanly terminates the periodic cleanup worker and joins the goroutine.
func (q *QuorumDampener) Stop() {
	q.mu.Lock()
	if q.stopCh == nil {
		q.mu.Unlock()
		return
	}
	close(q.stopCh)
	q.stopCh = nil
	q.mu.Unlock()
	q.wg.Wait()
}

// DampedSubscriberConfig configures a DampedSubscriber.
type DampedSubscriberConfig struct {
	BackoffConfig  AdaptiveBackoffConfig
	DampenerConfig QuorumDampenerConfig
	Retryable      func(err error) bool
	Logger         logging.Logger
}

// DefaultDampedSubscriberConfig returns safe production defaults.
func DefaultDampedSubscriberConfig() DampedSubscriberConfig {
	return DampedSubscriberConfig{
		BackoffConfig:  DefaultAdaptiveBackoffConfig(),
		DampenerConfig: DefaultQuorumDampenerConfig(),
	}
}

// DampedSubscriber wraps a Subscriber with adaptive decorrelated exponential backoff
// retry logic and sliding window quorum storm damping.
type DampedSubscriber struct {
	inner     Subscriber
	backoff   *AdaptiveBackoffCalculator
	dampener  *QuorumDampener
	logger    logging.Logger
	retryable func(err error) bool

	totalProcessed atomic.Int64
	totalRetries   atomic.Int64
	totalDamped    atomic.Int64
	totalFailures  atomic.Int64
	totalSuccesses atomic.Int64
}

// NewDampedSubscriber creates a DampedSubscriber wrapping inner with the supplied configuration.
func NewDampedSubscriber(inner Subscriber, cfg DampedSubscriberConfig) *DampedSubscriber {
	return &DampedSubscriber{
		inner:     inner,
		backoff:   NewAdaptiveBackoffCalculator(cfg.BackoffConfig),
		dampener:  NewQuorumDampener(cfg.DampenerConfig),
		logger:    cfg.Logger,
		retryable: cfg.Retryable,
	}
}

// NewDampedSubscriberWithComponents constructs a DampedSubscriber using pre-configured components.
func NewDampedSubscriberWithComponents(inner Subscriber, dampener *QuorumDampener, backoff *AdaptiveBackoffCalculator) *DampedSubscriber {
	if backoff == nil {
		backoff = NewAdaptiveBackoffCalculator(DefaultAdaptiveBackoffConfig())
	}
	if dampener == nil {
		dampener = NewQuorumDampener(DefaultQuorumDampenerConfig())
	}
	return &DampedSubscriber{
		inner:    inner,
		backoff:  backoff,
		dampener: dampener,
	}
}

// Name returns the inner subscriber name prefixed with damping identity.
func (d *DampedSubscriber) Name() string {
	if d.inner == nil {
		return "damped_nil"
	}
	return d.inner.Name()
}

// Inner returns the wrapped subscriber.
func (d *DampedSubscriber) Inner() Subscriber {
	return d.inner
}

// Dampener returns the underlying QuorumDampener.
func (d *DampedSubscriber) Dampener() *QuorumDampener {
	return d.dampener
}

// BackoffCalculator returns the underlying AdaptiveBackoffCalculator.
func (d *DampedSubscriber) BackoffCalculator() *AdaptiveBackoffCalculator {
	return d.backoff
}

// Metrics returns processed, retries, damped, failures, and successes counters.
func (d *DampedSubscriber) Metrics() (processed, retries, damped, failures, successes int64) {
	return d.totalProcessed.Load(),
		d.totalRetries.Load(),
		d.totalDamped.Load(),
		d.totalFailures.Load(),
		d.totalSuccesses.Load()
}

// Notify processes an event through quorum damping and automatic backoff retries.
func (d *DampedSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	d.totalProcessed.Add(1)

	if err := ctx.Err(); err != nil {
		d.totalFailures.Add(1)
		return err
	}

	if d.inner == nil {
		return nil
	}

	// 1. Quorum Damping Gate
	if d.dampener != nil {
		topic := d.dampener.cfg.TopicExtractor(entry)
		allowed, err := d.dampener.Allow(topic)
		if err != nil {
			d.totalDamped.Add(1)
			if d.logger != nil {
				d.logger.Warn("quorum damping rejected event",
					logging.String("subscriber", d.Name()),
					logging.String("topic", topic),
					logging.Error(err),
				)
			}
			return err
		}
		if !allowed {
			// Dropped or collapsed
			d.totalDamped.Add(1)
			return nil
		}
	}

	// 2. Retry Loop with Adaptive Decorrelated Backoff
	maxRetries := d.backoff.MaxRetries()
	var lastErr error
	var prevSleep time.Duration

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			sleepDur := d.backoff.Calculate(attempt, prevSleep)
			prevSleep = sleepDur
			d.totalRetries.Add(1)

			select {
			case <-time.After(sleepDur):
			case <-ctx.Done():
				d.totalFailures.Add(1)
				return ctx.Err()
			}
		}

		lastErr = d.inner.Notify(ctx, entry)
		if lastErr == nil {
			d.totalSuccesses.Add(1)
			return nil
		}

		if ctx.Err() != nil {
			d.totalFailures.Add(1)
			return ctx.Err()
		}

		if d.retryable != nil && !d.retryable(lastErr) {
			d.totalFailures.Add(1)
			return lastErr
		}
	}

	d.totalFailures.Add(1)
	return errfmt.Newf("subscriber %s exceeded maximum retries (%d)", d.Name(), maxRetries).Wrap(lastErr)
}
