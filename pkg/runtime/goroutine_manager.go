package runtime

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// GoroutineManager provides OS-level tracking and management of all goroutines
// in the system. It integrates with the coordination framework to emit events
// for observability and ensures no goroutine leaks.
//
// This is critical for operating system-level process/thread management.
type GoroutineManager struct {
	mu             sync.RWMutex
	goroutines     map[string]*TrackedGoroutine // goroutine ID -> tracked goroutine
	coordinator    coordination.EventCoordinator
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	shutdownOnce   sync.Once
	wg             sync.WaitGroup // Tracks all managed goroutines

	// Statistics
	activeCount  atomic.Int64 // Atomic counter for active goroutines
	totalStarted atomic.Int64 // Total goroutines started (lifetime)
	totalStopped atomic.Int64 // Total goroutines stopped (lifetime)
	totalErrors  atomic.Int64 // Total goroutines that errored

	// Configuration
	maxGoroutines   int           // Maximum concurrent goroutines (0 = unlimited)
	shutdownTimeout time.Duration // Timeout for graceful shutdown

	// Test-only: Notification channel for leak detection events
	// Set by tests to receive deterministic notifications when StatusLeaked is set
	leakNotificationChan chan string // goroutine ID
}

// TrackedGoroutine represents a tracked goroutine with full lifecycle metadata
type TrackedGoroutine struct {
	ID        string             // Unique identifier
	Name      string             // Human-readable name
	Purpose   string             // Purpose/function of this goroutine
	Category  string             // Category: "background", "worker", "listener", "periodic", etc.
	StartTime time.Time          // When goroutine started
	StopTime  *time.Time         // When goroutine stopped (nil if still running)
	Context   context.Context    // Context for cancellation
	Cancel    context.CancelFunc // Cancel function
	Resources []Resource         // Resources this goroutine uses (tickers, channels, etc.)
	Metadata  map[string]any     // Additional metadata
	Error     error              // Error if goroutine exited with error
	Status    GoroutineStatus    // Current status
	mu        sync.RWMutex       // Protects fields
}

// Resource represents a resource used by a goroutine (ticker, channel, etc.)
type Resource struct {
	Type        string       // "ticker", "channel", "file", "connection", etc.
	ID          string       // Resource identifier
	Description string       // Human-readable description
	CleanupFunc func() error // Function to cleanup this resource
}

// GoroutineStatus represents the lifecycle status of a goroutine
type GoroutineStatus string

const (
	StatusStarting GoroutineStatus = "starting" // Goroutine is starting
	StatusRunning  GoroutineStatus = "running"  // Goroutine is running
	StatusStopping GoroutineStatus = "stopping" // Goroutine is stopping
	StatusStopped  GoroutineStatus = "stopped"  // Goroutine has stopped
	StatusError    GoroutineStatus = "error"    // Goroutine exited with error
	StatusLeaked   GoroutineStatus = "leaked"   // Goroutine detected as leaked (didn't stop on shutdown)
)

// GoroutineConfig configures a goroutine before starting
type GoroutineConfig struct {
	Name      string                     // Human-readable name
	Purpose   string                     // Purpose/function
	Category  string                     // Category: "background", "worker", "listener", "periodic"
	Context   context.Context            // Base context (if nil, uses manager's context)
	Resources []Resource                 // Resources this goroutine will use
	Metadata  map[string]any             // Additional metadata
	OnStart   func(id string)            // Callback when goroutine starts
	OnStop    func(id string, err error) // Callback when goroutine stops
}

// NewGoroutineManager creates a new goroutine manager
// ctx: parent context from command entry point (should not be created here)
func NewGoroutineManager(ctx context.Context, coordinator coordination.EventCoordinator) *GoroutineManager {
	// Derive cancellation context from parent (command context)
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // G118: cancel stored; invoked on manager shutdown
	return &GoroutineManager{
		goroutines:      make(map[string]*TrackedGoroutine),
		coordinator:     coordinator,
		shutdownCtx:     ctx,
		shutdownCancel:  cancel,
		maxGoroutines:   0, // Unlimited by default
		shutdownTimeout: 30 * time.Second,
	}
}

