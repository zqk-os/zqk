package community

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrQuotaExceeded      = errors.New("provider concurrency quota exceeded")
	ErrRateLimitExceeded  = errors.New("provider execution rate limit exceeded")
	ErrInvalidQuotaConfig = errors.New("invalid resource governor configuration")
)

// GovernorOptions specifies concurrency and throughput constraints.
type GovernorOptions struct {
	MaxConcurrency     int
	RateLimitPerSecond int
}

// DefaultGovernorOptions returns standard limits for community provider execution.
func DefaultGovernorOptions() GovernorOptions {
	return GovernorOptions{
		MaxConcurrency:     10,
		RateLimitPerSecond: 100,
	}
}

// Validate verifies structural constraints on governor options.
func (o *GovernorOptions) Validate() error {
	if o.MaxConcurrency <= 0 {
		return fmt.Errorf("%w: MaxConcurrency must be > 0", ErrInvalidQuotaConfig)
	}
	if o.RateLimitPerSecond <= 0 {
		return fmt.Errorf("%w: RateLimitPerSecond must be > 0", ErrInvalidQuotaConfig)
	}
	return nil
}

// ProviderResourceGovernor controls active execution quotas and enforces rate limits.
type ProviderResourceGovernor struct {
	opts        GovernorOptions
	sem         chan struct{}
	activeCount atomic.Int64

	rateMu     sync.Mutex
	tokens     int
	lastRefill time.Time
}

// NewProviderResourceGovernor initializes a resource governor.
func NewProviderResourceGovernor(opts ...GovernorOptions) (*ProviderResourceGovernor, error) {
	o := DefaultGovernorOptions()
	if len(opts) > 0 {
		o = opts[0]
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}

	return &ProviderResourceGovernor{
		opts:       o,
		sem:        make(chan struct{}, o.MaxConcurrency),
		tokens:     o.RateLimitPerSecond,
		lastRefill: time.Now(),
	}, nil
}

// Acquire requests a concurrency slot and verifies rate limiting tokens.
func (g *ProviderResourceGovernor) Acquire(ctx context.Context) (release func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Try non-blocking slot acquisition or fail-fast if quota exceeded
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case g.sem <- struct{}{}:
	default:
		return nil, ErrQuotaExceeded
	}

	// Check rate limit after reserving slot; rollback if exceeded
	if !g.allowRateToken() {
		<-g.sem
		return nil, ErrRateLimitExceeded
	}

	g.activeCount.Add(1)
	var once sync.Once
	releaseFunc := func() {
		once.Do(func() {
			<-g.sem
			g.activeCount.Add(-1)
		})
	}
	return releaseFunc, nil
}

func (g *ProviderResourceGovernor) allowRateToken() bool {
	g.rateMu.Lock()
	defer g.rateMu.Unlock()

	now := time.Now()
	elapsed := now.Sub(g.lastRefill)
	if elapsed >= time.Second {
		g.tokens = g.opts.RateLimitPerSecond
		g.lastRefill = now
	}

	if g.tokens <= 0 {
		return false
	}
	g.tokens--
	return true
}

// ActiveCount returns current in-flight executions.
func (g *ProviderResourceGovernor) ActiveCount() int64 {
	return g.activeCount.Load()
}
