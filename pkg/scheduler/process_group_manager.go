package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ProcessGroupManager manages a group of goroutines and subprocesses for scheduler jobs
// It provides centralized control for shutdown, cancellation, and lifecycle management
// This ensures all spawned processes can be tracked and controlled
type ProcessGroupManager struct {
	mu              sync.RWMutex
	shutdownCtx     context.Context
	shutdownCancel  context.CancelFunc
	shutdownFlag    int32 // Atomic boolean: 0 = running, 1 = shutting down
	subprocesses    map[string]*TrackedSubprocess
	shutdownTimeout time.Duration // Time to wait for graceful shutdown before force kill
}

// TrackedSubprocess represents a tracked subprocess
type TrackedSubprocess struct {
	ID             string
	JobID          string
	ProcessID      int
	ProcessGroupID int // Negative PID for process group
	StartedAt      time.Time
	Description    string
	Critical       bool         // If true, must complete before shutdown
	KillFunc       func() error // Function to kill the process group
}

// NewProcessGroupManager creates a new process group manager for scheduler
// shutdownTimeout is the time to wait for graceful shutdown before force kill
// ctx: parent context from command entry point (should not be created here)
// NewProcessGroupManager creates a new process group manager
func NewProcessGroupManager(ctx context.Context, shutdownTimeout time.Duration) ProcessGroupManagerInterface {
	// Derive cancellation context from parent (command context)
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // G118: cancel stored; invoked on shutdown
	return &ProcessGroupManager{
		shutdownCtx:     ctx,
		shutdownCancel:  cancel,
		shutdownFlag:    0,
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
	return atomic.LoadInt32(&pgm.shutdownFlag) == 1
}

// RegisterSubprocess registers a subprocess for tracking
// processID is the PID of the process
// processGroupID is the negative PID for the process group (for killing the group)
// killFunc should kill the process group when called
func (pgm *ProcessGroupManager) RegisterSubprocess(id, jobID, description string, processID, processGroupID int, critical bool, killFunc func() error) {
	// Check shutdown before registering
	if pgm.IsShuttingDown() {
		// Shutdown already ordered - kill process immediately
		if killFunc != nil {
			_ = killFunc() //nolint:errcheck // Best effort
		}
		return
	}

	_ = concurrency.RunInLockWithLogger(
		&pgm.mu, LockNameProcessGroupRegister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pgm.subprocesses[id] = &TrackedSubprocess{
				ID:             id,
				JobID:          jobID,
				ProcessID:      processID,
				ProcessGroupID: processGroupID,
				StartedAt:      time.Now(),
				Description:    description,
				Critical:       critical,
				KillFunc:       killFunc,
			}
			return nil
		},
	)
}

// UnregisterSubprocess unregisters a subprocess
func (pgm *ProcessGroupManager) UnregisterSubprocess(id string) {
	_ = concurrency.RunInLockWithLogger(
		&pgm.mu, LockNameProcessGroupUnregister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(pgm.subprocesses, id)
			return nil
		},
	)
}

