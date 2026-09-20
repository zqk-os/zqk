package agent

import (
	"context"

	"github.com/zqk-os/zqk/pkg/adapters"
	"github.com/zqk-os/zqk/pkg/adapters/golang"
	"github.com/zqk-os/zqk/pkg/paths"
)

// worktree sandbox + pre-trunk build gate. Compiler names and package
// layouts live on the toolchain adapter, not here.

// isAgentWorktree reports whether root is an isolated ATK git worktree
// (default $TMPDIR/…/zqk-worktrees/…, never nested under the studio project).
func isAgentWorktree(root string) bool {
	return paths.IsAgentWorktreePath(root)
}

// agentWorktreeMainRepo returns the seated studio checkout for an isolated
// worktree. Parent-directory hops are not a mapping.
func agentWorktreeMainRepo(worktreeRoot string) (string, error) {
	return paths.AgentWorktreeMainRepo(worktreeRoot)
}

// worktreeBuildGate is the Go toolchain adapter. Other languages implement
// adapters.WorktreeBuildCheck; seating does not invoke `go` or `./cmd/...`.
var worktreeBuildGate adapters.WorktreeBuildCheck = golang.WorktreeBuildCheck{}

// worktreeBuildCheck runs the adapter gate before any merge into the trunk.
// Overridable in tests.
var worktreeBuildCheck = defaultWorktreeBuildCheck

func defaultWorktreeBuildCheck(ctx context.Context, worktreeRoot string) error {
	return worktreeBuildGate.Verify(ctx, worktreeRoot)
}
