package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/precommit"
)

// NewLintReportCmd creates the pre-commit lint-report command; prints .zqk/pre-commit/lint-output.txt for viewing lint issues.
func NewLintReportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitLintReportCommandBuilder()
	cmd.RunE = runPreCommitLintReport
	return cmd
}

func runPreCommitLintReport(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolvePreCommitProjectRoot(cmd)
	if err != nil {
		return err
	}
	path := precommit.LintOutputPath(projectRoot)
	missingMsg := "No lint output yet. Run ./scripts/pre-commit-lint.sh to capture lint results, then run this command again to view them (e.g. to create backlog items).\n"
	return outputPreCommitReportFile(cmd, path, missingMsg, "read lint output")
}
