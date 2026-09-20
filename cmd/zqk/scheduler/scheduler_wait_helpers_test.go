package scheduler

import (
	"context"
	"time"
)

func waitForConditionScheduler(ctx context.Context, condition func() bool, pollInterval time.Duration) bool {
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

func waitForConditionWithTimeoutScheduler(ctx context.Context, condition func() bool, timeout, pollInterval time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return waitForConditionScheduler(timeoutCtx, condition, pollInterval)
}
