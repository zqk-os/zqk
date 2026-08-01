package circuitbreaker

import (
	"context"
)

// CircuitBreaker defines the interface for circuit breaker patterns.
type CircuitBreaker interface {
	AllowRequest() error
	RecordSuccess()
	RecordFailure()
}

// RateLimiter defines the interface for rate limiting.
type RateLimiter interface {
	Allow(key string) bool
}

// TokenLimiter defines the interface for token-based rate limiting.
type TokenLimiter interface {
	Wait(ctx context.Context, requestedTokens int) error
}

// ConcurrencyLimiter defines the interface for limiting concurrency.
type ConcurrencyLimiter interface {
	Acquire(ctx context.Context, key string) error
	Release(key string)
	SetLimit(key string, limit int)
	MergeLimits(limits map[string]int)
	Snapshot(key string) (limit int, inUse int)
	ShouldLimit(key string) bool
}
