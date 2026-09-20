package precommit

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/precommit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewLintReportCmd creates the pre-commit lint-report command; prints .zqk/pre-commit/lint-output.txt for viewing lint issues.
func NewLintReportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitLintReportCommandBuilder()
	cmd.RunE = runPreCommitLintReport
	return cmd
}

func runPreCommitLintReport(cmd *cobra.Command, _ []string) error {
	projectRoot, _ := cmd.Flags().GetString("project-root")
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from repo or pass --project-root")
	}
	path := precommit.LintOutputPath(projectRoot)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return cli.WriteOutput(cmd, []byte("No lint output yet. Run ./scripts/pre-commit-lint.sh to capture lint results, then run this command again to view them (e.g. to create backlog items).\n"))
		}
		return errfmt.Newf("read lint output").Wrap(err)
	}
	return cli.WriteOutput(cmd, data)
}
