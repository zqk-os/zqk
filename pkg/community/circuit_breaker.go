package community

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitState represents the operational status of a provider circuit breaker.
type CircuitState string

const (
	StateClosed   CircuitState = "closed"
	StateOpen     CircuitState = "open"
	StateHalfOpen CircuitState = "half_open"
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is open: request rejected")
	ErrNilAction   = errors.New("cannot execute nil action")
)

// CircuitBreakerOptions defines the tuning parameters for circuit breaker behavior.
type CircuitBreakerOptions struct {
	ConsecutiveFailuresThreshold int
	OpenDuration                 time.Duration
	ProbeSuccessThreshold        int
}

// DefaultCircuitBreakerOptions returns production defaults for resilient provider execution.
func DefaultCircuitBreakerOptions() CircuitBreakerOptions {
	return CircuitBreakerOptions{
		ConsecutiveFailuresThreshold: 5,
		OpenDuration:                 30 * time.Second,
		ProbeSuccessThreshold:        2,
	}
}

// ProviderCircuitBreaker protects host execution by failing fast when a provider is degraded.
type ProviderCircuitBreaker struct {
	mu                   sync.RWMutex
	opts                 CircuitBreakerOptions
	state                CircuitState
	consecutiveFailures  int
	consecutiveSuccesses int
	lastFailureTime      time.Time

	totalInvocations atomic.Int64
	totalSuccesses   atomic.Int64
	totalFailures    atomic.Int64
}

// NewProviderCircuitBreaker creates a circuit breaker initialized with custom or default options.
func NewProviderCircuitBreaker(opts ...CircuitBreakerOptions) *ProviderCircuitBreaker {
	o := DefaultCircuitBreakerOptions()
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.ConsecutiveFailuresThreshold <= 0 {
		o.ConsecutiveFailuresThreshold = 5
	}
	if o.OpenDuration <= 0 {
		o.OpenDuration = 30 * time.Second
	}
	if o.ProbeSuccessThreshold <= 0 {
		o.ProbeSuccessThreshold = 2
	}

	return &ProviderCircuitBreaker{
		opts:  o,
		state: StateClosed,
	}
}

// State returns the current circuit breaker state, automatically checking for open timeout transition.
func (cb *ProviderCircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.checkTimeoutTransitionLocked()
	return cb.state
}

func (cb *ProviderCircuitBreaker) checkTimeoutTransitionLocked() {
	if cb.state == StateOpen && time.Since(cb.lastFailureTime) >= cb.opts.OpenDuration {
		cb.state = StateHalfOpen
		cb.consecutiveSuccesses = 0
	}
}

// Execute wraps an external action with circuit protection.
func (cb *ProviderCircuitBreaker) Execute(ctx context.Context, action func(ctx context.Context) error) error {
	if action == nil {
		return ErrNilAction
	}

	cb.mu.Lock()
	cb.checkTimeoutTransitionLocked()

	if cb.state == StateOpen {
		cb.mu.Unlock()
		return ErrCircuitOpen
	}
	cb.mu.Unlock()

	cb.totalInvocations.Add(1)

	err := action(ctx)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.totalFailures.Add(1)
		cb.consecutiveFailures++
		cb.consecutiveSuccesses = 0
		cb.lastFailureTime = time.Now()

		if cb.state == StateHalfOpen || cb.consecutiveFailures >= cb.opts.ConsecutiveFailuresThreshold {
			cb.state = StateOpen
		}
		return err
	}

	cb.totalSuccesses.Add(1)
	cb.consecutiveFailures = 0

	if cb.state == StateHalfOpen {
		cb.consecutiveSuccesses++
		if cb.consecutiveSuccesses >= cb.opts.ProbeSuccessThreshold {
			cb.state = StateClosed
			cb.consecutiveSuccesses = 0
		}
	}

	return nil
}

// Reset restores the circuit breaker to closed state with clean failure counts.
func (cb *ProviderCircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.consecutiveFailures = 0
	cb.consecutiveSuccesses = 0
}

// Trip manually forces the circuit breaker into open state.
func (cb *ProviderCircuitBreaker) Trip() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateOpen
	cb.lastFailureTime = time.Now()
}

// Telemetry returns a snapshot of execution metrics.
func (cb *ProviderCircuitBreaker) Telemetry() (invocations, successes, failures int64, state CircuitState) {
	return cb.totalInvocations.Load(), cb.totalSuccesses.Load(), cb.totalFailures.Load(), cb.State()
}
