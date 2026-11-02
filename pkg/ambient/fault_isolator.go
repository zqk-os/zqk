package ambient

import (
	"errors"
	"sync"
	"time"
)

// CircuitBreakerState represents the tri-state automaton state of a fault isolator.
type CircuitBreakerState string

const (
	// StateClosed allows all traffic and tracks operational error rates.
	StateClosed CircuitBreakerState = "Closed"
	// StateOpen fails-closed and rejects traffic during outage / cool-off windows.
	StateOpen CircuitBreakerState = "Open"
	// StateHalfOpen permits limited concurrent recovery probes to verify upstream health.
	StateHalfOpen CircuitBreakerState = "HalfOpen"

	// Canonical alias definitions matching criteria terminology.
	CircuitBreakerStateClosed   = StateClosed
	CircuitBreakerStateHalfOpen = StateHalfOpen
	CircuitBreakerStateOpen     = StateOpen
)

// String returns the string representation of the CircuitBreakerState.
func (s CircuitBreakerState) String() string {
	return string(s)
}

// Named default constants enforcing deterministic fault isolation.
const (
	DefaultFailureRateThreshold        = 0.50
	DefaultMinSamplesThreshold         = 10
	DefaultFaultIsolatorWindowDuration = 10 * time.Second
	DefaultCoolOffDuration             = 5 * time.Second
	DefaultHalfOpenMaxProbes           = 3
	DefaultConsecutiveSuccessesReset   = 3
	DefaultNumBuckets                  = 60
)

// Sentinel errors for fault isolator and sliding-window rate operations.
var (
	ErrNilFaultIsolator          = errors.New("fault isolator cannot be nil")
	ErrNilIsolator               = ErrNilFaultIsolator
	ErrNilRateTracker            = errors.New("sliding window rate tracker cannot be nil")
	ErrCircuitOpen               = errors.New("circuit breaker is open")
	ErrCircuitProbeLimitExceeded = errors.New("circuit breaker probe limit exceeded in half-open state")
	ErrProbeExhausted            = ErrCircuitProbeLimitExceeded
	ErrInvalidMinSamples         = errors.New("min samples threshold must be strictly positive")
	ErrInvalidCoolOffDuration    = errors.New("cool-off duration must be strictly positive")
	ErrInvalidCoolOff            = ErrInvalidCoolOffDuration
	ErrInvalidHalfOpenProbes     = errors.New("half-open max probes must be strictly positive")
	ErrInvalidConsecutiveSuccess = errors.New("consecutive successes reset must be strictly positive")
	ErrInvalidBuckets            = errors.New("number of buckets must be strictly positive")
	ErrInvalidWindow             = ErrInvalidWindowDuration
)

// FaultIsolatorConfig defines configuration parameters for adaptive circuit breaking.
type FaultIsolatorConfig struct {
	FailureRateThreshold      float64       `json:"failure_rate_threshold"`
	MinSamplesThreshold       int           `json:"min_samples_threshold"`
	WindowDuration            time.Duration `json:"window_duration"`
	CoolOffDuration           time.Duration `json:"cool_off_duration"`
	HalfOpenMaxProbes         int           `json:"half_open_max_probes"`
	ConsecutiveSuccessesReset int           `json:"consecutive_successes_reset"`
	NumBuckets                int           `json:"num_buckets"`
}

// DefaultFaultIsolatorConfig returns production defaults for fault isolation.
func DefaultFaultIsolatorConfig() FaultIsolatorConfig {
	return FaultIsolatorConfig{
		FailureRateThreshold:      DefaultFailureRateThreshold,
		MinSamplesThreshold:       DefaultMinSamplesThreshold,
		WindowDuration:            DefaultFaultIsolatorWindowDuration,
		CoolOffDuration:           DefaultCoolOffDuration,
		HalfOpenMaxProbes:         DefaultHalfOpenMaxProbes,
		ConsecutiveSuccessesReset: DefaultConsecutiveSuccessesReset,
		NumBuckets:                DefaultNumBuckets,
	}
}

// Validate checks invariant bounds on the configuration parameters.
func (c FaultIsolatorConfig) Validate() error {
	if c.FailureRateThreshold <= 0.0 || c.FailureRateThreshold > 1.0 {
		return ErrInvalidThreshold
	}
	if c.MinSamplesThreshold <= 0 {
		return ErrInvalidMinSamples
	}
	if c.WindowDuration <= 0 {
		return ErrInvalidWindowDuration
	}
	if c.CoolOffDuration <= 0 {
		return ErrInvalidCoolOffDuration
	}
	if c.HalfOpenMaxProbes <= 0 {
		return ErrInvalidHalfOpenProbes
	}
	if c.ConsecutiveSuccessesReset <= 0 {
		return ErrInvalidConsecutiveSuccess
	}
	if c.NumBuckets <= 0 {
		return ErrInvalidBuckets
	}
	return nil
}

