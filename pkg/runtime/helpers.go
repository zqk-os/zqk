package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

const (
	errManagerRequired      = "goroutine manager is required"
	errIntervalPositive     = "interval must be positive"
	errWorkerCountPositive  = "worker count must be positive"
	errWorkerFnRequired     = "worker function is required"
	errCoordinatorRequired  = "coordinator is required"
	errEventContextRequired = "event context is required"
	defaultWorkerCategory   = "worker"
	defaultEventCategory    = "event_emission"
	defaultAsyncCategory    = "async_operation"
	resourceTypeTicker      = "ticker"
	metaIntervalKey         = "interval"
	metaWorkerIDKey         = "worker_id"
	metaWorkerPoolKey       = "worker_pool"
	metaWorkerCountKey      = "worker_count"
	metaOperationIDKey      = "operation_id"
	metaOperationTypeKey    = "operation_type"
	metaStatusKey           = "status"
	goroutineEventEmitter   = "runtime_event_emitter"
	categoryPeriodic        = "periodic"
	tickerIDFmt             = "%s_ticker"
	errStartWorkerFmt       = "failed to start worker %d: %w"
	purposeEmitEventFmt     = "Emit %s event asynchronously"
	purposeExecuteAsyncFmt  = "Execute %s asynchronously"
	goroutineEmitEventFmt   = "emit_event_%s"
	emptyValue              = ""
)

// StartPeriodicTask starts a periodic background task with full lifecycle tracking.
// This is a reusable component for Category 1: Periodic Background Tasks.
//
// Requirements:
//   - R-GOROUTINE-001: Lifecycle tracking
//   - R-GOROUTINE-002: Context cancellation
//   - R-GOROUTINE-003: Resource management (ticker)
//   - R-GOROUTINE-004: Observability (events)
//   - R-GOROUTINE-005: Graceful shutdown
//
// Example:
//
//	id, ctx, err := StartPeriodicTask(
//	    manager,
//	    "metrics_compression",
//	    24*time.Hour,
//	    func(ctx context.Context) error {
//	        return compressMetrics(ctx)
//	    },
//	)
func StartPeriodicTask(
	manager *GoroutineManager,
	name string,
	interval time.Duration,
	fn func(ctx context.Context) error,
) (string, context.Context, error) {
	if manager == nil {
		return emptyValue, nil, errors.New(errManagerRequired)
	}
	if interval <= 0 {
		return emptyValue, nil, errors.New(errIntervalPositive)
	}

	// Create ticker
	ticker := time.NewTicker(interval)

	// Start goroutine with ticker as resource
	id, ctx, err := manager.Start(GoroutineConfig{
		Name:     name,
		Purpose:  fmt.Sprintf("Periodic task: %s (interval: %v)", name, interval),
		Category: categoryPeriodic,
		Resources: []Resource{
			{
				Type:        resourceTypeTicker,
				ID:          fmt.Sprintf(tickerIDFmt, name),
				Description: fmt.Sprintf("Periodic ticker for %s", name),
				CleanupFunc: func() error {
					ticker.Stop()
					return nil
				},
			},
		},
		Metadata: map[string]any{
			metaIntervalKey: interval.String(),
		},
	}, func(ctx context.Context) error {
		defer ticker.Stop()

		// Run once immediately (optional - can be removed if not desired)
		// if err := fn(ctx); err != nil {
		//     return err
		// }

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if err := fn(ctx); err != nil {
					// Log error but continue (periodic tasks should be resilient)
					// Return error only if it's a fatal error
					return err
				}
			}
		}
	})

	return id, ctx, err
}

// StartWorkerPool starts a worker pool with full lifecycle tracking.
// This is a reusable component for Category 2: Worker Pools.
//
// Requirements:
//   - R-GOROUTINE-001: Lifecycle tracking (per worker)
//   - R-GOROUTINE-002: Context cancellation
//   - R-GOROUTINE-004: Observability (events per worker)
//   - R-GOROUTINE-005: Graceful shutdown (all workers)
//
// Example:
//
//	workerIDs, err := StartWorkerPool(
//	    manager,
//	    "validation_worker",
//	    4,
//	    func(ctx context.Context, id int) error {
//	        return processValidationQueue(ctx, id)
//	    },
//	)
func StartWorkerPool(
	manager *GoroutineManager,
	name string,
	workerCount int,
	fn func(ctx context.Context, workerID int) error,
) ([]string, error) {
	if manager == nil {
		return nil, errors.New(errManagerRequired)
	}
	if workerCount <= 0 {
		return nil, errors.New(errWorkerCountPositive)
	}
	if fn == nil {
		return nil, errors.New(errWorkerFnRequired)
	}

	workerIDs := make([]string, 0, workerCount)

	for i := 0; i < workerCount; i++ {
		workerID := i
		workerName := fmt.Sprintf("%s_%d", name, workerID)

		id, _, err := manager.Start(GoroutineConfig{
			Name:     workerName,
			Purpose:  fmt.Sprintf("Worker %d of pool %s", workerID, name),
			Category: defaultWorkerCategory,
			Metadata: map[string]any{
				metaWorkerIDKey:    workerID,
				metaWorkerPoolKey:  name,
				metaWorkerCountKey: workerCount,
			},
		}, func(ctx context.Context) error {
			return fn(ctx, workerID)
		})

		if err != nil {
			// Stop already started workers
			for _, startedID := range workerIDs {
				_ = manager.Stop(startedID) // Best effort
			}
			return nil, errfmt.Errorf(errStartWorkerFmt, workerID, err)
		}

		workerIDs = append(workerIDs, id)
	}

	return workerIDs, nil
}

