package mcp

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ProcessGroupManager manages a group of goroutines and subprocesses
// It provides centralized control for shutdown, cancellation, and lifecycle management
// This ensures all spawned processes can be tracked and controlled
type ProcessGroupManager struct {
	mu                       sync.RWMutex
	shutdownCtx              context.Context
	shutdownCancel           context.CancelFunc
	shutdownFlag             atomic.Bool // Lock-free boolean: false = running, true = shutting down
	goroutines               map[string]*TrackedGoroutine
	subprocesses             map[string]*TrackedSubprocess
	shutdownTimeout          time.Duration // Time to wait for graceful shutdown before force kill
	goroutinesSpawnedTotal   atomic.Int64
	subprocessesSpawnedTotal atomic.Int64
}

// GetProcessGroupStats returns lifetime counters for total goroutines and subprocesses spawned.
func (pgm *ProcessGroupManager) GetProcessGroupStats() (goroutines, subprocesses int64) {
	if pgm == nil {
		return 0, 0
	}
	return pgm.goroutinesSpawnedTotal.Load(), pgm.subprocessesSpawnedTotal.Load()
}

// TrackedGoroutine represents a tracked goroutine
type TrackedGoroutine struct {
	ID          string
	Name        string
	Cancel      context.CancelFunc
	Ctx         context.Context
	StartedAt   time.Time
	Description string
	Critical    bool // If true, must complete before shutdown (e.g., saving state)
}

// TrackedSubprocess represents a tracked subprocess
type TrackedSubprocess struct {
	ID          string
	Name        string
	ProcessID   int
	StartedAt   time.Time
	Description string
	Critical    bool         // If true, must complete before shutdown
	KillFunc    func() error // Function to kill the process
}

func trackedGoroutineSlice(m map[string]*TrackedGoroutine) []*TrackedGoroutine {
	out := make([]*TrackedGoroutine, 0, len(m))
	for _, g := range m {
		out = append(out, g)
	}
	return out
}

func trackedSubprocessSlice(m map[string]*TrackedSubprocess) []*TrackedSubprocess {
	out := make([]*TrackedSubprocess, 0, len(m))
	for _, s := range m {
		out = append(out, s)
	}
	return out
}

// NewProcessGroupManager creates a new process group manager
// shutdownTimeout is the time to wait for graceful shutdown before force kill
// ctx: parent context from command entry point (should not be created here)
func NewProcessGroupManager(ctx context.Context, shutdownTimeout time.Duration) *ProcessGroupManager {
	// Derive cancellation context from parent (command context)
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // G118: cancel stored; invoked on shutdown
	return &ProcessGroupManager{
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
		// shutdownFlag starts at 0 (default for atomic.Int32)
		goroutines:      make(map[string]*TrackedGoroutine),
		subprocesses:    make(map[string]*TrackedSubprocess),
		shutdownTimeout: shutdownTimeout,
	}
}

// GetShutdownContext returns the shutdown context
// Components should check ctx.Done() to detect shutdown
func (pgm *ProcessGroupManager) GetShutdownContext() context.Context {
	return pgm.shutdownCtx
}

// IsShuttingDown checks if shutdown has been ordered
// Uses atomic load for lock-free, thread-safe check
func (pgm *ProcessGroupManager) IsShuttingDown() bool {
	return pgm.shutdownFlag.Load()
}