// Start starts a new goroutine with full lifecycle tracking
// Returns the goroutine ID and a context that will be cancelled on shutdown
func (gm *GoroutineManager) Start(config GoroutineConfig, fn func(ctx context.Context) error) (string, context.Context, error) {
	// Generate unique ID
	id := generateGoroutineID(config.Name)

	// Check max goroutines limit
	if gm.maxGoroutines > 0 {
		current := gm.activeCount.Load()
		if int(current) >= gm.maxGoroutines {
			return emptyValue, nil, errfmt.Errorf("max goroutines limit reached: %d", gm.maxGoroutines)
		}
	}

	// Create context for this goroutine
	baseCtx := config.Context
	if baseCtx == nil {
		baseCtx = gm.shutdownCtx
	}
	ctx, cancel := context.WithCancel(baseCtx) //nolint:gosec // G118: cancel stored on TrackedGoroutine

	// Create tracked goroutine
	tracked := &TrackedGoroutine{
		ID:        id,
		Name:      config.Name,
		Purpose:   config.Purpose,
		Category:  config.Category,
		StartTime: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
		Resources: config.Resources,
		Metadata:  config.Metadata,
		Status:    StatusStarting,
	}
	if tracked.Metadata == nil {
		tracked.Metadata = make(map[string]any)
	}

	// Register goroutine
	_ = concurrency.RunInLock(&gm.mu, func() error {
		gm.goroutines[id] = tracked
		return nil
	})

	gm.activeCount.Add(1)
	gm.totalStarted.Add(1)
	// Note: Do NOT call gm.wg.Add(1) here - goroutinelabels.WithWaitGroup() handles it automatically
	// when the goroutine starts. Calling it here would cause a double Add(1).

	// Emit start event
	gm.emitGoroutineEvent(id, "start", tracked, nil)

	// Call onStart callback
	if config.OnStart != nil {
		config.OnStart(id)
	}

	// Update status to running
	_ = concurrency.RunInLockWithLogger(
		&tracked.mu, LockNameGoroutineManagerSetRunning, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			tracked.Status = StatusRunning
			return nil
		},
	)

	// Start goroutine
	goroutinelabels.NewGoroutine(config.Name, config.Purpose).
		WithWaitGroup(&gm.wg).
		WithPanicHandler(func(r any) {
			err := errfmt.Errorf("panic in goroutine %s: %v", id, r)
			gm.stopGoroutine(id, err, config.OnStop)
		}).
		WithErrorHandler(func(err error) {
			gm.stopGoroutine(id, err, config.OnStop)
		}).
		WithPostCleanup(func() {
			// PostCleanup is called after the function completes but before defer done()
			// This is the right place to call stopGoroutine for normal completion
			// Error and panic handlers already call stopGoroutine, so we only need to handle normal completion here
			// Check if stopGoroutine was already called (by checking if status is still Running)
			// Use proper locking to access goroutines map
			// Use context.Background() for lock acquisition to ensure it works even if
			// gm.shutdownCtx is cancelled (e.g., when called from Shutdown())
			var tracked *TrackedGoroutine
			var exists bool
			_ = concurrency.RunInRLockWithLogger(
				&gm.mu, LockNameGoroutineManagerPostcleanupCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					var ok bool
					tracked, ok = gm.goroutines[id]
					exists = ok
					return nil
				},
			)
			if exists && tracked != nil {
				var status GoroutineStatus
				_ = concurrency.RunInRLock(&tracked.mu, func() error {
					status = tracked.Status
					return nil
				})
				if status == StatusRunning {
					// Normal completion - stopGoroutine wasn't called by error/panic handler
					gm.stopGoroutine(id, nil, config.OnStop)
				}
			}
		}).
		StartWithContext(ctx, func(ctx context.Context) error {
			// Just execute the function - stopGoroutine will be called in PostCleanup
			return fn(ctx)
		})

	return id, ctx, nil
}

