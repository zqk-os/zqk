package overseer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrMalformedWakePayload indicates a wake payload failed structural or invariant validation.
	ErrMalformedWakePayload = errors.New("malformed wake payload: missing required fields or invalid structure")

	// ErrWakeQuarantined indicates a wake payload was diverted to dead-letter quarantine.
	ErrWakeQuarantined = errors.New("wake signal quarantined by circuit breaker")

	// ErrCircuitBreakerTripped indicates the wake circuit breaker is open due to excessive failures.
	ErrCircuitBreakerTripped = errors.New("wake circuit breaker tripped: excessive wake failures")
)

// WakePayload defines the typed structure of a reactive callback wake signal.
type WakePayload struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	EventKind string         `json:"event_kind"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Validate checks static structural invariants on the wake payload.
func (p *WakePayload) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("%w: empty wake id", ErrMalformedWakePayload)
	}
	if p.Source == "" {
		return fmt.Errorf("%w: empty wake source", ErrMalformedWakePayload)
	}
	if p.EventKind == "" {
		return fmt.Errorf("%w: empty wake event kind", ErrMalformedWakePayload)
	}
	if p.Timestamp.IsZero() {
		return fmt.Errorf("%w: zero wake timestamp", ErrMalformedWakePayload)
	}
	if time.Since(p.Timestamp) > time.Hour {
		return fmt.Errorf("%w: stale wake timestamp exceeds max tolerance", ErrMalformedWakePayload)
	}
	return nil
}

// WakeCircuitBreaker isolates malformed or failing wakes to prevent supervisor stalls.
type WakeCircuitBreaker struct {
	mu           sync.Mutex
	failureCount int
	maxFailures  int
	quarantine   []WakePayload
	tripped      bool
	trippedUntil time.Time
	cooldown     time.Duration
}

// NewWakeCircuitBreaker creates a circuit breaker with failure thresholds.
func NewWakeCircuitBreaker(maxFailures int, cooldown time.Duration) *WakeCircuitBreaker {
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}
	return &WakeCircuitBreaker{
		maxFailures: maxFailures,
		cooldown:    cooldown,
		quarantine:  make([]WakePayload, 0),
	}
}

// RecordSuccess resets failure counts if circuit breaker is healthy.
func (cb *WakeCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if !cb.tripped {
		cb.failureCount = 0
	}
}

// RecordFailure increments failures and trips breaker if threshold exceeded.
func (cb *WakeCircuitBreaker) RecordFailure(payload WakePayload) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount++
	cb.quarantine = append(cb.quarantine, payload)
	if cb.failureCount >= cb.maxFailures {
		cb.tripped = true
		cb.trippedUntil = time.Now().Add(cb.cooldown)
		return true
	}
	return false
}

// IsTripped reports whether the circuit breaker is currently open.
func (cb *WakeCircuitBreaker) IsTripped() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.tripped {
		if time.Now().After(cb.trippedUntil) {
			cb.tripped = false
			cb.failureCount = 0
			return false
		}
		return true
	}
	return false
}

// QuarantineSnapshot returns all currently quarantined wake payloads.
func (cb *WakeCircuitBreaker) QuarantineSnapshot() []WakePayload {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	out := make([]WakePayload, len(cb.quarantine))
	copy(out, cb.quarantine)
	return out
}

// WakeHandler processes an incoming validated wake payload.
type WakeHandler func(ctx context.Context, payload WakePayload) error

// WakeDispatcher receives reactive callback events and dispatches them to listeners.
type WakeDispatcher struct {
	mu       sync.RWMutex
	handlers map[string][]WakeHandler
	breaker  *WakeCircuitBreaker
}

// NewWakeDispatcher constructs an initialized wake dispatcher with circuit breaking.
func NewWakeDispatcher(breaker *WakeCircuitBreaker) *WakeDispatcher {
	if breaker == nil {
		breaker = NewWakeCircuitBreaker(5, 10*time.Second)
	}
	return &WakeDispatcher{
		handlers: make(map[string][]WakeHandler),
		breaker:  breaker,
	}
}

// Subscribe registers a wake handler for a specific event kind.
func (d *WakeDispatcher) Subscribe(eventKind string, handler WakeHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[eventKind] = append(d.handlers[eventKind], handler)
}

// Dispatch validates, checks circuit state, and invokes registered handlers.
func (d *WakeDispatcher) Dispatch(ctx context.Context, payload WakePayload) error {
	if err := payload.Validate(); err != nil {
		d.breaker.RecordFailure(payload)
		return fmt.Errorf("%w: %v", ErrMalformedWakePayload, err)
	}

	if d.breaker.IsTripped() {
		d.breaker.RecordFailure(payload)
		return ErrCircuitBreakerTripped
	}

	d.mu.RLock()
	handlers := append([]WakeHandler(nil), d.handlers[payload.EventKind]...)
	d.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(ctx, payload); err != nil {
			d.breaker.RecordFailure(payload)
			return fmt.Errorf("handler failed: %w", err)
		}
	}

	d.breaker.RecordSuccess()
	return nil
}

// QuarantinedWakes returns all quarantined wake signals.
func (d *WakeDispatcher) QuarantinedWakes() []WakePayload {
	return d.breaker.QuarantineSnapshot()
}
