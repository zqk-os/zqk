package testkit

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// WireCLISubprocessForIsolatedProject configures cmd.Dir and cmd.Env for a child zqk process
// rooted at projectRoot. Use this together with [PrepareIsolatedTempProject] (pipeline stages
// under isolatedTempProjectPipelinePrefix) so subprocesses do not inherit ZQK_PROJECT_ROOT or
// duplicate ZQK_TEST_ROOT from the parent environment.
func WireCLISubprocessForIsolatedProject(cmd *exec.Cmd, projectRoot string) {
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
}

// ManagedCommand creates an exec.Cmd bound to ctx, isolated in its own process group,
// and registered with t.Cleanup to guarantee full process tree termination upon test exit.
// This prevents orphan compiler, daemon, or tool processes from consuming CPU in the background.
func ManagedCommand(t testing.TB, ctx context.Context, name string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	setProcessGroup(cmd)
	cmd.Cancel = func() error {
		return killProcessGroup(cmd)
	}
	cmd.WaitDelay = 2 * time.Second

	t.Cleanup(func() {
		_ = killProcessGroup(cmd)
	})
	return cmd
}

// KillManagedProcessGroup terminates the process group associated with cmd.
func KillManagedProcessGroup(cmd *exec.Cmd) error {
	return killProcessGroup(cmd)
}
