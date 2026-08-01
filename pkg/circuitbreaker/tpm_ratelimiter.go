package circuitbreaker

import (
	"context"
	"sync"
	"time"
)

// TPMRateLimiter manages token usage over time to prevent API 429 errors.
type TPMRateLimiter struct {
	mu         sync.Mutex
	tpm        int
	tokens     float64
	lastRefill time.Time
}

// NewTPMRateLimiter creates a new rate limiter that allows up to `tpm` Tokens Per Minute.
func NewTPMRateLimiter(tpm int) *TPMRateLimiter {
	return &TPMRateLimiter{
		tpm:        tpm,
		tokens:     float64(tpm),
		lastRefill: time.Now(),
	}
}

// Wait blocks until the requested number of tokens is available or the context is canceled.
func (rl *TPMRateLimiter) Wait(ctx context.Context, requestedTokens int) error {
	if requestedTokens > rl.tpm {
		// Cap to prevent deadlock if a single request exceeds TPM
		requestedTokens = rl.tpm
	}

	for {
		rl.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(rl.lastRefill)

		refillAmount := (float64(rl.tpm) / 60.0) * elapsed.Seconds()
		if refillAmount > 0 {
			rl.tokens += refillAmount
			if rl.tokens > float64(rl.tpm) {
				rl.tokens = float64(rl.tpm)
			}
			rl.lastRefill = now
		}

		if rl.tokens >= float64(requestedTokens) {
			rl.tokens -= float64(requestedTokens)
			rl.mu.Unlock()
			return nil
		}

		tokensNeeded := float64(requestedTokens) - rl.tokens
		waitSeconds := tokensNeeded / (float64(rl.tpm) / 60.0)
		waitDuration := time.Duration(waitSeconds * float64(time.Second))
		rl.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Check again after waiting
		}
	}
}
