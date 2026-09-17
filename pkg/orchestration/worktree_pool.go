package orchestration

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// WorktreeHandle encapsulates an allocated ephemeral worktree.
type WorktreeHandle struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	AgentID     string    `json:"agent_id"`
	LeaseExpiry time.Time `json:"lease_expiry"`
	InUse       bool      `json:"in_use"`
}

// WorktreePool manages a pool of isolated, ephemeral git worktrees.
type WorktreePool struct {
	mu          sync.Mutex
	baseDir     string
	capacity    int
	counter     uint64
	closed      bool
	allocations map[string]*WorktreeHandle
}

// NewWorktreePool creates a worktree pool under baseDir.
func NewWorktreePool(baseDir string, capacity int) *WorktreePool {
	return &WorktreePool{
		baseDir:     baseDir,
		capacity:    capacity,
		allocations: make(map[string]*WorktreeHandle),
	}
}

// Acquire requests an exclusive worktree handle with a lease duration.
func (p *WorktreePool) Acquire(ctx context.Context, agentID string, leaseDuration time.Duration) (*WorktreeHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, errors.New("worktree pool is closed")
	}

	if len(p.allocations) >= p.capacity {
		return nil, errors.New("worktree pool capacity exceeded")
	}

	seq := atomic.AddUint64(&p.counter, 1)
	handleID := fmt.Sprintf("wt-%d-%d", time.Now().UnixNano(), seq)
	wtPath := filepath.Join(p.baseDir, handleID)

	if err := fileutil.MkdirAll(wtPath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %w", err)
	}

	handle := &WorktreeHandle{
		ID:          handleID,
		Path:        wtPath,
		AgentID:     agentID,
		LeaseExpiry: time.Now().Add(leaseDuration),
		InUse:       true,
	}

	p.allocations[handleID] = handle
	return handle, nil
}

// Release yields the worktree back to the pool and removes the ephemeral directory.
func (p *WorktreePool) Release(ctx context.Context, handleID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	handle, exists := p.allocations[handleID]
	if !exists {
		return errors.New("handle not found in pool")
	}

	delete(p.allocations, handleID)
	_ = fileutil.RemoveAll(handle.Path)
	return nil
}

// ActiveCount returns the number of worktrees currently checked out.
func (p *WorktreePool) ActiveCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.allocations)
}

// Close terminates the pool and cleans up all active allocations.
func (p *WorktreePool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	for id, handle := range p.allocations {
		_ = fileutil.RemoveAll(handle.Path)
		delete(p.allocations, id)
	}
	return nil
}