// Shutdown orders shutdown of all tracked subprocesses
// It first attempts graceful shutdown (SIGTERM), then force kills (SIGKILL) if timeout is exceeded
// Returns error if critical processes didn't complete
func (pgm *ProcessGroupManager) Shutdown(reason string) error {
	// Set shutdown flag atomically
	if !atomic.CompareAndSwapInt32(&pgm.shutdownFlag, 0, 1) {
		return nil // Already shutting down
	}

	// Cancel shutdown context to notify all processes
	pgm.shutdownCancel()

	// Get all tracked subprocesses
	var subprocesses []*TrackedSubprocess
	_ = concurrency.RunInRLockWithLogger(
		&pgm.mu, LockNameProcessGroupShutdownCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			subprocesses = make([]*TrackedSubprocess, 0, len(pgm.subprocesses))
			for _, s := range pgm.subprocesses {
				subprocesses = append(subprocesses, s)
			}
			return nil
		},
	)

	// First, send SIGTERM to all process groups (graceful shutdown)
	for _, s := range subprocesses {
		if s.ProcessGroupID != 0 && s.KillFunc == nil {
			// Create kill function if not provided
			pgID := s.ProcessGroupID
			s.KillFunc = func() error {
				return syscall.Kill(pgID, syscall.SIGTERM)
			}
		}
		if s.KillFunc != nil {
			_ = s.KillFunc() //nolint:errcheck // Best effort - try graceful first
		}
	}

	// Wait for graceful shutdown or timeout. Use a fresh context so we actually wait;
	// pgm.shutdownCtx is already cancelled above, so WithTimeout(pgm.shutdownCtx, ...) would fire immediately.
	gracefulDone := make(chan bool, 1)
	gracefulCtx, cancel := context.WithTimeout(context.Background(), pgm.shutdownTimeout/2) // Background: request-or-shutdown derived
	defer cancel()
	goroutinelabels.NewGoroutine("scheduler_graceful_shutdown_timer", "waiting for graceful process shutdown").
		StartSimple(func() {
			select {
			case <-gracefulCtx.Done():
				select {
				case gracefulDone <- true:
				default:
				}
			case <-time.After(pgm.shutdownTimeout / 2):
				select {
				case gracefulDone <- true:
				default:
				}
			}
		})

	select {
	case <-gracefulDone:
		// Check if processes are still running
		var stillRunning []*TrackedSubprocess
		_ = concurrency.RunInRLockWithLogger(
			&pgm.mu, LockNameProcessGroupCheckRunning, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				stillRunning = make([]*TrackedSubprocess, 0)
				for _, s := range pgm.subprocesses {
					// Check if process is still running (simplified - in real implementation, check process table)
					stillRunning = append(stillRunning, s)
				}
				return nil
			},
		)

		// Force kill any remaining processes
		for _, s := range stillRunning {
			if s.ProcessGroupID != 0 {
				// Force kill process group
				_ = syscall.Kill(s.ProcessGroupID, syscall.SIGKILL) //nolint:errcheck // Best effort
			}
		}
	case <-time.After(pgm.shutdownTimeout):
		// Timeout - force kill all
		for _, s := range subprocesses {
			if s.ProcessGroupID != 0 {
				_ = syscall.Kill(s.ProcessGroupID, syscall.SIGKILL) //nolint:errcheck // Best effort
			}
		}
	}

	// Check if critical processes are still running
	var criticalRunning []*TrackedSubprocess
	_ = concurrency.RunInRLockWithLogger(
		&pgm.mu, LockNameProcessGroupCheckCritical, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			criticalRunning = make([]*TrackedSubprocess, 0)
			for _, s := range pgm.subprocesses {
				if s.Critical {
					// In real implementation, check if process is actually still running
					criticalRunning = append(criticalRunning, s)
				}
			}
			return nil
		},
	)

	// If critical subprocesses are still running, return error
	if len(criticalRunning) > 0 {
		return errfmt.Errorf("critical subprocesses still running after shutdown: %v", criticalRunning)
	}

	return nil
}

// GetStatus returns the current status of all tracked subprocesses
func (pgm *ProcessGroupManager) GetStatus() ProcessGroupStatus {
	var subprocessStatuses []SubprocessStatus
	_ = concurrency.RunInRLockWithLogger(
		&pgm.mu, LockNameProcessGroupGetStatus, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			subprocessStatuses = make([]SubprocessStatus, 0, len(pgm.subprocesses))
			for _, s := range pgm.subprocesses {
				subprocessStatuses = append(subprocessStatuses, SubprocessStatus{
					ID:             s.ID,
					JobID:          s.JobID,
					Description:    s.Description,
					ProcessID:      s.ProcessID,
					ProcessGroupID: s.ProcessGroupID,
					Critical:       s.Critical,
					StartedAt:      s.StartedAt,
				})
			}
			return nil
		},
	)
	return ProcessGroupStatus{
		ShuttingDown:    pgm.IsShuttingDown(),
		Subprocesses:    subprocessStatuses,
		SubprocessCount: len(subprocessStatuses),
	}
}

// ProcessGroupStatus represents the status of a process group
type ProcessGroupStatus struct {
	ShuttingDown    bool
	Subprocesses    []SubprocessStatus
	SubprocessCount int
}

// SubprocessStatus represents the status of a tracked subprocess
type SubprocessStatus struct {
	ID             string
	JobID          string
	Description    string
	ProcessID      int
	ProcessGroupID int
	Critical       bool
	StartedAt      time.Time
}
