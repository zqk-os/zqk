package testing

import (
	"context"
	"time"
)

// WaitForCondition waits deterministically for a condition to become true.
// Uses polling with a small interval to avoid busy-waiting while remaining deterministic.
// Returns true if condition became true, false if context was cancelled/timed out.
func WaitForCondition(ctx context.Context, condition func() bool, pollInterval time.Duration) bool {
	if condition() {
		return true
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if condition() {
				return true
			}
		}
	}
}

// WaitForConditionWithTimeout waits for a condition with a timeout.
// Returns true if condition became true within the timeout, false otherwise.
// DEPRECATED: Use WaitForCondition with a context from command entry point instead.
// This function is kept for backward compatibility but should not be used in new code.
// ctx: parent context from command entry point (should not be created here)
func WaitForConditionWithTimeout(ctx context.Context, condition func() bool, timeout, pollInterval time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return WaitForCondition(timeoutCtx, condition, pollInterval)
}
