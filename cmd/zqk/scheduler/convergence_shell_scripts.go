package scheduler

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

func runConvergencePromotionReadiness(cmd *cobra.Command, args []string) error {
	return runRepoBashScript(cmd, "scripts/check_convergence_promotion_readiness.sh", args)
}

func runConvergenceRecordOverseerRun(cmd *cobra.Command, args []string) error {
	return runRepoBashScript(cmd, "scripts/record_convergence_overseer_run.sh", args)
}

func runRepoBashScript(cmd *cobra.Command, scriptRel string, scriptArgs []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		return errfmt.Errorf("project root not found (run from a zqk project directory)")
	}
	scriptPath := filepath.Join(projectRoot, filepath.FromSlash(scriptRel))
	if _, err := fileutil.Stat(scriptPath); err != nil {
		return errfmt.Newf("script not found: %s", scriptRel).Wrap(err)
	}

	self, err := fileutil.Executable()
	if err != nil || strings.TrimSpace(self) == "" {
		self = "zqk"
	}

	runCtx := cli.CommandContextOr(cmd, context.Background()) // Background: request-or-shutdown derived
	bashArgv := append([]string{scriptPath}, scriptArgs...)
	c := execwrap.CommandContext(runCtx, "bash", bashArgv...) //nolint:gosec // argv from fixed script path + optional CVS id only
	zqkenv.WireExecForIsolatedProject(c, projectRoot)
	c.Env = os.Environ()
	c.Env = append(c.Env, "ZQK_PROJECT_ROOT="+projectRoot, "ZQK_BIN="+self)

	out := cli.CommandOutputWriter(cmd, nil)
	c.Stdout = out
	c.Stderr = out

	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return errfmt.Errorf("%s exited with code %d", filepath.Base(scriptRel), ee.ExitCode())
		}
		return errfmt.Newf("run %s", scriptRel).Wrap(err)
	}
	return nil
}
