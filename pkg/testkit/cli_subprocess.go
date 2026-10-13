package testkit

import (
	"context"
	"os/exec"
	"testing"

	"github.com/zqk-os/zqk/pkg/testkit/managedcmd"
)

// WireCLISubprocessForIsolatedProject configures cmd.Dir and cmd.Env for a child zqk process
// rooted at projectRoot. Use this together with [PrepareIsolatedTempProject] (pipeline stages
// under isolatedTempProjectPipelinePrefix) so subprocesses do not inherit ZQK_PROJECT_ROOT or
// duplicate ZQK_TEST_ROOT from the parent environment.
func WireCLISubprocessForIsolatedProject(cmd *exec.Cmd, projectRoot string) {
	managedcmd.WireCLISubprocessForIsolatedProject(cmd, projectRoot)
}

// ManagedCommand creates an exec.Cmd bound to ctx, isolated in its own process group,
// and registered with t.Cleanup to guarantee full process tree termination upon test exit.
// This prevents orphan compiler, daemon, or tool processes from consuming CPU in the background.
func ManagedCommand(t testing.TB, ctx context.Context, name string, args ...string) *exec.Cmd {
	t.Helper()
	return managedcmd.ManagedCommand(t, ctx, name, args...)
}

// KillManagedProcessGroup terminates the process group associated with cmd.
func KillManagedProcessGroup(cmd *exec.Cmd) error {
	return managedcmd.KillManagedProcessGroup(cmd)
}
