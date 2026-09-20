package testkit

import (
	"os/exec"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// WireCLISubprocessForIsolatedProject configures cmd.Dir and cmd.Env for a child zqk process
// rooted at projectRoot. Use this together with [PrepareIsolatedTempProject] (pipeline stages
// under isolatedTempProjectPipelinePrefix) so subprocesses do not inherit ZQK_PROJECT_ROOT or
// duplicate ZQK_TEST_ROOT from the parent environment.
func WireCLISubprocessForIsolatedProject(cmd *exec.Cmd, projectRoot string) {
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
}