// Stop stops a specific goroutine by ID
func (gm *GoroutineManager) Stop(id string) error {
	var tracked *TrackedGoroutine
	var exists bool
	_ = concurrency.RunInRLock(&gm.mu, func() error {
		tracked, exists = gm.goroutines[id]
		return nil
	})

	if !exists {
		return errfmt.Errorf("goroutine %s not found", id)
	}

	// Cancel context
	tracked.Cancel()

	// Cleanup resources
	for _, resource := range tracked.Resources {
		if resource.CleanupFunc != nil {
			if err := resource.CleanupFunc(); err != nil {
				// Log but don't fail
				gm.emitGoroutineEvent(id, "cleanup_error", tracked, err)
			}
		}
	}

	return nil
}

// StopAll stops all managed goroutines
func (gm *GoroutineManager) StopAll() {
	var ids []string
	// Use context.Background() for lock acquisition to ensure it works even if
	// gm.shutdownCtx is cancelled (e.g., when called from Shutdown())
	err := concurrency.RunInRLockWithLogger(
		&gm.mu, LockNameGoroutineManagerStopAllGetIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ids = make([]string, 0, len(gm.goroutines))
			for id := range gm.goroutines {
				ids = append(ids, id)
			}
			return nil
		},
	)
	if err != nil {
		// Log error but continue - best effort
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)).Warn("Failed to acquire lock in StopAll",
			concurrency.LockField{Key: "error", Value: err.Error()})
		return
	}

	for _, id := range ids {
		if err := gm.Stop(id); err != nil {
			// Log error but continue - best effort
			logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)).Warn("Failed to stop goroutine",
				concurrency.LockField{Key: "id", Value: id},
				concurrency.LockField{Key: "error", Value: err.Error()})
		}
	}
}

// Shutdown gracefully shuts down all goroutines with timeout
func (gm *GoroutineManager) Shutdown() error {
	var shutdownErr error
	gm.shutdownOnce.Do(func() {
		// Stop all goroutines first (calls tracked.Cancel() on each)
		// This ensures individual goroutine contexts are cancelled explicitly
		gm.StopAll()

		// Cancel all contexts (this will also cancel any remaining child contexts)
		// Note: StopAll() and stopGoroutine() use context.Background() for lock acquisition
		// to ensure they work even after gm.shutdownCtx is cancelled
		gm.shutdownCancel()

		// Wait for all goroutines to finish (with timeout)
		done := make(chan struct{})
		goroutinelabels.NewGoroutine("goroutine_manager_shutdown_wait", "waiting for all goroutines to stop during shutdown").
			WithCleanup(func() {
				close(done)
			}).
			StartSimple(func() {
				gm.wg.Wait()
			})

		select {
		case <-done:
			// All goroutines finished
		case <-time.After(gm.shutdownTimeout):
			// Timeout - mark remaining as leaked
			var goroutinesCopy map[string]*TrackedGoroutine
			var shutdownTimeout time.Duration
			_ = concurrency.RunInRLockWithLogger(
				&gm.mu, LockNameGoroutineManagerShutdownTimeoutCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					goroutinesCopy = make(map[string]*TrackedGoroutine)
					for k, v := range gm.goroutines {
						goroutinesCopy[k] = v
					}
					shutdownTimeout = gm.shutdownTimeout
					return nil
				},
			)
			for id, tracked := range goroutinesCopy {
				var wasMarkedLeaked bool
				_ = concurrency.RunInLockWithLogger(
					&tracked.mu, LockNameGoroutineManagerMarkLeaked, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						if tracked.Status == StatusRunning || tracked.Status == StatusStopping {
							tracked.Status = StatusLeaked
							wasMarkedLeaked = true
						}
						return nil
					},
				)
				if wasMarkedLeaked {
					gm.emitGoroutineEvent(id, "leaked", tracked, errfmt.Errorf("goroutine did not stop within %v", shutdownTimeout))
					// Notify test channel if set (non-blocking)
					if gm.leakNotificationChan != nil {
						select {
						case gm.leakNotificationChan <- id:
						default:
							// Channel full or closed - ignore
						}
					}
				}
			}
			shutdownErr = errfmt.Errorf("shutdown timeout: %d goroutines did not stop within %v", gm.ActiveCount(), gm.shutdownTimeout)
		}
	})

	return shutdownErr
}