// SpawnGoroutine spawns a goroutine and tracks it
// The goroutine will receive a context that is cancelled on shutdown
// If critical is true, the goroutine must complete before shutdown completes
// Returns the context for the goroutine and a function to unregister it
// CRITICAL: This function is thread-safe and can be called from any goroutine
func (pgm *ProcessGroupManager) SpawnGoroutine(id, name, description string, critical bool, fn func(ctx context.Context)) (context.Context, func()) {
	// Check shutdown before spawning (atomic check, no lock needed)
	if pgm.IsShuttingDown() {
		// Shutdown already ordered - don't spawn new goroutine
		// Return cancelled context so caller knows shutdown is in progress
		return pgm.shutdownCtx, func() {}
	}

	pgm.goroutinesSpawnedTotal.Add(1)

	// Create context for this goroutine
	goroutineCtx, cancel := context.WithCancel(pgm.shutdownCtx) //nolint:gosec // G118: cancel stored in TrackedGoroutine

	// Track the goroutine
	_ = concurrency.RunInLockWithLogger(
		&pgm.mu, LockNameProcessGroupRegisterGoroutine, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pgm.goroutines[id] = &TrackedGoroutine{
				ID:          id,
				Name:        name,
				Cancel:      cancel,
				Ctx:         goroutineCtx,
				StartedAt:   time.Now(),
				Description: description,
				Critical:    critical,
			}
			return nil
		},
	)

	// Spawn the goroutine
	goroutinelabels.NewGoroutine(name, description).
		WithShutdownCheck(func() bool {
			return pgm.IsShuttingDown()
		}).
		WithPreCleanup(func() {
			// Unregister on exit (cleanup)
			// Use context.Background() for lock acquisition to ensure it works even if // Background: request-or-shutdown derived
			// pgm.shutdownCtx is cancelled (e.g., when called from Shutdown())
			_ = concurrency.RunInLockWithLogger(
				&pgm.mu, LockNameProcessGroupUnregisterCleanup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					delete(pgm.goroutines, id)
					return nil
				},
			)
		}).
		WithPostCleanup(func() {
			cancel()
		}).
		StartWithContext(goroutineCtx, func(ctx context.Context) error {
			// Run the goroutine function
			// The function should check ctx.Done() in its loops
			fn(ctx)
			return nil
		})

	// Return context and unregister function
	return goroutineCtx, func() {
		_ = concurrency.RunInLockWithLogger(
			&pgm.mu, LockNameProcessGroupUnregisterGoroutineManual, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				delete(pgm.goroutines, id)
				return nil
			},
		)
		cancel()
	}
}

// RegisterSubprocess registers a subprocess for tracking
// killFunc should kill the process when called
func (pgm *ProcessGroupManager) RegisterSubprocess(id, name, description string, processID int, critical bool, killFunc func() error) {
	// Check shutdown before registering
	if pgm.IsShuttingDown() {
		// Shutdown already ordered - kill process immediately
		if killFunc != nil {
			_ = killFunc() //nolint:errcheck // Best effort
		}
		return
	}

	pgm.subprocessesSpawnedTotal.Add(1)

	_ = concurrency.RunInLockWithLogger(
		&pgm.mu, LockNameProcessGroupRegisterSubprocess, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pgm.subprocesses[id] = &TrackedSubprocess{
				ID:          id,
				Name:        name,
				ProcessID:   processID,
				StartedAt:   time.Now(),
				Description: description,
				Critical:    critical,
				KillFunc:    killFunc,
			}
			return nil
		},
	)
}

// UnregisterSubprocess unregisters a subprocess
func (pgm *ProcessGroupManager) UnregisterSubprocess(id string) {
	_ = concurrency.RunInLockWithLogger(
		&pgm.mu, LockNameProcessGroupUnregisterSubprocess, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(pgm.subprocesses, id)
			return nil
		},
	)
}

