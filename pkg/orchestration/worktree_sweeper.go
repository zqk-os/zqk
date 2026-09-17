package orchestration

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// WorktreeSweeper detects and prunes expired or abandoned worktree leases.
type WorktreeSweeper struct {
	pool *WorktreePool
}

// NewWorktreeSweeper instantiates a sweeper tied to a worktree pool.
func NewWorktreeSweeper(pool *WorktreePool) *WorktreeSweeper {
	return &WorktreeSweeper{pool: pool}
}

// Sweep examines active allocations and prunes those past expiry.
func (s *WorktreeSweeper) Sweep(now time.Time) (int, error) {
	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()

	pruned := 0
	for id, handle := range s.pool.allocations {
		if now.After(handle.LeaseExpiry) {
			_ = fileutil.RemoveAll(handle.Path)
			delete(s.pool.allocations, id)
			pruned++
		}
	}

	return pruned, nil
}

// SweepContext runs a sweep honoring context cancellation.
func (s *WorktreeSweeper) SweepContext(ctx context.Context, now time.Time) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
		return s.Sweep(now)
	}
}
