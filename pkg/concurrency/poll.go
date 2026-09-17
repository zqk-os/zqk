package concurrency

import (
	"context"
	"time"
)

// PollUntil repeatedly calls condition at interval until condition returns true or ctx is done.
// Returns true if condition succeeded before ctx expiration, false otherwise.
func PollUntil(ctx context.Context, interval time.Duration, condition func() bool) bool {
	if condition() {
		return true
	}
	if interval <= 0 {
		interval = 5 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return condition()
		case <-ticker.C:
			if condition() {
				return true
			}
		}
	}
}

// PollTimeout repeatedly calls condition at interval until condition returns true or timeout expires.
func PollTimeout(timeout time.Duration, interval time.Duration, condition func() bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return PollUntil(ctx, interval, condition)
}
