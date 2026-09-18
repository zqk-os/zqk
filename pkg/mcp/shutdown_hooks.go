package mcp

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ShutdownHook represents a function that should be called during shutdown
// The context will be cancelled when shutdown is ordered
// The hook should either finish its work and return, or exit immediately if it can't finish
type ShutdownHook func(ctx context.Context) error

// ShutdownHookManager manages shutdown hooks and notifies them when shutdown is ordered
type ShutdownHookManager struct {
	mu                   sync.Mutex
	hooks                []ShutdownHook
	shutdownCtx          context.Context
	shutdownCancel       context.CancelFunc
	shutdownOrdered      bool
	hooksRegisteredTotal atomic.Int64
	hooksExecutedTotal   atomic.Int64
	hookErrorsTotal      atomic.Int64
}

// GetShutdownHookStats returns lifetime counters for hooks registered, executed, and errors.
func (m *ShutdownHookManager) GetShutdownHookStats() (registered, executed, errors int64) {
	if m == nil {
		return 0, 0, 0
	}
	return m.hooksRegisteredTotal.Load(), m.hooksExecutedTotal.Load(), m.hookErrorsTotal.Load()
}

// NewShutdownHookManager creates a new shutdown hook manager
func NewShutdownHookManager() *ShutdownHookManager {
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext()) //nolint:gosec // G118: cancel stored; invoked when shutdown is ordered
	return &ShutdownHookManager{
		hooks:          make([]ShutdownHook, 0),
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
	}
}

// RegisterHook registers a shutdown hook that will be notified when shutdown is ordered
// Hooks are called in the order they were registered
// Each hook receives a context that is cancelled when shutdown is ordered
func (m *ShutdownHookManager) RegisterHook(hook ShutdownHook) {
	m.hooksRegisteredTotal.Add(1)
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameShutdownHooksRegister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.hooks = append(m.hooks, hook)
			return nil
		},
	)
}

// OrderShutdown signals that shutdown has been ordered
// This cancels the shutdown context, notifying all registered hooks
// Returns the shutdown context that hooks can use
func (m *ShutdownHookManager) OrderShutdown() context.Context {
	var shutdownCtx context.Context
	var cancel context.CancelFunc
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameShutdownHookOrder, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if m.shutdownOrdered {
				// Already ordered, return existing context
				shutdownCtx = m.shutdownCtx
				return nil
			}

			m.shutdownOrdered = true
			cancel = m.shutdownCancel
			shutdownCtx = m.shutdownCtx
			return nil
		},
	)

	// Cancel context outside lock (to notify all hooks)
	if cancel != nil {
		cancel()
	}
	return shutdownCtx
}

// NotifyHooks calls all registered hooks with the shutdown context
// Hooks are called sequentially - each hook should either finish quickly or exit
func (m *ShutdownHookManager) NotifyHooks() []error {
	var hooks []ShutdownHook
	var ctx context.Context
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameShutdownHooksNotify, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			hooks = make([]ShutdownHook, len(m.hooks))
			copy(hooks, m.hooks)
			ctx = m.shutdownCtx
			return nil
		},
	)

	// Call all hooks - they should check the context and exit if cancelled
	errors := make([]error, 0, len(hooks))
	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			errors = append(errors, err)
		}
	}
	m.hooksExecutedTotal.Add(int64(len(hooks)))
	m.hookErrorsTotal.Add(int64(len(errors)))
	return errors
}

// GetShutdownContext returns the shutdown context
// This can be used by components to check if shutdown has been ordered
func (m *ShutdownHookManager) GetShutdownContext() context.Context {
	var shutdownCtx context.Context
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameShutdownHookGetContext, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			shutdownCtx = m.shutdownCtx
			return nil
		},
	)
	return shutdownCtx
}

// IsShutdownOrdered checks if shutdown has been ordered
func (m *ShutdownHookManager) IsShutdownOrdered() bool {
	var ordered bool
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameShutdownHookIsOrdered, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ordered = m.shutdownOrdered
			return nil
		},
	)
	return ordered
}
