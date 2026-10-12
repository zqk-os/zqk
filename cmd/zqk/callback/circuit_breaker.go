package callback

import (
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

var (
	_ circuitbreaker.CircuitBreaker = (*AdaptiveCircuitBreaker)(nil)

	// ErrCircuitBreakerOpen is returned when requests are rejected because the circuit breaker is open.
	ErrCircuitBreakerOpen = errfmt.Errorf("circuit breaker open")

	// ErrCircuitHalfOpenProbing is returned when the breaker is testing canary recovery and rejecting excess traffic.
	ErrCircuitHalfOpenProbing = errfmt.Errorf("circuit half-open: canary probe in progress")
)

const (
	// StateClosed denotes normal operational state with zero shed traffic.
	StateClosed = "closed"
	// StateOpen denotes tripped fault isolation where calls are shed fail-fast.
	StateOpen = "open"
	// StateHalfOpen denotes recovery testing where limited canary probes are evaluated.
	StateHalfOpen = "half-open"
)

// AdaptiveCircuitBreaker implements a thread-safe, 3-state trip-and-reset circuit breaker
// with configurable failure thresholds, recovery timeouts, and half-open canary probing.
type AdaptiveCircuitBreaker struct {
	mu sync.Mutex

	state               string
	consecutiveFailures int
	failureThreshold    int
	recoveryTimeout     time.Duration
	lastFailureTime     time.Time
	halfOpenProbes      int
	maxHalfOpenProbes   int
}

// AdaptiveCircuitBreakerConfig configures trip thresholds and recovery durations.
type AdaptiveCircuitBreakerConfig struct {
	FailureThreshold  int           // Consecutive failures before tripping to Open (default: 3)
	RecoveryTimeout   time.Duration // Time to wait in Open before probing HalfOpen (default: 5s)
	MaxHalfOpenProbes int           // Maximum concurrent probes allowed in HalfOpen (default: 1)
}

// NewAdaptiveCircuitBreaker constructs a configured adaptive circuit breaker.
func NewAdaptiveCircuitBreaker(cfg AdaptiveCircuitBreakerConfig) *AdaptiveCircuitBreaker {
	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = 3
	}

	recovery := cfg.RecoveryTimeout
	if recovery <= 0 {
		recovery = 5 * time.Second
	}

	probes := cfg.MaxHalfOpenProbes
	if probes <= 0 {
		probes = 1
	}

	return &AdaptiveCircuitBreaker{
		state:             StateClosed,
		failureThreshold:  threshold,
		recoveryTimeout:   recovery,
		maxHalfOpenProbes: probes,
	}
}

// AllowRequest checks whether traffic is permitted under current circuit state.
func (cb *AdaptiveCircuitBreaker) AllowRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		if now.Sub(cb.lastFailureTime) >= cb.recoveryTimeout {
			// Transition to Half-Open for canary probe
			cb.state = StateHalfOpen
			cb.halfOpenProbes = 1
			return nil
		}
		return ErrCircuitBreakerOpen

	case StateHalfOpen:
		if cb.halfOpenProbes < cb.maxHalfOpenProbes {
			cb.halfOpenProbes++
			return nil
		}
		return ErrCircuitHalfOpenProbing

	default:
		return errfmt.Errorf("unknown circuit breaker state: %s", cb.state)
	}
}

// RecordSuccess registers a successful execution, resetting failure counters and closing the breaker.
func (cb *AdaptiveCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.consecutiveFailures = 0
	cb.halfOpenProbes = 0
}

// RecordFailure registers an execution failure, potentially tripping the breaker to Open.
func (cb *AdaptiveCircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	if cb.state == StateHalfOpen {
		// Canary probe failed; immediately trip back to Open
		cb.state = StateOpen
		cb.halfOpenProbes = 0
		return
	}

	cb.consecutiveFailures++
	if cb.consecutiveFailures >= cb.failureThreshold {
		cb.state = StateOpen
	}
}

// State returns the current lifecycle state ("closed", "open", "half-open").
func (cb *AdaptiveCircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Trip manually forces the breaker into Open state.
func (cb *AdaptiveCircuitBreaker) Trip() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateOpen
	cb.lastFailureTime = time.Now()
}

// Reset manually restores the breaker to Closed state.
func (cb *AdaptiveCircuitBreaker) Reset() {
	cb.RecordSuccess()
}

