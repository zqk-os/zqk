package callback

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// BackpressureStrategy defines the shedding behavior when subscriber capacity is exceeded.
type BackpressureStrategy int

const (
	// BackpressureDropSilent drops incoming entries when capacity is saturated without returning an error.
	BackpressureDropSilent BackpressureStrategy = iota
	// BackpressureDropError drops incoming entries and returns an explicit backpressure error.
	BackpressureDropError
	// BackpressureBlock waits up to a specified duration before shedding.
	BackpressureBlock
)

// ResilientSubscriberConfig configures adaptive backpressure, concurrency throttling, and circuit breaking.
type ResilientSubscriberConfig struct {
	MaxConcurrent      int
	ExecutionTimeout   time.Duration
	BlockTimeout       time.Duration
	BackpressurePolicy BackpressureStrategy
	Breaker            circuitbreaker.CircuitBreaker
	Logger             logging.Logger
}

// DefaultResilientSubscriberConfig returns safe production defaults.
func DefaultResilientSubscriberConfig() ResilientSubscriberConfig {
	return ResilientSubscriberConfig{
		MaxConcurrent:      8,
		ExecutionTimeout:   3 * time.Second,
		BlockTimeout:       500 * time.Millisecond,
		BackpressurePolicy: BackpressureDropError,
		Breaker:            circuitbreaker.NewCircuitBreaker(),
	}
}

// ResilientSubscriber wraps any CallbackSubscriber with fail-closed circuit breaking,
// bounded concurrency semaphores, and zero-idle adaptive backpressure shedding.
type ResilientSubscriber struct {
	inner   Subscriber
	cfg     ResilientSubscriberConfig
	sem     chan struct{}
	breaker circuitbreaker.CircuitBreaker

	// Telemetry metrics
	inFlight atomic.Int64
	dropped  atomic.Int64
	total    atomic.Int64
	failures atomic.Int64

	mu sync.RWMutex
}

// NewResilientSubscriber constructs a protected subscriber wrapper.
func NewResilientSubscriber(inner Subscriber, cfg ResilientSubscriberConfig) *ResilientSubscriber {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 8
	}
	if cfg.ExecutionTimeout <= 0 {
		cfg.ExecutionTimeout = 3 * time.Second
	}
	if cfg.Breaker == nil {
		cfg.Breaker = circuitbreaker.NewCircuitBreaker()
	}

	return &ResilientSubscriber{
		inner:   inner,
		cfg:     cfg,
		sem:     make(chan struct{}, cfg.MaxConcurrent),
		breaker: cfg.Breaker,
	}
}

// Name returns the underlying subscriber's name prefixed with resiliency identity.
func (r *ResilientSubscriber) Name() string {
	if r.inner == nil {
		return "resilient_nil"
	}
	return r.inner.Name()
}

// Inner returns the underlying wrapped subscriber.
func (r *ResilientSubscriber) Inner() Subscriber {
	return r.inner
}

// Metrics returns current in-flight, dropped, total, and failure counts.
func (r *ResilientSubscriber) Metrics() (inFlight, dropped, total, failures int64) {
	return r.inFlight.Load(), r.dropped.Load(), r.total.Load(), r.failures.Load()
}

// Notify routes the callback entry through the circuit breaker, concurrency limiter, and timeout boundary.
func (r *ResilientSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	r.total.Add(1)

	if r.inner == nil {
		return nil
	}

	// 1. Fast-Path Circuit Breaker Check
	if err := r.breaker.AllowRequest(); err != nil {
		r.dropped.Add(1)
		if r.cfg.Logger != nil {
			r.cfg.Logger.Warn("circuit breaker open for subscriber",
				logging.String("subscriber", r.Name()),
				logging.Error(err),
			)
		}
		return errfmt.Newf("circuit breaker open for subscriber %s", r.Name()).Wrap(err)
	}

	// 2. Concurrency Throttling & Adaptive Backpressure
	acquired := false
	switch r.cfg.BackpressurePolicy {
	case BackpressureDropSilent, BackpressureDropError:
		select {
		case r.sem <- struct{}{}:
			acquired = true
		default:
			r.dropped.Add(1)
			if r.cfg.BackpressurePolicy == BackpressureDropSilent {
				return nil
			}
			return errfmt.Errorf("backpressure: subscriber %s concurrency limit reached (%d)", r.Name(), r.cfg.MaxConcurrent)
		}
	case BackpressureBlock:
		blockTimeout := r.cfg.BlockTimeout
		if blockTimeout <= 0 {
			blockTimeout = 500 * time.Millisecond
		}
		select {
		case r.sem <- struct{}{}:
			acquired = true
		case <-time.After(blockTimeout):
			r.dropped.Add(1)
			return errfmt.Errorf("backpressure: subscriber %s timed out waiting for concurrency slot", r.Name())
		case <-ctx.Done():
			r.dropped.Add(1)
			return ctx.Err()
		}
	}

	if !acquired {
		return errfmt.Errorf("backpressure: failed to acquire concurrency token")
	}

	r.inFlight.Add(1)
	defer func() {
		<-r.sem
		r.inFlight.Add(-1)
	}()

	// 3. Execution Timeout Boundary
	execCtx := ctx
	var cancel context.CancelFunc
	if r.cfg.ExecutionTimeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, r.cfg.ExecutionTimeout)
		defer cancel()
	}

	// 4. Invariant Guarded Dispatch
	err := r.inner.Notify(execCtx, entry)
	if err != nil {
		r.failures.Add(1)
		r.breaker.RecordFailure()
		return err
	}

	r.breaker.RecordSuccess()
	return nil
}