// stopGoroutine marks a goroutine as stopped
func (gm *GoroutineManager) stopGoroutine(id string, err error, onStop func(string, error)) {
	now := time.Now()

	var tracked *TrackedGoroutine
	var exists bool
	var alreadyStopped bool
	// Use context.Background() for lock acquisition to ensure it works even if
	// gm.shutdownCtx is cancelled (e.g., when called from Shutdown())
	_ = concurrency.RunInLockWithLogger(
		&gm.mu, LockNameGoroutineManagerStopGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			tracked, ok = gm.goroutines[id]
			exists = ok
			if !exists {
				return nil
			}
			return nil
		},
	)

	if !exists {
		return
	}

	// Use context.Background() for lock acquisition to ensure it works even if
	// gm.shutdownCtx is cancelled (e.g., when called from Shutdown())
	_ = concurrency.RunInLockWithLogger(
		&tracked.mu, LockNameGoroutineManagerStopCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if tracked.Status == StatusStopped || tracked.Status == StatusError {
				alreadyStopped = true
				return nil
			}
			tracked.Status = StatusStopping
			return nil
		},
	)

	if alreadyStopped {
		return
	}

	// Cleanup resources
	for _, resource := range tracked.Resources {
		if resource.CleanupFunc != nil {
			_ = resource.CleanupFunc() // Best effort
		}
	}

	// Use context.Background() for lock acquisition to ensure it works even if
	// gm.shutdownCtx is cancelled (e.g., when called from Shutdown())
	_ = concurrency.RunInLockWithLogger(
		&tracked.mu, LockNameGoroutineManagerStopUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			tracked.StopTime = &now
			tracked.Error = err
			if err != nil {
				tracked.Status = StatusError
				gm.totalErrors.Add(1)
			} else {
				tracked.Status = StatusStopped
			}
			return nil
		},
	)

	gm.activeCount.Add(-1)
	gm.totalStopped.Add(1)
	// Note: Do NOT call gm.wg.Done() here - goroutinelabels.WithWaitGroup() handles it automatically
	// when the goroutine exits. Calling it here would cause a double Done() call.

	// Emit stop event
	gm.emitGoroutineEvent(id, "stop", tracked, err)

	// Call onStop callback
	if onStop != nil {
		onStop(id, err)
	}
}

