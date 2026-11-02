package ambient

import (
	"errors"
	"math"
	"sync"
	"time"
)

// Named default constants enforcing deterministic backpressure regulation.
const (
	DefaultBaselineCapacity      = 100
	DefaultMaxCapacityMultiplier = 10.0
	DefaultRefillRate            = 100.0 // tokens per second
	DefaultWindowDuration        = 1 * time.Second
	DefaultLoadSheddingThreshold = 0.95
	DefaultMinBackoff            = 10 * time.Millisecond
	DefaultMaxBackoff            = 2 * time.Second
	DefaultBackoffFactor         = 2.0
)

// AdmissionDecision represents the outcome of an admission evaluation.
type AdmissionDecision string

const (
	// AdmissionAllow indicates the request is admitted immediately.
	AdmissionAllow AdmissionDecision = "allow"
	// AdmissionWait indicates temporary exhaustion; caller should back off and retry.
	AdmissionWait AdmissionDecision = "wait"
	// AdmissionReject indicates load shedding; caller request is dropped due to critical saturation.
	AdmissionReject AdmissionDecision = "reject"
)

// Sentinel errors for backpressure and capacity operations.
var (
	ErrNilRegulator          = errors.New("backpressure regulator cannot be nil")
	ErrNilLimiter            = errors.New("token bucket limiter cannot be nil")
	ErrInvalidCapacity       = errors.New("baseline capacity must be strictly positive")
	ErrInvalidMultiplier     = errors.New("max capacity multiplier must be greater than or equal to 1.0")
	ErrInvalidRefillRate     = errors.New("refill rate must be strictly positive")
	ErrInvalidLoadRatio      = errors.New("load ratio cannot be negative")
	ErrInvalidWindowDuration = errors.New("window duration must be strictly positive")
	ErrInvalidThreshold      = errors.New("load shedding threshold must be in (0.0, 1.0]")
	ErrInvalidBackoff        = errors.New("invalid backoff duration range: min must be positive and <= max")
	ErrInvalidBackoffFactor  = errors.New("backoff factor must be >= 1.0")
	ErrInvalidCost           = errors.New("admission cost must be strictly positive")
	ErrCostExceedsCapacity   = errors.New("admission cost exceeds maximum regulator capacity")
	ErrRegulatorClosed       = errors.New("backpressure regulator is closed")
)

// ElasticCapacityConfig defines configuration parameters for elastic backpressure regulation.
type ElasticCapacityConfig struct {
	BaselineCapacity      int           `json:"baseline_capacity"`
	MaxCapacityMultiplier float64       `json:"max_capacity_multiplier"`
	RefillRate            float64       `json:"refill_rate"` // tokens per second
	WindowDuration        time.Duration `json:"window_duration"`
	LoadSheddingThreshold float64       `json:"load_shedding_threshold"`
	MinBackoff            time.Duration `json:"min_backoff"`
	MaxBackoff            time.Duration `json:"max_backoff"`
	BackoffFactor         float64       `json:"backoff_factor"`
}

// DefaultElasticCapacityConfig returns production defaults for elastic capacity.
func DefaultElasticCapacityConfig() ElasticCapacityConfig {
	return ElasticCapacityConfig{
		BaselineCapacity:      DefaultBaselineCapacity,
		MaxCapacityMultiplier: DefaultMaxCapacityMultiplier,
		RefillRate:            DefaultRefillRate,
		WindowDuration:        DefaultWindowDuration,
		LoadSheddingThreshold: DefaultLoadSheddingThreshold,
		MinBackoff:            DefaultMinBackoff,
		MaxBackoff:            DefaultMaxBackoff,
		BackoffFactor:         DefaultBackoffFactor,
	}
}

// BackpressureConfig is a type alias for ElasticCapacityConfig.
type BackpressureConfig = ElasticCapacityConfig

// DefaultBackpressureConfig returns production defaults for backpressure regulation.
func DefaultBackpressureConfig() BackpressureConfig {
	return DefaultElasticCapacityConfig()
}

