package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
)

// SupervisorAdapter manages services directly as child processes (suitable for containers/CI/tests).
type SupervisorAdapter struct {
	mu        sync.RWMutex
	specs     map[string]ServiceSpec
	processes map[string]*supervisedProc
}

type supervisedProc struct {
	cmd       *exec.Cmd
	startedAt time.Time
	cancel    context.CancelFunc
}

// NewSupervisorAdapter creates a process-group based supervisor adapter.
func NewSupervisorAdapter() *SupervisorAdapter {
	return &SupervisorAdapter{
		specs:     make(map[string]ServiceSpec),
		processes: make(map[string]*supervisedProc),
	}
}

func (s *SupervisorAdapter) Name() string {
	return "supervisor"
}

func (s *SupervisorAdapter) IsAvailable() bool {
	return true
}

func (s *SupervisorAdapter) Install(ctx context.Context, spec ServiceSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.specs[spec.ID]; exists {
		return fmt.Errorf("%w: %s", ErrServiceAlreadyExists, spec.ID)
	}

	s.specs[spec.ID] = spec

	if spec.RunAtLoad {
		go func() {
			_ = s.startLocked(spec.ID)
		}()
	}

	return nil
}

func (s *SupervisorAdapter) Uninstall(ctx context.Context, id string) error {
	_ = s.Stop(ctx, id)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.specs[id]; !exists {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}

	delete(s.specs, id)
	return nil
}

func (s *SupervisorAdapter) Start(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked(id)
}

func (s *SupervisorAdapter) startLocked(id string) error {
	spec, exists := s.specs[id]
	if !exists {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}

	if proc, ok := s.processes[id]; ok && proc.cmd != nil && proc.cmd.Process != nil {
		if proc.cmd.ProcessState == nil || !proc.cmd.ProcessState.Exited() {
			return fmt.Errorf("%w: %s", ErrServiceAlreadyActive, id)
		}
	}

	cmdCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(cmdCtx, spec.Executable, spec.Arguments...)

	if spec.WorkingDir != "" {
		cmd.Dir = spec.WorkingDir
	}

	if len(spec.Environment) > 0 {
		env := os.Environ()
		for k, v := range spec.Environment {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	if spec.StandardOutPath != "" {
		if f, err := os.OpenFile(spec.StandardOutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, paths.FilePerm644); err == nil {
			cmd.Stdout = f
		}
	}
	if spec.StandardErrorPath != "" {
		if f, err := os.OpenFile(spec.StandardErrorPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, paths.FilePerm644); err == nil {
			cmd.Stderr = f
		}
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("spawn process for %s: %w", id, err)
	}

	proc := &supervisedProc{
		cmd:       cmd,
		startedAt: time.Now(),
		cancel:    cancel,
	}
	s.processes[id] = proc

	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		if current, ok := s.processes[id]; ok && current == proc {
			delete(s.processes, id)
		}
	}()

	return nil
}

func (s *SupervisorAdapter) Stop(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	proc, ok := s.processes[id]
	if !ok || proc.cmd == nil || proc.cmd.Process == nil {
		if _, specExists := s.specs[id]; !specExists {
			return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
		}
		return fmt.Errorf("%w: %s", ErrServiceNotRunning, id)
	}

	proc.cancel()
	_ = proc.cmd.Process.Signal(syscall.SIGTERM)

	delete(s.processes, id)
	return nil
}

func (s *SupervisorAdapter) Restart(ctx context.Context, id string) error {
	_ = s.Stop(ctx, id)
	return s.Start(ctx, id)
}

func (s *SupervisorAdapter) Status(ctx context.Context, id string) (ServiceStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st := ServiceStatus{
		ID:    id,
		State: StateStopped,
	}

	if _, exists := s.specs[id]; !exists {
		return st, nil
	}

	proc, ok := s.processes[id]
	if !ok || proc.cmd == nil || proc.cmd.Process == nil {
		return st, nil
	}

	st.State = StateRunning
	st.PID = proc.cmd.Process.Pid
	st.Uptime = time.Since(proc.startedAt)

	return st, nil
}

func (s *SupervisorAdapter) CleanupLegacy(ctx context.Context, legacyIDs []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var cleaned []string
	for _, id := range legacyIDs {
		if _, exists := s.specs[id]; exists {
			if proc, ok := s.processes[id]; ok && proc.cancel != nil {
				proc.cancel()
				delete(s.processes, id)
			}
			delete(s.specs, id)
			cleaned = append(cleaned, id)
		}
	}
	return cleaned, nil
}
