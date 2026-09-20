package agent

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// worktree sandbox + pre-trunk build gate.

const (
	agentWorktreeMarker       = ".zqk/worktrees/"
	agentWorktreeTempMarker   = "zqk-worktrees/"
	worktreeBuildCheckTimeout = 3 * time.Minute
)

// isAgentWorktree reports whether root is under .zqk/worktrees/<ATK-*>.
func isAgentWorktree(root string) bool {
	return paths.IsAgentWorktreePath(root)
}

// agentWorktreeMainRepo returns the studio repo root for a .zqk/worktrees/<id> path.
func agentWorktreeMainRepo(worktreeRoot string) string {
	// .../.zqk/worktrees/<id> → three Dir hops to repo root
	return filepath.Dir(filepath.Dir(filepath.Dir(worktreeRoot)))
}

// worktreeBuildCheck runs a local syntactic/build gate in the agent worktree
// before any merge into the main trunk. Overridable in tests.
var worktreeBuildCheck = defaultWorktreeBuildCheck

func defaultWorktreeBuildCheck(ctx context.Context, worktreeRoot string) error {
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, worktreeBuildCheckTimeout)
	defer cancel()

	outBin := filepath.Join(fileutil.TempDir(), "zqk-worktree-build-check")
	cmd := execwrap.CommandContext(timeoutCtx, "go", "build", "-o", outBin, "./cmd/zqk")
	cmd.Dir = worktreeRoot
	out, err := cmd.CombinedOutput()
	_ = fileutil.Remove(outBin)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errfmt.Errorf("worktree build check failed: %s", truncateOutput(msg, 2000))
	}
	return nil
}

func truncateOutput(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