// Validate checks invariant bounds on the configuration parameters.
func (c ElasticCapacityConfig) Validate() error {
	if c.BaselineCapacity <= 0 {
		return ErrInvalidCapacity
	}
	if c.MaxCapacityMultiplier < 1.0 {
		return ErrInvalidMultiplier
	}
	if c.RefillRate <= 0 {
		return ErrInvalidRefillRate
	}
	if c.WindowDuration <= 0 {
		return ErrInvalidWindowDuration
	}
	if c.LoadSheddingThreshold <= 0.0 || c.LoadSheddingThreshold > 1.0 {
		return ErrInvalidThreshold
	}
	if c.MinBackoff <= 0 || c.MaxBackoff <= 0 || c.MinBackoff > c.MaxBackoff {
		return ErrInvalidBackoff
	}
	if c.BackoffFactor < 1.0 {
		return ErrInvalidBackoffFactor
	}
	return nil
}

// AdmissionPolicy encapsulates the rules and thresholds for throttling and shedding decisions.
type AdmissionPolicy struct {
	LoadSheddingThreshold float64       `json:"load_shedding_threshold"`
	MinBackoff            time.Duration `json:"min_backoff"`
	MaxBackoff            time.Duration `json:"max_backoff"`
	BackoffFactor         float64       `json:"backoff_factor"`
}

