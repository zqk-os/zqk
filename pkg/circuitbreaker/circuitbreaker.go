package circuitbreaker

import (
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// DefaultCircuitBreaker implements the CircuitBreaker interface.
type DefaultCircuitBreaker struct {
	mu              sync.Mutex
	state           string // "closed", "open", "half-open"
	lastFailure     time.Time
	failureCount    int
	halfOpenAllowed bool
}

// NewCircuitBreaker creates a new default circuit breaker.
func NewCircuitBreaker() *DefaultCircuitBreaker {
	return &DefaultCircuitBreaker{
		state: "closed",
	}
}

func (cb *DefaultCircuitBreaker) withLock(fn func(now time.Time) error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return fn(time.Now())
}

// AllowRequest checks if a request is allowed based on the circuit state.
func (cb *DefaultCircuitBreaker) AllowRequest() error {
	return cb.withLock(func(now time.Time) error {
		switch cb.state {
		case "closed":
			return nil
		case "open":
			if now.Sub(cb.lastFailure) > 5*time.Second {
				cb.state = "half-open"
				cb.halfOpenAllowed = true
				return nil
			}
			return errfmt.Errorf("circuit_open: endpoint suspected unavailable")
		case "half-open":
			if !cb.halfOpenAllowed {
				return errfmt.Errorf("circuit_half_open_probing")
			}
			cb.halfOpenAllowed = false
			return nil
		default:
			return errfmt.Errorf("unknown_circuit_state")
		}
	})
}

// RecordSuccess marks the circuit as successful, closing it.
func (cb *DefaultCircuitBreaker) RecordSuccess() {
	_ = cb.withLock(func(_ time.Time) error {
		cb.state = "closed"
		cb.failureCount = 0
		return nil
	})
}

// RecordFailure marks a failure, potentially opening the circuit.
// Decays failures older than 30s sliding window (K:F-L-RELIABILITY-003).
func (cb *DefaultCircuitBreaker) RecordFailure() {
	_ = cb.withLock(func(now time.Time) error {
		if !cb.lastFailure.IsZero() && now.Sub(cb.lastFailure) > 30*time.Second {
			cb.failureCount = 0
		}
		cb.failureCount++
		cb.lastFailure = now
		if cb.failureCount >= 3 {
			cb.state = "open"
		}
		return nil
	})
}