// emitGoroutineEvent emits an event through the coordination framework
func (gm *GoroutineManager) emitGoroutineEvent(id, eventType string, tracked *TrackedGoroutine, err error) {
	if gm.coordinator == nil {
		return
	}

	var duration time.Duration
	var status string
	var metadata map[string]any
	_ = concurrency.RunInRLock(&tracked.mu, func() error {
		duration = time.Since(tracked.StartTime)
		if tracked.StopTime != nil {
			duration = tracked.StopTime.Sub(tracked.StartTime)
		}
		status = "running"
		if tracked.Status != emptyValue {
			status = string(tracked.Status)
		}
		metadata = map[string]any{
			"goroutine_id":           id,
			"goroutine_name":         tracked.Name,
			objects.FieldKeyPurpose:  tracked.Purpose,
			objects.FieldKeyCategory: tracked.Category,
			objects.FieldKeyStatus:   status,
			"duration_ms":            duration.Milliseconds(),
			"start_time":             tracked.StartTime.Format(time.RFC3339),
		}
		if tracked.StopTime != nil {
			metadata["stop_time"] = tracked.StopTime.Format(time.RFC3339)
		}
		if err != nil {
			metadata["error"] = err.Error()
		}
		if len(tracked.Resources) > 0 {
			resources := make([]map[string]any, 0, len(tracked.Resources))
			for _, r := range tracked.Resources {
				resources = append(resources, map[string]any{
					objects.FieldKeyType:        r.Type,
					objects.FieldKeyID:          r.ID,
					objects.FieldKeyDescription: r.Description,
				})
			}
			metadata["resources"] = resources
		}
		for k, v := range tracked.Metadata {
			metadata[k] = v
		}
		return nil
	})

	eventCtx := coordination.NewEventContext(id, "goroutine_lifecycle", eventType).
		WithEventData(&coordination.EventData{
			LoggingFields: []coordination.LoggingField{
				{Key: "event", Value: fmt.Sprintf("goroutine_%s", eventType)},
				{Key: "goroutine_id", Value: id},
				{Key: "goroutine_name", Value: tracked.Name},
				{Key: "category", Value: tracked.Category},
			},
			AuditMetadata: metadata,
			MetricsData: map[string]any{
				objects.FieldKeyOperation: "goroutine_lifecycle",
				objects.FieldKeyEventType: eventType,
				"goroutine_id":            id,
				objects.FieldKeyCategory:  tracked.Category,
				"duration_ms":             duration.Milliseconds(),
				objects.FieldKeyStatus:    status,
			},
		}).
		WithChannels(true, true, true, true) // All channels

	if err != nil {
		eventCtx.Error = err
		eventCtx.Status = "error"
	}

	_ = gm.coordinator.Emit(gm.shutdownCtx, eventCtx) //nolint:errcheck // Best effort
}

// GetGoroutine returns information about a specific goroutine
func (gm *GoroutineManager) GetGoroutine(id string) (*TrackedGoroutine, error) {
	var tracked *TrackedGoroutine
	var exists bool
	_ = concurrency.RunInRLock(&gm.mu, func() error {
		tracked, exists = gm.goroutines[id]
		return nil
	})
	if !exists {
		return nil, errfmt.Errorf("goroutine %s not found", id)
	}
	return tracked, nil
}

// ListGoroutines returns all tracked goroutines
func (gm *GoroutineManager) ListGoroutines() []*TrackedGoroutine {
	var result []*TrackedGoroutine
	_ = concurrency.RunInRLock(&gm.mu, func() error {
		result = make([]*TrackedGoroutine, 0, len(gm.goroutines))
		for _, tracked := range gm.goroutines {
			result = append(result, tracked)
		}
		return nil
	})
	return result
}

// ActiveCount returns the number of currently active goroutines
func (gm *GoroutineManager) ActiveCount() int {
	return int(gm.activeCount.Load())
}

// TotalStarted returns the total number of goroutines started (lifetime)
func (gm *GoroutineManager) TotalStarted() int64 {
	return gm.totalStarted.Load()
}

// TotalStopped returns the total number of goroutines stopped (lifetime)
func (gm *GoroutineManager) TotalStopped() int64 {
	return gm.totalStopped.Load()
}

// TotalErrors returns the total number of goroutines that errored
func (gm *GoroutineManager) TotalErrors() int64 {
	return gm.totalErrors.Load()
}