// TokenBucketLimiter implements a leaky-bucket / token-bucket rate limiter
// with bounded accumulation floors and thread-safe dynamic capacity updates.
type TokenBucketLimiter struct {
	mu         sync.RWMutex
	capacity   float64
	tokens     float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewTokenBucketLimiter constructs a new TokenBucketLimiter initialized fully replenished.
func NewTokenBucketLimiter(capacity float64, refillRate float64) (*TokenBucketLimiter, error) {
	if capacity <= 0 {
		return nil, ErrInvalidCapacity
	}
	if refillRate <= 0 {
		return nil, ErrInvalidRefillRate
	}

	return &TokenBucketLimiter{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}, nil
}

// withRefillLock provides a synchronized scope that refills tokens before executing the mutation or read.
func (l *TokenBucketLimiter) withRefillLock(action func()) {
	l.mu.Lock()
	l.refillLocked(time.Now())
	action()
	l.mu.Unlock()
}

// TryAcquire attempts to decrement the specified cost from the bucket.
// Returns true if sufficient tokens were available, false otherwise.
func (l *TokenBucketLimiter) TryAcquire(cost int) bool {
	if l == nil || cost <= 0 {
		return false
	}

	acquired := false
	l.withRefillLock(func() {
		costFloat := float64(cost)
		if l.tokens >= costFloat {
			l.tokens -= costFloat
			if l.tokens < 0.0 {
				l.tokens = 0.0
			}
			acquired = true
		}
	})
	return acquired
}

// AvailableTokens returns the current number of available tokens after refilling.
func (l *TokenBucketLimiter) AvailableTokens() float64 {
	if l == nil {
		return 0.0
	}

	var current float64
	l.withRefillLock(func() {
		current = l.tokens
	})
	return current
}

func (l *TokenBucketLimiter) queryMetric(metricFn func() float64) float64 {
	if l == nil {
		return 0.0
	}
	l.mu.RLock()
	val := metricFn()
	l.mu.RUnlock()
	return val
}

// Capacity returns the current token bucket capacity.
func (l *TokenBucketLimiter) Capacity() float64 {
	return l.queryMetric(func() float64 { return l.capacity })
}

// RefillRate returns the token refill rate per second.
func (l *TokenBucketLimiter) RefillRate() float64 {
	return l.queryMetric(func() float64 { return l.refillRate })
}

// SetCapacity dynamically adjusts the token bucket capacity, bounding existing tokens to the new capacity.
func (l *TokenBucketLimiter) SetCapacity(newCapacity float64) error {
	if l == nil {
		return ErrNilLimiter
	}
	if newCapacity <= 0 {
		return ErrInvalidCapacity
	}

	l.withRefillLock(func() {
		l.capacity = newCapacity
		if l.tokens > l.capacity {
			l.tokens = l.capacity
		}
	})
	return nil
}

// SetRefillRate updates the token replenishment rate per second.
func (l *TokenBucketLimiter) SetRefillRate(newRate float64) error {
	if l == nil {
		return ErrNilLimiter
	}
	if newRate <= 0 {
		return ErrInvalidRefillRate
	}

	l.withRefillLock(func() {
		l.refillRate = newRate
	})
	return nil
}

func (l *TokenBucketLimiter) refillLocked(now time.Time) {
	elapsed := now.Sub(l.lastRefill).Seconds()
	if elapsed > 0 {
		added := elapsed * l.refillRate
		l.tokens = math.Min(l.capacity, l.tokens+added)
		l.lastRefill = now
	}
}

// BackpressureRegulator provides dynamic capacity scaling, throughput rate tracking,
// and adaptive admission throttling under variable workload stress.
type BackpressureRegulator struct {
	mu               sync.RWMutex
	config           ElasticCapacityConfig
	policy           AdmissionPolicy
	limiter          *TokenBucketLimiter
	currentCapacity  int
	maxCapacity      int
	arrivals         []time.Time
	consecutiveWaits int
	closed           bool
}

// NewBackpressureRegulator constructs and initializes an elastic BackpressureRegulator.
func NewBackpressureRegulator(cfg ElasticCapacityConfig) (*BackpressureRegulator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	maxCap := int(math.Floor(float64(cfg.BaselineCapacity) * cfg.MaxCapacityMultiplier))
	limiter, err := NewTokenBucketLimiter(float64(cfg.BaselineCapacity), cfg.RefillRate)
	if err != nil {
		return nil, err
	}

	return &BackpressureRegulator{
		config: cfg,
		policy: AdmissionPolicy{
			LoadSheddingThreshold: cfg.LoadSheddingThreshold,
			MinBackoff:            cfg.MinBackoff,
			MaxBackoff:            cfg.MaxBackoff,
			BackoffFactor:         cfg.BackoffFactor,
		},
		limiter:         limiter,
		currentCapacity: cfg.BaselineCapacity,
		maxCapacity:     maxCap,
		arrivals:        make([]time.Time, 0, 1024),
	}, nil
}

func (r *BackpressureRegulator) withActiveLock(fn func() error) error {
	if r == nil {
		return ErrNilRegulator
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRegulatorClosed
	}
	return fn()
}

// ScaleCapacity dynamically scales the regulator's capacity from baseline capacity (1x)
// up to MaxCapacityMultiplier based on the supplied loadRatio.
func (r *BackpressureRegulator) ScaleCapacity(loadRatio float64) (int, error) {
	if loadRatio < 0 {
		return 0, ErrInvalidLoadRatio
	}

	var newCapacity int
	err := r.withActiveLock(func() error {
		effectiveRatio := math.Min(1.0, loadRatio)
		baseline := float64(r.config.BaselineCapacity)
		maxCap := float64(r.maxCapacity)

		targetCap := baseline + effectiveRatio*(maxCap-baseline)
		newCapacity = int(math.Round(targetCap))
		if newCapacity < r.config.BaselineCapacity {
			newCapacity = r.config.BaselineCapacity
		}
		if newCapacity > r.maxCapacity {
			newCapacity = r.maxCapacity
		}

		if setErr := r.limiter.SetCapacity(float64(newCapacity)); setErr != nil {
			return setErr
		}
		r.currentCapacity = newCapacity
		return nil
	})
	if err != nil {
		return 0, err
	}
	return newCapacity, nil
}

// RecordArrival records a request arrival at the current timestamp in the rolling window.
func (r *BackpressureRegulator) RecordArrival() error {
	return r.RecordArrivalAt(time.Now())
}

// RecordArrivalAt records a request arrival at an explicit timestamp.
func (r *BackpressureRegulator) RecordArrivalAt(t time.Time) error {
	return r.withActiveLock(func() error {
		r.arrivals = append(r.arrivals, t)
		r.pruneArrivalsLocked(t)
		return nil
	})
}

// CurrentThroughput returns the request throughput (arrivals/sec) over the configured window.
func (r *BackpressureRegulator) CurrentThroughput() float64 {
	if r == nil {
		return 0.0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.pruneArrivalsLocked(now)

	windowSec := r.config.WindowDuration.Seconds()
	if windowSec <= 0 {
		return 0.0
	}

	return float64(len(r.arrivals)) / windowSec
}

func (r *BackpressureRegulator) pruneArrivalsLocked(now time.Time) {
	if len(r.arrivals) == 0 {
		return
	}
	cutoff := now.Add(-r.config.WindowDuration)
	idx := 0
	for idx < len(r.arrivals) && r.arrivals[idx].Before(cutoff) {
		idx++
	}
	if idx > 0 {
		r.arrivals = r.arrivals[idx:]
	}
}

// CurrentSaturation returns the current capacity saturation ratio in [0.0, 1.0].
func (r *BackpressureRegulator) CurrentSaturation() float64 {
	if r == nil {
		return 1.0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.currentCapacity <= 0 {
		return 1.0
	}
	avail := r.limiter.AvailableTokens()
	sat := 1.0 - (avail / float64(r.currentCapacity))
	if sat < 0.0 {
		return 0.0
	}
	if sat > 1.0 {
		return 1.0
	}
	return sat
}

// EvaluateAdmission evaluates whether an incoming request of specified cost should be
// admitted (AdmissionAllow), delayed (AdmissionWait with backoff duration), or rejected
// (AdmissionReject due to load shedding when saturation exceeds LoadSheddingThreshold).
func (r *BackpressureRegulator) EvaluateAdmission(cost int) (AdmissionDecision, time.Duration, error) {
	if r == nil {
		return AdmissionReject, 0, ErrNilRegulator
	}
	if cost <= 0 {
		return AdmissionReject, 0, ErrInvalidCost
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return AdmissionReject, 0, ErrRegulatorClosed
	}
	if cost > r.maxCapacity {
		return AdmissionReject, 0, ErrCostExceedsCapacity
	}

	avail := r.limiter.AvailableTokens()
	capFloat := float64(r.currentCapacity)
	saturation := 0.0
	if capFloat > 0 {
		saturation = 1.0 - (avail / capFloat)
	}

	if saturation >= r.config.LoadSheddingThreshold {
		return AdmissionReject, 0, nil
	}

	if !r.limiter.TryAcquire(cost) {
		waitDuration := r.calculateBackoffLocked(r.consecutiveWaits)
		r.consecutiveWaits++
		return AdmissionWait, waitDuration, nil
	}

	if r.consecutiveWaits > 0 {
		r.consecutiveWaits--
	}
	return AdmissionAllow, 0, nil
}

// CalculateBackoff calculates the recommended backoff duration for a given retry attempt.
func (r *BackpressureRegulator) CalculateBackoff(retries int) time.Duration {
	if r == nil {
		return DefaultMinBackoff
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.calculateBackoffLocked(retries)
}

func (r *BackpressureRegulator) calculateBackoffLocked(retries int) time.Duration {
	if retries < 0 {
		retries = 0
	}
	if retries > 30 {
		retries = 30
	}

	minB := r.config.MinBackoff
	maxB := r.config.MaxBackoff
	factor := r.config.BackoffFactor

	backoffFloat := float64(minB) * math.Pow(factor, float64(retries))
	if backoffFloat > float64(maxB) {
		backoffFloat = float64(maxB)
	}

	jitterFactor := float64(time.Now().UnixNano()%100) / 500.0
	total := time.Duration(backoffFloat * (1.0 + jitterFactor))
	if total > maxB {
		total = maxB
	}
	if total < minB {
		total = minB
	}
	return total
}

// CurrentCapacity returns the active capacity limit of the regulator.
func (r *BackpressureRegulator) CurrentCapacity() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentCapacity
}

// AvailableTokens returns the currently available tokens in the regulator.
func (r *BackpressureRegulator) AvailableTokens() float64 {
	if r == nil {
		return 0.0
	}
	return r.limiter.AvailableTokens()
}

// Config returns a copy of the regulator's elastic capacity configuration.
func (r *BackpressureRegulator) Config() ElasticCapacityConfig {
	if r == nil {
		return DefaultElasticCapacityConfig()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// Policy returns the active admission policy.
func (r *BackpressureRegulator) Policy() AdmissionPolicy {
	if r == nil {
		cfg := DefaultElasticCapacityConfig()
		return AdmissionPolicy{
			LoadSheddingThreshold: cfg.LoadSheddingThreshold,
			MinBackoff:            cfg.MinBackoff,
			MaxBackoff:            cfg.MaxBackoff,
			BackoffFactor:         cfg.BackoffFactor,
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.policy
}

// Close gracefully terminates the backpressure regulator.
func (r *BackpressureRegulator) Close() error {
	if r == nil {
		return ErrNilRegulator
	}
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}

// IsClosed reports whether the regulator has been closed.
func (r *BackpressureRegulator) IsClosed() bool {
	if r == nil {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}
