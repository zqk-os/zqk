package overseer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// CommandExecutor represents the interface for launching subprocesses.
type CommandExecutor func(ctx context.Context, name string, args ...string) *exec.Cmd

// Supervisor runs the reconciliation loop, managing process lifecycles and crash recovery.
type Supervisor struct {
	mu          sync.Mutex
	projectRoot string
	registry    *Registry
	pgMgr       *ProcessGroupManager
	statuses    map[string]*DaemonStatus
	processes   map[string]*exec.Cmd
	cmdExecutor CommandExecutor
	stopCh      chan struct{}
	doneCh      chan struct{}
}

// NewSupervisor instantiates a daemon supervisor.
func NewSupervisor(projectRoot string, registry *Registry, pgMgr *ProcessGroupManager) *Supervisor {
	sup := &Supervisor{
		projectRoot: projectRoot,
		registry:    registry,
		pgMgr:       pgMgr,
		statuses:    make(map[string]*DaemonStatus),
		processes:   make(map[string]*exec.Cmd),
		cmdExecutor: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	sup.initStatuses()
	return sup
}

// SetCommandExecutor overrides the command launcher (primarily for testing).
func (s *Supervisor) SetCommandExecutor(fn CommandExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cmdExecutor = fn
}

func (s *Supervisor) initStatuses() {
	specs := s.registry.List()
	for _, spec := range specs {
		s.statuses[spec.Name] = &DaemonStatus{
			Name:         spec.Name,
			DesiredState: spec.DesiredState,
			ActualState:  ActualStateStopped,
		}
	}
}

// Start begins background reconciliation at the specified poll interval.
func (s *Supervisor) Start(ctx context.Context, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}

	// Initial reconciliation
	if err := s.Reconcile(ctx); err != nil {
		return err
	}

	go func() {
		defer close(s.doneCh)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.Reconcile(ctx)
				s.pgMgr.ReapZombies()
			}
		}
	}()

	return nil
}

// Stop terminates the supervisor and all managed daemons.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	select {
	case <-s.stopCh:
		s.mu.Unlock()
		return nil
	default:
		close(s.stopCh)
	}
	s.mu.Unlock()

	<-s.doneCh

	// Terminate all running processes
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, cmd := range s.processes {
		if cmd != nil && cmd.Process != nil {
			pgid := cmd.Process.Pid
			_ = s.pgMgr.TerminateGroup(ctx, pgid, 300*time.Millisecond)
		}
		if status, ok := s.statuses[name]; ok {
			status.ActualState = ActualStateStopped
			status.PID = 0
			status.PGID = 0
		}
	}
	s.processes = make(map[string]*exec.Cmd)
	return nil
}

// Reconcile evaluates all daemon specs, starting enabled ones and stopping disabled ones.
func (s *Supervisor) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	specs := s.registry.List()
	now := time.Now()

	for _, spec := range specs {
		status, exists := s.statuses[spec.Name]
		if !exists {
			status = &DaemonStatus{
				Name:         spec.Name,
				DesiredState: spec.DesiredState,
				ActualState:  ActualStateStopped,
			}
			s.statuses[spec.Name] = status
		}
		status.DesiredState = spec.DesiredState

		switch spec.DesiredState {
		case DesiredStateEnabled:
			if status.ActualState != ActualStateRunning {
				// Check backoff
				if now.Before(status.BackoffUntil) {
					status.ActualState = ActualStateBackoff
					continue
				}

				// Launch daemon
				if err := s.launchDaemonLocked(ctx, spec, status); err != nil {
					status.ActualState = ActualStateCrashed
					status.LastError = err.Error()
				}
			}

		case DesiredStateDisabled:
			if status.ActualState == ActualStateRunning {
				cmd, ok := s.processes[spec.Name]
				if ok && cmd != nil && cmd.Process != nil {
					_ = s.pgMgr.TerminateGroup(ctx, cmd.Process.Pid, 200*time.Millisecond)
				}
				delete(s.processes, spec.Name)
				status.ActualState = ActualStateStopped
				status.PID = 0
				status.PGID = 0
			} else if status.ActualState == ActualStateBackoff {
				status.ActualState = ActualStateStopped
				status.BackoffUntil = time.Time{}
			}
		}
	}

	return nil
}