// Shutdown orders shutdown of all tracked processes
// It first attempts graceful shutdown, then force kills if timeout is exceeded
// Returns error if critical processes didn't complete
func (pgm *ProcessGroupManager) Shutdown(reason string) error {
	// Set shutdown flag atomically
	if !pgm.shutdownFlag.CompareAndSwap(false, true) {
		return nil // Already shutting down
	}

	// Cancel shutdown context to notify all processes
	pgm.shutdownCancel()

	// Get all tracked processes
	var goroutines []*TrackedGoroutine
	var subprocesses []*TrackedSubprocess
	_ = concurrency.RunInRLockWithLogger(
		&pgm.mu, LockNameProcessGroupShutdownCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			goroutines = trackedGoroutineSlice(pgm.goroutines)
			subprocesses = trackedSubprocessSlice(pgm.subprocesses)
			return nil
		},
	)

	// Cancel all goroutine contexts (graceful shutdown)
	for _, g := range goroutines {
		if g.Cancel != nil {
			g.Cancel()
		}
	}

	// Wait for non-critical goroutines to exit (with timeout)
	nonCriticalDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("process_group_cleanup", "cleaning up process group goroutines").
		StartSimple(func() {
			for _, g := range goroutines {
				if !g.Critical {
					// Wait for context to be cancelled (goroutine should exit)
					select {
					case <-g.Ctx.Done():
						// Goroutine exited
					case <-time.After(100 * time.Millisecond):
						// Timeout - goroutine didn't exit quickly
					}
				}
			}
			nonCriticalDone <- true
		})

	// Wait for graceful shutdown or timeout
	select {
	case <-nonCriticalDone:
		// Non-critical goroutines exited
	case <-time.After(pgm.shutdownTimeout):
		// Timeout - proceed to force kill
	}

	// Check if critical goroutines are still running
	var criticalRunning []*TrackedGoroutine
	var allGoroutines []*TrackedGoroutine
	_ = concurrency.RunInRLockWithLogger(
		&pgm.mu, LockNameProcessGroupShutdownCheckCritical, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			allGoroutines = trackedGoroutineSlice(pgm.goroutines)
			return nil
		},
	)

	// Check critical goroutines outside lock
	criticalRunning = make([]*TrackedGoroutine, 0)
	for _, g := range allGoroutines {
		if g.Critical {
			// Check if context is still active (goroutine might still be running)
			select {
			case <-g.Ctx.Done():
				// Goroutine exited
			default:
				// Still running
				criticalRunning = append(criticalRunning, g)
			}
		}
	}

	// Kill all subprocesses
	for _, s := range subprocesses {
		if s.KillFunc != nil {
			_ = s.KillFunc() //nolint:errcheck // Best effort
		}
	}

	// If critical goroutines are still running, return error
	if len(criticalRunning) > 0 {
		return errfmt.Errorf("critical goroutines still running after shutdown: %v", criticalRunning)
	}

	return nil
}

// GetStatus returns the current status of all tracked processes
func (pgm *ProcessGroupManager) GetStatus() ProcessGroupStatus {
	var goroutines []*TrackedGoroutine
	var subprocesses []*TrackedSubprocess
	_ = concurrency.RunInRLockWithLogger(
		&pgm.mu, LockNameProcessGroupGetStatus, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			goroutines = trackedGoroutineSlice(pgm.goroutines)
			subprocesses = trackedSubprocessSlice(pgm.subprocesses)
			return nil
		},
	)

	// Build status outside lock
	goroutineStatuses := make([]GoroutineStatus, 0, len(goroutines))
	for _, g := range goroutines {
		goroutineStatuses = append(goroutineStatuses, GoroutineStatus{
			ID:          g.ID,
			Name:        g.Name,
			Description: g.Description,
			Critical:    g.Critical,
			StartedAt:   g.StartedAt,
			Running:     g.Ctx.Err() == nil,
		})
	}

	subprocessStatuses := make([]SubprocessStatus, 0, len(subprocesses))
	for _, s := range subprocesses {
		subprocessStatuses = append(subprocessStatuses, SubprocessStatus{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			ProcessID:   s.ProcessID,
			Critical:    s.Critical,
			StartedAt:   s.StartedAt,
		})
	}

	return ProcessGroupStatus{
		ShuttingDown:    pgm.IsShuttingDown(),
		Goroutines:      goroutineStatuses,
		Subprocesses:    subprocessStatuses,
		GoroutineCount:  len(goroutines),
		SubprocessCount: len(subprocesses),
	}
}

// ProcessGroupStatus represents the status of a process group
type ProcessGroupStatus struct {
	ShuttingDown    bool
	Goroutines      []GoroutineStatus
	Subprocesses    []SubprocessStatus
	GoroutineCount  int
	SubprocessCount int
}

// GoroutineStatus represents the status of a tracked goroutine
type GoroutineStatus struct {
	ID          string
	Name        string
	Description string
	Critical    bool
	StartedAt   time.Time
	Running     bool
}

// SubprocessStatus represents the status of a tracked subprocess
type SubprocessStatus struct {
	ID          string
	Name        string
	Description string
	ProcessID   int
	Critical    bool
	StartedAt   time.Time
}
