package loader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const (
	waitForChannelPollInterval = 10 * time.Millisecond
	errWaitChannelTimeoutFmt   = "timeout waiting for load channel (max %v)"
	errWaitLoadTimeoutFmt      = "timeout waiting for load to complete (max %v)"
)

// Runner implements the retryable component loader pattern: atomics (loaded, loading),
// completion channel with configurable timeout, optional StateChangeCallback, and
// lastErr so waiters see load failure after channel closes.
type Runner struct {
	name     string
	loadFn   LoadFn
	callback StateChangeCallback
	cfg      LoaderTimeoutConfig

	loaded   atomic.Bool
	loading  atomic.Bool
	loadDone atomic.Value          // chan struct{}, closed when load completes (never store nil)
	lastErr  atomic.Pointer[error] // non-nil only when load failed
}

// RunnerOption configures a Runner.
type RunnerOption func(*Runner)

// WithCallback sets the state-change callback (default: NoOpStateChangeCallback).
func WithCallback(cb StateChangeCallback) RunnerOption {
	return func(r *Runner) {
		if cb != nil {
			r.callback = cb
		}
	}
}

// WithTimeoutConfig overrides timeout config (default: from GetLoaderTimeoutConfig(name)).
func WithTimeoutConfig(cfg LoaderTimeoutConfig) RunnerOption {
	return func(r *Runner) {
		r.cfg = cfg
	}
}

// NewRunner creates a Runner that runs loadFn with the shared-state + channel + timeout pattern.
// name is used for GetLoaderTimeoutConfig(name) unless WithTimeoutConfig is passed.
func NewRunner(name string, loadFn LoadFn, opts ...RunnerOption) *Runner {
	r := &Runner{
		name:     name,
		loadFn:   loadFn,
		callback: &NoOpStateChangeCallback{},
		cfg:      GetLoaderTimeoutConfig(name),
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Load ensures the component is loaded: fast path if already loaded, wait on channel with
// timeout if another goroutine is loading, or run loadFn (one winner via CAS) and signal completion.
func (r *Runner) Load(ctx context.Context) error {
	if r.loaded.Load() {
		return nil
	}
	if r.loading.Load() {
		return r.waitForCompletion(ctx)
	}
	if !r.loading.CompareAndSwap(false, true) {
		return r.Load(ctx)
	}
	return r.runLoad(ctx)
}

func (r *Runner) waitForCompletion(ctx context.Context) error {
	cfg := r.cfg
	var ch chan struct{}
	deadline := time.Now().Add(cfg.PublishChannel)
	for time.Now().Before(deadline) {
		if r.loaded.Load() {
			return nil
		}
		if x := r.loadDone.Load(); x != nil {
			ch = x.(chan struct{})
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitForChannelPollInterval):
		}
	}
	if ch == nil {
		if r.loaded.Load() {
			return nil
		}
		return errfmt.Errorf(errWaitChannelTimeoutFmt, cfg.PublishChannel)
	}
	select {
	case <-ch:
		if r.loaded.Load() {
			return nil
		}
		if e := r.lastErr.Load(); e != nil && *e != nil {
			return *e
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(cfg.WaitForCompletion):
		r.callback.OnTimeout()
		return errfmt.Errorf(errWaitLoadTimeoutFmt, cfg.WaitForCompletion)
	}
}

func (r *Runner) runLoad(ctx context.Context) error {
	r.callback.OnLoading()
	ch := make(chan struct{})
	r.loadDone.Store(ch)
	defer func() {
		r.loading.Store(false)
		close(ch)
		// Do not store nil in loadDone; atomic.Value cannot hold nil. Next Load will overwrite.
	}()

	loadCtx := ctx
	if r.cfg.LoadOperation > 0 {
		var cancel context.CancelFunc
		loadCtx, cancel = context.WithTimeout(ctx, r.cfg.LoadOperation)
		defer cancel()
	}
	err := r.loadFn(loadCtx)
	if err != nil {
		r.lastErr.Store(&err)
		r.callback.OnError(err)
		return err
	}
	r.loaded.Store(true)
	r.lastErr.Store(nil)
	r.callback.OnLoaded(nil)
	return nil
}

// TimeoutConfig returns the timeout configuration for this runner.
func (r *Runner) TimeoutConfig() LoaderTimeoutConfig {
	return r.cfg
}

// Loaded returns true if the component has been loaded successfully (lock-free).
func (r *Runner) Loaded() bool {
	return r.loaded.Load()
}

// ResetLoaded clears the loaded state so the next Load() will run loadFn again (e.g. for ReloadPatterns).
func (r *Runner) ResetLoaded() {
	r.loaded.Store(false)
	r.lastErr.Store(nil) // atomic.Pointer[error] allows nil
}