// windowBucket stores discrete quantized successes and failures.
type windowBucket struct {
	bucketEpoch int64
	successes   int64
	failures    int64
}

// SlidingWindowRateTracker provides bounded error floors, zero-allocation rate lookups,
// and strictly bounded O(1) memory usage under continuous high-throughput load.
type SlidingWindowRateTracker struct {
	mu             sync.RWMutex
	windowDuration time.Duration
	bucketDuration time.Duration
	numBuckets     int
	buckets        []windowBucket
}

// NewSlidingWindowRateTracker initializes a circular bucket rate tracker.
func NewSlidingWindowRateTracker(windowDuration time.Duration, numBuckets int) (*SlidingWindowRateTracker, error) {
	if windowDuration <= 0 {
		return nil, ErrInvalidWindowDuration
	}
	if numBuckets <= 0 {
		return nil, ErrInvalidBuckets
	}

	bucketDuration := windowDuration / time.Duration(numBuckets)
	if bucketDuration <= 0 {
		bucketDuration = time.Millisecond
	}

	return &SlidingWindowRateTracker{
		windowDuration: windowDuration,
		bucketDuration: bucketDuration,
		numBuckets:     numBuckets,
		buckets:        make([]windowBucket, numBuckets),
	}, nil
}

// Record increments a success or failure sample at the current wall-clock time.
func (t *SlidingWindowRateTracker) Record(success bool) error {
	if t == nil {
		return ErrNilRateTracker
	}
	return t.RecordAt(success, time.Now())
}

