package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// HashRegistryManager tracks all HashRegistry instances for shutdown coordination
type HashRegistryManager struct {
	registries map[string]*HashRegistry // kind -> registry
	mu         sync.RWMutex
}

var (
	globalHashRegistryManager     *HashRegistryManager
	globalHashRegistryManagerOnce sync.Once
)

// GetGlobalHashRegistryManager returns the global hash registry manager (singleton)
func GetGlobalHashRegistryManager() *HashRegistryManager {
	globalHashRegistryManagerOnce.Do(func() {
		globalHashRegistryManager = &HashRegistryManager{
			registries: make(map[string]*HashRegistry),
		}
		// Register with shutdown coordinator so InitiateShutdown() propagates to all registries
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil {
			coordinator.RegisterQueue(globalHashRegistryManager)
		}
	})
	return globalHashRegistryManager
}

// RegisterRegistry registers a hash registry instance
func (m *HashRegistryManager) RegisterRegistry(kind string, registry *HashRegistry) {
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameHashRegistryManagerRegister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		m.registries[kind] = registry
		return nil
	})

	// If shutdown is already initiated, cancel this registry's context immediately
	// This ensures that HashRegistry instances created after shutdown is initiated
	// don't start workers that will leak
	coordinator := GetGlobalShutdownCoordinator()
	if coordinator != nil && coordinator.IsShutdownInitiated() {
		var _err_83266877 = registry.InitiateShutdown()
		if // Best effort - error is non-critical
		_err_83266877 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

				// GetAllRegistries returns all registered hash registries
				ProfileSystem))).Error(ErrMsgSwallowedError,

				_err_83266877).Log()
		}
	}
}

func (m *HashRegistryManager) GetAllRegistries() []*HashRegistry {
	var registries []*HashRegistry
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameHashRegistryManagerGetAll, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		registries = make([]*HashRegistry, 0, len(m.registries))
		for _, registry := range m.registries {
			registries = append(registries, registry)
		}
		return nil
	})
	return registries
}

// InitiateShutdown implements QueueShutdownHandler
// Stops accepting new save operations for all registries
func (m *HashRegistryManager) InitiateShutdown() error {
	registries := m.GetAllRegistries()
	for _, registry := range registries {
		if err := registry.InitiateShutdown(); err != nil {
			return errfmt.Newf(ConstMiscFailedToInitiateShutdownForRegistryS, registry.GetName()).Wrap(err)
		}
	}
	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending save operations for all registries
func (m *HashRegistryManager) Drain(ctx context.Context) error {
	registries := m.GetAllRegistries()

	// Drain all registries in parallel
	var wg sync.WaitGroup
	drainErrors := make(chan error, len(registries))

	for _, registry := range registries {
		hr := registry // Capture loop variable
		goroutinelabels.NewGoroutine(fmt.Sprintf(ConstMiscHashRegistryDrainS, hr.GetName()), fmt.Sprintf(ConstMiscDrainingHashRegistryS, hr.GetName())).
			WithWaitGroup(&wg).
			WithErrorHandler(func(err error) {
				if err != nil && !errors.Is(err, context.Canceled) {
					drainErrors <- errfmt.Newf(ConstMiscRegistrySDrainFailed, hr.GetName()).Wrap(err)
				}
			}).
			StartWithContext(ctx, func(ctx context.Context) error {
				return hr.Drain(ctx)
			})
	}

	done := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstMiscHashRegistryDrainWait, ConstMiscWaitingForHashRegistryDrainOperations).
		WithContext(ctx).
		WithCleanup(func() {
			close(done)
		}).
		StartSimple(func() {
			wg.Wait()
		})

	select {
	case <-done:
		// Check for errors
		close(drainErrors)
		var errors []error
		for err := range drainErrors {
			errors = append(errors, err)
		}
		if len(errors) > 0 {
			return errfmt.Errorf(ConstMiscDrainErrorsV, errors)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (m *HashRegistryManager) IsDrained() bool {
	registries := m.GetAllRegistries()
	for _, registry := range registries {
		if !registry.IsDrained() {
			return false
		}
	}
	return true
}

// GetPendingCount implements QueueShutdownHandler
func (m *HashRegistryManager) GetPendingCount() int64 {
	registries := m.GetAllRegistries()
	var total int64
	for _, registry := range registries {
		total += registry.GetPendingCount()
	}
	return total
}

// GetName implements QueueShutdownHandler
func (m *HashRegistryManager) GetName() string {
	return ConstMiscHashRegistryManager
}

// IsCritical implements QueueShutdownHandler
// Hash registry saves are critical - must complete before shutdown
func (m *HashRegistryManager) IsCritical() bool {
	return true
}
