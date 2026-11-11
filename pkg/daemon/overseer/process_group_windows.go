//go:build windows

package overseer

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// ProcessGroupManager is a degraded windows stub (no posix process groups).
// TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE G13 — windows residual for full pg semantics.
type ProcessGroupManager struct {
	mu       sync.RWMutex
	pgid     int
	isLeader bool
	children map[int]*exec.Cmd
}

func NewProcessGroupManager(makeLeader bool) (*ProcessGroupManager, error) {
	return &ProcessGroupManager{
		children: make(map[int]*exec.Cmd),
		isLeader: makeLeader,
		pgid:     0,
	}, nil
}

func (m *ProcessGroupManager) PGID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pgid
}

func (m *ProcessGroupManager) IsLeader() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isLeader
}

func (m *ProcessGroupManager) PrepareCommand(cmd *exec.Cmd, setOwnPGID bool) {
	_ = setOwnPGID
	if cmd == nil {
		return
	}
}

func (m *ProcessGroupManager) RegisterChild(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.children[cmd.Process.Pid] = cmd
}

func (m *ProcessGroupManager) UnregisterChild(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.children, pid)
}

func (m *ProcessGroupManager) TrackedChildren() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pids := make([]int, 0, len(m.children))
	for pid := range m.children {
		pids = append(pids, pid)
	}
	return pids
}

func (m *ProcessGroupManager) SignalGroup(targetPgid int, sig syscall.Signal) error {
	_ = targetPgid
	_ = sig
	return errfmt.Errorf("process groups unsupported on windows")
}

func (m *ProcessGroupManager) TerminateGroup(ctx context.Context, targetPgid int, gracePeriod time.Duration) error {
	_ = ctx
	_ = targetPgid
	_ = gracePeriod
	return errfmt.Errorf("process groups unsupported on windows")
}

func (m *ProcessGroupManager) EnableSubreaper() error { return nil }

func (m *ProcessGroupManager) IsSubreaperSupported() bool { return false }

func (m *ProcessGroupManager) IsSubreaperEnabled() bool { return false }

// ReapedProcess holds info about a reaped child (unused on windows stub).
type ReapedProcess struct {
	PID    int
	Status syscall.WaitStatus
}

func (m *ProcessGroupManager) ReapZombies() []ReapedProcess { return nil }