// EmitEventAsync emits an event asynchronously (fire-and-forget).
// This is a reusable component for Category 3: Async Event Emission.
//
// Requirements:
//   - R-GOROUTINE-001: Lifecycle tracking (short-lived)
//   - R-GOROUTINE-004: Observability (event emission)
//
// Example:
//
//	EmitEventAsync(manager, coordinator, eventCtx)
func EmitEventAsync(
	manager *GoroutineManager,
	coordinator coordination.EventCoordinator,
	eventCtx *coordination.EventContext,
) error {
	if coordinator == nil {
		return errors.New(errCoordinatorRequired)
	}
	if eventCtx == nil {
		return errors.New(errEventContextRequired)
	}

	// For fire-and-forget events, we can use a simple goroutine
	// if manager is nil (backward compatibility), or use manager for tracking
	if manager == nil {
		// Fallback: direct goroutine (not tracked, but acceptable for fire-and-forget)
		goroutinelabels.NewGoroutine(goroutineEventEmitter, fmt.Sprintf("emitting %s event", eventCtx.OperationType)).
			StartSimple(func() {
				// Use system context for background event emission
				_ = coordinator.Emit(pkgctx.NewSystemContext(), eventCtx) //nolint:errcheck // Best effort
			})
		return nil
	}

	// Use manager for tracking
	_, _, err := manager.Start(GoroutineConfig{
		Name:     fmt.Sprintf(goroutineEmitEventFmt, eventCtx.OperationType),
		Purpose:  fmt.Sprintf(purposeEmitEventFmt, eventCtx.OperationType),
		Category: defaultEventCategory,
		Metadata: map[string]any{
			metaOperationIDKey:   eventCtx.OperationID,
			metaOperationTypeKey: eventCtx.OperationType,
			metaStatusKey:        eventCtx.Status,
		},
	}, func(ctx context.Context) error {
		// Use provided context or background
		emitCtx := ctx
		if eventCtx.Ctx != nil {
			emitCtx = eventCtx.Ctx
		}
		return coordinator.Emit(emitCtx, eventCtx)
	})

	return err
}

// ExecuteAsync executes an operation asynchronously with full tracking.
// This is a reusable component for Category 4: One-Off Async Operations.
//
// Requirements:
//   - R-GOROUTINE-001: Lifecycle tracking
//   - R-GOROUTINE-002: Context cancellation
//   - R-GOROUTINE-004: Observability
//
// Example:
//
//	id, ctx, errChan := ExecuteAsync(
//	    manager,
//	    "async_operation",
//	    func(ctx context.Context) error {
//	        return doWork(ctx)
//	    },
//	)
func ExecuteAsync(
	manager *GoroutineManager,
	name string,
	fn func(ctx context.Context) error,
) (string, context.Context, <-chan error) {
	errChan := make(chan error, 1)

	if manager == nil {
		// Fallback: direct goroutine (not tracked)
		// Use system context for fallback goroutine creation
		ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
		goroutinelabels.NewGoroutine(name, fmt.Sprintf(purposeExecuteAsyncFmt, name)).
			WithCleanup(func() {
				cancel()
			}).
			StartWithContext(ctx, func(ctx context.Context) error {
				errChan <- fn(ctx)
				return nil
			})
		return emptyValue, ctx, errChan
	}

	id, ctx, err := manager.Start(GoroutineConfig{
		Name:     name,
		Purpose:  fmt.Sprintf(purposeExecuteAsyncFmt, name),
		Category: defaultAsyncCategory,
	}, func(ctx context.Context) error {
		err := fn(ctx)
		errChan <- err
		return err
	})

	if err != nil {
		errChan <- err
		return emptyValue, nil, errChan
	}

	return id, ctx, errChan
}

// Service is the standard interface for background services.
// This is a reusable component for Category 6: Background Services.
//
// Requirements:
//   - R-GOROUTINE-001: Lifecycle tracking (all goroutines)
//   - R-GOROUTINE-005: Graceful shutdown
//
// Example:
//
//	type MyService struct {
//	    manager *GoroutineManager
//	}
//
//	func (s *MyService) Start(ctx context.Context) error {
//	    // Start all goroutines via manager
//	}
//
//	func (s *MyService) Stop() error {
//	    return s.manager.Shutdown()
//	}
type Service interface {
	// Start starts the service and all its goroutines
	Start(ctx context.Context) error

	// Stop stops the service and all its goroutines
	Stop() error

	// GetGoroutineManager returns the goroutine manager for this service
	GetGoroutineManager() *GoroutineManager
}

// BaseService provides a base implementation of Service
type BaseService struct {
	manager *GoroutineManager
}

// NewBaseService creates a new base service
func NewBaseService(coordinator coordination.EventCoordinator) *BaseService {
	return &BaseService{
		// Note: BaseService may be created before command context exists, use system context
		manager: NewGoroutineManager(pkgctx.NewSystemContext(), coordinator),
	}
}

// GetGoroutineManager returns the goroutine manager
func (s *BaseService) GetGoroutineManager() *GoroutineManager {
	return s.manager
}

// Stop stops the service by shutting down all goroutines
func (s *BaseService) Stop() error {
	if s.manager == nil {
		return nil
	}
	return s.manager.Shutdown()
}
