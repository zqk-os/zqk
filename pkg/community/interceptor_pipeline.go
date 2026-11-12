package community

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

var (
	ErrNilHandler       = errors.New("cannot execute nil handler")
	ErrExecutionTimeout = errors.New("provider execution exceeded timeout duration")
)

// ProviderHandler represents the terminal or core provider capability invocation.
type ProviderHandler func(ctx context.Context, req any) (any, error)

// ProviderInterceptor intercepts a provider invocation, providing pre- and post-processing.
type ProviderInterceptor func(ctx context.Context, req any, next ProviderHandler) (any, error)

// InterceptorPipeline manages an ordered stack of provider middleware interceptors.
type InterceptorPipeline struct {
	mu           sync.RWMutex
	interceptors []ProviderInterceptor
}

// NewInterceptorPipeline creates an empty interceptor pipeline.
func NewInterceptorPipeline() *InterceptorPipeline {
	return &InterceptorPipeline{
		interceptors: make([]ProviderInterceptor, 0, 8),
	}
}

// Use appends one or more interceptors to the end of the pipeline.
func (p *InterceptorPipeline) Use(interceptors ...ProviderInterceptor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.interceptors = append(p.interceptors, interceptors...)
}

// Wrap wraps a base handler with all registered interceptors in onion order (first registered executes outer pre-processing).
func (p *InterceptorPipeline) Wrap(handler ProviderHandler) ProviderHandler {
	if handler == nil {
		return func(ctx context.Context, req any) (any, error) {
			return nil, ErrNilHandler
		}
	}

	p.mu.RLock()
	chain := make([]ProviderInterceptor, len(p.interceptors))
	copy(chain, p.interceptors)
	p.mu.RUnlock()

	cur := handler
	for i := len(chain) - 1; i >= 0; i-- {
		interceptor := chain[i]
		next := cur
		cur = func(ctx context.Context, req any) (any, error) {
			return interceptor(ctx, req, next)
		}
	}
	return cur
}

// Execute wraps and immediately executes the handler with the given request.
func (p *InterceptorPipeline) Execute(ctx context.Context, req any, handler ProviderHandler) (any, error) {
	wrapped := p.Wrap(handler)
	return wrapped(ctx, req)
}

// NewLoggingInterceptor creates a middleware that records execution duration and outcomes.
func NewLoggingInterceptor(logFn func(msg string)) ProviderInterceptor {
	return func(ctx context.Context, req any, next ProviderHandler) (any, error) {
		start := time.Now()
		res, err := next(ctx, req)
		dur := time.Since(start)
		if logFn != nil {
			if err != nil {
				logFn(fmt.Sprintf("[interceptor] failure after %v: %v", dur, err))
			} else {
				logFn(fmt.Sprintf("[interceptor] success in %v", dur))
			}
		}
		return res, err
	}
}

// NewTimeoutInterceptor creates a middleware enforcing a strict execution timeout.
func NewTimeoutInterceptor(timeout time.Duration) ProviderInterceptor {
	return func(ctx context.Context, req any, next ProviderHandler) (any, error) {
		ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		type result struct {
			val any
			err error
		}

		resCh := make(chan result, 1)

		goroutinelabels.NewGoroutine("community.interceptor.timeout", "async handler execution").StartSimple(func() {
			val, err := next(ctxWithTimeout, req)
			resCh <- result{val: val, err: err}
		})

		select {
		case <-ctxWithTimeout.Done():
			if errors.Is(ctxWithTimeout.Err(), context.DeadlineExceeded) {
				return nil, ErrExecutionTimeout
			}
			return nil, ctxWithTimeout.Err()
		case r := <-resCh:
			return r.val, r.err
		}
	}
}