// RecordAt increments a success or failure sample at a specific timestamp.
func (t *SlidingWindowRateTracker) RecordAt(success bool, at time.Time) error {
	if t == nil {
		return ErrNilRateTracker
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	epoch := at.UnixNano() / t.bucketDuration.Nanoseconds()
	idx := int((epoch%int64(t.numBuckets) + int64(t.numBuckets)) % int64(t.numBuckets))

	if t.buckets[idx].bucketEpoch != epoch {
		t.buckets[idx].bucketEpoch = epoch
		t.buckets[idx].successes = 0
		t.buckets[idx].failures = 0
	}

	if success {
		t.buckets[idx].successes++
	} else {
		t.buckets[idx].failures++
	}
	return nil
}

// FailureRate returns the ratio of failures to total samples within the sliding window.
func (t *SlidingWindowRateTracker) FailureRate() float64 {
	if t == nil {
		return 0.0
	}
	return t.FailureRateAt(time.Now())
}

// ErrorRate returns the ratio of failures to total samples within the sliding window (alias for FailureRate).
func (t *SlidingWindowRateTracker) ErrorRate() float64 {
	return t.FailureRate()
}

// FailureRateAt returns the ratio of failures to total samples evaluated at reference time now.
func (t *SlidingWindowRateTracker) FailureRateAt(now time.Time) float64 {
	s, f := t.CountsAt(now)
	total := s + f
	if total == 0 {
		return 0.0
	}
	return float64(f) / float64(total)
}

// TotalSamples returns the count of active samples within the sliding window.
func (t *SlidingWindowRateTracker) TotalSamples() int64 {
	if t == nil {
		return 0
	}
	return t.TotalSamplesAt(time.Now())
}

// TotalSamplesAt returns the count of active samples within the sliding window evaluated at reference time now.
func (t *SlidingWindowRateTracker) TotalSamplesAt(now time.Time) int64 {
	s, f := t.CountsAt(now)
	return s + f
}

// Counts returns active success and failure counts within the sliding window.
func (t *SlidingWindowRateTracker) Counts() (successes, failures int64) {
	if t == nil {
		return 0, 0
	}
	return t.CountsAt(time.Now())
}

// CountsAt returns active success and failure counts evaluated at reference time now.
func (t *SlidingWindowRateTracker) CountsAt(now time.Time) (successes, failures int64) {
	if t == nil {
		return 0, 0
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	currentEpoch := now.UnixNano() / t.bucketDuration.Nanoseconds()
	minEpoch := (now.UnixNano() - t.windowDuration.Nanoseconds()) / t.bucketDuration.Nanoseconds()

	for i := 0; i < t.numBuckets; i++ {
		b := t.buckets[i]
		if b.bucketEpoch >= minEpoch && b.bucketEpoch <= currentEpoch {
			successes += b.successes
			failures += b.failures
		}
	}
	return successes, failures
}

// SuccessCount returns active success counts within the sliding window.
func (t *SlidingWindowRateTracker) SuccessCount() int64 {
	s, _ := t.Counts()
	return s
}

// FailureCount returns active failure counts within the sliding window.
func (t *SlidingWindowRateTracker) FailureCount() int64 {
	_, f := t.Counts()
	return f
}

func (t *SlidingWindowRateTracker) clearBucketLocked(idx int) {
	t.buckets[idx].bucketEpoch = 0
	t.buckets[idx].successes = 0
	t.buckets[idx].failures = 0
}

// Prune evicts expired buckets older than the sliding window.
func (t *SlidingWindowRateTracker) Prune(now time.Time) error {
	if t == nil {
		return ErrNilRateTracker
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	minEpoch := (now.UnixNano() - t.windowDuration.Nanoseconds()) / t.bucketDuration.Nanoseconds()
	for i := 0; i < t.numBuckets; i++ {
		if t.buckets[i].bucketEpoch < minEpoch {
			t.clearBucketLocked(i)
		}
	}
	return nil
}

// EvictStale evicts expired buckets older than the sliding window (alias for Prune).
func (t *SlidingWindowRateTracker) EvictStale(now time.Time) error {
	return t.Prune(now)
}

// Reset clears all accumulated buckets.
func (t *SlidingWindowRateTracker) Reset() error {
	if t == nil {
		return ErrNilRateTracker
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := 0; i < t.numBuckets; i++ {
		t.clearBucketLocked(i)
	}
	return nil
}

// FaultIsolator manages adaptive circuit breaking for ambient holons.
// It coordinates state transitions across Closed, Open, and HalfOpen states,
// enforces concurrency limits on half-open probes, and tracks error rates.
type FaultIsolator struct {
	mu                   sync.RWMutex
	cfg                  FaultIsolatorConfig
	state                CircuitBreakerState
	lastStateTransition  time.Time
	tracker              *SlidingWindowRateTracker
	activeProbes         int
	consecutiveSuccesses int
}

// NewFaultIsolator constructs a new FaultIsolator with validated configuration.
func NewFaultIsolator(cfg FaultIsolatorConfig) (*FaultIsolator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	numBuckets := cfg.NumBuckets
	if numBuckets <= 0 {
		numBuckets = DefaultNumBuckets
	}
	tracker, err := NewSlidingWindowRateTracker(cfg.WindowDuration, numBuckets)
	if err != nil {
		return nil, err
	}
	return &FaultIsolator{
		cfg:                  cfg,
		state:                StateClosed,
		lastStateTransition:  time.Now(),
		tracker:              tracker,
		activeProbes:         0,
		consecutiveSuccesses: 0,
	}, nil
}

// DefaultFaultIsolator constructs a FaultIsolator with production default configuration.
func DefaultFaultIsolator() *FaultIsolator {
	iso, _ := NewFaultIsolator(DefaultFaultIsolatorConfig())
	return iso
}

func (f *FaultIsolator) checkOpenCoolOffLocked(now time.Time) {
	if f.state == StateOpen && now.Sub(f.lastStateTransition) >= f.cfg.CoolOffDuration {
		f.transitionToLocked(StateHalfOpen, now)
	}
}

// Allow evaluates whether a caller is permitted to execute an action.
// In StateClosed: permitted unconditionally (returns nil).
// In StateOpen: fails closed and returns ErrCircuitOpen unless cool-off has elapsed.
// When cool-off duration has elapsed, automatically enters StateHalfOpen and bounds
// concurrent recovery probes up to HalfOpenMaxProbes.
// In StateHalfOpen: permits exactly HalfOpenMaxProbes concurrent probes, rejecting excess
// with ErrCircuitProbeLimitExceeded.
func (f *FaultIsolator) Allow() error {
	if f == nil {
		return ErrNilFaultIsolator
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	f.checkOpenCoolOffLocked(now)

	switch f.state {
	case StateClosed:
		return nil

	case StateHalfOpen:
		if f.activeProbes < f.cfg.HalfOpenMaxProbes {
			f.activeProbes++
			return nil
		}
		return ErrCircuitProbeLimitExceeded

	case StateOpen:
		return ErrCircuitOpen

	default:
		return ErrCircuitOpen
	}
}

// RecordResult records the outcome of an execution.
// In StateClosed: evaluates sliding-window error rates against FailureRateThreshold.
// When sample count exceeds MinSamplesThreshold and failure rate exceeds threshold, trips to StateOpen.
// In StateHalfOpen: decrements active probes. Consecutive successes reset to StateClosed;
// any failure immediately trips back to StateOpen.
func (f *FaultIsolator) RecordResult(success bool) error {
	if f == nil {
		return ErrNilFaultIsolator
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	f.checkOpenCoolOffLocked(now)

	switch f.state {
	case StateClosed:
		if f.tracker != nil {
			if err := f.tracker.RecordAt(success, now); err != nil {
				return err
			}
			total := f.tracker.TotalSamplesAt(now)
			if total >= int64(f.cfg.MinSamplesThreshold) {
				rate := f.tracker.FailureRateAt(now)
				if rate > f.cfg.FailureRateThreshold {
					f.transitionToLocked(StateOpen, now)
				}
			}
		}
		return nil

	case StateHalfOpen:
		if f.activeProbes > 0 {
			f.activeProbes--
		}
		if success {
			f.consecutiveSuccesses++
			if f.consecutiveSuccesses >= f.cfg.ConsecutiveSuccessesReset {
				f.transitionToLocked(StateClosed, now)
			}
		} else {
			// Fail-closed recovery: probe failed, trip back to Open
			f.transitionToLocked(StateOpen, now)
		}
		return nil

	case StateOpen:
		// When circuit is open, reject new admissions; incoming results are safe no-ops
		return nil

	default:
		return nil
	}
}

// Record is an alias for RecordResult.
func (f *FaultIsolator) Record(success bool) error {
	return f.RecordResult(success)
}

// RecordSuccess records a successful execution result.
func (f *FaultIsolator) RecordSuccess() error {
	return f.RecordResult(true)
}

// RecordFailure records a failed execution result.
func (f *FaultIsolator) RecordFailure() error {
	return f.RecordResult(false)
}

// State returns the current CircuitBreakerState, transitioning to StateHalfOpen if cool-off elapsed.
func (f *FaultIsolator) State() CircuitBreakerState {
	if f == nil {
		return StateOpen
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	f.checkOpenCoolOffLocked(time.Now())
	return f.state
}

// Config returns a copy of the FaultIsolatorConfig.
func (f *FaultIsolator) Config() FaultIsolatorConfig {
	if f == nil {
		return FaultIsolatorConfig{}
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cfg
}

func (f *FaultIsolator) transitionWithLock(target CircuitBreakerState) error {
	if f == nil {
		return ErrNilFaultIsolator
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transitionToLocked(target, time.Now())
	return nil
}

// Reset restores the isolator to StateClosed and clears all trackers.
func (f *FaultIsolator) Reset() error {
	return f.transitionWithLock(StateClosed)
}

// Trip forces the circuit breaker into StateOpen immediately.
func (f *FaultIsolator) Trip() error {
	return f.transitionWithLock(StateOpen)
}

// ForceState overrides the current circuit breaker state for testing or administrative control.
func (f *FaultIsolator) ForceState(state CircuitBreakerState) error {
	return f.transitionWithLock(state)
}

// ActiveProbes returns the current count of in-flight half-open probes.
func (f *FaultIsolator) ActiveProbes() int {
	if f == nil {
		return 0
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.activeProbes
}

// ConsecutiveSuccesses returns the current count of consecutive half-open probe successes.
func (f *FaultIsolator) ConsecutiveSuccesses() int {
	if f == nil {
		return 0
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.consecutiveSuccesses
}

// LastStateTransition returns the timestamp of the most recent state change.
func (f *FaultIsolator) LastStateTransition() time.Time {
	if f == nil {
		return time.Time{}
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastStateTransition
}

// FailureRate returns the failure rate calculated by the underlying sliding window tracker.
func (f *FaultIsolator) FailureRate() float64 {
	if f == nil || f.tracker == nil {
		return 0.0
	}
	return f.tracker.FailureRate()
}

// TotalSamples returns the total samples recorded in the sliding window tracker.
func (f *FaultIsolator) TotalSamples() int64 {
	if f == nil || f.tracker == nil {
		return 0
	}
	return f.tracker.TotalSamples()
}

// RateTracker returns the underlying SlidingWindowRateTracker.
func (f *FaultIsolator) RateTracker() *SlidingWindowRateTracker {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.tracker
}

// transitionToLocked handles atomic state mutation within holding f.mu.
func (f *FaultIsolator) transitionToLocked(newState CircuitBreakerState, now time.Time) {
	if f.state == newState {
		return
	}
	f.state = newState
	f.lastStateTransition = now
	f.activeProbes = 0
	f.consecutiveSuccesses = 0

	if newState == StateClosed && f.tracker != nil {
		_ = f.tracker.Reset()
	}
}
