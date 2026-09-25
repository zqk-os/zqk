package overseer

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"golang.org/x/sys/unix"
)

// ProcessGroupManager manages process group membership, child tracking,
// subreaper registration, and atomic group signaling.
type ProcessGroupManager struct {
	mu           sync.RWMutex
	pgid         int
	isLeader     bool
	children     map[int]*exec.Cmd
	subreaperSet bool
}

// NewProcessGroupManager creates a new ProcessGroupManager.
// If makeLeader is true, this process establishes itself as a process group leader.
func NewProcessGroupManager(makeLeader bool) (*ProcessGroupManager, error) {
	mgr := &ProcessGroupManager{
		children: make(map[int]*exec.Cmd),
	}
	pid := os.Getpid()
	if makeLeader {
		if err := unix.Setpgid(pid, pid); err != nil && err != unix.EACCES && err != unix.EPERM {
			existingPgid, getErr := unix.Getpgid(pid)
			if getErr != nil {
				return nil, errfmt.Newf("setpgid for process group manager").Wrap(err)
			}
			mgr.pgid = existingPgid
		} else {
			mgr.pgid = pid
		}
		mgr.isLeader = true
	} else {
		existingPgid, err := unix.Getpgid(pid)
		if err != nil {
			return nil, errfmt.Newf("getpgid for process").Wrap(err)
		}
		mgr.pgid = existingPgid
	}
	return mgr, nil
}

// PGID returns the managed process group ID.
func (m *ProcessGroupManager) PGID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pgid
}

// IsLeader returns true if this manager established the process group.
func (m *ProcessGroupManager) IsLeader() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isLeader
}

// PrepareCommand configures a command to join or lead a process group.
// If setOwnPGID is true, the child will become its own process group leader (setpgid(0, 0)),
// allowing atomic signaling of that specific child and all its descendants.
// If false and manager is leader, child joins the manager's PGID.
func (m *ProcessGroupManager) PrepareCommand(cmd *exec.Cmd, setOwnPGID bool) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	if setOwnPGID {
		cmd.SysProcAttr.Setpgid = true
	} else if m.isLeader && m.pgid > 0 {
		cmd.SysProcAttr.Setpgid = true
		cmd.SysProcAttr.Pgid = m.pgid
	}
}

// RegisterChild tracks a started child process under management.
func (m *ProcessGroupManager) RegisterChild(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.children[cmd.Process.Pid] = cmd
}

// UnregisterChild removes a child from tracking.
func (m *ProcessGroupManager) UnregisterChild(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.children, pid)
}

// TrackedChildren returns a slice of currently tracked child PIDs.
func (m *ProcessGroupManager) TrackedChildren() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pids := make([]int, 0, len(m.children))
	for pid := range m.children {
		pids = append(pids, pid)
	}
	return pids
}

// SignalGroup sends a signal to all processes in the specified PGID.
// If targetPgid <= 0, the manager's PGID is used.
func (m *ProcessGroupManager) SignalGroup(targetPgid int, sig syscall.Signal) error {
	pgid := targetPgid
	if pgid <= 0 {
		pgid = m.PGID()
	}
	if pgid <= 1 {
		return errfmt.Errorf("refusing to signal invalid or root PGID %d", pgid)
	}
	if err := unix.Kill(-pgid, unix.Signal(sig)); err != nil {
		if err == unix.ESRCH || err == unix.EPERM {
			return nil // Process group already terminated or zombie in BSD
		}
		return errfmt.Newf("kill -pgid %d signal %d", pgid, sig).Wrap(err)
	}
	return nil
}

// TerminateGroup sends SIGTERM to the process group, waits for gracePeriod,
// and sends SIGKILL if any processes remain alive.
func (m *ProcessGroupManager) TerminateGroup(ctx context.Context, targetPgid int, gracePeriod time.Duration) error {
	pgid := targetPgid
	if pgid <= 0 {
		pgid = m.PGID()
	}
	if pgid <= 1 {
		return errfmt.Errorf("refusing to terminate invalid or root PGID %d", pgid)
	}

	// 1. Send SIGTERM
	_ = m.SignalGroup(pgid, syscall.SIGTERM)

	// 2. Poll for termination up to gracePeriod
	deadline := time.Now().Add(gracePeriod)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			_ = m.SignalGroup(pgid, syscall.SIGKILL)
			return ctx.Err()
		default:
		}

		err := unix.Kill(-pgid, 0)
		if err == unix.ESRCH || err == unix.EPERM {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}

	// 3. Force kill with SIGKILL
	if err := m.SignalGroup(pgid, syscall.SIGKILL); err != nil && err != unix.ESRCH && err != unix.EPERM {
		return err
	}
	return nil
}

// EnableSubreaper configures the current process as a child subreaper on supported platforms.
func (m *ProcessGroupManager) EnableSubreaper() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := setSubreaper(); err != nil {
		return errfmt.Newf("enable child subreaper").Wrap(err)
	}
	m.subreaperSet = true
	return nil
}

// IsSubreaperSupported returns whether child subreaping is supported on this platform.
func (m *ProcessGroupManager) IsSubreaperSupported() bool {
	return isSubreaperSupported()
}

// IsSubreaperEnabled returns whether subreaper mode was enabled.
func (m *ProcessGroupManager) IsSubreaperEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subreaperSet
}

// ReapedProcess holds info about a reaped child or orphaned descendant.
type ReapedProcess struct {
	PID    int
	Status syscall.WaitStatus
}

// ReapZombies performs a non-blocking wait4 on -1 to harvest any terminated
// child processes or adopted orphans, preventing zombie accumulation.
func (m *ProcessGroupManager) ReapZombies() []ReapedProcess {
	var reaped []ReapedProcess
	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, nil)
		if pid <= 0 || err != nil {
			break
		}
		m.UnregisterChild(pid)
		reaped = append(reaped, ReapedProcess{
			PID:    pid,
			Status: syscall.WaitStatus(ws),
		})
	}
	return reaped
}