func (s *Supervisor) launchDaemonLocked(ctx context.Context, spec *DaemonSpec, status *DaemonStatus) error {
	if len(spec.Command) == 0 {
		return errfmt.Errorf("daemon %s has empty command", spec.Name)
	}

	exe := spec.Command[0]
	var args []string
	if len(spec.Command) > 1 {
		args = spec.Command[1:]
	}

	// Resolve executable relative to projectRoot if needed
	if !filepath.IsAbs(exe) {
		binCandidate := filepath.Join(s.projectRoot, "bin", exe)
		if _, err := os.Stat(binCandidate); err == nil {
			exe = binCandidate
		}
	}

	cmd := s.cmdExecutor(ctx, exe, args...)
	if spec.WorkingDir != "" {
		cmd.Dir = spec.WorkingDir
	} else {
		cmd.Dir = s.projectRoot
	}

	if len(spec.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range spec.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Assign child to its own dedicated process group
	s.pgMgr.PrepareCommand(cmd, true)

	if err := cmd.Start(); err != nil {
		return errfmt.Newf("spawn daemon %s", spec.Name).Wrap(err)
	}

	pid := cmd.Process.Pid
	s.pgMgr.RegisterChild(cmd)
	s.processes[spec.Name] = cmd

	status.ActualState = ActualStateRunning
	status.PID = pid
	status.PGID = pid
	status.StartedAt = time.Now()
	status.LastError = ""

	// Launch async monitor for unexpected exit
	go s.monitorProcessExit(spec.Name, cmd, spec)

	return nil
}

func (s *Supervisor) monitorProcessExit(name string, cmd *exec.Cmd, spec *DaemonSpec) {
	waitErr := cmd.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up process tracking
	delete(s.processes, name)
	if cmd.Process != nil {
		s.pgMgr.UnregisterChild(cmd.Process.Pid)
	}

	status, exists := s.statuses[name]
	if !exists {
		return
	}

	status.ExitedAt = time.Now()
	status.PID = 0
	status.PGID = 0

	// If disabled or stopping, do not restart
	if status.DesiredState == DesiredStateDisabled {
		status.ActualState = ActualStateStopped
		return
	}

	// Handle exit and exponential backoff
	status.RestartCount++
	if waitErr != nil {
		status.LastError = waitErr.Error()
	}

	shift := status.RestartCount - 1
	if shift > 6 {
		shift = 6
	}
	backoff := spec.BackoffMin * time.Duration(1<<shift)
	if backoff > spec.BackoffMax {
		backoff = spec.BackoffMax
	}
	if backoff < spec.BackoffMin {
		backoff = spec.BackoffMin
	}

	status.BackoffUntil = time.Now().Add(backoff)
	status.ActualState = ActualStateBackoff
}

// Enable marks a daemon as enabled in the registry and triggers reconciliation.
func (s *Supervisor) Enable(ctx context.Context, name string) error {
	if err := s.registry.SetDesiredState(name, DesiredStateEnabled); err != nil {
		return err
	}
	return s.Reconcile(ctx)
}

// Disable marks a daemon as disabled in the registry and stops it if running.
func (s *Supervisor) Disable(ctx context.Context, name string) error {
	if err := s.registry.SetDesiredState(name, DesiredStateDisabled); err != nil {
		return err
	}
	return s.Reconcile(ctx)
}

// Restart stops and restarts a daemon.
func (s *Supervisor) Restart(ctx context.Context, name string) error {
	s.mu.Lock()
	cmd, ok := s.processes[name]
	if ok && cmd != nil && cmd.Process != nil {
		_ = s.pgMgr.TerminateGroup(ctx, cmd.Process.Pid, 200*time.Millisecond)
	}
	delete(s.processes, name)
	if status, exists := s.statuses[name]; exists {
		status.ActualState = ActualStateStopped
		status.PID = 0
		status.PGID = 0
		status.BackoffUntil = time.Time{}
	}
	s.mu.Unlock()

	return s.Reconcile(ctx)
}

// GetStatus returns the status of a single daemon.
func (s *Supervisor) GetStatus(name string) (*DaemonStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, ok := s.statuses[name]
	if !ok {
		return nil, errfmt.Errorf("daemon %q not found", name)
	}
	statusCopy := *status
	return &statusCopy, nil
}

// GetAllStatuses returns snapshots of all managed daemon statuses.
func (s *Supervisor) GetAllStatuses() []DaemonStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	statuses := make([]DaemonStatus, 0, len(s.statuses))
	for _, status := range s.statuses {
		statuses = append(statuses, *status)
	}
	return statuses
}