// DetectLeaks detects goroutines that should have stopped but are still running
func (gm *GoroutineManager) DetectLeaks() []*TrackedGoroutine {
	var goroutinesCopy map[string]*TrackedGoroutine
	_ = concurrency.RunInRLockWithLogger(
		&gm.mu, LockNameGoroutineManagerDetectLeaksCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			goroutinesCopy = make(map[string]*TrackedGoroutine)
			for k, v := range gm.goroutines {
				goroutinesCopy[k] = v
			}
			return nil
		},
	)

	leaks := make([]*TrackedGoroutine, 0)
	now := time.Now()

	for _, tracked := range goroutinesCopy {
		var status GoroutineStatus
		var startTime time.Time
		_ = concurrency.RunInRLockWithLogger(
			&tracked.mu, LockNameGoroutineManagerLeakCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				status = tracked.Status
				startTime = tracked.StartTime
				return nil
			},
		)

		// Consider a goroutine leaked if:
		// 1. It's been running for more than 1 hour (configurable threshold)
		// 2. It's in running status
		// 3. The manager has been shut down
		duration := now.Sub(startTime)
		if status == StatusRunning && duration > time.Hour {
			leaks = append(leaks, tracked)
		}
	}

	return leaks
}

// SetMaxGoroutines sets the maximum number of concurrent goroutines
func (gm *GoroutineManager) SetMaxGoroutines(max int) {
	_ = concurrency.RunInLockWithLogger(
		&gm.mu, LockNameGoroutineManagerSetMax, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			gm.maxGoroutines = max
			return nil
		},
	)
}

// SetShutdownTimeout sets the timeout for graceful shutdown
func (gm *GoroutineManager) SetShutdownTimeout(timeout time.Duration) {
	_ = concurrency.RunInLockWithLogger(
		&gm.mu, LockNameGoroutineManagerSetShutdownTimeout, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			gm.shutdownTimeout = timeout
			return nil
		},
	)
}

// SetLeakNotificationChannel sets a channel to receive notifications when goroutines are marked as leaked
// This is primarily for testing to enable deterministic, event-driven leak detection
// The channel receives goroutine IDs when StatusLeaked is set
func (gm *GoroutineManager) SetLeakNotificationChannel(ch chan string) {
	_ = concurrency.RunInLockWithLogger(
		&gm.mu, LockNameGoroutineManagerSetLeakChannel, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			gm.leakNotificationChan = ch
			return nil
		},
	)
}

// generateGoroutineID generates a unique ID for a goroutine
func generateGoroutineID(name string) string {
	// Use timestamp + name hash for uniqueness
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("goroutine_%s_%d", name, timestamp)
}

// GetRuntimeStats returns runtime statistics about all goroutines
func (gm *GoroutineManager) GetRuntimeStats() map[string]any {
	var goroutinesCopy map[string]*TrackedGoroutine
	var maxGoroutines int
	_ = concurrency.RunInRLockWithLogger(
		&gm.mu, LockNameGoroutineManagerGetStatsCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			goroutinesCopy = make(map[string]*TrackedGoroutine)
			for k, v := range gm.goroutines {
				goroutinesCopy[k] = v
			}
			maxGoroutines = gm.maxGoroutines
			return nil
		},
	)

	stats := map[string]any{
		"active_count":   gm.ActiveCount(),
		"total_started":  gm.TotalStarted(),
		"total_stopped":  gm.TotalStopped(),
		"total_errors":   gm.TotalErrors(),
		"max_goroutines": maxGoroutines,
	}

	// Count by status
	statusCounts := make(map[string]int)
	categoryCounts := make(map[string]int)

	for _, tracked := range goroutinesCopy {
		var status string
		var category string
		_ = concurrency.RunInRLockWithLogger(
			&tracked.mu, LockNameGoroutineManagerStatsRead, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				status = string(tracked.Status)
				category = tracked.Category
				return nil
			},
		)

		statusCounts[status]++
		categoryCounts[category]++
	}

	stats[objects.FieldKeyStatusCounts] = statusCounts
	stats["category_counts"] = categoryCounts

	// Runtime stats
	stats["runtime_goroutines"] = runtime.NumGoroutine()

	return stats
}
