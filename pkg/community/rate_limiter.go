package community

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrDispatcherQueueFull is returned when fair-share work queues are saturated.
	ErrDispatcherQueueFull = errors.New("fair share dispatcher queue capacity reached")
)

// TokenBucket tracks available execution tokens for a provider.
type TokenBucket struct {
	Capacity       float64
	Tokens         float64
	RefillRate     float64 // tokens per second
	LastRefillTime time.Time
}

// ProviderRateLimiter manages token buckets per provider.
type ProviderRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*TokenBucket
}

// NewProviderRateLimiter creates an initialized rate limiter.
func NewProviderRateLimiter() *ProviderRateLimiter {
	return &ProviderRateLimiter{
		buckets: make(map[string]*TokenBucket),
	}
}

// ConfigureProvider sets capacity and refill rate (tokens/sec) for a provider.
func (rl *ProviderRateLimiter) ConfigureProvider(providerID string, capacity, refillRate float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.buckets[providerID] = &TokenBucket{
		Capacity:       capacity,
		Tokens:         capacity,
		RefillRate:     refillRate,
		LastRefillTime: time.Now(),
	}
}

// Allow reports whether a single invocation is allowed under the rate limit.
func (rl *ProviderRateLimiter) Allow(providerID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, ok := rl.buckets[providerID]
	if !ok {
		return true // Unconfigured providers default to unrestricted
	}

	now := time.Now()
	elapsed := now.Sub(bucket.LastRefillTime).Seconds()
	bucket.Tokens += elapsed * bucket.RefillRate
	if bucket.Tokens > bucket.Capacity {
		bucket.Tokens = bucket.Capacity
	}
	bucket.LastRefillTime = now

	if bucket.Tokens >= 1.0 {
		bucket.Tokens -= 1.0
		return true
	}

	return false
}

// FairShareTask represents a work item scheduled on the fair-share dispatcher.
type FairShareTask struct {
	ProviderID string
	Payload    string
}

// ProviderFairShareDispatcher dispatches tasks round-robin across registered providers.
type ProviderFairShareDispatcher struct {
	mu       sync.Mutex
	queues   map[string][]FairShareTask
	order    []string
	maxQueue int
}

// NewProviderFairShareDispatcher creates an initialized fair-share dispatcher.
func NewProviderFairShareDispatcher(maxQueuePerProvider int) *ProviderFairShareDispatcher {
	return &ProviderFairShareDispatcher{
		queues:   make(map[string][]FairShareTask),
		order:    make([]string, 0),
		maxQueue: maxQueuePerProvider,
	}
}

// Enqueue adds a task to the provider's execution queue.
func (d *ProviderFairShareDispatcher) Enqueue(task FairShareTask) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	q, ok := d.queues[task.ProviderID]
	if !ok {
		d.order = append(d.order, task.ProviderID)
	}

	if len(q) >= d.maxQueue {
		return ErrDispatcherQueueFull
	}

	d.queues[task.ProviderID] = append(q, task)
	return nil
}

// DispatchNext pops the next available task using round-robin fairness across active providers.
func (d *ProviderFairShareDispatcher) DispatchNext(ctx context.Context) (*FairShareTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.order) == 0 {
		return nil, nil
	}

	// Scan through provider queues round-robin
	for i := 0; i < len(d.order); i++ {
		providerID := d.order[0]
		// Rotate order
		d.order = append(d.order[1:], providerID)

		q := d.queues[providerID]
		if len(q) > 0 {
			task := q[0]
			d.queues[providerID] = q[1:]
			return &task, nil
		}
	}

	return nil, nil
}
