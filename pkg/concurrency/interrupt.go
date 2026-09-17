package concurrency

import (
	"context"
	"time"
)

// DefaultInterruptCheckFrequency is the standard interval for evaluating interrupt checks in batch processes.
const DefaultInterruptCheckFrequency = 100 * time.Millisecond

// InterruptChecker provides a consistent, configurable way to evaluate interrupt signals
// (context cancellation) during long-running batch processes or tight loops without checking
// the context channel every single iteration (which can add overhead in extremely tight loops).
type InterruptChecker struct {
	frequency time.Duration
	lastCheck time.Time
}

// NewInterruptChecker creates a new checker that evaluates context cancellation at most
// once per the configured frequency. If frequency <= 0, it evaluates on every call.
func NewInterruptChecker(frequency time.Duration) *InterruptChecker {
	return &InterruptChecker{
		frequency: frequency,
		lastCheck: time.Now(),
	}
}

// Check evaluates whether the context is cancelled. If the configured frequency has not
// elapsed since the last check, it returns nil immediately (fast path).
func (c *InterruptChecker) Check(ctx context.Context) error {
	if c.frequency > 0 {
		now := time.Now()
		if now.Sub(c.lastCheck) < c.frequency {
			return nil // Fast path: skip check until frequency elapses
		}
		c.lastCheck = now
	}

	if ctx.Done() == nil {
		return nil // Fast path: context can never be cancelled (e.g. Background)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
